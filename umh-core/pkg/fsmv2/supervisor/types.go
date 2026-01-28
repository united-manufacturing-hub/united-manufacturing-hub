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
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/lockmanager"
)

// CollectorHealth tracks the health and restart state of the observation collector.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or other infrastructure problems.
//
// This struct is internal to Supervisor and tracks runtime state.
// Configuration is provided via CollectorHealthConfig.
type CollectorHealth struct {
	lastRestart        time.Time     // Timestamp of last restart attempt
	observationTimeout time.Duration // Maximum time for single observation operation
	staleThreshold     time.Duration // Age when data considered stale (pause FSM)
	timeout            time.Duration // Age when collector considered broken (trigger restart)
	maxRestartAttempts int           // Maximum restart attempts before escalation
	restartCount       int           // Current restart attempt counter
}

// SupervisorInterface provides a type-erased interface for hierarchical supervisor composition.
// A parent Supervisor[TObserved, TDesired] can manage children of different types.
type SupervisorInterface interface {
	Start(ctx context.Context) <-chan struct{}
	Shutdown()
	RequestShutdown(ctx context.Context, reason string) error
	ListWorkers() []string
	tick(ctx context.Context) error
	updateUserSpec(spec config.UserSpec)
	getUserSpec() config.UserSpec
	getChildStartStates() []string
	setChildStartStates(states []string)
	getMappedParentState() string
	setMappedParentState(state string)
	calculateHierarchySize() int
	calculateHierarchyDepth() int
	GetChildren() map[string]SupervisorInterface
	AddWorker(identity deps.Identity, worker fsmv2.Worker) error
	setParent(parent SupervisorInterface, parentID string)
	// GetHierarchyPath returns the full hierarchy path from root to this supervisor.
	// Format: "workerID(workerType)/childID(childType)/..."
	// Example: "scenario123(application)/parent-123(parent)/child001(child)"
	GetHierarchyPath() string
	// GetHierarchyPathUnlocked returns the hierarchy path without acquiring locks.
	// Use this from within locked contexts to avoid deadlock.
	// Caller must hold s.mu.RLock() or s.mu.Lock().
	GetHierarchyPathUnlocked() string
	// GetCurrentStateName returns the current FSM state name for this supervisor's worker.
	// Returns "unknown" if no worker or state is set.
	GetCurrentStateName() string
	// GetCurrentStateNameAndReason returns the current FSM state name and reason.
	// Returns ("unknown", "") if no worker or state is set.
	// Used by ChildInfo to populate StateReason field.
	GetCurrentStateNameAndReason() (stateName string, reason string)
	// GetObservedStateName returns the observed state's State field (e.g., "running_healthy_connected").
	// Use config.ParseLifecyclePhase() to convert this to a LifecyclePhase for health checks.
	// Returns "unknown" if no observation has been collected yet.
	GetObservedStateName() string
	// GetLifecyclePhase returns the lifecycle phase of the current state.
	// Used by parent supervisors to classify child health via phase.IsHealthy().
	// Returns PhaseUnknown if no state is set.
	GetLifecyclePhase() config.LifecyclePhase
	// GetWorkerType returns the type of workers this supervisor manages.
	// Example: "examplechild", "exampleparent", "application"
	GetWorkerType() string
	// IsObservationStale returns true if the last observation is older than the stale threshold.
	// Used by ChildInfo to report infrastructure status to parents.
	IsObservationStale() bool
	// IsCircuitOpen returns true if the circuit breaker is open for this supervisor.
	// When open, it indicates infrastructure failure (child consistency check failed).
	// Used by ChildInfo to report infrastructure status to parents.
	IsCircuitOpen() bool
	// TestGetUserSpec returns the current userSpec for testing. DO NOT USE in production code.
	TestGetUserSpec() config.UserSpec
	// GetDebugInfo returns introspection data for debugging and monitoring.
	// The returned data provides a snapshot of the supervisor's state.
	// Returns interface{} to satisfy metrics.FSMv2DebugProvider interface.
	GetDebugInfo() interface{}
}

