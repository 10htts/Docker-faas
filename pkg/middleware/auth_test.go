package middleware

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestBasicAuthMiddleware(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{}) // Use a buffer instead of nil to avoid panic

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	})

	t.Run("AuthDisabled", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "admin", false, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "Success", rr.Body.String())
	})

	t.Run("ValidCredentials", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		auth := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
		req.Header.Set("Authorization", "Basic "+auth)

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "Success", rr.Body.String())
	})

	t.Run("InvalidCredentials", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		auth := base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
		req.Header.Set("Authorization", "Basic "+auth)

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("NoCredentials", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("HealthCheckBypass", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
