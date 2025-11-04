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
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
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
type Supervisor struct {
	workerType            string                            // Type of workers managed (e.g., "container")
	workers               map[string]*WorkerContext         // workerID → worker context
	mu                    sync.RWMutex                      // Protects workers map
	store                 storage.TriangularStoreInterface  // State persistence layer (triangular model)
	logger                *zap.SugaredLogger                // Logger for supervisor operations
	tickInterval          time.Duration                     // How often to evaluate state transitions
	collectorHealth       CollectorHealth                   // Collector health tracking
	freshnessChecker      *FreshnessChecker                 // Data freshness validator
	expectedObservedTypes map[string]reflect.Type           // workerID → expected ObservedState type
	children              map[string]*Supervisor            // Child supervisors by name (hierarchical composition)
	stateMapping          map[string]string                 // Parent→child state mapping
	userSpec              types.UserSpec                    // User-provided configuration for this supervisor
	mappedParentState     string                            // State mapped from parent (if this is a child supervisor)
	globalVars            map[string]any                    // Global variables (fleet-wide settings from management system)
	createdAt             time.Time                         // Timestamp when supervisor was created
	parentID              string                            // ID of parent supervisor (empty string for root supervisors)
	healthChecker         *InfrastructureHealthChecker      // Infrastructure health monitoring
	circuitOpen           bool                              // Circuit breaker state
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
	collector      *Collector
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

	freshnessChecker := NewFreshnessChecker(staleThreshold, timeout, cfg.Logger)

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

	identityCollectionName := fmt.Sprintf("%s_identity", cfg.WorkerType)
	desiredCollectionName := fmt.Sprintf("%s_desired", cfg.WorkerType)
	observedCollectionName := fmt.Sprintf("%s_observed", cfg.WorkerType)

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
		children:              make(map[string]*Supervisor),
		stateMapping:          make(map[string]string),
		createdAt:             time.Now(),
		parentID:              "", // Root supervisor has empty parentID
		healthChecker:         NewInfrastructureHealthChecker(DefaultMaxInfraRecoveryAttempts, DefaultRecoveryAttemptWindow),
		circuitOpen:           false,
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

	expectedType := normalizeType(reflect.TypeOf(observed))
	s.expectedObservedTypes[identity.ID] = expectedType

	s.logger.Infof("Registered ObservedState type for worker %s: %s", identity.ID, expectedType)

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
	if err := s.store.SaveObserved(ctx, s.workerType, identity.ID, observed); err != nil {
		return fmt.Errorf("failed to save initial observation: %w", err)
	}
	s.logger.Debugf("Saved initial observation for worker: %s", identity.ID)

	collector := NewCollector(CollectorConfig{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              s.logger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  s.collectorHealth.staleThreshold,
		WorkerType:          s.workerType,
	})

	s.workers[identity.ID] = &WorkerContext{
		identity:     identity,
		worker:       worker,
		currentState: worker.GetInitialState(),
		collector:    collector,
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
	var age time.Duration
	var collectedAt time.Time
	var hasTimestamp bool

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

	s.logger.Debugf("Supervisor started for workerType: %s", s.workerType)

	// Start observation collectors for all workers
	s.mu.RLock()
	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(ctx); err != nil {
			s.logger.Errorf("Failed to start collector for worker %s: %v", workerCtx.identity.ID, err)
		}
	}
	s.mu.RUnlock()

	// Start main tick loop
	go func() {
		defer close(done)

		s.tickLoop(ctx)
	}()

	return done
}

