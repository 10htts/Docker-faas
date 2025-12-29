package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
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

		os.Clearenv()
	})

	t.Run("DefaultCORSWhenAuthDisabled", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("AUTH_ENABLED", "false")

		cfg := LoadConfig()

		assert.Equal(t, []string{"*"}, cfg.CORSAllowedOrigins)
		os.Clearenv()
	})
}
