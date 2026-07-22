package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	scale            *scaleToZeroDeps
	// asyncCallbackBlockInternal opts into SSRF filtering of async X-Callback-Url
	// targets (default false = OpenFaaS-compatible: any callback host allowed).
	asyncCallbackBlockInternal bool
}

// SetAsyncCallbackPolicy enables/disables SSRF filtering of async callback URLs.
func (g *Gateway) SetAsyncCallbackPolicy(blockInternal bool) {
	g.asyncCallbackBlockInternal = blockInternal
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

// Provider identity constants surfaced by /system/info and X-Served-By.
const (
	providerName          = "docker-faas"
	providerOrchestration = "docker"
	providerRelease       = "2.2.0"
	providerSHA           = "dev"
)

// defaultMaxReplicas mirrors the config default applied when no ConfigView is
// wired into the gateway.
const defaultMaxReplicas = 10

// HandleSystemInfo handles GET /system/info.
//
// The JSON shape mirrors the pinned gateway (openfaas/faas 0.27.13)
// GatewayInfo/faas-provider v0.25.12 ProviderInfo: the provider name is under
// key "provider" (plus legacy additive "name"), version objects carry
// sha/release.
func (g *Gateway) HandleSystemInfo(w http.ResponseWriter, r *http.Request) {
	version := &types.VersionInfo{
		Release: providerRelease,
		SHA:     providerSHA,
	}
	info := types.SystemInfo{
		Provider: types.ProviderInfo{
			Name:          providerName,
			LegacyName:    providerName,
			Orchestration: providerOrchestration,
			Version:       version,
		},
		Version: version,
		Arch:    "x86_64",
	}

	g.writeJSON(w, http.StatusOK, info)
}

// HandleListNamespaces handles GET /system/namespaces. The response shape is
// the plain JSON string list served by the pinned providers (faas-cli 0.18.0
// proxy/namespaces.go decodes []string).
func (g *Gateway) HandleListNamespaces(w http.ResponseWriter, r *http.Request) {
	g.writeJSON(w, http.StatusOK, []string{DefaultFunctionNamespace})
}

// runningReplicas counts containers that report a running state.
func runningReplicas(containers []*types.Container) int {
	running := 0
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			running++
		}
	}
	return running
}

// functionStatusFromMetadata builds the pinned faas-provider FunctionStatus
// shape from stored metadata plus the observed available replica count.
func (g *Gateway) functionStatusFromMetadata(fn *types.FunctionMetadata, availableReplicas int) types.FunctionStatus {
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

	return types.FunctionStatus{
		Name:                   fn.Name,
		Image:                  fn.Image,
		Namespace:              DefaultFunctionNamespace,
		Replicas:               fn.Replicas,
		AvailableReplicas:      availableReplicas,
		EnvProcess:             fn.EnvProcess,
		EnvVars:                store.DecodeMap(fn.EnvVars),
		Labels:                 store.DecodeMap(fn.Labels),
		Annotations:            store.DecodeMap(fn.Annotations),
		Secrets:                store.DecodeSlice(fn.Secrets),
		Network:                fn.Network,
		Limits:                 limits,
		Requests:               requests,
		ReadOnlyRootFilesystem: fn.ReadOnly,
		Debug:                  fn.Debug,
		CreatedAt:              fn.CreatedAt,
		UpdatedAt:              fn.UpdatedAt,
	}
}

