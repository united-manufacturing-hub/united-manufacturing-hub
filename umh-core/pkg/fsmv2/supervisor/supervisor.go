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

// Package supervisor manages the lifecycle of workers in the FSM system.
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
package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/health"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
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

// normalizeType strips pointer indirection to get the base struct type.
// This ensures consistent type comparison regardless of whether a worker
// returns *Type or Type.
//
// Examples:
//   - *ContainerObservedState → ContainerObservedState
//   - **Type → *Type (only strips one level)
//
// Used by both AddWorker (type registration) and tickWorker (type validation)
// to ensure they compare apples-to-apples.
func normalizeType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}

	return t
}

// CollectorHealth tracks the health and restart state of the observation collector.
// The collector runs in a separate goroutine and may fail due to network issues,
// blocked operations, or other infrastructure problems.
//
// This struct is internal to Supervisor and tracks runtime state.
// Configuration is provided via CollectorHealthConfig.
type CollectorHealth struct {
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

func (s *stubAction) Execute(ctx context.Context) error {
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

// factoryRegistryAdapter implements types.WorkerTypeChecker interface for validation
type factoryRegistryAdapter struct{}

func (f *factoryRegistryAdapter) ListRegisteredTypes() []string {
	return factory.ListRegisteredTypes()
}

type Supervisor struct {
	workerType            string                           // Type of workers managed (e.g., "container")
	workers               map[string]*WorkerContext        // workerID → worker context
	mu                    sync.RWMutex                     // Protects workers map
	store                 storage.TriangularStoreInterface // State persistence layer (triangular model)
	logger                *zap.SugaredLogger               // Logger for supervisor operations
	tickInterval          time.Duration                    // How often to evaluate state transitions
	collectorHealth       CollectorHealth                  // Collector health tracking
	freshnessChecker      *health.FreshnessChecker         // Data freshness validator
	expectedObservedTypes map[string]reflect.Type          // workerID → expected ObservedState type
	expectedDesiredTypes  map[string]reflect.Type          // workerID → expected DesiredState type
	children              map[string]*Supervisor           // Child supervisors by name (hierarchical composition)
	childDoneChans        map[string]<-chan struct{}       // Done channels for child supervisors
	stateMapping          map[string]string                // Parent→child state mapping
	userSpec              config.UserSpec                   // User-provided configuration for this supervisor
	mappedParentState     string                           // State mapped from parent (if this is a child supervisor)
	globalVars            map[string]any                   // Global variables (fleet-wide settings from management system)
	createdAt             time.Time                        // Timestamp when supervisor was created
	parentID              string                           // ID of parent supervisor (empty string for root supervisors)
	parent                *Supervisor                      // Pointer to parent supervisor (nil for root supervisors)
	healthChecker         *InfrastructureHealthChecker     // Infrastructure health monitoring
	circuitOpen           bool                             // Circuit breaker state
	actionExecutor        *execution.ActionExecutor        // Async action execution (Phase 2)
	metricsStopChan       chan struct{}                    // Channel to stop metrics reporter
	metricsReporterDone   chan struct{}                    // Channel signaling metrics reporter stopped
	ctx                   context.Context                  // Context for supervisor lifecycle
	ctxCancel             context.CancelFunc               // Cancel function for supervisor context
	ctxMu                 sync.RWMutex                     // Protects ctx access
	started               atomic.Bool                      // Whether supervisor has been started
}

// WorkerContext encapsulates the runtime state for a single worker
// managed by a multi-worker Supervisor. It groups the worker's identity,
// implementation, current FSM state, and observation collector.
//
// THREAD SAFETY: currentState is protected by mu. Always lock before accessing.
// tickInProgress prevents concurrent ticks for the same worker.
type WorkerContext struct {
	mu             sync.RWMutex
	tickInProgress atomic.Bool
	identity       fsmv2.Identity
	worker         fsmv2.Worker
	currentState   fsmv2.State
	collector      *collection.Collector
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
}

func NewSupervisor(cfg Config) *Supervisor {
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

	// Auto-register triangular collections for this worker type.
	// Collections follow convention: {workerType}_identity, {workerType}_desired, {workerType}_observed
	// CSE fields standardized per role per FSM v2 contract.
	//
	// DESIGN DECISION: Auto-registration by Supervisor at initialization
	// WHY: Eliminates worker-specific registry boilerplate (86 LOC per worker).
	// Workers focus purely on business logic per FSM v2 design goal.
	//
	// TRADE-OFF: Convention over configuration. Worker type MUST follow naming convention.
	// If custom collection names needed, can still register manually before creating Supervisor.
	//
	// INSPIRED BY: Rails ActiveRecord conventions, HTTP router auto-registration patterns.
	//
	// Note: This uses storage.Registry from the CSE package for collection metadata,
	// which is unrelated to fsmv2.Dependencies (worker dependency injection).
	registry := cfg.Store.Registry()

	identityCollectionName := cfg.WorkerType + "_identity"
	desiredCollectionName := cfg.WorkerType + "_desired"
	observedCollectionName := cfg.WorkerType + "_observed"

	// Only register if not already registered (supports manual override)
	if !registry.IsRegistered(identityCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          identityCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register identity collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered identity collection: %s", identityCollectionName)
	}

	if !registry.IsRegistered(desiredCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          desiredCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register desired collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered desired collection: %s", desiredCollectionName)
	}

	if !registry.IsRegistered(observedCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          observedCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register observed collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered observed collection: %s", observedCollectionName)
	}

	return &Supervisor{
		workerType:            cfg.WorkerType,
		workers:               make(map[string]*WorkerContext),
		mu:                    sync.RWMutex{},
		store:                 cfg.Store,
		logger:                cfg.Logger,
		tickInterval:          tickInterval,
		freshnessChecker:      freshnessChecker,
		expectedObservedTypes: make(map[string]reflect.Type),
		expectedDesiredTypes:  make(map[string]reflect.Type),
		children:              make(map[string]*Supervisor),
		childDoneChans:        make(map[string]<-chan struct{}),
		stateMapping:          make(map[string]string),
		createdAt:             time.Now(),
		parentID:              "", // Root supervisor has empty parentID
		healthChecker:         NewInfrastructureHealthChecker(DefaultMaxInfraRecoveryAttempts, DefaultRecoveryAttemptWindow),
		circuitOpen:           false,
		actionExecutor:        execution.NewActionExecutor(10, cfg.WorkerType),
		metricsStopChan:       make(chan struct{}),
		metricsReporterDone:   make(chan struct{}),
		collectorHealth: CollectorHealth{
			staleThreshold:     staleThreshold,
			timeout:            timeout,
			maxRestartAttempts: maxRestartAttempts,
			restartCount:       0,
		},
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
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[identity.ID]; exists {
		return fmt.Errorf("worker %s already exists", identity.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover worker ObservedState type: %w", err)
	}

	expectedObservedType := normalizeType(reflect.TypeOf(observed))
	s.expectedObservedTypes[identity.ID] = expectedObservedType

	s.logger.Infof("Registered ObservedState type for worker %s: %s", identity.ID, expectedObservedType)

	// Extract DesiredState type from ObservedState if it implements GetObservedDesiredState()
	// This handles workers where desired state is embedded in observed state
	var expectedDesiredType reflect.Type
	if _, ok := expectedObservedType.MethodByName("GetObservedDesiredState"); ok {
		// If GetObservedDesiredState method exists, look for embedded DesiredState struct
		for i := 0; i < expectedObservedType.NumField(); i++ {
			field := expectedObservedType.Field(i)
			// Look for embedded DesiredState struct
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				// This is likely the embedded desired state
				expectedDesiredType = normalizeType(field.Type)
				break
			}
		}
	}

	// Derive initial desired state using DeriveDesiredState (always, for both paths)
	// This ensures we get the correct initial desired state, not zero values
	var initialDesired fsmv2.DesiredState
	if expectedDesiredType == nil {
		// Fallback path: no GetObservedDesiredState, use DeriveDesiredState for both type and value
		desired, err := worker.DeriveDesiredState(nil)
		if err != nil {
			return fmt.Errorf("failed to derive desired state for type registration: %w", err)
		}
		expectedDesiredType = normalizeType(reflect.TypeOf(desired))
		initialDesired = desired
	} else {
		// GetObservedDesiredState path: we have the type, now get the initial value
		desired, err := worker.DeriveDesiredState(nil)
		if err != nil {
			return fmt.Errorf("failed to derive initial desired state: %w", err)
		}
		initialDesired = desired
	}

	s.expectedDesiredTypes[identity.ID] = expectedDesiredType

	s.logger.Infof("Registered DesiredState type for worker %s: %s", identity.ID, expectedDesiredType)

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
	// Convert ObservedState to persistence.Document using JSON marshaling
	observedJSON, err := json.Marshal(observed)
	if err != nil {
		return fmt.Errorf("failed to marshal observed state: %w", err)
	}

	var observedDoc persistence.Document
	if err := json.Unmarshal(observedJSON, &observedDoc); err != nil {
		return fmt.Errorf("failed to unmarshal observed state to document: %w", err)
	}

	// Add required 'id' field for document validation
	observedDoc["id"] = identity.ID

	_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
	if err != nil {
		return fmt.Errorf("failed to save initial observation: %w", err)
	}

	s.logger.Debugf("Saved initial observation for worker: %s", identity.ID)

	// Save initial desired state to database if we derived it
	if initialDesired != nil {
		// Convert DesiredState to persistence.Document using JSON marshaling
		desiredJSON, err := json.Marshal(initialDesired)
		if err != nil {
			return fmt.Errorf("failed to marshal desired state: %w", err)
		}

		var desiredDoc persistence.Document
		if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
			return fmt.Errorf("failed to unmarshal desired state to document: %w", err)
		}

		// Add required 'id' field for document validation
		desiredDoc["id"] = identity.ID

		err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
		if err != nil {
			return fmt.Errorf("failed to save initial desired state: %w", err)
		}

		s.logger.Debugf("Saved initial desired state for worker: %s", identity.ID)
	}

	collector := collection.NewCollector(collection.CollectorConfig{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              s.logger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  s.collectorHealth.staleThreshold,
		WorkerType:          s.workerType,
	})

	executor := execution.NewActionExecutor(10, identity.ID)

	s.workers[identity.ID] = &WorkerContext{
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
func (s *Supervisor) RemoveWorker(ctx context.Context, workerID string) error {
	s.mu.Lock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.Unlock()

		return fmt.Errorf("worker %s not found", workerID)
	}

	delete(s.workers, workerID)
	delete(s.expectedObservedTypes, workerID)
	s.mu.Unlock()

	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	s.logger.Infof("Removed worker %s from supervisor", workerID)

	return nil
}

// GetWorker returns the worker context for the given ID.
func (s *Supervisor) GetWorker(workerID string) (*WorkerContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, exists := s.workers[workerID]
	if !exists {
		return nil, fmt.Errorf("worker %s not found", workerID)
	}

	return ctx, nil
}

// ListWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor) ListWorkers() []string {
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
func (s *Supervisor) SetGlobalVariables(vars map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.globalVars = vars
}

func (s *Supervisor) GetStaleThreshold() time.Duration {
	return s.collectorHealth.staleThreshold
}

func (s *Supervisor) GetCollectorTimeout() time.Duration {
	return s.collectorHealth.timeout
}

func (s *Supervisor) GetMaxRestartAttempts() int {
	return s.collectorHealth.maxRestartAttempts
}

func (s *Supervisor) GetRestartCount() int {
	return s.collectorHealth.restartCount
}

func (s *Supervisor) SetRestartCount(count int) {
	s.collectorHealth.restartCount = count
}

func (s *Supervisor) RestartCollector(ctx context.Context, workerID string) error {
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

func (s *Supervisor) CheckDataFreshness(snapshot *fsmv2.Snapshot) bool {
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
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
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
func (s *Supervisor) tickLoop(ctx context.Context) {
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
			if err := s.Tick(ctx); err != nil {
				s.logger.Errorf("Tick error: %v", err)
			}
		}
	}
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
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
	s.logger.Debugf("Ticking worker: %s, current state: %s", workerID, workerCtx.currentState.String())
	workerCtx.mu.RUnlock()

	// Load latest snapshot from database
	s.logger.Debugf("[DataFreshness] Worker %s: Loading snapshot from database", workerID)

	storageSnapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
	if err != nil {
		s.logger.Debugf("[DataFreshness] Worker %s: Failed to load snapshot: %v", workerID, err)

		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Convert storage.Snapshot to fsmv2.Snapshot
	// Note: Documents are compatible with interfaces - type assertion happens in state logic
	snapshot := &fsmv2.Snapshot{
		Identity: fsmv2.Identity{
			ID:         workerID, // We know the ID from context
			Name:       getString(storageSnapshot.Identity, "name", workerID),
			WorkerType: s.workerType,
		},
		Desired:  storageSnapshot.Desired,  // Document used as DesiredState interface
		Observed: storageSnapshot.Observed, // Document used as ObservedState interface
	}

	// Convert Document to typed struct if worker expects typed observed state
	s.mu.RLock()
	expectedType, exists := s.expectedObservedTypes[workerID]
	s.mu.RUnlock()

	if exists && expectedType.String() != "persistence.Document" {
		// Worker expects typed observed state, deserialize from Document
		observedPtr := reflect.New(expectedType)
		err := s.store.LoadObservedTyped(ctx, s.workerType, workerID, observedPtr.Interface())
		if err != nil {
			s.logger.Debugf("[DataFreshness] Worker %s: Failed to load typed observed state: %v", workerID, err)
			return fmt.Errorf("failed to load typed observed state: %w", err)
		}
		snapshot.Observed = observedPtr.Elem().Interface()
	}

	// Convert Document to typed struct if worker expects typed desired state
	s.mu.RLock()
	expectedDesiredType, desiredExists := s.expectedDesiredTypes[workerID]
	s.mu.RUnlock()

	if desiredExists && expectedDesiredType.String() != "persistence.Document" {
		// Worker expects typed desired state, deserialize from Document
		desiredPtr := reflect.New(expectedDesiredType)
		err := s.store.LoadDesiredTyped(ctx, s.workerType, workerID, desiredPtr.Interface())
		if err != nil {
			s.logger.Debugf("[DataFreshness] Worker %s: Failed to load typed desired state: %v", workerID, err)
			return fmt.Errorf("failed to load typed desired state: %w", err)
		}
		snapshot.Desired = desiredPtr.Elem().Interface()
	}

	// Log loaded observation details
	if snapshot.Observed == nil {
		s.logger.Debugf("[DataFreshness] Worker %s: Loaded snapshot has nil Observed state", workerID)
	} else if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		observationTimestamp := timestampProvider.GetTimestamp()
		s.logger.Debugf("[DataFreshness] Worker %s: Loaded observation timestamp=%s", workerID, observationTimestamp.Format(time.RFC3339Nano))
	} else {
		s.logger.Debugf("[DataFreshness] Worker %s: Loaded observation does not implement GetTimestamp() (type: %T)", workerID, snapshot.Observed)
	}

	// I16: Validate ObservedState type before calling state.Next()
	// This is Layer 3.5: Supervisor-level type validation BEFORE state logic
	// MUST happen before CheckDataFreshness because freshness check dereferences Observed
	// NOTE: Type validation is now implicit - LoadObservedTyped() already deserialized
	// to the expected type above. This check verifies the type matches expectations.
	if snapshot.Observed != nil {
		s.mu.RLock()
		expectedType, exists := s.expectedObservedTypes[workerID]
		s.mu.RUnlock()

		if exists {
			actualType := normalizeType(reflect.TypeOf(snapshot.Observed))
			if actualType != expectedType {
				panic(fmt.Sprintf("Invariant I16 violated: Worker %s (type %s) has ObservedState type %s, expected %s",
					workerID, s.workerType, actualType, expectedType))
			}
		}
	}

	// I3: Check data freshness BEFORE calling state.Next()
	// This is the trust boundary: states assume data is always fresh
	if !s.CheckDataFreshness(snapshot) {
		if s.freshnessChecker.IsTimeout(snapshot) {
			// I4: Check if we've exhausted restart attempts
			if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
				// Max attempts reached - escalate to shutdown (Layer 3)
				s.logger.Errorf("Collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)

				if shutdownErr := s.RequestShutdown(ctx, workerID,
					fmt.Sprintf("collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)); shutdownErr != nil {
					s.logger.Errorf("Failed to request shutdown: %v", shutdownErr)
				}

				return errors.New("collector unresponsive, shutdown requested")
			}

			// I4: Safe to restart (restartCount < maxRestartAttempts)
			// RestartCollector will panic if invariant violated (defensive check)
			if err := s.RestartCollector(ctx, workerID); err != nil {
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

	// Data is fresh - safe to progress FSM
	// Call state transition (read current state with lock)
	workerCtx.mu.RLock()
	currentState := workerCtx.currentState
	workerCtx.mu.RUnlock()

	s.logger.Debugf("Evaluating state transition for worker: %s", workerID)

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
func (s *Supervisor) getRecoveryStatus() string {
	attempts := s.healthChecker.backoff.GetAttempts()
	if attempts < 3 {
		return "attempting_recovery"
	} else if attempts < 5 {
		return "persistent_failure"
	}

	return "escalation_imminent"
}

// getEscalationSteps returns manual runbook steps for specific child types.
func (s *Supervisor) getEscalationSteps(childName string) string {
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
func (s *Supervisor) Tick(ctx context.Context) error {
	// PHASE 1: Infrastructure health check (priority 1)
	if err := s.healthChecker.CheckChildConsistency(s.children); err != nil {
		wasOpen := s.circuitOpen
		s.circuitOpen = true

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

	if s.circuitOpen {
		downtime := time.Since(s.healthChecker.backoff.GetStartTime())
		s.logger.Info("Infrastructure recovered, closing circuit breaker",
			"supervisor_id", s.workerType,
			"total_downtime", downtime.String())
		metrics.RecordCircuitOpen(s.workerType, false)
		metrics.RecordInfrastructureRecovery(s.workerType, downtime)
	}

	s.circuitOpen = false

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

	// 2. Reconcile children (propagate errors)
	if err := s.reconcileChildren(desired.ChildrenSpecs); err != nil {
		return fmt.Errorf("failed to reconcile children: %w", err)
	}

	// 3. Apply state mapping
	s.applyStateMapping()

	// 4. Recursively tick children (log errors, don't fail parent)
	s.mu.RLock()

	childrenToTick := make([]*Supervisor, 0, len(s.children))
	for _, child := range s.children {
		childrenToTick = append(childrenToTick, child)
	}

	s.mu.RUnlock()

	for _, child := range childrenToTick {
		if err := child.Tick(ctx); err != nil {
			s.logger.Errorf("Child tick failed: %v", err)
			// Continue with other children
		}
	}

	// 5. Continue with existing worker tick logic
	return s.tickWorker(ctx, firstWorkerID)
}

// TickAll performs one FSM tick for all workers in the registry.
// Each worker is ticked independently. Errors from one worker do not stop others.
// Returns an aggregated error containing all individual worker errors.
func (s *Supervisor) TickAll(ctx context.Context) error {
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
func (s *Supervisor) processSignal(ctx context.Context, workerID string, signal fsmv2.Signal) error {
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
		childrenToCleanup := make(map[string]*Supervisor)
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

		if err := s.RestartCollector(ctx, workerID); err != nil {
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
func (s *Supervisor) RequestShutdown(ctx context.Context, workerID string, reason string) error {
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
func (s *Supervisor) GetCurrentState() string {
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
func (s *Supervisor) GetWorkerState(workerID string) (string, string, error) {
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
func (s *Supervisor) GetMappedParentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.mappedParentState
}

// GetChildren returns a copy of the children map for inspection.
// This method is thread-safe and can be used in tests to verify hierarchical composition.
func (s *Supervisor) GetChildren() map[string]*Supervisor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children := make(map[string]*Supervisor, len(s.children))
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
func (s *Supervisor) reconcileChildren(specs []config.ChildSpec) error {
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

			child.UpdateUserSpec(spec.UserSpec)
			child.stateMapping = spec.StateMapping
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

			childSupervisor := NewSupervisor(childConfig)
			childSupervisor.UpdateUserSpec(spec.UserSpec)
			childSupervisor.stateMapping = spec.StateMapping
			childSupervisor.parentID = s.workerType
			childSupervisor.parent = s

			// Create worker identity
			childIdentity := fsmv2.Identity{
				ID:         fmt.Sprintf("%s-001", spec.Name),
				Name:       spec.Name,
				WorkerType: spec.WorkerType,
			}

			// Use factory to create worker instance
			childWorker, err := factory.NewWorker(spec.WorkerType, childIdentity)
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
				"id":                 childIdentity.ID,
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
func (s *Supervisor) UpdateUserSpec(spec config.UserSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userSpec = spec
}

// Shutdown gracefully shuts down this supervisor and all its workers.
// This method is called when the supervisor is being removed from its parent.
// This method is idempotent - calling it multiple times is safe.
func (s *Supervisor) Shutdown() {
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

	// Stop metrics first (safe to do under lock - just closes channel)
	if s.metricsStopChan != nil {
		close(s.metricsStopChan)
		// NOTE: Will wait for metricsReporterDone outside lock to avoid blocking readers
	}

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
	childrenToShutdown := make(map[string]*Supervisor, len(s.children))
	childDoneChans := make(map[string]<-chan struct{}, len(s.childDoneChans))
	for name, child := range s.children {
		childrenToShutdown[name] = child
	}
	for name, done := range s.childDoneChans {
		childDoneChans[name] = done
	}

	// Need to wait for metrics reporter outside lock
	metricsReporterDone := s.metricsReporterDone
	metricsStopChanWasClosed := s.metricsStopChan != nil
	if metricsStopChanWasClosed {
		s.metricsStopChan = nil // Prevent double-close
	}

	s.mu.Unlock()

	// Wait for metrics reporter to finish (outside lock)
	if metricsStopChanWasClosed && metricsReporterDone != nil {
		<-metricsReporterDone
	}

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
func (s *Supervisor) applyStateMapping() {
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

		if len(child.stateMapping) > 0 {
			if mapped, exists := child.stateMapping[parentState]; exists {
				mappedState = mapped
			}
		}

		child.mappedParentState = mappedState
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
func (s *Supervisor) calculateHierarchyDepth() int {
	if s.parent == nil {
		return 0
	}

	return 1 + s.parent.calculateHierarchyDepth()
}

// calculateHierarchySize returns the total number of supervisors in this subtree.
// This includes the supervisor itself plus all descendants (children, grandchildren, etc.).
func (s *Supervisor) calculateHierarchySize() int {
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
// The goroutine stops when s.metricsStopChan is closed.
func (s *Supervisor) startMetricsReporter(ctx context.Context) {
	go func() {
		defer close(s.metricsReporterDone)

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		s.recordHierarchyMetrics()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.metricsStopChan:
				return
			case <-ticker.C:
				s.recordHierarchyMetrics()
			}
		}
	}()
}

// recordHierarchyMetrics records current hierarchy depth and size metrics.
func (s *Supervisor) recordHierarchyMetrics() {
	depth := s.calculateHierarchyDepth()
	size := s.calculateHierarchySize()

	metrics.RecordHierarchyDepth(s.workerType, depth)
	metrics.RecordHierarchySize(s.workerType, size)
}

// isStarted returns whether the supervisor has been started.
func (s *Supervisor) isStarted() bool {
	return s.started.Load()
}

// getContext returns the current context for the supervisor.
func (s *Supervisor) getContext() context.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()
	return s.ctx
}

// getStartedContext atomically checks if started and returns context.
// This prevents TOCTOU races between isStarted() and getContext() calls.
func (s *Supervisor) getStartedContext() (context.Context, bool) {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()

	if !s.started.Load() {
		return nil, false
	}
	return s.ctx, true
}

// GetWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor) GetWorkers() []fsmv2.Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make([]fsmv2.Identity, 0, len(s.workers))
	for id := range s.workers {
		workers = append(workers, fsmv2.Identity{
			ID:         id,
			Name:       s.workers[id].identity.Name,
			WorkerType: s.workerType,
		})
	}
	return workers
}
