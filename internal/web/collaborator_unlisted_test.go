package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/repo"
)

func TestRepoCollaborator_KeepUnlistedVisibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina-collab-unlisted-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoDir := filepath.Join(tempDir, "my-repo.git")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	// Create dummy HEAD to mark bare repo
	if err := os.WriteFile(filepath.Join(repoDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

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

	// 1. Set repo visibility to "unlisted"
	err = aclMgr.UpdateVisibilityMetadata("my-repo.git", "unlisted", nil, "admin")
	if err != nil {
		t.Fatalf("failed to set unlisted: %v", err)
	}

	vis, _, _ := aclMgr.GetEffectiveVisibility("my-repo.git")
	if vis != "unlisted" {
		t.Fatalf("initial visibility should be unlisted, got %s", vis)
	}

	// 2. Add collaborator "alice" via API
	adminToken, _, _ := tokenMgr.GenerateAccessToken("admin")
	body := []byte(`{"username":"alice","role":"rw"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repos/my-repo.git/settings/collaborators", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{
		Name:  "gitina_session",
		Value: adminToken,
	})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handled := handler.ServeAPI(rec, req)
	if !handled || rec.Code != http.StatusOK {
		t.Fatalf("collaborator POST returned handled=%v, code=%d (body=%s)", handled, rec.Code, rec.Body.String())
	}

	// 3. Verify visibility is still "unlisted"
	vis, _, collabs := aclMgr.GetEffectiveVisibility("my-repo.git")
	if vis != "unlisted" {
		t.Fatalf("visibility changed unexpectedly to %s after adding collaborator! Want 'unlisted'", vis)
	}
	if collabs["alice"] != "rw" {
		t.Fatalf("expected alice to have rw access, got %v", collabs)
	}
}
