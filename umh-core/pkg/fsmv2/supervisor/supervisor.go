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
// by a supervisor have consistent ObservedState and DesiredState types.
//
// # Type Safety with Generics
//
// Example usage (Worker API v2 / register.Worker):
//
//	supervisor := NewSupervisor[fsmv2.Observation[ExamplechildStatus], *fsmv2.WrappedDesiredState[ExamplechildConfig]](config)
//	supervisor.AddWorker(identity, worker)
//
// In practice, worker packages do not construct supervisors directly — register.Worker
// wires the auto-generated supervisor factory at init time. The worker type string
// is the canonical name (e.g. "examplechild"), and the folder name must match.
//
// # Architecture Constraints
//
// Storage Abstraction: The supervisor MUST interact with storage exclusively through
// the TriangularStore adapter interface. Tests may use direct storage access for
// setup and verification.
//
// # LOCK ORDER
//
// To prevent deadlocks, locks must be acquired in this order:
//
// 1. MANDATORY: Supervisor.mu → WorkerContext.mu (violation = immediate deadlock)
// 2. ADVISORY: Supervisor.mu → Supervisor.ctxMu (ctxMu is independent and can be acquired alone)
// 3. CRITICAL: Never hold Supervisor.mu while calling child/worker methods
// 4. WorkerContext.mu locks are independent (enables parallel worker processing)
package supervisor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

const (
	lockNameSupervisorMu    = "Supervisor.mu"
	lockNameSupervisorCtxMu = "Supervisor.ctxMu"
	lockNameWorkerContextMu = "WorkerContext.mu"
)

const heartbeatTickInterval = 100 // ticks between heartbeat logs

// Lock levels for ordering (lower = acquired first).
const (
	lockLevelSupervisorMu    = 1
	lockLevelSupervisorCtxMu = 2
	lockLevelWorkerContextMu = 3
)

// Supervisor manages worker lifecycles via two goroutines: observation loop and tick loop.
//
// 4-layer defense for data freshness:
//   - Layer 1: Pause FSM when data is stale (>10s)
//   - Layer 2: Restart collector when data times out (>20s)
//   - Layer 3: Request graceful shutdown after max restart attempts
//   - Layer 4: Logging and metrics
//
// Single-node coordination only; distributed deployments require a different storage backend.
type Supervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState] struct {
	createdAt           time.Time
	store               storage.TriangularStoreInterface
	parent              SupervisorInterface
	ctx                 context.Context
	cachedDesiredState  fsmv2.DesiredState
	cachedFirstWorkerID atomic.Value // string - cached for GetHierarchyPathUnlocked()
	workers             map[string]*WorkerContext[TObserved, TDesired]
	// mu Protects access to workers map, children, childDoneChans, globalVars, and mappedParentState.
	//
	// This is a lockmanager.Lock wrapping sync.RWMutex to allow concurrent reads from multiple goroutines
	// (e.g., GetWorker, ListWorkers) while ensuring exclusive writes when modifying
	// worker registry state (e.g., AddWorker, RemoveWorker).
	//
	// Lock Order: Must be acquired BEFORE WorkerContext.mu when both are needed.
	// See package-level LOCK ORDER section for details.
	mu                 *lockmanager.Lock
	lockManager        *lockmanager.LockManager
	logger             deps.FSMLogger
	baseLogger         deps.FSMLogger // Un-enriched logger for child supervisors
	freshnessChecker   *health.FreshnessChecker
	children           map[string]SupervisorInterface
	childDoneChans     map[string]<-chan struct{}
	pendingRemoval     map[string]bool
	pendingRestart     map[string]bool
	restartRequestedAt map[string]time.Time
	globalVars         map[string]any
	healthChecker      *InfrastructureHealthChecker
	panicTracker       *panicRecovery
	actionExecutor     *execution.ActionExecutor
	ctxCancel          context.CancelFunc
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
	deps                     map[string]any
	validatedSpecHashes      map[string]string // name -> hash of last validated spec
	noStateMachineLoggedOnce sync.Map
	userSpec                 config.UserSpec
	workerType               string
	mappedParentState string
	parentID          string
	lastUserSpecHash  string
	collectorHealth   CollectorHealth
	metricsWg                sync.WaitGroup
	tickInterval             time.Duration
	tickCount                uint64
	gracefulShutdownTimeout  time.Duration
	metricsReportInterval    time.Duration
	childShutdownTimeout     time.Duration
	circuitOpen              atomic.Bool
	panicCircuitOpen         atomic.Bool
	started                  atomic.Bool
	noWorkersWarnedOnce      atomic.Bool
	enableTraceLogging       bool
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

	cfg.Logger.Info("timeout_configuration",
		deps.String("worker", cfg.WorkerType),
		deps.Duration("observation_timeout", observationTimeout),
		deps.Duration("stale_threshold", staleThreshold),
		deps.Duration("collector_timeout", timeout))

	freshnessChecker := health.NewFreshnessChecker(staleThreshold, timeout, cfg.WorkerType, cfg.Logger)

	lm := lockmanager.NewLockManager()

	gracefulShutdownTimeout := cfg.GracefulShutdownTimeout
	if gracefulShutdownTimeout == 0 {
		gracefulShutdownTimeout = DefaultGracefulShutdownTimeout
	}

	metricsReportInterval := cfg.MetricsReportInterval
	if metricsReportInterval == 0 {
		metricsReportInterval = DefaultMetricsReportInterval
	}

	childShutdownTimeout := cfg.ChildShutdownTimeout
	if childShutdownTimeout == 0 {
		childShutdownTimeout = DefaultChildShutdownTimeout
	}

	return &Supervisor[TObserved, TDesired]{
		workerType:         cfg.WorkerType,
		workers:            make(map[string]*WorkerContext[TObserved, TDesired]),
		lockManager:        lm,
		mu:                 lm.NewLock(lockNameSupervisorMu, lockLevelSupervisorMu),
		ctxMu:              lm.NewLock(lockNameSupervisorCtxMu, lockLevelSupervisorCtxMu),
		store:              cfg.Store,
		logger:             cfg.Logger,
		baseLogger:         cfg.Logger,
		tickInterval:       tickInterval,
		freshnessChecker:   freshnessChecker,
		children:           make(map[string]SupervisorInterface),
		childDoneChans:     make(map[string]<-chan struct{}),
		pendingRemoval:     make(map[string]bool),
		pendingRestart:     make(map[string]bool),
		restartRequestedAt: make(map[string]time.Time),
		createdAt:          time.Now(),
		parentID:           "",
		healthChecker:      NewInfrastructureHealthChecker(DefaultMaxInfraRecoveryAttempts, DefaultRecoveryAttemptWindow),
		panicTracker:       newPanicRecovery(DefaultPanicEscalationWindow, DefaultMaxTickPanics),
		actionExecutor:     execution.NewActionExecutor(10, cfg.WorkerType, deps.Identity{WorkerType: cfg.WorkerType}, cfg.Logger),
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
		metricsReportInterval:   metricsReportInterval,
		childShutdownTimeout:    childShutdownTimeout,
		deps:                    ensureNonNilDeps(cfg.Dependencies),
		validatedSpecHashes:     make(map[string]string),
	}
}

// ensureNonNilDeps returns deps if non-nil, or an initialized empty map.
// Worker factories may write to the deps map (e.g., to pass dependencies to child workers),
// so it must always be non-nil. This is consistent with how other maps in the supervisor
// (children, pendingRemoval, etc.) are always initialized.
func ensureNonNilDeps(deps map[string]any) map[string]any {
	if deps == nil {
		return make(map[string]any)
	}

	return deps
}
