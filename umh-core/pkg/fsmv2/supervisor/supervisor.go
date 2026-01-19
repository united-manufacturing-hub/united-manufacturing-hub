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

// Package supervisor provides a generic finite state machine supervisor for managing
// worker lifecycles with compile-time type safety.
//
// The Supervisor[TObserved, TDesired] type uses generics to ensure all workers managed
// by a supervisor have consistent ObservedState and DesiredState types. This eliminates
// runtime type errors and provides better IDE support.
//
// # Type Safety with Generics
//
// Example usage:
//
//	supervisor := NewSupervisor[ExampleparentObservedState, *ExampleparentDesiredState](config)
//	supervisor.AddWorker(identity, worker)
//
// Worker types are automatically derived from the observed state type name:
//   - ExampleparentObservedState -> "exampleparent"
//   - ContainerObservedState -> "container"
//
// This eliminates manual WorkerType constants and provides compile-time validation.
//
// # Architecture Constraints
//
// Storage Abstraction:
//
//	The supervisor MUST interact with storage exclusively through the
//	TriangularStore adapter interface. Direct access to storage packages
//	(e.g., pkg/cse/storage) is not permitted in supervisor production code.
//
//	Rationale:
//	  - Maintains clean abstraction boundaries
//	  - Prevents tight coupling to storage implementations
//	  - Enables storage backend changes without supervisor modifications
//	  - Facilitates testing through adapter mocking
//
//	Tests may use direct storage access or mock implementations for
//	test setup and verification purposes.
//
// # LOCK ORDER
//
// To prevent deadlocks, locks must be acquired in this order:
//
// 1. MANDATORY: Supervisor.mu → WorkerContext.mu
//   - Always acquire Supervisor.mu first to lookup worker
//   - Then acquire WorkerContext.mu to modify state
//   - Violation risk: HIGH (immediate deadlock)
//
// 2. ADVISORY: Supervisor.mu → Supervisor.ctxMu
//   - If both needed, acquire mu first
//   - However, ctxMu is independent and can be acquired alone
//   - Violation risk: LOW (rarely needed together)
//
// 3. CRITICAL: Never hold Supervisor.mu while calling child/worker methods
//   - Extract data under lock, release, then call methods
//   - Prevents circular dependencies in hierarchy
//   - Current code already follows this (lines 1783-1791, 1420-1436)
//
// 4. WorkerContext.mu locks are independent
//   - No ordering between different workers' locks
//   - Enables true parallel worker processing
package supervisor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/health"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/lockmanager"
)

// =============================================================================
// SUPERVISOR INVARIANTS
// =============================================================================
//
// The supervisor maintains the following invariants to ensure correct operation.
// Violations indicate programming errors (bugs in supervisor logic or incorrect usage).
//
// I1: restartCount range
//     MUST: 0 <= restartCount <= maxRestartAttempts
//     WHY:  Prevents infinite restart loops and ensures bounded recovery
//     ENFORCED: RestartCollector() panics if called when count >= max
//
// I2: threshold ordering
//     MUST: 0 < staleThreshold < timeout
//     WHY:  Stale detection must occur before timeout triggers collector restart
//     ENFORCED: NewSupervisor() panics if configuration violates this
//
// I3: trust boundary (data freshness)
//     MUST: state.Next() only called when CheckDataFreshness() returns true
//     WHY:  States assume observation data is always fresh (supervisor's responsibility)
//     ENFORCED: tick() checks freshness and pauses FSM if data is stale
//
// I4: bounded retry (escalation)
//     MUST: RestartCollector() not called when restartCount >= maxRestartAttempts
//     WHY:  Must escalate to shutdown after max attempts, not retry forever
//     ENFORCED: tick() logic ensures this + RestartCollector() panics if violated
//
// I7: timeout ordering validation
//     MUST: ObservationTimeout < StaleThreshold < CollectorTimeout
//     WHY:  Observation failures must not trigger stale detection, and stale detection
//           must occur before collector restart
//     ENFORCED: NewSupervisor() panics if configuration violates this ordering
//
// I16: type safety (ObservedState type validation)
//     MUST: Worker returns consistent ObservedState type matching initial discovery
//     WHY:  Type mismatches indicate programming errors (wrong state type wiring)
//           States assume snapshot.Observed has correct concrete type for assertions
//           Catching this at supervisor boundary prevents invalid type assertions in states
//     ENFORCED: AddWorker() discovers expected type via CollectObservedState()
//               tickWorker() validates type before calling state.Next() (Layer 3.5)
//               Panics with clear message showing worker type and actual/expected types
//     LAYER: Defense Layer 3.5 (between freshness check and state logic)
//
// =============================================================================

