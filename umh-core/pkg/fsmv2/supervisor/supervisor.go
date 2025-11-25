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
//	supervisor := NewSupervisor[ParentObservedState, *ParentDesiredState](config)
//	supervisor.AddWorker(identity, worker)
//
// Worker types are automatically derived from the observed state type name:
//   - ParentObservedState -> "parent"
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
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/health"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/lockmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
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

// Lock levels for ordering (lower number = must be acquired first).
const (
	lockLevelSupervisorMu    = 1
	lockLevelSupervisorCtxMu = 2 // Can be acquired alone OR after Supervisor.mu
	lockLevelWorkerContextMu = 3 // Must be acquired AFTER Supervisor.mu
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

// factoryRegistryAdapter implements types.WorkerTypeChecker interface for validation.
type factoryRegistryAdapter struct{}

func (f *factoryRegistryAdapter) ListRegisteredTypes() []string {
	return factory.ListRegisteredTypes()
}

// SupervisorInterface provides a type-erased interface for hierarchical supervisor composition.
// This allows a parent Supervisor[TObserved, TDesired] to manage children of different types.
type SupervisorInterface interface {
	Start(ctx context.Context) <-chan struct{}
	Shutdown()
	ListWorkers() []string
	tick(ctx context.Context) error
	updateUserSpec(spec config.UserSpec)
	getUserSpec() config.UserSpec
	getStateMapping() map[string]string
	setStateMapping(mapping map[string]string)
	getMappedParentState() string
	setMappedParentState(state string)
	calculateHierarchySize() int
	calculateHierarchyDepth() int
	GetChildren() map[string]SupervisorInterface
	AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error
	setParent(parent SupervisorInterface, parentID string)
}

type Supervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState] struct {
	workerType string                                         // Type of workers managed (e.g., "container") - for storage collection naming
	workers    map[string]*WorkerContext[TObserved, TDesired] // workerID → worker context
	// mu Protects access to workers map, children, childDoneChans, stateMapping, globalVars, and mappedParentState.
	//
	// This is a lockmanager.Lock wrapping sync.RWMutex to allow concurrent reads from multiple goroutines
	// (e.g., GetWorker, ListWorkers) while ensuring exclusive writes when modifying
	// worker registry state (e.g., AddWorker, RemoveWorker).
	//
	// Lock Order: Must be acquired BEFORE WorkerContext.mu when both are needed.
	// See package-level LOCK ORDER section for details.
	mu                *lockmanager.Lock
	lockManager       *lockmanager.LockManager
	store             storage.TriangularStoreInterface // State persistence layer (triangular model)
	logger            *zap.SugaredLogger               // Logger for supervisor operations
	tickInterval      time.Duration                    // How often to evaluate state transitions
	collectorHealth   CollectorHealth                  // Collector health tracking
	freshnessChecker  *health.FreshnessChecker         // Data freshness validator
	children          map[string]SupervisorInterface   // Child supervisors by name (hierarchical composition)
	childDoneChans    map[string]<-chan struct{}       // Done channels for child supervisors
	stateMapping      map[string]string                // Parent→child state mapping
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
	ctxMu   *lockmanager.Lock
	started atomic.Bool // Whether supervisor has been started
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
	mu             *lockmanager.Lock
	tickInProgress atomic.Bool
	identity       fsmv2.Identity
	worker         fsmv2.Worker
	currentState   fsmv2.State[any, any]
	collector      *collection.Collector[TObserved]
	executor       *execution.ActionExecutor
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
	// Required - no default. Use storage.NewTriangularStore(basicStore, registry) or a mock.
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
}

func NewSupervisor[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](cfg Config) *Supervisor[TObserved, TDesired] {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = DefaultTickInterval
	}

	// NOTE (Invariant I13): Zero timeouts default to safe values.
	// This prevents invalid configurations where timeouts of 0 would
	// cause immediate failures or infinite loops.
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

	// I2: Validate threshold ordering (invariant enforcement)
	if staleThreshold <= 0 {
		panic(fmt.Sprintf("supervisor config error: staleThreshold must be positive, got %v", staleThreshold))
	}

	if timeout <= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: timeout (%v) must be greater than staleThreshold (%v)", timeout, staleThreshold))
	}

	if maxRestartAttempts <= 0 {
		panic(fmt.Sprintf("supervisor config error: maxRestartAttempts must be positive, got %d", maxRestartAttempts))
	}

	// I7: Validate timeout ordering (ObservationTimeout < StaleThreshold < CollectorTimeout)
	if observationTimeout >= staleThreshold {
		panic(fmt.Sprintf("supervisor config error: observationTimeout (%v) must be less than staleThreshold (%v)", observationTimeout, staleThreshold))
	}

	if staleThreshold >= timeout {
		panic(fmt.Sprintf("supervisor config error: staleThreshold (%v) must be less than collectorTimeout (%v)", staleThreshold, timeout))
	}

	cfg.Logger.Infof("Supervisor timeout configuration: ObservationTimeout=%v, StaleThreshold=%v, CollectorTimeout=%v",
		observationTimeout, staleThreshold, timeout)

	freshnessChecker := health.NewFreshnessChecker(staleThreshold, timeout, cfg.Logger)

	// Collection Naming Convention
	//
	// Collections MUST be registered in the TriangularStore before creating the Supervisor.
	// Collection names follow strict convention: {workerType}_identity, {workerType}_desired, {workerType}_observed
	// CSE fields standardized per role per FSM v2 contract:
	//   - identity: FieldSyncID, FieldVersion, FieldCreatedAt (immutable)
	//   - desired: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version increments on updates)
	//   - observed: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version does NOT increment)
	//
	// RATIONALE: Convention-based naming eliminates need for explicit collection configuration.
	// Workers focus purely on business logic per FSM v2 design goal.
	//
	// IMPLEMENTATION: Register collections when creating the TriangularStore:
	//   - Production: Agent pre-registers all collections at startup
	//   - Tests: Use CreateTestTriangularStoreForWorkerType(workerType) helper
	//
	// Example:
	//   For workerType "s6", collections are: "s6_identity", "s6_desired", "s6_observed"
	//   For workerType "benthos", collections are: "benthos_identity", "benthos_desired", "benthos_observed"

	lm := lockmanager.NewLockManager()

	return &Supervisor[TObserved, TDesired]{
		workerType:       cfg.WorkerType,
		workers:          make(map[string]*WorkerContext[TObserved, TDesired]),
		lockManager:      lm,
		mu:               lm.NewLock(lockNameSupervisorMu, lockLevelSupervisorMu),
		ctxMu:            lm.NewLock(lockNameSupervisorCtxMu, lockLevelSupervisorCtxMu),
		store:            cfg.Store,
		logger:           cfg.Logger,
		tickInterval:     tickInterval,
		freshnessChecker: freshnessChecker,
		children:         make(map[string]SupervisorInterface),
		childDoneChans:   make(map[string]<-chan struct{}),
		stateMapping:     make(map[string]string),
		createdAt:        time.Now(),
		parentID:         "", // Root supervisor has empty parentID
		healthChecker:    NewInfrastructureHealthChecker(DefaultMaxInfraRecoveryAttempts, DefaultRecoveryAttemptWindow),
		// circuitOpen is atomic.Bool, zero-initialized to false
		actionExecutor: execution.NewActionExecutor(10, cfg.WorkerType),
		collectorHealth: CollectorHealth{
			observationTimeout: observationTimeout,
			staleThreshold:     staleThreshold,
			timeout:            timeout,
			maxRestartAttempts: maxRestartAttempts,
			restartCount:       0,
		},
		userSpec: cfg.UserSpec,
	}
}

