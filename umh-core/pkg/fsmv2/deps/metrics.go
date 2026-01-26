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

package deps

import (
	"sync"
)

// =============================================================================
// METRIC NAME TYPES (Type-Safe Constants)
// =============================================================================

// CounterName is a type-safe metric name for counters (cumulative, only increase).
type CounterName string

// GaugeName is a type-safe metric name for gauges (point-in-time, can go up or down).
type GaugeName string

// Generic worker counter names for common operations.
const (
	// CounterStateTransitions tracks total state transitions.
	CounterStateTransitions CounterName = "state_transitions"

	// CounterActionExecutions tracks total action executions.
	CounterActionExecutions CounterName = "action_executions"

	// CounterActionErrors tracks total action execution errors.
	CounterActionErrors CounterName = "action_errors"
)

// Generic worker gauge names for common monitoring.
const (
	// GaugeTimeInCurrentStateMs tracks time spent in the current state in milliseconds.
	// NOTE: Also available via FrameworkMetrics.TimeInCurrentStateMs (supervisor-injected).
	GaugeTimeInCurrentStateMs GaugeName = "time_in_current_state_ms"
)

// Framework metric names (supervisor-provided, for Prometheus export consistency).
// Actual values are in FrameworkMetrics struct fields, not map-based metrics.
const (
	// GaugeStateEnteredAtUnix tracks when the current state was entered (Unix timestamp).
	GaugeStateEnteredAtUnix GaugeName = "state_entered_at_unix"

	// CounterStateTransitionsTotal tracks total state transitions this session.
	// Different from CounterStateTransitions which is the per-tick counter.
	CounterStateTransitionsTotal CounterName = "state_transitions_total"

	// CounterCollectorRestarts tracks per-worker collector restart count.
	CounterCollectorRestarts CounterName = "collector_restarts"

	// CounterStartupCount tracks how many times this worker has been started (persistent).
	// This value survives restarts - loaded from CSE and incremented on each AddWorker().
	CounterStartupCount CounterName = "startup_count"
)

// Communicator worker counter names for bidirectional sync.
const (
	// CounterPullOps tracks total pull operations attempted.
	CounterPullOps CounterName = "pull_ops"

	// CounterPullSuccess tracks successful pull operations.
	CounterPullSuccess CounterName = "pull_success"

	// CounterPullFailures tracks failed pull operations.
	CounterPullFailures CounterName = "pull_failures"

	// CounterMessagesPulled tracks total messages pulled from backend.
	CounterMessagesPulled CounterName = "messages_pulled"

	// CounterPushOps tracks total push operations attempted.
	CounterPushOps CounterName = "push_ops"

	// CounterPushSuccess tracks successful push operations.
	CounterPushSuccess CounterName = "push_success"

	// CounterPushFailures tracks failed push operations.
	CounterPushFailures CounterName = "push_failures"

	// CounterMessagesPushed tracks total messages pushed to backend.
	CounterMessagesPushed CounterName = "messages_pushed"

	// CounterBytesPulled tracks total bytes pulled from backend.
	CounterBytesPulled CounterName = "bytes_pulled"

	// CounterBytesPushed tracks total bytes pushed to backend.
	CounterBytesPushed CounterName = "bytes_pushed"
)

// Error type counter names for intelligent backoff and monitoring.
const (
	// CounterAuthFailuresTotal tracks authentication failures (401/403).
	CounterAuthFailuresTotal CounterName = "auth_failures_total"

	// CounterServerErrorsTotal tracks server errors (5xx).
	CounterServerErrorsTotal CounterName = "server_errors_total"

	// CounterNetworkErrorsTotal tracks network/connection errors.
	CounterNetworkErrorsTotal CounterName = "network_errors_total"

	// CounterCloudflareErrorsTotal tracks Cloudflare challenge responses (429 + HTML).
	CounterCloudflareErrorsTotal CounterName = "cloudflare_errors_total"

	// CounterProxyBlockErrorsTotal tracks proxy block responses (Zscaler, etc.).
	CounterProxyBlockErrorsTotal CounterName = "proxy_block_errors_total"

	// CounterBackendRateLimitErrorsTotal tracks backend rate limit errors (429 + JSON).
	CounterBackendRateLimitErrorsTotal CounterName = "backend_rate_limit_errors_total"

	// CounterInstanceDeletedTotal tracks instance deleted errors (404).
	CounterInstanceDeletedTotal CounterName = "instance_deleted_total"
)

// Communicator worker gauge names for monitoring.
const (
	// GaugeLastPullLatencyMs tracks the latency of the most recent pull operation in milliseconds.
	GaugeLastPullLatencyMs GaugeName = "last_pull_latency_ms"

	// GaugeLastPushLatencyMs tracks the latency of the most recent push operation in milliseconds.
	GaugeLastPushLatencyMs GaugeName = "last_push_latency_ms"

	// GaugeConsecutiveErrors tracks the number of consecutive errors.
	// Resets to 0 on successful operation.
	GaugeConsecutiveErrors GaugeName = "consecutive_errors"
)

