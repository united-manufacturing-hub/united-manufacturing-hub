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
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// Component Labels
	ComponentControlLoop         = "control_loop"
	ComponentBaseFSMManager      = "base_fsm_manager"
	ComponentS6Manager           = "s6_manager"
	ComponentBenthosManager      = "benthos_manager"
	ComponentRedpandaManager     = "redpanda_manager"
	ComponentDataFlowCompManager = "dataflow_component_manager"
	ComponentS6Instance          = "s6_instance"
	ComponentBenthosInstance     = "benthos_instance"
	ComponentRedpandaInstance    = "redpanda_instance"
	ComponentS6Service           = "s6_service"
	ComponentBenthosService      = "benthos_service"
	ComponentRedpandaService     = "redpanda_service"
	ComponentFilesystem          = "filesystem"
	ComponentContainerMonitor    = "container_monitor"
  ComponentDataflowComponentInstance = "dataflow_component_instance"
)

var (
	// Namespace and subsystem for all metrics
	namespace = "umh"
	subsystem = "core"

	// Error counters
	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors encountered by component",
		},
		[]string{"component", "instance"},
	)

	// Reconcile timing
	reconcileTime = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconcile_duration_milliseconds",
			Help:      "Time taken to reconcile (in milliseconds)",
			Objectives: map[float64]float64{
				0.5:  0.01, // 50th percentile with 1% error
				0.9:  0.01, // 90th percentile with 1% error
				0.95: 0.01, // 95th percentile with 1% error
				0.99: 0.01, // 99th percentile with 1% error
			},
		},
		[]string{"component", "instance"},
	)

	// Starvation timer
	starvationSeconds = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconcile_starved_total_seconds",
			Help:      "Total seconds the reconcile loop was starved",
		},
	)

	// TODO: observed state
)

// SetupMetricsEndpoint starts an HTTP server to expose metrics
// This should be called once at application startup
func SetupMetricsEndpoint(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return server
}

// IncErrorCount increments the error counter for a component
func IncErrorCount(component, instance string) {
	errorCounter.WithLabelValues(component, instance).Inc()
}

// InitErrorCounter initializes the error counter for a component
func InitErrorCounter(component, instance string) {
	errorCounter.WithLabelValues(component, instance).Add(0)
}

// ObserveReconcileTime records the time taken for a reconciliation
func ObserveReconcileTime(component, instance string, duration time.Duration) {
	reconcileTime.WithLabelValues(component, instance).Observe(float64(duration.Milliseconds()))
}

// AddStarvationTime increases the starvation counter by the specified seconds
func AddStarvationTime(seconds float64) {
	starvationSeconds.Add(seconds)
}
