package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type tokenInfo struct {
	username  string
	issuedAt  time.Time
	expiresAt time.Time
	lastSeen  time.Time
}

// Manager issues and validates short-lived auth tokens.
type Manager struct {
	mu     sync.RWMutex
	ttl    time.Duration
	tokens map[string]tokenInfo
}

// NewManager creates a new token manager with the provided TTL.
func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Manager{
		ttl:    ttl,
		tokens: make(map[string]tokenInfo),
	}
}

// Issue creates a new token for the given username.
func (m *Manager) Issue(username string) (string, time.Time, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := time.Now()
	expiresAt := now.Add(m.ttl)

	m.mu.Lock()
	m.tokens[token] = tokenInfo{
		username:  username,
		issuedAt:  now,
		expiresAt: expiresAt,
		lastSeen:  now,
	}
	m.mu.Unlock()

	return token, expiresAt, nil
}

// Validate checks whether a token is valid and returns the username.
func (m *Manager) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.tokens[token]
	if !ok {
		return "", false
	}
	if now.After(info.expiresAt) {
		delete(m.tokens, token)
		return "", false
	}

	info.lastSeen = now
	m.tokens[token] = info
	return info.username, true
}

// Revoke invalidates a token.
func (m *Manager) Revoke(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.tokens, token)
	m.mu.Unlock()
}
