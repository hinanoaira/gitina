package tiered

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBundleCreateAndRestore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_bundle_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	srcRepo := filepath.Join(tempDir, "src.git")
	targetRepo := filepath.Join(tempDir, "target.git")
	bundleFile := filepath.Join(tempDir, "test.bundle")

	// 1. Initialize src bare repo and create a commit
	_ = os.MkdirAll(srcRepo, 0755)
	if err := exec.Command("git", "init", "--bare", srcRepo).Run(); err != nil {
		t.Fatalf("git init --bare failed: %v", err)
	}

	// Create a commit in work tree and push to src
	workDir := filepath.Join(tempDir, "work")
	_ = os.MkdirAll(workDir, 0755)
	_ = exec.Command("git", "init", workDir).Run()
	_ = exec.Command("git", "-C", workDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", workDir, "config", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "-C", workDir, "config", "commit.gpgsign", "false").Run()
	_ = os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("hello world"), 0644)
	if out, err := exec.Command("git", "-C", workDir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s: %v", string(out), err)
	}
	if out, err := exec.Command("git", "-C", workDir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s: %v", string(out), err)
	}
	if out, err := exec.Command("git", "-C", workDir, "push", srcRepo, "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("git push failed: %s: %v", string(out), err)
	}

	// 2. Create bundle
	if err := CreateBundle(srcRepo, bundleFile); err != nil {
		t.Fatalf("CreateBundle failed: %v", err)
	}

	// Check bundle exists
	if _, err := os.Stat(bundleFile); err != nil {
		t.Fatalf("bundle file not found: %v", err)
	}

	// 3. Restore bundle
	if err := RestoreBundle(bundleFile, targetRepo); err != nil {
		t.Fatalf("RestoreBundle failed: %v", err)
	}

	// Check restored repo has HEAD
	if _, err := os.Stat(filepath.Join(targetRepo, "HEAD")); err != nil {
		t.Fatalf("restored repo HEAD not found: %v", err)
	}

	// Verify log in restored repo
	out, err := exec.Command("git", "-C", targetRepo, "log", "-n", "1", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log on restored repo failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected commit log in restored repo")
	}
}

func TestLRUInspectAndPurge(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_lru_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repo1 := filepath.Join(tempDir, "repo1.git")
	repo2 := filepath.Join(tempDir, "repo2.git")

	for _, r := range []string{repo1, repo2} {
		_ = os.MkdirAll(r, 0755)
		_ = exec.Command("git", "init", "--bare", r).Run()
	}

	// Set repo1 to be older
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(filepath.Join(repo1, "HEAD"), oldTime, oldTime)

	// Inspect
	items, total, err := InspectCache(tempDir)
	if err != nil {
		t.Fatalf("InspectCache failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if total <= 0 {
		t.Fatalf("expected total > 0, got %d", total)
	}

	// Purge with targetBytes = 0 (purge everything not locked)
	locked := map[string]bool{"repo2.git": true}
	purged, err := PurgeOldestCache(tempDir, 1, 0, locked)
	if err != nil {
		t.Fatalf("PurgeOldestCache failed: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected 1 purged repo (repo1), got %d", purged)
	}

	// Verify repo1 is gone, repo2 remains
	if _, err := os.Stat(repo1); !os.IsNotExist(err) {
		t.Fatalf("expected repo1 to be purged")
	}
	if _, err := os.Stat(repo2); os.IsNotExist(err) {
		t.Fatalf("expected repo2 to still exist")
	}
}
