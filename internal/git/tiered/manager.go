package tiered

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitina/internal/config"
)

type Manager struct {
	cfg       *config.Config
	s3        *S3Client
	repoLocks sync.Mutex
	locks     map[string]*sync.Mutex
	active    map[string]bool // currently in-use repos
}

func NewManager(ctx context.Context, cfg *config.Config) (*Manager, error) {
	var s3Client *S3Client
	var err error

	if cfg.RepositoryStorage.Type == "s3" {
		s3Client, err = NewS3Client(ctx, cfg.RepositoryStorage.S3)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tiered storage S3 client: %w", err)
		}
		log.Printf("[Tiered] Initialized S3 storage backend (bucket: %s, prefix: %s)", cfg.RepositoryStorage.S3.Bucket, cfg.RepositoryStorage.S3.Prefix)
	}

	m := &Manager{
		cfg:    cfg,
		s3:     s3Client,
		locks:  make(map[string]*sync.Mutex),
		active: make(map[string]bool),
	}

	if cfg.RepositoryStorage.Type == "s3" {
		go m.startLRUWorker()
	}

	return m, nil
}

func (m *Manager) IsTiered() bool {
	return m.cfg.RepositoryStorage.Type == "s3" && m.s3 != nil
}

func (m *Manager) S3() *S3Client {
	return m.s3
}

func (m *Manager) getMutex(repoPath string) *sync.Mutex {
	m.repoLocks.Lock()
	defer m.repoLocks.Unlock()

	mu, exists := m.locks[repoPath]
	if !exists {
		mu = &sync.Mutex{}
		m.locks[repoPath] = mu
	}
	return mu
}

// Acquire locks the repository and ensures it is present in the local cache.
// Returns an unlock function that must be deferred or called when Git operation finishes.
func (m *Manager) Acquire(ctx context.Context, repoPath string) (func(), error) {
	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}

	mu := m.getMutex(repoPath)
	mu.Lock()

	m.repoLocks.Lock()
	m.active[repoPath] = true
	m.repoLocks.Unlock()

	release := func() {
		m.repoLocks.Lock()
		delete(m.active, repoPath)
		m.repoLocks.Unlock()
		mu.Unlock()
	}

	fullLocalPath := filepath.Join(m.cfg.Server.RepoRoot, repoPath)
	headPath := filepath.Join(fullLocalPath, "HEAD")

	// 1. Check if present in local cache
	if _, err := os.Stat(headPath); err == nil {
		// Cache hit! Update access time
		touch(headPath)
		return release, nil
	}

	// 2. Not in local cache. If tiered storage is not enabled, initialize bare repo locally.
	if !m.IsTiered() {
		if err := initBareRepo(fullLocalPath); err != nil {
			release()
			return nil, fmt.Errorf("failed to auto-init repository: %w", err)
		}
		touch(headPath)
		return release, nil
	}

	// 3. Check if exists on S3
	log.Printf("[Tiered] Cache miss for %q. Checking S3...", repoPath)
	exists, err := m.s3.Exists(ctx, repoPath)
	if err != nil {
		release()
		return nil, fmt.Errorf("failed to check S3 repository existence: %w", err)
	}

	if exists {
		// Fetch bundle from S3 and restore into local cache
		log.Printf("[Tiered] Fetching bundle from S3 for %q...", repoPath)
		tmpBundle := filepath.Join(os.TempDir(), fmt.Sprintf("gitina_%d.bundle", time.Now().UnixNano()))
		defer os.Remove(tmpBundle)

		if err := m.s3.DownloadBundle(ctx, repoPath, tmpBundle); err != nil {
			release()
			return nil, fmt.Errorf("failed to download bundle from S3: %w", err)
		}

		if err := RestoreBundle(tmpBundle, fullLocalPath); err != nil {
			release()
			return nil, fmt.Errorf("failed to restore bundle to local cache: %w", err)
		}
		// Also restore gitina-meta.json if present on S3
		localMetaPath := filepath.Join(fullLocalPath, "gitina-meta.json")
		_ = m.s3.DownloadMetadata(ctx, repoPath, localMetaPath)

		log.Printf("[Tiered] Restored %q to local cache successfully", repoPath)
		touch(headPath)
	} else {
		// New repository: initialize bare repo locally
		if err := initBareRepo(fullLocalPath); err != nil {
			release()
			return nil, fmt.Errorf("failed to auto-init new repository: %w", err)
		}
		log.Printf("[Tiered] Initialized new repository %q locally", repoPath)
		touch(headPath)
	}

	return release, nil
}

func initBareRepo(fullLocalPath string) error {
	if err := os.MkdirAll(fullLocalPath, 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = fullLocalPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init --bare failed: %s: %w", string(out), err)
	}
	_ = exec.Command("git", "-C", fullLocalPath, "symbolic-ref", "HEAD", "refs/heads/main").Run()
	_ = exec.Command("git", "-C", fullLocalPath, "config", "http.receivepack", "true").Run()
	return nil
}

// Sync uploads the updated repository bundle to S3.
func (m *Manager) Sync(ctx context.Context, repoPath string) error {
	if !m.IsTiered() {
		return nil
	}

	repoPath = strings.Trim(filepath.ToSlash(repoPath), "/")
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}

	fullLocalPath := filepath.Join(m.cfg.Server.RepoRoot, repoPath)

	tmpBundle := filepath.Join(os.TempDir(), fmt.Sprintf("gitina_sync_%d.bundle", time.Now().UnixNano()))
	defer os.Remove(tmpBundle)

	// Create bundle
	if err := CreateBundle(fullLocalPath, tmpBundle); err != nil {
		if errors.Is(err, ErrEmptyRepo) {
			// Repository has no commits yet (empty repo), skip bundle sync
			return nil
		}
		return fmt.Errorf("failed to create bundle for sync: %w", err)
	}

	// Upload bundle to S3
	if err := m.s3.UploadBundle(ctx, repoPath, tmpBundle); err != nil {
		return fmt.Errorf("failed to upload bundle to S3: %w", err)
	}

	// Also sync gitina-meta.json if exists locally
	localMetaPath := filepath.Join(fullLocalPath, "gitina-meta.json")
	if err := m.s3.UploadMetadata(ctx, repoPath, localMetaPath); err != nil {
		log.Printf("[Tiered] Warning: failed to upload metadata for %q: %v", repoPath, err)
	}

	log.Printf("[Tiered] Synced %q to S3 successfully", repoPath)
	return nil
}

func (m *Manager) startLRUWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		maxBytes := int64(m.cfg.RepositoryStorage.Cache.MaxSizeGB) * 1024 * 1024 * 1024
		targetBytes := int64(float64(maxBytes) * 0.7) // purge down to 70%

		m.repoLocks.Lock()
		activeCopy := make(map[string]bool, len(m.active))
		for k, v := range m.active {
			activeCopy[k] = v
		}
		m.repoLocks.Unlock()

		purged, err := PurgeOldestCache(m.cfg.Server.RepoRoot, maxBytes, targetBytes, activeCopy)
		if err != nil {
			log.Printf("[Tiered] LRU purge error: %v", err)
		} else if purged > 0 {
			log.Printf("[Tiered] LRU purged %d cached repository/repositories", purged)
		}
	}
}

func touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}
