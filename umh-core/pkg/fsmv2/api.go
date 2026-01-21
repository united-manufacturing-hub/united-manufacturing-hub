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

package fsmv2

import (
	"context"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/metrics"
)

// States use Signal to communicate special conditions to the supervisor.
// These signals trigger supervisor-level actions beyond normal state transitions.
type Signal int

const (
	// SignalNone indicates normal operation, no special action needed.
	SignalNone Signal = iota

	// SignalNeedsRemoval tells supervisor this worker has completed cleanup and can be removed.
	// Emitted by Stopped state when IsShutdownRequested() returns true and cleanup is complete.
	SignalNeedsRemoval

	// SignalNeedsRestart tells supervisor the worker has detected an unrecoverable error
	// and needs a full restart. The supervisor will:
	//   1. Call SetShutdownRequested(true) (trigger graceful shutdown)
	//   2. Wait for worker to complete shutdown and emit SignalNeedsRemoval
	//   3. Reset worker to initial state instead of removing it
	//   4. Restart the observation collector
	//
	// If graceful shutdown takes too long (>30s), the supervisor force-resets the worker.
	//
	// Use this when:
	//   - Action failures indicate permanent misconfiguration
	//   - Worker state is corrupted and needs a fresh start
	//   - External resource needs reconnection from scratch
	//
	// Pseudo-code example:
	//
	//   func (s *TryingToConnectState) Next(snap MySnapshot) (State, Signal, Action) {
	//       if snap.Observed.ConsecutiveFailures > 100 {
	//           return s, fsmv2.SignalNeedsRestart, nil
	//       }
	//       return s, fsmv2.SignalNone, &ConnectAction{}
	//   }
	//
	// Note: Due to Go's lack of covariance, actual implementations use State[any, any].
	// See doc.go "Immutability" section for the actual implementation pattern.
	SignalNeedsRestart
)

// Identity uniquely identifies a worker instance.
// This is immutable for the lifetime of the worker.
type Identity struct {
	ID            string `json:"id"`            // Unique identifier (e.g., UUID).
	Name          string `json:"name"`          // Human-readable name.
	WorkerType    string `json:"workerType"`    // Type of worker (e.g., "container", "pod").
	HierarchyPath string `json:"hierarchyPath"` // Full path from root: "scenario123(application)/parent-123(parent)/child001(child)".
}

// String implements fmt.Stringer for logging purposes.
// Returns HierarchyPath if available, falls back to "ID(Type)" for root workers.
func (i Identity) String() string {
	if i.HierarchyPath != "" {
		return i.HierarchyPath
	}

	if i.ID != "" && i.WorkerType != "" {
		return i.ID + "(" + i.WorkerType + ")"
	}

	return "unknown"
}

// ObservedState represents the actual state gathered from monitoring the system.
// Implementations should include timestamps to detect staleness.
// The supervisor collects this via CollectObservedState() in a separate goroutine.
type ObservedState interface {
	// GetObservedDesiredState returns the desired state that is actually deployed.
	// Comparing what's deployed vs what we want to deploy.
	// This interface enforces that all configured values are read back for verification.
	GetObservedDesiredState() DesiredState

	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}

// DesiredState represents what we want the system to be.
// Derived from user configuration via DeriveDesiredState().
//
// DesiredState does not contain Dependencies (runtime interfaces).
// Pass dependencies to Action.Execute() instead.
type DesiredState interface {
	// IsShutdownRequested is set by supervisor to initiate graceful shutdown.
	// States should check this first in their Next() method.
	IsShutdownRequested() bool

	// GetState returns the desired lifecycle state ("running", "stopped", etc.).
	// Used by supervisor to validate state values after DeriveDesiredState.
	GetState() string
}

// ShutdownRequestable allows setting the shutdown flag on any DesiredState.
// All DesiredState types should embed config.BaseDesiredState (from pkg/fsmv2/config) to satisfy this interface.
// Type-safe shutdown request propagation from supervisor to workers.
//
// Example usage:
//
//	if sr, ok := any(desired).(fsmv2.ShutdownRequestable); ok {
//	    sr.SetShutdownRequested(true)
//	}
type ShutdownRequestable interface {
	SetShutdownRequested(bool)
}

// Snapshot is the complete view of the worker at a point in time.
// The supervisor passes Snapshot by value to State.Next(), making it inherently immutable.
// Use helpers.ConvertSnapshot[O, D](snapAny) for type-safe field access.
type Snapshot struct {
	Identity Identity    // Who am I?
	Observed interface{} // What is the actual state? (ObservedState or basic.Document).
	Desired  interface{} // What should the state be? (DesiredState or basic.Document).
}

// Action represents an idempotent side effect that modifies external system state.
// The supervisor executes actions after State.Next() returns them.
// When an action fails, the state remains unchanged. On the next tick,
// state.Next() may return the same action.
//
// Actions must be idempotent (safe to call multiple times).
// For detailed patterns and examples, see doc.go "Actions" section.
type Action[TDeps any] interface {
	// Execute performs the action. Must handle context cancellation.
	Execute(ctx context.Context, deps TDeps) error
	// Name returns a descriptive name for logging/debugging.
	Name() string
}

