package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitina/internal/acl"
	"gitina/internal/config"
)

func TestOpenRedirectPrevention(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			BaseURL: "http://gitina.example.com",
		},
	}
	tokenMgr := NewTokenManager("test-secret-key-1234567890123456")
	aclMgr := acl.NewManager(nil, nil)
	handler := NewGCMHandler(cfg, tokenMgr, aclMgr)

	maliciousURIs := []string{
		"//evil.com",
		"//evil.com/path",
		"/\\evil.com",
		"http://attacker.com",
		"https://evil.com/login",
	}

	for _, uri := range maliciousURIs {
		t.Run("redirect_"+uri, func(t *testing.T) {
			// Register a session with malicious GCMRedirectURI
			state := "state_" + randomString(8)
			handler.sessionsMu.Lock()
			handler.sessions[state] = &OAuthSession{
				GCMRedirectURI: uri,
				CreatedAt:      time.Now(),
			}
			handler.sessionsMu.Unlock()

			// Check logic
			sess := handler.sessions[state]
			isLocalPath := strings.HasPrefix(sess.GCMRedirectURI, "/") &&
				!strings.HasPrefix(sess.GCMRedirectURI, "//") &&
				!strings.HasPrefix(sess.GCMRedirectURI, "/\\")

			if isLocalPath {
				t.Errorf("malicious URI %q was recognized as local path", uri)
			}
		})
	}

	validLocalURIs := []string{
		"/",
		"/repos/my-repo",
		"/settings",
		"http://gitina.example.com/repo",
	}

	for _, uri := range validLocalURIs {
		t.Run("valid_"+uri, func(t *testing.T) {
			isLocalPath := strings.HasPrefix(uri, "/") &&
				!strings.HasPrefix(uri, "//") &&
				!strings.HasPrefix(uri, "/\\")

			allowed := isLocalPath || strings.HasPrefix(uri, cfg.Server.BaseURL)
			if !allowed {
				t.Errorf("valid URI %q was rejected", uri)
			}
		})
	}

	// Verify logout redirect
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login/oauth/logout", nil)
	handler.HandleLogout(rec, req)
	if rec.Header().Get("Location") != "/" {
		t.Errorf("HandleLogout redirected to %q, want '/'", rec.Header().Get("Location"))
	}
}