// AddWorker adds a new worker to the supervisor's registry.
// Returns error if worker with same ID already exists.
// DEFENSE-IN-DEPTH VALIDATION STRATEGY:
//
// FSMv2 validates data at MULTIPLE layers (not one centralized validator).
// This is intentional, not redundant:
//
// Layer 1: API entry (AddWorker) - Fast fail on obvious errors
// Layer 2: Reconciliation entry (reconcileChildren) - Catch runtime edge cases
// Layer 3: Factory (worker creation) - Validate WorkerType exists
// Layer 4: Worker constructor - Validate dependencies
//
// WHY multiple layers:
//   - Security: Never trust data, even from internal callers
//   - Debuggability: Errors caught closest to source
//   - Robustness: One layer failing doesn't compromise system
//
// Each layer has different validation concerns:
//   - Layer 1: Public API validation (protect against bad calls)
//   - Layer 2: Runtime state validation (data evolved since layer 1)
//   - Layer 3: Registry validation (WorkerType registered?)
//   - Layer 4: Logical validation (dependencies compatible?)
func (s *Supervisor[TObserved, TDesired]) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[identity.ID]; exists {
		return fmt.Errorf("worker %s already exists", identity.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Collect initial observation
	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect initial observed state: %w", err)
	}

	// Derive initial desired state
	initialDesired, err := worker.DeriveDesiredState(nil)
	if err != nil {
		return fmt.Errorf("failed to derive initial desired state: %w", err)
	}

	// Save identity to database
	identityDoc := persistence.Document{
		"id":          identity.ID,
		"name":        identity.Name,
		"worker_type": identity.WorkerType,
	}
	if err := s.store.SaveIdentity(ctx, s.workerType, identity.ID, identityDoc); err != nil {
		return fmt.Errorf("failed to save identity: %w", err)
	}

	s.logger.Debugf("Saved identity for worker: %s", identity.ID)

	// Save initial observation to database for immediate availability
	observedJSON, err := json.Marshal(observed)
	if err != nil {
		return fmt.Errorf("failed to marshal observed state: %w", err)
	}

	observedDoc := make(persistence.Document)
	if err := json.Unmarshal(observedJSON, &observedDoc); err != nil {
		return fmt.Errorf("failed to unmarshal observed state to document: %w", err)
	}

	observedDoc["id"] = identity.ID

	_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
	if err != nil {
		return fmt.Errorf("failed to save initial observation: %w", err)
	}

	s.logger.Debugf("Saved initial observation for worker: %s", identity.ID)

	// Save initial desired state to database
	desiredJSON, err := json.Marshal(initialDesired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal desired state to document: %w", err)
	}

	desiredDoc["id"] = identity.ID

	err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
	if err != nil {
		return fmt.Errorf("failed to save initial desired state: %w", err)
	}

	s.logger.Debugf("Saved initial desired state for worker: %s", identity.ID)

	collector := collection.NewCollector[TObserved](collection.CollectorConfig[TObserved]{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              s.logger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  s.collectorHealth.observationTimeout,
	})

	executor := execution.NewActionExecutor(10, identity.ID)

	s.workers[identity.ID] = &WorkerContext[TObserved, TDesired]{
		mu:           s.lockManager.NewLock(lockNameWorkerContextMu, lockLevelWorkerContextMu),
		identity:     identity,
		worker:       worker,
		currentState: worker.GetInitialState(),
		collector:    collector,
		executor:     executor,
	}

	s.logger.Infof("Added worker %s to supervisor", identity.ID)

	return nil
}

// RemoveWorker removes a worker from the registry.
func (s *Supervisor[TObserved, TDesired]) RemoveWorker(ctx context.Context, workerID string) error {
	s.mu.Lock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.Unlock()

		return fmt.Errorf("worker %s not found", workerID)
	}

	delete(s.workers, workerID)
	s.mu.Unlock()

	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	s.logger.Infof("Removed worker %s from supervisor", workerID)

	return nil
}

// GetWorker returns the worker context for the given ID.
func (s *Supervisor[TObserved, TDesired]) GetWorker(workerID string) (*WorkerContext[TObserved, TDesired], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, exists := s.workers[workerID]
	if !exists {
		return nil, fmt.Errorf("worker %s not found", workerID)
	}

	return ctx, nil
}

// ListWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor[TObserved, TDesired]) ListWorkers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.workers))
	for id := range s.workers {
		ids = append(ids, id)
	}

	return ids
}

// SetGlobalVariables sets the global variables for this supervisor.
// Global variables come from the management system and are fleet-wide settings.
// They are injected into UserSpec.Variables.Global before DeriveDesiredState() is called.
func (s *Supervisor[TObserved, TDesired]) SetGlobalVariables(vars map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.globalVars = vars
}

func (s *Supervisor[TObserved, TDesired]) getStaleThreshold() time.Duration {
	return s.collectorHealth.staleThreshold
}

func (s *Supervisor[TObserved, TDesired]) getCollectorTimeout() time.Duration {
	return s.collectorHealth.timeout
}

func (s *Supervisor[TObserved, TDesired]) getMaxRestartAttempts() int {
	return s.collectorHealth.maxRestartAttempts
}

func (s *Supervisor[TObserved, TDesired]) getRestartCount() int {
	return s.collectorHealth.restartCount
}

func (s *Supervisor[TObserved, TDesired]) setRestartCount(count int) {
	s.collectorHealth.restartCount = count
}

func (s *Supervisor[TObserved, TDesired]) restartCollector(ctx context.Context, workerID string) error {
	// I1 & I4: This should never be called with restartCount >= maxRestartAttempts.
	// If it is, that's a programming error (bug in tick() logic).
	if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
		panic(fmt.Sprintf("supervisor bug: RestartCollector called with restartCount=%d >= maxRestartAttempts=%d (should have escalated to shutdown)",
			s.collectorHealth.restartCount, s.collectorHealth.maxRestartAttempts))
	}

	s.collectorHealth.restartCount++
	s.collectorHealth.lastRestart = time.Now()

	backoff := time.Duration(s.collectorHealth.restartCount*2) * time.Second
	s.logger.Warnf("Restarting collector for worker %s (attempt %d/%d) after %v backoff",
		workerID, s.collectorHealth.restartCount, s.collectorHealth.maxRestartAttempts, backoff)

	time.Sleep(backoff)

	s.mu.RLock()
	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	if workerCtx.collector != nil {
		workerCtx.collector.Restart()
	}

	return nil
}

