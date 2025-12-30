package auth

import (
	"testing"
	"time"
)

func TestManagerIssueAndValidate(t *testing.T) {
	manager := NewManager(50 * time.Millisecond)

	token, expiresAt, err := manager.Issue("admin")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if token == "" {
		t.Fatal("expected token")
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatal("expected future expiry")
	}

	user, ok := manager.Validate(token)
	if !ok {
		t.Fatal("expected token to be valid")
	}
	if user != "admin" {
		t.Fatalf("expected user admin, got %s", user)
	}
}

func TestManagerExpiresToken(t *testing.T) {
	manager := NewManager(10 * time.Millisecond)

	token, _, err := manager.Issue("admin")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	time.Sleep(15 * time.Millisecond)
	if _, ok := manager.Validate(token); ok {
		t.Fatal("expected token to expire")
	}
}

func TestManagerRevokeToken(t *testing.T) {
	manager := NewManager(time.Minute)

	token, _, err := manager.Issue("admin")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	manager.Revoke(token)
	if _, ok := manager.Validate(token); ok {
		t.Fatal("expected token to be revoked")
	}
}
