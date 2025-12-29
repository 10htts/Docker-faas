package middleware

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		middleware := NewBasicAuthMiddleware("admin", "admin", false, true, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "Success", rr.Body.String())
	})

	t.Run("ValidCredentials", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, nil, logger)
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
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		auth := base64.StdEncoding.EncodeToString([]byte("admin:wrong"))
		req.Header.Set("Authorization", "Basic "+auth)

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("NoCredentials", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("HealthCheckBypass", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("FunctionBypassWhenDisabled", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, false, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("POST", "/function/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("FunctionRequiresAuthWhenEnabled", func(t *testing.T) {
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, nil, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("POST", "/function/test", nil)
		rr := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("RateLimitOnFailures", func(t *testing.T) {
		limiter := NewAuthRateLimiter(1, time.Minute)
		middleware := NewBasicAuthMiddleware("admin", "secret", true, true, limiter, logger)
		wrappedHandler := middleware.Middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		req2 := httptest.NewRequest("GET", "/test", nil)
		rr2 := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	})
}