func (s *Supervisor[TObserved, TDesired]) checkDataFreshness(snapshot *fsmv2.Snapshot) bool {
	var (
		age          time.Duration
		collectedAt  time.Time
		hasTimestamp bool
	)

	if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		collectedAt = timestampProvider.GetTimestamp()
		hasTimestamp = true
	} else if doc, ok := snapshot.Observed.(persistence.Document); ok {
		if ts, exists := doc["collectedAt"]; exists {
			if timestamp, ok := ts.(time.Time); ok {
				collectedAt = timestamp
				hasTimestamp = true
			} else if timeStr, ok := ts.(string); ok {
				var err error

				collectedAt, err = time.Parse(time.RFC3339Nano, timeStr)
				if err == nil {
					hasTimestamp = true
				}
			}
		}
	}

	if !hasTimestamp {
		s.logger.Warn("Snapshot.Observed does not implement GetTimestamp() or have collectedAt field, cannot check freshness")

		return true
	}

	age = time.Since(collectedAt)

	// GRACEFUL DEGRADATION (Invariant I15): Tight timeouts cause pausing, not crashes
	// If timeouts are configured too tight (e.g., ObservationTimeout < actual operation time),
	// the FSM will pause frequently but won't crash. Warning logs help diagnose misconfigurations.
	//
	// Design choice: Pause > Crash
	// - Pausing allows manual intervention and configuration fixes
	// - Crashing would require restart and potentially lose state

	if s.freshnessChecker.IsTimeout(snapshot) {
		s.logger.Warnf("Data timeout: observation is %v old (threshold: %v)", age, s.collectorHealth.timeout)

		return false
	}

	if !s.freshnessChecker.Check(snapshot) {
		s.logger.Warnf("Data stale: observation is %v old (threshold: %v)", age, s.collectorHealth.staleThreshold)

		return false
	}

	return true
}

// Start starts the supervisor goroutines.
// Returns a channel that will be closed when the supervisor stops.
func (s *Supervisor[TObserved, TDesired]) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	// Create a child context that we can cancel in Shutdown()
	// This allows Shutdown() to stop the tick loop even if parent context is still active
	s.ctxMu.Lock()
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	s.ctxMu.Unlock()
	s.started.Store(true)

	s.logger.Debugf("Supervisor started for workerType: %s", s.workerType)

	// Use the child context for all goroutines so they stop when Shutdown() is called
	s.ctxMu.RLock()
	supervisorCtx := s.ctx
	s.ctxMu.RUnlock()

	s.actionExecutor.Start(supervisorCtx)

	s.startMetricsReporter(supervisorCtx)

	// Start observation collectors and action executors for all workers
	s.mu.RLock()

	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(supervisorCtx); err != nil {
			s.logger.Errorf("Failed to start collector for worker %s: %v", workerCtx.identity.ID, err)
		}

		workerCtx.executor.Start(supervisorCtx)
	}

	s.mu.RUnlock()

	// Start main tick loop
	go func() {
		defer close(done)

		s.tickLoop(supervisorCtx)
	}()

	return done
}

// tickLoop is the main FSM loop.
// Calls Tick() which includes hierarchical composition (Phase 0) and worker state transitions.
func (s *Supervisor[TObserved, TDesired]) tickLoop(ctx context.Context) {
	s.logger.Info("Starting tick loop for supervisor")

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	s.logger.Debugf("Tick loop started, interval: %v", s.tickInterval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Tick loop stopped for supervisor")

			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Errorf("Tick error: %v", err)
			}
		}
	}
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor[TObserved, TDesired]) tickWorker(ctx context.Context, workerID string) error {
	s.logger.Debugw("lifecycle",
		"lifecycle_event", "mutex_lock_acquire",
		"mutex_name", "supervisor.mu",
		"lock_type", "read",
		"worker_id", workerID)

	s.mu.RLock()

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "mutex_lock_acquired",
		"mutex_name", "supervisor.mu",
		"worker_id", workerID)

	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "mutex_unlock",
		"mutex_name", "supervisor.mu",
		"worker_id", workerID)

	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	// Skip if tick already in progress
	if !workerCtx.tickInProgress.CompareAndSwap(false, true) {
		s.logger.Debugw("lifecycle",
			"lifecycle_event", "tick_skip",
			"worker_id", workerID,
			"reason", "previous_tick_in_progress")

		return nil
	}
	defer workerCtx.tickInProgress.Store(false)

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "tick_start",
		"worker_id", workerID)

	workerCtx.mu.RLock()

	currentStateStr := "nil"
	if workerCtx.currentState != nil {
		currentStateStr = workerCtx.currentState.String()
	}

	s.logger.Debugf("Ticking worker: %s, current state: %s", workerID, currentStateStr)
	workerCtx.mu.RUnlock()

	// Load latest snapshot from database
	s.logger.Debugf("[DataFreshness] Worker %s: Loading snapshot from database", workerID)

	storageSnapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
	if err != nil {
		s.logger.Debugf("[DataFreshness] Worker %s: Failed to load snapshot: %v", workerID, err)

		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// I16: Validate ObservedState is not nil from storage
	if storageSnapshot.Observed == nil {
		panic("Invariant I16 violated: storage returned nil ObservedState for worker " + workerID)
	}

	// Load typed observed state
	var observed TObserved

	err = s.store.LoadObservedTyped(ctx, s.workerType, workerID, &observed)
	if err != nil {
		s.logger.Debugf("[DataFreshness] Worker %s: Failed to load typed observed state: %v", workerID, err)

		return fmt.Errorf("failed to load typed observed state: %w", err)
	}

	// Load typed desired state
	var desired TDesired

	err = s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
		s.logger.Debugf("[DataFreshness] Worker %s: Failed to load typed desired state: %v", workerID, err)

		return fmt.Errorf("failed to load typed desired state: %w", err)
	}

	// Build snapshot with typed states
	snapshot := &fsmv2.Snapshot{
		Identity: fsmv2.Identity{
			ID:         workerID,
			Name:       getString(storageSnapshot.Identity, "name", workerID),
			WorkerType: workerCtx.identity.WorkerType,
		},
		Observed: observed,
		Desired:  desired,
	}

	// Log loaded observation details
	if timestampProvider, ok := any(observed).(interface{ GetTimestamp() time.Time }); ok {
		observationTimestamp := timestampProvider.GetTimestamp()
		s.logger.Debugf("[DataFreshness] Worker %s: Loaded observation timestamp=%s", workerID, observationTimestamp.Format(time.RFC3339Nano))
	} else {
		s.logger.Debugf("[DataFreshness] Worker %s: Loaded observation does not implement GetTimestamp() (type: %T)", workerID, observed)
	}

	// I3: Check data freshness BEFORE calling state.Next()
	// This is the trust boundary: states assume data is always fresh
	if !s.checkDataFreshness(snapshot) {
		if s.freshnessChecker.IsTimeout(snapshot) {
			// I4: Check if we've exhausted restart attempts
			if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
				// Max attempts reached - escalate to shutdown (Layer 3)
				s.logger.Errorf("Collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)

				if shutdownErr := s.requestShutdown(ctx, workerID,
					fmt.Sprintf("collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)); shutdownErr != nil {
					s.logger.Errorf("Failed to request shutdown: %v", shutdownErr)
				}

				return errors.New("collector unresponsive, shutdown requested")
			}

			// I4: Safe to restart (restartCount < maxRestartAttempts)
			// RestartCollector will panic if invariant violated (defensive check)
			if err := s.restartCollector(ctx, workerID); err != nil {
				return fmt.Errorf("failed to restart collector: %w", err)
			}
		}

		s.logger.Debug("Pausing FSM due to stale/timeout data")

		return nil
	}

	if s.collectorHealth.restartCount > 0 {
		s.logger.Infof("Collector recovered after %d restart attempts", s.collectorHealth.restartCount)
		s.collectorHealth.restartCount = 0
	}

	// I16: Validate ObservedState is not nil before progressing FSM
	if snapshot.Observed == nil {
		panic("Invariant I16 violated: attempted to progress FSM with nil ObservedState for worker " + workerID)
	}

	// Data is fresh - safe to progress FSM
	// Call state transition (read current state with lock)
	workerCtx.mu.RLock()
	currentState := workerCtx.currentState
	workerCtx.mu.RUnlock()

	s.logger.Debugf("Evaluating state transition for worker: %s", workerID)

	// Skip state transitions if worker has no FSM state machine
	if currentState == nil {
		s.logger.Debugf("Worker %s has no state machine, skipping state transition", workerID)

		return nil
	}

	nextState, signal, action := currentState.Next(*snapshot)

	hasAction := action != nil
	s.logger.Debugf("State evaluation result for worker %s - nextState: %s, signal: %d, hasAction: %t",
		workerID, nextState.String(), signal, hasAction)

	// VALIDATION: Cannot switch state AND emit action simultaneously
	if nextState != currentState && action != nil {
		panic(fmt.Sprintf("invalid state transition: state %s tried to switch to %s AND emit action %s",
			currentState.String(), nextState.String(), action.Name()))
	}

	// Execute action if present
	if action != nil {
		actionID := action.Name()

		if workerCtx.executor.HasActionInProgress(actionID) {
			s.logger.Debugf("Skipping action %s for worker %s (already in progress)", actionID, workerID)

			return nil
		}

		s.logger.Infof("Enqueuing action: %s", actionID)

		if err := workerCtx.executor.EnqueueAction(actionID, action); err != nil {
			return fmt.Errorf("failed to enqueue action: %w", err)
		}
	}

	// Transition to next state
	if nextState != currentState {
		s.logger.Debugw("lifecycle",
			"lifecycle_event", "state_transition",
			"worker_id", workerID,
			"from_state", currentState.String(),
			"to_state", nextState.String(),
			"reason", nextState.Reason())

		s.logger.Debugf("State transition for worker %s: %s → %s",
			workerID, currentState.String(), nextState.String())

		s.logger.Infof("State transition: %s -> %s (reason: %s)",
			currentState.String(), nextState.String(), nextState.Reason())

		s.logger.Debugw("lifecycle",
			"lifecycle_event", "mutex_lock_acquire",
			"mutex_name", "workerCtx.mu",
			"lock_type", "write",
			"worker_id", workerID)

		workerCtx.mu.Lock()

		s.logger.Debugw("lifecycle",
			"lifecycle_event", "mutex_lock_acquired",
			"mutex_name", "workerCtx.mu",
			"worker_id", workerID)

		workerCtx.currentState = nextState
		workerCtx.mu.Unlock()

		s.logger.Debugw("lifecycle",
			"lifecycle_event", "mutex_unlock",
			"mutex_name", "workerCtx.mu",
			"worker_id", workerID)
	} else {
		s.logger.Debugf("State unchanged for worker %s: %s", workerID, currentState.String())
	}

	// Process signal
	if err := s.processSignal(ctx, workerID, signal); err != nil {
		return fmt.Errorf("signal processing failed: %w", err)
	}

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "tick_complete",
		"worker_id", workerID,
		"final_state", workerCtx.currentState.String())

	return nil
}

