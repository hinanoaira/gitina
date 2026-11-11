package git

import (
	"os"
	"testing"
)

func TestRepoReader_FlagInjectionPrevention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina-reader-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := EnsureRepo(tempDir); err != nil {
		t.Fatalf("failed to ensure repo: %v", err)
	}

	reader := NewRepoReader(tempDir)

	maliciousRefs := []string{
		"--output=/tmp/pwn",
		"--help",
		"-n1",
		"-all",
	}

	for _, badRef := range maliciousRefs {
		t.Run("commits_"+badRef, func(t *testing.T) {
			_, err := reader.ListCommits(badRef, 10)
			if err == nil {
				t.Errorf("ListCommits should fail on malicious ref %q", badRef)
			}
		})

		t.Run("tree_"+badRef, func(t *testing.T) {
			_, err := reader.ListTree(badRef, "")
			if err == nil {
				t.Errorf("ListTree should fail on malicious ref %q", badRef)
			}
		})

		t.Run("blob_"+badRef, func(t *testing.T) {
			_, err := reader.GetBlob(badRef, "README.md")
			if err == nil {
				t.Errorf("GetBlob should fail on malicious ref %q", badRef)
			}
		})
	}
}
