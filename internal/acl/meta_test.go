package acl

import (
	"os"
	"path/filepath"
	"testing"

	"gitina/internal/config"
)

func TestDynamicMetadataOverride(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_meta_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoDir := filepath.Join(tempDir, "owner", "project.git")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	repos := []config.RepositoryConfig{
		{
			Path:   "owner/project.git",
			Public: false,
			Collaborators: map[string]string{
				"alice": "ro",
			},
		},
	}

	mgr := NewManager([]string{"admin_user"}, repos)
	mgr.SetRepoRoot(tempDir)

	// Initially: private, alice is ro, bob has no access
	if mgr.CanRead("", "owner/project.git") {
		t.Fatal("expected anonymous read to be forbidden for private repo")
	}
	if !mgr.CanRead("alice", "owner/project.git") || mgr.CanWrite("alice", "owner/project.git") {
		t.Fatal("expected alice to have read-only access")
	}
	if mgr.CanRead("bob", "owner/project.git") {
		t.Fatal("expected bob to have no access")
	}

	// 1. Update metadata dynamically: make public, add bob as rw, promote alice to rw
	err = mgr.UpdateMetadata("owner/project.git", true, map[string]string{
		"alice": "rw",
		"bob":   "rw",
	}, "admin_user")
	if err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	// Verify updated behavior
	if !mgr.CanRead("", "owner/project.git") {
		t.Fatal("expected anonymous read to be allowed after making public")
	}
	if !mgr.CanWrite("bob", "owner/project.git") {
		t.Fatal("expected bob to have write access after dynamic update")
	}
	if !mgr.CanWrite("alice", "owner/project.git") {
		t.Fatal("expected alice to have write access after dynamic update")
	}

	// 2. Update metadata dynamically: make private again
	err = mgr.UpdateMetadata("owner/project.git", false, map[string]string{
		"bob": "ro",
	}, "admin_user")
	if err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	if mgr.CanRead("", "owner/project.git") {
		t.Fatal("expected anonymous read to be forbidden after making private")
	}
	if !mgr.CanRead("bob", "owner/project.git") || mgr.CanWrite("bob", "owner/project.git") {
		t.Fatal("expected bob to have read-only access now")
	}
}
