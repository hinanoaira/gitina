package auth

import (
	"testing"
)

func TestJWT(t *testing.T) {
	secret := "test-secret-key-123456"
	tm := NewTokenManager(secret)

	token, exp, err := tm.GenerateAccessToken("hinanoaira")
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	if token == "" || exp <= 0 {
		t.Fatalf("invalid token output: token=%q, exp=%d", token, exp)
	}

	user, err := tm.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if user != "hinanoaira" {
		t.Fatalf("expected username hinanoaira, got %s", user)
	}

	// Tampered token test
	_, err = tm.ValidateAccessToken(token + "tampered")
	if err == nil {
		t.Fatalf("expected error for tampered token, got nil")
	}

	// Different secret test
	tmWrong := NewTokenManager("wrong-secret")
	_, err = tmWrong.ValidateAccessToken(token)
	if err == nil {
		t.Fatalf("expected error for token with different secret, got nil")
	}
}