// Lock names for tracking.
const (
	lockNameSupervisorMu    = "Supervisor.mu"
	lockNameSupervisorCtxMu = "Supervisor.ctxMu"
	lockNameWorkerContextMu = "WorkerContext.mu"
)

// heartbeatTickInterval defines how many ticks between heartbeat logs.
// At 100ms tick interval, this logs every 10 seconds.
const heartbeatTickInterval = 100

// Lock levels for ordering (lower number = must be acquired first).
const (
	lockLevelSupervisorMu    = 1
	lockLevelSupervisorCtxMu = 2 // Can be acquired alone OR after Supervisor.mu
	lockLevelWorkerContextMu = 3 // Must be acquired AFTER Supervisor.mu
)

// Supervisor manages the lifecycle of a single worker.
// It runs two goroutines:
//  1. Observation loop: Continuously calls worker.CollectObservedState()
//  2. Main tick loop: Calls state.Next() and executes actions
//
// The supervisor implements the 4-layer defense for data freshness:
//   - Layer 1: Pause FSM when data is stale (>10s by default)
//   - Layer 2: Restart collector when data times out (>20s by default)
//   - Layer 3: Request graceful shutdown after max restart attempts
//   - Layer 4: Comprehensive logging and metrics (observability)
//
// ARCHITECTURE: Single-Node Coordination
//
// This implementation coordinates multiple workers on a single node only.
// State persistence uses TriangularStore, which currently assumes all workers
// run in the same process and can share in-memory state. For distributed deployments,
// the persistence layer would need to be replaced with a distributed storage backend.
type Supervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState] struct {
	workerType string                                         // Type of workers managed (e.g., "container") - for storage collection naming
	workers    map[string]*WorkerContext[TObserved, TDesired] // workerID → worker context
	// mu Protects access to workers map, children, childDoneChans, globalVars, and mappedParentState.
	//
	// This is a lockmanager.Lock wrapping sync.RWMutex to allow concurrent reads from multiple goroutines
	// (e.g., GetWorker, ListWorkers) while ensuring exclusive writes when modifying
	// worker registry state (e.g., AddWorker, RemoveWorker).
	//
	// Lock Order: Must be acquired BEFORE WorkerContext.mu when both are needed.
	// See package-level LOCK ORDER section for details.
	mu               *lockmanager.Lock
	lockManager      *lockmanager.LockManager
	store            storage.TriangularStoreInterface // State persistence layer (triangular model)
	logger           *zap.SugaredLogger               // Logger for supervisor operations (enriched with worker path)
	baseLogger       *zap.SugaredLogger               // Original logger without worker enrichment (for child supervisors)
	tickInterval     time.Duration                    // How often to evaluate state transitions
	collectorHealth  CollectorHealth                  // Collector health tracking
	freshnessChecker *health.FreshnessChecker         // Data freshness validator
	children         map[string]SupervisorInterface   // Child supervisors by name (hierarchical composition)
	childDoneChans   map[string]<-chan struct{}       // Done channels for child supervisors
	pendingRemoval      map[string]bool                  // Children pending graceful shutdown (waiting for SignalNeedsRemoval)
	pendingRestart      map[string]bool                  // Workers pending restart after shutdown completes
	restartRequestedAt  map[string]time.Time             // When restart was requested for each worker
	childStartStates    []string                         // Parent FSM states where this child should run
	userSpec          config.UserSpec                  // User-provided configuration for this supervisor
	mappedParentState string                           // State mapped from parent (if this is a child supervisor)
	globalVars        map[string]any                   // Global variables (fleet-wide settings from management system)
	createdAt         time.Time                        // Timestamp when supervisor was created
	parentID          string                           // ID of parent supervisor (empty string for root supervisors)
	parent            SupervisorInterface              // Pointer to parent supervisor (nil for root supervisors)
	healthChecker     *InfrastructureHealthChecker     // Infrastructure health monitoring
	circuitOpen       atomic.Bool                      // Circuit breaker state (atomic for concurrent access)
	actionExecutor    *execution.ActionExecutor        // Async action execution (Phase 2)
	metricsWg         sync.WaitGroup                   // WaitGroup for metrics reporter goroutine
	ctx               context.Context                  // Context for supervisor lifecycle
	ctxCancel         context.CancelFunc               // Cancel function for supervisor context
	// ctxMu Protects ctx and ctxCancel to prevent TOCTOU races during shutdown.
	//
	// Without this lock, a goroutine could check ctx.Err() (finding it non-cancelled),
	// then another goroutine calls ctxCancel(), then the first goroutine uses ctx
	// assuming it's still valid. This lock ensures atomic read-check-use patterns.
	//
	// This lock is independent from Supervisor.mu and can be acquired separately.
	// It can be acquired alone when checking context status, or after Supervisor.mu
	// if both are needed (advisory order).
	ctxMu                    *lockmanager.Lock
	started                  atomic.Bool    // Whether supervisor has been started
	enableTraceLogging       bool           // Whether to emit verbose lifecycle logs (mutex, tick events)
	noStateMachineLoggedOnce sync.Map       // Tracks workers that have logged no_state_machine (workerID → true)
	tickCount                uint64         // Counter for ticks, used for periodic heartbeat logging
	gracefulShutdownTimeout  time.Duration  // How long to wait for workers to shutdown gracefully
	lastUserSpecHash   string             // Hash of last UserSpec used for DeriveDesiredState
	cachedDesiredState fsmv2.DesiredState // Cached result from DeriveDesiredState (typed TDesired stored as interface)
	deps               map[string]any     // Dependencies for worker creation (injected, not global)
}

func NewSupervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](cfg Config) *Supervisor[TObserved, TDesired] {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = DefaultTickInterval
	}

	observationTimeout := cfg.CollectorHealth.ObservationTimeout
	if observationTimeout == 0 {
		observationTimeout = DefaultObservationTimeout
	}

	staleThreshold := cfg.CollectorHealth.StaleThreshold
	if staleThreshold == 0 {
		staleThreshold = DefaultStaleThreshold
	}

	timeout := cfg.CollectorHealth.Timeout
	if timeout == 0 {
		timeout = DefaultCollectorTimeout
	}

	maxRestartAttempts := cfg.CollectorHealth.MaxRestartAttempts
	if maxRestartAttempts == 0 {
		maxRestartAttempts = DefaultMaxRestartAttempts
	}

	if staleThreshold <= 0 {
		panic(fmt.Sprintf("supervisor config error: staleThreshold must be positive, got %v", staleThreshold))
	}

	if timeout <= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: timeout (%v) must be greater than staleThreshold (%v)", timeout, staleThreshold))
	}

	if maxRestartAttempts <= 0 {
		panic(fmt.Sprintf("supervisor config error: maxRestartAttempts must be positive, got %d", maxRestartAttempts))
	}

	if observationTimeout >= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: observationTimeout (%v) must be less than staleThreshold (%v)", observationTimeout, staleThreshold))
	}

	if staleThreshold >= timeout {
		panic(fmt.Sprintf("supervisor config error: staleThreshold (%v) must be less than collectorTimeout (%v)", staleThreshold, timeout))
	}

	cfg.Logger.Infow("timeout_configuration",
		"worker", cfg.WorkerType,
		"observation_timeout", observationTimeout,
		"stale_threshold", staleThreshold,
		"collector_timeout", timeout)

	freshnessChecker := health.NewFreshnessChecker(staleThreshold, timeout, cfg.Logger)

	lm := lockmanager.NewLockManager()

	gracefulShutdownTimeout := cfg.GracefulShutdownTimeout
	if gracefulShutdownTimeout == 0 {
		gracefulShutdownTimeout = DefaultGracefulShutdownTimeout
	}

	return &Supervisor[TObserved, TDesired]{
		workerType:       cfg.WorkerType,
		workers:          make(map[string]*WorkerContext[TObserved, TDesired]),
		lockManager:      lm,
		mu:               lm.NewLock(lockNameSupervisorMu, lockLevelSupervisorMu),
		ctxMu:            lm.NewLock(lockNameSupervisorCtxMu, lockLevelSupervisorCtxMu),
		store:            cfg.Store,
		logger:           cfg.Logger,
		baseLogger:       cfg.Logger, // Store un-enriched logger for passing to child supervisors
		tickInterval:     tickInterval,
		freshnessChecker: freshnessChecker,
		children:       make(map[string]SupervisorInterface),
		childDoneChans:     make(map[string]<-chan struct{}),
		pendingRemoval:     make(map[string]bool),
		pendingRestart:     make(map[string]bool),
		restartRequestedAt: make(map[string]time.Time),
		createdAt:          time.Now(),
		parentID:         "",
		healthChecker:    NewInfrastructureHealthChecker(DefaultMaxInfraRecoveryAttempts, DefaultRecoveryAttemptWindow),
		actionExecutor:   execution.NewActionExecutor(10, cfg.WorkerType, cfg.Logger),
		collectorHealth: CollectorHealth{
			observationTimeout: observationTimeout,
			staleThreshold:     staleThreshold,
			timeout:            timeout,
			maxRestartAttempts: maxRestartAttempts,
			restartCount:       0,
		},
		userSpec:                cfg.UserSpec,
		enableTraceLogging:      cfg.EnableTraceLogging,
		gracefulShutdownTimeout: gracefulShutdownTimeout,
		deps:                    cfg.Dependencies,
	}
}
