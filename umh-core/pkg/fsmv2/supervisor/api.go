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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// AddWorker adds a new worker to the supervisor's registry.
// Returns error if worker with same ID already exists.
// Multi-layer validation strategy:
//
// FSMv2 validates data at multiple layers (not one centralized validator).
// This is intentional, not redundant:
//
// Layer 1: API entry (AddWorker) - Fast fail on obvious errors
// Layer 2: Reconciliation entry (reconcileChildren) - Catch runtime edge cases
// Layer 3: Factory (worker creation) - Validate WorkerType exists
// Layer 4: Worker constructor - Validate dependencies
//
// Why multiple layers:
//   - Security: Never trust data, even from internal callers
//   - Debuggability: Errors caught closest to source
//   - Reliability: One layer failing doesn't compromise system
//
// Each layer has different validation concerns:
//   - Layer 1: Public API validation (protect against bad calls)
//   - Layer 2: Runtime state validation (data evolved since layer 1)
//   - Layer 3: Registry validation (WorkerType registered?)
//   - Layer 4: Logical validation (dependencies compatible?)
func (s *Supervisor[TObserved, TDesired]) AddWorker(identity deps.Identity, worker fsmv2.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[identity.ID]; exists {
		s.logger.Warnw("worker_add_rejected",
			"hierarchy_path", identity.HierarchyPath,
			"reason", "already_exists")

		return errors.New("worker already exists")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		s.logger.Errorw("worker_add_collect_observed_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to collect initial observed state: %w", err)
	}

	initialDesired, err := worker.DeriveDesiredState(nil)
	if err != nil {
		s.logger.Errorw("worker_add_derive_desired_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to derive initial desired state: %w", err)
	}

	if valErr := config.ValidateDesiredState(initialDesired.GetState()); valErr != nil {
		s.logger.Errorw("worker_add_validate_desired_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", valErr)

		return fmt.Errorf("failed to derive initial desired state: %w", valErr)
	}

	identityDoc := persistence.Document{
		"id":             identity.ID,
		"name":           identity.Name,
		"worker_type":    identity.WorkerType,
		"hierarchy_path": identity.HierarchyPath,
	}
	if err := s.store.SaveIdentity(ctx, s.workerType, identity.ID, identityDoc); err != nil {
		s.logger.Errorw("worker_add_save_identity_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to save identity: %w", err)
	}

	s.logger.Debugw("identity_saved")

	observedJSON, err := json.Marshal(observed)
	if err != nil {
		s.logger.Errorw("worker_add_marshal_observed_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to marshal observed state: %w", err)
	}

	observedDoc := make(persistence.Document)
	if err := json.Unmarshal(observedJSON, &observedDoc); err != nil {
		s.logger.Errorw("worker_add_unmarshal_observed_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to unmarshal observed state to document: %w", err)
	}

	observedDoc["id"] = identity.ID

	_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
	if err != nil {
		s.logger.Errorw("worker_add_save_observed_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to save initial observation: %w", err)
	}

	s.logger.Debugw("initial_observation_saved")

	desiredJSON, err := json.Marshal(initialDesired)
	if err != nil {
		s.logger.Errorw("worker_add_marshal_desired_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		s.logger.Errorw("worker_add_unmarshal_desired_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to unmarshal desired state to document: %w", err)
	}

	desiredDoc["id"] = identity.ID

	_, err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
	if err != nil {
		s.logger.Errorw("worker_add_save_desired_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)

		return fmt.Errorf("failed to save initial desired state: %w", err)
	}

	s.logger.Debugw("initial_desired_state_saved")

	// Use baseLogger (un-enriched) to prevent duplicate "worker" fields.
	workerLogger := s.baseLogger.With("worker", identity.String())

	// Declared early so closures can capture it by reference.
	var workerCtx *WorkerContext[TObserved, TDesired]

	collector := collection.NewCollector[TObserved](collection.CollectorConfig[TObserved]{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              workerLogger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  s.collectorHealth.observationTimeout,
		StateProvider: func() string {
			if workerCtx == nil {
				return "unknown"
			}
			workerCtx.mu.RLock()
			defer workerCtx.mu.RUnlock()
			if workerCtx.currentState == nil {
				return "unknown"
			}

			return workerCtx.currentState.String()
		},
		ShutdownRequestedProvider: func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var desired TDesired
			if err := s.store.LoadDesiredTyped(ctx, s.workerType, identity.ID, &desired); err != nil {
				return false
			}

			return desired.IsShutdownRequested()
		},
		// A child is healthy ONLY if in PhaseRunningHealthy (fully stable).
		// PhaseRunningDegraded is operational but NOT healthy.
		// Uses lifecycle phase enum for type-safe health checks.
		ChildrenCountsProvider: func() (healthy int, unhealthy int) {
			s.mu.RLock()
			defer s.mu.RUnlock()

			for _, child := range s.children {
				phase := child.GetLifecyclePhase()
				if phase.IsHealthy() {
					healthy++
				} else if !phase.IsStopped() {
					// Everything except healthy and stopped is unhealthy
					// This includes: PhaseUnknown, PhaseStarting, PhaseRunningDegraded, PhaseStopping
					unhealthy++
				}
				// Stopped states are neither healthy nor unhealthy
			}

			return healthy, unhealthy
		},
		MappedParentStateProvider: func() string {
			return s.getMappedParentState()
		},
		ChildrenViewProvider: func() any {
			s.mu.RLock()
			defer s.mu.RUnlock()

			childrenCopy := make(map[string]SupervisorInterface, len(s.children))
			for name, child := range s.children {
				childrenCopy[name] = child
			}

			return NewChildrenManager(childrenCopy)
		},
		// Called BEFORE CollectObservedState to compute metrics.
		FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
			if workerCtx == nil {
				return nil
			}
			workerCtx.mu.RLock()
			defer workerCtx.mu.RUnlock()

			// Copy stateTransitions to avoid race condition with reconciliation goroutine.
			// Without this copy, the map reference escapes the lock and can be read
			// while reconciliation writes to it.
			transitionsCopy := make(map[string]int64, len(workerCtx.stateTransitions))
			for state, count := range workerCtx.stateTransitions {
				transitionsCopy[state] = count
			}

			// Convert stateDurations (map[string]time.Duration) to milliseconds
			cumulativeTimeMs := make(map[string]int64, len(workerCtx.stateDurations))
			for state, duration := range workerCtx.stateDurations {
				cumulativeTimeMs[state] = duration.Milliseconds()
			}

			return &deps.FrameworkMetrics{
				TimeInCurrentStateMs:    time.Since(workerCtx.stateEnteredAt).Milliseconds(),
				StateEnteredAtUnix:      workerCtx.stateEnteredAt.Unix(),
				StateTransitionsTotal:   workerCtx.totalTransitions,
				TransitionsByState:      transitionsCopy,
				CumulativeTimeByStateMs: cumulativeTimeMs,
				CollectorRestarts:       workerCtx.collectorRestarts,
				StartupCount:            workerCtx.startupCount,
				StateReason:             workerCtx.currentStateReason,
			}
		},
		FrameworkMetricsSetter: func(fm *deps.FrameworkMetrics) {
			// Must use GetDependenciesAny() (returns any), not GetDependencies() (returns D).
			type depsGetter interface {
				GetDependenciesAny() any
			}
			if dg, ok := worker.(depsGetter); ok {
				workerDeps := dg.GetDependenciesAny()
				if setter, ok := workerDeps.(interface{ SetFrameworkState(*deps.FrameworkMetrics) }); ok {
					setter.SetFrameworkState(fm)
				}
			}
		},
		// Called BEFORE CollectObservedState to drain action history buffer.
		ActionHistoryProvider: func() []deps.ActionResult {
			if workerCtx == nil || workerCtx.actionHistory == nil {
				return nil
			}

			return workerCtx.actionHistory.Drain()
		},
		ActionHistorySetter: func(history []deps.ActionResult) {
			type depsGetter interface {
				GetDependenciesAny() any
			}
			if dg, ok := worker.(depsGetter); ok {
				workerDeps := dg.GetDependenciesAny()
				if setter, ok := workerDeps.(interface{ SetActionHistory([]deps.ActionResult) }); ok {
					setter.SetActionHistory(history)
				}
			}
		},
	})

	executor := execution.NewActionExecutor(10, s.workerType, identity, workerLogger)

	actionHistoryBuffer := deps.NewInMemoryActionHistoryRecorder()

	executor.SetOnActionComplete(func(result deps.ActionResult) {
		actionHistoryBuffer.Record(result)

		// Trigger immediate observation after action completes.
		// This eliminates the delay between action and FSM progression.
		if workerCtx != nil && workerCtx.collector != nil && workerCtx.collector.IsRunning() {
			workerCtx.collector.TriggerNow()
		}
	})

	// Survives restarts, incremented on each AddWorker().
	var startupCount int64 = 1

	loadCtx, loadCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loadCancel()

	var prevObserved TObserved

	loadErr := s.store.LoadObservedTyped(loadCtx, s.workerType, identity.ID, &prevObserved)
	if loadErr == nil {
		if holder, ok := any(prevObserved).(deps.MetricsHolder); ok {
			fm := holder.GetFrameworkMetrics()
			if fm.StartupCount > 0 {
				startupCount = fm.StartupCount + 1
			}
		}
	}

	initialState := worker.GetInitialState()

	workerCtx = &WorkerContext[TObserved, TDesired]{
		mu:                 s.lockManager.NewLock(lockNameWorkerContextMu, lockLevelWorkerContextMu),
		identity:           identity,
		worker:             worker,
		currentState:       initialState,
		currentStateReason: "initial",
		collector:          collector,
		executor:           executor,
		actionHistory:      actionHistoryBuffer,
		stateEnteredAt:     time.Now(),
		stateTransitions:   make(map[string]int64),
		stateDurations:     make(map[string]time.Duration),
		totalTransitions:   0,
		collectorRestarts:  0,
		startupCount:       startupCount,
	}
	s.workers[identity.ID] = workerCtx

	// Cache the first worker ID for lock-free access in GetHierarchyPathUnlocked()
	if s.cachedFirstWorkerID.Load() == nil {
		s.cachedFirstWorkerID.Store(identity.ID)
	}

	if len(s.workers) == 1 && identity.HierarchyPath != "" {
		s.logger = workerLogger
	}

	s.logger.Infow("worker_added")

	return nil
}

