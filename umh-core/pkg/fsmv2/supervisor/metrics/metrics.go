// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics provides Prometheus metrics for FSMv2 supervisors.
//
// # Label Convention: hierarchy_path
//
// All metrics use "hierarchy_path" as the primary identification label.
// Format: "workerID(workerType)/childID(childType)/..."
// Example: "application-main(application)/communicator-001(communicator)"
//
// This provides:
//   - Full hierarchical context in a single label
//   - Consistency with logging (same field used everywhere)
//   - Both instance ID and type embedded in the path
//   - Easy filtering by depth or subtree using regex
//
// # Cardinality Considerations
//
// High cardinality mitigation strategies:
//   - hierarchy_path cardinality is O(hierarchy_depth) not O(workers)
//   - Configure Prometheus retention and scrape intervals appropriately
//   - Use recording rules to aggregate metrics by worker type
//   - Monitor _metric_count and _cardinality_total in Prometheus
//
// For aggregation by worker type, use regex: {hierarchy_path=~".*\\(application\\).*"}
package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var (
	namespace = "umh"
	subsystem = "fsmv2"

	circuitOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "circuit_open",
			Help:      "Circuit breaker state (0=closed, 1=open)",
		},
		[]string{"hierarchy_path"},
	)

	infrastructureRecoveryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_total",
			Help:      "Total number of infrastructure recovery events",
		},
		[]string{"hierarchy_path"},
	)

	infrastructureRecoveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_duration_seconds",
			Help:      "Duration of infrastructure recovery in seconds",
		},
		[]string{"hierarchy_path"},
	)

	actionQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queued_total",
			Help:      "Total number of actions queued",
		},
		[]string{"hierarchy_path", "action_type"},
	)

	actionQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queue_size",
			Help:      "Current size of the action queue",
		},
		[]string{"hierarchy_path"},
	)

	actionExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_execution_duration_seconds",
			Help:      "Duration of action execution in seconds",
		},
		[]string{"hierarchy_path", "action_type", "status"},
	)

	actionTimeoutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_timeout_total",
			Help:      "Total number of action timeouts",
		},
		[]string{"hierarchy_path", "action_type"},
	)

	workerPoolUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "worker_pool_utilization",
			Help:      "Worker pool utilization (0.0 to 1.0)",
		},
		[]string{"pool_name"},
	)

	workerPoolQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "worker_pool_queue_size",
			Help:      "Current size of the worker pool queue",
		},
		[]string{"pool_name"},
	)

	childCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "child_count",
			Help:      "Current number of child supervisors",
		},
		[]string{"hierarchy_path"},
	)

	reconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_total",
			Help:      "Total number of reconciliation cycles",
		},
		[]string{"hierarchy_path", "result"},
	)

	reconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_duration_seconds",
			Help:      "Duration of reconciliation cycles in seconds",
		},
		[]string{"hierarchy_path"},
	)

	tickPropagationDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_depth",
			Help:      "Depth of tick propagation in supervisor hierarchy",
		},
		[]string{"hierarchy_path"},
	)

	tickPropagationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_duration_seconds",
			Help:      "Duration of tick propagation in seconds",
		},
		[]string{"hierarchy_path"},
	)

	templateRenderingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "template_rendering_duration_seconds",
			Help:      "Duration of template rendering in seconds",
		},
		[]string{"hierarchy_path", "status"},
	)

	templateRenderingErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "template_rendering_errors_total",
			Help:      "Total number of template rendering errors",
		},
		[]string{"hierarchy_path", "error_type"},
	)

	variablePropagationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "variable_propagation_total",
			Help:      "Total number of variable propagation events",
		},
		[]string{"hierarchy_path"},
	)

	hierarchyDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "hierarchy_depth",
			Help:      "Depth of supervisor in hierarchy tree (0=root, 1=child, 2=grandchild, etc.)",
		},
		[]string{"hierarchy_path"},
	)

	hierarchySize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "hierarchy_size",
			Help:      "Total number of supervisors in subtree (self + all descendants)",
		},
		[]string{"hierarchy_path"},
	)

	stateTransitionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "state_transitions_total",
			Help:      "Total state transitions by from_state and to_state",
		},
		[]string{"hierarchy_path", "from_state", "to_state"},
	)

	// stateDurationSeconds tracks time in current state per worker.
	// Uses hierarchy_path which provides both instance identity and hierarchy context.
	// Cleanup via CleanupStateDuration() when workers are removed.
	stateDurationSeconds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "state_duration_seconds",
			Help:      "Time spent in current state",
		},
		[]string{"hierarchy_path", "state"},
	)

	observationSaveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "observation_save_total",
			Help:      "Total number of observation save attempts",
		},
		[]string{"hierarchy_path", "changed"},
	)

	observationSaveDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "observation_save_duration_seconds",
			Help:      "Duration of observation save operations (including delta checking)",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"hierarchy_path", "changed"},
	)
)

