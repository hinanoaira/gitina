package lfs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gitina/internal/config"
)

var (
	oidRegex = regexp.MustCompile(`(?m)^oid sha256:([a-f0-9]{64})$`)
)

// GCStats records statistics of a garbage collection run.
type GCStats struct {
	TotalObjects   int
	ReachableOIDs  int
	OrphanObjects  int
	DeletedObjects int
	FreedBytes     int64
	DryRun         bool
	Duration       time.Duration
}

// GarbageCollector performs garbage collection on unused LFS objects.
type GarbageCollector struct {
	repoRoot string
	storage  StorageBackend
	cfg      config.LFSGCConfig
}

// NewGarbageCollector creates a new GarbageCollector.
func NewGarbageCollector(repoRoot string, storage StorageBackend, cfg config.LFSGCConfig) *GarbageCollector {
	return &GarbageCollector{
		repoRoot: repoRoot,
		storage:  storage,
		cfg:      cfg,
	}
}

// StartWorker starts a background worker that runs GC periodically based on IntervalHours.
func (gc *GarbageCollector) StartWorker(ctx context.Context) {
	if !gc.cfg.Enabled {
		return
	}

	interval := time.Duration(gc.cfg.IntervalHours) * time.Hour
	if interval < 1*time.Hour {
		interval = 24 * time.Hour
	}

	log.Printf("[LFS GC] Background worker started (interval: %v, grace period: %d days, dry run: %v)",
		interval, gc.cfg.GracePeriodDays, gc.cfg.DryRun)

	go func() {
		// Run initial check slightly after startup (e.g. 1 minute later)
		select {
		case <-time.After(1 * time.Minute):
			if stats, err := gc.Run(ctx); err != nil {
				log.Printf("[LFS GC] Periodic run error: %v", err)
			} else if stats.DeletedObjects > 0 || stats.OrphanObjects > 0 {
				log.Printf("[LFS GC] Run finished: scanned %d objects, found %d orphans, deleted %d objects, freed %d bytes (took %v)",
					stats.TotalObjects, stats.OrphanObjects, stats.DeletedObjects, stats.FreedBytes, stats.Duration)
			}
		case <-ctx.Done():
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats, err := gc.Run(ctx)
				if err != nil {
					log.Printf("[LFS GC] Periodic run error: %v", err)
				} else if stats.DeletedObjects > 0 || stats.OrphanObjects > 0 {
					log.Printf("[LFS GC] Run finished: scanned %d objects, found %d orphans, deleted %d objects, freed %d bytes (took %v)",
						stats.TotalObjects, stats.OrphanObjects, stats.DeletedObjects, stats.FreedBytes, stats.Duration)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Run executes a single GC pass.
func (gc *GarbageCollector) Run(ctx context.Context) (*GCStats, error) {
	startTime := time.Now()
	stats := &GCStats{
		DryRun: gc.cfg.DryRun,
	}

	// 1. Discover all reachable LFS OIDs across all Git repositories in repoRoot
	reachableOIDs, err := gc.ScanReachableOIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to scan reachable OIDs: %w", err)
	}
	stats.ReachableOIDs = len(reachableOIDs)

	// 2. List all objects from the LFS storage backend
	storedObjects, err := gc.storage.ListObjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list stored LFS objects: %w", err)
	}
	stats.TotalObjects = len(storedObjects)

	gracePeriod := time.Duration(gc.cfg.GracePeriodDays) * 24 * time.Hour
	now := time.Now()

	var toDelete []string
	for _, obj := range storedObjects {
		// If reachable, it's safe and active
		if _, exists := reachableOIDs[obj.OID]; exists {
			continue
		}

		// It is an orphan. Check if within grace period
		if !obj.LastModified.IsZero() && now.Sub(obj.LastModified) < gracePeriod {
			// Protected by grace period (likely an in-flight push or recent commit)
			continue
		}

		stats.OrphanObjects++
		stats.FreedBytes += obj.Size
		toDelete = append(toDelete, obj.OID)
	}

	// 3. Delete orphans if not in dry-run mode
	if len(toDelete) > 0 && !gc.cfg.DryRun {
		if err := gc.storage.DeleteObjects(ctx, toDelete); err != nil {
			return stats, fmt.Errorf("failed to delete orphan LFS objects: %w", err)
		}
		stats.DeletedObjects = len(toDelete)
	} else if gc.cfg.DryRun {
		stats.DeletedObjects = 0
	}

	stats.Duration = time.Since(startTime)
	return stats, nil
}

// ScanReachableOIDs traverses all bare Git repositories under repoRoot and finds all active LFS pointers.
func (gc *GarbageCollector) ScanReachableOIDs(ctx context.Context) (map[string]struct{}, error) {
	reachable := make(map[string]struct{})

	repos, err := gc.findBareRepos(gc.repoRoot)
	if err != nil {
		return nil, err
	}

	for _, repoDir := range repos {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		oids, err := scanRepoLFSPointers(repoDir)
		if err != nil {
			log.Printf("[LFS GC] Warning: failed to scan pointers in %s: %v", repoDir, err)
			continue
		}
		for _, oid := range oids {
			reachable[oid] = struct{}{}
		}
	}

	return reachable, nil
}

// findBareRepos locates all directories containing HEAD and objects (bare Git repos).
func (gc *GarbageCollector) findBareRepos(root string) ([]string, error) {
	var repos []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}

		// Check if current directory is a bare git repo
		if isBareRepo(path) {
			repos = append(repos, path)
			return filepath.SkipDir
		}
		return nil
	})
	return repos, err
}

func isBareRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, "objects")); err != nil {
		return false
	}
	return true
}

// scanRepoLFSPointers runs rev-list on all refs filtering small blobs to find LFS pointers.
func scanRepoLFSPointers(repoDir string) ([]string, error) {
	// git rev-list --objects --all --filter=blob:limit=1k
	cmd := exec.Command("git", "--git-dir", repoDir, "rev-list", "--objects", "--all", "--filter=blob:limit=1k")
	out, err := cmd.Output()
	if err != nil {
		// Empty repository or no commits
		return nil, nil
	}

	var blobHashes []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		hash := parts[0]
		// In git rev-list --objects, commits, trees and blobs are listed.
		// We'll inspect each object that might be a blob.
		blobHashes = append(blobHashes, hash)
	}

	if len(blobHashes) == 0 {
		return nil, nil
	}

	// Use git cat-file --batch-check and --batch to efficiently extract LFS OIDs
	// Batch check if object is a blob of size around 100-200 bytes
	return extractOIDsFromBlobs(repoDir, blobHashes)
}

func extractOIDsFromBlobs(repoDir string, hashes []string) ([]string, error) {
	cmd := exec.Command("git", "--git-dir", repoDir, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var foundOIDs []string
	readDone := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(stdout)
		for {
			headerLine, err := reader.ReadString('\n')
			if err != nil {
				readDone <- nil
				return
			}
			headerLine = strings.TrimSpace(headerLine)
			// Format: <hash> <type> <size>
			parts := strings.Split(headerLine, " ")
			if len(parts) < 3 {
				continue
			}
			objType := parts[1]
			var size int64
			fmt.Sscanf(parts[2], "%d", &size)

			content := make([]byte, size)
			_, err = ioReadFull(reader, content)
			// cat-file batch outputs a trailing newline after object data
			_, _ = reader.ReadByte()

			if err != nil {
				readDone <- err
				return
			}

			if objType == "blob" && size > 50 && size < 500 {
				matches := oidRegex.FindSubmatch(content)
				if len(matches) > 1 {
					foundOIDs = append(foundOIDs, string(matches[1]))
				}
			}
		}
	}()

	for _, h := range hashes {
		fmt.Fprintf(stdin, "%s\n", h)
	}
	stdin.Close()

	if err := <-readDone; err != nil {
		_ = cmd.Wait()
		return nil, err
	}
	_ = cmd.Wait()

	return foundOIDs, nil
}

func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
