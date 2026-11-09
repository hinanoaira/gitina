package tiered

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"gitina/internal/config"
)

type S3RepoInfo struct {
	RepoPath  string
	Size      int64
	UpdatedAt time.Time
}

type S3Client struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3Client(ctx context.Context, cfg config.RepositoryStorageS3) (*S3Client, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("repository_storage.s3.bucket is required")
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
		return nil, fmt.Errorf("failed to load aws config for tiered storage: %w", err)
	}

	s3OptFns := func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "repos/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &S3Client{
		client: s3.NewFromConfig(awsConfig, s3OptFns),
		bucket: cfg.Bucket,
		prefix: prefix,
	}, nil
}

func (s *S3Client) objectKey(repoPath string) string {
	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}
	return s.prefix + repoPath + ".bundle"
}

// Exists checks if the repository bundle exists on S3.
func (s *S3Client) Exists(ctx context.Context, repoPath string) (bool, error) {
	key := s.objectKey(repoPath)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
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
	return true, nil
}

// DownloadBundle downloads the repository bundle from S3 to local file.
func (s *S3Client) DownloadBundle(ctx context.Context, repoPath, destFile string) error {
	key := s.objectKey(repoPath)
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download bundle from S3 key %q: %w", key, err)
	}
	defer resp.Body.Close()

	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return err
	}

	f, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to save bundle: %w", err)
	}

	return nil
}

// UploadBundle uploads a local bundle file to S3.
func (s *S3Client) UploadBundle(ctx context.Context, repoPath, srcFile string) error {
	key := s.objectKey(repoPath)
	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(info.Size()),
	})
	if err != nil {
		return fmt.Errorf("failed to upload bundle to S3 key %q: %w", key, err)
	}

	return nil
}

// DeleteBundle deletes or moves to .trash/ on S3.
func (s *S3Client) DeleteBundle(ctx context.Context, repoPath string, force bool) error {
	key := s.objectKey(repoPath)

	if force {
		_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		return err
	}

	// Move to .trash/ on S3
	trashKey := s.prefix + ".trash/" + fmt.Sprintf("%d_", time.Now().Unix()) + strings.ReplaceAll(repoPath, "/", "_") + ".bundle"

	// Copy to trash
	copySource := s.bucket + "/" + key
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(trashKey),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		return fmt.Errorf("failed to copy bundle to trash in S3: %w", err)
	}

	// Delete original
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// ListRepos lists all repository bundles stored on S3.
func (s *S3Client) ListRepos(ctx context.Context) ([]S3RepoInfo, error) {
	var list []S3RepoInfo

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			// Skip .trash/
			if strings.Contains(key, "/.trash/") {
				continue
			}
			if !strings.HasSuffix(key, ".bundle") {
				continue
			}

			rel := strings.TrimPrefix(key, s.prefix)
			rel = strings.TrimSuffix(rel, ".bundle")

			var size int64
			if obj.Size != nil {
				size = *obj.Size
			}
			var updated time.Time
			if obj.LastModified != nil {
				updated = *obj.LastModified
			}

			list = append(list, S3RepoInfo{
				RepoPath:  rel,
				Size:      size,
				UpdatedAt: updated,
			})
		}
	}

	return list, nil
}
