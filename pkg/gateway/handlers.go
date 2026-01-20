package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/metrics"
	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/store"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// Gateway handles OpenFaaS API requests
type Gateway struct {
	store            Store
	provider         Provider
	router           Router
	logger           *logrus.Logger
	network          string
	builds           *BuildTracker
	authUser         string
	authPass         string
	authMgr          AuthManager
	config           *ConfigView
	buildOutputLimit int
}

// NewGateway creates a new gateway instance
func NewGateway(store Store, provider Provider, router Router, logger *logrus.Logger, network string) *Gateway {
	return &Gateway{
		store:            store,
		provider:         provider,
		router:           router,
		logger:           logger,
		network:          network,
		builds:           NewBuildTracker(100, 0),
		buildOutputLimit: 200 * 1024,
	}
}

// SetAuth configures auth credentials and token manager.
func (g *Gateway) SetAuth(manager AuthManager, username, password string) {
	g.authMgr = manager
	g.authUser = username
	g.authPass = password
}

// SetConfigView configures the read-only config view.
func (g *Gateway) SetConfigView(view *ConfigView) {
	g.config = view
}

// SetBuildTracker configures the build tracker.
func (g *Gateway) SetBuildTracker(tracker *BuildTracker) {
	g.builds = tracker
}

// SetBuildOutputLimit configures the max build output bytes retained.
func (g *Gateway) SetBuildOutputLimit(limit int) {
	if limit > 0 {
		g.buildOutputLimit = limit
	}
}

// HandleSystemInfo handles GET /system/info
func (g *Gateway) HandleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := types.SystemInfo{
		Arch: "x86_64",
	}
	info.Provider.Name = "docker-faas"
	info.Provider.Version = "2.2.0"
	info.Provider.Orchestration = "docker"
	info.Version.Release = "2.2.0"
	info.Version.SHA = "dev"

	g.writeJSON(w, http.StatusOK, info)
}

// HandleListFunctions handles GET /system/functions
func (g *Gateway) HandleListFunctions(w http.ResponseWriter, r *http.Request) {
	functions, err := g.store.ListFunctions()
	if err != nil {
		g.logger.Errorf("Failed to list functions: %v", err)
		http.Error(w, "Failed to list functions", http.StatusInternalServerError)
		return
	}

	statuses := make([]types.FunctionStatus, 0, len(functions))
	for _, fn := range functions {
		containers, err := g.provider.GetFunctionContainers(r.Context(), fn.Name)
		if err != nil {
			g.logger.Warnf("Failed to get containers for function %s: %v", fn.Name, err)
			continue
		}

		availableReplicas := 0
		for _, c := range containers {
			if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
				availableReplicas++
			}
		}

		var limits *types.FunctionLimits
		if fn.Limits != "" {
			var parsed types.FunctionLimits
			if err := json.Unmarshal([]byte(fn.Limits), &parsed); err == nil {
				limits = &parsed
			} else {
				g.logger.Warnf("Failed to parse limits for %s: %v", fn.Name, err)
			}
		}

		var requests *types.FunctionResources
		if fn.Requests != "" {
			var parsed types.FunctionResources
			if err := json.Unmarshal([]byte(fn.Requests), &parsed); err == nil {
				requests = &parsed
			} else {
				g.logger.Warnf("Failed to parse requests for %s: %v", fn.Name, err)
			}
		}

		status := types.FunctionStatus{
			Name:                   fn.Name,
			Image:                  fn.Image,
			Replicas:               fn.Replicas,
			AvailableReplicas:      availableReplicas,
			EnvProcess:             fn.EnvProcess,
			EnvVars:                store.DecodeMap(fn.EnvVars),
			Labels:                 store.DecodeMap(fn.Labels),
			Annotations:            make(map[string]string),
			Secrets:                store.DecodeSlice(fn.Secrets),
			Network:                fn.Network,
			Limits:                 limits,
			Requests:               requests,
			ReadOnlyRootFilesystem: fn.ReadOnly,
			Debug:                  fn.Debug,
			CreatedAt:              fn.CreatedAt,
			UpdatedAt:              fn.UpdatedAt,
		}

		statuses = append(statuses, status)
	}

	g.writeJSON(w, http.StatusOK, statuses)
}

