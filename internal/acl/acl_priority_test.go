package acl

import (
	"os"
	"path/filepath"
	"testing"

	"gitina/internal/config"
)

func TestACL_PriorityConfigVsParentDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina-acl-priority-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// config.yaml defines pattern "**" with public: false
	repoConfigs := []config.RepositoryConfig{
		{
			Path:   "**",
			Public: false,
		},
	}

	mgr := NewManager(nil, repoConfigs)
	mgr.SetRepoRoot(tempDir)

	// 1. Initial check: by default config.yaml matches and makes it private
	vis, isPub, _ := mgr.GetEffectiveVisibility("team/project.git")
	if vis != "private" || isPub {
		t.Fatalf("expected private from config.yaml, got %s (isPub=%v)", vis, isPub)
	}

	// 2. Set parent folder "team" to "public" via gitina-meta.json
	teamDir := filepath.Join(tempDir, "team")
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		t.Fatalf("failed to create team dir: %v", err)
	}
	err = SaveMetadata(teamDir, &RepoMetadata{
		Visibility: "public",
		Public:     true,
	})
	if err != nil {
		t.Fatalf("failed to save team metadata: %v", err)
	}

	// 3. Sub-repository "team/project.git" without its own meta should inherit "public" from parent "team",
	// overriding config.yaml default!
	vis, isPub, _ = mgr.GetEffectiveVisibility("team/project.git")
	if vis != "public" || !isPub {
		t.Fatalf("expected public inherited from parent folder, but got %s (isPub=%v)", vis, isPub)
	}

	// 4. If repository itself specifies "unlisted", it overrides parent folder
	projDir := filepath.Join(teamDir, "project.git")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("failed to create proj dir: %v", err)
	}
	err = SaveMetadata(projDir, &RepoMetadata{
		Visibility: "unlisted",
		Public:     false,
	})
	if err != nil {
		t.Fatalf("failed to save proj metadata: %v", err)
	}

	vis, isPub, _ = mgr.GetEffectiveVisibility("team/project.git")
	if vis != "unlisted" {
		t.Fatalf("expected unlisted overridden by repo itself, got %s", vis)
	}
}
