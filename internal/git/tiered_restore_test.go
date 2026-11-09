package git

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git/tiered"
)

// MockS3Server simulates S3 HeadObject, GetObject, and PutObject.
type MockS3Server struct {
	mu      sync.Mutex
	objects map[string][]byte
	server  *httptest.Server
}

func newMockS3Server() *MockS3Server {
	m := &MockS3Server{
		objects: make(map[string][]byte),
	}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		key := strings.TrimPrefix(r.URL.Path, "/")
		// S3 path-style: /bucket/key
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 {
			key = parts[1]
		}

		switch r.Method {
		case http.MethodHead:
			data, exists := m.objects[key]
			if !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			data, exists := m.objects[key]
			if !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			m.objects[key] = body
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	return m
}

func TestS3TieredRestoreOnClone(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_tiered_restore_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Create a sample Git repository and pack it into a bundle
	srcDir := filepath.Join(tempDir, "src")
	_ = os.MkdirAll(srcDir, 0755)
	_ = exec.Command("git", "init", srcDir).Run()
	_ = exec.Command("git", "-C", srcDir, "config", "user.name", "TieredTest").Run()
	_ = exec.Command("git", "-C", srcDir, "config", "user.email", "tiered@example.com").Run()
	_ = exec.Command("git", "-C", srcDir, "config", "commit.gpgsign", "false").Run()

	secretFile := filepath.Join(srcDir, "important_data.txt")
	_ = os.WriteFile(secretFile, []byte("THIS_IS_PERSISTED_IN_S3"), 0644)
	_ = exec.Command("git", "-C", srcDir, "add", ".").Run()
	_ = exec.Command("git", "-C", srcDir, "commit", "-m", "commit for s3 test").Run()
	_ = exec.Command("git", "-C", srcDir, "branch", "-M", "main").Run()

	bundleFile := filepath.Join(tempDir, "sample.bundle")
	if err := exec.Command("git", "-C", srcDir, "bundle", "create", bundleFile, "--all").Run(); err != nil {
		t.Fatalf("failed to create test bundle: %v", err)
	}

	bundleBytes, err := os.ReadFile(bundleFile)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Put the bundle into Mock S3
	mockS3 := newMockS3Server()
	defer mockS3.server.Close()

	// S3 key: repos/s3-test-project.git.bundle
	s3Key := "repos/s3-test-project.git.bundle"
	mockS3.objects[s3Key] = bundleBytes

	// 3. Configure gitina with S3 tiered storage enabled, but EMPTY local repo_root
	emptyRepoRoot := filepath.Join(tempDir, "empty_repo_root")
	_ = os.MkdirAll(emptyRepoRoot, 0755)

	cfg := &config.Config{
		Server: config.ServerConfig{
			RepoRoot:  emptyRepoRoot,
			JWTSecret: "test-secret",
			BaseURL:   "http://localhost:8080",
		},
		Admins: []string{"admin_user"},
		RepositoryStorage: config.RepositoryStorageConfig{
			Type: "s3",
			S3: config.RepositoryStorageS3{
				Endpoint:        mockS3.server.URL,
				Region:          "us-east-1",
				Bucket:          "my-bucket",
				Prefix:          "repos/",
				AccessKeyID:     "dummy-key",
				SecretAccessKey: "dummy-secret",
				UsePathStyle:    true,
			},
		},
	}

	aclMgr := acl.NewManager(cfg.Admins, nil)
	tokenMgr := auth.NewTokenManager(cfg.Server.JWTSecret)
	gcmHandler := auth.NewGCMHandler(cfg, tokenMgr, aclMgr)

	tieredMgr, err := tiered.NewManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create tiered manager: %v", err)
	}

	handler, err := NewHandler(cfg, aclMgr, gcmHandler, nil, tieredMgr)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// 4. Verify local repo does NOT exist before request
	localRepoPath := filepath.Join(emptyRepoRoot, "s3-test-project.git")
	if _, err := os.Stat(localRepoPath); !os.IsNotExist(err) {
		t.Fatalf("local repo should not exist yet")
	}

	// 5. Simulate client sending `git clone http://localhost:8080/s3-test-project.git`
	token, _, err := tokenMgr.GenerateAccessToken("admin_user")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/s3-test-project.git/info/refs?service=git-upload-pack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// 6. Verify local bare repository was downloaded and unpacked from S3!
	if _, err := os.Stat(filepath.Join(localRepoPath, "HEAD")); err != nil {
		t.Fatalf("HEAD file not found in restored local repo: %v", err)
	}

	// Verify the commit log in restored repo matches the one uploaded to S3
	logOut, err := exec.Command("git", "-C", localRepoPath, "log", "-n", "1", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log failed on restored repo: %v", err)
	}
	if !strings.Contains(string(logOut), "commit for s3 test") {
		t.Fatalf("expected commit log to contain 'commit for s3 test', got: %s", string(logOut))
	}

	t.Logf("SUCCESS: S3 bundle was automatically restored and served on clone! Log: %s", strings.TrimSpace(string(logOut)))
}
