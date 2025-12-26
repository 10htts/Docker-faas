package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration
type Config struct {
	// Gateway settings
	GatewayPort      string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	ExecTimeout      time.Duration

	// Docker settings
	DockerHost       string
	FunctionsNetwork string

	// Authentication
	AuthEnabled      bool
	AuthUser         string
	AuthPassword     string

	// Database
	StateDBPath      string

	// Metrics
	MetricsEnabled   bool
	MetricsPort      string

	// Logging
	LogLevel         string

	// Defaults
	DefaultReplicas  int
	MaxReplicas      int

	// Debug settings
	DebugBindAddress string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		GatewayPort:      getEnv("GATEWAY_PORT", "8080"),
		ReadTimeout:      getDurationEnv("READ_TIMEOUT", 60*time.Second),
		WriteTimeout:     getDurationEnv("WRITE_TIMEOUT", 60*time.Second),
		ExecTimeout:      getDurationEnv("EXEC_TIMEOUT", 60*time.Second),
		DockerHost:       getEnv("DOCKER_HOST", ""),
		FunctionsNetwork: getEnv("FUNCTIONS_NETWORK", "docker-faas-net"),
		AuthEnabled:      getBoolEnv("AUTH_ENABLED", true),
		AuthUser:         getEnv("AUTH_USER", "admin"),
		AuthPassword:     getEnv("AUTH_PASSWORD", "admin"),
		StateDBPath:      getEnv("STATE_DB_PATH", "docker-faas.db"),
		MetricsEnabled:   getBoolEnv("METRICS_ENABLED", true),
		MetricsPort:      getEnv("METRICS_PORT", "9090"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		DefaultReplicas:  getIntEnv("DEFAULT_REPLICAS", 1),
		MaxReplicas:      getIntEnv("MAX_REPLICAS", 10),
		DebugBindAddress: getEnv("DEBUG_BIND_ADDRESS", "127.0.0.1"),
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
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
