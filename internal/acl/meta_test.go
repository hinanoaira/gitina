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

	// 3. Test "unlisted" visibility:
	// URL direct access (CanRead) is allowed for everyone (including anonymous),
	// but CanList is only allowed for users with explicit access (bob) or admin.
	err = mgr.UpdateVisibilityMetadata("owner/project.git", "unlisted", map[string]string{
		"bob": "ro",
	}, "admin_user")
	if err != nil {
		t.Fatalf("UpdateVisibilityMetadata failed: %v", err)
	}

	// Direct access test:
	if !mgr.CanRead("", "owner/project.git") {
		t.Fatal("unlisted should be readable by anonymous via direct URL")
	}
	if !mgr.CanRead("random_user", "owner/project.git") {
		t.Fatal("unlisted should be readable by any user via direct URL")
	}
	if mgr.CanWrite("", "owner/project.git") || mgr.CanWrite("random_user", "owner/project.git") {
		t.Fatal("unlisted should NOT be writable without explicit permission")
	}

	// List test:
	if mgr.CanList("", "owner/project.git") {
		t.Fatal("unlisted should NOT appear in listing for anonymous")
	}
	if mgr.CanList("random_user", "owner/project.git") {
		t.Fatal("unlisted should NOT appear in listing for users without permission")
	}
	if !mgr.CanList("bob", "owner/project.git") {
		t.Fatal("unlisted should appear in listing for users with RO access")
	}
	if !mgr.CanList("admin_user", "owner/project.git") {
		t.Fatal("unlisted should appear in listing for admin")
	}

	// 4. Test Parent Folder Permission Inheritance:
	// Set permissions on "owner" folder: charlie -> rw
	err = mgr.UpdateVisibilityMetadata("owner", "private", map[string]string{
		"charlie": "rw",
	}, "admin_user")
	if err != nil {
		t.Fatalf("UpdateVisibilityMetadata on folder failed: %v", err)
	}

	// charlie should inherit write access to owner/project.git
	if !mgr.CanWrite("charlie", "owner/project.git") {
		t.Fatal("charlie should inherit write access from parent folder 'owner'")
	}
}
