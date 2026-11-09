package git

import (
	"fmt"
	"net/http"
	"net/http/cgi"
	"os/exec"
	"path/filepath"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git/tiered"
	"gitina/internal/lfs"
)

type Handler struct {
	cfg        *config.Config
	aclMgr     *acl.Manager
	gcmHandler *auth.GCMHandler
	lfsHandler *lfs.Handler
	tieredMgr  *tiered.Manager
	gitBackend string
}

func NewHandler(cfg *config.Config, aclMgr *acl.Manager, gcmHandler *auth.GCMHandler, lfsHandler *lfs.Handler, tieredMgr *tiered.Manager) (*Handler, error) {
	// Verify git executable is available in PATH
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git binary not found in PATH: %w", err)
	}

	// Locate git-http-backend
	// git --exec-path output + /git-http-backend
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git exec path: %w", err)
	}
	execPath := strings.TrimSpace(string(out))
	gitBackend := filepath.Join(execPath, "git-http-backend")

	return &Handler{
		cfg:        cfg,
		aclMgr:     aclMgr,
		gcmHandler: gcmHandler,
		lfsHandler: lfsHandler,
		tieredMgr:  tieredMgr,
		gitBackend: gitBackend,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse URL path to extract repository and subpath
	parsed, err := ParseURLPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// 1. Dispatch LFS endpoints if requested
	if parsed.IsLFS && h.lfsHandler != nil {
		if parsed.SubPath == "info/lfs/objects/batch" {
			h.lfsHandler.HandleBatch(w, r, parsed.RepoPath)
			return
		}
		if parsed.SubPath == "info/lfs/objects/verify" {
			h.lfsHandler.HandleVerify(w, r, parsed.RepoPath)
			return
		}
		if strings.HasPrefix(parsed.SubPath, "info/lfs/objects/") {
			h.lfsHandler.HandleObject(w, r, parsed.RepoPath, parsed.SubPath)
			return
		}
		http.NotFound(w, r)
		return
	}

	// 2. Determine required Git permission
	service := r.URL.Query().Get("service")
	isWrite := false

	if service == "git-receive-pack" || strings.HasSuffix(parsed.SubPath, "git-receive-pack") {
		isWrite = true
	} else if service == "git-upload-pack" || strings.HasSuffix(parsed.SubPath, "git-upload-pack") || parsed.SubPath == "HEAD" {
		isWrite = false
	} else if parsed.SubPath == "info/refs" {
		// When no service is passed (dumb http or probe), treat as read
		isWrite = false
	}

	// Extract authenticated username
	username := h.gcmHandler.ExtractUsername(r)

	// Check access
	if isWrite {
		if !h.aclMgr.CanWrite(username, parsed.RepoPath) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: write access denied", http.StatusForbidden)
			}
			return
		}
	} else {
		if !h.aclMgr.CanRead(username, parsed.RepoPath) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: read access denied", http.StatusForbidden)
			}
			return
		}
	}

	// 3. Acquire repository (tiered storage restore / local lock)
	if h.tieredMgr != nil {
		release, err := h.tieredMgr.Acquire(r.Context(), parsed.RepoPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to acquire repository: %v", err), http.StatusInternalServerError)
			return
		}
		defer release()
	} else {
		// Fallback: Ensure bare repository exists locally
		fullRepoPath := filepath.Join(h.cfg.Server.RepoRoot, parsed.RepoPath)
		if err := EnsureRepo(fullRepoPath); err != nil {
			http.Error(w, fmt.Sprintf("failed to initialize repository: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// 4. Delegate to git-http-backend using net/http/cgi
	cgiHandler := &cgi.Handler{
		Path: h.gitBackend,
		Dir:  h.cfg.Server.RepoRoot,
		Env: []string{
			"GIT_PROJECT_ROOT=" + h.cfg.Server.RepoRoot,
			"GIT_HTTP_EXPORT_ALL=1",
			"PATH_INFO=/" + parsed.RepoPath + "/" + parsed.SubPath,
			"REMOTE_USER=" + username,
		},
	}

	cgiHandler.ServeHTTP(w, r)

	// 5. If it was a push (write), sync updated bundle to S3
	if isWrite && h.tieredMgr != nil {
		_ = h.tieredMgr.Sync(r.Context(), parsed.RepoPath)
	}
}
