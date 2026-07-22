package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// GatewayRestartsTotal tracks gateway process starts/restarts.
	GatewayRestartsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_restarts_total",
			Help: "Total number of gateway process starts",
		},
	)

	// GatewayHTTPRequestsTotal tracks total HTTP requests to the gateway
	GatewayHTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests to the gateway",
		},
		[]string{"method", "path", "code"},
	)

	// GatewayHTTPErrorsTotal tracks HTTP error responses from the gateway
	GatewayHTTPErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_errors_total",
			Help: "Total number of HTTP error responses from the gateway",
		},
		[]string{"method", "path", "code"},
	)

	// GatewayRequestDurationSeconds tracks gateway request duration
	GatewayRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Duration of gateway HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// FunctionInvocationsTotal tracks total function invocations
	FunctionInvocationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_invocations_total",
			Help: "Total number of function invocations",
		},
		[]string{"function_name", "code"},
	)

	// FunctionDurationSeconds tracks function invocation duration
	FunctionDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "function_duration_seconds",
			Help:    "Duration of function invocations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"function_name"},
	)

	// FunctionErrorsTotal tracks total function errors
	FunctionErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_errors_total",
			Help: "Total number of function errors",
		},
		[]string{"function_name"},
	)

	// DBOperationsTotal tracks database operations
	DBOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_db_operations_total",
			Help: "Total number of database operations",
		},
		[]string{"operation", "status"},
	)

	// DBOperationDurationSeconds tracks database operation duration
	DBOperationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "store_db_operation_duration_seconds",
			Help:    "Duration of database operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// FunctionsDeployed tracks number of deployed functions
	FunctionsDeployed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "functions_deployed",
			Help: "Number of currently deployed functions",
		},
	)

	// FunctionReplicas tracks replica count per function
	FunctionReplicas = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "function_replicas",
			Help: "Number of replicas per function",
		},
		[]string{"function_name"},
	)

	// --- Idle scale-to-zero metrics (SZ-09) ---

	// FunctionObservedReplicas tracks the PROVIDER-OBSERVED running replica
	// count per function (as opposed to the desired FunctionReplicas gauge).
	FunctionObservedReplicas = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "function_observed_replicas",
			Help: "Provider-observed running replica count per function",
		},
		[]string{"function_name"},
	)

	// FunctionColdStartsTotal counts scale-from-zero cold starts per function.
	FunctionColdStartsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_cold_starts_total",
			Help: "Total scale-from-zero cold starts per function",
		},
		[]string{"function_name", "result"},
	)

	// FunctionColdStartDurationSeconds measures cold-start latency per function.
	FunctionColdStartDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "function_cold_start_duration_seconds",
			Help:    "Duration of scale-from-zero cold starts in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"function_name"},
	)

	// FunctionIdleReclamationsTotal counts idle scale-to-zero reclamations.
	FunctionIdleReclamationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_idle_reclamations_total",
			Help: "Total idle scale-to-zero reclamations per function",
		},
		[]string{"function_name"},
	)

	// FunctionReclaimedContainersTotal counts containers removed by reclamation.
	FunctionReclaimedContainersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_reclaimed_containers_total",
			Help: "Total containers removed by idle reclamation per function",
		},
		[]string{"function_name"},
	)

	// FunctionReclaimedNetworksTotal counts per-function networks removed.
	FunctionReclaimedNetworksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_reclaimed_networks_total",
			Help: "Total per-function networks removed by idle reclamation",
		},
		[]string{"function_name"},
	)

	// FunctionReclaimedMemoryBytesTotal counts memory-limit bytes reclaimed.
	FunctionReclaimedMemoryBytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_reclaimed_memory_bytes_total",
			Help: "Total container memory-limit bytes reclaimed by idle reclamation",
		},
		[]string{"function_name"},
	)

	// FunctionReclaimedNanoCPUsTotal counts nano-CPU capacity reclaimed.
	FunctionReclaimedNanoCPUsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_reclaimed_nano_cpus_total",
			Help: "Total container nano-CPU capacity reclaimed by idle reclamation",
		},
		[]string{"function_name"},
	)

	// FunctionStaleGenerationsCleanedTotal counts stale-generation cleanups.
	FunctionStaleGenerationsCleanedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_stale_generations_cleaned_total",
			Help: "Total stale generation/lease entries cleaned per function",
		},
		[]string{"function_name"},
	)

	// FunctionScaleDecisionsTotal counts idle decisions by outcome.
	FunctionScaleDecisionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_scale_decisions_total",
			Help: "Total idle scale decisions per function and decision",
		},
		[]string{"function_name", "decision"},
	)

	// ActivityLeasesTotal counts activity-lease requests by result.
	ActivityLeasesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "function_activity_leases_total",
			Help: "Total activity-lease requests received by result",
		},
		[]string{"result"},
	)

	// IdleReconcilePassesTotal counts completed idle reconcile passes.
	IdleReconcilePassesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "function_idle_reconcile_passes_total",
			Help: "Total completed idle reconcile passes",
		},
	)
)