// RecordCircuitOpen records circuit breaker state.
func RecordCircuitOpen(hierarchyPath string, open bool) {
	value := 0.0
	if open {
		value = 1.0
	}

	circuitOpen.WithLabelValues(hierarchyPath).Set(value)
}

// RecordInfrastructureRecovery records an infrastructure recovery event.
func RecordInfrastructureRecovery(hierarchyPath string, duration time.Duration) {
	infrastructureRecoveryTotal.WithLabelValues(hierarchyPath).Inc()
	infrastructureRecoveryDuration.WithLabelValues(hierarchyPath).Observe(duration.Seconds())
}

// RecordActionQueued records an action being queued.
func RecordActionQueued(hierarchyPath, actionType string) {
	actionQueuedTotal.WithLabelValues(hierarchyPath, actionType).Inc()
}

// RecordActionQueueSize records the current action queue size.
func RecordActionQueueSize(hierarchyPath string, size int) {
	actionQueueSize.WithLabelValues(hierarchyPath).Set(float64(size))
}

// RecordActionExecutionDuration records action execution duration.
func RecordActionExecutionDuration(hierarchyPath, actionType, status string, duration time.Duration) {
	actionExecutionDuration.WithLabelValues(hierarchyPath, actionType, status).Observe(duration.Seconds())
}

// RecordActionTimeout records an action timeout.
func RecordActionTimeout(hierarchyPath, actionType string) {
	actionTimeoutTotal.WithLabelValues(hierarchyPath, actionType).Inc()
}

// RecordWorkerPoolUtilization records worker pool utilization.
func RecordWorkerPoolUtilization(poolName string, utilization float64) {
	workerPoolUtilization.WithLabelValues(poolName).Set(utilization)
}

// RecordWorkerPoolQueueSize records worker pool queue size.
func RecordWorkerPoolQueueSize(poolName string, size int) {
	workerPoolQueueSize.WithLabelValues(poolName).Set(float64(size))
}

// RecordChildCount records the number of child supervisors.
func RecordChildCount(hierarchyPath string, count int) {
	childCount.WithLabelValues(hierarchyPath).Set(float64(count))
}

// RecordReconciliation records a reconciliation cycle.
func RecordReconciliation(hierarchyPath, result string, duration time.Duration) {
	reconciliationTotal.WithLabelValues(hierarchyPath, result).Inc()
	reconciliationDuration.WithLabelValues(hierarchyPath).Observe(duration.Seconds())
}

// RecordTickPropagationDepth records tick propagation depth.
func RecordTickPropagationDepth(hierarchyPath string, depth int) {
	tickPropagationDepth.WithLabelValues(hierarchyPath).Set(float64(depth))
}

// RecordTickPropagationDuration records tick propagation duration.
func RecordTickPropagationDuration(hierarchyPath string, duration time.Duration) {
	tickPropagationDuration.WithLabelValues(hierarchyPath).Observe(duration.Seconds())
}

// RecordTemplateRenderingDuration records template rendering duration.
func RecordTemplateRenderingDuration(hierarchyPath, status string, duration time.Duration) {
	templateRenderingDuration.WithLabelValues(hierarchyPath, status).Observe(duration.Seconds())
}

// RecordTemplateRenderingError records a template rendering error.
func RecordTemplateRenderingError(hierarchyPath, errorType string) {
	templateRenderingErrorsTotal.WithLabelValues(hierarchyPath, errorType).Inc()
}

// RecordVariablePropagation records a variable propagation event.
func RecordVariablePropagation(hierarchyPath string) {
	variablePropagationTotal.WithLabelValues(hierarchyPath).Inc()
}

// RecordHierarchyDepth records the depth in the supervisor hierarchy.
func RecordHierarchyDepth(hierarchyPath string, depth int) {
	hierarchyDepth.WithLabelValues(hierarchyPath).Set(float64(depth))
}

