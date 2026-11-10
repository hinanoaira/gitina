package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"
	"time"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
	"gitina/internal/git"
	"gitina/internal/git/tiered"
	"gitina/internal/repo"
)

//go:embed templates/*
var templateFS embed.FS

const sessionCookieName = "gitina_session"

type Handler struct {
	cfg       *config.Config
	aclMgr    *acl.Manager
	tokenMgr  *auth.TokenManager
	repoMgr   *repo.Manager
	tieredMgr *tiered.Manager
	templates *template.Template
}

func NewHandler(cfg *config.Config, aclMgr *acl.Manager, tokenMgr *auth.TokenManager, repoMgr *repo.Manager, tieredMgr *tiered.Manager) (*Handler, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02 15:04")
		},
		"formatSize": func(b int64) string {
			const unit = 1024
			if b < unit {
				return fmt.Sprintf("%d B", b)
			}
			div, exp := int64(unit), 0
			for n := b / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
		},
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse web templates: %w", err)
	}

	return &Handler{
		cfg:       cfg,
		aclMgr:    aclMgr,
		tokenMgr:  tokenMgr,
		repoMgr:   repoMgr,
		tieredMgr: tieredMgr,
		templates: tmpl,
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

// ServeHome renders the dashboard repository list.
func (h *Handler) ServeHome(w http.ResponseWriter, r *http.Request) {
	user := h.CurrentUser(r)
	isAdmin := h.aclMgr.IsAdmin(user)

	allRepos, err := h.repoMgr.List(r.Context())
	if err != nil {
		allRepos = nil
	}

	type displayRepo struct {
		Path      string
		DiskSize  int64
		UpdatedAt time.Time
		IsCached  bool
		Storage   string
		IsPublic  bool
	}

	var visibleRepos []displayRepo
	for _, r := range allRepos {
		canRead := h.aclMgr.CanRead(user, r.Path)
		if canRead || isAdmin {
			isPublic, _ := h.aclMgr.GetEffectiveConfig(r.Path)
			visibleRepos = append(visibleRepos, displayRepo{
				Path:      r.Path,
				DiskSize:  r.DiskSize,
				UpdatedAt: r.UpdatedAt,
				IsCached:  r.IsCached,
				Storage:   r.Storage,
				IsPublic:  isPublic,
			})
		}
	}

	data := map[string]interface{}{
		"CurrentUser": user,
		"IsAdmin":     isAdmin,
		"BaseURL":     h.cfg.Server.BaseURL,
		"Repos":       visibleRepos,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "home.html", data)
}

// ServeHTTP handles browser requests for repository viewing and management.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parsed, err := git.ParseURLPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := h.CurrentUser(r)
	repoPath := parsed.RepoPath

	// Ensure repository is available locally if tiered
	if h.tieredMgr != nil {
		release, err := h.tieredMgr.Acquire(r.Context(), repoPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to acquire repository: %v", err), http.StatusInternalServerError)
			return
		}
		defer release()
	}

	fullRepoPath := path.Join(h.cfg.Server.RepoRoot, repoPath)

	// Access control
	canRead := h.aclMgr.CanRead(user, repoPath)
	canWrite := h.aclMgr.CanWrite(user, repoPath)
	isAdmin := h.aclMgr.IsAdmin(user)
	isPublic, collaborators := h.aclMgr.GetEffectiveConfig(repoPath)

	if !canRead {
		if user == "" {
			// Redirect to GitHub login
			redirectURI := r.URL.RequestURI()
			http.Redirect(w, r, "/login/oauth/authorize?redirect_uri="+redirectURI, http.StatusFound)
		} else {
			http.Error(w, "403 Forbidden: You do not have permission to view this repository", http.StatusForbidden)
		}
		return
	}

	// Dispatch sub-actions
	sub := parsed.SubPath

	// Settings actions (Admin only)
	if strings.HasPrefix(sub, "settings/update-visibility") && r.Method == http.MethodPost {
		if !isAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		newPublic := r.FormValue("public") == "true"
		_ = h.aclMgr.UpdateMetadata(repoPath, newPublic, collaborators, user)
		if h.tieredMgr != nil {
			go h.tieredMgr.Sync(r.Context(), repoPath)
		}
		http.Redirect(w, r, "/"+repoPath+"?tab=settings", http.StatusFound)
		return
	}

	if strings.HasPrefix(sub, "settings/add-collaborator") && r.Method == http.MethodPost {
		if !isAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		collabUser := strings.TrimSpace(r.FormValue("username"))
		collabRole := strings.TrimSpace(r.FormValue("role"))
		if collabUser != "" && (collabRole == "ro" || collabRole == "rw") {
			collaborators[collabUser] = collabRole
			_ = h.aclMgr.UpdateMetadata(repoPath, isPublic, collaborators, user)
			if h.tieredMgr != nil {
				go h.tieredMgr.Sync(r.Context(), repoPath)
			}
		}
		http.Redirect(w, r, "/"+repoPath+"?tab=settings", http.StatusFound)
		return
	}

	if strings.HasPrefix(sub, "settings/delete-collaborator") && r.Method == http.MethodPost {
		if !isAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		collabUser := strings.TrimSpace(r.FormValue("username"))
		if collabUser != "" {
			delete(collaborators, collabUser)
			_ = h.aclMgr.UpdateMetadata(repoPath, isPublic, collaborators, user)
			if h.tieredMgr != nil {
				go h.tieredMgr.Sync(r.Context(), repoPath)
			}
		}
		http.Redirect(w, r, "/"+repoPath+"?tab=settings", http.StatusFound)
		return
	}

	if strings.HasPrefix(sub, "settings/delete-repo") && r.Method == http.MethodPost {
		if !isAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		confirmName := strings.TrimSpace(r.FormValue("confirm_name"))
		if confirmName != repoPath && confirmName != strings.TrimSuffix(repoPath, ".git") {
			http.Error(w, "Repository name confirmation does not match", http.StatusBadRequest)
			return
		}
		force := r.FormValue("force") == "true"
		_, err := h.repoMgr.Remove(r.Context(), repoPath, force)
		if err != nil {
			http.Error(w, "Failed to delete repository: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "code"
	}

	// PAT Generation action (RO, RW, Admin)
	var generatedToken string
	if strings.HasPrefix(sub, "tokens/generate") && r.Method == http.MethodPost {
		tab = "tokens"
		if user == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		scope := r.FormValue("scope")
		if !canWrite && scope != "ro" {
			scope = "ro" // RO users can only generate readonly tokens
		}
		if scope != "ro" && scope != "rw" {
			scope = "ro"
		}

		days := r.FormValue("days")
		var duration time.Duration
		switch days {
		case "30":
			duration = 30 * 24 * time.Hour
		case "90":
			duration = 90 * 24 * time.Hour
		case "365":
			duration = 365 * 24 * time.Hour
		default:
			duration = 0 // unlimited
		}

		tok, err := h.tokenMgr.GeneratePAT(user, repoPath, scope, duration)
		if err == nil {
			generatedToken = tok
		}
	}

	// Main repository reader
	reader := git.NewRepoReader(fullRepoPath)
	branches, _ := reader.ListBranches()
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = reader.GetDefaultBranch()
	}

	// Clone URL
	cloneURL := fmt.Sprintf("%s/%s", h.cfg.Server.BaseURL, repoPath)

	data := map[string]interface{}{
		"RepoPath":       repoPath,
		"CurrentUser":    user,
		"IsAdmin":        isAdmin,
		"CanWrite":       canWrite,
		"CanRead":        canRead,
		"IsPublic":       isPublic,
		"Collaborators":  collaborators,
		"CloneURL":       cloneURL,
		"Branches":       branches,
		"CurrentRef":     ref,
		"Tab":            tab,
		"GeneratedToken": generatedToken,
	}

	// Handle specific tabs / paths
	switch tab {
	case "commits":
		commits, _ := reader.ListCommits(ref, 50)
		data["Commits"] = commits
	case "settings":
		// Handled via data["Collaborators"], etc.
	default:
		// "code" tab: display tree or blob
		filePath := r.URL.Query().Get("path")
		if filePath != "" {
			// Check if file or directory
			blob, err := reader.GetBlob(ref, filePath)
			if err == nil {
				data["Blob"] = blob
				data["FilePath"] = filePath
			} else {
				// Might be a directory
				entries, _ := reader.ListTree(ref, filePath)
				data["TreeEntries"] = entries
				data["CurrentPath"] = filePath
			}
		} else {
			// Root tree
			entries, _ := reader.ListTree(ref, "")
			data["TreeEntries"] = entries
			data["CurrentPath"] = ""

			// Try to read README if present
			for _, e := range entries {
				if strings.EqualFold(e.Name, "README.md") || strings.EqualFold(e.Name, "README") {
					readmeBlob, _ := reader.GetBlob(ref, e.Name)
					data["Readme"] = readmeBlob
					break
				}
			}
		}
		// Also attach recent commit
		if commit, err := reader.GetCommit(ref); err == nil {
			data["LatestCommit"] = commit
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "repo.html", data); err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
	}
}
