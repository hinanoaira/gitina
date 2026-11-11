package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gitina/internal/auth"
	"gitina/internal/git"
	"gitina/internal/git/tiered"
)

// JSON helper response functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// HandleAuthMe returns the current authenticated user info.
func (h *Handler) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	user := h.CurrentUser(r)
	isAdmin := h.aclMgr.IsAdmin(user)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":     user,
		"is_admin": isAdmin,
		"base_url": h.cfg.Server.BaseURL,
	})
}

// HandleAPIRepos returns the list of repositories accessible to the current user.
func (h *Handler) HandleAPIRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user := h.CurrentUser(r)
	isAdmin := h.aclMgr.IsAdmin(user)

	allRepos, err := h.repoMgr.List(r.Context())
	if err != nil {
		allRepos = nil
	}

	type repoItem struct {
		Path      string    `json:"path"`
		DiskSize  int64     `json:"disk_size"`
		UpdatedAt time.Time `json:"updated_at"`
		IsCached  bool      `json:"is_cached"`
		Storage   string    `json:"storage"`
		IsPublic  bool      `json:"is_public"`
		CanRead   bool      `json:"can_read"`
		CanWrite  bool      `json:"can_write"`
	}

	var visibleRepos []repoItem
	for _, rp := range allRepos {
		canRead := h.aclMgr.CanRead(user, rp.Path)
		if canRead || isAdmin {
			isPublic, _ := h.aclMgr.GetEffectiveConfig(rp.Path)
			canWrite := h.aclMgr.CanWrite(user, rp.Path)
			visibleRepos = append(visibleRepos, repoItem{
				Path:      rp.Path,
				DiskSize:  rp.DiskSize,
				UpdatedAt: rp.UpdatedAt,
				IsCached:  rp.IsCached,
				Storage:   rp.Storage,
				IsPublic:  isPublic,
				CanRead:   canRead,
				CanWrite:  canWrite,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repos": visibleRepos,
	})
}

// acquireRepoHelper verifies access and acquires repository from storage if tiered.
func (h *Handler) acquireRepoHelper(r *http.Request, repoPath string) (func(), error) {
	if h.tieredMgr != nil {
		release, err := h.tieredMgr.Acquire(r.Context(), repoPath, false)
		if err != nil {
			return nil, err
		}
		return release, nil
	}

	// Local storage fallback: verify bare repo exists
	fullRepoPath := path.Join(h.cfg.Server.RepoRoot, repoPath)
	if _, err := os.Stat(filepath.Join(fullRepoPath, "HEAD")); err != nil {
		return nil, tiered.ErrRepoNotFound
	}
	return func() {}, nil
}

// ServeAPI handles incoming /api/v1/... requests for the Web UI.
func (h *Handler) ServeAPI(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path

	if p == "/api/v1/auth/me" {
		h.HandleAuthMe(w, r)
		return true
	}

	if p == "/api/v1/repos" || p == "/api/v1/repos/" {
		h.HandleAPIRepos(w, r)
		return true
	}

	if strings.HasPrefix(p, "/api/v1/repos/") {
		rest := strings.TrimPrefix(p, "/api/v1/repos/")
		// Find where the repoPath ends and subAction starts.
		// repoPath must end with .git or be the repository identifier.
		// e.g. "path/to/repo.git/tree" -> repoPath="path/to/repo.git", subAction="tree"
		// or "my-repo.git" -> repoPath="my-repo.git", subAction=""
		var repoPath, subAction string
		if idx := strings.Index(rest, ".git/"); idx != -1 {
			repoPath = rest[:idx+4]
			subAction = strings.Trim(rest[idx+5:], "/")
		} else if strings.HasSuffix(rest, ".git") {
			repoPath = rest
			subAction = ""
		} else {
			// Subaction without trailing slash or repo without .git
			// Try splitting at common action prefixes: /tree, /blob, /commits, /tokens, /settings
			actions := []string{"/tree", "/blob", "/commits", "/tokens", "/settings/visibility", "/settings/collaborators", "/settings"}
			found := false
			for _, act := range actions {
				if idx := strings.Index(rest, act); idx != -1 {
					repoPart := rest[:idx]
					if !strings.HasSuffix(repoPart, ".git") {
						repoPart += ".git"
					}
					repoPath = repoPart
					subAction = strings.Trim(rest[idx+1:], "/")
					found = true
					break
				}
			}
			if !found {
				repoPath = rest
				if !strings.HasSuffix(repoPath, ".git") {
					repoPath += ".git"
				}
				subAction = ""
			}
		}

		h.HandleAPIRepoDetail(w, r, repoPath, subAction)
		return true
	}

	return false
}
// URL pattern: /api/v1/repos/{repoPath...}
func (h *Handler) HandleAPIRepoDetail(w http.ResponseWriter, r *http.Request, repoPath string, subAction string) {
	user := h.CurrentUser(r)

	// 1. Check Read permission
	canRead := h.aclMgr.CanRead(user, repoPath)
	canWrite := h.aclMgr.CanWrite(user, repoPath)
	isAdmin := h.aclMgr.IsAdmin(user)
	isPublic, collaborators := h.aclMgr.GetEffectiveConfig(repoPath)

	if !canRead {
		if user == "" {
			writeError(w, http.StatusUnauthorized, "Authentication required")
		} else {
			writeError(w, http.StatusForbidden, "Forbidden: No read access")
		}
		return
	}

	// 2. Check existence / acquire
	release, err := h.acquireRepoHelper(r, repoPath)
	if err != nil {
		if errors.Is(err, tiered.ErrRepoNotFound) {
			writeError(w, http.StatusNotFound, "Repository not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load repository: "+err.Error())
		return
	}
	defer release()

	fullRepoPath := path.Join(h.cfg.Server.RepoRoot, repoPath)
	reader := git.NewRepoReader(fullRepoPath)

	// Sub-actions routing
	switch subAction {
	case "", "info":
		if r.Method == http.MethodDelete {
			// Delete repo
			if !isAdmin {
				writeError(w, http.StatusForbidden, "Admin permission required")
				return
			}
			var req struct {
				ConfirmName string `json:"confirm_name"`
				Force       bool   `json:"force"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.ConfirmName != repoPath && req.ConfirmName != strings.TrimSuffix(repoPath, ".git") {
				writeError(w, http.StatusBadRequest, "Confirmation name mismatch")
				return
			}
			_, err := h.repoMgr.Remove(r.Context(), repoPath, req.Force)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to remove repo: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}

		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		branches, _ := reader.ListBranches()
		defaultBranch := reader.GetDefaultBranch()
		cloneURL := fmt.Sprintf("%s/%s", h.cfg.Server.BaseURL, repoPath)

		resp := map[string]interface{}{
			"path":           repoPath,
			"is_public":      isPublic,
			"can_read":       canRead,
			"can_write":      canWrite,
			"is_admin":       isAdmin,
			"default_branch": defaultBranch,
			"branches":       branches,
			"clone_url":      cloneURL,
		}
		if isAdmin {
			resp["collaborators"] = collaborators
		}
		writeJSON(w, http.StatusOK, resp)

	case "tree":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = reader.GetDefaultBranch()
		}
		subPath := r.URL.Query().Get("path")
		entries, err := reader.ListTree(ref, subPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Failed to list tree: "+err.Error())
			return
		}

		var readme *git.BlobContent
		if subPath == "" {
			for _, e := range entries {
				if strings.EqualFold(e.Name, "README.md") || strings.EqualFold(e.Name, "README") {
					readme, _ = reader.GetBlob(ref, e.Name)
					break
				}
			}
		}

		latestCommit, _ := reader.GetCommit(ref)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ref":           ref,
			"path":          subPath,
			"entries":       entries,
			"readme":        readme,
			"latest_commit": latestCommit,
		})

	case "blob":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = reader.GetDefaultBranch()
		}
		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			writeError(w, http.StatusBadRequest, "Missing path parameter")
			return
		}

		blob, err := reader.GetBlob(ref, filePath)
		if err != nil {
			writeError(w, http.StatusNotFound, "Blob not found: "+err.Error())
			return
		}

		latestCommit, _ := reader.GetCommit(ref)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ref":           ref,
			"blob":          blob,
			"latest_commit": latestCommit,
		})

	case "commits":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = reader.GetDefaultBranch()
		}
		commits, err := reader.ListCommits(ref, 50)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Failed to list commits: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ref":     ref,
			"commits": commits,
		})

	case "tokens":
		if user == "" {
			writeError(w, http.StatusUnauthorized, "Authentication required to manage tokens")
			return
		}

		patStore := h.tokenMgr.PATStore()
		if patStore == nil {
			writeError(w, http.StatusInternalServerError, "PAT store is not configured")
			return
		}

		switch r.Method {
		case http.MethodGet:
			tokens := patStore.List(user, repoPath, isAdmin)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"tokens": tokens,
			})

		case http.MethodPost:
			var req struct {
				Name  string `json:"name"`
				Days  string `json:"days"`
				Scope string `json:"scope"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			scope := req.Scope
			if !canWrite && scope != "ro" {
				scope = "ro"
			}
			if scope != "ro" && scope != "rw" {
				scope = "ro"
			}

			var duration time.Duration
			switch req.Days {
			case "30":
				duration = 30 * 24 * time.Hour
			case "90":
				duration = 90 * 24 * time.Hour
			case "365":
				duration = 365 * 24 * time.Hour
			default:
				duration = 0 // unlimited
			}

			pat, rawSecret, err := patStore.Create(user, repoPath, req.Name, scope, duration)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to create token: "+err.Error())
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":         pat.ID,
				"token":      rawSecret,
				"name":       pat.Name,
				"scope":      pat.Scope,
				"created_at": pat.CreatedAt,
				"expires_at": pat.ExpiresAt,
			})

		case http.MethodDelete:
			targetID := r.URL.Query().Get("id")
			if targetID == "" {
				var req struct {
					ID string `json:"id"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				targetID = req.ID
			}

			if targetID == "" {
				writeError(w, http.StatusBadRequest, "Missing token id")
				return
			}

			if err := patStore.Revoke(targetID, user, isAdmin); err != nil {
				if errors.Is(err, auth.ErrPATNotFound) {
					writeError(w, http.StatusNotFound, "Token not found")
					return
				}
				writeError(w, http.StatusForbidden, err.Error())
				return
			}

			writeJSON(w, http.StatusOK, map[string]string{
				"status": "revoked",
			})

		default:
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}

	case "settings/visibility":
		if !isAdmin {
			writeError(w, http.StatusForbidden, "Admin permission required")
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var req struct {
			IsPublic bool `json:"is_public"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		_ = h.aclMgr.UpdateMetadata(repoPath, req.IsPublic, collaborators, user)
		if h.tieredMgr != nil {
			go h.tieredMgr.Sync(r.Context(), repoPath)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"is_public": req.IsPublic,
		})

	case "settings/collaborators":
		if !isAdmin {
			writeError(w, http.StatusForbidden, "Admin permission required")
			return
		}
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Invalid JSON payload")
				return
			}
			collabUser := strings.TrimSpace(req.Username)
			collabRole := strings.TrimSpace(req.Role)
			if collabUser != "" && (collabRole == "ro" || collabRole == "rw") {
				collaborators[collabUser] = collabRole
				_ = h.aclMgr.UpdateMetadata(repoPath, isPublic, collaborators, user)
				if h.tieredMgr != nil {
					go h.tieredMgr.Sync(r.Context(), repoPath)
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"collaborators": collaborators,
				})
				return
			}
			writeError(w, http.StatusBadRequest, "Invalid username or role (must be ro or rw)")

		case http.MethodDelete:
			var req struct {
				Username string `json:"username"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Invalid JSON payload")
				return
			}
			collabUser := strings.TrimSpace(req.Username)
			if collabUser != "" {
				delete(collaborators, collabUser)
				_ = h.aclMgr.UpdateMetadata(repoPath, isPublic, collaborators, user)
				if h.tieredMgr != nil {
					go h.tieredMgr.Sync(r.Context(), repoPath)
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"collaborators": collaborators,
				})
				return
			}
			writeError(w, http.StatusBadRequest, "Invalid username")

		default:
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}

	default:
		writeError(w, http.StatusNotFound, "Unknown API sub-action")
	}
}
