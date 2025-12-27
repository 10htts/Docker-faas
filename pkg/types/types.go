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

// FunctionStatus represents the runtime status of a function
type FunctionStatus struct {
	Name                   string             `json:"name"`
	Image                  string             `json:"image"`
	Replicas               int                `json:"replicas"`
	AvailableReplicas      int                `json:"availableReplicas"`
	InvocationCount        int64              `json:"invocationCount"`
	EnvProcess             string             `json:"envProcess,omitempty"`
	EnvVars                map[string]string  `json:"envVars,omitempty"`
	Labels                 map[string]string  `json:"labels,omitempty"`
	Annotations            map[string]string  `json:"annotations,omitempty"`
	Namespace              string             `json:"namespace,omitempty"`
	Secrets                []string           `json:"secrets,omitempty"`
	Network                string             `json:"network,omitempty"`
	Limits                 *FunctionLimits    `json:"limits,omitempty"`
	Requests               *FunctionResources `json:"requests,omitempty"`
	ReadOnlyRootFilesystem bool               `json:"readOnlyRootFilesystem,omitempty"`
	Debug                  bool               `json:"debug,omitempty"`
	CreatedAt              time.Time          `json:"createdAt,omitempty"`
	UpdatedAt              time.Time          `json:"updatedAt,omitempty"`
}

// ScaleServiceRequest defines a scaling request
type ScaleServiceRequest struct {
	ServiceName string `json:"serviceName"`
	Replicas    int    `json:"replicas"`
}

// SystemInfo represents gateway system information
type SystemInfo struct {
	Provider struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		Orchestration string `json:"orchestration"`
	} `json:"provider"`
	Version struct {
		Release    string `json:"release"`
		SHA        string `json:"sha"`
		CommitDate string `json:"commit_date,omitempty"`
	} `json:"version"`
	Arch string `json:"arch"`
}

// FunctionMetadata represents stored function metadata
type FunctionMetadata struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Image      string    `json:"image"`
	EnvProcess string    `json:"envProcess,omitempty"`
	EnvVars    string    `json:"envVars,omitempty"` // JSON encoded
	Labels     string    `json:"labels,omitempty"`  // JSON encoded
	Secrets    string    `json:"secrets,omitempty"` // JSON encoded
	Network    string    `json:"network"`
	Replicas   int       `json:"replicas"`
	Limits     string    `json:"limits,omitempty"`   // JSON encoded
	Requests   string    `json:"requests,omitempty"` // JSON encoded
	ReadOnly   bool      `json:"readOnly"`
	Debug      bool      `json:"debug"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
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
