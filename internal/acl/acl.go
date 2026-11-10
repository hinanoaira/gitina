package acl

import (
	"errors"
	"path"
	"path/filepath"
	"strings"

	"gitina/internal/config"
)

type AccessLevel string

const (
	AccessNone      AccessLevel = "none"
	AccessReadOnly  AccessLevel = "ro"
	AccessReadWrite AccessLevel = "rw"
)

type Manager struct {
	admins       map[string]bool
	repositories []config.RepositoryConfig
	repoRoot     string
}

func NewManager(admins []string, repositories []config.RepositoryConfig) *Manager {
	adminMap := make(map[string]bool, len(admins))
	for _, a := range admins {
		adminMap[strings.ToLower(strings.TrimSpace(a))] = true
	}

	return &Manager{
		admins:       adminMap,
		repositories: repositories,
	}
}

// SetRepoRoot sets the filesystem root path for repositories to load dynamic metadata.
func (m *Manager) SetRepoRoot(root string) {
	m.repoRoot = root
}

// IsAdmin checks if the username is configured as an admin.
func (m *Manager) IsAdmin(username string) bool {
	if username == "" {
		return false
	}
	return m.admins[strings.ToLower(strings.TrimSpace(username))]
}

// GetEffectiveConfig returns the resolved public status and collaborators for a repository,
// taking dynamic gitina-meta.json into account over config.yaml defaults.
func (m *Manager) GetEffectiveConfig(repoPath string) (isPublic bool, collaborators map[string]string) {
	repoPath = normalizeRepoPath(repoPath)
	collaborators = make(map[string]string)

	// 1. Defaults from config.yaml
	var matchedRule *config.RepositoryConfig
	for i := range m.repositories {
		rule := &m.repositories[i]
		if matchPattern(rule.Path, repoPath) {
			matchedRule = rule
			break
		}
	}
	if matchedRule != nil {
		isPublic = matchedRule.Public
		for u, lvl := range matchedRule.Collaborators {
			collaborators[strings.ToLower(strings.TrimSpace(u))] = lvl
		}
	}

	// 2. Override from dynamic gitina-meta.json if present
	if m.repoRoot != "" {
		dir := filepath.Join(m.repoRoot, repoPath)
		if meta, err := LoadMetadata(dir); err == nil && meta != nil {
			isPublic = meta.Public
			if meta.Collaborators != nil {
				// Metadata replaces or specifies the full collaborator list
				collaborators = make(map[string]string)
				for u, lvl := range meta.Collaborators {
					collaborators[strings.ToLower(strings.TrimSpace(u))] = lvl
				}
			}
		}
	}

	return isPublic, collaborators
}

// UpdateMetadata saves dynamic metadata to gitina-meta.json for a repository.
func (m *Manager) UpdateMetadata(repoPath string, isPublic bool, collaborators map[string]string, updatedBy string) error {
	if m.repoRoot == "" {
		return errors.New("repoRoot not configured in ACL manager")
	}
	repoPath = normalizeRepoPath(repoPath)
	dir := filepath.Join(m.repoRoot, repoPath)

	meta := &RepoMetadata{
		Public:        isPublic,
		Collaborators: collaborators,
		UpdatedBy:     updatedBy,
	}
	return SaveMetadata(dir, meta)
}

// CheckAccess determines the access level of a user for a given repository.
// username can be empty for unauthenticated / anonymous requests.
func (m *Manager) CheckAccess(username, repoPath string) AccessLevel {
	username = strings.ToLower(strings.TrimSpace(username))
	repoPath = normalizeRepoPath(repoPath)

	// Admin has full read-write access to all repositories.
	if username != "" && m.admins[username] {
		return AccessReadWrite
	}

	isPublic, collaborators := m.GetEffectiveConfig(repoPath)

	// Check user in collaborators
	var userCollab AccessLevel = AccessNone
	if username != "" {
		if lvl, ok := collaborators[username]; ok {
			lvl = strings.ToLower(strings.TrimSpace(lvl))
			if lvl == "rw" {
				userCollab = AccessReadWrite
			} else if lvl == "ro" || lvl == "r" {
				userCollab = AccessReadOnly
			}
		}
	}

	if userCollab == AccessReadWrite {
		return AccessReadWrite
	}

	if isPublic {
		return AccessReadOnly
	}

	return userCollab
}

// CanRead checks if the user has at least read access.
func (m *Manager) CanRead(username, repoPath string) bool {
	level := m.CheckAccess(username, repoPath)
	return level == AccessReadOnly || level == AccessReadWrite
}

// CanWrite checks if the user has write access.
func (m *Manager) CanWrite(username, repoPath string) bool {
	level := m.CheckAccess(username, repoPath)
	return level == AccessReadWrite
}

// normalizeRepoPath ensures repoPath is clean, trimmed, and ends with .git for uniform matching.
func normalizeRepoPath(p string) string {
	p = strings.Trim(filepath.ToSlash(p), "/")
	if !strings.HasSuffix(p, ".git") {
		p = p + ".git"
	}
	return p
}

// matchPattern matches a pattern (which may have glob or wildcards) against repoPath.
func matchPattern(pattern, repoPath string) bool {
	pattern = strings.Trim(filepath.ToSlash(pattern), "/")
	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")

	// If pattern ends with /** or /*
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(repoPath, prefix+"/") || repoPath == prefix || repoPath == prefix+".git"
	}

	// Normalize pattern to .git if it does not contain wildcards
	if !strings.Contains(pattern, "*") && !strings.HasSuffix(pattern, ".git") {
		pattern = pattern + ".git"
	}

	// Standard glob match
	matched, err := path.Match(pattern, repoPath)
	if err == nil && matched {
		return true
	}

	// Also test without .git on pattern if repoPath has .git
	if strings.HasSuffix(repoPath, ".git") {
		repoWithoutGit := strings.TrimSuffix(repoPath, ".git")
		patternWithoutGit := strings.TrimSuffix(pattern, ".git")
		matched, err = path.Match(patternWithoutGit, repoWithoutGit)
		if err == nil && matched {
			return true
		}
	}

	return false
}