// HandleListFunctions handles GET /system/functions
func (g *Gateway) HandleListFunctions(w http.ResponseWriter, r *http.Request) {
	// Only the default namespace exists; unknown namespaces answer 400
	// "namespace not valid" like the pinned reference provider (faasd).
	if err := validateNamespace(r.URL.Query().Get("namespace")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	functions, err := g.store.ListFunctions()
	if err != nil {
		g.logger.Errorf("Failed to list functions: %v", err)
		http.Error(w, "Failed to list functions", http.StatusInternalServerError)
		return
	}

	statuses := make([]types.FunctionStatus, 0, len(functions))
	for _, fn := range functions {
		// A transient container-query error must NOT drop the function from the
		// listing (that would make faas-cli list silently show fewer functions
		// than exist, with no error signalled). Report it with 0 available
		// replicas instead — the function is declared and stored regardless of
		// current container state.
		available := 0
		if containers, err := g.provider.GetFunctionContainers(r.Context(), fn.Name); err != nil {
			g.logger.Warnf("Failed to get containers for function %s (reporting availableReplicas=0): %v", fn.Name, err)
		} else {
			available = runningReplicas(containers)
		}
		statuses = append(statuses, g.functionStatusFromMetadata(fn, available))
	}

	g.writeJSON(w, http.StatusOK, statuses)
}

// HandleGetFunction handles GET /system/function/{name}.
//
// Response shape and status codes follow the pinned OpenAPI spec
// (openfaas/faas 0.27.13 api-docs/spec.openapi.yml: 200/404/500) and the
// pinned faas-provider v0.25.12 FunctionStatus JSON keys, which faas-cli
// 0.18.0 `describe` decodes.
func (g *Gateway) HandleGetFunction(w http.ResponseWriter, r *http.Request) {
	functionName := normalizeFunctionName(mux.Vars(r)["name"])
	if functionName == "" {
		http.Error(w, "Function name is required", http.StatusBadRequest)
		return
	}
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateNamespace(r.URL.Query().Get("namespace")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fn, err := g.store.GetFunction(functionName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	containers, err := g.provider.GetFunctionContainers(r.Context(), functionName)
	if err != nil {
		g.logger.Errorf("Failed to get containers for function %s: %v", functionName, err)
		http.Error(w, "Failed to get function containers", http.StatusInternalServerError)
		return
	}

	g.writeJSON(w, http.StatusOK, g.functionStatusFromMetadata(fn, runningReplicas(containers)))
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
	if err := validateNamespace(deployment.Namespace); err != nil {
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

	// Initial replicas honor the scale.min label (custom com.docker-faas label
	// wins over com.openfaas.scale.min), clamped to [1, config MaxReplicas].
	replicas := g.initialReplicas(deployment.Service, deployment.Labels)

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
	annotations, err := store.EncodeMap(deployment.Annotations)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode annotations: %v", err), http.StatusBadRequest)
		return
	}
	secretsJSON, err := store.EncodeSlice(deployment.Secrets)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode secrets: %v", err), http.StatusBadRequest)
		return
	}

	metadata := &types.FunctionMetadata{
		Name:        deployment.Service,
		Image:       deployment.Image,
		EnvProcess:  deployment.EnvProcess,
		EnvVars:     envVars,
		Labels:      labels,
		Annotations: annotations,
		Secrets:     secretsJSON,
		Network:     deployment.Network,
		Replicas:    replicas,
		ReadOnly:    deployment.ReadOnlyRootFilesystem,
		Debug:       deployment.Debug,
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
	if err := validateNamespace(deployment.Namespace); err != nil {
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
	annotations, err := store.EncodeMap(deployment.Annotations)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode annotations: %v", err), http.StatusBadRequest)
		return
	}
	secretsJSON, err := store.EncodeSlice(deployment.Secrets)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode secrets: %v", err), http.StatusBadRequest)
		return
	}

	existing.EnvVars = envVars
	existing.Labels = labels
	existing.Annotations = annotations
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
	namespace := r.URL.Query().Get("namespace")
	if r.Body != nil {
		// Pinned request body shape: faas-provider v0.25.12
		// DeleteFunctionRequest {functionName, namespace}. The legacy "service"
		// key is accepted additively.
		var payload struct {
			FunctionName string `json:"functionName"`
			Service      string `json:"service"`
			Namespace    string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			if functionName == "" {
				if payload.FunctionName != "" {
					functionName = payload.FunctionName
				} else if payload.Service != "" {
					functionName = payload.Service
				}
			}
			if payload.Namespace != "" {
				namespace = payload.Namespace
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
	if err := validateNamespace(namespace); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	// Drop the function's scale-to-zero coordination state so it neither leaks
	// nor survives a delete→redeploy of the same name (a stale ready flag or
	// generation must not carry over to the new function's lifecycle, RT-216).
	if g.scale != nil {
		if g.scale.gates != nil {
			g.scale.gates.Forget(functionName)
		}
		if g.scale.leases != nil {
			g.scale.leases.Forget(functionName)
		}
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
	if err := validateNamespace(scaleReq.Namespace); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	functionLabels := store.DecodeMap(metadata.Labels)

	// Clamp the requested replicas to the function's max-replica label
	// (com.openfaas.scale.max; the custom com.docker-faas.scale.max-replicas
	// label wins when both are present). Upstream clamp-vs-reject behavior is
	// ambiguous at the pin, so we clamp and record the decision in
	// redteam/CONFORMANCE_MATRIX.md. Explicit scale to 0 (pause) stays allowed.
	if _, maxReplicas := scaleBoundsFromLabels(functionLabels); maxReplicas > 0 && scaleReq.Replicas > maxReplicas {
		g.logger.Warnf("Clamping scale request for %s from %d to label max %d",
			scaleReq.ServiceName, scaleReq.Replicas, maxReplicas)
		scaleReq.Replicas = maxReplicas
	}

	// Build deployment spec
	deployment := &types.FunctionDeployment{
		Service:                metadata.Name,
		Image:                  metadata.Image,
		Network:                metadata.Network,
		EnvProcess:             metadata.EnvProcess,
		EnvVars:                store.DecodeMap(metadata.EnvVars),
		Labels:                 functionLabels,
		Annotations:            store.DecodeMap(metadata.Annotations),
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

// HandleGetLogs (GET /system/logs) lives in logs_handlers.go and serves the
// pinned faas-provider NDJSON log contract.

// initialReplicas resolves the initial replica count for a new deployment:
// clamp(scale.min label if set else 1, 1, config MaxReplicas).
func (g *Gateway) initialReplicas(functionName string, labels map[string]string) int {
	replicas := 1
	if minReplicas, _ := scaleBoundsFromLabels(labels); minReplicas > 1 {
		replicas = minReplicas
	}
	if maxReplicas := g.configMaxReplicas(); replicas > maxReplicas {
		g.logger.Warnf("Clamping initial replicas for %s from %d to config max %d",
			functionName, replicas, maxReplicas)
		replicas = maxReplicas
	}
	return replicas
}

// configMaxReplicas returns the configured max replicas, defaulting to
// defaultMaxReplicas when no ConfigView is wired.
func (g *Gateway) configMaxReplicas() int {
	if g.config != nil && g.config.MaxReplicas > 0 {
		return g.config.MaxReplicas
	}
	return defaultMaxReplicas
}

// stampCallHeaders mirrors the pinned gateway's call-id middleware
// (openfaas/faas 0.27.13 gateway/handlers/callid_middleware.go): a caller
// supplied X-Call-Id is preserved, otherwise one is generated; X-Start-Time is
// the UTC start time in UnixNano; both are stamped on the upstream request and
// the response, plus an X-Served-By marker.
func (g *Gateway) stampCallHeaders(w http.ResponseWriter, r *http.Request) (callID string, start time.Time) {
	start = time.Now()
	callID = r.Header.Get("X-Call-Id")
	if callID == "" {
		callID = generateCallID()
		r.Header.Set("X-Call-Id", callID)
	}
	startNanos := fmt.Sprintf("%d", start.UTC().UnixNano())
	r.Header.Set("X-Start-Time", startNanos)

	w.Header().Set("X-Call-Id", callID)
	w.Header().Set("X-Start-Time", startNanos)
	w.Header().Set("X-Served-By", providerName+"/"+providerRelease)
	return callID, start
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

	_, startTime := g.stampCallHeaders(w, r)

	// Get function metadata
	fn, err := g.store.GetFunction(functionName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Register this invocation as in-flight BEFORE checking replica
	// availability so the idle reaper cannot reclaim the function between the
	// check and the route (SZ-01). The token also records whether a reclaim was
	// already in progress, forcing a cold start rather than routing to a
	// container being torn down.
	release, reclaimInProgress := g.trackInvocation(functionName)
	defer release()

	// CRITICAL: Check if function needs to scale up from zero
	containers, err := g.provider.GetFunctionContainers(r.Context(), functionName)
	if err != nil {
		g.logger.Errorf("Failed to get containers for function %s: %v", functionName, err)
		http.Error(w, "Failed to get function containers", http.StatusInternalServerError)
		return
	}

	availableReplicas := 0
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			availableReplicas++
		}
	}

	if availableReplicas == 0 || reclaimInProgress {
		g.logger.Infof("Scaling function %s from zero...", functionName)

		// Single-leader cold start: concurrent requests create ONE container
		// and the rest wait on readiness (SZ-02).
		if err := g.ensureReadyFromZero(r.Context(), fn); err != nil {
			g.logger.Errorf("Failed to scale function %s from zero: %v", functionName, err)
			http.Error(w, fmt.Sprintf("Failed to scale function: %v", err), http.StatusInternalServerError)
			return
		}

		g.logger.Infof("Function %s scaled from zero and ready", functionName)
	}

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

// scaleFromZero scales a function from zero replicas to one replica
func (g *Gateway) scaleFromZero(ctx context.Context, fn *types.FunctionMetadata) error {
	// Build deployment spec from stored metadata
	deployment := &types.FunctionDeployment{
		Service:                fn.Name,
		Image:                  fn.Image,
		Network:                fn.Network,
		EnvProcess:             fn.EnvProcess,
		EnvVars:                store.DecodeMap(fn.EnvVars),
		Labels:                 store.DecodeMap(fn.Labels),
		Annotations:            store.DecodeMap(fn.Annotations),
		Secrets:                store.DecodeSlice(fn.Secrets),
		ReadOnlyRootFilesystem: fn.ReadOnly,
		Debug:                  fn.Debug,
	}

	// Parse limits if present
	if fn.Limits != "" {
		var limits types.FunctionLimits
		if err := json.Unmarshal([]byte(fn.Limits), &limits); err == nil {
			deployment.Limits = &limits
		}
	}

	// Parse requests if present
	if fn.Requests != "" {
		var requests types.FunctionResources
		if err := json.Unmarshal([]byte(fn.Requests), &requests); err == nil {
			deployment.Requests = &requests
		}
	}

	// Scale to 1 replica
	targetReplicas := 1
	if err := g.provider.ScaleFunction(ctx, deployment, targetReplicas); err != nil {
		return fmt.Errorf("failed to scale function: %w", err)
	}

	// Update replicas in store
	if err := g.store.UpdateReplicas(fn.Name, targetReplicas); err != nil {
		g.logger.Warnf("Failed to update replicas in store for %s: %v", fn.Name, err)
	}

	// Update metrics
	metrics.UpdateFunctionReplicas(fn.Name, targetReplicas)

	return nil
}

// waitForFunctionReady waits for a function to have at least one running container
func (g *Gateway) waitForFunctionReady(ctx context.Context, functionName string, timeout time.Duration) error {
	// Immediate first check: a container that is already running must not pay
	// the 500ms tick floor on every cold start (RT-219).
	if g.isContainerHealthy(ctx, functionName) {
		return nil
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check if function has running containers
			if g.isContainerHealthy(ctx, functionName) {
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for function to be ready")
			}
		}
	}
}

// isContainerHealthy checks if at least one container is running for the function
func (g *Gateway) isContainerHealthy(ctx context.Context, functionName string) bool {
	containers, err := g.provider.GetFunctionContainers(ctx, functionName)
	if err != nil {
		g.logger.Debugf("Failed to get containers for %s during health check: %v", functionName, err)
		return false
	}

	// Check if at least one container is running
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			// Container is running, consider it healthy
			g.logger.Debugf("Container %s for function %s is healthy (status: %s)", c.Name, functionName, c.Status)
			return true
		}
	}

	return false
}

// writeJSON writes a JSON response
func (g *Gateway) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
