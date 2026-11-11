package web

import (
	"encoding/json"
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
	"gitina/internal/repo"
)

func setupTestAPIEnv(t *testing.T) (*Handler, string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "gitina-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	repoRoot := filepath.Join(tmpDir, "repos")
	_ = os.MkdirAll(repoRoot, 0755)

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

	// Create a sample bare repository with an initial commit
	sampleRepoPath := filepath.Join(repoRoot, "test-api.git")
	_ = os.MkdirAll(sampleRepoPath, 0755)
	runCmd(tmpDir, "git", "init", "--bare", sampleRepoPath)

	// Make a non-bare repo, push to bare
	workDir := filepath.Join(tmpDir, "work")
	runCmd(tmpDir, "git", "clone", sampleRepoPath, workDir)
	runCmd(workDir, "git", "config", "commit.gpgsign", "false")
	_ = os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Hello API"), 0644)
	_ = os.WriteFile(filepath.Join(workDir, "code.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	runCmd(workDir, "git", "add", ".")
	runCmd(workDir, "git", "commit", "-m", "initial commit")
	runCmd(workDir, "git", "branch", "-M", "main")
	runCmd(workDir, "git", "push", "origin", "main")
	runCmd(sampleRepoPath, "git", "symbolic-ref", "HEAD", "refs/heads/main")

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  repoRoot,
			BaseURL:   "http://localhost:8080",
			JWTSecret: "test-secret-long-enough-for-hs256-signing-must-be-32-bytes",
		},
		Admins: []string{"admin_user"},
		Repositories: []config.RepositoryConfig{
			{
				Path:   "test-api.git",
				Public: true,
			},
		},
	}

	aclMgr := acl.NewManager(cfg.Admins, cfg.Repositories)
	aclMgr.SetRepoRoot(repoRoot)
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	repoMgr := repo.NewManager(repoRoot, nil)

	handler, err := NewHandler(cfg, aclMgr, tokenMgr, repoMgr, nil)
	if err != nil {
		t.Fatalf("failed to create web handler: %v", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return handler, repoRoot, cleanup
}

func TestAPIReposAndTree(t *testing.T) {
	handler, _, cleanup := setupTestAPIEnv(t)
	defer cleanup()

	// 1. Test /api/v1/auth/me (unauthenticated)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	handled := handler.ServeAPI(w, req)
	if !handled {
		t.Fatalf("ServeAPI did not handle /api/v1/auth/me")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var meResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &meResp)
	if meResp["is_admin"] != false || meResp["user"] != "" {
		t.Errorf("unexpected meResp: %v", meResp)
	}

	// 2. Test /api/v1/repos
	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var reposResp map[string][]map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &reposResp)
	if len(reposResp["repos"]) == 0 {
		t.Errorf("expected at least 1 repo")
	}

	// 3. Test /api/v1/repos/test-api.git info
	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos/test-api.git", nil)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var infoResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &infoResp)
	if infoResp["path"] != "test-api.git" || infoResp["default_branch"] != "main" {
		t.Errorf("unexpected infoResp: %v", infoResp)
	}

	// 4. Test /api/v1/repos/test-api.git/tree
	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos/test-api.git/tree?ref=main", nil)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var treeResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &treeResp)
	entries, ok := treeResp["entries"].([]interface{})
	if !ok || len(entries) < 2 {
		t.Errorf("expected at least 2 entries in tree, got %v", treeResp)
	}
	readme, _ := treeResp["readme"].(map[string]interface{})
	if readme == nil || !strings.Contains(readme["content"].(string), "Hello API") {
		t.Errorf("expected README with Hello API, got %v", readme)
	}

	// 5. Test /api/v1/repos/test-api.git/blob?path=code.go
	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos/test-api.git/blob?ref=main&path=code.go", nil)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var blobResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &blobResp)
	blob, _ := blobResp["blob"].(map[string]interface{})
	if blob == nil || !strings.Contains(blob["content"].(string), "package main") {
		t.Errorf("expected blob with package main, got %v", blobResp)
	}

	// 6. Test /api/v1/repos/test-api.git/commits
	req = httptest.NewRequest(http.MethodGet, "/api/v1/repos/test-api.git/commits?ref=main", nil)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var commitsResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &commitsResp)
	commits, _ := commitsResp["commits"].([]interface{})
	if len(commits) == 0 {
		t.Errorf("expected at least 1 commit, got 0")
	}

	// 7. Test /api/v1/repos/test-api.git/tokens (PAT) with logged in user
	wSession := httptest.NewRecorder()
	handler.SetUserSession(wSession, "admin_user")
	cookies := wSession.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie, got none")
	}
	sessionCookie := cookies[0]

	tokenPayload := `{"days": "30", "scope": "rw"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/repos/test-api.git/tokens", strings.NewReader(tokenPayload))
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var tokenResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &tokenResp)
	if tokenResp["token"] == "" || tokenResp["scope"] != "rw" {
		t.Errorf("unexpected tokenResp: %v", tokenResp)
	}

	// 8. Test /api/v1/repos/test-api.git/settings/visibility (Admin)
	visPayload := `{"is_public": false}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/repos/test-api.git/settings/visibility", strings.NewReader(visPayload))
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	handler.ServeAPI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var visResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &visResp)
	if visResp["is_public"] != false {
		t.Errorf("expected is_public to be false, got %v", visResp)
	}
}
