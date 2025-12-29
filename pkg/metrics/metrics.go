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
)

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
}
