package lfs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"gitina/internal/config"
)

type S3Storage struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

func NewS3Storage(ctx context.Context, cfg config.LFSS3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("lfs.s3.bucket is required")
	}

	optFns := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		optFns = append(optFns, awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsConfig, err := awscfg.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	s3OptFns := func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	}

	client := s3.NewFromConfig(awsConfig, s3OptFns)
	presignClient := s3.NewPresignClient(client)

	return &S3Storage{
		client:        client,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
	}, nil
}

func (s *S3Storage) s3Key(oid string) string {
	oid = strings.ToLower(strings.TrimSpace(oid))
	if len(oid) < 4 {
		return "lfs/incomplete/" + oid
	}
	return fmt.Sprintf("lfs/%s/%s/%s", oid[0:2], oid[2:4], oid)
}

func (s *S3Storage) Exists(ctx context.Context, repoPath, oid string, size int64) (bool, error) {
	key := s.s3Key(oid)
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}

	if size > 0 && output.ContentLength != nil && *output.ContentLength != size {
		return false, nil
	}
	return true, nil
}

func (s *S3Storage) GetUploadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (*ActionLink, *ActionLink, error) {
	key := s.s3Key(oid)
	presignReq, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(24*time.Hour))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate s3 presigned upload url: %w", err)
	}

	uploadAction := &ActionLink{
		Href:      presignReq.URL,
		ExpiresIn: 86400,
	}

	// Verify URL points back to gitina so gitina can HeadObject S3
	verifyHref := fmt.Sprintf("%s/%s/info/lfs/objects/verify", baseURL, repoPath)
	verifyAction := &ActionLink{
		Href:      verifyHref,
		ExpiresIn: 86400,
	}

	return uploadAction, verifyAction, nil
}

func (s *S3Storage) GetDownloadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (*ActionLink, error) {
	key := s.s3Key(oid)
	presignReq, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to generate s3 presigned download url: %w", err)
	}

	return &ActionLink{
		Href:      presignReq.URL,
		ExpiresIn: 86400,
	}, nil
}

func (s *S3Storage) VerifyObject(ctx context.Context, repoPath, oid string, size int64) error {
	exists, err := s.Exists(ctx, repoPath, oid, size)
	if err != nil {
		return fmt.Errorf("failed to verify s3 object: %w", err)
	}
	if !exists {
		return errors.New("object not found on s3 or size mismatch")
	}
	return nil
}

func (s *S3Storage) ServeObject(w http.ResponseWriter, r *http.Request, repoPath, oid string) {
	http.Error(w, "direct object download is handled via S3 Presigned URL", http.StatusBadRequest)
}

func (s *S3Storage) PutObject(w http.ResponseWriter, r *http.Request, repoPath, oid string) {
	http.Error(w, "direct object upload is handled via S3 Presigned URL", http.StatusBadRequest)
}

// ListObjects lists all LFS objects under prefix "lfs/" in the S3 bucket.
func (s *S3Storage) ListObjects(ctx context.Context) ([]StorageObject, error) {
	var objects []StorageObject
	prefix := "lfs/"

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list s3 lfs objects: %w", err)
		}

		for _, item := range page.Contents {
			key := aws.ToString(item.Key)
			parts := strings.Split(key, "/")
			oid := parts[len(parts)-1]
			if len(oid) != 64 {
				continue
			}

			var modTime time.Time
			if item.LastModified != nil {
				modTime = *item.LastModified
			}

			objects = append(objects, StorageObject{
				OID:          oid,
				Size:         aws.ToInt64(item.Size),
				LastModified: modTime,
			})
		}
	}

	return objects, nil
}

// DeleteObjects deletes the specified objects in batches of up to 1000 from S3.
func (s *S3Storage) DeleteObjects(ctx context.Context, oids []string) error {
	if len(oids) == 0 {
		return nil
	}

	const batchSize = 1000
	for i := 0; i < len(oids); i += batchSize {
		end := i + batchSize
		if end > len(oids) {
			end = len(oids)
		}

		chunk := oids[i:end]
		var toDelete []types.ObjectIdentifier
		for _, oid := range chunk {
			toDelete = append(toDelete, types.ObjectIdentifier{
				Key: aws.String(s.s3Key(oid)),
			})
		}

		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{
				Objects: toDelete,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to batch delete s3 lfs objects: %w", err)
		}
	}

	return nil
}
