package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/secrets"
	faasTypes "github.com/docker-faas/docker-faas/pkg/types"
)

const (
	// LabelNamespace is the label key for function namespace
	LabelNamespace = "com.docker-faas.namespace"
	// LabelFunction is the label key for function name
	LabelFunction = "com.docker-faas.function"
	// LabelType is the label key for container type
	LabelType = "com.docker-faas.type"
	// LabelReplica is the label key for replica index
	LabelReplica = "com.docker-faas.replica"
	// LabelNetwork is the label key for function network
	LabelNetwork = "com.docker-faas.network.name"
	// LabelNetworkType is the label key for managed network type
	LabelNetworkType = "com.docker-faas.network.type"
	// LabelNetworkFunction is the label key for function-specific networks
	LabelNetworkFunction = "com.docker-faas.network.function"
)

// DockerProvider manages Docker containers for functions
type DockerProvider struct {
	client           *client.Client
	network          string
	logger           *logrus.Logger
	secretManager    *secrets.SecretManager
	gatewayID        string
	connectGateway   bool
	debugBindAddress string
}

// NewDockerProvider creates a new Docker provider
func NewDockerProvider(dockerHost, networkName, debugBindAddress string, logger *logrus.Logger) (*DockerProvider, error) {
	var cli *client.Client
	var err error

	if dockerHost != "" {
		cli, err = client.NewClientWithOpts(
			client.WithHost(dockerHost),
			client.WithAPIVersionNegotiation(),
		)
	} else {
		cli, err = client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Initialize secret manager
	secretManager, err := secrets.NewSecretManager("", logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize secret manager: %w", err)
	}

	gatewayID, connectGateway := resolveGatewayContainer()

	// Default to localhost if not provided for security
	if debugBindAddress == "" {
		debugBindAddress = "127.0.0.1"
	}

	provider := &DockerProvider{
		client:           cli,
		network:          networkName,
		logger:           logger,
		secretManager:    secretManager,
		gatewayID:        gatewayID,
		connectGateway:   connectGateway,
		debugBindAddress: debugBindAddress,
	}

	// Ensure network exists
	if provider.network != "" {
		labels := map[string]string{
			LabelNetworkType: "base",
		}
		if err := provider.ensureNetwork(context.Background(), provider.network, labels); err != nil {
			return nil, fmt.Errorf("failed to ensure network: %w", err)
		}
	}

	return provider, nil
}

// ensureNetwork creates the Docker network if it doesn't exist
func (p *DockerProvider) ensureNetwork(ctx context.Context, networkName string, labels map[string]string) error {
	networks, err := p.client.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", networkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	exists := false
	for _, network := range networks {
		if network.Name == networkName {
			exists = true
			break
		}
	}

	if !exists {
		networkLabels := map[string]string{
			"com.docker-faas.network": "true",
		}
		for key, value := range labels {
			networkLabels[key] = value
		}

		p.logger.Infof("Creating network: %s", networkName)
		_, err = p.client.NetworkCreate(ctx, networkName, network.CreateOptions{
			Driver: "bridge",
			Labels: networkLabels,
		})
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	return nil
}

// DeployFunction deploys a function with specified replicas
func (p *DockerProvider) DeployFunction(ctx context.Context, deployment *faasTypes.FunctionDeployment, replicas int) error {
	p.logger.Infof("Deploying function: %s with %d replicas", deployment.Service, replicas)

	// Pull image
	if err := p.pullImage(ctx, deployment.Image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Create containers for each replica
	for i := 0; i < replicas; i++ {
		containerName := fmt.Sprintf("%s-%d", deployment.Service, i)

		if err := p.createContainer(ctx, deployment, containerName, i); err != nil {
			return fmt.Errorf("failed to create container %s: %w", containerName, err)
		}
	}

	return nil
}

// pullImage pulls the Docker image
func (p *DockerProvider) pullImage(ctx context.Context, imageStr string) error {
	if p.imageExists(ctx, imageStr) {
		p.logger.Infof("Using local image: %s", imageStr)
		return nil
	}

	p.logger.Infof("Pulling image: %s", imageStr)

	reader, err := p.client.ImagePull(ctx, imageStr, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Consume the output to ensure the pull completes
	_, err = io.Copy(io.Discard, reader)
	return err
}

func (p *DockerProvider) imageExists(ctx context.Context, imageStr string) bool {
	_, _, err := p.client.ImageInspectWithRaw(ctx, imageStr)
	if err == nil {
		return true
	}
	if errdefs.IsNotFound(err) {
		return false
	}
	p.logger.Debugf("Image inspect failed for %s: %v", imageStr, err)
	return false
}

func (p *DockerProvider) resolveSecretsHostPath(ctx context.Context) string {
	basePath := p.secretManager.GetBasePath()
	if p.gatewayID == "" {
		return basePath
	}

	inspect, err := p.client.ContainerInspect(ctx, p.gatewayID)
	if err != nil {
		p.logger.Debugf("Failed to inspect gateway container for secrets mount: %v", err)
		return basePath
	}

	for _, m := range inspect.Mounts {
		if m.Destination != secrets.ContainerSecretsPath && m.Destination != basePath {
			continue
		}
		if m.Type == "bind" && m.Source != "" {
			return m.Source
		}
		if m.Type == "volume" && m.Name != "" {
			vol, err := p.client.VolumeInspect(ctx, m.Name)
			if err == nil && vol.Mountpoint != "" {
				return vol.Mountpoint
			}
			if err != nil {
				p.logger.Debugf("Volume inspect failed for %s: %v", m.Name, err)
			}
		}
		if m.Source != "" {
			return m.Source
		}
	}

	return basePath
}

// createContainer creates and starts a function container
func (p *DockerProvider) createContainer(ctx context.Context, deployment *faasTypes.FunctionDeployment, name string, replicaIndex int) error {
	networkName := deployment.Network
	if networkName == "" {
		networkName = p.network
	}

	if networkName == "" {
		return fmt.Errorf("network is required for function %s", deployment.Service)
	}

	networkLabels := map[string]string{
		LabelNetworkType:     "function",
		LabelNetworkFunction: deployment.Service,
	}
	if err := p.ensureNetwork(ctx, networkName, networkLabels); err != nil {
		return fmt.Errorf("failed to ensure network %s: %w", networkName, err)
	}

	if err := p.ensureGatewayConnected(ctx, networkName); err != nil {
		return fmt.Errorf("failed to connect gateway to network %s: %w", networkName, err)
	}

	containerLabels := make(map[string]string)
	containerLabels[LabelFunction] = deployment.Service
	containerLabels[LabelType] = "function"
	containerLabels[LabelReplica] = fmt.Sprintf("%d", replicaIndex)
	containerLabels[LabelNetwork] = networkName

	// Add custom labels
	for k, v := range deployment.Labels {
		containerLabels[k] = v
	}

	env := []string{}
	for k, v := range deployment.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add fprocess environment variable if specified
	if deployment.EnvProcess != "" {
		env = append(env, fmt.Sprintf("fprocess=%s", deployment.EnvProcess))
	}

	containerConfig := &container.Config{
		Image:  deployment.Image,
		Labels: containerLabels,
		Env:    env,
	}

	if deployment.Debug {
		containerConfig.ExposedPorts = nat.PortSet{
			"40000/tcp": {},
			"5678/tcp":  {},
		}
	}

	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(networkName),
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		SecurityOpt: []string{"no-new-privileges:true"},
		CapDrop:     []string{"ALL"},
	}

	if deployment.Debug {
		// Bind debug ports to configured address (default: 127.0.0.1 for security)
		hostConfig.PortBindings = nat.PortMap{
			"40000/tcp": []nat.PortBinding{{HostIP: p.debugBindAddress, HostPort: "0"}},
			"5678/tcp":  []nat.PortBinding{{HostIP: p.debugBindAddress, HostPort: "0"}},
		}

		// Log security warning if debug ports are exposed on all interfaces
		if p.debugBindAddress == "0.0.0.0" {
			p.logger.Warnf("DEBUG MODE: Function %s has debug ports exposed on ALL interfaces (0.0.0.0)", deployment.Service)
			p.logger.Warn("This is a security risk in production. Set DEBUG_BIND_ADDRESS=127.0.0.1 to restrict access")
		} else {
			p.logger.Infof("Debug mode enabled for %s - ports bound to %s", deployment.Service, p.debugBindAddress)
		}
	}

	// Apply resource limits if specified
	if deployment.Limits != nil {
		if deployment.Limits.Memory != "" {
			hostConfig.Resources.Memory = parseMemory(deployment.Limits.Memory)
		}
		if deployment.Limits.CPU != "" {
			hostConfig.Resources.NanoCPUs = parseCPU(deployment.Limits.CPU)
		}
	}

	// Apply read-only root filesystem if specified
	if deployment.ReadOnlyRootFilesystem {
		hostConfig.ReadonlyRootfs = true
	}

	// Mount secrets if specified
	if len(deployment.Secrets) > 0 {
		created, err := p.secretManager.EnsureSecrets(deployment.Secrets)
		if err != nil {
			return fmt.Errorf("failed to ensure secrets: %w", err)
		}
		if len(created) > 0 {
			p.logger.Warnf("Auto-created missing secrets for %s: %s", deployment.Service, strings.Join(created, ", "))
		}

		// Validate secrets exist
		if err := p.secretManager.ValidateSecrets(deployment.Secrets); err != nil {
			return fmt.Errorf("secret validation failed: %w", err)
		}

		// Create bind mounts for each secret
		mounts := make([]mount.Mount, 0, len(deployment.Secrets))
		hostSecretsPath := p.resolveSecretsHostPath(ctx)
		for _, secretName := range deployment.Secrets {
			secretPath := filepath.Join(hostSecretsPath, secretName)
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   secretPath,
				Target:   fmt.Sprintf("%s/%s", secrets.ContainerSecretsPath, secretName),
				ReadOnly: true,
			})
		}
		hostConfig.Mounts = mounts

		p.logger.Infof("Mounting %d secrets for function %s", len(deployment.Secrets), deployment.Service)
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {},
		},
	}

	// Create container
	resp, err := p.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, name)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := p.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	p.logger.Infof("Container created and started: %s (ID: %s)", name, resp.ID)
	return nil
}