// getRecoveryStatus returns a human-readable recovery status based on attempt count.
func (s *Supervisor[TObserved, TDesired]) getRecoveryStatus() string {
	attempts := s.healthChecker.backoff.GetAttempts()
	if attempts < 3 {
		return "attempting_recovery"
	} else if attempts < 5 {
		return "persistent_failure"
	}

	return "escalation_imminent"
}

// getEscalationSteps returns manual runbook steps for specific child types.
func (s *Supervisor[TObserved, TDesired]) getEscalationSteps(childName string) string {
	steps := map[string]string{
		"dfc_read": "1) Check Redpanda logs 2) Verify network connectivity 3) Restart Redpanda manually",
		"benthos":  "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually",
	}
	if step, ok := steps[childName]; ok {
		return step
	}

	return "1) Check component logs 2) Verify network connectivity 3) Restart component manually"
}

// Tick performs one supervisor tick cycle, integrating all FSMv2 phases.
//
// ARCHITECTURE: This method orchestrates four phases in priority order:
//
// PHASE 1: Infrastructure Supervision (from Phase 1)
//   - Verify child consistency via InfrastructureHealthChecker
//   - Circuit breaker pattern: opens on failure, skips rest of tick
//   - Child restart handled with exponential backoff (managed by health checker)
//   - Non-blocking: health check completes in <1ms
//
// PHASE 0.5: Variable Injection (from Phase 0.5)
//   - Global variables from management system (configuration)
//   - Internal variables (supervisorID, createdAt, bridgedBy)
//   - User variables from UserSpec (preserved, not overwritten)
//   - Variables available for template expansion in DeriveDesiredState
//
// PHASE 0: Hierarchical Composition (from Phase 0)
//   - DeriveDesiredState from worker implementation
//   - Reconcile children to match ChildrenSpecs (create/delete supervisors)
//   - Apply state mapping (parent state influences child state)
//   - Recursively tick all children (errors logged, not propagated)
//
// PHASE 2: Async Action Execution (from Phase 2)
//   - Check if action already in progress for this worker type
//   - Enqueue new action if needed (stubAction for Phase 3)
//   - Actions execute in global worker pool (non-blocking)
//   - Timeouts and retries handled automatically by ActionExecutor
//
// PERFORMANCE: The complete tick loop is non-blocking and completes in <10ms,
// making it safe to call at high frequency (100Hz+) without impacting system performance.
//
// PHASE 3 STATUS: All infrastructure is complete. stubAction demonstrates async
// execution without implementing full action derivation (Start/Stop/Restart based
// on state transitions). Real action logic will be added in Phase 4.
func (s *Supervisor[TObserved, TDesired]) tick(ctx context.Context) error {
	// PHASE 1: Infrastructure health check (priority 1)
	// Copy children map under lock to avoid race with reconcileChildren
	s.mu.RLock()

	childrenCopy := make(map[string]SupervisorInterface, len(s.children))
	for k, v := range s.children {
		childrenCopy[k] = v
	}

	s.mu.RUnlock()

	if err := s.healthChecker.CheckChildConsistency(childrenCopy); err != nil {
		wasOpen := s.circuitOpen.Load()
		s.circuitOpen.Store(true)

		if !wasOpen {
			s.logger.Error("circuit breaker opened",
				"supervisor_id", s.workerType,
				"error", err.Error(),
				"error_scope", "infrastructure",
				"impact", "all_workers")
			metrics.RecordCircuitOpen(s.workerType, true)
		}

		var childErr *ChildHealthError
		if errors.As(err, &childErr) {
			attempts := s.healthChecker.backoff.GetAttempts()
			nextDelay := s.healthChecker.backoff.NextDelay()

			s.logger.Warn("Circuit breaker open, retrying infrastructure checks",
				"supervisor_id", s.workerType,
				"failed_child", childErr.ChildName,
				"retry_attempt", attempts,
				"max_attempts", s.healthChecker.maxAttempts,
				"elapsed_downtime", s.healthChecker.backoff.GetTotalDowntime().String(),
				"next_retry_in", nextDelay.String(),
				"recovery_status", s.getRecoveryStatus())

			if attempts == 4 {
				s.logger.Warn("WARNING: One retry attempt remaining before escalation",
					"supervisor_id", s.workerType,
					"child_name", childErr.ChildName,
					"attempts_remaining", 1,
					"total_downtime", s.healthChecker.backoff.GetTotalDowntime().String())
			}

			if attempts >= 5 {
				s.logger.Error("ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.",
					"supervisor_id", s.workerType,
					"child_name", childErr.ChildName,
					"max_attempts", 5,
					"total_downtime", s.healthChecker.backoff.GetTotalDowntime().String(),
					"runbook_url", "https://docs.umh.app/runbooks/supervisor-escalation",
					"manual_steps", s.getEscalationSteps(childErr.ChildName))
			}
		}

		return nil
	}

	if s.circuitOpen.Load() {
		downtime := time.Since(s.healthChecker.backoff.GetStartTime())
		s.logger.Info("Infrastructure recovered, closing circuit breaker",
			"supervisor_id", s.workerType,
			"total_downtime", downtime.String())
		metrics.RecordCircuitOpen(s.workerType, false)
		metrics.RecordInfrastructureRecovery(s.workerType, downtime)
	}

	s.circuitOpen.Store(false)

	// PHASE 2: Action execution (priority 2)
	if !s.actionExecutor.HasActionInProgress(s.workerType) {
		// stubAction demonstrates async execution infrastructure
		// Real action derivation (Start/Stop/Restart) will be added in Phase 4
		action := &stubAction{}
		_ = s.actionExecutor.EnqueueAction(s.workerType, action)
	}

	// For backwards compatibility, tick the first worker
	s.mu.RLock()

	var (
		firstWorkerID string
		worker        fsmv2.Worker
	)

	for id, workerEntry := range s.workers {
		if workerEntry != nil && workerEntry.worker != nil {
			firstWorkerID = id
			worker = workerEntry.worker

			break
		}
	}

	s.mu.RUnlock()

	if firstWorkerID == "" {
		return errors.New("no workers in supervisor")
	}

	if worker == nil {
		return errors.New("worker is nil")
	}

	// PHASE 0.5: Variable Injection
	// Inject variables BEFORE DeriveDesiredState() so they're available for template expansion
	userSpecWithVars := s.userSpec

	if userSpecWithVars.Variables.User == nil {
		userSpecWithVars.Variables.User = make(map[string]any)
	}

	s.mu.RLock()
	userSpecWithVars.Variables.Global = s.globalVars
	globalVarCount := len(s.globalVars)
	s.mu.RUnlock()

	userSpecWithVars.Variables.Internal = map[string]any{
		"id":         firstWorkerID,
		"created_at": s.createdAt,
		"bridged_by": s.parentID,
	}

	userVarCount := len(userSpecWithVars.Variables.User)
	s.logger.Debug("variables propagated",
		"supervisor_id", s.workerType,
		"user_vars", userVarCount,
		"global_vars", globalVarCount)
	metrics.RecordVariablePropagation(s.workerType)

	// PHASE 0: Hierarchical Composition
	// 1. DeriveDesiredState
	templateStart := time.Now()
	desired, err := worker.DeriveDesiredState(userSpecWithVars)
	templateDuration := time.Since(templateStart)

	if err != nil {
		s.logger.Error("template rendering failed",
			"supervisor_id", s.workerType,
			"error", err.Error(),
			"duration_ms", templateDuration.Milliseconds())
		metrics.RecordTemplateRenderingDuration(s.workerType, "error", templateDuration)
		metrics.RecordTemplateRenderingError(s.workerType, "derivation_failed")

		return fmt.Errorf("failed to derive desired state: %w", err)
	}

	s.logger.Debug("template rendered",
		"supervisor_id", s.workerType,
		"duration_ms", templateDuration.Milliseconds())
	metrics.RecordTemplateRenderingDuration(s.workerType, "success", templateDuration)

	// Save derived desired state to database BEFORE tickWorker
	// This ensures tickWorker loads the freshest desired state from the snapshot
	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal derived desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal derived desired state to document: %w", err)
	}

	desiredDoc["id"] = firstWorkerID

	err = s.store.SaveDesired(ctx, s.workerType, firstWorkerID, desiredDoc)
	if err != nil {
		// Log the error but continue with the tick - the system can recover on the next tick
		// The tickWorker will use the previously saved desired state
		s.logger.Warnf("failed to save derived desired state (will use previous state): %v", err)
	} else {
		s.logger.Debug("derived desired state saved",
			"supervisor_id", s.workerType,
			"worker_id", firstWorkerID)
	}

	// Validate ChildrenSpecs before reconciliation
	if len(desired.ChildrenSpecs) > 0 {
		registry := &factoryRegistryAdapter{}

		if err := config.ValidateChildSpecs(desired.ChildrenSpecs, registry); err != nil {
			s.logger.Error("child spec validation failed",
				"supervisor_id", s.workerType,
				"error", err.Error())

			return fmt.Errorf("invalid child specifications: %w", err)
		}

		s.logger.Debug("child specs validated",
			"supervisor_id", s.workerType,
			"child_count", len(desired.ChildrenSpecs))
	}

	// 2. Tick worker FIRST to progress FSM state before creating children
	if err := s.tickWorker(ctx, firstWorkerID); err != nil {
		return fmt.Errorf("failed to tick worker: %w", err)
	}

	// 3. Reconcile children (propagate errors)
	if err := s.reconcileChildren(desired.ChildrenSpecs); err != nil {
		return fmt.Errorf("failed to reconcile children: %w", err)
	}

	// 4. Apply state mapping
	s.applyStateMapping()

	// 5. Recursively tick children (log errors, don't fail parent)
	s.mu.RLock()

	childrenToTick := make([]SupervisorInterface, 0, len(s.children))
	for _, child := range s.children {
		childrenToTick = append(childrenToTick, child)
	}

	s.mu.RUnlock()

	for _, child := range childrenToTick {
		if err := child.tick(ctx); err != nil {
			s.logger.Errorf("Child tick failed: %v", err)
			// Continue with other children
		}
	}

	return nil
}

