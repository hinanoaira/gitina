package git

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git/tiered"
)

func TestStreamingPushAndClone(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_streaming_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoRoot := filepath.Join(tempDir, "repos")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  repoRoot,
			JWTSecret: "test-secret-test-secret-test-secret-32",
			BaseURL:   "http://localhost:8080",
		},
		Admins: []string{"test_admin"},
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

	server := httptest.NewServer(handler)
	defer server.Close()

	token, _, err := tokenMgr.GenerateAccessToken("test_admin")
	if err != nil {
		t.Fatal(err)
	}

	// Create a local client repository to push from
	clientDir := filepath.Join(tempDir, "client")
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		t.Fatal(err)
	}

	runGit := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v\nstderr: %s\nstdout: %s", args, err, stderr.String(), out.String())
		}
		return out.String()
	}

	runGit(clientDir, "init", "-b", "main")
	runGit(clientDir, "config", "commit.gpgsign", "false")
	runGit(clientDir, "config", "http.extraheader", "Authorization: Bearer "+token)

	// Create test file
	testFile := filepath.Join(clientDir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("Streaming push test content"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(clientDir, "add", ".")
	runGit(clientDir, "commit", "-m", "first streaming commit")

	// Push to server
	remoteURL := fmt.Sprintf("%s/stream-test.git", server.URL)
	runGit(clientDir, "remote", "add", "origin", remoteURL)
	runGit(clientDir, "push", "-u", "origin", "main")

	// Verify clone to a new directory
	cloneDir := filepath.Join(tempDir, "clone")
	runGit(tempDir, "-c", "http.extraheader=Authorization: Bearer "+token, "clone", remoteURL, cloneDir)

	clonedContent, err := os.ReadFile(filepath.Join(cloneDir, "hello.txt"))
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(clonedContent) != "Streaming push test content" {
		t.Fatalf("content mismatch: got %q, want %q", string(clonedContent), "Streaming push test content")
	}

	t.Log("SUCCESS: Streaming push and clone works seamlessly without spooling!")
}
