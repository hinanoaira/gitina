package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git/tiered"
)

func TestCloneUncreatedRepoAutoInit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_auto_init_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoRoot := filepath.Join(tempDir, "repos")
	_ = os.MkdirAll(repoRoot, 0755)

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  repoRoot,
			JWTSecret: "test-secret",
			BaseURL:   "http://localhost:8080",
		},
		Admins: []string{"admin_user"},
		RepositoryStorage: config.RepositoryStorageConfig{
			Type: "local",
		},
	}

	aclMgr := acl.NewManager(cfg.Admins, nil)
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	gcmHandler := auth.NewGCMHandler(cfg, tokenMgr, aclMgr)
	tieredMgr, err := tiered.NewManager(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	handler, err := NewHandler(cfg, aclMgr, gcmHandler, nil, tieredMgr)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Simulate admin attempting to clone a non-existent repo: /nested/brand/new.git
	token, _, err := tokenMgr.GenerateAccessToken("admin_user")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/nested/brand/new.git/info/refs?service=git-upload-pack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK on cloning uncreated repo, got %d", resp.StatusCode)
	}

	// 2. Check bare repository was automatically initialized
	expectedBareRepo := filepath.Join(repoRoot, "nested", "brand", "new.git")
	headPath := filepath.Join(expectedBareRepo, "HEAD")
	if _, err := os.Stat(headPath); os.IsNotExist(err) {
		t.Fatalf("bare repo HEAD was not created at %q", headPath)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/x-git-upload-pack-advertisement" {
		t.Fatalf("unexpected content type: %s", contentType)
	}
}