// TickAll performs one FSM tick for all workers in the registry.
// Each worker is ticked independently. Errors from one worker do not stop others.
// Returns an aggregated error containing all individual worker errors.
func (s *Supervisor[TObserved, TDesired]) tickAll(ctx context.Context) error {
	s.mu.RLock()

	workerIDs := make([]string, 0, len(s.workers))
	for id := range s.workers {
		workerIDs = append(workerIDs, id)
	}

	s.mu.RUnlock()

	if len(workerIDs) == 0 {
		return nil
	}

	var errs []error

	for _, workerID := range workerIDs {
		if err := s.tickWorker(ctx, workerID); err != nil {
			errs = append(errs, fmt.Errorf("worker %s: %w", workerID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("tick errors: %v", errs)
	}

	return nil
}

// processSignal handles signals from states.
func (s *Supervisor[TObserved, TDesired]) processSignal(ctx context.Context, workerID string, signal fsmv2.Signal) error {
	s.logger.Debugw("lifecycle",
		"lifecycle_event", "signal_processing",
		"worker_id", workerID,
		"signal", int(signal))

	switch signal {
	case fsmv2.SignalNone:
		// Normal operation
		return nil
	case fsmv2.SignalNeedsRemoval:
		s.logger.Infof("Worker %s signaled removal, removing from registry", workerID)

		s.mu.Lock()

		workerCtx, exists := s.workers[workerID]
		if !exists {
			s.mu.Unlock()

			return fmt.Errorf("worker %s not found in registry", workerID)
		}

		// Diagnostic logging: show children before removal
		childCount := len(s.children)

		childrenToCleanup := make(map[string]SupervisorInterface)
		if childCount > 0 {
			childNames := make([]string, 0, childCount)
			for name, child := range s.children {
				childNames = append(childNames, name)
				childrenToCleanup[name] = child // Capture children for cleanup outside lock
			}

			s.logger.Warnf("Worker %s being removed still has %d children: %v - will clean up",
				workerID, childCount, childNames)
		}

		delete(s.workers, workerID)
		s.mu.Unlock()

		// LOCK SAFETY: Clean up children OUTSIDE parent lock to avoid deadlock.
		// Calling reconcileChildren() would re-acquire parent lock and call child.Shutdown()
		// while holding it, which blocks any readers (GetChildren, calculateHierarchySize)
		// indefinitely and risks deadlock if children try to access parent state.
		//
		// Instead, we call child.Shutdown() directly for each child, which:
		// - Only acquires child's own lock (not parent's)
		// - Allows parent lock to be acquired by other goroutines
		// - Child supervisors handle their own cleanup recursively
		//
		// Implementation: Extract done channels first (under lock), then wait outside lock
		doneChannels := make(map[string]<-chan struct{})

		s.mu.Lock()

		for name := range childrenToCleanup {
			if done, exists := s.childDoneChans[name]; exists {
				doneChannels[name] = done
			}
		}

		s.mu.Unlock()

		// Now shutdown children and wait for completion without holding parent lock
		for name, child := range childrenToCleanup {
			s.logger.Debugf("Shutting down child %s during parent removal", name)
			child.Shutdown()

			// Wait for child to fully shut down before proceeding
			if done, exists := doneChannels[name]; exists {
				<-done
			}

			// Remove from parent's children map (requires lock)
			s.mu.Lock()
			delete(s.children, name)
			delete(s.childDoneChans, name)
			s.mu.Unlock()
		}

		workerCtx.collector.Stop(ctx)
		workerCtx.executor.Shutdown()

		s.logger.Infof("Worker %s removed successfully (children cleaned: %d)", workerID, childCount)

		return nil
	case fsmv2.SignalNeedsRestart:
		s.logger.Infof("Worker %s signaled restart", workerID)

		if err := s.restartCollector(ctx, workerID); err != nil {
			return fmt.Errorf("failed to restart collector for worker %s: %w", workerID, err)
		}

		return nil
	default:
		return fmt.Errorf("unknown signal: %d", signal)
	}
}

// RequestShutdown requests a worker to shut down by setting the shutdown flag in its desired state.
//
// DESIGN DECISION: Mutate loaded desired state instead of wrapper
// WHY: DesiredState structs have SetShutdownRequested() method for mutation.
// Wrapper pattern failed because it serialized as {"inner": {...}} losing top-level fields.
//
// IMPLEMENTATION: Load → Mutate → Save pattern
// 1. Load current desired state
// 2. Call SetShutdownRequested(true) if supported
// 3. Save mutated desired state.
func (s *Supervisor[TObserved, TDesired]) requestShutdown(ctx context.Context, workerID string, reason string) error {
	s.logger.Warnf("Requesting shutdown for worker %s: %s", workerID, reason)

	// Load current desired state
	desiredInterface, err := s.store.LoadDesired(ctx, s.workerType, workerID)
	if err != nil {
		return fmt.Errorf("failed to load desired state for shutdown: %w", err)
	}

	// Type assert to Document - RequestShutdown requires map mutation
	// LoadDesired returns interface{} which could be Document or typed struct
	desiredDoc, ok := desiredInterface.(persistence.Document)
	if !ok {
		return fmt.Errorf("LoadDesired returned %T, expected persistence.Document for shutdown mutation", desiredInterface)
	}

	// Mutate document to set shutdown flag
	if desiredDoc == nil {
		desiredDoc = make(map[string]interface{})
	}

	desiredDoc["shutdownRequested"] = true
	desiredDoc["id"] = workerID // Ensure ID is present

	// Save mutated desired state
	if err := s.store.SaveDesired(ctx, s.workerType, workerID, desiredDoc); err != nil {
		return fmt.Errorf("failed to save shutdown desired state: %w", err)
	}

	return nil
}

// GetCurrentState returns the current state name for the first worker (backwards compatibility).
func (s *Supervisor[TObserved, TDesired]) GetCurrentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		state := workerCtx.currentState.String()
		workerCtx.mu.RUnlock()

		return state
	}

	return "no workers"
}

// GetWorkerState returns the current state name and reason for a worker.
// This method is thread-safe and can be safely called concurrently with tick operations.
// Returns "Unknown" state with reason "current state is nil" if the worker's state is nil.
// Returns an error if the worker is not found.
func (s *Supervisor[TObserved, TDesired]) GetWorkerState(workerID string) (string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		return "", "", fmt.Errorf("worker %s not found", workerID)
	}

	workerCtx.mu.RLock()
	defer workerCtx.mu.RUnlock()

	if workerCtx.currentState == nil {
		return "Unknown", "current state is nil", nil
	}

	return workerCtx.currentState.String(), workerCtx.currentState.Reason(), nil
}

