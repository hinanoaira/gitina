package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server            ServerConfig            `yaml:"server"`
	RepositoryStorage RepositoryStorageConfig `yaml:"repository_storage"`
	LFS               LFSConfig               `yaml:"lfs"`
	GitHub            GitHubConfig            `yaml:"github"`
	Admins            []string                `yaml:"admins"`
	Repositories      []RepositoryConfig      `yaml:"repositories"`
}

type ServerConfig struct {
	Listen    string `yaml:"listen"`     // e.g. ":8080"
	BaseURL   string `yaml:"base_url"`   // e.g. "https://git.hns.jp"
	RepoRoot  string `yaml:"repo_root"`  // directory where bare repos are stored
	JWTSecret string `yaml:"jwt_secret"` // secret key for JWT signing
}

type RepositoryStorageConfig struct {
	Type  string                 `yaml:"type"` // "local" or "s3"
	S3    RepositoryStorageS3    `yaml:"s3"`
	Cache RepositoryStorageCache `yaml:"cache"`
}

type RepositoryStorageS3 struct {
	Endpoint        string `yaml:"endpoint"` // e.g. Cloudflare R2 or MinIO URL
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	Prefix          string `yaml:"prefix"` // default: "repos/"
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	UsePathStyle    bool   `yaml:"use_path_style"`
}

type RepositoryStorageCache struct {
	MaxSizeGB int `yaml:"max_size_gb"` // default: 15
}

type LFSConfig struct {
	Storage string            `yaml:"storage"` // "local" or "s3"
	Local   LFSLocalConfig    `yaml:"local"`
	S3      LFSS3Config       `yaml:"s3"`
	GC      LFSGCConfig       `yaml:"gc"`
}

type LFSGCConfig struct {
	Enabled         bool `yaml:"enabled"`            // whether automatic LFS GC is enabled
	IntervalHours   int  `yaml:"interval_hours"`     // execution interval in hours (default: 24)
	GracePeriodDays int  `yaml:"grace_period_days"`  // minimum age in days before an orphan is deleted (default: 7)
	DryRun          bool `yaml:"dry_run"`            // if true, logs actions without deleting
}

type LFSLocalConfig struct {
	Path string `yaml:"path"` // local directory for LFS objects
}

type LFSS3Config struct {
	Endpoint        string `yaml:"endpoint"`          // optional for S3 compatible (e.g. Cloudflare R2, MinIO)
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	UsePathStyle    bool   `yaml:"use_path_style"`
}

type GitHubConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type RepositoryConfig struct {
	Path          string            `yaml:"path"`          // e.g. "open-source/*", "path/to/directory.git"
	Public        bool              `yaml:"public"`        // anyone can clone/fetch
	Collaborators map[string]string `yaml:"collaborators"` // username -> "rw" or "ro"
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	// Defaults
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.RepoRoot == "" {
		cfg.Server.RepoRoot = "./data/repositories"
	}
	cfg.Server.RepoRoot = resolveConfigPath(path, cfg.Server.RepoRoot)

	if cfg.Server.JWTSecret == "" {
		return nil, fmt.Errorf("server.jwt_secret is required")
	}
	if cfg.Server.BaseURL == "" {
		cfg.Server.BaseURL = "http://localhost:8080"
	}
	cfg.Server.BaseURL = strings.TrimRight(cfg.Server.BaseURL, "/")

	if cfg.RepositoryStorage.Type == "" {
		cfg.RepositoryStorage.Type = "local"
	}
	if cfg.RepositoryStorage.S3.Prefix == "" {
		cfg.RepositoryStorage.S3.Prefix = "repos/"
	}
	if !strings.HasSuffix(cfg.RepositoryStorage.S3.Prefix, "/") {
		cfg.RepositoryStorage.S3.Prefix += "/"
	}
	if cfg.RepositoryStorage.Cache.MaxSizeGB <= 0 {
		cfg.RepositoryStorage.Cache.MaxSizeGB = 15
	}

	if cfg.LFS.Storage == "" {
		cfg.LFS.Storage = "local"
	}
	if cfg.LFS.Storage == "local" {
		if cfg.LFS.Local.Path == "" {
			cfg.LFS.Local.Path = "./data/lfs"
		}
		cfg.LFS.Local.Path = resolveConfigPath(path, cfg.LFS.Local.Path)
	}
	if cfg.LFS.GC.IntervalHours <= 0 {
		cfg.LFS.GC.IntervalHours = 24
	}
	if cfg.LFS.GC.GracePeriodDays <= 0 {
		cfg.LFS.GC.GracePeriodDays = 7
	}

	return &cfg, nil
}

// resolveConfigPath resolves relative paths against:
// 1. Current working directory (if exists)
// 2. Config file directory (if exists)
// 3. /var/lib/gitina (standard service working directory, if exists)
// 4. Fallback: absolute path based on cwd
func resolveConfigPath(configFilePath, targetPath string) string {
	if filepath.IsAbs(targetPath) {
		return filepath.Clean(targetPath)
	}

	// 1. Check current working directory
	if cwdAbs, err := filepath.Abs(targetPath); err == nil {
		if _, err := os.Stat(cwdAbs); err == nil {
			return cwdAbs
		}
	}

	// 2. Check relative to config file location
	if configFilePath != "" {
		cfgDir := filepath.Dir(configFilePath)
		joined := filepath.Join(cfgDir, targetPath)
		if _, err := os.Stat(joined); err == nil {
			if abs, err := filepath.Abs(joined); err == nil {
				return abs
			}
			return joined
		}
	}

	// 3. Check /var/lib/gitina
	varLibCandidate := filepath.Join("/var/lib/gitina", targetPath)
	if _, err := os.Stat(varLibCandidate); err == nil {
		return varLibCandidate
	}

	// Default fallback to cwd absolute
	cwdAbs, err := filepath.Abs(targetPath)
	if err == nil {
		return cwdAbs
	}
	return targetPath
}
