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
		[]string{"worker_type"},
	)

	infrastructureRecoveryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_total",
			Help:      "Total number of infrastructure recovery events",
		},
		[]string{"worker_type"},
	)

	infrastructureRecoveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_duration_seconds",
			Help:      "Duration of infrastructure recovery in seconds",
		},
		[]string{"worker_type"},
	)

	actionQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queued_total",
			Help:      "Total number of actions queued",
		},
		[]string{"worker_type", "action_type"},
	)

	actionQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queue_size",
			Help:      "Current size of the action queue",
		},
		[]string{"worker_type"},
	)

	actionExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_execution_duration_seconds",
			Help:      "Duration of action execution in seconds",
		},
		[]string{"worker_type", "action_type", "status"},
	)

	actionTimeoutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_timeout_total",
			Help:      "Total number of action timeouts",
		},
		[]string{"worker_type", "action_type"},
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
		[]string{"worker_type"},
	)

	reconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_total",
			Help:      "Total number of reconciliation cycles",
		},
		[]string{"worker_type", "result"},
	)

	reconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_duration_seconds",
			Help:      "Duration of reconciliation cycles in seconds",
		},
		[]string{"worker_type"},
	)

	tickPropagationDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_depth",
			Help:      "Depth of tick propagation in supervisor hierarchy",
		},
		[]string{"worker_type"},
	)

	tickPropagationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_duration_seconds",
			Help:      "Duration of tick propagation in seconds",
		},
		[]string{"worker_type"},
	)

	templateRenderingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "template_rendering_duration_seconds",
			Help:      "Duration of template rendering in seconds",
		},
		[]string{"worker_type", "status"},
	)

	templateRenderingErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "template_rendering_errors_total",
			Help:      "Total number of template rendering errors",
		},
		[]string{"worker_type", "error_type"},
	)

	variablePropagationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "variable_propagation_total",
			Help:      "Total number of variable propagation events",
		},
		[]string{"worker_type"},
	)

	hierarchyDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "hierarchy_depth",
			Help:      "Depth of supervisor in hierarchy tree (0=root, 1=child, 2=grandchild, etc.)",
		},
		[]string{"worker_type"},
	)

	hierarchySize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "hierarchy_size",
			Help:      "Total number of supervisors in subtree (self + all descendants)",
		},
		[]string{"worker_type"},
	)

	stateTransitionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "state_transitions_total",
			Help:      "Total state transitions by from_state and to_state",
		},
		[]string{"worker_type", "from_state", "to_state"},
	)

	stateDurationSeconds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "state_duration_seconds",
			Help:      "Time spent in current state",
		},
		[]string{"worker_type", "worker_id", "state"},
	)

	observationSaveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "observation_save_total",
			Help:      "Total number of observation save attempts",
		},
		[]string{"worker_type", "changed"},
	)

	observationSaveDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "observation_save_duration_seconds",
			Help:      "Duration of observation save operations (including delta checking)",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"worker_type", "changed"},
	)
)

func RecordCircuitOpen(supervisorID string, open bool) {
	value := 0.0
	if open {
		value = 1.0
	}

	circuitOpen.WithLabelValues(supervisorID).Set(value)
}

func RecordInfrastructureRecovery(supervisorID string, duration time.Duration) {
	infrastructureRecoveryTotal.WithLabelValues(supervisorID).Inc()
	infrastructureRecoveryDuration.WithLabelValues(supervisorID).Observe(duration.Seconds())
}

func RecordActionQueued(supervisorID, actionType string) {
	actionQueuedTotal.WithLabelValues(supervisorID, actionType).Inc()
}

func RecordActionQueueSize(supervisorID string, size int) {
	actionQueueSize.WithLabelValues(supervisorID).Set(float64(size))
}

func RecordActionExecutionDuration(supervisorID, actionType, status string, duration time.Duration) {
	actionExecutionDuration.WithLabelValues(supervisorID, actionType, status).Observe(duration.Seconds())
}

func RecordActionTimeout(supervisorID, actionType string) {
	actionTimeoutTotal.WithLabelValues(supervisorID, actionType).Inc()
}

func RecordWorkerPoolUtilization(poolName string, utilization float64) {
	workerPoolUtilization.WithLabelValues(poolName).Set(utilization)
}

