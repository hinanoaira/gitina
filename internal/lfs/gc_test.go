package lfs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gitina/internal/config"
)

func createTestRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// git init --bare
	cmd := exec.Command("git", "init", "--bare", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v: %s", err, string(out))
	}
	return dir
}

func commitLFSPointer(t *testing.T, bareDir, branch, oid string, size int64) {
	t.Helper()
	// Create a temp work tree to commit
	workDir, err := os.MkdirTemp("", "gitina_work_*")
	if err != nil {
		t.Fatalf("failed to create temp workdir: %v", err)
	}
	defer os.RemoveAll(workDir)

	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command failed %v: %v\nOutput: %s", args, err, string(out))
		}
	}

	runCmd("clone", bareDir, ".")
	runCmd("checkout", "-B", branch)

	// Write LFS pointer content
	pointerContent := fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n", oid, size)
	if err := os.WriteFile(filepath.Join(workDir, "large_file.bin"), []byte(pointerContent), 0644); err != nil {
		t.Fatalf("failed to write pointer: %v", err)
	}

	runCmd("add", "large_file.bin")
	runCmd("commit", "-m", "add lfs pointer")
	runCmd("push", "origin", branch)
}

func TestLFSGarbageCollector(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_lfs_gc_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoRoot := filepath.Join(tempDir, "repos")
	lfsDir := filepath.Join(tempDir, "lfs")

	repoPath := filepath.Join(repoRoot, "my-repo.git")
	createTestRepo(t, repoPath)

	localStorage, err := NewLocalStorage(lfsDir)
	if err != nil {
		t.Fatalf("failed to create local storage: %v", err)
	}

	// 1. Setup 3 LFS Objects in storage:
	// OID 1: Referenced in Git commit
	oidActive := "1111111111111111111111111111111111111111111111111111111111111111"
	// OID 2: Orphaned, but modified recently (e.g. 1 hour ago) -> within 7-day Grace Period
	oidRecentOrphan := "2222222222222222222222222222222222222222222222222222222222222222"
	// OID 3: Orphaned, and modified 10 days ago -> EXCEEDS 7-day Grace Period -> SHOULD BE DELETED
	oidOldOrphan := "3333333333333333333333333333333333333333333333333333333333333333"

	// Create objects directly in storage
	for _, oid := range []string{oidActive, oidRecentOrphan, oidOldOrphan} {
		p := localStorage.objectPath(oid)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte("dummy content for "+oid), 0644); err != nil {
			t.Fatalf("failed to write object file: %v", err)
		}
	}

	// Set ModTime for oidOldOrphan to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	_ = os.Chtimes(localStorage.objectPath(oidOldOrphan), oldTime, oldTime)

	// Set ModTime for oidRecentOrphan to 1 hour ago
	recentTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(localStorage.objectPath(oidRecentOrphan), recentTime, recentTime)

	// Commit pointer for oidActive to repo
	commitLFSPointer(t, repoPath, "main", oidActive, 100)

	cfg := config.LFSGCConfig{
		Enabled:         true,
		IntervalHours:   24,
		GracePeriodDays: 7,
		DryRun:          false,
	}

	gc := NewGarbageCollector(repoRoot, localStorage, cfg)

	ctx := context.Background()

	// 1st Test: Run with DryRun = true
	gc.cfg.DryRun = true
	stats, err := gc.Run(ctx)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if stats.ReachableOIDs != 1 {
		t.Errorf("expected 1 reachable OID, got %d", stats.ReachableOIDs)
	}
	if stats.OrphanObjects != 1 {
		t.Errorf("expected 1 orphan object exceeding grace period, got %d", stats.OrphanObjects)
	}
	if stats.DeletedObjects != 0 {
		t.Errorf("expected 0 deleted objects in dry-run, got %d", stats.DeletedObjects)
	}

	// Verify all 3 still exist
	for _, oid := range []string{oidActive, oidRecentOrphan, oidOldOrphan} {
		exists, _ := localStorage.Exists(ctx, "", oid, 0)
		if !exists {
			t.Errorf("object %s should still exist after dry-run", oid)
		}
	}

	// 2nd Test: Run with DryRun = false
	gc.cfg.DryRun = false
	stats, err = gc.Run(ctx)
	if err != nil {
		t.Fatalf("actual gc run failed: %v", err)
	}

	if stats.DeletedObjects != 1 {
		t.Errorf("expected 1 deleted object, got %d", stats.DeletedObjects)
	}

	// Verify oidActive still exists
	exists, _ := localStorage.Exists(ctx, "", oidActive, 0)
	if !exists {
		t.Errorf("active object %s should NOT have been deleted", oidActive)
	}

	// Verify oidRecentOrphan still exists (protected by grace period)
	exists, _ = localStorage.Exists(ctx, "", oidRecentOrphan, 0)
	if !exists {
		t.Errorf("recent orphan object %s should NOT have been deleted (within grace period)", oidRecentOrphan)
	}

	// Verify oidOldOrphan was deleted!
	exists, _ = localStorage.Exists(ctx, "", oidOldOrphan, 0)
	if exists {
		t.Errorf("old orphan object %s SHOULD have been deleted", oidOldOrphan)
	}
}
