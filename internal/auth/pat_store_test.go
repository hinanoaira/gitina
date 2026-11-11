package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPATStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitina-pat-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "tokens.json")
	store, err := NewPATStore(tokenFile)
	if err != nil {
		t.Fatalf("NewPATStore failed: %v", err)
	}

	// 1. Create a token (RO)
	pat1, rawSecret1, err := store.Create("alice", "org/repo.git", "MacBook Readonly", "ro", 24*time.Hour)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if pat1.ID == "" || rawSecret1 == "" {
		t.Fatalf("expected non-empty ID and secret")
	}

	// 2. Validate Read access -> should pass
	uname, ok := store.Validate(pat1.ID, rawSecret1, "org/repo.git", false)
	if !ok || uname != "alice" {
		t.Errorf("Validate read expected alice/true, got %s/%v", uname, ok)
	}

	// 3. Validate Write access -> should fail (it's RO)
	uname, ok = store.Validate(pat1.ID, rawSecret1, "org/repo.git", true)
	if ok || uname != "alice" {
		t.Errorf("Validate write on RO expected alice/false, got %s/%v", uname, ok)
	}

	// 4. Validate wrong repository -> should fail
	uname, ok = store.Validate(pat1.ID, rawSecret1, "other/repo.git", false)
	if ok {
		t.Errorf("Validate on wrong repo expected false, got true")
	}

	// 5. Validate wrong secret -> should fail
	uname, ok = store.Validate(pat1.ID, "wrong_secret", "org/repo.git", false)
	if ok {
		t.Errorf("Validate with wrong secret expected false, got true")
	}

	// 6. Create another token (RW, unlimited)
	pat2, rawSecret2, err := store.Create("alice", "org/repo.git", "CI Deploy", "rw", 0)
	if err != nil {
		t.Fatalf("Create RW failed: %v", err)
	}

	// Validate Write access -> should pass
	uname, ok = store.Validate(pat2.ID, rawSecret2, "org/repo.git", true)
	if !ok || uname != "alice" {
		t.Errorf("Validate write expected alice/true, got %s/%v", uname, ok)
	}

	// 7. List tokens
	list := store.List("alice", "org/repo.git", false)
	if len(list) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(list))
	}

	// 8. Revoke token
	if err := store.Revoke(pat1.ID, "alice", false); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// After revoke, validation must fail
	_, ok = store.Validate(pat1.ID, rawSecret1, "org/repo.git", false)
	if ok {
		t.Errorf("expected revoked token to fail validation")
	}

	// 9. Persistence test: load new store from same file
	store2, err := NewPATStore(tokenFile)
	if err != nil {
		t.Fatalf("NewPATStore reload failed: %v", err)
	}
	list2 := store2.List("alice", "org/repo.git", false)
	if len(list2) != 1 || list2[0].ID != pat2.ID {
		t.Errorf("expected 1 token after reload, got %d", len(list2))
	}
}