// HandleFunctionContainers handles GET /system/function/<name>/containers
func (g *Gateway) HandleFunctionContainers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	functionName := normalizeFunctionName(vars["name"])

	if functionName == "" {
		http.Error(w, "Function name is required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	containers, err := g.provider.GetFunctionContainers(r.Context(), functionName)
	if err != nil {
		g.logger.Errorf("Failed to get containers for function %s: %v", functionName, err)
		http.Error(w, "Failed to get containers", http.StatusInternalServerError)
		return
	}

	g.writeJSON(w, http.StatusOK, containers)
}

// HandleDeployFunction handles POST /system/functions
func (g *Gateway) HandleDeployFunction(w http.ResponseWriter, r *http.Request) {
	var deployment types.FunctionDeployment
	if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if deployment.Service == "" || deployment.Image == "" {
		http.Error(w, "Service name and image are required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(deployment.Service); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.logger.Infof("Deploying function: %s (image: %s)", deployment.Service, deployment.Image)

	// Set network if not specified
	if deployment.Network == "" {
		deployment.Network = provider.FunctionNetworkName(g.network, deployment.Service)
	}

	// Check if function already exists
	existing, _ := g.store.GetFunction(deployment.Service)
	if existing != nil {
		http.Error(w, "Function already exists, use PUT to update", http.StatusConflict)
		return
	}

	// Default to 1 replica
	replicas := 1

	// Deploy function containers
	if err := g.provider.DeployFunction(r.Context(), &deployment, replicas); err != nil {
		g.logger.Errorf("Failed to deploy function: %v", err)
		http.Error(w, fmt.Sprintf("Failed to deploy function: %v", err), http.StatusInternalServerError)
		return
	}

	// Store function metadata
	envVars, err := store.EncodeMap(deployment.EnvVars)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode envVars: %v", err), http.StatusBadRequest)
		return
	}
	labels, err := store.EncodeMap(deployment.Labels)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode labels: %v", err), http.StatusBadRequest)
		return
	}
	secretsJSON, err := store.EncodeSlice(deployment.Secrets)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode secrets: %v", err), http.StatusBadRequest)
		return
	}

	metadata := &types.FunctionMetadata{
		Name:       deployment.Service,
		Image:      deployment.Image,
		EnvProcess: deployment.EnvProcess,
		EnvVars:    envVars,
		Labels:     labels,
		Secrets:    secretsJSON,
		Network:    deployment.Network,
		Replicas:   replicas,
		ReadOnly:   deployment.ReadOnlyRootFilesystem,
		Debug:      deployment.Debug,
	}

	if deployment.Limits != nil {
		limitsJSON, err := json.Marshal(deployment.Limits)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode limits: %v", err), http.StatusBadRequest)
			return
		}
		metadata.Limits = string(limitsJSON)
	}

	if deployment.Requests != nil {
		requestsJSON, err := json.Marshal(deployment.Requests)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode requests: %v", err), http.StatusBadRequest)
			return
		}
		metadata.Requests = string(requestsJSON)
	}

	if err := g.store.CreateFunction(metadata); err != nil {
		g.logger.Errorf("Failed to store function metadata: %v", err)
		// Try to cleanup deployed containers
		g.provider.RemoveFunction(r.Context(), deployment.Service)
		http.Error(w, "Failed to store function metadata", http.StatusInternalServerError)
		return
	}

	// Update metrics
	functions, _ := g.store.ListFunctions()
	metrics.UpdateFunctionsDeployed(len(functions))
	metrics.UpdateFunctionReplicas(deployment.Service, replicas)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Function deployed successfully"))
}

