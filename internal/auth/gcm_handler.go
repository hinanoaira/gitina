package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gitina/internal/acl"
	"gitina/internal/config"
)

const (
	GiteaUniversalClientID = "e90ee53c-94e2-48ac-9358-a874fb9e0662"
)

type OAuthSession struct {
	GCMRedirectURI string
	GCMState       string
	CodeChallenge  string
	CreatedAt      time.Time
}

type AuthCodeData struct {
	Username  string
	CreatedAt time.Time
}

type GCMHandler struct {
	cfg          *config.Config
	tokenMgr     *TokenManager
	aclMgr       *acl.Manager
	sessionsMu   sync.Mutex
	sessions     map[string]*OAuthSession // key: random state sent to GitHub
	authCodesMu  sync.Mutex
	authCodes    map[string]*AuthCodeData // key: one-time auth code issued to GCM
}

func NewGCMHandler(cfg *config.Config, tokenMgr *TokenManager, aclMgr *acl.Manager) *GCMHandler {
	h := &GCMHandler{
		cfg:         cfg,
		tokenMgr:    tokenMgr,
		aclMgr:      aclMgr,
		sessions:    make(map[string]*OAuthSession),
		authCodes:   make(map[string]*AuthCodeData),
	}

	// Periodic cleanup of expired sessions and auth codes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			h.cleanup()
		}
	}()

	return h
}

func (h *GCMHandler) cleanup() {
	now := time.Now()
	h.sessionsMu.Lock()
	for k, s := range h.sessions {
		if now.Sub(s.CreatedAt) > 10*time.Minute {
			delete(h.sessions, k)
		}
	}
	h.sessionsMu.Unlock()

	h.authCodesMu.Lock()
	for k, c := range h.authCodes {
		if now.Sub(c.CreatedAt) > 5*time.Minute {
			delete(h.authCodes, k)
		}
	}
	h.authCodesMu.Unlock()
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HandleAuthorize handles GET /login/oauth/authorize
func (h *GCMHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := q.Get("redirect_uri")
	gcmState := q.Get("state")
	codeChallenge := q.Get("code_challenge")

	if redirectURI == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}

	// Generate internal state for GitHub OAuth
	internalState := randomString(16)
	h.sessionsMu.Lock()
	h.sessions[internalState] = &OAuthSession{
		GCMRedirectURI: redirectURI,
		GCMState:       gcmState,
		CodeChallenge:  codeChallenge,
		CreatedAt:      time.Now(),
	}
	h.sessionsMu.Unlock()

	// Redirect to GitHub OAuth
	githubCallbackURL := h.cfg.Server.BaseURL + "/login/oauth/github/callback"
	githubAuthURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=read:user&state=%s",
		url.QueryEscape(h.cfg.GitHub.ClientID),
		url.QueryEscape(githubCallbackURL),
		url.QueryEscape(internalState),
	)

	http.Redirect(w, r, githubAuthURL, http.StatusFound)
}

