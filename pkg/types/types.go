package types

import "time"

// FunctionDeployment represents a function deployment specification
type FunctionDeployment struct {
	Service                string             `json:"service"`
	Image                  string             `json:"image"`
	Network                string             `json:"network,omitempty"`
	EnvProcess             string             `json:"envProcess,omitempty"`
	EnvVars                map[string]string  `json:"envVars,omitempty"`
	Labels                 map[string]string  `json:"labels,omitempty"`
	Secrets                []string           `json:"secrets,omitempty"`
	Limits                 *FunctionLimits    `json:"limits,omitempty"`
	Requests               *FunctionResources `json:"requests,omitempty"`
	Constraints            []string           `json:"constraints,omitempty"`
	Annotations            map[string]string  `json:"annotations,omitempty"`
	Namespace              string             `json:"namespace,omitempty"`
	ReadOnlyRootFilesystem bool               `json:"readOnlyRootFilesystem,omitempty"`
	Debug                  bool               `json:"debug,omitempty"`
}

// FunctionLimits defines resource limits
type FunctionLimits struct {
	Memory string `json:"memory,omitempty"`
	CPU    string `json:"cpu,omitempty"`
}

// FunctionResources defines resource requests
type FunctionResources struct {
	Memory string `json:"memory,omitempty"`
	CPU    string `json:"cpu,omitempty"`
}

// FunctionStatus represents the runtime status of a function.
//
// The JSON keys mirror github.com/openfaas/faas-provider/types.FunctionStatus
// at the pinned tag v0.25.12 exactly (name, image, namespace, envProcess,
// envVars, constraints, secrets, labels, annotations, limits, requests,
// readOnlyRootFilesystem, invocationCount, replicas, availableReplicas,
// createdAt, usage). Extra keys (network, debug, updatedAt) are additive
// extensions and do not collide with pinned names.
type FunctionStatus struct {
	Name                   string             `json:"name"`
	Image                  string             `json:"image"`
	Namespace              string             `json:"namespace,omitempty"`
	EnvProcess             string             `json:"envProcess,omitempty"`
	EnvVars                map[string]string  `json:"envVars,omitempty"`
	Constraints            []string           `json:"constraints,omitempty"`
	Secrets                []string           `json:"secrets,omitempty"`
	Labels                 map[string]string  `json:"labels,omitempty"`
	Annotations            map[string]string  `json:"annotations,omitempty"`
	Limits                 *FunctionLimits    `json:"limits,omitempty"`
	Requests               *FunctionResources `json:"requests,omitempty"`
	ReadOnlyRootFilesystem bool               `json:"readOnlyRootFilesystem,omitempty"`
	InvocationCount        int64              `json:"invocationCount"`
	Replicas               int                `json:"replicas"`
	AvailableReplicas      int                `json:"availableReplicas"`
	CreatedAt              time.Time          `json:"createdAt,omitempty"`
	Usage                  *FunctionUsage     `json:"usage,omitempty"`

	// Additive extensions (not part of the pinned faas-provider contract).
	Network   string    `json:"network,omitempty"`
	Debug     bool      `json:"debug,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// FunctionUsage mirrors faas-provider types.FunctionUsage (pinned v0.25.12).
type FunctionUsage struct {
	CPU              float64 `json:"cpu,omitempty"`
	TotalMemoryBytes float64 `json:"totalMemoryBytes,omitempty"`
}

// ScaleServiceRequest defines a scaling request. Keys mirror faas-provider
// v0.25.12 types.ScaleServiceRequest (serviceName, replicas, namespace).
type ScaleServiceRequest struct {
	ServiceName string `json:"serviceName"`
	Replicas    int    `json:"replicas"`
	Namespace   string `json:"namespace,omitempty"`
}

// VersionInfo mirrors faas-provider types.VersionInfo (pinned v0.25.12):
// keys sha, release, commit_message. commit_date is a legacy additive key.
type VersionInfo struct {
	SHA           string `json:"sha"`
	Release       string `json:"release"`
	CommitMessage string `json:"commit_message,omitempty"`
	CommitDate    string `json:"commit_date,omitempty"`
}

// ProviderInfo mirrors faas-provider types.ProviderInfo (pinned v0.25.12):
// the provider name is emitted under the pinned key "provider" and, for
// backwards compatibility with earlier docker-faas clients, additively under
// the legacy key "name" with the same value.
type ProviderInfo struct {
	Name          string       `json:"provider"`
	LegacyName    string       `json:"name,omitempty"`
	Orchestration string       `json:"orchestration"`
	Version       *VersionInfo `json:"version,omitempty"`
}

// SystemInfo represents gateway system information. The JSON shape mirrors the
// pinned gateway (openfaas/faas 0.27.13) GatewayInfo: {provider, version, arch}.
type SystemInfo struct {
	Provider ProviderInfo `json:"provider"`
	Version  *VersionInfo `json:"version"`
	Arch     string       `json:"arch"`
}

// FunctionMetadata represents stored function metadata
type FunctionMetadata struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	EnvProcess  string    `json:"envProcess,omitempty"`
	EnvVars     string    `json:"envVars,omitempty"`     // JSON encoded
	Labels      string    `json:"labels,omitempty"`      // JSON encoded
	Annotations string    `json:"annotations,omitempty"` // JSON encoded
	Secrets     string    `json:"secrets,omitempty"`     // JSON encoded
	Network     string    `json:"network"`
	Replicas    int       `json:"replicas"`
	Limits      string    `json:"limits,omitempty"`   // JSON encoded
	Requests    string    `json:"requests,omitempty"` // JSON encoded
	ReadOnly    bool      `json:"readOnly"`
	Debug       bool      `json:"debug"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Container represents a running function container instance
type Container struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	IPAddress string            `json:"ipAddress,omitempty"`
	Status    string            `json:"status"`
	Ports     map[string]string `json:"ports,omitempty"` // ContainerPort -> HostPort
	Created   time.Time         `json:"createdAt"`
}

// InvocationMetrics stores metrics for function invocations
type InvocationMetrics struct {
	FunctionName string
	StatusCode   int
	Duration     time.Duration
	Timestamp    time.Time
}
