package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type BranchInfo struct {
	Name       string
	IsDefault  bool
	CommitHash string
	CommitMsg  string
	AuthorDate time.Time
}

type CommitInfo struct {
	Hash       string
	ShortHash  string
	AuthorName string
	AuthorDate time.Time
	Message    string
}

type TreeEntry struct {
	Mode string
	Type string // "blob" or "tree"
	Hash string
	Name string
	Path string
	Size int64
}

type BlobContent struct {
	Name     string
	Path     string
	Size     int64
	IsBinary bool
	Content  string
}

type RepoReader struct {
	repoDir string
}

func NewRepoReader(repoDir string) *RepoReader {
	return &RepoReader{repoDir: repoDir}
}

// GetDefaultBranch returns the default branch name (e.g. "main" or "master").
func (r *RepoReader) GetDefaultBranch() string {
	cmd := exec.Command("git", "--git-dir", r.repoDir, "symbolic-ref", "--short", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		def := strings.TrimSpace(string(out))
		if def != "" {
			return def
		}
	}
	// Fallback check if refs/heads/main exists
	if _, err := r.GetCommit("main"); err == nil {
		return "main"
	}
	if _, err := r.GetCommit("master"); err == nil {
		return "master"
	}
	return "main"
}

// ListBranches returns all local branches in the repository.
func (r *RepoReader) ListBranches() ([]BranchInfo, error) {
	defaultBranch := r.GetDefaultBranch()
	cmd := exec.Command("git", "--git-dir", r.repoDir, "for-each-ref", "--format=%(refname:short)|%(objectname)|%(committerdate:iso8601)|%(contents:subject)", "refs/heads/")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []BranchInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		branches = append(branches, BranchInfo{
			Name:       parts[0],
			IsDefault:  parts[0] == defaultBranch,
			CommitHash: parts[1],
			AuthorDate: t,
			CommitMsg:  parts[3],
		})
	}
	return branches, nil
}

// ListCommits returns recent commits from a branch or ref up to limit.
func (r *RepoReader) ListCommits(ref string, limit int) ([]CommitInfo, error) {
	if ref == "" {
		ref = r.GetDefaultBranch()
	}
	if limit <= 0 {
		limit = 30
	}

	cmd := exec.Command("git", "--git-dir", r.repoDir, "log", fmt.Sprintf("-n%d", limit), "--format=%H|%h|%an|%cI|%s", ref)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, parts[3])
		commits = append(commits, CommitInfo{
			Hash:       parts[0],
			ShortHash:  parts[1],
			AuthorName: parts[2],
			AuthorDate: t,
			Message:    parts[4],
		})
	}
	return commits, nil
}

// GetCommit returns commit info for a specific ref.
func (r *RepoReader) GetCommit(ref string) (*CommitInfo, error) {
	commits, err := r.ListCommits(ref, 1)
	if err != nil || len(commits) == 0 {
		return nil, errors.New("commit not found")
	}
	return &commits[0], nil
}

// ListTree lists files and subdirectories at subPath in ref.
func (r *RepoReader) ListTree(ref, subPath string) ([]TreeEntry, error) {
	if ref == "" {
		ref = r.GetDefaultBranch()
	}
	subPath = strings.Trim(subPath, "/")

	treeRef := ref
	if subPath != "" {
		treeRef = fmt.Sprintf("%s:%s", ref, subPath)
	}

	cmd := exec.Command("git", "--git-dir", r.repoDir, "ls-tree", "-l", treeRef)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		// Format: <mode> <type> <object> <size>\t<file>
		tabIdx := strings.Index(line, "\t")
		if tabIdx == -1 {
			continue
		}
		meta := line[:tabIdx]
		name := line[tabIdx+1:]

		fields := strings.Fields(meta)
		if len(fields) < 3 {
			continue
		}
		mode := fields[0]
		objType := fields[1]
		hash := fields[2]
		var size int64
		if len(fields) >= 4 && fields[3] != "-" {
			size, _ = strconv.ParseInt(fields[3], 10, 64)
		}

		entryPath := name
		if subPath != "" {
			entryPath = path.Join(subPath, name)
		}

		entries = append(entries, TreeEntry{
			Mode: mode,
			Type: objType,
			Hash: hash,
			Name: name,
			Path: entryPath,
			Size: size,
		})
	}
	return entries, nil
}

// GetBlob returns the content of a file at filePath in ref.
func (r *RepoReader) GetBlob(ref, filePath string) (*BlobContent, error) {
	if ref == "" {
		ref = r.GetDefaultBranch()
	}
	filePath = strings.Trim(filePath, "/")
	target := fmt.Sprintf("%s:%s", ref, filePath)

	cmd := exec.Command("git", "--git-dir", r.repoDir, "cat-file", "-p", target)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	isBinary := false
	if len(out) > 0 {
		// Check for null bytes or invalid UTF-8
		checkLen := len(out)
		if checkLen > 8000 {
			checkLen = 8000
		}
		if bytes.IndexByte(out[:checkLen], 0) != -1 || !utf8.Valid(out[:checkLen]) {
			isBinary = true
		}
	}

	contentStr := ""
	if !isBinary {
		contentStr = string(out)
	}

	return &BlobContent{
		Name:     path.Base(filePath),
		Path:     filePath,
		Size:     int64(len(out)),
		IsBinary: isBinary,
		Content:  contentStr,
	}, nil
}
