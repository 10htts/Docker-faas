package gateway

import "time"

// AuthManager defines the token operations used by the gateway auth handlers.
type AuthManager interface {
	Issue(username string) (string, time.Time, error)
	Validate(token string) (string, bool)
	Revoke(token string)
}

// ConfigView exposes safe configuration values for the UI.
type ConfigView struct {
	AuthEnabled                  bool     `json:"authEnabled"`
	RequireAuthForFunctions      bool     `json:"requireAuthForFunctions"`
	CORSAllowedOrigins           []string `json:"corsAllowedOrigins"`
	FunctionsNetwork             string   `json:"functionsNetwork"`
	DefaultReplicas              int      `json:"defaultReplicas"`
	MaxReplicas                  int      `json:"maxReplicas"`
	MetricsEnabled               bool     `json:"metricsEnabled"`
	MetricsPort                  string   `json:"metricsPort"`
	DebugBindAddress             string   `json:"debugBindAddress"`
	AuthRateLimit                int      `json:"authRateLimit"`
	AuthRateWindowSeconds        int      `json:"authRateWindowSeconds"`
	AuthTokenTTLSeconds          int      `json:"authTokenTTLSeconds"`
	BuildHistoryLimit            int      `json:"buildHistoryLimit"`
	BuildHistoryRetentionSeconds int      `json:"buildHistoryRetentionSeconds"`
	BuildOutputLimit             int      `json:"buildOutputLimit"`
}