// UpdateFunction updates a function deployment
func (p *DockerProvider) UpdateFunction(ctx context.Context, deployment *faasTypes.FunctionDeployment, replicas int) error {
	// For simplicity, we remove old containers and create new ones
	if err := p.RemoveFunction(ctx, deployment.Service); err != nil {
		p.logger.Warnf("Failed to remove old containers: %v", err)
	}

	return p.DeployFunction(ctx, deployment, replicas)
}

// RemoveFunction removes all containers for a function
func (p *DockerProvider) RemoveFunction(ctx context.Context, functionName string) error {
	p.logger.Infof("Removing function: %s", functionName)

	containers, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		// Stop container
		timeout := int(10) // 10 seconds
		if err := p.client.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			p.logger.Warnf("Failed to stop container %s: %v", c.ID, err)
		}

		// Remove container
		if err := p.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
			p.logger.Warnf("Failed to remove container %s: %v", c.ID, err)
		} else {
			p.logger.Infof("Removed container: %s", c.ID[:12])
		}
	}

	return nil
}

// ScaleFunction scales a function to the specified replica count
func (p *DockerProvider) ScaleFunction(ctx context.Context, deployment *faasTypes.FunctionDeployment, targetReplicas int) error {
	p.logger.Infof("Scaling function %s to %d replicas", deployment.Service, targetReplicas)

	containers, err := p.listFunctionContainers(ctx, deployment.Service)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	currentReplicas := len(containers)

	if targetReplicas > currentReplicas {
		// Scale up: create new containers
		for i := currentReplicas; i < targetReplicas; i++ {
			containerName := fmt.Sprintf("%s-%d", deployment.Service, i)
			if err := p.createContainer(ctx, deployment, containerName, i); err != nil {
				return fmt.Errorf("failed to create container: %w", err)
			}
		}
	} else if targetReplicas < currentReplicas {
		// Scale down: remove excess containers
		for i := targetReplicas; i < currentReplicas; i++ {
			containerName := fmt.Sprintf("%s-%d", deployment.Service, i)
			if err := p.removeContainer(ctx, containerName); err != nil {
				p.logger.Warnf("Failed to remove container %s: %v", containerName, err)
			}
		}
	}

	return nil
}