// RemoveWorker removes a worker from the registry.
func (s *Supervisor[TObserved, TDesired]) RemoveWorker(ctx context.Context, workerID string) error {
	s.mu.Lock()

	// Cache hierarchy path while holding the lock to avoid data race
	hierarchyPath := s.GetHierarchyPathUnlocked()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.Unlock()

		s.logger.Warnw("worker_remove_not_found",
			"hierarchy_path", hierarchyPath,
			"target_worker_id", workerID)

		return errors.New("worker not found")
	}

	delete(s.workers, workerID)
	s.mu.Unlock()

	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	workerCtx.mu.RLock()

	if workerCtx.currentState != nil {
		metrics.CleanupStateDuration(hierarchyPath, workerCtx.currentState.String())
	}

	workerCtx.mu.RUnlock()

	s.logger.Infow("worker_removed")

	return nil
}

// GetWorker returns the worker context for the given ID.
func (s *Supervisor[TObserved, TDesired]) GetWorker(workerID string) (*WorkerContext[TObserved, TDesired], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, exists := s.workers[workerID]
	if !exists {
		return nil, errors.New("worker not found")
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

// GetWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor[TObserved, TDesired]) GetWorkers() []deps.Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make([]deps.Identity, 0, len(s.workers))
	for id, worker := range s.workers {
		if worker == nil {
			continue
		}

		workers = append(workers, deps.Identity{
			ID:            id,
			Name:          worker.identity.Name,
			WorkerType:    s.workerType,
			HierarchyPath: worker.identity.HierarchyPath,
		})
	}

	return workers
}

// GetCurrentState returns the current state of the first worker.
// Deprecated: Use GetCurrentStateName() for interface compatibility, or
// GetWorkerState(workerID) for full state information including reason.
// This method will be removed in a future version.
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
		return "", "", errors.New("worker not found")
	}

	workerCtx.mu.RLock()
	defer workerCtx.mu.RUnlock()

	if workerCtx.currentState == nil {
		return "Unknown", "current state is nil", nil
	}

	return workerCtx.currentState.String(), workerCtx.currentStateReason, nil
}

