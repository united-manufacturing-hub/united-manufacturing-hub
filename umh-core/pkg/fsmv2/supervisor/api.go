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
	"strings"
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
			"worker_id", identity.ID,
			"worker_name", identity.Name,
			"reason", "already_exists")

		return errors.New("worker already exists")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect initial observed state: %w", err)
	}

	initialDesired, err := worker.DeriveDesiredState(nil)
	if err != nil {
		return fmt.Errorf("failed to derive initial desired state: %w", err)
	}

	if valErr := config.ValidateDesiredState(initialDesired.GetState()); valErr != nil {
		return fmt.Errorf("failed to derive initial desired state: %w", valErr)
	}

	identityDoc := persistence.Document{
		"id":             identity.ID,
		"name":           identity.Name,
		"worker_type":    identity.WorkerType,
		"hierarchy_path": identity.HierarchyPath,
	}
	if err := s.store.SaveIdentity(ctx, s.workerType, identity.ID, identityDoc); err != nil {
		return fmt.Errorf("failed to save identity: %w", err)
	}

	s.logger.Debugw("identity_saved")

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

	s.logger.Debugw("initial_observation_saved")

	desiredJSON, err := json.Marshal(initialDesired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal desired state to document: %w", err)
	}

	desiredDoc["id"] = identity.ID

	_, err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
	if err != nil {
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
		// A child is healthy if its state contains "Running" or "Connected".
		ChildrenCountsProvider: func() (healthy int, unhealthy int) {
			s.mu.RLock()
			defer s.mu.RUnlock()

			for _, child := range s.children {
				stateName := child.GetCurrentStateName()
				if strings.Contains(stateName, "Running") || strings.Contains(stateName, "Connected") {
					healthy++
				} else if stateName != "" && stateName != "unknown" &&
					!strings.Contains(stateName, "Stopped") {
					// TryingToStop counts as unhealthy so parent waits for full stop
					unhealthy++
				}
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

			// Convert stateDurations (map[string]time.Duration) to milliseconds
			cumulativeTimeMs := make(map[string]int64, len(workerCtx.stateDurations))
			for state, duration := range workerCtx.stateDurations {
				cumulativeTimeMs[state] = duration.Milliseconds()
			}

			return &deps.FrameworkMetrics{
				TimeInCurrentStateMs:    time.Since(workerCtx.stateEnteredAt).Milliseconds(),
				StateEnteredAtUnix:      workerCtx.stateEnteredAt.Unix(),
				StateTransitionsTotal:   workerCtx.totalTransitions,
				TransitionsByState:      workerCtx.stateTransitions,
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

	workerCtx = &WorkerContext[TObserved, TDesired]{
		mu:                s.lockManager.NewLock(lockNameWorkerContextMu, lockLevelWorkerContextMu),
		identity:          identity,
		worker:            worker,
		currentState:      worker.GetInitialState(),
		collector:         collector,
		executor:          executor,
		actionHistory:     actionHistoryBuffer,
		stateEnteredAt:    time.Now(),
		stateTransitions:  make(map[string]int64),
		stateDurations:    make(map[string]time.Duration),
		totalTransitions:  0,
		collectorRestarts: 0,
		startupCount:      startupCount,
	}
	s.workers[identity.ID] = workerCtx

	if len(s.workers) == 1 && identity.HierarchyPath != "" {
		s.logger = workerLogger
	}

	s.logger.Infow("worker_added")

	return nil
}

// RemoveWorker removes a worker from the registry.
func (s *Supervisor[TObserved, TDesired]) RemoveWorker(ctx context.Context, workerID string) error {
	s.mu.Lock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.Unlock()

		s.logger.Warnw("worker_remove_not_found",
			"worker_id", workerID)

		return errors.New("worker not found")
	}

	delete(s.workers, workerID)
	s.mu.Unlock()

	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	workerCtx.mu.RLock()

	if workerCtx.currentState != nil {
		metrics.CleanupStateDuration(s.workerType, workerID, workerCtx.currentState.String())
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
		stateEnteredAt := workerCtx.stateEnteredAt
		workerCtx.mu.RUnlock()

		if stateEnteredAt.IsZero() {
			return true
		}

		age := time.Since(stateEnteredAt)

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
// Use this from within locked contexts to avoid deadlock.
// Caller must hold s.mu.RLock() or s.mu.Lock().
func (s *Supervisor[TObserved, TDesired]) GetHierarchyPathUnlocked() string {
	var workerID string
	for id := range s.workers {
		workerID = id

		break
	}

	if workerID == "" {
		workerID = "unknown"
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

		defer workerCtx.mu.RUnlock()

		if workerCtx.currentState != nil {
			return workerCtx.currentState.String()
		}

		return "unknown"
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
		defer workerCtx.mu.RUnlock()

		if workerCtx.currentState != nil {
			return workerCtx.currentState.String(), workerCtx.currentStateReason
		}

		return "unknown", ""
	}

	return "unknown", ""
}

// GetWorkerType returns the type of workers this supervisor manages.
// For example: "examplechild", "exampleparent", "application".
func (s *Supervisor[TObserved, TDesired]) GetWorkerType() string {
	return s.workerType
}
