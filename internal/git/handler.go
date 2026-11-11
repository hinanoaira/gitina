package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/lfs"
)

type Handler struct {
	cfg        *config.Config
	aclMgr     *acl.Manager
	gcmHandler *auth.GCMHandler
	lfsHandler *lfs.Handler
	gitBackend string
}

func NewHandler(cfg *config.Config, aclMgr *acl.Manager, gcmHandler *auth.GCMHandler, lfsHandler *lfs.Handler) (*Handler, error) {
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

	// Extract authenticated username and verify token permissions (PAT scope check)
	username, tokenAllowed := h.gcmHandler.AuthenticateRequest(r, parsed.RepoPath, isWrite)

	// Check access
	if isWrite {
		if !tokenAllowed || !h.aclMgr.CanWrite(username, parsed.RepoPath) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: write access denied", http.StatusForbidden)
			}
			return
		}
	} else {
		if !h.aclMgr.CanRead(username, parsed.RepoPath) || (username != "" && !tokenAllowed) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: read access denied", http.StatusForbidden)
			}
			return
		}
	}

	// 3. Ensure bare repository exists locally
	// Only auto-create bare repository if:
	// - This is a push operation (isWrite)
	// - OR an authenticated user who has write access to this repo is cloning/initializing it
	// Anonymous reads / scanners will NEVER trigger repository auto-creation.
	autoCreate := isWrite || (username != "" && h.aclMgr.CanWrite(username, parsed.RepoPath))

	fullRepoPath := filepath.Join(h.cfg.Server.RepoRoot, parsed.RepoPath)
	if _, err := os.Stat(filepath.Join(fullRepoPath, "HEAD")); err != nil {
		if !autoCreate {
			http.NotFound(w, r)
			return
		}
		if err := EnsureRepo(fullRepoPath); err != nil {
			http.Error(w, fmt.Sprintf("failed to initialize repository: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// 4. Delegate to git-http-backend directly via streaming pipes
	absRepoRoot, err := filepath.Abs(h.cfg.Server.RepoRoot)
	if err != nil {
		absRepoRoot = h.cfg.Server.RepoRoot
	}

	h.serveGitBackend(w, r, parsed.RepoPath, parsed.SubPath, username, absRepoRoot)
}

// serveGitBackend executes git-http-backend directly with streaming pipes (no spooling).
func (h *Handler) serveGitBackend(w http.ResponseWriter, r *http.Request, repoPath, subPath, username, absRepoRoot string) {
	cmd := exec.CommandContext(r.Context(), h.gitBackend)
	cmd.Dir = absRepoRoot

	pathInfo := "/" + repoPath + "/" + subPath

	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}

	env := []string{
		"GIT_PROJECT_ROOT=" + absRepoRoot,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO=" + pathInfo,
		"REMOTE_USER=" + username,
		"REMOTE_ADDR=" + remoteIP,
		"REQUEST_METHOD=" + r.Method,
		"QUERY_STRING=" + r.URL.RawQuery,
		"REQUEST_URI=" + r.URL.RequestURI(),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
		"PATH=" + os.Getenv("PATH"),
	}

	if r.ContentLength > 0 {
		env = append(env, fmt.Sprintf("CONTENT_LENGTH=%d", r.ContentLength))
	}
	if ctype := r.Header.Get("Content-Type"); ctype != "" {
		env = append(env, "CONTENT_TYPE="+ctype)
	}
	if proto := r.Header.Get("Git-Protocol"); proto != "" {
		env = append(env, "GIT_PROTOCOL="+proto, "HTTP_GIT_PROTOCOL="+proto)
	}

	for k, vv := range r.Header {
		upper := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		if upper == "CONTENT_TYPE" || upper == "CONTENT_LENGTH" {
			continue
		}
		for _, v := range vv {
			env = append(env, "HTTP_"+upper+"="+v)
		}
	}

	cmd.Env = env
	cmd.Stdin = r.Body

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "failed to create git backend pipe", http.StatusInternalServerError)
		return
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("failed to start git backend: %v", err), http.StatusInternalServerError)
		return
	}

	bufr := bufio.NewReader(stdoutPipe)
	tp := textproto.NewReader(bufr)
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		cmd.Wait()
		log.Printf("[GitBackend] Error reading headers from %s: %v (stderr: %s)", pathInfo, err, stderrBuf.String())
		http.Error(w, "internal git backend error", http.StatusInternalServerError)
		return
	}

	statusCode := http.StatusOK
	if status := mimeHeader.Get("Status"); status != "" {
		parts := strings.SplitN(status, " ", 2)
		if code, err := strconv.Atoi(parts[0]); err == nil {
			statusCode = code
		}
		mimeHeader.Del("Status")
	}

	for k, vv := range mimeHeader {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(statusCode)

	io.Copy(w, bufr)
	if err := cmd.Wait(); err != nil {
		if stderrBuf.Len() > 0 {
			log.Printf("[GitBackend] git-http-backend exited with error for %s: %v (stderr: %s)", pathInfo, err, stderrBuf.String())
		}
	}
}
