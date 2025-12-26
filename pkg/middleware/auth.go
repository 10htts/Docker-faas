package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/sirupsen/logrus"
)

// BasicAuthMiddleware provides basic authentication
type BasicAuthMiddleware struct {
	username string
	password string
	enabled  bool
	logger   *logrus.Logger
}

// NewBasicAuthMiddleware creates a new basic auth middleware
func NewBasicAuthMiddleware(username, password string, enabled bool, logger *logrus.Logger) *BasicAuthMiddleware {
	return &BasicAuthMiddleware{
		username: username,
		password: password,
		enabled:  enabled,
		logger:   logger,
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

		// Get credentials from request
		username, password, ok := r.BasicAuth()
		if !ok {
			m.unauthorized(w)
			return
		}

		// Constant-time comparison to prevent timing attacks
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(m.username)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(m.password)) == 1

		if !usernameMatch || !passwordMatch {
			m.logger.Warnf("Authentication failed for user: %s from %s", username, r.RemoteAddr)
			m.unauthorized(w)
			return
		}

		// Authentication successful
		next.ServeHTTP(w, r)
	})
}

func (m *BasicAuthMiddleware) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="docker-faas"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Unauthorized"))
}