// =============================================================================
// METRICS INFRASTRUCTURE
// =============================================================================

// Metrics is the standard metrics container for all FSMv2 workers.
// Workers set values here via MetricsRecorder, supervisor automatically exports to Prometheus.
//
// Design principle: Single source of truth. Metrics are stored in ObservedState
// (persisted to CSE) and derived for Prometheus export. No duplication.
//
// Counters are cumulative values. Supervisor computes deltas for Prometheus.
// Gauges are point-in-time values. Exported directly to Prometheus.
type Metrics struct {
	// Counters are cumulative values that only increase.
	// Examples: "pull_ops", "messages_pulled", "errors_total"
	// The supervisor computes deltas between observations for Prometheus counters.
	Counters map[string]int64 `json:"counters,omitempty"`

	// Gauges are point-in-time values that can increase or decrease.
	// Examples: "consecutive_errors", "queue_depth", "last_pull_latency_ms"
	// These are exported directly to Prometheus gauges.
	Gauges map[string]float64 `json:"gauges,omitempty"`
}

// NewMetrics creates an initialized Metrics struct with empty maps.
func NewMetrics() Metrics {
	return Metrics{
		Counters: make(map[string]int64),
		Gauges:   make(map[string]float64),
	}
}

// MetricsHolder is implemented by ObservedState types that have metrics.
// Supervisor uses this to automatically export metrics to Prometheus.
//
// Workers embed MetricsEmbedder which provides value receiver implementations.
// Value receivers are required because ObservedState flows as values through
// interfaces, and pointer receiver methods are NOT in the method set of values.
//
// Usage:
//
//	holder, ok := observed.(deps.MetricsHolder)
//	if ok {
//	    workerMetrics := holder.GetWorkerMetrics()
//	    frameworkMetrics := holder.GetFrameworkMetrics()
//	}
type MetricsHolder interface {
	GetWorkerMetrics() Metrics
	GetFrameworkMetrics() FrameworkMetrics
}

// MetricsRecorder buffers per-tick metric updates from actions.
// Actions call IncrementCounter/SetGauge during Execute().
// CollectObservedState calls Drain() to merge buffered metrics into ObservedState.Metrics.
//
// Thread-safety: Uses sync.Mutex for all operations.
// Actions may run concurrently with collection, so locking is required.
//
// Lifecycle:
//  1. Action calls IncrementCounter/SetGauge (buffered in recorder)
//  2. CollectObservedState calls Drain() (returns buffer, clears for next tick)
//  3. CollectObservedState merges drained values into cumulative Metrics
//  4. Metrics persisted to CSE as part of ObservedState
type MetricsRecorder struct {
	counters map[string]int64
	gauges   map[string]float64
	mu       sync.Mutex
}

// NewMetricsRecorder creates a MetricsRecorder ready for use.
func NewMetricsRecorder() *MetricsRecorder {
	return &MetricsRecorder{
		counters: make(map[string]int64),
		gauges:   make(map[string]float64),
	}
}

// IncrementCounter adds delta to a counter.
// Uses typed CounterName for compile-time safety against typos.
//
// Example:
//
//	deps.Metrics().IncrementCounter(deps.CounterMessagesPulled, int64(len(messages)))
func (r *MetricsRecorder) IncrementCounter(name CounterName, delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counters[string(name)] += delta
}

// SetGauge sets a gauge to the specified value.
// Uses typed GaugeName for compile-time safety against typos.
//
// Example:
//
//	deps.Metrics().SetGauge(deps.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
func (r *MetricsRecorder) SetGauge(name GaugeName, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gauges[string(name)] = value
}

// DrainResult holds the buffered metrics from a Drain() operation.
type DrainResult struct {
	// Counters contains the delta values accumulated since last drain.
	Counters map[string]int64
	// Gauges contains the most recent gauge values.
	Gauges map[string]float64
}

// Drain returns buffered metrics and resets the buffer for the next tick.
// Called by CollectObservedState to merge per-tick metrics into cumulative state.
//
// Returns a DrainResult containing:
//   - Counters: Delta values to ADD to cumulative counters
//   - Gauges: Current values to SET as gauge values
func (r *MetricsRecorder) Drain() DrainResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := DrainResult{
		Counters: r.counters,
		Gauges:   r.gauges,
	}

	r.counters = make(map[string]int64)
	r.gauges = make(map[string]float64)

	return result
}

// =============================================================================
// FRAMEWORK METRICS (Supervisor-Provided)
// =============================================================================

