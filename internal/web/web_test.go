package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/repo"
)

func TestWebHandlerSPA(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_web_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoRoot := filepath.Join(tempDir, "repos")
	_ = os.MkdirAll(repoRoot, 0755)

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  repoRoot,
			BaseURL:   "http://localhost:8080",
			JWTSecret: "test-secret-key-32-bytes-long-test",
		},
		Admins: []string{"admin_user"},
	}

	aclMgr := acl.NewManager(cfg.Admins, cfg.Repositories)
	aclMgr.SetRepoRoot(repoRoot)
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	repoMgr := repo.NewManager(repoRoot)

	handler, err := NewHandler(cfg, aclMgr, tokenMgr, repoMgr)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	// 1. Root / access -> returns SPA index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHome(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for root, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Gitina") && !strings.Contains(body, "<html") {
		t.Fatalf("expected body to contain Gitina HTML, got:\n%s", body)
	}

	// 2. Deep repository path /org/repo.git -> SPA fallback returns 200 OK
	reqDeep := httptest.NewRequest(http.MethodGet, "/org/repo.git", nil)
	wDeep := httptest.NewRecorder()
	handler.ServeHTTP(wDeep, reqDeep)

	if wDeep.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for deep SPA route, got %d", wDeep.Code)
	}
	if !strings.Contains(wDeep.Body.String(), "<html") {
		t.Fatalf("expected HTML response for SPA fallback")
	}
}
