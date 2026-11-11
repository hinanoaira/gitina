package git

import (
	"testing"
)

func TestParseURLPath(t *testing.T) {
	tests := []struct {
		input       string
		wantRepo    string
		wantSub     string
		wantLFS     bool
		wantErr     bool
	}{
		{
			input:    "/path/to/directory.git/info/refs",
			wantRepo: "path/to/directory.git",
			wantSub:  "info/refs",
			wantLFS:  false,
		},
		{
			input:    "/path/to/directory/hoge.git/git-receive-pack",
			wantRepo: "path/to/directory/hoge.git",
			wantSub:  "git-receive-pack",
			wantLFS:  false,
		},
		{
			input:    "/fuga.git/git-upload-pack",
			wantRepo: "fuga.git",
			wantSub:  "git-upload-pack",
			wantLFS:  false,
		},
		{
			input:    "/deep/nested/repo.git/info/lfs/objects/batch",
			wantRepo: "deep/nested/repo.git",
			wantSub:  "info/lfs/objects/batch",
			wantLFS:  true,
		},
		{
			input:    "/repo.git/info/lfs/objects/1234567890abcdef",
			wantRepo: "repo.git",
			wantSub:  "info/lfs/objects/1234567890abcdef",
			wantLFS:  true,
		},
		{
			input:    "/simple/repo/info/refs", // without .git in URL
			wantRepo: "simple/repo.git",
			wantSub:  "info/refs",
			wantLFS:  false,
		},
		{
			input:   "/../etc/passwd",
			wantErr: true,
		},
		{
			input:   "/.git",
			wantErr: true,
		},
		{
			input:    "/phpinfo.php.git",
			wantRepo: "phpinfo.php.git",
			wantSub:  "",
			wantLFS:  false,
		},
		{
			input:    "/my-org/project.php.git/info/refs",
			wantRepo: "my-org/project.php.git",
			wantSub:  "info/refs",
			wantLFS:  false,
		},
		{
			input:    "/team/HEAD.git/info/refs",
			wantRepo: "team/HEAD.git",
			wantSub:  "info/refs",
			wantLFS:  false,
		},
		{
			input:    "/HEAD-repo.git/git-upload-pack",
			wantRepo: "HEAD-repo.git",
			wantSub:  "git-upload-pack",
			wantLFS:  false,
		},
		{
			input:    "/HEAD-repo.git/HEAD",
			wantRepo: "HEAD-repo.git",
			wantSub:  "HEAD",
			wantLFS:  false,
		},
		{
			input:    "/team/HEAD.git/info/lfs/objects/batch",
			wantRepo: "team/HEAD.git",
			wantSub:  "info/lfs/objects/batch",
			wantLFS:  true,
		},
		{
			input:   "/.hidden/repo.git",
			wantErr: true,
		},
		{
			input:   "/info/refs", // missing repo
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := ParseURLPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseURLPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil {
				if res.RepoPath != tt.wantRepo {
					t.Errorf("RepoPath = %q, want %q", res.RepoPath, tt.wantRepo)
				}
				if res.SubPath != tt.wantSub {
					t.Errorf("SubPath = %q, want %q", res.SubPath, tt.wantSub)
				}
				if res.IsLFS != tt.wantLFS {
					t.Errorf("IsLFS = %v, want %v", res.IsLFS, tt.wantLFS)
				}
			}
		})
	}
}
