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

package supervisor

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
		[]string{"supervisor_id"},
	)

	infrastructureRecoveryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_total",
			Help:      "Total number of infrastructure recovery events",
		},
		[]string{"supervisor_id"},
	)

	infrastructureRecoveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "infrastructure_recovery_duration_seconds",
			Help:      "Duration of infrastructure recovery in seconds",
		},
		[]string{"supervisor_id"},
	)

	childHealthCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "child_health_check_total",
			Help:      "Total number of child health checks by status",
		},
		[]string{"supervisor_id", "child_name", "status"},
	)

	actionQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queued_total",
			Help:      "Total number of actions queued",
		},
		[]string{"supervisor_id", "action_type"},
	)

	actionQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_queue_size",
			Help:      "Current size of the action queue",
		},
		[]string{"supervisor_id"},
	)

	actionExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_execution_duration_seconds",
			Help:      "Duration of action execution in seconds",
		},
		[]string{"supervisor_id", "action_type", "status"},
	)

	actionTimeoutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "action_timeout_total",
			Help:      "Total number of action timeouts",
		},
		[]string{"supervisor_id", "action_type"},
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
		[]string{"supervisor_id"},
	)

	reconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_total",
			Help:      "Total number of reconciliation cycles",
		},
		[]string{"supervisor_id", "result"},
	)

	reconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_duration_seconds",
			Help:      "Duration of reconciliation cycles in seconds",
		},
		[]string{"supervisor_id"},
	)

	tickPropagationDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_depth",
			Help:      "Depth of tick propagation in supervisor hierarchy",
		},
		[]string{"supervisor_id"},
	)

	tickPropagationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tick_propagation_duration_seconds",
			Help:      "Duration of tick propagation in seconds",
		},
		[]string{"supervisor_id"},
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

func RecordChildHealthCheck(supervisorID, childName, status string) {
	childHealthCheckTotal.WithLabelValues(supervisorID, childName, status).Inc()
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