// RecordColdStart records a cold-start outcome and latency.
func RecordColdStart(functionName, result string, duration float64) {
	FunctionColdStartsTotal.WithLabelValues(functionName, result).Inc()
	if result == "success" {
		FunctionColdStartDurationSeconds.WithLabelValues(functionName).Observe(duration)
	}
}

// UpdateFunctionObservedReplicas sets the provider-observed replica gauge.
func UpdateFunctionObservedReplicas(functionName string, replicas int) {
	FunctionObservedReplicas.WithLabelValues(functionName).Set(float64(replicas))
}

// RecordIdleReclamation records an idle reclamation and the resources freed.
func RecordIdleReclamation(functionName string, containers, networks int, memoryBytes, nanoCPUs int64) {
	FunctionIdleReclamationsTotal.WithLabelValues(functionName).Inc()
	if containers > 0 {
		FunctionReclaimedContainersTotal.WithLabelValues(functionName).Add(float64(containers))
	}
	if networks > 0 {
		FunctionReclaimedNetworksTotal.WithLabelValues(functionName).Add(float64(networks))
	}
	if memoryBytes > 0 {
		FunctionReclaimedMemoryBytesTotal.WithLabelValues(functionName).Add(float64(memoryBytes))
	}
	if nanoCPUs > 0 {
		FunctionReclaimedNanoCPUsTotal.WithLabelValues(functionName).Add(float64(nanoCPUs))
	}
}

// RecordStaleGenerationsCleaned records stale generation/lease cleanup.
func RecordStaleGenerationsCleaned(functionName string, count int) {
	if count > 0 {
		FunctionStaleGenerationsCleanedTotal.WithLabelValues(functionName).Add(float64(count))
	}
}

// RecordScaleDecision records an idle decision outcome.
func RecordScaleDecision(functionName, decision string) {
	FunctionScaleDecisionsTotal.WithLabelValues(functionName, decision).Inc()
}

// RecordActivityLease records an activity-lease request result.
func RecordActivityLease(result string) {
	ActivityLeasesTotal.WithLabelValues(result).Inc()
}

// RecordIdleReconcilePass records a completed idle reconcile pass.
func RecordIdleReconcilePass() {
	IdleReconcilePassesTotal.Inc()
}

// RecordFunctionInvocation records a function invocation with duration and status
func RecordFunctionInvocation(functionName string, statusCode int, duration float64) {
	FunctionInvocationsTotal.WithLabelValues(functionName, strconv.Itoa(statusCode)).Inc()
	FunctionDurationSeconds.WithLabelValues(functionName).Observe(duration)

	if statusCode >= 400 {
		FunctionErrorsTotal.WithLabelValues(functionName).Inc()
	}
}

// RecordGatewayRestart increments the gateway restart counter.
func RecordGatewayRestart() {
	GatewayRestartsTotal.Inc()
}

// RecordGatewayRequest records a gateway HTTP request
func RecordGatewayRequest(method, path string, statusCode int, duration float64) {
	GatewayHTTPRequestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
	GatewayRequestDurationSeconds.WithLabelValues(method, path).Observe(duration)
	if statusCode >= 400 {
		GatewayHTTPErrorsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
	}
}

// RecordDBOperation records a database operation metric
func RecordDBOperation(operation string, duration float64, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	DBOperationsTotal.WithLabelValues(operation, status).Inc()
	DBOperationDurationSeconds.WithLabelValues(operation).Observe(duration)
}

// UpdateFunctionsDeployed updates the total number of deployed functions
func UpdateFunctionsDeployed(count int) {
	FunctionsDeployed.Set(float64(count))
}

// UpdateFunctionReplicas updates the replica count for a function
func UpdateFunctionReplicas(functionName string, replicas int) {
	FunctionReplicas.WithLabelValues(functionName).Set(float64(replicas))
}

// DeleteFunctionMetrics removes metrics for a deleted function
func DeleteFunctionMetrics(functionName string) {
	FunctionReplicas.DeleteLabelValues(functionName)
	FunctionObservedReplicas.DeleteLabelValues(functionName)
}
