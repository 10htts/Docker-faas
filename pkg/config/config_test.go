package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Capture a temp dir BEFORE any subtest calls os.Clearenv() (which wipes
	// TMP/TEMP for the whole process and would break a later t.TempDir()).
	leaseSecretDir := t.TempDir()

	t.Run("DefaultValues", func(t *testing.T) {
		// Clear environment variables
		os.Clearenv()

		cfg := LoadConfig()

		assert.Equal(t, "8080", cfg.GatewayPort)
		assert.Empty(t, cfg.CORSAllowedOrigins)
		assert.Equal(t, "docker-faas-net", cfg.FunctionsNetwork)
		assert.Equal(t, true, cfg.AuthEnabled)
		assert.Equal(t, "admin", cfg.AuthUser)
		assert.Equal(t, "admin", cfg.AuthPassword)
		assert.Equal(t, true, cfg.RequireAuthForFunctions)
		assert.Equal(t, 10, cfg.AuthRateLimit)
		assert.Equal(t, time.Minute, cfg.AuthRateWindow)
		assert.Equal(t, "docker-faas.db", cfg.StateDBPath)
		assert.Equal(t, 1, cfg.DefaultReplicas)
		assert.Equal(t, 10, cfg.MaxReplicas)
		assert.Equal(t, true, cfg.ReconcileFunctionNetworks)
		assert.Equal(t, 60, cfg.ReconcileIntervalSeconds)
	})

	t.Run("CustomValues", func(t *testing.T) {
		os.Setenv("GATEWAY_PORT", "9000")
		os.Setenv("FUNCTIONS_NETWORK", "custom-network")
		os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com, http://localhost:8080")
		os.Setenv("AUTH_ENABLED", "false")
		os.Setenv("AUTH_USER", "testuser")
		os.Setenv("AUTH_PASSWORD", "testpass")
		os.Setenv("REQUIRE_AUTH_FOR_FUNCTIONS", "false")
		os.Setenv("AUTH_RATE_LIMIT", "5")
		os.Setenv("AUTH_RATE_WINDOW", "30s")
		os.Setenv("STATE_DB_PATH", "custom.db")
		os.Setenv("DEFAULT_REPLICAS", "3")
		os.Setenv("MAX_REPLICAS", "20")
		os.Setenv("READ_TIMEOUT", "30s")
		os.Setenv("RECONCILE_FUNCTION_NETWORKS", "false")
		os.Setenv("RECONCILE_INTERVAL_SECONDS", "120")

		cfg := LoadConfig()

		assert.Equal(t, "9000", cfg.GatewayPort)
		assert.Equal(t, []string{"https://example.com", "http://localhost:8080"}, cfg.CORSAllowedOrigins)
		assert.Equal(t, "custom-network", cfg.FunctionsNetwork)
		assert.Equal(t, false, cfg.AuthEnabled)
		assert.Equal(t, "testuser", cfg.AuthUser)
		assert.Equal(t, "testpass", cfg.AuthPassword)
		assert.Equal(t, false, cfg.RequireAuthForFunctions)
		assert.Equal(t, 5, cfg.AuthRateLimit)
		assert.Equal(t, 30*time.Second, cfg.AuthRateWindow)
		assert.Equal(t, "custom.db", cfg.StateDBPath)
		assert.Equal(t, 3, cfg.DefaultReplicas)
		assert.Equal(t, 20, cfg.MaxReplicas)
		assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
		assert.Equal(t, false, cfg.ReconcileFunctionNetworks)
		assert.Equal(t, 120, cfg.ReconcileIntervalSeconds)

		os.Clearenv()
	})

	t.Run("DefaultCORSWhenAuthDisabled", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("AUTH_ENABLED", "false")

		cfg := LoadConfig()

		assert.Equal(t, []string{"*"}, cfg.CORSAllowedOrigins)
		os.Clearenv()
	})

	// CV-06: the isolated activity-lease secret is absent by default, readable
	// from the direct env var, and readable from a _FILE mount that takes
	// precedence (so it can be delivered as a Docker/K8s secret).
	t.Run("ActivityLeaseSecretDefaultsEmpty", func(t *testing.T) {
		os.Clearenv()
		cfg := LoadConfig()
		assert.Empty(t, cfg.ActivityLeaseSecret)
	})

	t.Run("ActivityLeaseSecretFromEnv", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("FAAS_ACTIVITY_LEASE_SECRET", "  provider-lease-secret  ")
		cfg := LoadConfig()
		assert.Equal(t, "provider-lease-secret", cfg.ActivityLeaseSecret)
		os.Clearenv()
	})

	t.Run("ActivityLeaseSecretFileTakesPrecedence", func(t *testing.T) {
		// Use a dir captured BEFORE any subtest called os.Clearenv() (which wipes
		// TMP/TEMP for the whole process and would break a fresh t.TempDir()).
		path := filepath.Join(leaseSecretDir, "lease.secret")
		if err := os.WriteFile(path, []byte("file-delivered-secret\n"), 0o600); err != nil {
			t.Fatalf("write secret file: %v", err)
		}
		os.Clearenv()
		os.Setenv("FAAS_ACTIVITY_LEASE_SECRET", "env-secret-should-be-overridden")
		os.Setenv("FAAS_ACTIVITY_LEASE_SECRET_FILE", path)
		cfg := LoadConfig()
		assert.Equal(t, "file-delivered-secret", cfg.ActivityLeaseSecret)
		os.Clearenv()
	})
}