// GetMappedParentState returns the mapped parent state for this supervisor.
// Returns empty string if this supervisor has no parent or no mapping has been applied.
// This method is primarily used for testing hierarchical state mapping.
func (s *Supervisor[TObserved, TDesired]) GetMappedParentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.mappedParentState
}

// getMappedParentState implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) getMappedParentState() string {
	return s.GetMappedParentState()
}

// isCircuitOpen returns true if the circuit breaker is open for this supervisor.
// Used by InfrastructureHealthChecker.CheckChildConsistency() to detect unhealthy children.
func (s *Supervisor[TObserved, TDesired]) isCircuitOpen() bool {
	return s.circuitOpen.Load()
}

// IsCircuitOpen implements SupervisorInterface.
// Returns true if the circuit breaker is open (infrastructure failure detected).
// Used by ChildInfo to report infrastructure status to parents.
func (s *Supervisor[TObserved, TDesired]) IsCircuitOpen() bool {
	return s.circuitOpen.Load()
}

// IsObservationStale implements SupervisorInterface.
// Returns true if the last observation is older than the stale threshold.
// Used by ChildInfo to report infrastructure status to parents.
func (s *Supervisor[TObserved, TDesired]) IsObservationStale() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		if workerCtx.collector == nil {
			return true
		}

		workerCtx.mu.RLock()
		lastCollectedAt := workerCtx.lastObservationCollectedAt
		workerCtx.mu.RUnlock()

		if lastCollectedAt.IsZero() {
			return true // Never collected = stale
		}

		age := time.Since(lastCollectedAt)

		return age > s.collectorHealth.staleThreshold
	}

	return true
}