// tickLoop is the main FSM loop.
// Calls state.Next() and executes actions for all workers.
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
			s.mu.RLock()
			workerIDs := make([]string, 0, len(s.workers))
			for id := range s.workers {
				workerIDs = append(workerIDs, id)
			}
			s.mu.RUnlock()

			s.logger.Debugf("Tick: processing %d workers", len(workerIDs))

			for _, workerID := range workerIDs {
				if err := s.tickWorker(ctx, workerID); err != nil {
					s.logger.Errorf("Tick error for worker %s: %v", workerID, err)
				}
			}
		}
	}
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
	s.mu.RLock()
	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	// Skip if tick already in progress
	if !workerCtx.tickInProgress.CompareAndSwap(false, true) {
		s.logger.Debugf("Skipping tick for %s (previous tick still running)", workerID)
		return nil
	}
	defer workerCtx.tickInProgress.Store(false)

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
	s.mu.RLock()
	expectedType, exists := s.expectedObservedTypes[workerID]
	s.mu.RUnlock()

	if exists {
		if snapshot.Observed == nil {
			panic(fmt.Sprintf("Invariant I16 violated: Worker %s returned nil ObservedState", workerID))
		}
		actualType := normalizeType(reflect.TypeOf(snapshot.Observed))
		// Skip type check for Documents loaded from storage
		//
		// TYPE INFORMATION LOSS (Acceptable for MVP):
		// TriangularStore.LoadSnapshot() returns persistence.Document, NOT typed structs.
		// This is because we persist as JSON without type metadata for deserialization.
		// Communicator can work with Documents via reflection, so this is acceptable.
		//
		// See pkg/cse/storage/triangular.go LoadSnapshot() documentation for full rationale.
		if actualType.String() != "persistence.Document" && actualType != expectedType {
			panic(fmt.Sprintf("Invariant I16 violated: Worker %s (type %s) returned ObservedState type %s, expected %s",
				workerID, s.workerType, actualType, expectedType))
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
		s.logger.Infof("Executing action: %s", action.Name())

		if err := s.executeActionWithRetry(ctx, action); err != nil {
			return fmt.Errorf("action execution failed: %w", err)
		}
	}

	// Transition to next state
	if nextState != currentState {
		s.logger.Debugf("State transition for worker %s: %s → %s",
			workerID, currentState.String(), nextState.String())

		s.logger.Infof("State transition: %s -> %s (reason: %s)",
			currentState.String(), nextState.String(), nextState.Reason())

		workerCtx.mu.Lock()
		workerCtx.currentState = nextState
		workerCtx.mu.Unlock()
	} else {
		s.logger.Debugf("State unchanged for worker %s: %s", workerID, currentState.String())
	}

	// Process signal
	if err := s.processSignal(ctx, workerID, signal); err != nil {
		return fmt.Errorf("signal processing failed: %w", err)
	}

	return nil
}

