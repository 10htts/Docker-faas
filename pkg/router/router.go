package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/types"
	"github.com/sirupsen/logrus"
)

// Router handles routing requests to function containers
type Router struct {
	provider     *provider.DockerProvider
	logger       *logrus.Logger
	readTimeout  time.Duration
	writeTimeout time.Duration
	execTimeout  time.Duration
	roundRobin   map[string]*uint64 // Function name -> counter for round-robin
}

// NewRouter creates a new router instance
func NewRouter(provider *provider.DockerProvider, logger *logrus.Logger, readTimeout, writeTimeout, execTimeout time.Duration) *Router {
	return &Router{
		provider:     provider,
		logger:       logger,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		execTimeout:  execTimeout,
		roundRobin:   make(map[string]*uint64),
	}
}

// RouteRequest routes a request to a function container
func (r *Router) RouteRequest(ctx context.Context, functionName string, req *http.Request) (*http.Response, error) {
	// Get function containers
	containers, err := r.provider.GetFunctionContainers(ctx, functionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get function containers: %w", err)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers available for function: %s", functionName)
	}

	// Select container using round-robin
	container := r.selectContainer(functionName, containers)

	// Forward request to container
	return r.forwardRequest(ctx, container, req)
}

// selectContainer selects a container using round-robin load balancing
func (r *Router) selectContainer(functionName string, containers []*types.Container) *types.Container {
	if _, ok := r.roundRobin[functionName]; !ok {
		var counter uint64 = 0
		r.roundRobin[functionName] = &counter
	}

	counter := r.roundRobin[functionName]
	index := atomic.AddUint64(counter, 1) % uint64(len(containers))

	// Filter for running containers
	runningContainers := make([]*types.Container, 0)
	for _, c := range containers {
		if c.Status == "running" || c.Status == "Up" {
			runningContainers = append(runningContainers, c)
		}
	}

	if len(runningContainers) == 0 {
		// Fallback to any container
		return containers[index]
	}

	return runningContainers[index%uint64(len(runningContainers))]
}

// forwardRequest forwards an HTTP request to a container
func (r *Router) forwardRequest(ctx context.Context, container *types.Container, req *http.Request) (*http.Response, error) {
	// Build target URL (OpenFaaS watchdog listens on port 8080)
	targetURL := fmt.Sprintf("http://%s:8080", container.IPAddress)

	// Create new request
	proxyReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy headers
	for key, values := range req.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Add X-Forwarded headers
	proxyReq.Header.Set("X-Forwarded-For", req.RemoteAddr)
	proxyReq.Header.Set("X-Forwarded-Host", req.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", req.URL.Scheme)

	// Create HTTP client with timeouts
	client := &http.Client{
		Timeout: r.execTimeout,
		Transport: &http.Transport{
			ResponseHeaderTimeout: r.readTimeout,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	// Execute request
	resp, err := client.Do(proxyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to forward request to container: %w", err)
	}

	return resp, nil
}

// InvokeFunction is a convenience method that wraps RouteRequest
func (r *Router) InvokeFunction(ctx context.Context, functionName string, body io.Reader, headers map[string]string) ([]byte, int, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "/", body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Route request
	resp, err := r.RouteRequest(ctx, functionName, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}
