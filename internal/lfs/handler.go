package lfs

import (
	"encoding/json"
	"net/http"
	"strings"

	"gitina/internal/acl"
	"gitina/internal/auth"
	"gitina/internal/config"
)

type Handler struct {
	cfg        *config.Config
	storage    StorageBackend
	aclMgr     *acl.Manager
	gcmHandler *auth.GCMHandler
}

func NewHandler(cfg *config.Config, storage StorageBackend, aclMgr *acl.Manager, gcmHandler *auth.GCMHandler) *Handler {
	return &Handler{
		cfg:        cfg,
		storage:    storage,
		aclMgr:     aclMgr,
		gcmHandler: gcmHandler,
	}
}

// HandleBatch handles POST /<repo>/info/lfs/objects/batch
func (h *Handler) HandleBatch(w http.ResponseWriter, r *http.Request, repoPath string) {
	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json request body", http.StatusBadRequest)
		return
	}

	isWrite := (req.Operation == "upload")
	username, tokenAllowed := h.gcmHandler.AuthenticateRequest(r, repoPath, isWrite)

	// ACL check
	if isWrite {
		if !tokenAllowed || !h.aclMgr.CanWrite(username, repoPath) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: write access required", http.StatusForbidden)
			}
			return
		}
	} else { // download
		if !h.aclMgr.CanRead(username, repoPath) || (username != "" && !tokenAllowed) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden: read access required", http.StatusForbidden)
			}
			return
		}
	}

	res := BatchResponse{
		Transfer: "basic",
		Objects:  make([]ObjectResponseItem, 0, len(req.Objects)),
	}

	ctx := r.Context()
	baseURL := h.cfg.Server.BaseURL

	for _, obj := range req.Objects {
		item := ObjectResponseItem{
			Oid:  obj.Oid,
			Size: obj.Size,
		}

		exists, err := h.storage.Exists(ctx, repoPath, obj.Oid, obj.Size)
		if err != nil {
			item.Error = &ObjectError{
				Code:    500,
				Message: err.Error(),
			}
			res.Objects = append(res.Objects, item)
			continue
		}

		if req.Operation == "upload" {
			if exists {
				// Object already exists: omit actions so client skips upload!
				res.Objects = append(res.Objects, item)
				continue
			}

			// Generate upload and verify actions
			uploadAction, verifyAction, err := h.storage.GetUploadAction(ctx, baseURL, repoPath, obj.Oid, obj.Size)
			if err != nil {
				item.Error = &ObjectError{Code: 500, Message: err.Error()}
			} else {
				item.Actions = map[string]*ActionLink{
					"upload": uploadAction,
				}
				if verifyAction != nil {
					item.Actions["verify"] = verifyAction
				}
			}
		} else { // download
			if !exists {
				item.Error = &ObjectError{
					Code:    404,
					Message: "Object does not exist",
				}
			} else {
				downloadAction, err := h.storage.GetDownloadAction(ctx, baseURL, repoPath, obj.Oid, obj.Size)
				if err != nil {
					item.Error = &ObjectError{Code: 500, Message: err.Error()}
				} else {
					item.Actions = map[string]*ActionLink{
						"download": downloadAction,
					}
				}
			}
		}

		res.Objects = append(res.Objects, item)
	}

	w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
	_ = json.NewEncoder(w).Encode(res)
}

// HandleVerify handles POST /<repo>/info/lfs/objects/verify
func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request, repoPath string) {
	username, tokenAllowed := h.gcmHandler.AuthenticateRequest(r, repoPath, true)
	if !tokenAllowed || !h.aclMgr.CanWrite(username, repoPath) {
		if username == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Error(w, "forbidden", http.StatusForbidden)
		}
		return
	}

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json request body", http.StatusBadRequest)
		return
	}

	if err := h.storage.VerifyObject(r.Context(), repoPath, req.Oid, req.Size); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleObject handles GET and PUT /<repo>/info/lfs/objects/<oid> (mainly for local storage)
func (h *Handler) HandleObject(w http.ResponseWriter, r *http.Request, repoPath, subPath string) {
	oid := strings.TrimPrefix(subPath, "info/lfs/objects/")

	if r.Method == http.MethodPut {
		username, tokenAllowed := h.gcmHandler.AuthenticateRequest(r, repoPath, true)
		if !tokenAllowed || !h.aclMgr.CanWrite(username, repoPath) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden", http.StatusForbidden)
			}
			return
		}
		h.storage.PutObject(w, r, repoPath, oid)
		return
	}

	if r.Method == http.MethodGet {
		username, tokenAllowed := h.gcmHandler.AuthenticateRequest(r, repoPath, false)
		if !h.aclMgr.CanRead(username, repoPath) || (username != "" && !tokenAllowed) {
			if username == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "forbidden", http.StatusForbidden)
			}
			return
		}
		h.storage.ServeObject(w, r, repoPath, oid)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
