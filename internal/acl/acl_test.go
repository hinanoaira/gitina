package acl

import (
	"testing"

	"gitina/internal/config"
)

func TestACL(t *testing.T) {
	admins := []string{"hinanoaira", "superadmin"}
	repos := []config.RepositoryConfig{
		{
			Path:   "public-repos/**",
			Public: true,
			Collaborators: map[string]string{
				"contributor": "rw",
			},
		},
		{
			Path:   "projects/team-a/*",
			Public: false,
			Collaborators: map[string]string{
				"alice": "rw",
				"bob":   "ro",
			},
		},
		{
			Path:   "fuga.git",
			Public: false,
			Collaborators: map[string]string{
				"charlie": "rw",
			},
		},
	}

	m := NewManager(admins, repos)

	// Admin tests
	if !m.CanWrite("hinanoaira", "any/random/path.git") {
		t.Errorf("admin should have write access to any repo")
	}
	if !m.CanRead("superadmin", "fuga.git") {
		t.Errorf("admin should have read access to fuga.git")
	}

	// Public repo tests
	if !m.CanRead("", "public-repos/nested/deep/repo.git") {
		t.Errorf("anonymous user should be able to read public repo")
	}
	if m.CanWrite("", "public-repos/nested/deep/repo.git") {
		t.Errorf("anonymous user should NOT be able to write to public repo")
	}
	if !m.CanWrite("contributor", "public-repos/nested/deep/repo.git") {
		t.Errorf("contributor should have write access to public repo")
	}
	if m.CanWrite("random_user", "public-repos/nested/deep/repo.git") {
		t.Errorf("random user should NOT have write access to public repo")
	}
	if !m.CanRead("random_user", "public-repos/nested/deep/repo.git") {
		t.Errorf("random user should be able to read public repo")
	}

	// Private repo with wildcard: projects/team-a/*
	if !m.CanWrite("alice", "projects/team-a/backend.git") {
		t.Errorf("alice should have write access to projects/team-a/backend.git")
	}
	if !m.CanRead("bob", "projects/team-a/backend.git") {
		t.Errorf("bob should have read access to projects/team-a/backend.git")
	}
	if m.CanWrite("bob", "projects/team-a/backend.git") {
		t.Errorf("bob should NOT have write access to projects/team-a/backend.git")
	}
	if m.CanRead("charlie", "projects/team-a/backend.git") {
		t.Errorf("charlie should NOT have access to projects/team-a/backend.git")
	}

	// Single repo: fuga.git
	if !m.CanWrite("charlie", "fuga.git") {
		t.Errorf("charlie should have write access to fuga.git")
	}
	if !m.CanWrite("charlie", "fuga") { // without .git suffix test
		t.Errorf("charlie should have write access to fuga (normalized)")
	}
	if m.CanRead("alice", "fuga.git") {
		t.Errorf("alice should NOT have access to fuga.git")
	}

	// Unlisted repo
	if m.CanRead("alice", "unlisted.git") {
		t.Errorf("alice should NOT have access to unlisted.git")
	}
	if !m.CanWrite("hinanoaira", "unlisted.git") {
		t.Errorf("admin should have access to unlisted.git")
	}
}