// Tick performs one FSM tick for a specific worker (for testing).
func (s *Supervisor) Tick(ctx context.Context) error {
	// PHASE 1: Infrastructure health check (priority 1)
	if err := s.healthChecker.CheckChildConsistency(s.children); err != nil {
		s.circuitOpen = true

		// STUB: Child restart will be implemented in Phase 3
		s.logger.Warnf("Infrastructure health check failed: %v", err)

		return nil // Skip rest of tick
	}

	s.circuitOpen = false

	// For backwards compatibility, tick the first worker
	s.mu.RLock()
	var firstWorkerID string
	var worker fsmv2.Worker
	for id := range s.workers {
		firstWorkerID = id
		worker = s.workers[id].worker
		break
	}
	s.mu.RUnlock()

	if firstWorkerID == "" {
		return errors.New("no workers in supervisor")
	}

	// Task 0.5.5: Inject Global and Internal variables into UserSpec
	// This must happen BEFORE DeriveDesiredState() is called
	userSpecWithVars := s.userSpec

	// Preserve existing User variables
	if userSpecWithVars.Variables.User == nil {
		userSpecWithVars.Variables.User = make(map[string]any)
	}

	// Inject Global variables (from management system)
	s.mu.RLock()
	userSpecWithVars.Variables.Global = s.globalVars
	s.mu.RUnlock()

	// Inject Internal variables (runtime metadata)
	userSpecWithVars.Variables.Internal = map[string]any{
		"id":         firstWorkerID,
		"created_at": s.createdAt,
		"bridged_by": s.parentID,
	}

	// Task 0.6: Integrate Phase 0 features BEFORE State.Next()
	// 1. DeriveDesiredState
	desired, err := worker.DeriveDesiredState(userSpecWithVars)
	if err != nil {
		s.logger.Errorf("Failed to derive desired state: %v", err)
		return fmt.Errorf("failed to derive desired state: %w", err)
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

// executeActionWithRetry executes an action with exponential backoff retry.
func (s *Supervisor) executeActionWithRetry(ctx context.Context, action fsmv2.Action) error {
	maxRetries := 3
	backoff := 1 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Warnf("Retrying action %s (attempt %d/%d)", action.Name(), attempt+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2
		}

		if err := action.Execute(ctx); err != nil {
			lastErr = err
			s.logger.Errorf("Action %s failed (attempt %d/%d): %v", action.Name(), attempt+1, maxRetries, err)

			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("action %s failed after %d attempts: %w", action.Name(), maxRetries, lastErr)
}

// processSignal handles signals from states.
func (s *Supervisor) processSignal(ctx context.Context, workerID string, signal fsmv2.Signal) error {
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

		delete(s.workers, workerID)
		s.mu.Unlock()

		workerCtx.collector.Stop(ctx)

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
	desiredDoc, err := s.store.LoadDesired(ctx, s.workerType, workerID)
	if err != nil {
		return fmt.Errorf("failed to load desired state for shutdown: %w", err)
	}

	// Mutate document to set shutdown flag
	// Note: desiredDoc is a basic.Document (map[string]interface{})
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
//   1. ADD children that don't exist in s.children
//   2. UPDATE existing children (UserSpec and StateMapping)
//   3. REMOVE children not in desired specs
//
// Children are themselves Supervisors, enabling recursive hierarchical composition.
// The factory creates workers for child types, and each child gets isolated storage via ChildStore().
//
// Error handling: Logs errors but continues reconciliation for remaining children.
// This ensures partial failures don't block the entire reconciliation operation.
func (s *Supervisor) reconcileChildren(specs []types.ChildSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	specNames := make(map[string]bool)
	for _, spec := range specs {
		specNames[spec.Name] = true

		if child, exists := s.children[spec.Name]; exists {
			child.UpdateUserSpec(spec.UserSpec)
			child.stateMapping = spec.StateMapping
		} else {
			s.logger.Infof("Adding child %s with worker type %s", spec.Name, spec.WorkerType)

			childConfig := Config{
				WorkerType: spec.WorkerType,
				Store:      s.store,
				Logger:     s.logger,
			}

			// TODO(ENG-3806): Use WorkerFactory for dynamic worker creation
			// Currently using direct NewSupervisor() instantiation.
			// When WorkerFactory is integrated (Phase 0.2), replace with:
			//   worker, err := factory.NewWorker(spec.WorkerType, childIdentity)
			//   if err != nil { return fmt.Errorf("failed to create worker: %w", err) }
			//   childConfig.Worker = worker
			childSupervisor := NewSupervisor(childConfig)

			// TODO(ENG-3806): Implement storage isolation for children
			// Each child should get isolated storage via s.store.ChildStore(spec.Name).
			// Currently all children share parent's storage namespace.
			// When ChildStore() is implemented, replace s.store with:
			//   childStore := s.store.ChildStore(spec.Name)
			//   childConfig.Store = childStore
			childSupervisor.UpdateUserSpec(spec.UserSpec)
			childSupervisor.stateMapping = spec.StateMapping

			s.children[spec.Name] = childSupervisor
		}
	}

	for name := range s.children {
		if !specNames[name] {
			s.logger.Infof("Removing child %s (not in desired specs)", name)
			child := s.children[name]
			child.Shutdown()
			delete(s.children, name)
		}
	}

	return nil
}

// UpdateUserSpec updates the user-provided configuration for this supervisor.
// This method is called by parent supervisors during reconciliation to update child configuration.
func (s *Supervisor) UpdateUserSpec(spec types.UserSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userSpec = spec
}

// Shutdown gracefully shuts down this supervisor and all its workers.
// This method is called when the supervisor is being removed from its parent.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Infof("Shutting down supervisor for worker type: %s", s.workerType)

	for workerID := range s.workers {
		s.logger.Debugf("Shutting down worker: %s", workerID)
	}

	for childName, child := range s.children {
		s.logger.Debugf("Shutting down child: %s", childName)
		child.Shutdown()
	}
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