// HandleGitHubCallback handles GET /login/oauth/github/callback
func (h *GCMHandler) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	githubCode := q.Get("code")
	internalState := q.Get("state")

	h.sessionsMu.Lock()
	sess, ok := h.sessions[internalState]
	if ok {
		delete(h.sessions, internalState)
	}
	h.sessionsMu.Unlock()

	if !ok || sess == nil {
		http.Error(w, "invalid or expired oauth state session", http.StatusBadRequest)
		return
	}

	// Exchange GitHub code for GitHub access token
	githubUser, err := h.fetchGitHubUser(githubCode)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to authenticate with GitHub: %v", err), http.StatusUnauthorized)
		return
	}

	// If the redirect URI is a web path (starts with "/" or matches BaseURL), set web session cookie
	if strings.HasPrefix(sess.GCMRedirectURI, "/") || strings.HasPrefix(sess.GCMRedirectURI, h.cfg.Server.BaseURL) {
		token, _, err := h.tokenMgr.GenerateAccessToken(githubUser)
		if err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "gitina_session",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   90 * 24 * 3600,
			})
		}
		dest := sess.GCMRedirectURI
		if dest == "" {
			dest = "/"
		}
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}

	// Create one-time auth code for GCM
	authCode := "code_" + randomString(24)
	h.authCodesMu.Lock()
	h.authCodes[authCode] = &AuthCodeData{
		Username:  githubUser,
		CreatedAt: time.Now(),
	}
	h.authCodesMu.Unlock()

	// Redirect back to GCM's local listener
	redirectURL, err := url.Parse(sess.GCMRedirectURI)
	if err != nil {
		http.Error(w, "invalid gcm redirect_uri", http.StatusBadRequest)
		return
	}

	rq := redirectURL.Query()
	rq.Set("code", authCode)
	if sess.GCMState != "" {
		rq.Set("state", sess.GCMState)
	}
	redirectURL.RawQuery = rq.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (h *GCMHandler) fetchGitHubUser(code string) (string, error) {
	// 1. Exchange code for access_token
	tokenURL := "https://github.com/login/oauth/access_token"
	data := url.Values{
		"client_id":     {h.cfg.GitHub.ClientID},
		"client_secret": {h.cfg.GitHub.ClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return "", err
	}
	if tokenRes.Error != "" {
		return "", fmt.Errorf("%s: %s", tokenRes.Error, tokenRes.ErrorDesc)
	}
	if tokenRes.AccessToken == "" {
		return "", errors.New("no access_token returned by GitHub")
	}

	// 2. Fetch GitHub user profile
	userReq, err := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
	userReq.Header.Set("Accept", "application/vnd.github.v3+json")

	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		return "", err
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(userResp.Body)
		return "", fmt.Errorf("GitHub /user API returned %d: %s", userResp.StatusCode, string(b))
	}

	var userRes struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&userRes); err != nil {
		return "", err
	}

	if userRes.Login == "" {
		return "", errors.New("empty github username")
	}

	return userRes.Login, nil
}

// HandleToken handles POST /login/oauth/access_token
func (h *GCMHandler) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	h.authCodesMu.Lock()
	codeData, ok := h.authCodes[code]
	if ok {
		delete(h.authCodes, code)
	}
	h.authCodesMu.Unlock()

	if !ok || codeData == nil || time.Since(codeData.CreatedAt) > 5*time.Minute {
		http.Error(w, "invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	tokenStr, expiresIn, err := h.tokenMgr.GenerateAccessToken(codeData.Username)
	if err != nil {
		http.Error(w, "failed to generate access token", http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"access_token": tokenStr,
		"token_type":   "bearer",
		"expires_in":   expiresIn,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// HandleUser handles GET /api/v1/user
func (h *GCMHandler) HandleUser(w http.ResponseWriter, r *http.Request) {
	username := h.ExtractUsername(r)
	if username == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="Gitea"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	res := map[string]interface{}{
		"id":       1,
		"login":    username,
		"username": username,
		"email":    username + "@users.noreply.github.com",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// HandleVersion handles GET /api/v1/version
func (h *GCMHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"version": "1.22.0",
	})
}

// HandleLogout handles GET /login/oauth/logout
func (h *GCMHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gitina_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// ExtractUsername parses the username from HTTP Authorization header (Bearer or Basic).
func (h *GCMHandler) ExtractUsername(r *http.Request) string {
	u, _ := h.AuthenticateRequest(r, "", false)
	return u
}

// AuthenticateRequest verifies Authorization header and checks PAT scope/repo permissions.
func (h *GCMHandler) AuthenticateRequest(r *http.Request, repoPath string, isWrite bool) (username string, tokenAllowed bool) {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		u, allowed, err := h.tokenMgr.ValidatePermission(tokenStr, repoPath, isWrite)
		if err == nil {
			return u, allowed
		}
	} else if u, p, ok := r.BasicAuth(); ok {
		// 1. Check PATStore if ID and Secret (Basic Auth: ID:Secret)
		if patStore := h.tokenMgr.PATStore(); patStore != nil {
			if uname, ok := patStore.Validate(u, p, repoPath, isWrite); uname != "" {
				return uname, ok
			}
		}

		// 2. Fallback to JWT in password or username
		if _, err := h.tokenMgr.ValidateAccessToken(p); err == nil {
			uClaim, allowed, err := h.tokenMgr.ValidatePermission(p, repoPath, isWrite)
			if err == nil {
				return uClaim, allowed
			}
		} else if _, err := h.tokenMgr.ValidateAccessToken(u); err == nil {
			uClaim, allowed, err := h.tokenMgr.ValidatePermission(u, repoPath, isWrite)
			if err == nil {
				return uClaim, allowed
			}
		}
	}

	return "", false
}

// TokenManager returns the underlying TokenManager.
func (h *GCMHandler) TokenManager() *TokenManager {
	return h.tokenMgr
}