// setMappedParentState implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) setMappedParentState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mappedParentState = state
}

// getChildStartStates implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) getChildStartStates() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.childStartStates == nil {
		return nil
	}

	states := make([]string, len(s.childStartStates))
	copy(states, s.childStartStates)

	return states
}

// setChildStartStates implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) setChildStartStates(states []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.childStartStates = states
}

// updateUserSpec implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) updateUserSpec(spec config.UserSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userSpec = spec
}

// getUserSpec implements SupervisorInterface.
// Returns a deep copy to prevent callers from modifying internal state.
func (s *Supervisor[TObserved, TDesired]) getUserSpec() config.UserSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.userSpec.Clone()
}

// setParent implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) setParent(parent SupervisorInterface, parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.parent = parent
	s.parentID = parentID
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

// GetHierarchyPath returns the full hierarchy path from root to this supervisor.
// Format: "workerID(workerType)/childID(childType)/..."
// Example: "scenario123(application)/parent-123(parent)/child001(child)"
// Returns "unknown(workerType)" if no workers are registered yet.
func (s *Supervisor[TObserved, TDesired]) GetHierarchyPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.GetHierarchyPathUnlocked()
}

// GetHierarchyPathUnlocked computes the hierarchy path without acquiring the lock.
// This function is safe to call without holding the lock because it uses a cached
// worker ID (set atomically in AddWorker) instead of iterating over the workers map.
func (s *Supervisor[TObserved, TDesired]) GetHierarchyPathUnlocked() string {
	// Use cached first worker ID for lock-free access
	workerID := "unknown"
	if cached := s.cachedFirstWorkerID.Load(); cached != nil {
		workerID = cached.(string)
	}

	segment := fmt.Sprintf("%s(%s)", workerID, s.workerType)

	if s.parent == nil {
		return segment
	}

	parentPath := s.parent.GetHierarchyPathUnlocked()

	return parentPath + "/" + segment
}

// GetCurrentStateName returns the current FSM state name for this supervisor's worker.
// Returns "unknown" if no worker or state is set.
// Used by parent supervisors to track children's health status.
func (s *Supervisor[TObserved, TDesired]) GetCurrentStateName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		stateName := "unknown"
		if workerCtx.currentState != nil {
			stateName = workerCtx.currentState.String()
		}

		workerCtx.mu.RUnlock()

		return stateName
	}

	return "unknown"
}

// GetCurrentStateNameAndReason returns the current FSM state name and reason.
// Returns ("unknown", "") if no worker or state is set.
// Used by ChildInfo to populate StateReason field for parent workers.
func (s *Supervisor[TObserved, TDesired]) GetCurrentStateNameAndReason() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		stateName := "unknown"
		stateReason := ""

		if workerCtx.currentState != nil {
			stateName = workerCtx.currentState.String()
			stateReason = workerCtx.currentStateReason
		}

		workerCtx.mu.RUnlock()

		return stateName, stateReason
	}

	return "unknown", ""
}

// GetObservedStateName returns the observed state's State field (e.g., "running_healthy_connected").
// This exposes the lifecycle prefix for health checks using config.IsOperational().
// Returns "unknown" if no observation has been collected yet.
// Used by parent supervisors to determine child health based on lifecycle phase.
func (s *Supervisor[TObserved, TDesired]) GetObservedStateName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		stateName := workerCtx.lastObservedStateName
		workerCtx.mu.RUnlock()

		if stateName == "" {
			return "unknown"
		}

		return stateName
	}

	return "unknown"
}

