package lfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local lfs directory: %w", err)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) objectPath(oid string) string {
	oid = strings.ToLower(strings.TrimSpace(oid))
	if len(oid) < 4 {
		return filepath.Join(s.baseDir, "incomplete", oid)
	}
	return filepath.Join(s.baseDir, oid[0:2], oid[2:4], oid)
}

func (s *LocalStorage) Exists(ctx context.Context, repoPath, oid string, size int64) (bool, error) {
	path := s.objectPath(oid)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if size > 0 && info.Size() != size {
		return false, nil
	}
	return true, nil
}

func (s *LocalStorage) GetUploadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (*ActionLink, *ActionLink, error) {
	uploadHref := fmt.Sprintf("%s/%s/info/lfs/objects/%s", baseURL, repoPath, oid)
	verifyHref := fmt.Sprintf("%s/%s/info/lfs/objects/verify", baseURL, repoPath)

	uploadAction := &ActionLink{
		Href:      uploadHref,
		ExpiresIn: 86400,
	}
	verifyAction := &ActionLink{
		Href:      verifyHref,
		ExpiresIn: 86400,
	}
	return uploadAction, verifyAction, nil
}

func (s *LocalStorage) GetDownloadAction(ctx context.Context, baseURL, repoPath, oid string, size int64) (*ActionLink, error) {
	downloadHref := fmt.Sprintf("%s/%s/info/lfs/objects/%s", baseURL, repoPath, oid)
	return &ActionLink{
		Href:      downloadHref,
		ExpiresIn: 86400,
	}, nil
}

func (s *LocalStorage) VerifyObject(ctx context.Context, repoPath, oid string, size int64) error {
	path := s.objectPath(oid)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("object not found: %w", err)
	}
	if size > 0 && info.Size() != size {
		return fmt.Errorf("object size mismatch: expected %d, got %d", size, info.Size())
	}
	return nil
}

func (s *LocalStorage) ServeObject(w http.ResponseWriter, r *http.Request, repoPath, oid string) {
	path := s.objectPath(oid)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "object not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	io.Copy(w, file)
}

func (s *LocalStorage) PutObject(w http.ResponseWriter, r *http.Request, repoPath, oid string) {
	destPath := s.objectPath(oid)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		http.Error(w, "failed to create directory", http.StatusInternalServerError)
		return
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), "lfs_tmp_*")
	if err != nil {
		http.Error(w, "failed to create temporary file", http.StatusInternalServerError)
		return
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	written, err := io.Copy(writer, r.Body)
	tmpFile.Close()
	if err != nil {
		http.Error(w, "failed to write file content", http.StatusInternalServerError)
		return
	}

	actualOid := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualOid, oid) {
		http.Error(w, fmt.Sprintf("corrupt object: checksum mismatch (expected %s, got %s)", oid, actualOid), http.StatusBadRequest)
		return
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		http.Error(w, "failed to finalize object", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
	_ = written
}

// ListObjects walks the local storage directory and lists all LFS objects.
func (s *LocalStorage) ListObjects(ctx context.Context) ([]StorageObject, error) {
	var objects []StorageObject

	err := filepath.WalkDir(s.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		// Check if file is inside incomplete or is a temporary file
		name := d.Name()
		if strings.HasPrefix(name, "lfs_tmp_") {
			return nil
		}

		// A valid OID is a 64-character lowercase hex string
		if len(name) != 64 {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		objects = append(objects, StorageObject{
			OID:          name,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk local lfs storage: %w", err)
	}
	return objects, nil
}

// DeleteObjects deletes the specified objects from local storage.
func (s *LocalStorage) DeleteObjects(ctx context.Context, oids []string) error {
	for _, oid := range oids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		path := s.objectPath(oid)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete local lfs object %s: %w", oid, err)
		}

		// Attempt to remove parent empty subdirectories (e.g. 12/34/)
		parent := filepath.Dir(path)
		_ = os.Remove(parent)
		grandParent := filepath.Dir(parent)
		_ = os.Remove(grandParent)
	}
	return nil
}
