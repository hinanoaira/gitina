package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrPATNotFound = errors.New("personal access token not found")
	ErrPATExpired  = errors.New("personal access token has expired")
)

type PersonalAccessToken struct {
	ID         string     `json:"id"`                    // e.g. "pat_a1b2c3d4e5f6..."
	Name       string     `json:"name"`                  // User-defined descriptive label
	Username   string     `json:"username"`              // Owner username
	RepoPath   string     `json:"repo_path"`             // Allowed repository
	Scope      string     `json:"scope"`                 // "ro" or "rw"
	HashedSecret string   `json:"hashed_secret"`         // SHA-256 of raw secret
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`  // nil if no expiration
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
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

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
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

	id := "pat_" + generateRandomHex(8) // 16-hex characters
	rawSecret := "pts_" + generateRandomHex(20) // 40-hex characters

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
		HashedSecret: hashSecret(rawSecret),
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

	// Verify secret using constant time comparison
	expectedHash := hashSecret(secret)
	if subtle.ConstantTimeCompare([]byte(pat.HashedSecret), []byte(expectedHash)) != 1 {
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
