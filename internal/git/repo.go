package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// EnsureRepo checks if the bare repository exists at fullPath.
// If it does not exist, it automatically initializes a bare repository.
func EnsureRepo(fullPath string) error {
	// Check if already exists (HEAD file present)
	headPath := filepath.Join(fullPath, "HEAD")
	if _, err := os.Stat(headPath); err == nil {
		return nil
	}

	// Create parent directories
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", fullPath, err)
	}

	// Run git init --bare
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = fullPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init --bare failed in %q: %s: %w", fullPath, string(output), err)
	}

	// Set default branch to main
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = fullPath
	_ = cmd.Run()

	// Enable http.receivepack so push is supported over HTTP
	cmd = exec.Command("git", "config", "http.receivepack", "true")
	cmd.Dir = fullPath
	_ = cmd.Run()

	return nil
}
