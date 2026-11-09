package tiered

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type CacheItem struct {
	RepoPath   string
	FullPath   string
	Size       int64
	LastAccess time.Time
}

// InspectCache traverses repoRoot and returns information about all cached repositories.
func InspectCache(repoRoot string) ([]CacheItem, int64, error) {
	var items []CacheItem
	var totalSize int64

	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".trash" {
			return filepath.SkipDir
		}

		headPath := filepath.Join(p, "HEAD")
		if _, statErr := os.Stat(headPath); statErr == nil {
			rel, _ := filepath.Rel(repoRoot, p)
			rel = filepath.ToSlash(rel)

			size, latestTime := getDirInfo(p)
			items = append(items, CacheItem{
				RepoPath:   rel,
				FullPath:   p,
				Size:       size,
				LastAccess: latestTime,
			})
			totalSize += size
			return filepath.SkipDir
		}

		return nil
	})

	return items, totalSize, err
}

// PurgeOldestCache purges repositories until total size drops below targetBytes.
// lockedRepos is a set of repository paths currently in use (should not be purged).
func PurgeOldestCache(repoRoot string, maxBytes int64, targetBytes int64, lockedRepos map[string]bool) (int, error) {
	items, currentSize, err := InspectCache(repoRoot)
	if err != nil {
		return 0, err
	}

	if currentSize <= maxBytes {
		return 0, nil
	}

	// Sort items by LastAccess ascending (oldest first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastAccess.Before(items[j].LastAccess)
	})

	purgedCount := 0
	for _, item := range items {
		if currentSize <= targetBytes {
			break
		}
		if lockedRepos[item.RepoPath] {
			continue // skip locked/active repos
		}

		if err := os.RemoveAll(item.FullPath); err == nil {
			currentSize -= item.Size
			purgedCount++
		}
	}

	return purgedCount, nil
}

func getDirInfo(dir string) (int64, time.Time) {
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