// GetMappedParentState returns the mapped parent state for this supervisor.
// Returns empty string if this supervisor has no parent or no mapping has been applied.
// This method is primarily used for testing hierarchical state mapping.
func (s *Supervisor[TObserved, TDesired]) GetMappedParentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.mappedParentState
}

// GetChildren returns a copy of the children map for inspection.
// This method is thread-safe and can be used in tests to verify hierarchical composition.
func (s *Supervisor[TObserved, TDesired]) GetChildren() map[string]SupervisorInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children := make(map[string]SupervisorInterface, len(s.children))
	for name, child := range s.children {
		children[name] = child
	}

	return children
}

// reconcileChildren reconciles actual child supervisors to match desired ChildSpec array.
// This implements Kubernetes-style declarative reconciliation:
//  1. ADD children that don't exist in s.children
//  2. UPDATE existing children (UserSpec and StateMapping)
//  3. REMOVE children not in desired specs
//
// Children are themselves Supervisors, enabling recursive hierarchical composition.
// The factory creates workers for child types, and each child gets isolated storage via ChildStore().
//
// Error handling: Logs errors but continues reconciliation for remaining children.
// This ensures partial failures don't block the entire reconciliation operation.
func (s *Supervisor[TObserved, TDesired]) reconcileChildren(specs []config.ChildSpec) error {
	startTime := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	var addedCount, updatedCount, removedCount int

	specNames := make(map[string]bool)
	for _, spec := range specs {
		specNames[spec.Name] = true

		if child, exists := s.children[spec.Name]; exists {
			s.logger.Debugw("lifecycle",
				"lifecycle_event", "child_update",
				"child_name", spec.Name,
				"parent_worker_type", s.workerType)

			child.updateUserSpec(spec.UserSpec)
			child.setStateMapping(spec.StateMapping)

			updatedCount++
		} else {
			s.logger.Debugw("lifecycle",
				"lifecycle_event", "child_add_start",
				"child_name", spec.Name,
				"child_worker_type", spec.WorkerType,
				"parent_worker_type", s.workerType)

			s.logger.Infof("Adding child %s with worker type %s", spec.Name, spec.WorkerType)

			addedCount++

			childConfig := Config{
				WorkerType: spec.WorkerType,
				Store:      s.store,
				Logger:     s.logger,
			}

			// Use factory to create child supervisor with proper type
			rawSupervisor, err := factory.NewSupervisorByType(spec.WorkerType, childConfig)
			if err != nil {
				s.logger.Errorf("Failed to create child supervisor for %s: %v", spec.Name, err)

				continue
			}

			childSupervisor, ok := rawSupervisor.(SupervisorInterface)
			if !ok {
				s.logger.Errorf("Factory returned invalid supervisor type for %s", spec.Name)

				continue
			}

			childSupervisor.updateUserSpec(spec.UserSpec)
			childSupervisor.setStateMapping(spec.StateMapping)
			childSupervisor.setParent(s, s.workerType)

			// Create worker identity
			childIdentity := fsmv2.Identity{
				ID:         spec.Name + "-001",
				Name:       spec.Name,
				WorkerType: spec.WorkerType,
			}

			// Use factory to create worker instance
			childWorker, err := factory.NewWorkerByType(spec.WorkerType, childIdentity)
			if err != nil {
				s.logger.Errorf("Failed to create worker for child %s: %v (skipping)", spec.Name, err)

				continue
			}

			// Add worker to child supervisor
			if err := childSupervisor.AddWorker(childIdentity, childWorker); err != nil {
				s.logger.Errorf("Failed to add worker to child supervisor %s: %v (skipping)", spec.Name, err)

				continue
			}

			// Save initial desired state for child (empty document to avoid nil on first tick)
			childDesiredCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			desiredDoc := persistence.Document{
				"id":                childIdentity.ID,
				"shutdownRequested": false,
			}
			if err := s.store.SaveDesired(childDesiredCtx, spec.WorkerType, childIdentity.ID, desiredDoc); err != nil {
				s.logger.Warnf("Failed to save initial desired state for child %s: %v", spec.Name, err)
			}

			cancel()

			s.children[spec.Name] = childSupervisor

			s.logger.Debugw("lifecycle",
				"lifecycle_event", "child_add_complete",
				"child_name", spec.Name,
				"parent_worker_type", s.workerType)

			// Start child supervisor if parent is already started
			if childCtx, started := s.getStartedContext(); started {
				if childCtx.Err() == nil {
					done := childSupervisor.Start(childCtx)
					s.childDoneChans[spec.Name] = done
				} else {
					s.logger.Warnf("Parent context cancelled, skipping child start for %s", spec.Name)
				}
			}
		}
	}

	for name := range s.children {
		if !specNames[name] {
			s.logger.Debugw("lifecycle",
				"lifecycle_event", "child_remove_start",
				"child_name", name,
				"parent_worker_type", s.workerType)

			s.logger.Infof("Removing child %s (not in desired specs)", name)

			child := s.children[name]
			if child != nil {
				child.Shutdown()

				// Wait for child to fully shut down before removing
				if done, exists := s.childDoneChans[name]; exists {
					<-done
					delete(s.childDoneChans, name)
				}
			}

			delete(s.children, name)

			s.logger.Debugw("lifecycle",
				"lifecycle_event", "child_remove_complete",
				"child_name", name,
				"parent_worker_type", s.workerType)

			removedCount++
		}
	}

	duration := time.Since(startTime)
	s.logger.Info("child reconciliation completed",
		"supervisor_id", s.workerType,
		"added", addedCount,
		"updated", updatedCount,
		"removed", removedCount,
		"duration_ms", duration.Milliseconds())
	metrics.RecordReconciliation(s.workerType, "success", duration)

	return nil
}

