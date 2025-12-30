package middleware

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/auth"
)

// BasicAuthMiddleware provides basic authentication
type BasicAuthMiddleware struct {
	username            string
	password            string
	enabled             bool
	requireFunctionAuth bool
	rateLimiter         *authRateLimiter
	tokenManager        *auth.Manager
	logger              *logrus.Logger
}

// NewBasicAuthMiddleware creates a new basic auth middleware
func NewBasicAuthMiddleware(username, password string, enabled bool, requireFunctionAuth bool, rateLimiter *authRateLimiter, tokenManager *auth.Manager, logger *logrus.Logger) *BasicAuthMiddleware {
	return &BasicAuthMiddleware{
		username:            username,
		password:            password,
		enabled:             enabled,
		requireFunctionAuth: requireFunctionAuth,
		rateLimiter:         rateLimiter,
		tokenManager:        tokenManager,
		logger:              logger,
	}
}

// Middleware returns the middleware function
func (m *BasicAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if disabled
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for health check endpoint
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow login endpoint without auth
		if r.URL.Path == "/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow unauthenticated function invocation for OpenFaaS compatibility.
		if !m.requireFunctionAuth && strings.HasPrefix(r.URL.Path, "/function/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check bearer token first when provided
		if token := bearerToken(r.Header.Get("Authorization")); token != "" && m.tokenManager != nil {
			if _, ok := m.tokenManager.Validate(token); ok {
				m.rateLimiter.reset(clientKey(r))
				next.ServeHTTP(w, r)
				return
			}
			if allowed, retryAfter := m.rateLimiter.allow(clientKey(r)); !allowed {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))
				}
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			m.unauthorized(w)
			return
		}

		// Get credentials from request
		username, password, ok := r.BasicAuth()
		if !ok {
			if allowed, retryAfter := m.rateLimiter.allow(clientKey(r)); !allowed {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))
				}
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			m.unauthorized(w)
			return
		}

		// Constant-time comparison to prevent timing attacks
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(m.username)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(m.password)) == 1

		if !usernameMatch || !passwordMatch {
			if allowed, retryAfter := m.rateLimiter.allow(clientKey(r)); !allowed {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))
				}
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			m.logger.Warnf("Authentication failed for user: %s from %s", username, r.RemoteAddr)
			m.unauthorized(w)
			return
		}

		// Authentication successful
		m.rateLimiter.reset(clientKey(r))
		next.ServeHTTP(w, r)
	})
}

func (m *BasicAuthMiddleware) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="docker-faas"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Unauthorized"))
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
