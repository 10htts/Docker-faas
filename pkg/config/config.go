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
	// StrictContainerOwnership scopes per-function container selection to
	// containers owned by THIS gateway (excludes unlabeled/legacy ones). On a
	// shared daemon, recreate legacy containers while this is false, then enable
	// it after every container has the ownership label (RT-234). Default false
	// preserves single-gateway upgrade behavior.
	StrictContainerOwnership bool

	// Authentication
	AuthEnabled             bool
	AuthUser                string
	AuthPassword            string
	RequireAuthForFunctions bool
	AuthRateLimit           int
	AuthRateWindow          time.Duration
	AuthTokenTTL            time.Duration

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

	// AsyncCallbackBlockInternal, when true, rejects async X-Callback-Url targets
	// that resolve to loopback / link-local / private / cloud-metadata addresses
	// (SSRF defense in depth, reusing the git-URL block list). Default false to
	// preserve OpenFaaS behavior, which allows callbacks to any host (including
	// legitimate internal services). Not a substitute for network egress control.
	AsyncCallbackBlockInternal bool

	// Build history
	BuildHistoryLimit     int
	BuildHistoryRetention time.Duration
	BuildOutputLimit      int

	// Network reconciliation
	ReconcileFunctionNetworks bool
	ReconcileIntervalSeconds  int

	// Idle scale-to-zero (provider-side; policy is authoritatively owned by the
	// AIDrivenMES control plane, SZ-11). The global Enabled flag ships OFF so
	// existing deployments are unaffected; the control plane enables scale-to-
	// zero per function via deployment labels, or globally by setting this flag.
	IdleScaleToZeroEnabled bool
	IdleReconcileSeconds   int
	IdleDurationSeconds    int
	IdleWarmMinReplicas    int

	// ActivityLeaseSecret is the ISOLATED, provider-side HMAC secret used to
	// authenticate the control plane's activity-lease requests and to sign the
	// activity-lease responses (CV-06 / SZ-12). It MUST be a dedicated shared
	// secret negotiated with the AIDrivenMES control plane — NOT the application
	// database secret and never hardcoded. Read from FAAS_ACTIVITY_LEASE_SECRET,
	// or from a file whose path is FAAS_ACTIVITY_LEASE_SECRET_FILE (the file form
	// takes precedence, so it can be delivered as a Docker/K8s secret mount). When
	// idle scale-to-zero is enabled this must be present or startup fails closed.
	ActivityLeaseSecret string

	// ActivityLeasePreviousSecret optionally holds the PREVIOUS lease secret
	// during a rotation overlap window: requests signed with either secret
	// verify, responses are always signed with the active secret (RT-214).
	// Remove it once the control plane has rolled over. Same env/file semantics
	// as the active secret (FAAS_ACTIVITY_LEASE_SECRET_PREVIOUS[_FILE]).
	ActivityLeasePreviousSecret string
	// ActivityLeaseMaxSkew bounds |now - issued_at| on signed activity-lease
	// requests (RT-214). Requests outside the window are rejected 401. Zero
	// disables the check (emergency escape hatch for broken clocks — logged).
	ActivityLeaseMaxSkew time.Duration
	// ActivityLeaseBodyLimit bounds the activity-lease request body in bytes
	// (RT-214). <=0 falls back to the built-in 64 KiB default; the limit cannot
	// be disabled.
	ActivityLeaseBodyLimit int64
	// ActivityLeaseReplayCap bounds the nonce replay cache entry count
	// (RT-217). <=0 falls back to the built-in 65536 default. Size it at
	// (leased functions × renewals per replay window) with headroom; at
	// capacity the oldest entry is evicted (request still accepted).
	ActivityLeaseReplayCap int
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
		GatewayPort:                getEnv("GATEWAY_PORT", "8080"),
		ReadTimeout:                getDurationEnv("READ_TIMEOUT", 60*time.Second),
		WriteTimeout:               getDurationEnv("WRITE_TIMEOUT", 60*time.Second),
		ExecTimeout:                getDurationEnv("EXEC_TIMEOUT", 60*time.Second),
		CORSAllowedOrigins:         corsAllowedOrigins,
		DockerHost:                 getEnv("DOCKER_HOST", ""),
		FunctionsNetwork:           getEnv("FUNCTIONS_NETWORK", "docker-faas-net"),
		StrictContainerOwnership:   getBoolEnv("FAAS_STRICT_CONTAINER_OWNERSHIP", false),
		AuthEnabled:                authEnabled,
		AuthUser:                   getEnv("AUTH_USER", "admin"),
		AuthPassword:               getEnv("AUTH_PASSWORD", "admin"),
		RequireAuthForFunctions:    getBoolEnv("REQUIRE_AUTH_FOR_FUNCTIONS", true),
		AuthRateLimit:              getIntEnv("AUTH_RATE_LIMIT", 10),
		AuthRateWindow:             getDurationEnv("AUTH_RATE_WINDOW", time.Minute),
		AuthTokenTTL:               getDurationEnv("AUTH_TOKEN_TTL", 30*time.Minute),
		StateDBPath:                getEnv("STATE_DB_PATH", "docker-faas.db"),
		MetricsEnabled:             getBoolEnv("METRICS_ENABLED", true),
		MetricsPort:                getEnv("METRICS_PORT", "9090"),
		LogLevel:                   logLevel,
		DefaultReplicas:            getIntEnv("DEFAULT_REPLICAS", 1),
		MaxReplicas:                getIntEnv("MAX_REPLICAS", 10),
		DebugBindAddress:           getEnv("DEBUG_BIND_ADDRESS", "127.0.0.1"),
		AsyncCallbackBlockInternal: getBoolEnv("FAAS_ASYNC_CALLBACK_BLOCK_INTERNAL", false),
		BuildHistoryLimit:          getIntEnv("BUILD_HISTORY_LIMIT", 100),
		BuildHistoryRetention:      getDurationEnv("BUILD_HISTORY_RETENTION", 24*time.Hour),
		BuildOutputLimit:           getIntEnv("BUILD_OUTPUT_LIMIT", 200*1024),
		ReconcileFunctionNetworks:  getBoolEnv("RECONCILE_FUNCTION_NETWORKS", true),
		ReconcileIntervalSeconds:   getIntEnv("RECONCILE_INTERVAL_SECONDS", 60),
		IdleScaleToZeroEnabled:     getBoolEnv("IDLE_SCALE_TO_ZERO_ENABLED", false),
		IdleReconcileSeconds:       getIntEnv("IDLE_RECONCILE_SECONDS", 30),
		IdleDurationSeconds:        getIntEnv("IDLE_DURATION_SECONDS", 300),
		IdleWarmMinReplicas:        getIntEnv("IDLE_WARM_MIN_REPLICAS", 0),
		ActivityLeaseSecret:        getSecretEnv("FAAS_ACTIVITY_LEASE_SECRET"),

		ActivityLeasePreviousSecret: getSecretEnv("FAAS_ACTIVITY_LEASE_SECRET_PREVIOUS"),
		ActivityLeaseMaxSkew:        getDurationEnv("FAAS_ACTIVITY_LEASE_MAX_SKEW", 2*time.Minute),
		ActivityLeaseBodyLimit:      int64(getIntEnv("FAAS_ACTIVITY_LEASE_BODY_LIMIT", 64*1024)),
		ActivityLeaseReplayCap:      getIntEnv("FAAS_ACTIVITY_LEASE_REPLAY_CAP", 64*1024),
	}
}

// getSecretEnv resolves a secret from either a _FILE-pointed file (preferred, so
// it can be delivered as a mounted Docker/Kubernetes secret) or the direct env
// var. The file form takes precedence; its trailing whitespace/newline is
// trimmed. Returns "" when neither is set.
func getSecretEnv(key string) string {
	if path := strings.TrimSpace(os.Getenv(key + "_FILE")); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return strings.TrimSpace(os.Getenv(key))
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
