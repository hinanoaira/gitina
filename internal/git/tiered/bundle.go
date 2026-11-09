package tiered

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrEmptyRepo = errors.New("repository has no commits or references")

// HasRefs checks if the repository has any refs/branches/tags.
func HasRefs(repoDir string) (bool, error) {
	out, err := exec.Command("git", "-C", repoDir, "for-each-ref", "--count=1").Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// CreateBundle creates a git bundle archive of the entire repository at bundlePath.
// If the repository has no commits/refs, it returns ErrEmptyRepo.
func CreateBundle(repoDir, bundlePath string) error {
	hasRefs, err := HasRefs(repoDir)
	if err != nil {
		return err
	}
	if !hasRefs {
		return ErrEmptyRepo
	}

	if err := os.MkdirAll(filepath.Dir(bundlePath), 0755); err != nil {
		return fmt.Errorf("failed to create bundle directory: %w", err)
	}

	// git -C repoDir bundle create bundlePath --all
	cmd := exec.Command("git", "-C", repoDir, "bundle", "create", bundlePath, "--all")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git bundle create failed in %q: %s: %w", repoDir, string(out), err)
	}

	return nil
}

// RestoreBundle restores a bare repository from a bundle archive into targetRepoDir.
func RestoreBundle(bundlePath, targetRepoDir string) error {
	if err := os.MkdirAll(filepath.Dir(targetRepoDir), 0755); err != nil {
		return fmt.Errorf("failed to create target repo parent directory: %w", err)
	}

	// Remove target if it partially exists
	_ = os.RemoveAll(targetRepoDir)

	// git clone --bare bundlePath targetRepoDir
	cmd := exec.Command("git", "clone", "--bare", bundlePath, targetRepoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --bare from bundle failed: %s: %w", string(out), err)
	}

	// Ensure http.receivepack is enabled
	cmd = exec.Command("git", "-C", targetRepoDir, "config", "http.receivepack", "true")
	_ = cmd.Run()

	return nil
}