// HandleUpdateFunction handles PUT /system/functions
func (g *Gateway) HandleUpdateFunction(w http.ResponseWriter, r *http.Request) {
	var deployment types.FunctionDeployment
	if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if deployment.Service == "" || deployment.Image == "" {
		http.Error(w, "Service name and image are required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(deployment.Service); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.logger.Infof("Updating function: %s (image: %s)", deployment.Service, deployment.Image)

	// Get existing function
	existing, err := g.store.GetFunction(deployment.Service)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Set network if not specified
	if deployment.Network == "" {
		if existing.Network != "" {
			deployment.Network = existing.Network
		} else {
			deployment.Network = provider.FunctionNetworkName(g.network, deployment.Service)
		}
	}

	// Update function containers
	if err := g.provider.UpdateFunction(r.Context(), &deployment, existing.Replicas); err != nil {
		g.logger.Errorf("Failed to update function: %v", err)
		http.Error(w, fmt.Sprintf("Failed to update function: %v", err), http.StatusInternalServerError)
		return
	}

	// Update function metadata
	existing.Image = deployment.Image
	existing.EnvProcess = deployment.EnvProcess
	envVars, err := store.EncodeMap(deployment.EnvVars)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode envVars: %v", err), http.StatusBadRequest)
		return
	}
	labels, err := store.EncodeMap(deployment.Labels)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode labels: %v", err), http.StatusBadRequest)
		return
	}
	secretsJSON, err := store.EncodeSlice(deployment.Secrets)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode secrets: %v", err), http.StatusBadRequest)
		return
	}

	existing.EnvVars = envVars
	existing.Labels = labels
	existing.Secrets = secretsJSON
	existing.Network = deployment.Network
	existing.ReadOnly = deployment.ReadOnlyRootFilesystem
	existing.Debug = deployment.Debug

	if deployment.Limits != nil {
		limitsJSON, err := json.Marshal(deployment.Limits)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode limits: %v", err), http.StatusBadRequest)
			return
		}
		existing.Limits = string(limitsJSON)
	}

	if deployment.Requests != nil {
		requestsJSON, err := json.Marshal(deployment.Requests)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode requests: %v", err), http.StatusBadRequest)
			return
		}
		existing.Requests = string(requestsJSON)
	}

	if err := g.store.UpdateFunction(existing); err != nil {
		g.logger.Errorf("Failed to update function metadata: %v", err)
		http.Error(w, "Failed to update function metadata", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Function updated successfully"))
}

// HandleDeleteFunction handles DELETE /system/functions
func (g *Gateway) HandleDeleteFunction(w http.ResponseWriter, r *http.Request) {
	functionName := r.URL.Query().Get("functionName")
	if functionName == "" && r.Body != nil {
		var payload struct {
			FunctionName string `json:"functionName"`
			Service      string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			if payload.FunctionName != "" {
				functionName = payload.FunctionName
			} else if payload.Service != "" {
				functionName = payload.Service
			}
		}
	}
	functionName = normalizeFunctionName(functionName)
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if functionName == "" {
		http.Error(w, "functionName parameter is required", http.StatusBadRequest)
		return
	}

	g.logger.Infof("Deleting function: %s", functionName)

	metadata, err := g.store.GetFunction(functionName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Remove function containers
	if err := g.provider.RemoveFunction(r.Context(), functionName); err != nil {
		g.logger.Errorf("Failed to remove function containers: %v", err)
		http.Error(w, fmt.Sprintf("Failed to remove function: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete function metadata
	if err := g.store.DeleteFunction(functionName); err != nil {
		g.logger.Errorf("Failed to delete function metadata: %v", err)
		http.Error(w, "Failed to delete function metadata", http.StatusInternalServerError)
		return
	}

	if err := g.provider.CleanupFunctionNetwork(r.Context(), metadata.Name, metadata.Network); err != nil {
		g.logger.Warnf("Failed to cleanup function network: %v", err)
	}

	// Update metrics
	functions, _ := g.store.ListFunctions()
	metrics.UpdateFunctionsDeployed(len(functions))
	metrics.DeleteFunctionMetrics(functionName)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Function deleted successfully"))
}

// HandleScaleFunction handles POST /system/scale-function/<name>
func (g *Gateway) HandleScaleFunction(w http.ResponseWriter, r *http.Request) {
	var scaleReq types.ScaleServiceRequest
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &scaleReq); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			if scaleReq.ServiceName == "" {
				var alt struct {
					Service      string `json:"service"`
					FunctionName string `json:"functionName"`
				}
				if err := json.Unmarshal(body, &alt); err == nil {
					if alt.Service != "" {
						scaleReq.ServiceName = alt.Service
					} else if alt.FunctionName != "" {
						scaleReq.ServiceName = alt.FunctionName
					}
				}
			}
		}
	}

	if scaleReq.ServiceName == "" {
		if name := mux.Vars(r)["name"]; name != "" {
			scaleReq.ServiceName = name
		}
	}
	scaleReq.ServiceName = normalizeFunctionName(scaleReq.ServiceName)
	if err := validateFunctionName(scaleReq.ServiceName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if scaleReq.ServiceName == "" {
		http.Error(w, "serviceName is required", http.StatusBadRequest)
		return
	}

	if scaleReq.Replicas < 0 {
		http.Error(w, "replicas must be >= 0", http.StatusBadRequest)
		return
	}

	g.logger.Infof("Scaling function %s to %d replicas", scaleReq.ServiceName, scaleReq.Replicas)

	// Get function metadata
	metadata, err := g.store.GetFunction(scaleReq.ServiceName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Build deployment spec
	deployment := &types.FunctionDeployment{
		Service:                metadata.Name,
		Image:                  metadata.Image,
		Network:                metadata.Network,
		EnvProcess:             metadata.EnvProcess,
		EnvVars:                store.DecodeMap(metadata.EnvVars),
		Labels:                 store.DecodeMap(metadata.Labels),
		Secrets:                store.DecodeSlice(metadata.Secrets),
		ReadOnlyRootFilesystem: metadata.ReadOnly,
		Debug:                  metadata.Debug,
	}

	// Scale function
	if err := g.provider.ScaleFunction(r.Context(), deployment, scaleReq.Replicas); err != nil {
		g.logger.Errorf("Failed to scale function: %v", err)
		http.Error(w, fmt.Sprintf("Failed to scale function: %v", err), http.StatusInternalServerError)
		return
	}

	// Update replicas in store
	if err := g.store.UpdateReplicas(scaleReq.ServiceName, scaleReq.Replicas); err != nil {
		g.logger.Errorf("Failed to update replicas in store: %v", err)
		http.Error(w, "Failed to update replicas", http.StatusInternalServerError)
		return
	}

	// Update metrics
	metrics.UpdateFunctionReplicas(scaleReq.ServiceName, scaleReq.Replicas)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Function scaled successfully"))
}

// HandleGetLogs handles GET /system/logs?name=<function>
func (g *Gateway) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	functionName := normalizeFunctionName(r.URL.Query().Get("name"))
	if functionName == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tail := 100
	if tailStr := r.URL.Query().Get("tail"); tailStr != "" {
		if t, err := strconv.Atoi(tailStr); err == nil {
			tail = t
		}
	}

	logs, err := g.provider.GetContainerLogs(r.Context(), functionName, tail)
	if err != nil {
		g.logger.Errorf("Failed to get logs: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(logs))
}

// HandleInvokeFunction handles POST /function/<name>
func (g *Gateway) HandleInvokeFunction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	functionName := normalizeFunctionName(vars["name"])

	if functionName == "" {
		http.Error(w, "Function name is required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	startTime := time.Now()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Create new request
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "/", strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Route request
	resp, err := g.router.RouteRequest(r.Context(), functionName, req)
	if err != nil {
		g.logger.Errorf("Failed to invoke function %s: %v", functionName, err)
		metrics.RecordFunctionInvocation(functionName, http.StatusInternalServerError, time.Since(startTime).Seconds())
		http.Error(w, fmt.Sprintf("Failed to invoke function: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy response body
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	// Record metrics
	duration := time.Since(startTime).Seconds()
	metrics.RecordFunctionInvocation(functionName, resp.StatusCode, duration)
}

func normalizeFunctionName(name string) string {
	name = strings.TrimSpace(name)
	for _, suffix := range []string{".openfaas-fn", ".openfaas"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

// HandleHealthz handles GET /healthz
func (g *Gateway) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := map[string]string{}
	ok := true

	if err := g.store.HealthCheck(ctx); err != nil {
		checks["database"] = err.Error()
		ok = false
	} else {
		checks["database"] = "ok"
	}

	if err := g.provider.HealthCheck(ctx); err != nil {
		checks["docker"] = err.Error()
		ok = false
	} else {
		checks["docker"] = "ok"
	}

	if err := g.provider.CheckNetwork(ctx); err != nil {
		checks["network"] = err.Error()
		ok = false
	} else {
		checks["network"] = "ok"
	}

	acceptsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	if ok {
		if acceptsJSON {
			g.writeJSON(w, http.StatusOK, map[string]interface{}{
				"status": "ok",
				"checks": checks,
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}

	if acceptsJSON {
		g.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"checks": checks,
		})
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("Unhealthy"))
}

// writeJSON writes a JSON response
func (g *Gateway) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