// State represents a single state in the FSM lifecycle.
// States are stateless - they examine the snapshot and decide what happens next.
// See doc.go "States" section for patterns and rationale.
type State[TSnapshot any, TDeps any] interface {
	// Next evaluates the snapshot and returns the next transition.
	// Pure function called on each tick. The supervisor passes the snapshot by value (immutable).
	// Returns: nextState, signal to supervisor, optional action to execute.
	Next(snapshot TSnapshot) (State[TSnapshot, TDeps], Signal, Action[TDeps])

	// String returns the state name for logging/debugging.
	String() string

	// Reason returns the reason for the current state and gives more background information.
	// For degraded state, it reports exactly what degrades functionality.
	// In TryingTo... states, it reports the target state - for example, benthos reports that S6 is not yet started.
	Reason() string
}

// Worker is the business logic interface that developers implement.
// The supervisor manages the worker lifecycle using these methods.
type Worker interface {
	// CollectObservedState monitors the actual system state.
	// The supervisor calls this method in a separate goroutine with timeout protection.
	// Must respect context cancellation. Errors are logged but don't stop the FSM.
	CollectObservedState(ctx context.Context) (ObservedState, error)

	// DeriveDesiredState derives the target state from user configuration (spec).
	// This is a derivation, not a copy - it parses, validates, and computes derived fields.
	// Pure function - no side effects. Called on each tick.
	//
	// Return type is DesiredState interface, allowing workers to return their typed
	// DesiredState structs (e.g., *snapshot.CommunicatorDesiredState). The supervisor
	// type-asserts to TDesired before marshaling, preserving all typed fields.
	DeriveDesiredState(spec interface{}) (DesiredState, error)

	// GetInitialState returns the starting state for this worker.
	// Called once during worker creation.
	GetInitialState() State[any, any]

	// Shutdown is managed by the supervisor via ShutdownRequested in desired state.
}

// DependencyProvider is an optional interface that workers can implement
// to expose their dependencies for action execution.
// Workers that embed helpers.BaseWorker automatically satisfy this interface.
//
// Example usage:
//
//	if provider, ok := worker.(DependencyProvider); ok {
//	    deps := provider.GetDependenciesAny()
//	    action.Execute(ctx, deps)
//	}
type DependencyProvider interface {
	// GetDependenciesAny returns the worker's dependencies as any.
	// This is used by the ActionExecutor to pass dependencies to actions.
	GetDependenciesAny() any
}

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
//	holder, ok := observed.(fsmv2.MetricsHolder)
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
	mu       sync.Mutex
	counters map[string]int64
	gauges   map[string]float64
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
//	deps.Metrics().IncrementCounter(metrics.CounterMessagesPulled, int64(len(messages)))
func (r *MetricsRecorder) IncrementCounter(name metrics.CounterName, delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counters[string(name)] += delta
}

// SetGauge sets a gauge to the specified value.
// Uses typed GaugeName for compile-time safety against typos.
//
// Example:
//
//	deps.Metrics().SetGauge(metrics.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
func (r *MetricsRecorder) SetGauge(name metrics.GaugeName, value float64) {
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

	// Reset buffers for next tick
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

	// TimeInCurrentStateMs is milliseconds since the current state was entered.
	// Updated each tick by supervisor.
	TimeInCurrentStateMs int64 `json:"time_in_current_state_ms"`

	// StateEnteredAtUnix is Unix timestamp (seconds) when current state was entered.
	StateEnteredAtUnix int64 `json:"state_entered_at_unix"`

	// StateTransitionsTotal is the total number of state transitions this session.
	StateTransitionsTotal int64 `json:"state_transitions_total"`

	// TransitionsByState maps state names to the number of times that state was entered.
	// Example: {"running": 5, "degraded": 2, "stopped": 1}
	TransitionsByState map[string]int64 `json:"transitions_by_state,omitempty"`

	// CumulativeTimeByStateMs maps state names to total milliseconds spent in that state.
	// Example: {"running": 60000, "degraded": 5000}
	CumulativeTimeByStateMs map[string]int64 `json:"cumulative_time_by_state_ms,omitempty"`

	// CollectorRestarts is the number of times the observation collector was restarted
	// for this specific worker (not global). Reset on worker restart.
	CollectorRestarts int64 `json:"collector_restarts"`

	// === Persistent Counters (survive restart) ===

	// StartupCount is the number of times this worker has been started.
	// Loaded from CSE on AddWorker(), incremented, and persisted.
	// Use this to detect restart loops or track worker lifetime.
	StartupCount int64 `json:"startup_count"`
}

// MetricsContainer groups framework and worker metrics together.
// Named type (not anonymous struct) for testability, godoc, and error messages.
//
// JSON structure: {"framework":{...},"worker":{...}}
// Access pattern: snap.Observed.Metrics.Framework.TimeInCurrentStateMs
type MetricsContainer struct {
	// Framework contains supervisor-computed metrics (copied from deps by worker).
	Framework FrameworkMetrics `json:"framework,omitempty"`

	// Worker contains worker-defined metrics (recorded via MetricsRecorder).
	Worker Metrics `json:"worker,omitempty"`
}

// MetricsEmbedder provides both framework and worker metrics.
// Embed this in ObservedState structs to guarantee CSE and Prometheus paths are consistent.
//
// Usage:
//
//	type MyObservedState struct {
//	    fsmv2.MetricsEmbedder  // Provides Metrics field with Framework and Worker
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

