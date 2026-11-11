package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrPATNotFound = errors.New("personal access token not found")
	ErrPATExpired  = errors.New("personal access token has expired")
)

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultArgon2Params = &argon2Params{
	memory:      64 * 1024, // 64 MB
	iterations:  1,
	parallelism: 2,
	saltLength:  16,
	keyLength:   32,
}

type PersonalAccessToken struct {
	ID           string     `json:"id"`                    // e.g. "pat_a1b2c3d4e5f6..."
	Name         string     `json:"name"`                  // User-defined descriptive label
	Username     string     `json:"username"`              // Owner username
	RepoPath     string     `json:"repo_path"`             // Allowed repository
	Scope        string     `json:"scope"`                 // "ro" or "rw"
	HashedSecret string     `json:"hashed_secret"`         // Argon2id hash (or legacy SHA-256)
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`  // nil if no expiration
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

type PATStore struct {
	mu       sync.RWMutex
	filePath string
	tokens   map[string]*PersonalAccessToken // key: ID
}

func NewPATStore(filePath string) (*PATStore, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create PAT directory: %w", err)
	}

	store := &PATStore{
		filePath: filePath,
		tokens:   make(map[string]*PersonalAccessToken),
	}

	_ = store.load()
	return store, nil
}

func (s *PATStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []*PersonalAccessToken
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.tokens = make(map[string]*PersonalAccessToken, len(list))
	for _, t := range list {
		s.tokens[t.ID] = t
	}
	return nil
}

func (s *PATStore) saveLocked() error {
	list := make([]*PersonalAccessToken, 0, len(s.tokens))
	for _, t := range s.tokens {
		list = append(list, t)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", s.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.filePath)
}

func hashSecret(secret string) (string, error) {
	salt := make([]byte, defaultArgon2Params.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(secret),
		salt,
		defaultArgon2Params.iterations,
		defaultArgon2Params.memory,
		defaultArgon2Params.parallelism,
		defaultArgon2Params.keyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		defaultArgon2Params.memory,
		defaultArgon2Params.iterations,
		defaultArgon2Params.parallelism,
		b64Salt,
		b64Hash,
	), nil
}

func verifySecret(secret, encodedHash string) (match bool, needsUpgrade bool, err error) {
	if strings.HasPrefix(encodedHash, "$argon2id$") {
		match, err := verifyArgon2(secret, encodedHash)
		return match, false, err
	}

	// Legacy SHA-256 fallback (hex-encoded 64 chars)
	sum := sha256.Sum256([]byte(secret))
	expectedHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(encodedHash), []byte(expectedHash)) == 1 {
		return true, true, nil
	}

	return false, false, nil
}

func verifyArgon2(secret, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	// Format: "$argon2id$v=19$m=65536,t=1,p=2$<salt>$<hash>"
	// parts: ["", "argon2id", "v=19", "m=65536,t=1,p=2", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid argon2 hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("incompatible argon2 version")
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errors.New("invalid argon2 parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("invalid argon2 salt")
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errors.New("invalid argon2 hash")
	}

	keyLength := uint32(len(hash))
	comparisonHash := argon2.IDKey([]byte(secret), salt, iterations, memory, parallelism, keyLength)

	if subtle.ConstantTimeCompare(hash, comparisonHash) == 1 {
		return true, nil
	}
	return false, nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create generates a new Personal Access Token with a unique ID and raw secret.
func (s *PATStore) Create(username, repoPath, name, scope string, duration time.Duration) (*PersonalAccessToken, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		name = "Token " + time.Now().Format("2006-01-02 15:04")
	}

	id := "pat_" + generateRandomHex(8)         // 16-hex characters
	rawSecret := "pts_" + generateRandomHex(20) // 40-hex characters

	hashed, err := hashSecret(rawSecret)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash token secret: %w", err)
	}

	now := time.Now()
	var exp *time.Time
	if duration > 0 {
		e := now.Add(duration)
		exp = &e
	}

	pat := &PersonalAccessToken{
		ID:           id,
		Name:         name,
		Username:     username,
		RepoPath:     repoPath,
		Scope:        scope,
		HashedSecret: hashed,
		CreatedAt:    now,
		ExpiresAt:    exp,
	}

	s.tokens[id] = pat
	if err := s.saveLocked(); err != nil {
		delete(s.tokens, id)
		return nil, "", err
	}

	return pat, rawSecret, nil
}

// Validate verifies a Basic Auth ID and Secret.
func (s *PATStore) Validate(id, secret, targetRepo string, isWrite bool) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pat, exists := s.tokens[id]
	if !exists {
		return "", false
	}

	// Verify expiration
	if pat.ExpiresAt != nil && time.Now().After(*pat.ExpiresAt) {
		return "", false
	}

	// Verify secret using constant-time comparison
	matched, needsUpgrade, _ := verifySecret(secret, pat.HashedSecret)
	if !matched {
		return "", false
	}

	// Scope restriction
	if isWrite && pat.Scope == "ro" {
		return pat.Username, false // Read-only token cannot write
	}

	// Repository restriction
	if pat.RepoPath != "" && pat.RepoPath != "*" {
		cleanPatRepo := pat.RepoPath
		cleanTarget := targetRepo
		if cleanPatRepo != cleanTarget && cleanPatRepo+".git" != cleanTarget && cleanPatRepo != cleanTarget+".git" {
			return pat.Username, false
		}
	}

	now := time.Now()
	pat.LastUsedAt = &now

	// Seamlessly upgrade legacy hash (e.g. SHA-256) to Argon2id
	if needsUpgrade {
		if newHash, err := hashSecret(secret); err == nil {
			pat.HashedSecret = newHash
		}
	}

	_ = s.saveLocked()

	return pat.Username, true
}

// List returns PATs belonging to username and matching repoPath (or all if repoPath is "").
// If username is empty or user is admin, all can be viewed.
func (s *PATStore) List(username, repoPath string, isAdmin bool) []*PersonalAccessToken {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*PersonalAccessToken
	for _, t := range s.tokens {
		if !isAdmin && t.Username != username {
			continue
		}
		if repoPath != "" {
			cleanT := t.RepoPath
			cleanR := repoPath
			if cleanT != cleanR && cleanT+".git" != cleanR && cleanT != cleanR+".git" {
				continue
			}
		}
		// Return a copy without revealing hashed secret directly
		copyToken := *t
		copyToken.HashedSecret = ""
		result = append(result, &copyToken)
	}
	return result
}

// Revoke deletes a PAT by ID.
func (s *PATStore) Revoke(id, username string, isAdmin bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pat, exists := s.tokens[id]
	if !exists {
		return ErrPATNotFound
	}

	if !isAdmin && pat.Username != username {
		return errors.New("forbidden: cannot revoke another user's token")
	}

	delete(s.tokens, id)
	return s.saveLocked()
}