// RecordHierarchySize records the size of the supervisor subtree.
func RecordHierarchySize(hierarchyPath string, size int) {
	hierarchySize.WithLabelValues(hierarchyPath).Set(float64(size))
}

// RecordObservationSave records an observation save operation.
func RecordObservationSave(hierarchyPath string, changed bool, duration time.Duration) {
	changedStr := "false"
	if changed {
		changedStr = "true"
	}

	observationSaveTotal.WithLabelValues(hierarchyPath, changedStr).Inc()
	observationSaveDuration.WithLabelValues(hierarchyPath, changedStr).Observe(duration.Seconds())
}

// RecordStateTransition records a state transition event.
func RecordStateTransition(hierarchyPath, fromState, toState string) {
	stateTransitionsTotal.WithLabelValues(hierarchyPath, fromState, toState).Inc()
}

// RecordStateDuration records how long a worker has been in its current state.
func RecordStateDuration(hierarchyPath, state string, duration time.Duration) {
	stateDurationSeconds.WithLabelValues(hierarchyPath, state).Set(duration.Seconds())
}

// CleanupStateDuration removes state duration metric for a worker.
func CleanupStateDuration(hierarchyPath, state string) {
	stateDurationSeconds.DeleteLabelValues(hierarchyPath, state)
}

// WorkerMetricsExporter exports worker-specific metrics from ObservedState to Prometheus.
// Tracks previous counter values to compute deltas.
//
// Uses hierarchy_path label for consistent identification across the codebase.
type WorkerMetricsExporter struct {
	counters     map[string]*prometheus.CounterVec
	gauges       map[string]*prometheus.GaugeVec
	prevCounters map[string]int64 // Key: "hierarchyPath:metricName"
	mu           sync.Mutex
}

var workerMetricsExporter = &WorkerMetricsExporter{
	counters:     make(map[string]*prometheus.CounterVec),
	gauges:       make(map[string]*prometheus.GaugeVec),
	prevCounters: make(map[string]int64),
}

// ExportWorkerMetrics exports metrics from ObservedState to Prometheus.
// No-op if observed does not implement MetricsHolder.
func ExportWorkerMetrics(hierarchyPath string, observed fsmv2.ObservedState) {
	holder, ok := observed.(deps.MetricsHolder)
	if !ok {
		return
	}

	metrics := holder.GetWorkerMetrics()
	if metrics.Counters == nil && metrics.Gauges == nil {
		return
	}

	workerMetricsExporter.export(hierarchyPath, &metrics)
}

func (e *WorkerMetricsExporter) export(hierarchyPath string, metrics *deps.Metrics) {
	e.mu.Lock()
	defer e.mu.Unlock()

	labels := prometheus.Labels{
		"hierarchy_path": hierarchyPath,
	}

	for name, value := range metrics.Counters {
		prevKey := hierarchyPath + ":" + name
		prevValue := e.prevCounters[prevKey]

		delta := value - prevValue
		if delta > 0 {
			counter := e.getOrCreateCounter(name)
			counter.With(labels).Add(float64(delta))
		}

		e.prevCounters[prevKey] = value
	}

	for name, value := range metrics.Gauges {
		gauge := e.getOrCreateGauge(name)
		gauge.With(labels).Set(value)
	}
}

func (e *WorkerMetricsExporter) getOrCreateCounter(name string) *prometheus.CounterVec {
	if counter, exists := e.counters[name]; exists {
		return counter
	}

	counter := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem + "_worker",
			Name:      name,
			Help:      "Worker metric: " + name,
		},
		[]string{"hierarchy_path"},
	)
	e.counters[name] = counter

	return counter
}

func (e *WorkerMetricsExporter) getOrCreateGauge(name string) *prometheus.GaugeVec {
	if gauge, exists := e.gauges[name]; exists {
		return gauge
	}

	gauge := promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem + "_worker",
			Name:      name,
			Help:      "Worker metric: " + name,
		},
		[]string{"hierarchy_path"},
	)
	e.gauges[name] = gauge

	return gauge
}

// GetHierarchyDepthGauge returns the hierarchy depth gauge for testing.
func GetHierarchyDepthGauge() *prometheus.GaugeVec {
	return hierarchyDepth
}

// GetHierarchySizeGauge returns the hierarchy size gauge for testing.
func GetHierarchySizeGauge() *prometheus.GaugeVec {
	return hierarchySize
}
