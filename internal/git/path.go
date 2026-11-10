package git

import (
	"errors"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid or malicious repository path")

	suspiciousExtensions = []string{
		".php", ".asp", ".aspx", ".jsp", ".cgi", ".env", ".ini", ".conf", ".sql", ".bak",
	}
)

// Known Git and LFS endpoints
var gitEndpoints = []string{
	"/info/refs",
	"/git-upload-pack",
	"/git-receive-pack",
	"/HEAD",
	"/info/lfs/objects/batch",
	"/info/lfs/objects/verify",
	"/info/lfs/objects/", // for /<oid>
}

// ParsedRequest holds the decomposed repository path and subpath.
type ParsedRequest struct {
	RepoPath string // e.g. "path/to/repo.git"
	SubPath  string // e.g. "info/refs" or "info/lfs/objects/batch"
	IsLFS    bool
}

// ParseURLPath extracts the repo path and Git/LFS subpath from an incoming HTTP request URL path.
func ParseURLPath(urlPath string) (*ParsedRequest, error) {
	urlPath = strings.Trim(filepath.ToSlash(urlPath), "/")
	if urlPath == "" {
		return nil, ErrInvalidPath
	}

	// Security: check for path traversal
	if strings.Contains(urlPath, "..") {
		return nil, ErrInvalidPath
	}
	cleaned := filepath.Clean("/" + urlPath)
	if strings.Contains(cleaned, "..") || cleaned == "/" {
		return nil, ErrInvalidPath
	}
	urlPath = strings.TrimPrefix(cleaned, "/")

	// Try matching against known Git and LFS endpoints
	for _, ep := range gitEndpoints {
		idx := strings.Index("/"+urlPath, ep)
		if idx != -1 {
			// Extract repo part before ep
			repoPart := ("/" + urlPath)[:idx]
			subPart := ("/" + urlPath)[idx:]

			repoPart = strings.Trim(repoPart, "/")
			subPart = strings.Trim(subPart, "/")

			if repoPart == "" {
				return nil, ErrInvalidPath
			}

			// Ensure repoPart ends with .git
			if !strings.HasSuffix(repoPart, ".git") {
				repoPart = repoPart + ".git"
			}

			if !validateRepoPath(repoPart) {
				return nil, ErrInvalidPath
			}

			isLFS := strings.HasPrefix(subPart, "info/lfs")
			return &ParsedRequest{
				RepoPath: repoPart,
				SubPath:  subPart,
				IsLFS:    isLFS,
			}, nil
		}
	}

	// Check if path contains .git/ or ends with .git
	if idx := strings.Index(urlPath, ".git/"); idx != -1 {
		repoPart := urlPath[:idx+4]
		subPart := strings.Trim(urlPath[idx+5:], "/")
		if !validateRepoPath(repoPart) {
			return nil, ErrInvalidPath
		}
		return &ParsedRequest{
			RepoPath: repoPart,
			SubPath:  subPart,
			IsLFS:    false,
		}, nil
	}

	if strings.HasSuffix(urlPath, ".git") {
		if !validateRepoPath(urlPath) {
			return nil, ErrInvalidPath
		}
		return &ParsedRequest{
			RepoPath: urlPath,
			SubPath:  "",
			IsLFS:    false,
		}, nil
	}

	// Also support URLs without .git (e.g. "path/to/repo")
	if !strings.Contains(filepath.Base(urlPath), ".") {
		repoPart := urlPath + ".git"
		if !validateRepoPath(repoPart) {
			return nil, ErrInvalidPath
		}
		return &ParsedRequest{
			RepoPath: repoPart,
			SubPath:  "",
			IsLFS:    false,
		}, nil
	}

	return nil, ErrInvalidPath
}

// validateRepoPath validates that repository name is clean and does not match scanner/malicious patterns.
func validateRepoPath(repoPart string) bool {
	clean := strings.Trim(filepath.ToSlash(repoPart), "/")
	if clean == "" || clean == ".git" {
		return false
	}
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		if part == "" {
			return false
		}
		// If last part ends with .git, check base name
		if i == len(parts)-1 && strings.HasSuffix(part, ".git") {
			base := strings.TrimSuffix(part, ".git")
			if base == "" || strings.HasPrefix(base, ".") {
				return false
			}
			lowerBase := strings.ToLower(base)
			for _, ext := range suspiciousExtensions {
				if strings.HasSuffix(lowerBase, ext) {
					return false
				}
			}
		} else {
			// Subdirectory segment
			if strings.HasPrefix(part, ".") {
				return false
			}
			lowerPart := strings.ToLower(part)
			for _, ext := range suspiciousExtensions {
				if strings.HasSuffix(lowerPart, ext) || strings.Contains(lowerPart, ext+"/") {
					return false
				}
			}
		}
	}
	return true
}