// FrameworkMetrics contains metrics automatically provided by the supervisor.
// Workers don't need to implement anything - these are always available.
// The supervisor injects these values into ObservedState before calling State.Next().
//
// THREE CATEGORIES:
//   - Session Metrics: Reset on restart (TimeInCurrentStateMs, TransitionsByState, etc.)
//   - Persistent Counters: Survive restart (StartupCount)
//
// Workers access these in State.Next() via:
//
//	fm := snap.Observed.GetFrameworkMetrics()
//	timeInState := time.Duration(fm.TimeInCurrentStateMs) * time.Millisecond
//	startupCount := fm.StartupCount  // Persists across restarts
type FrameworkMetrics struct {
	// === Session Metrics (reset on restart) ===

	// TransitionsByState maps state names to the number of times that state was entered.
	// Example: {"running": 5, "degraded": 2, "stopped": 1}
	TransitionsByState map[string]int64 `json:"transitions_by_state,omitempty"`

	// CumulativeTimeByStateMs maps state names to total milliseconds spent in that state.
	// Example: {"running": 60000, "degraded": 5000}
	CumulativeTimeByStateMs map[string]int64 `json:"cumulative_time_by_state_ms,omitempty"`

	// === State Information ===

	// StateReason is a human-readable explanation for the current state.
	// Set by the supervisor during state transitions from NextResult.Reason.
	// Useful for understanding WHY the worker is in its current state.
	StateReason string `json:"state_reason,omitempty"`

	// TimeInCurrentStateMs is milliseconds since the current state was entered.
	// Updated each tick by supervisor.
	TimeInCurrentStateMs int64 `json:"time_in_current_state_ms"`

	// StateEnteredAtUnix is Unix timestamp (seconds) when current state was entered.
	StateEnteredAtUnix int64 `json:"state_entered_at_unix"`

	// StateTransitionsTotal is the total number of state transitions this session.
	StateTransitionsTotal int64 `json:"state_transitions_total"`

	// CollectorRestarts is the number of times the observation collector was restarted
	// for this specific worker (not global). Reset on worker restart.
	CollectorRestarts int64 `json:"collector_restarts"`

	// === Persistent Counters (survive restart) ===

	// StartupCount is the number of times this worker has been started.
	// Loaded from CSE on AddWorker(), incremented, and persisted.
	// Use this to detect restart loops or track worker lifetime.
	StartupCount int64 `json:"startup_count"`
}

// NewFrameworkMetrics creates an initialized FrameworkMetrics struct with empty maps.
func NewFrameworkMetrics() FrameworkMetrics {
	return FrameworkMetrics{
		TransitionsByState:      make(map[string]int64),
		CumulativeTimeByStateMs: make(map[string]int64),
	}
}

// MetricsContainer groups framework and worker metrics together.
// Named type (not anonymous struct) for testability, godoc, and error messages.
//
// JSON structure: {"framework":{...},"worker":{...}}.
// Access pattern: snap.Observed.Metrics.Framework.TimeInCurrentStateMs.
type MetricsContainer struct {
	// Worker contains worker-defined metrics (recorded via MetricsRecorder).
	Worker Metrics `json:"worker,omitempty"`
	// Framework contains supervisor-computed metrics (copied from deps by worker).
	Framework FrameworkMetrics `json:"framework,omitempty"`
}

// MetricsEmbedder provides both framework and worker metrics.
// Embed this in ObservedState structs to guarantee CSE and Prometheus paths are consistent.
//
// Usage:
//
//	type MyObservedState struct {
//	    deps.MetricsEmbedder  // Provides Metrics field with Framework and Worker
//	    CollectedAt time.Time
//	    // ... other fields
//	}
//
// The embedded struct provides:
//   - Direct field access: snap.Observed.Metrics.Framework.TimeInCurrentStateMs
//   - MetricsHolder interface methods for Prometheus export (value receivers)
//
// Workers copy framework metrics explicitly in CollectObservedState:
//
//	fm := deps.GetFrameworkState()
//	if fm != nil {
//	    obs.Metrics.Framework = *fm
//	}
type MetricsEmbedder struct {
	// Metrics contains both framework and worker metrics in a nested structure.
	// JSON: {"metrics":{"framework":{...},"worker":{...}}}
	Metrics MetricsContainer `json:"metrics,omitempty"`
}

// GetWorkerMetrics implements MetricsHolder interface for worker metrics.
// Returns by value (not pointer) because ObservedState flows as values through
// interfaces, and pointer receiver methods are NOT in the method set of values.
func (m MetricsEmbedder) GetWorkerMetrics() Metrics {
	return m.Metrics.Worker
}

// GetFrameworkMetrics implements MetricsHolder interface for framework metrics.
// Returns by value (not pointer) for the same reason as GetWorkerMetrics.
func (m MetricsEmbedder) GetFrameworkMetrics() FrameworkMetrics {
	return m.Metrics.Framework
}
