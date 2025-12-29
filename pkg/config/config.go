package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	// Gateway settings
	GatewayPort        string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ExecTimeout        time.Duration
	CORSAllowedOrigins []string

	// Docker settings
	DockerHost       string
	FunctionsNetwork string

	// Authentication
	AuthEnabled             bool
	AuthUser                string
	AuthPassword            string
	RequireAuthForFunctions bool
	AuthRateLimit           int
	AuthRateWindow          time.Duration

	// Database
	StateDBPath string

	// Metrics
	MetricsEnabled bool
	MetricsPort    string

	// Logging
	LogLevel string

	// Defaults
	DefaultReplicas int
	MaxReplicas     int

	// Debug settings
	DebugBindAddress string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	authEnabled := getBoolEnv("AUTH_ENABLED", true)
	logLevel := getEnv("LOG_LEVEL", "info")
	corsAllowedOrigins := getCSVEnv("CORS_ALLOWED_ORIGINS")
	if len(corsAllowedOrigins) == 0 && !authEnabled {
		corsAllowedOrigins = []string{"*"}
	}

	return &Config{
		GatewayPort:             getEnv("GATEWAY_PORT", "8080"),
		ReadTimeout:             getDurationEnv("READ_TIMEOUT", 60*time.Second),
		WriteTimeout:            getDurationEnv("WRITE_TIMEOUT", 60*time.Second),
		ExecTimeout:             getDurationEnv("EXEC_TIMEOUT", 60*time.Second),
		CORSAllowedOrigins:      corsAllowedOrigins,
		DockerHost:              getEnv("DOCKER_HOST", ""),
		FunctionsNetwork:        getEnv("FUNCTIONS_NETWORK", "docker-faas-net"),
		AuthEnabled:             authEnabled,
		AuthUser:                getEnv("AUTH_USER", "admin"),
		AuthPassword:            getEnv("AUTH_PASSWORD", "admin"),
		RequireAuthForFunctions: getBoolEnv("REQUIRE_AUTH_FOR_FUNCTIONS", true),
		AuthRateLimit:           getIntEnv("AUTH_RATE_LIMIT", 10),
		AuthRateWindow:          getDurationEnv("AUTH_RATE_WINDOW", time.Minute),
		StateDBPath:             getEnv("STATE_DB_PATH", "docker-faas.db"),
		MetricsEnabled:          getBoolEnv("METRICS_ENABLED", true),
		MetricsPort:             getEnv("METRICS_PORT", "9090"),
		LogLevel:                logLevel,
		DefaultReplicas:         getIntEnv("DEFAULT_REPLICAS", 1),
		MaxReplicas:             getIntEnv("MAX_REPLICAS", 10),
		DebugBindAddress:        getEnv("DEBUG_BIND_ADDRESS", "127.0.0.1"),
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getCSVEnv(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func getBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolVal, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return boolVal
}

func getIntEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intVal, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intVal
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}
