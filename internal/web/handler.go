package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/repo"
)

//go:embed dist/*
var distFS embed.FS

const sessionCookieName = "gitina_session"

type Handler struct {
	cfg        *config.Config
	aclMgr     *acl.Manager
	tokenMgr   *auth.TokenManager
	repoMgr    *repo.Manager
	distSubFS  fs.FS
	fileServer http.Handler
}

func NewHandler(cfg *config.Config, aclMgr *acl.Manager, tokenMgr *auth.TokenManager, repoMgr *repo.Manager) (*Handler, error) {
	subFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}

	return &Handler{
		cfg:        cfg,
		aclMgr:     aclMgr,
		tokenMgr:   tokenMgr,
		repoMgr:    repoMgr,
		distSubFS:  subFS,
		fileServer: http.FileServer(http.FS(subFS)),
	}, nil
}

// CurrentUser extracts the logged in username from session cookie.
func (h *Handler) CurrentUser(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	user, err := h.tokenMgr.ValidateAccessToken(cookie.Value)
	if err != nil {
		return ""
	}
	return user
}

// SetUserSession sets the auth session cookie.
func (h *Handler) SetUserSession(w http.ResponseWriter, username string) {
	token, _, err := h.tokenMgr.GenerateAccessToken(username)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   90 * 24 * 3600, // 90 days
	})
}

// ClearUserSession deletes the auth session cookie.
func (h *Handler) ClearUserSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// ServeHome handles the root dashboard URL, serving the SPA index.html.
func (h *Handler) ServeHome(w http.ResponseWriter, r *http.Request) {
	h.serveSPAIndex(w, r)
}

// ServeHTTP handles all Web UI SPA requests and static asset delivery.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Try dispatching to Web API if under /api/v1/
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		if h.ServeAPI(w, r) {
			return
		}
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/")

	// 2. If requesting a static asset that exists in distFS (e.g. assets/*, logo.svg)
	if reqPath != "" && reqPath != "index.html" {
		if f, err := h.distSubFS.Open(reqPath); err == nil {
			_ = f.Close()
			if strings.HasPrefix(reqPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			h.fileServer.ServeHTTP(w, r)
			return
		}
	}

	// 3. Fallback: serve index.html for Vue Router (HTML5 History Mode)
	h.serveSPAIndex(w, r)
}

func (h *Handler) serveSPAIndex(w http.ResponseWriter, r *http.Request) {
	indexFile, err := h.distSubFS.Open("index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Gitina</title></head>
<body style="font-family: sans-serif; padding: 2rem; background: #0d1117; color: #c9d1d9;">
  <h2>Gitina Web UI</h2>
  <p>Frontend assets are not yet built. Please run <code>make build</code> or <code>cd web && npm run build</code>.</p>
</body>
</html>`))
		return
	}
	defer indexFile.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = io.Copy(w, indexFile)
}
