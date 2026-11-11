package repo

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gitina/internal/git/tiered"
)

type RepoInfo struct {
	Path      string
	DiskSize  int64
	UpdatedAt time.Time
	IsCached  bool
	Storage   string // "local" or "s3"
}

type Manager struct {
	repoRoot  string
	tieredMgr *tiered.Manager
}

func NewManager(repoRoot string, tieredMgr *tiered.Manager) *Manager {
	return &Manager{
		repoRoot:  repoRoot,
		tieredMgr: tieredMgr,
	}
}

// RepoRoot returns the configured local repository root directory.
func (m *Manager) RepoRoot() string {
	return m.repoRoot
}

// List returns a list of all bare repositories hosted under repoRoot or on S3.
func (m *Manager) List(ctx context.Context) ([]RepoInfo, error) {
	if m.tieredMgr != nil && m.tieredMgr.IsTiered() {
		return m.listTiered(ctx)
	}
	return m.listLocal()
}

func (m *Manager) listTiered(ctx context.Context) ([]RepoInfo, error) {
	s3Repos, err := m.tieredMgr.S3().ListRepos(ctx)
	if err != nil {
		return nil, err
	}

	resMap := make(map[string]RepoInfo)
	for _, r := range s3Repos {
		localPath := filepath.Join(m.repoRoot, r.RepoPath, "HEAD")
		isCached := false
		if _, statErr := os.Stat(localPath); statErr == nil {
			isCached = true
		}
		resMap[r.RepoPath] = RepoInfo{
			Path:      r.RepoPath,
			DiskSize:  r.Size,
			UpdatedAt: r.UpdatedAt,
			IsCached:  isCached,
			Storage:   "s3",
		}
	}

	// Also inspect local for any newly created repos not yet synced
	localRepos, _ := m.listLocal()
	for _, lr := range localRepos {
		if _, exists := resMap[lr.Path]; !exists {
			lr.IsCached = true
			lr.Storage = "local"
			resMap[lr.Path] = lr
		}
	}

	var list []RepoInfo
	for _, item := range resMap {
		list = append(list, item)
	}
	return list, nil
}

func (m *Manager) listLocal() ([]RepoInfo, error) {
	if stat, err := os.Stat(m.repoRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("repository root %q does not exist", m.repoRoot)
		}
		return nil, fmt.Errorf("cannot access repository root %q: %w", m.repoRoot, err)
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("repository root %q is not a directory", m.repoRoot)
	}

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
				IsCached:  false,
				Storage:   "local",
			})
			return filepath.SkipDir // no need to dive into this repo
		}

		return nil
	})

	return list, err
}

// Remove deletes or trashes a repository both locally and on S3.
func (m *Manager) Remove(ctx context.Context, repoPath string, force bool) (string, error) {
	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}

	target := filepath.Join(m.repoRoot, repoPath)
	localExists := false
	if _, err := os.Stat(target); err == nil {
		localExists = true
	}

	s3Exists := false
	if m.tieredMgr != nil && m.tieredMgr.IsTiered() {
		if ex, err := m.tieredMgr.S3().Exists(ctx, repoPath); err == nil && ex {
			s3Exists = true
		}
	}

	if !localExists && !s3Exists {
		return "", fmt.Errorf("repository %q does not exist", repoPath)
	}

	// 1. Remove from S3 if present
	if s3Exists {
		if err := m.tieredMgr.S3().DeleteBundle(ctx, repoPath, force); err != nil {
			return "", fmt.Errorf("failed to delete repository from S3: %w", err)
		}
	}

	// 2. Remove from local if present
	if localExists {
		if force {
			if err := os.RemoveAll(target); err != nil {
				return "", fmt.Errorf("failed to permanently delete local repository: %w", err)
			}
		} else {
			trashDir := filepath.Join(m.repoRoot, ".trash")
			if err := os.MkdirAll(trashDir, 0755); err != nil {
				return "", fmt.Errorf("failed to create trash directory: %w", err)
			}
			inheritOwnership(m.repoRoot, trashDir)

			sanitizedName := strings.ReplaceAll(repoPath, "/", "_")
			trashTarget := filepath.Join(trashDir, fmt.Sprintf("%d_%s", time.Now().Unix(), sanitizedName))

			if err := os.Rename(target, trashTarget); err != nil {
				return "", fmt.Errorf("failed to move local repository to trash: %w", err)
			}
		}
		cleanEmptyParents(m.repoRoot, filepath.Dir(target))
	}

	if force {
		return "permanently deleted", nil
	}
	return "moved to trash", nil
}

func cleanEmptyParents(root, dir string) {
	root = filepath.Clean(root)
	dir = filepath.Clean(dir)
	for dir != root && strings.HasPrefix(dir, root) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
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
