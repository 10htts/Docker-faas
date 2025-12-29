package gateway

import (
	"context"
	"net/http"

	"github.com/docker/docker/client"

	"github.com/docker-faas/docker-faas/pkg/secrets"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// Store defines the storage operations used by the gateway.
type Store interface {
	ListFunctions() ([]*types.FunctionMetadata, error)
	GetFunction(name string) (*types.FunctionMetadata, error)
	CreateFunction(metadata *types.FunctionMetadata) error
	UpdateFunction(metadata *types.FunctionMetadata) error
	DeleteFunction(name string) error
	UpdateReplicas(name string, replicas int) error
	HealthCheck(ctx context.Context) error
}

// Provider defines the container operations used by the gateway.
type Provider interface {
	DeployFunction(ctx context.Context, deployment *types.FunctionDeployment, replicas int) error
	UpdateFunction(ctx context.Context, deployment *types.FunctionDeployment, replicas int) error
	RemoveFunction(ctx context.Context, functionName string) error
	ScaleFunction(ctx context.Context, deployment *types.FunctionDeployment, targetReplicas int) error
	GetFunctionContainers(ctx context.Context, functionName string) ([]*types.Container, error)
	GetContainerLogs(ctx context.Context, functionName string, tail int) (string, error)
	CleanupFunctionNetwork(ctx context.Context, functionName, networkName string) error
	HealthCheck(ctx context.Context) error
	CheckNetwork(ctx context.Context) error
	DockerClient() *client.Client
	GetSecretManager() *secrets.SecretManager
}

// Router defines the routing operations used by the gateway.
type Router interface {
	RouteRequest(ctx context.Context, functionName string, req *http.Request) (*http.Response, error)
}
