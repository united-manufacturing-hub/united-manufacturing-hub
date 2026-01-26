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
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"sync"
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

	// Filesystem operation metrics.
	filesystemOpsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "filesystem_ops_total",
			Help:      "Total number of filesystem operations by type and path",
		},
		[]string{"operation", "path", "cached"},
	)

	filesystemOpsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "filesystem_ops_duration_seconds",
			Help:      "Duration of filesystem operations in seconds",
			Buckets:   []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"operation", "cached"},
	)
)

// FSMv2DebugProvider provides FSMv2 introspection data for the debug endpoint.
// Implementations should return a JSON-serializable struct with supervisor state.
type FSMv2DebugProvider interface {
	GetDebugInfo() interface{}
}

// fsmv2DebugRegistry holds registered FSMv2 debug providers.
var fsmv2DebugRegistry struct {
	providers map[string]FSMv2DebugProvider
	mu        sync.RWMutex
}

// RegisterFSMv2DebugProvider registers a debug provider for the /debug/fsmv2 endpoint.
// Call this after creating an FSMv2 supervisor to expose its introspection data.
func RegisterFSMv2DebugProvider(name string, provider FSMv2DebugProvider) {
	fsmv2DebugRegistry.mu.Lock()
	defer fsmv2DebugRegistry.mu.Unlock()

	if fsmv2DebugRegistry.providers == nil {
		fsmv2DebugRegistry.providers = make(map[string]FSMv2DebugProvider)
	}

	fsmv2DebugRegistry.providers[name] = provider
}

// UnregisterFSMv2DebugProvider removes a debug provider from the registry.
// Call this when shutting down an FSMv2 supervisor.
func UnregisterFSMv2DebugProvider(name string) {
	fsmv2DebugRegistry.mu.Lock()
	defer fsmv2DebugRegistry.mu.Unlock()

	delete(fsmv2DebugRegistry.providers, name)
}

// handleFSMv2Debug handles the /debug/fsmv2 endpoint.
func handleFSMv2Debug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	fsmv2DebugRegistry.mu.RLock()
	defer fsmv2DebugRegistry.mu.RUnlock()

	if len(fsmv2DebugRegistry.providers) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"no_providers_registered","message":"No FSMv2 supervisors are registered for debugging"}`))

		return
	}

	response := make(map[string]interface{}, len(fsmv2DebugRegistry.providers))
	for name, provider := range fsmv2DebugRegistry.providers {
		response[name] = provider.GetDebugInfo()
	}

	w.Header().Set("Content-Type", "application/json")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(response); err != nil {
		http.Error(w, "Failed to encode debug info", http.StatusInternalServerError)
	}
}

// SetupMetricsEndpoint starts an HTTP server to expose metrics
// This should be called once at application startup.
func SetupMetricsEndpoint(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/fsmv2", handleFSMv2Debug)

	server := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// RecordFilesystemOp records a filesystem operation metric.
func RecordFilesystemOp(operation, path string, cached bool, duration time.Duration) {
	cachedStr := "false"
	if cached {
		cachedStr = "true"
	}

	filesystemOpsTotal.WithLabelValues(operation, path, cachedStr).Inc()
	filesystemOpsDuration.WithLabelValues(operation, cachedStr).Observe(duration.Seconds())
}