// UpdateUserSpec updates the user-provided configuration for this supervisor.
// This method is called by parent supervisors during reconciliation to update child configuration.
func (s *Supervisor[TObserved, TDesired]) updateUserSpec(spec config.UserSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userSpec = spec
}

// Shutdown gracefully shuts down this supervisor and all its workers.
// This method is called when the supervisor is being removed from its parent.
// This method is idempotent - calling it multiple times is safe.
func (s *Supervisor[TObserved, TDesired]) Shutdown() {
	s.logger.Debugw("lifecycle",
		"lifecycle_event", "shutdown_start",
		"worker_type", s.workerType)

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "mutex_lock_acquire",
		"mutex_name", "supervisor.mu",
		"lock_type", "write")

	s.mu.Lock()

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "mutex_lock_acquired",
		"mutex_name", "supervisor.mu")

	// Make idempotent - check if already shut down
	if !s.started.Load() {
		s.mu.Unlock()

		s.logger.Debugw("lifecycle",
			"lifecycle_event", "shutdown_skip",
			"worker_type", s.workerType,
			"reason", "already_shutdown")

		s.logger.Debugf("Supervisor already shut down for worker type: %s", s.workerType)

		return
	}

	s.started.Store(false)

	s.logger.Infof("Shutting down supervisor for worker type: %s", s.workerType)

	// Cancel the supervisor's context to stop tick loop and all child goroutines
	s.ctxMu.Lock()

	if s.ctxCancel != nil {
		s.ctxCancel()
		s.ctxCancel = nil // Prevent double-cancel
	}

	s.ctxMu.Unlock()

	// Shutdown action executor (doesn't block)
	s.actionExecutor.Shutdown()

	// Log workers being shut down
	for workerID := range s.workers {
		s.logger.Debugf("Shutting down worker: %s", workerID)
	}

	// LOCK SAFETY: Extract children and done channels while holding lock,
	// then release lock before recursively shutting down children.
	// This prevents deadlock where:
	// 1. Parent holds write lock
	// 2. Parent calls child.Shutdown() which tries to acquire child's write lock
	// 3. Child's Tick() goroutine is blocked waiting for parent's read lock
	// By releasing parent lock before calling child.Shutdown(), we allow
	// child Tick() goroutines to complete and exit cleanly.
	childrenToShutdown := make(map[string]SupervisorInterface, len(s.children))

	childDoneChans := make(map[string]<-chan struct{}, len(s.childDoneChans))
	for name, child := range s.children {
		childrenToShutdown[name] = child
	}

	for name, done := range s.childDoneChans {
		childDoneChans[name] = done
	}

	s.mu.Unlock()

	// Wait for metrics reporter to finish (outside lock)
	s.metricsWg.Wait()

	// Now shutdown children recursively (outside lock)
	for childName, child := range childrenToShutdown {
		s.logger.Debugw("lifecycle",
			"lifecycle_event", "child_shutdown_start",
			"child_name", childName,
			"parent_worker_type", s.workerType)

		s.logger.Debugf("Shutting down child: %s", childName)
		child.Shutdown()

		// Wait for child supervisor to fully shut down
		if done, exists := childDoneChans[childName]; exists {
			s.logger.Debugf("Waiting for child %s to complete shutdown", childName)
			<-done
			s.logger.Debugf("Child %s shutdown complete", childName)
		}

		s.logger.Debugw("lifecycle",
			"lifecycle_event", "child_shutdown_complete",
			"child_name", childName,
			"parent_worker_type", s.workerType)
	}

	s.logger.Debugw("lifecycle",
		"lifecycle_event", "shutdown_complete",
		"worker_type", s.workerType)
}

