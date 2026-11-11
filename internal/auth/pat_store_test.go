package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if !strings.HasPrefix(pat1.HashedSecret, "$argon2id$") {
		t.Errorf("expected argon2id hash, got: %s", pat1.HashedSecret)
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
	if !strings.HasPrefix(pat2.HashedSecret, "$argon2id$") {
		t.Errorf("expected argon2id hash for pat2, got: %s", pat2.HashedSecret)
	}

	// Validate Write access -> should pass
	uname, ok = store.Validate(pat2.ID, rawSecret2, "org/repo.git", true)
	if !ok || uname != "alice" {
		t.Errorf("Validate write expected alice/true, got %s/%v", uname, ok)
	}

	// 7. List tokens (ensure hashed_secret is omitted in List result)
	list := store.List("alice", "org/repo.git", false)
	if len(list) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(list))
	}
	for _, tok := range list {
		if tok.HashedSecret != "" {
			t.Errorf("expected empty HashedSecret in List output, got: %s", tok.HashedSecret)
		}
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

func TestPATStore_LegacySHA256Migration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitina-pat-migration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "tokens.json")

	// Pre-populate tokens.json with a legacy SHA-256 hashed secret
	rawSecret := "pts_legacy_secret_value_12345"
	sum := sha256.Sum256([]byte(rawSecret))
	sha256Hex := hex.EncodeToString(sum[:])

	legacyToken := &PersonalAccessToken{
		ID:           "pat_legacy_01",
		Name:         "Legacy SHA256 Token",
		Username:     "bob",
		RepoPath:     "bob/project.git",
		Scope:        "rw",
		HashedSecret: sha256Hex,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
	}

	data, err := json.MarshalIndent([]*PersonalAccessToken{legacyToken}, "", "  ")
	if err != nil {
		t.Fatalf("Marshal legacy token failed: %v", err)
	}
	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Load store
	store, err := NewPATStore(tokenFile)
	if err != nil {
		t.Fatalf("NewPATStore failed: %v", err)
	}

	// 1. Validate with wrong secret -> fail
	uname, ok := store.Validate("pat_legacy_01", "wrong_secret", "bob/project.git", false)
	if ok {
		t.Fatalf("expected validation failure with wrong secret")
	}

	// 2. Validate with correct secret -> pass and upgrade to Argon2id
	uname, ok = store.Validate("pat_legacy_01", rawSecret, "bob/project.git", true)
	if !ok || uname != "bob" {
		t.Fatalf("Validate expected bob/true, got %s/%v", uname, ok)
	}

	// Verify internal token has been upgraded to argon2id
	store.mu.RLock()
	upgradedTok := store.tokens["pat_legacy_01"]
	store.mu.RUnlock()
	if !strings.HasPrefix(upgradedTok.HashedSecret, "$argon2id$") {
		t.Errorf("expected upgraded secret to start with $argon2id$, got: %s", upgradedTok.HashedSecret)
	}

	// 3. Reload store from disk and verify persistence of upgraded hash
	store2, err := NewPATStore(tokenFile)
	if err != nil {
		t.Fatalf("NewPATStore reload failed: %v", err)
	}
	store2.mu.RLock()
	persistedTok := store2.tokens["pat_legacy_01"]
	store2.mu.RUnlock()
	if !strings.HasPrefix(persistedTok.HashedSecret, "$argon2id$") {
		t.Errorf("expected persisted secret to start with $argon2id$, got: %s", persistedTok.HashedSecret)
	}

	// 4. Validate again using the upgraded hash
	uname, ok = store2.Validate("pat_legacy_01", rawSecret, "bob/project.git", true)
	if !ok || uname != "bob" {
		t.Fatalf("Validate on reloaded upgraded token expected bob/true, got %s/%v", uname, ok)
	}
}

func TestVerifyArgon2_InvalidFormat(t *testing.T) {
	// Various invalid argon2 format checks
	invalidHashes := []string{
		"",
		"$argon2i$v=19$m=65536,t=1,p=2$c2FsdA$aGFzaA", // wrong type (argon2i)
		"$argon2id$v=99$m=65536,t=1,p=2$c2FsdA$aGFzaA", // incompatible version
		"$argon2id$v=19$invalidparams$c2FsdA$aGFzaA",   // malformed params
		"$argon2id$v=19$m=65536,t=1,p=2$!invalid-b64!$aGFzaA",
		"$argon2id$v=19$m=65536,t=1,p=2$c2FsdA$!invalid-b64!",
	}

	for _, ih := range invalidHashes {
		match, err := verifyArgon2("secret", ih)
		if match || err == nil {
			t.Errorf("expected verifyArgon2 to fail for hash %q, got match=%v, err=%v", ih, match, err)
		}
	}
}
