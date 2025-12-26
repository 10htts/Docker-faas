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
		assert.Equal(t, "docker-faas-net", cfg.FunctionsNetwork)
		assert.Equal(t, true, cfg.AuthEnabled)
		assert.Equal(t, "admin", cfg.AuthUser)
		assert.Equal(t, "admin", cfg.AuthPassword)
		assert.Equal(t, "docker-faas.db", cfg.StateDBPath)
		assert.Equal(t, 1, cfg.DefaultReplicas)
		assert.Equal(t, 10, cfg.MaxReplicas)
	})

	t.Run("CustomValues", func(t *testing.T) {
		os.Setenv("GATEWAY_PORT", "9000")
		os.Setenv("FUNCTIONS_NETWORK", "custom-network")
		os.Setenv("AUTH_ENABLED", "false")
		os.Setenv("AUTH_USER", "testuser")
		os.Setenv("AUTH_PASSWORD", "testpass")
		os.Setenv("STATE_DB_PATH", "custom.db")
		os.Setenv("DEFAULT_REPLICAS", "3")
		os.Setenv("MAX_REPLICAS", "20")
		os.Setenv("READ_TIMEOUT", "30s")

		cfg := LoadConfig()

		assert.Equal(t, "9000", cfg.GatewayPort)
		assert.Equal(t, "custom-network", cfg.FunctionsNetwork)
		assert.Equal(t, false, cfg.AuthEnabled)
		assert.Equal(t, "testuser", cfg.AuthUser)
		assert.Equal(t, "testpass", cfg.AuthPassword)
		assert.Equal(t, "custom.db", cfg.StateDBPath)
		assert.Equal(t, 3, cfg.DefaultReplicas)
		assert.Equal(t, 20, cfg.MaxReplicas)
		assert.Equal(t, 30*time.Second, cfg.ReadTimeout)

		os.Clearenv()
	})
}
