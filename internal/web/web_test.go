package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git/tiered"
	"gitina/internal/repo"
)

func TestWebHandler(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_web_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoRoot := filepath.Join(tempDir, "repos")
	_ = os.MkdirAll(repoRoot, 0755)

	repoPath := filepath.Join(repoRoot, "sample", "repo.git")
	_ = os.MkdirAll(repoPath, 0755)

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v failed: %s: %v", name, args, string(out), err)
		}
	}

	runCmd(tempDir, "git", "init", "--bare", repoPath)

	workDir := filepath.Join(tempDir, "work")
	runCmd(tempDir, "git", "clone", repoPath, workDir)
	runCmd(workDir, "git", "config", "commit.gpgsign", "false")
	_ = os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Sample Repo"), 0644)
	runCmd(workDir, "git", "add", ".")
	runCmd(workDir, "git", "commit", "-m", "Init")
	runCmd(workDir, "git", "branch", "-M", "main")
	runCmd(workDir, "git", "push", "origin", "main")

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  repoRoot,
			BaseURL:   "http://localhost:8080",
			JWTSecret: "test-secret-key-32-bytes-long-test",
		},
		Admins: []string{"admin_user"},
		Repositories: []config.RepositoryConfig{
			{
				Path:   "sample/repo.git",
				Public: true,
			},
		},
	}

	aclMgr := acl.NewManager(cfg.Admins, cfg.Repositories)
	aclMgr.SetRepoRoot(repoRoot)
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	tieredMgr, _ := tiered.NewManager(context.Background(), cfg)
	repoMgr := repo.NewManager(repoRoot, tieredMgr)

	handler, err := NewHandler(cfg, aclMgr, tokenMgr, repoMgr, tieredMgr)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	// 1. Anonymous access to public repo -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/sample/repo.git", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for anonymous public access, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "sample/repo.git") || !strings.Contains(body, "Public") {
		t.Fatalf("expected body to contain repo info and Public badge, got:\n%s", body)
	}

	// 2. Admin access via session cookie -> displays Settings tab
	adminToken, _, _ := tokenMgr.GenerateAccessToken("admin_user")
	reqAdmin := httptest.NewRequest(http.MethodGet, "/sample/repo.git?tab=settings", nil)
	reqAdmin.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: adminToken,
	})
	wAdmin := httptest.NewRecorder()
	handler.ServeHTTP(wAdmin, reqAdmin)

	if wAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin settings, got %d", wAdmin.Code)
	}
	adminBody := wAdmin.Body.String()
	if !strings.Contains(adminBody, "共同作業者 (Collaborators) の管理") || !strings.Contains(adminBody, "Danger Zone") {
		t.Fatalf("expected admin settings in body, got:\n%s", adminBody)
	}

	// 3. Admin switches public -> private
	reqToggle := httptest.NewRequest(http.MethodPost, "/sample/repo.git/settings/update-visibility", strings.NewReader("public=false"))
	reqToggle.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqToggle.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: adminToken,
	})
	wToggle := httptest.NewRecorder()
	handler.ServeHTTP(wToggle, reqToggle)

	if wToggle.Code != http.StatusFound {
		t.Fatalf("expected redirect after visibility update, got %d", wToggle.Code)
	}

	// Verify repo is now private: anonymous access -> redirect to login (302 Found)
	reqAnonPrivate := httptest.NewRequest(http.MethodGet, "/sample/repo.git", nil)
	wAnonPrivate := httptest.NewRecorder()
	handler.ServeHTTP(wAnonPrivate, reqAnonPrivate)

	if wAnonPrivate.Code != http.StatusFound {
		t.Fatalf("expected redirect to login for private repo, got %d", wAnonPrivate.Code)
	}
}
