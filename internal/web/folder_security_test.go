package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/repo"
)

func TestFolderAPI_PathTraversalPrevention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina-web-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot: tempDir,
			BaseURL:  "http://localhost:8080",
		},
		Admins: []string{"admin"},
	}

	aclMgr := acl.NewManager(cfg.Admins, nil)
	aclMgr.SetRepoRoot(tempDir)
	tokenMgr := auth.NewTokenManager("secret-key-1234567890123456")
	repoMgr := repo.NewManager(tempDir)

	handler, err := NewHandler(cfg, aclMgr, tokenMgr, repoMgr)
	if err != nil {
		t.Fatalf("failed to create web handler: %v", err)
	}

	traversalPaths := []string{
		"../../outside",
		"folder/../../outside",
		"/../etc/passwd",
		"..",
	}

	adminToken, _, _ := tokenMgr.GenerateAccessToken("admin")

	for _, p := range traversalPaths {
		t.Run("folder_traversal_"+p, func(t *testing.T) {
			// POST /api/v1/folders/visibility
			body := []byte(`{"path":"` + p + `","visibility":"public"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/folders/visibility", bytes.NewReader(body))
			req.AddCookie(&http.Cookie{
				Name:  "gitina_session",
				Value: adminToken,
			})
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handled := handler.ServeAPI(rec, req)
			if !handled {
				t.Fatalf("expected API to handle request")
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("path %q returned HTTP %d, want %d", p, rec.Code, http.StatusBadRequest)
			}
		})
	}
}
