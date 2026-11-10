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

	// PAT tests
	roPAT, err := tm.GeneratePAT("alice", "org/repo.git", "ro", 0)
	if err != nil {
		t.Fatalf("GeneratePAT failed: %v", err)
	}

	// alice roPAT reading org/repo.git -> allowed
	u, ok, err := tm.ValidatePermission(roPAT, "org/repo.git", false)
	if err != nil || !ok || u != "alice" {
		t.Fatalf("expected roPAT read allowed, got u=%s, ok=%v, err=%v", u, ok, err)
	}

	// alice roPAT writing org/repo.git -> denied
	_, ok, err = tm.ValidatePermission(roPAT, "org/repo.git", true)
	if err != nil || ok {
		t.Fatalf("expected roPAT write denied, got ok=%v, err=%v", ok, err)
	}

	// alice roPAT accessing another repo -> denied
	_, ok, err = tm.ValidatePermission(roPAT, "other/repo.git", false)
	if err != nil || ok {
		t.Fatalf("expected roPAT access to other repo denied, got ok=%v, err=%v", ok, err)
	}
}