// removeContainer removes a specific container by name
func (p *DockerProvider) removeContainer(ctx context.Context, name string) error {
	timeout := int(10)
	if err := p.client.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := p.client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	return nil
}

// listFunctionContainers lists all containers for a function
func (p *DockerProvider) listFunctionContainers(ctx context.Context, functionName string) ([]container.Summary, error) {
	filters := filters.NewArgs()
	filters.Add("label", fmt.Sprintf("%s=%s", LabelFunction, functionName))

	return p.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters,
	})
}

// GetFunctionContainers retrieves container information for a function
func (p *DockerProvider) GetFunctionContainers(ctx context.Context, functionName string) ([]*faasTypes.Container, error) {
	containers, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return nil, err
	}

	result := make([]*faasTypes.Container, 0, len(containers))
	for _, c := range containers {
		info, err := p.client.ContainerInspect(ctx, c.ID)
		if err != nil {
			p.logger.Warnf("Failed to inspect container %s: %v", c.ID, err)
			continue
		}

		ipAddress := ""
		if info.NetworkSettings != nil {
			networkName := p.network
			if info.Config != nil && info.Config.Labels != nil {
				if labelNetwork, ok := info.Config.Labels[LabelNetwork]; ok && labelNetwork != "" {
					networkName = labelNetwork
				}
			}

			if ep, ok := info.NetworkSettings.Networks[networkName]; ok {
				ipAddress = ep.IPAddress
			} else {
				for _, ep := range info.NetworkSettings.Networks {
					ipAddress = ep.IPAddress
					break
				}
			}
		}

		ports := make(map[string]string)
		if info.NetworkSettings != nil {
			for port, bindings := range info.NetworkSettings.Ports {
				if len(bindings) > 0 {
					ports[string(port)] = bindings[0].HostPort
				}
			}
		}

		result = append(result, &faasTypes.Container{
			ID:        c.ID,
			Name:      strings.TrimPrefix(c.Names[0], "/"),
			IPAddress: ipAddress,
			Status:    c.Status,
			Ports:     ports,
			Created:   time.Unix(c.Created, 0),
		})
	}

	return result, nil
}

