package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type authRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	bucket map[string]*authBucket
}

type authBucket struct {
	count int
	reset time.Time
}

func newAuthRateLimiter(limit int, window time.Duration) *authRateLimiter {
	if limit <= 0 || window <= 0 {
		return nil
	}
	return &authRateLimiter{
		limit:  limit,
		window: window,
		bucket: make(map[string]*authBucket),
	}
}

// NewAuthRateLimiter returns a rate limiter or nil when disabled.
func NewAuthRateLimiter(limit int, window time.Duration) *authRateLimiter {
	return newAuthRateLimiter(limit, window)
}

func (l *authRateLimiter) allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.bucket[key]
	if b == nil || now.After(b.reset) {
		b = &authBucket{count: 0, reset: now.Add(l.window)}
		l.bucket[key] = b
	}

	if b.count >= l.limit {
		return false, time.Until(b.reset)
	}

	b.count++
	return true, 0
}

func (l *authRateLimiter) reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.bucket, key)
	l.mu.Unlock()
}

func clientKey(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
