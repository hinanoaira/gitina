package lfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorage_OIDValidationAndTraversal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina-lfs-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// 1. Invalid OID checks
	invalidOIDs := []string{
		"../../../../etc/passwd",
		"../abc",
		"short",
		strings.Repeat("g", 64), // not hex
		strings.Repeat("a", 63), // too short
		strings.Repeat("a", 65), // too long
		"",
	}

	for _, badOID := range invalidOIDs {
		t.Run("invalid_oid_"+badOID, func(t *testing.T) {
			if _, err := storage.Exists(ctx, "repo.git", badOID, 100); err == nil {
				t.Errorf("Exists should fail on invalid OID %q", badOID)
			}
			if err := storage.VerifyObject(ctx, "repo.git", badOID, 100); err == nil {
				t.Errorf("VerifyObject should fail on invalid OID %q", badOID)
			}
			if _, _, err := storage.GetUploadAction(ctx, "http://localhost", "repo.git", badOID, 100); err == nil {
				t.Errorf("GetUploadAction should fail on invalid OID %q", badOID)
			}
			if _, err := storage.GetDownloadAction(ctx, "http://localhost", "repo.git", badOID, 100); err == nil {
				t.Errorf("GetDownloadAction should fail on invalid OID %q", badOID)
			}

			// ServeObject HTTP 400 check
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/repo.git/info/lfs/objects/"+badOID, nil)
			storage.ServeObject(rec, req, "repo.git", badOID)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("ServeObject(%q) returned %d, want %d", badOID, rec.Code, http.StatusBadRequest)
			}

			// PutObject HTTP 400 check
			recPut := httptest.NewRecorder()
			reqPut := httptest.NewRequest(http.MethodPut, "/repo.git/info/lfs/objects/"+badOID, strings.NewReader("data"))
			storage.PutObject(recPut, reqPut, "repo.git", badOID)
			if recPut.Code != http.StatusBadRequest {
				t.Errorf("PutObject(%q) returned %d, want %d", badOID, recPut.Code, http.StatusBadRequest)
			}
		})
	}

	// 2. Valid OID should work normally
	validOID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	exists, err := storage.Exists(ctx, "repo.git", validOID, 0)
	if err != nil {
		t.Fatalf("Exists returned unexpected error on valid OID: %v", err)
	}
	if exists {
		t.Fatalf("object should not exist yet")
	}

	expectedPath := filepath.Join(tempDir, "01", "23", validOID)
	if storage.objectPath(validOID) != expectedPath {
		t.Errorf("objectPath = %q, want %q", storage.objectPath(validOID), expectedPath)
	}
}