// GetContainerLogs retrieves logs from a function container
func (p *DockerProvider) GetContainerLogs(ctx context.Context, functionName string, tail int) (string, error) {
	containers, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return "", err
	}

	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found for function: %s", functionName)
	}

	// Get logs from the first container
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}

	reader, err := p.client.ContainerLogs(ctx, containers[0].ID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	logs, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}

// Close closes the Docker client
func (p *DockerProvider) Close() error {
	return p.client.Close()
}

// DockerClient exposes the underlying Docker client for advanced operations.
func (p *DockerProvider) DockerClient() *client.Client {
	return p.client
}

// GetSecretManager returns the secret manager instance
func (p *DockerProvider) GetSecretManager() *secrets.SecretManager {
	return p.secretManager
}

// CleanupFunctionNetwork removes a managed per-function network if unused.
func (p *DockerProvider) CleanupFunctionNetwork(ctx context.Context, functionName, networkName string) error {
	if networkName == "" {
		return nil
	}

	inspect, err := p.client.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err != nil {
		if isNetworkNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect network %s: %w", networkName, err)
	}

	if !p.isManagedFunctionNetwork(networkName, functionName, inspect.Labels) {
		return nil
	}

	if p.connectGateway && p.gatewayID != "" {
		if err := p.client.NetworkDisconnect(ctx, networkName, p.gatewayID, true); err != nil && !isNotConnectedErr(err) {
			p.logger.Warnf("Failed to disconnect gateway from network %s: %v", networkName, err)
		}
	}

	inspect, err = p.client.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err != nil {
		if isNetworkNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("failed to re-inspect network %s: %w", networkName, err)
	}

	if len(inspect.Containers) > 0 {
		p.logger.Infof("Network %s still has %d containers attached; skipping removal", networkName, len(inspect.Containers))
		return nil
	}

	if err := p.client.NetworkRemove(ctx, networkName); err != nil {
		if isNetworkInUseErr(err) {
			p.logger.Infof("Network %s still in use; skipping removal", networkName)
			return nil
		}
		return fmt.Errorf("failed to remove network %s: %w", networkName, err)
	}

	p.logger.Infof("Removed function network: %s", networkName)
	return nil
}