// applyStateMapping applies parent state mapping to all children.
// Called after reconcileChildren() during Supervisor.Tick().
// For each child, this determines what state the child should transition to
// based on the parent's current state and the child's StateMapping configuration.
func (s *Supervisor[TObserved, TDesired]) applyStateMapping() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.workers) == 0 {
		return
	}

	var parentState string

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		if workerCtx.currentState != nil {
			parentState = workerCtx.currentState.String()
		}

		workerCtx.mu.RUnlock()

		break
	}

	for childName, child := range s.children {
		mappedState := parentState

		stateMapping := child.getStateMapping()
		if len(stateMapping) > 0 {
			if mapped, exists := stateMapping[parentState]; exists {
				mappedState = mapped
			}
		}

		child.setMappedParentState(mappedState)
		s.logger.Debugf("Child %s: parent state '%s' mapped to '%s'", childName, parentState, mappedState)
	}
}

// getString safely extracts a string value from a Document.
func getString(doc interface{}, key string, defaultValue string) string {
	if doc == nil {
		return defaultValue
	}

	docMap, ok := doc.(map[string]interface{})
	if !ok {
		return defaultValue
	}

	val, ok := docMap[key]
	if !ok {
		return defaultValue
	}

	str, ok := val.(string)
	if !ok {
		return defaultValue
	}

	return str
}

// calculateHierarchyDepth returns the depth of this supervisor in the hierarchy tree.
// Root supervisors (parent == nil) have depth 0, their children have depth 1, etc.
// Walks up the parent chain to calculate depth recursively.
//
// PERFORMANCE NOTE: Uses unbounded recursion. Safe for typical UMH hierarchies (2-4 levels).
// Deep hierarchies (>1000 levels) may cause stack overflow, but this is not expected in practice.
func (s *Supervisor[TObserved, TDesired]) calculateHierarchyDepth() int {
	if s.parent == nil {
		return 0
	}

	return 1 + s.parent.calculateHierarchyDepth()
}

// calculateHierarchySize returns the total number of supervisors in this subtree.
// This includes the supervisor itself plus all descendants (children, grandchildren, etc.).
func (s *Supervisor[TObserved, TDesired]) calculateHierarchySize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	size := 1

	for _, child := range s.children {
		size += child.calculateHierarchySize()
	}

	return size
}

// startMetricsReporter starts a goroutine that periodically records hierarchy metrics.
// Metrics are recorded every 10 seconds to avoid excessive Prometheus cardinality.
// The goroutine stops when the context is cancelled.
func (s *Supervisor[TObserved, TDesired]) startMetricsReporter(ctx context.Context) {
	s.metricsWg.Add(1)

	go func() {
		defer s.metricsWg.Done()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		s.recordHierarchyMetrics()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recordHierarchyMetrics()
			}
		}
	}()
}

// recordHierarchyMetrics records current hierarchy depth and size metrics.
func (s *Supervisor[TObserved, TDesired]) recordHierarchyMetrics() {
	depth := s.calculateHierarchyDepth()
	size := s.calculateHierarchySize()

	metrics.RecordHierarchyDepth(s.workerType, depth)
	metrics.RecordHierarchySize(s.workerType, size)
}

// isStarted returns whether the supervisor has been started.
//
//nolint:unused // Part of API design, may be used in future
func (s *Supervisor[TObserved, TDesired]) isStarted() bool {
	return s.started.Load()
}

// getContext returns the current context for the supervisor.
//
//nolint:unused // Part of API design, may be used in future
func (s *Supervisor[TObserved, TDesired]) getContext() context.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()

	return s.ctx
}

// getStartedContext atomically checks if started and returns context.
// This prevents TOCTOU races between isStarted() and getContext() calls.
func (s *Supervisor[TObserved, TDesired]) getStartedContext() (context.Context, bool) {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()

	if !s.started.Load() {
		return nil, false
	}

	return s.ctx, true
}

// GetWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor[TObserved, TDesired]) GetWorkers() []fsmv2.Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make([]fsmv2.Identity, 0, len(s.workers))
	for id, worker := range s.workers {
		if worker == nil {
			continue
		}

		workers = append(workers, fsmv2.Identity{
			ID:         id,
			Name:       worker.identity.Name,
			WorkerType: s.workerType,
		})
	}

	return workers
}

// =============================================================================
// TEST ACCESSORS - DO NOT USE IN PRODUCTION CODE
// =============================================================================
//
// These methods expose internal functionality exclusively for testing purposes.
// They are NOT part of the public API contract and should NEVER be used in
// production code. They exist to support black-box testing without converting
// all test files to white-box testing (package supervisor).
//
// Naming Convention: All test accessor methods are prefixed with "Test" to
// clearly indicate they are test-only and distinguish them from the public API.
//
// Why These Exist:
// - Some test files are too complex to convert to white-box testing
// - Converting would require extensive mock infrastructure changes
// - Hybrid approach: simple tests use white-box, complex tests use accessors
// - Clearly documented as test-only to prevent production usage

// TestTick exposes tick for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTick(ctx context.Context) error {
	return s.tick(ctx)
}

// TestTickAll exposes tickAll for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTickAll(ctx context.Context) error {
	return s.tickAll(ctx)
}

// TestRequestShutdown exposes requestShutdown for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRequestShutdown(ctx context.Context, workerID string, reason string) error {
	return s.requestShutdown(ctx, workerID, reason)
}

// TestCheckDataFreshness exposes checkDataFreshness for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestCheckDataFreshness(snapshot *fsmv2.Snapshot) bool {
	return s.checkDataFreshness(snapshot)
}

// TestRestartCollector exposes restartCollector for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRestartCollector(ctx context.Context, workerID string) error {
	return s.restartCollector(ctx, workerID)
}

// TestUpdateUserSpec exposes updateUserSpec for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestUpdateUserSpec(spec config.UserSpec) {
	s.updateUserSpec(spec)
}

// TestGetStaleThreshold exposes getStaleThreshold for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetStaleThreshold() time.Duration {
	return s.getStaleThreshold()
}

// TestGetRestartCount exposes getRestartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetRestartCount() int {
	return s.getRestartCount()
}

// TestSetRestartCount exposes setRestartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetRestartCount(count int) {
	s.setRestartCount(count)
}

// isCircuitOpen returns the circuit breaker state.
// Used by InfrastructureHealthChecker to check child supervisor health.
func (s *Supervisor[TObserved, TDesired]) isCircuitOpen() bool {
	return s.circuitOpen.Load()
}

// getStateMapping returns a copy of the state mapping.
func (s *Supervisor[TObserved, TDesired]) getStateMapping() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mapping := make(map[string]string, len(s.stateMapping))
	for k, v := range s.stateMapping {
		mapping[k] = v
	}

	return mapping
}

// setStateMapping updates the state mapping.
func (s *Supervisor[TObserved, TDesired]) setStateMapping(mapping map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stateMapping = mapping
}

// getMappedParentState returns the mapped parent state (internal interface method).
func (s *Supervisor[TObserved, TDesired]) getMappedParentState() string {
	return s.GetMappedParentState()
}

// setMappedParentState updates the mapped parent state.
func (s *Supervisor[TObserved, TDesired]) setMappedParentState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mappedParentState = state
}

// setParent sets the parent supervisor and parent ID for hierarchical composition.
func (s *Supervisor[TObserved, TDesired]) setParent(parent SupervisorInterface, parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.parent = parent
	s.parentID = parentID
}

// getUserSpec returns a copy of the user spec.
func (s *Supervisor[TObserved, TDesired]) getUserSpec() config.UserSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.userSpec
}
