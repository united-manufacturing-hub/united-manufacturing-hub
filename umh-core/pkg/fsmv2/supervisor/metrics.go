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
