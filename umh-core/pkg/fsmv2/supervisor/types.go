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
	observationTimeout time.Duration // Maximum time for single observation operation
	staleThreshold     time.Duration // Age when data considered stale (pause FSM)
	timeout            time.Duration // Age when collector considered broken (trigger restart)
	maxRestartAttempts int           // Maximum restart attempts before escalation
	restartCount       int           // Current restart attempt counter
	lastRestart        time.Time     // Timestamp of last restart attempt
}

// stubAction is a no-op action used for Phase 2 integration testing.
// This will be replaced with real action derivation in Phase 3.
// It exists to prevent EnqueueAction from being test-only.
type stubAction struct{}

func (s *stubAction) Execute(ctx context.Context, deps any) error {
	// No-op: Phase 2 stub, real actions in Phase 3
	return nil
}

func (s *stubAction) Name() string {
	return "stub-action-phase2"
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
	AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error
	setParent(parent SupervisorInterface, parentID string)
	// GetHierarchyPath returns the full hierarchy path from root to this supervisor.
	// Format: "workerID(workerType)/childID(childType)/..."
	// Example: "scenario123(application)/parent-123(parent)/child001(child)"
	GetHierarchyPath() string
	// GetCurrentStateName returns the current FSM state name for this supervisor's worker.
	// Returns "unknown" if no worker or state is set.
	GetCurrentStateName() string
	// GetWorkerType returns the type of workers this supervisor manages.
	// Example: "examplechild", "exampleparent", "application"
	GetWorkerType() string
	// TestGetUserSpec returns the current userSpec for testing. DO NOT USE in production code.
	TestGetUserSpec() config.UserSpec
}

// WorkerContext encapsulates the runtime state for a single worker
// managed by a multi-worker Supervisor. It groups the worker's identity,
// implementation, current FSM state, and observation collector.
//
// THREAD SAFETY: currentState is protected by mu. Always lock before accessing.
// tickInProgress prevents concurrent ticks for the same worker.
type WorkerContext[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState] struct {
	// mu Protects currentState (per-worker FSM state).
	//
	// This is a lockmanager.Lock wrapping sync.RWMutex because state is checked on every tick (frequent concurrent reads)
	// but only written during state transitions (infrequent writes).
	//
	// Per-worker isolation: Each WorkerContext has its own independent mu lock,
	// enabling true parallel processing of multiple workers without contention.
	// There is no ordering between different workers' mu locks.
	//
	// Lock Order: This lock must be acquired AFTER Supervisor.mu when both are needed.
	// See package-level LOCK ORDER section for details.
	mu                *lockmanager.Lock
	tickInProgress    atomic.Bool
	identity          fsmv2.Identity
	worker            fsmv2.Worker
	currentState      fsmv2.State[any, any]
	collector         *collection.Collector[TObserved]
	executor          *execution.ActionExecutor
	actionPending     bool
	lastActionObsTime time.Time
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
	// WorkerType identifies the type of workers this supervisor manages.
	// Required - no default.
	WorkerType string

	// Store persists FSM state (identity, desired, observed) using triangular model.
	// Required - no default. Use storage.NewTriangularStore(basicStore, logger) or a mock.
	Store storage.TriangularStoreInterface

	// Logger for supervisor operations.
	// Required - no default (use zap.NewNop().Sugar() for tests).
	Logger *zap.SugaredLogger

	// TickInterval is how often supervisor evaluates FSM state transitions.
	// Optional - defaults to DefaultTickInterval (1 second).
	TickInterval time.Duration

	// CollectorHealth configures observation collector monitoring.
	// Optional - all fields default to values in constants.go.
	CollectorHealth CollectorHealthConfig

	// UserSpec provides initial user configuration for the supervisor.
	// This is optional for supervisors created by parents (they receive config via updateUserSpec).
	// For root supervisors (like application supervisors), this provides the initial configuration.
	UserSpec config.UserSpec

	// EnableTraceLogging enables verbose lifecycle event logging (mutex locks, tick events, etc.)
	// Optional - defaults to false. Set ENABLE_TRACE_LOGGING=true for deep debugging.
	// When false, these high-frequency internal logs are suppressed to improve signal-to-noise ratio.
	EnableTraceLogging bool

	// GracefulShutdownTimeout is how long to wait for workers to complete graceful shutdown.
	// During shutdown, the supervisor requests graceful shutdown on all workers and waits
	// for them to be removed from the workers map (by emitting SignalNeedsRemoval).
	// If this timeout is reached, the supervisor proceeds with forced shutdown.
	// Optional - defaults to 5 seconds.
	GracefulShutdownTimeout time.Duration
}
