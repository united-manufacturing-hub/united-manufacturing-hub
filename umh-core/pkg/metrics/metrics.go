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
	ComponentControlLoop     = "control_loop"
	ComponentBaseFSMManager  = "base_fsm_manager"
	ComponentS6Manager       = "s6_manager"
	ComponentBenthosManager  = "benthos_manager"
	ComponentS6Instance      = "s6_instance"
	ComponentBenthosInstance = "benthos_instance"
	ComponentS6Service       = "s6_service"
	ComponentFilesystem      = "filesystem"
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
