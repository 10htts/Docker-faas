package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// GatewayHTTPRequestsTotal tracks total HTTP requests to the gateway
	GatewayHTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests to the gateway",
		},
		[]string{"method", "path", "code"},
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

// RecordGatewayRequest records a gateway HTTP request
func RecordGatewayRequest(method, path string, statusCode int) {
	GatewayHTTPRequestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
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
