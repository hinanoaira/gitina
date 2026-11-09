package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepoManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_repo_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create two bare repos
	repo1 := filepath.Join(tempDir, "path", "to", "first.git")
	repo2 := filepath.Join(tempDir, "flat.git")

	for _, r := range []string{repo1, repo2} {
		_ = os.MkdirAll(r, 0755)
		cmd := exec.Command("git", "init", "--bare")
		cmd.Dir = r
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to init bare repo: %v", err)
		}
	}

	mgr := NewManager(tempDir)

	// 1. List
	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(list))
	}

	// 2. Remove (soft delete to trash)
	msg, err := mgr.Remove("path/to/first.git", false)
	if err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	t.Logf("Remove result: %s", msg)

	// List again, should have 1
	list, err = mgr.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 repository after soft delete, got %d", len(list))
	}

	// Check that trash has the repo
	trashEntries, err := os.ReadDir(filepath.Join(tempDir, ".trash"))
	if err != nil || len(trashEntries) != 1 {
		t.Fatalf("expected 1 entry in trash, got %d", len(trashEntries))
	}

	// 3. Clean trash
	deleted, err := mgr.CleanTrash(0) // older than 0 days (immediate purge for test)
	if err != nil {
		t.Fatalf("CleanTrash() error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 cleaned entry, got %d", deleted)
	}

	// 4. Force remove
	_, err = mgr.Remove("flat.git", true)
	if err != nil {
		t.Fatalf("Remove(force=true) error: %v", err)
	}
	list, _ = mgr.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 repositories, got %d", len(list))
	}
}
