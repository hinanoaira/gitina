package repo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type RepoInfo struct {
	Path      string
	DiskSize  int64
	UpdatedAt time.Time
}

type Manager struct {
	repoRoot string
}

func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

// List returns a list of all bare repositories hosted under repoRoot.
func (m *Manager) List() ([]RepoInfo, error) {
	var list []RepoInfo

	err := filepath.WalkDir(m.repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		// Skip .trash directory
		if d.Name() == ".trash" {
			return filepath.SkipDir
		}

		// Check if it's a bare repository (contains HEAD file)
		if _, statErr := os.Stat(filepath.Join(p, "HEAD")); statErr == nil {
			rel, _ := filepath.Rel(m.repoRoot, p)
			rel = filepath.ToSlash(rel)

			size, updated := getDirStats(p)
			list = append(list, RepoInfo{
				Path:      rel,
				DiskSize:  size,
				UpdatedAt: updated,
			})
			return filepath.SkipDir // no need to dive into this repo
		}

		return nil
	})

	return list, err
}

// Remove deletes or trashes a repository.
func (m *Manager) Remove(repoPath string, force bool) (string, error) {
	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}

	target := filepath.Join(m.repoRoot, repoPath)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return "", fmt.Errorf("repository %q does not exist", repoPath)
	}

	if force {
		if err := os.RemoveAll(target); err != nil {
			return "", fmt.Errorf("failed to permanently delete repository: %w", err)
		}
		return "permanently deleted", nil
	}

	// Move to .trash
	trashDir := filepath.Join(m.repoRoot, ".trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create trash directory: %w", err)
	}
	inheritOwnership(m.repoRoot, trashDir)

	sanitizedName := strings.ReplaceAll(repoPath, "/", "_")
	trashTarget := filepath.Join(trashDir, fmt.Sprintf("%d_%s", time.Now().Unix(), sanitizedName))

	if err := os.Rename(target, trashTarget); err != nil {
		return "", fmt.Errorf("failed to move repository to trash: %w", err)
	}

	return fmt.Sprintf("moved to trash (%s)", filepath.Base(trashTarget)), nil
}

// CleanTrash removes items in .trash older than the specified days.
func (m *Manager) CleanTrash(days int) (int, error) {
	trashDir := filepath.Join(m.repoRoot, ".trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	count := 0

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			p := filepath.Join(trashDir, entry.Name())
			if err := os.RemoveAll(p); err == nil {
				count++
			}
		}
	}

	return count, nil
}

func getDirStats(dir string) (int64, time.Time) {
	var totalSize int64
	var latestTime time.Time

	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			if !d.IsDir() {
				totalSize += info.Size()
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
			}
		}
		return nil
	})

	return totalSize, latestTime
}

func inheritOwnership(srcDir, destDir string) {
	if info, err := os.Stat(srcDir); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			_ = os.Chown(destDir, int(stat.Uid), int(stat.Gid))
		}
	}
}
