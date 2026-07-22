package scaletozero

import "github.com/docker-faas/docker-faas/pkg/metrics"

// PrometheusMetrics adapts the idle reconciler's MetricsSink to the process-wide
// Prometheus recorders in pkg/metrics. It carries no state.
type PrometheusMetrics struct{}

var _ MetricsSink = PrometheusMetrics{}

// ObservedReplicas sets the provider-observed replica gauge.
func (PrometheusMetrics) ObservedReplicas(fn string, replicas int) {
	metrics.UpdateFunctionObservedReplicas(fn, replicas)
}

// Decision records the idle decision outcome.
func (PrometheusMetrics) Decision(fn string, action Action) {
	metrics.RecordScaleDecision(fn, action.String())
}

// IdleReclamation records a reclamation and the resources it freed (SZ-09).
func (PrometheusMetrics) IdleReclamation(fn string, report ReclaimReport) {
	metrics.RecordIdleReclamation(fn, report.ContainersRemoved, report.NetworksRemoved, report.MemoryBytesFreed, report.NanoCPUsFreed)
}

// StaleGenerationsCleaned records stale generation/lease cleanup.
func (PrometheusMetrics) StaleGenerationsCleaned(fn string, count int) {
	metrics.RecordStaleGenerationsCleaned(fn, count)
}

// ReconcilePass records a completed reconcile pass.
func (PrometheusMetrics) ReconcilePass(reclaimed, held, keptWarm, orphans, skipped int) {
	metrics.RecordIdleReconcilePass()
}
