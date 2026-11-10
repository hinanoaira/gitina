package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
)

type Claims struct {
	Username string `json:"username"`
	RepoPath string `json:"repo_path,omitempty"` // optional repo restriction
	Scope    string `json:"scope,omitempty"`     // "ro" or "rw"
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{
		secret: []byte(secret),
	}
}

// GenerateAccessToken generates a standard JWT access token valid for 90 days.
func (tm *TokenManager) GenerateAccessToken(username string) (string, int64, error) {
	expiresIn := int64(90 * 24 * 3600) // 90 days in seconds
	claims := Claims{
		Username: username,
		Scope:    "rw",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			Issuer:    "gitina",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiresIn) * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(tm.secret)
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenStr, expiresIn, nil
}

// GeneratePAT generates a personal access token for a specific repository and scope.
// If duration is 0, the token has no expiration (unlimited).
func (tm *TokenManager) GeneratePAT(username, repoPath, scope string, duration time.Duration) (string, error) {
	claims := Claims{
		Username: username,
		RepoPath: repoPath,
		Scope:    scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  username,
			Issuer:   "gitina-pat",
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	if duration > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(duration))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.secret)
}

// ValidateAccessToken parses and validates a JWT token string and returns the username.
func (tm *TokenManager) ValidateAccessToken(tokenStr string) (string, error) {
	claims, err := tm.ParseClaims(tokenStr)
	if err != nil {
		return "", err
	}
	return claims.Username, nil
}

// ParseClaims parses and verifies a JWT token string.
func (tm *TokenManager) ParseClaims(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return tm.secret, nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Username == "" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidatePermission checks if the token grants the required access (read or write) to targetRepo.
func (tm *TokenManager) ValidatePermission(tokenStr, targetRepo string, isWrite bool) (string, bool, error) {
	claims, err := tm.ParseClaims(tokenStr)
	if err != nil {
		return "", false, err
	}

	// Scope restriction
	if isWrite && claims.Scope == "ro" {
		return claims.Username, false, nil // PAT is read-only
	}

	// Repository restriction
	if claims.RepoPath != "" && claims.RepoPath != "*" {
		cleanClaimRepo := claims.RepoPath
		cleanTargetRepo := targetRepo
		if cleanClaimRepo != cleanTargetRepo && cleanClaimRepo+".git" != cleanTargetRepo && cleanClaimRepo != cleanTargetRepo+".git" {
			return claims.Username, false, nil // PAT restricted to another repository
		}
	}

	return claims.Username, true, nil
}