// Helper functions

// FunctionNetworkName builds a per-function network name from a base network.
func FunctionNetworkName(baseNetwork, service string) string {
	if baseNetwork == "" {
		return service
	}
	return fmt.Sprintf("%s-%s", baseNetwork, service)
}

func (p *DockerProvider) ensureGatewayConnected(ctx context.Context, networkName string) error {
	if !p.connectGateway || p.gatewayID == "" {
		return nil
	}

	if err := p.client.NetworkConnect(ctx, networkName, p.gatewayID, nil); err != nil {
		if isAlreadyConnectedErr(err) {
			return nil
		}
		return err
	}

	p.logger.Infof("Connected gateway container %s to network %s", p.gatewayID, networkName)
	return nil
}

func (p *DockerProvider) isManagedFunctionNetwork(networkName, functionName string, labels map[string]string) bool {
	if functionName != "" && networkName == FunctionNetworkName(p.network, functionName) {
		return true
	}

	if labels == nil {
		return false
	}

	if labels[LabelNetworkType] != "function" {
		return false
	}

	if labels[LabelNetworkFunction] == "" {
		return true
	}

	return labels[LabelNetworkFunction] == functionName
}

func resolveGatewayContainer() (string, bool) {
	if name := os.Getenv("GATEWAY_CONTAINER_NAME"); name != "" {
		return name, true
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		if hostname, err := os.Hostname(); err == nil && hostname != "" {
			return hostname, true
		}
	}

	return "", false
}

func isAlreadyConnectedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already connected")
}

func isNotConnectedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not connected") || strings.Contains(msg, "is not connected")
}

func isNetworkNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "network") && strings.Contains(msg, "not found")
}

func isNetworkInUseErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "has active endpoints") || strings.Contains(msg, "network is in use")
}

func parseMemory(mem string) int64 {
	// Simple parser for memory strings like "128m", "1g"
	mem = strings.TrimSpace(strings.ToLower(mem))
	if mem == "" {
		return 0
	}

	multiplier := int64(1)
	if strings.HasSuffix(mem, "k") {
		multiplier = 1024
		mem = mem[:len(mem)-1]
	} else if strings.HasSuffix(mem, "m") {
		multiplier = 1024 * 1024
		mem = mem[:len(mem)-1]
	} else if strings.HasSuffix(mem, "g") {
		multiplier = 1024 * 1024 * 1024
		mem = mem[:len(mem)-1]
	}

	var value int64
	fmt.Sscanf(mem, "%d", &value)
	return value * multiplier
}

func parseCPU(cpu string) int64 {
	// Parse CPU strings like "0.5", "1", "2"
	var value float64
	fmt.Sscanf(cpu, "%f", &value)
	return int64(value * 1e9) // Convert to nano CPUs
}