// WorkerContext encapsulates the runtime state for a single worker
// managed by a multi-worker Supervisor. It groups the worker's identity,
// implementation, current FSM state, and observation collector.
//
// THREAD SAFETY: currentState is protected by mu. Always lock before accessing.
// tickInProgress prevents concurrent ticks for the same worker.
type WorkerContext[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState] struct {
	lastActionObsTime time.Time

	// Supervisor-internal state tracking (copied into FrameworkMetrics before State.Next()).
	// Workers read via FrameworkMetrics; MetricsRecorder handles worker-written metrics.
	stateEnteredAt time.Time // When current state was entered

	// lastObservationCollectedAt stores the CollectedAt timestamp from the most recent
	// observation loaded during tickWorker(). This is cached here (rather than re-fetched
	// from CSE) so that IsObservationStale() can be called by the PARENT supervisor
	// during its tick without blocking on CSE reads. The parent calls child.IsObservationStale()
	// when building ChildInfo for SetChildrenView().
	lastObservationCollectedAt time.Time
	// lastObservedStateName caches the constructed observed state name (e.g., "running_healthy_connected").
	// Constructed by supervisor as: phase.Prefix() + lowercase(state.String())
	// Used by parent supervisors via GetObservedStateName() for health checks.
	lastObservedStateName string
	// lastLifecyclePhase caches the lifecycle phase of the current state.
	// Used by parent supervisors via GetLifecyclePhase() to classify child health.
	// This is the source of truth for health classification (phase.IsHealthy()).
	lastLifecyclePhase config.LifecyclePhase
	worker                     fsmv2.Worker
	currentState               fsmv2.State[any, any]
	// mu protects currentState. Uses RWMutex for frequent reads, rare writes.
	// Lock Order: Acquire AFTER Supervisor.mu when both needed.
	// WorkerContext.mu locks are independent from each other (enables parallel worker processing).
	mu            *lockmanager.Lock
	collector     *collection.Collector[TObserved]
	executor      *execution.ActionExecutor
	actionHistory *deps.InMemoryActionHistoryRecorder // Supervisor-owned buffer for action results

	stateTransitions   map[string]int64         // state_name → total times entered
	stateDurations     map[string]time.Duration // state_name → cumulative time spent
	identity           deps.Identity
	currentStateReason string // Human-readable reason for current state (from NextResult.Reason)
	totalTransitions   int64  // Sum of all stateTransitions values
	collectorRestarts  int64  // Per-worker collector restarts
	startupCount       int64  // PERSISTENT: Loaded from CSE, incremented on AddWorker()
	tickInProgress       atomic.Bool
	actionPending        bool
	gatingExplainedOnce  bool  // True after first DEBUG log for action-observation gating
	gatingBlockedCount   int64 // Count of ticks blocked by action-observation gating
}

// CollectorHealthConfig configures observation collector health monitoring.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or infrastructure problems. These settings control when to
// pause the FSM (stale data) and when to restart the collector (timeout).
type CollectorHealthConfig struct {
	// ObservationTimeout is the maximum time allowed for a single observation operation.
	// If an observation takes longer than this, it is cancelled and considered failed.
	// Must be less than StaleThreshold to ensure observation failures don't trigger stale detection.
	// Default: ~1.3 seconds (see DefaultObservationTimeout in constants.go)
	ObservationTimeout time.Duration

	// StaleThreshold is how old observation data can be before FSM pauses.
	// When exceeded, supervisor stops calling state.Next() but does not restart collector.
	// Default: 10 seconds (see DefaultStaleThreshold in constants.go)
	StaleThreshold time.Duration

	// Timeout is how old observation data can be before collector is considered broken.
	// When exceeded, supervisor triggers collector restart with exponential backoff.
	// Should be significantly larger than StaleThreshold to avoid restart thrashing.
	// Default: 20 seconds (see DefaultCollectorTimeout in constants.go)
	Timeout time.Duration

	// MaxRestartAttempts is the maximum number of collector restart attempts.
	// After this many failed restarts, supervisor escalates to graceful FSM shutdown.
	// Each restart uses exponential backoff: attempt N waits N*2 seconds.
	// Default: 3 attempts (see DefaultMaxRestartAttempts in constants.go)
	MaxRestartAttempts int
}

// Config contains supervisor configuration.
// All fields except WorkerType and Store have sensible defaults.
type Config struct {

	// UserSpec provides initial user configuration for the supervisor.
	// This is optional for supervisors created by parents (they receive config via updateUserSpec).
	// For root supervisors (like application supervisors), this provides the initial configuration.
	UserSpec config.UserSpec

	// Store persists FSM state (identity, desired, observed) using triangular model.
	// Required - no default. Use storage.NewTriangularStore(basicStore, logger) or a mock.
	Store storage.TriangularStoreInterface

	// Logger for supervisor operations.
	// Required - no default (use zap.NewNop().Sugar() for tests).
	Logger *zap.SugaredLogger

	// Dependencies is an optional map of named dependencies to inject into child workers.
	// Worker factories can access these via the deps parameter to avoid global state.
	// Example: deps["channelProvider"] could provide channels for the communicator worker.
	// Optional - defaults to nil.
	Dependencies map[string]any
	// WorkerType identifies the type of workers this supervisor manages.
	// Required - no default.
	WorkerType string

	// CollectorHealth configures observation collector monitoring.
	// Optional - all fields default to values in constants.go.
	CollectorHealth CollectorHealthConfig

	// TickInterval is how often supervisor evaluates FSM state transitions.
	// Optional - defaults to DefaultTickInterval (1 second).
	TickInterval time.Duration

	// GracefulShutdownTimeout is how long to wait for workers to complete graceful shutdown.
	// During shutdown, the supervisor requests graceful shutdown on all workers and waits
	// for them to be removed from the workers map (by emitting SignalNeedsRemoval).
	// If this timeout is reached, the supervisor proceeds with forced shutdown.
	// Optional - defaults to 5 seconds.
	GracefulShutdownTimeout time.Duration

	// EnableTraceLogging enables verbose lifecycle event logging (mutex locks, tick events, etc.)
	// Optional - defaults to false. Set ENABLE_TRACE_LOGGING=true for deep debugging.
	// When false, these high-frequency internal logs are suppressed to improve signal-to-noise ratio.
	EnableTraceLogging bool
}
