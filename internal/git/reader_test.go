package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepoReader(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitina_reader_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	repoDir := filepath.Join(tempDir, "test.git")
	_ = os.MkdirAll(repoDir, 0755)

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v failed: %s: %v", name, args, string(out), err)
		}
	}

	// 1. Init bare repo
	runCmd(tempDir, "git", "init", "--bare", repoDir)

	// 2. Clone locally to make initial commits
	workDir := filepath.Join(tempDir, "work")
	runCmd(tempDir, "git", "clone", repoDir, workDir)
	runCmd(workDir, "git", "config", "commit.gpgsign", "false")

	// Create files
	_ = os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Hello Gitina\nThis is a test."), 0644)
	_ = os.MkdirAll(filepath.Join(workDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(workDir, "src", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	runCmd(workDir, "git", "add", ".")
	runCmd(workDir, "git", "commit", "-m", "Initial commit")
	runCmd(workDir, "git", "branch", "-M", "main")
	runCmd(workDir, "git", "push", "origin", "main")

	// Create a feature branch
	runCmd(workDir, "git", "checkout", "-b", "feature")
	_ = os.WriteFile(filepath.Join(workDir, "feature.txt"), []byte("feature content"), 0644)
	runCmd(workDir, "git", "add", ".")
	runCmd(workDir, "git", "commit", "-m", "Add feature")
	runCmd(workDir, "git", "push", "origin", "feature")

	// Now test RepoReader on the bare repo
	reader := NewRepoReader(repoDir)

	// Check default branch
	defBranch := reader.GetDefaultBranch()
	if defBranch != "main" {
		t.Fatalf("expected default branch 'main', got %q", defBranch)
	}

	// List branches
	branches, err := reader.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}

	// List commits
	commits, err := reader.ListCommits("main", 10)
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}
	if len(commits) != 1 || commits[0].Message != "Initial commit" {
		t.Fatalf("unexpected commits: %+v", commits)
	}

	// List tree root
	entries, err := reader.ListTree("main", "")
	if err != nil {
		t.Fatalf("ListTree root failed: %v", err)
	}
	if len(entries) != 2 { // README.md and src
		t.Fatalf("expected 2 tree entries, got %d", len(entries))
	}

	// List tree src/
	srcEntries, err := reader.ListTree("main", "src")
	if err != nil {
		t.Fatalf("ListTree src failed: %v", err)
	}
	if len(srcEntries) != 1 || srcEntries[0].Name != "main.go" {
		t.Fatalf("expected main.go in src, got %+v", srcEntries)
	}

	// Get blob README.md
	blob, err := reader.GetBlob("main", "README.md")
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	if blob.IsBinary || blob.Content != "# Hello Gitina\nThis is a test." {
		t.Fatalf("unexpected blob content: %+v", blob)
	}
}
