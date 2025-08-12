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
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

const (
	// Component Labels.
	ComponentControlLoop = "control_loop"
	// Manager.
	ComponentBaseFSMManager           = "base_fsm_manager"
	ComponentS6Manager                = "s6_manager"
	ComponentBenthosManager           = "benthos_manager"
	ComponentBenthosMonitorManager    = "benthos_monitor_manager"
	ComponentRedpandaManager          = "redpanda_manager"
	ComponentNmapManager              = "nmap_manager"
	ComponentDataFlowCompManager      = "dataflow_component_manager"
	ComponentConnectionManager        = "connection_manager"
	ComponentProtocolConverterManager = "protocol_converter_manager"
	ComponentStreamProcessorManager   = "stream_processor_manager"
	ComponentTopicBrowserManager      = "topic_browser_manager"
	// Instances.
	ComponentBaseFSMInstance           = "base_fsm_instance"
	ComponentS6Instance                = "s6_instance"
	ComponentBenthosInstance           = "benthos_instance"
	ComponentRedpandaInstance          = "redpanda_instance"
	ComponentNmapInstance              = "nmap_instance"
	ComponentDataflowComponentInstance = "dataflow_component_instance"
	ComponentConnectionInstance        = "connection_instance"
	ComponentProtocolConverterInstance = "protocol_converter_instance"
	ComponentStreamProcessorInstance   = "stream_processor_instance"
	ComponentAgentMonitor              = "agent_monitor"
	ComponentBenthosMonitor            = "benthos_monitor"
	ComponentRedpandaMonitor           = "redpanda_monitor"
	ComponentTopicBrowserInstance      = "topic_browser_instance"
	// Services.
	ComponentS6Service         = "s6_service"
	ComponentBenthosService    = "benthos_service"
	ComponentRedpandaService   = "redpanda_service"
	ComponentNmapService       = "nmap_service"
	ComponentConnectionService = "connection_service"
	ComponentFilesystem        = "filesystem"
	ComponentContainerMonitor  = "container_monitor"
)

var (
	// Namespace and subsystem for all metrics.
	namespace = "umh"
	subsystem = "core"

	// Error counters.
	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors encountered by component",
		},
		[]string{"component", "instance"},
	)

	// Reconcile timing.
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

	// Starvation timer.
	starvationSeconds = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconcile_starved_total_seconds",
			Help:      "Total seconds the reconcile loop was starved",
		},
	)

	// Service state metrics.
	serviceCurrentState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "service_current_state",
			Help:      "Current state of the service (0=Stopped, 1=Starting, 2=Running, 3=Active, 4=Idle, 5=Degraded, -1=Unknown)",
		},
		[]string{"component", "instance"},
	)

	serviceDesiredState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "service_desired_state",
			Help:      "Desired state of the service (0=Stopped, 1=Starting, 2=Running, 3=Active, 4=Idle, 5=Degraded, -1=Unknown)",
		},
		[]string{"component", "instance"},
	)

	// TODO: observed state.
)

// SetupMetricsEndpoint starts an HTTP server to expose metrics
// This should be called once at application startup.
func SetupMetricsEndpoint(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris attacks
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			sentry.ReportIssue(err, sentry.IssueTypeFatal, logger.For("metrics"))
		}
	}()

	return server
}

// printDetailedStackTrace prints a detailed stack trace with more information.
func printDetailedStackTrace() {
	// Get stack trace for all goroutines with a large buffer
	buf := make([]byte, 1024*1024) // Allocate 1MB buffer
	n := runtime.Stack(buf, true)

	// Print the full stack trace
	logger.For("stacktrace").Debugf("=== DETAILED STACK TRACE ===\n%s", string(buf[:n]))
}

// IncErrorCountAndLog increments the error counter for a component and logs a debug message if a logger is provided.
func IncErrorCountAndLog(component, instance string, err error, logger *zap.SugaredLogger) {
	IncErrorCount(component, instance)

	if logger != nil {
		// Display detailed stacktrace
		printDetailedStackTrace()
		logger.Debugf("Component %s instance %s reconciliation failed: %v", component, instance, err)
	}
}

// IncErrorCount increments the error counter for a component.
func IncErrorCount(component, instance string) {
	errorCounter.WithLabelValues(component, instance).Inc()
}

// InitErrorCounter initializes the error counter for a component.
func InitErrorCounter(component, instance string) {
	errorCounter.WithLabelValues(component, instance).Add(0)
}

// ObserveReconcileTime records the time taken for a reconciliation.
func ObserveReconcileTime(component, instance string, duration time.Duration) {
	reconcileTime.WithLabelValues(component, instance).Observe(float64(duration.Milliseconds()))
}

// AddStarvationTime increases the starvation counter by the specified seconds.
func AddStarvationTime(seconds float64) {
	starvationSeconds.Add(seconds)
}

// UpdateServiceState updates the current and desired state metrics for a service.
func UpdateServiceState(component, instance string, currentState, desiredState string) {
	// Convert state strings to numeric values
	currentValue := getStateValue(currentState)
	desiredValue := getStateValue(desiredState)

	// Update the metrics
	serviceCurrentState.WithLabelValues(component, instance).Set(currentValue)
	serviceDesiredState.WithLabelValues(component, instance).Set(desiredValue)
}

// getStateValue converts a state string to a numeric value for the metric.
func getStateValue(state string) float64 {
	switch state {
	case "stopped":
		return 0
	case "starting":
		return 1
	case "running":
		return 2
	case "active":
		return 3
	case "idle":
		return 4
	case "degraded":
		return 5
	default:
		return -1 // Unknown state
	}
}