func RecordWorkerPoolQueueSize(poolName string, size int) {
	workerPoolQueueSize.WithLabelValues(poolName).Set(float64(size))
}

func RecordChildCount(supervisorID string, count int) {
	childCount.WithLabelValues(supervisorID).Set(float64(count))
}

func RecordReconciliation(supervisorID, result string, duration time.Duration) {
	reconciliationTotal.WithLabelValues(supervisorID, result).Inc()
	reconciliationDuration.WithLabelValues(supervisorID).Observe(duration.Seconds())
}

func RecordTickPropagationDepth(supervisorID string, depth int) {
	tickPropagationDepth.WithLabelValues(supervisorID).Set(float64(depth))
}

func RecordTickPropagationDuration(supervisorID string, duration time.Duration) {
	tickPropagationDuration.WithLabelValues(supervisorID).Observe(duration.Seconds())
}

func RecordTemplateRenderingDuration(supervisorID, status string, duration time.Duration) {
	templateRenderingDuration.WithLabelValues(supervisorID, status).Observe(duration.Seconds())
}

func RecordTemplateRenderingError(supervisorID, errorType string) {
	templateRenderingErrorsTotal.WithLabelValues(supervisorID, errorType).Inc()
}

func RecordVariablePropagation(supervisorID string) {
	variablePropagationTotal.WithLabelValues(supervisorID).Inc()
}

func RecordHierarchyDepth(supervisorID string, depth int) {
	hierarchyDepth.WithLabelValues(supervisorID).Set(float64(depth))
}

func RecordHierarchySize(supervisorID string, size int) {
	hierarchySize.WithLabelValues(supervisorID).Set(float64(size))
}

func RecordObservationSave(workerType string, changed bool, duration time.Duration) {
	changedStr := "false"
	if changed {
		changedStr = "true"
	}

	observationSaveTotal.WithLabelValues(workerType, changedStr).Inc()
	observationSaveDuration.WithLabelValues(workerType, changedStr).Observe(duration.Seconds())
}

// RecordStateTransition records a state transition event.
func RecordStateTransition(workerType, fromState, toState string) {
	stateTransitionsTotal.WithLabelValues(workerType, fromState, toState).Inc()
}

// RecordStateDuration records how long a worker has been in its current state.
func RecordStateDuration(workerType, workerID, state string, duration time.Duration) {
	stateDurationSeconds.WithLabelValues(workerType, workerID, state).Set(duration.Seconds())
}

// CleanupStateDuration removes state duration metric for a worker.
func CleanupStateDuration(workerType, workerID, state string) {
	stateDurationSeconds.DeleteLabelValues(workerType, workerID, state)
}

// WorkerMetricsExporter exports worker-specific metrics from ObservedState to Prometheus.
// Tracks previous counter values to compute deltas.
type WorkerMetricsExporter struct {

	counters     map[string]*prometheus.CounterVec
	gauges       map[string]*prometheus.GaugeVec
	prevCounters map[string]int64 // Key: "workerType:workerID:metricName"
	mu           sync.Mutex
}

var workerMetricsExporter = &WorkerMetricsExporter{
	counters:     make(map[string]*prometheus.CounterVec),
	gauges:       make(map[string]*prometheus.GaugeVec),
	prevCounters: make(map[string]int64),
}

// ExportWorkerMetrics exports metrics from ObservedState to Prometheus.
// No-op if observed does not implement MetricsHolder.
func ExportWorkerMetrics(workerType, workerID string, observed fsmv2.ObservedState) {
	holder, ok := observed.(deps.MetricsHolder)
	if !ok {
		return
	}

	metrics := holder.GetWorkerMetrics()
	if metrics.Counters == nil && metrics.Gauges == nil {
		return
	}

	workerMetricsExporter.export(workerType, workerID, &metrics)
}

func (e *WorkerMetricsExporter) export(workerType, workerID string, metrics *deps.Metrics) {
	e.mu.Lock()
	defer e.mu.Unlock()

	labels := prometheus.Labels{
		"worker_type": workerType,
		"worker_id":   workerID,
	}

	for name, value := range metrics.Counters {
		prevKey := workerType + ":" + workerID + ":" + name
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
		[]string{"worker_type", "worker_id"},
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
		[]string{"worker_type", "worker_id"},
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