// GetLifecyclePhase returns the lifecycle phase of the current state.
// Used by parent supervisors to classify child health via phase.IsHealthy().
// Returns PhaseUnknown if no state is set or no workers are registered.
func (s *Supervisor[TObserved, TDesired]) GetLifecyclePhase() config.LifecyclePhase {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		phase := workerCtx.lastLifecyclePhase
		workerCtx.mu.RUnlock()

		return phase
	}

	return config.PhaseUnknown
}

// GetWorkerType returns the type of workers this supervisor manages.
// For example: "examplechild", "exampleparent", "application".
func (s *Supervisor[TObserved, TDesired]) GetWorkerType() string {
	return s.workerType
}

// WorkerDebugInfo contains debug information for a single worker.
type WorkerDebugInfo struct {
	StateEnteredAt      time.Time `json:"state_entered_at"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	WorkerType          string    `json:"worker_type"`
	HierarchyPath       string    `json:"hierarchy_path"`
	State               string    `json:"state"`
	StateReason         string    `json:"state_reason"`
	TimeInCurrentStateS float64   `json:"time_in_current_state_s"`
	TotalTransitions    int64     `json:"total_transitions"`
	CollectorRestarts   int64     `json:"collector_restarts"`
	StartupCount        int64     `json:"startup_count"`
	ActionPending       bool      `json:"action_pending"`
}

// SupervisorDebugInfo contains debug information for a supervisor and its hierarchy.
type SupervisorDebugInfo struct {
	Children            map[string]SupervisorDebugInfo `json:"children,omitempty"`
	WorkerType          string                         `json:"worker_type"`
	HierarchyPath       string                         `json:"hierarchy_path"`
	MappedParentState   string                         `json:"mapped_parent_state,omitempty"`
	Workers             []WorkerDebugInfo              `json:"workers"`
	CollectedAtUnixNano int64                          `json:"collected_at_unix_nano"`
	CircuitOpen         bool                           `json:"circuit_open"`
}

// GetDebugInfo returns introspection data for debugging and monitoring.
// This method is thread-safe and provides a snapshot of the supervisor state.
// The returned data is suitable for JSON serialization and /debug/fsmv2 endpoint.
// Returns interface{} to satisfy metrics.FSMv2DebugProvider interface.
func (s *Supervisor[TObserved, TDesired]) GetDebugInfo() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := SupervisorDebugInfo{
		WorkerType:          s.workerType,
		HierarchyPath:       s.GetHierarchyPathUnlocked(),
		CircuitOpen:         s.circuitOpen.Load(),
		MappedParentState:   s.mappedParentState,
		CollectedAtUnixNano: time.Now().UnixNano(),
		Workers:             make([]WorkerDebugInfo, 0, len(s.workers)),
	}

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		workerInfo := WorkerDebugInfo{
			ID:                  workerCtx.identity.ID,
			Name:                workerCtx.identity.Name,
			WorkerType:          workerCtx.identity.WorkerType,
			HierarchyPath:       workerCtx.identity.HierarchyPath,
			State:               "unknown",
			StateReason:         workerCtx.currentStateReason,
			StateEnteredAt:      workerCtx.stateEnteredAt,
			TimeInCurrentStateS: time.Since(workerCtx.stateEnteredAt).Seconds(),
			TotalTransitions:    workerCtx.totalTransitions,
			CollectorRestarts:   workerCtx.collectorRestarts,
			StartupCount:        workerCtx.startupCount,
			ActionPending:       workerCtx.actionPending,
		}

		if workerCtx.currentState != nil {
			workerInfo.State = workerCtx.currentState.String()
		}

		workerCtx.mu.RUnlock()

		info.Workers = append(info.Workers, workerInfo)
	}

	// Recursively collect child debug info
	if len(s.children) > 0 {
		info.Children = make(map[string]SupervisorDebugInfo, len(s.children))

		for name, child := range s.children {
			// Use the interface method which returns interface{}, then type assert
			if debuggable, ok := child.(interface{ GetDebugInfo() interface{} }); ok {
				if childInfo, ok := debuggable.GetDebugInfo().(SupervisorDebugInfo); ok {
					info.Children[name] = childInfo
				}
			}
		}
	}

	return info
}
