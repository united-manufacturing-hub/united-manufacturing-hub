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
	"fmt"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
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

	// Validate DesiredState.State is a valid lifecycle state ("stopped" or "running")
	if valErr := config.ValidateDesiredState(initialDesired.State); valErr != nil {
		return fmt.Errorf("failed to derive initial desired state: %w", valErr)
	}

	// Save identity to database
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

	s.logger.Debugw("initial_observation_saved")

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

	_, err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
	if err != nil {
		return fmt.Errorf("failed to save initial desired state: %w", err)
	}

	s.logger.Debugw("initial_desired_state_saved")

	// Create worker-enriched logger for collector and executor.
	// Important: Use baseLogger (un-enriched) to prevent duplicate "worker" fields.
	// Format: "workerID(workerType)/childID(childType)/..."
	// Example: "scenario123(application)/parent-123(parent)/child001(child)"
	workerLogger := s.baseLogger.With("worker", identity.HierarchyPath)

	// Declare workerCtx early so the closure can capture it.
	// StateProvider can access the FSM state safely.
	var workerCtx *WorkerContext[TObserved, TDesired]

	collector := collection.NewCollector[TObserved](collection.CollectorConfig[TObserved]{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              workerLogger,
		ObservationInterval: DefaultObservationInterval,
		ObservationTimeout:  s.collectorHealth.observationTimeout,
		// StateProvider returns the current FSM state name.
		// The closure captures workerCtx by reference; workerCtx is assigned below
		// before the collector is started (in supervisor.Start), ensuring thread safety.
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
		// ShutdownRequestedProvider returns the current shutdown status from the desired state.
		// It loads the desired state from the database and checks IsShutdownRequested().
		// The closure captures identity.ID and s.store for database access.
		ShutdownRequestedProvider: func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var desired TDesired
			if err := s.store.LoadDesiredTyped(ctx, s.workerType, identity.ID, &desired); err != nil {
				return false
			}

			return desired.IsShutdownRequested()
		},
		// ChildrenCountsProvider returns the count of healthy and unhealthy children.
		// Used by parent workers to track their children's health status for state transitions.
		// A child is considered healthy if its state contains "Running" or "Connected".
		ChildrenCountsProvider: func() (healthy int, unhealthy int) {
			s.mu.RLock()
			defer s.mu.RUnlock()

			for _, child := range s.children {
				stateName := child.GetCurrentStateName()
				// A child is healthy if it's in a running/connected operational state
				if strings.Contains(stateName, "Running") || strings.Contains(stateName, "Connected") {
					healthy++
				} else if stateName != "" && stateName != "unknown" &&
					!strings.Contains(stateName, "Stopped") {
					// Only count as unhealthy if it has a known state that's not healthy
					// and not in Stopped state (TryingToStop still counts as unhealthy
					// so parent waits for children to fully stop)
					unhealthy++
				}
			}
			return healthy, unhealthy
		},
		// MappedParentStateProvider returns the mapped state from parent's ChildStartStates.
		// Used by child workers to know when parent wants them to start/stop.
		// Returns "running" when parent is in a state listed in ChildStartStates,
		// Returns "stopped" when parent is not in any ChildStartStates,
		// Returns "running" when ChildStartStates is empty (child always runs).
		MappedParentStateProvider: func() string {
			return s.getMappedParentState()
		},
		// ChildrenViewProvider returns a config.ChildrenView for parent workers.
		// Used by parent workers to inspect individual child states, not just counts.
		// The closure captures s.children and s.mu for thread-safe access.
		ChildrenViewProvider: func() any {
			s.mu.RLock()
			defer s.mu.RUnlock()

			// Create a copy of children map for the manager
			childrenCopy := make(map[string]SupervisorInterface, len(s.children))
			for name, child := range s.children {
				childrenCopy[name] = child
			}

			return NewChildrenManager(childrenCopy)
		},
	})

	executor := execution.NewActionExecutor(10, identity.ID, workerLogger)

	workerCtx = &WorkerContext[TObserved, TDesired]{
		mu:           s.lockManager.NewLock(lockNameWorkerContextMu, lockLevelWorkerContextMu),
		identity:     identity,
		worker:       worker,
		currentState: worker.GetInitialState(),
		collector:    collector,
		executor:     executor,
	}
	s.workers[identity.ID] = workerCtx

	// Enrich supervisor's own logger with worker path from Identity.
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

		return fmt.Errorf("worker %s not found", workerID)
	}

	delete(s.workers, workerID)
	s.mu.Unlock()

	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	s.logger.Infow("worker_removed")

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
			ID:            id,
			Name:          worker.identity.Name,
			WorkerType:    s.workerType,
			HierarchyPath: worker.identity.HierarchyPath,
		})
	}

	return workers
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

// getMappedParentState implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) getMappedParentState() string {
	return s.GetMappedParentState()
}

// isCircuitOpen returns true if the circuit breaker is open for this supervisor.
// Used by InfrastructureHealthChecker.CheckChildConsistency() to detect unhealthy children.
func (s *Supervisor[TObserved, TDesired]) isCircuitOpen() bool {
	return s.circuitOpen.Load()
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

	// Return a copy to prevent external modification
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

	return s.getHierarchyPathUnlocked()
}

// getHierarchyPathUnlocked computes the hierarchy path without acquiring the lock.
// Caller must hold s.mu.RLock() or s.mu.Lock().
func (s *Supervisor[TObserved, TDesired]) getHierarchyPathUnlocked() string {
	// Get first worker ID for this supervisor's segment
	var workerID string
	for id := range s.workers {
		workerID = id
		break
	}
	if workerID == "" {
		workerID = "unknown"
	}

	// Build this node's path segment: "workerID(workerType)"
	segment := fmt.Sprintf("%s(%s)", workerID, s.workerType)

	// If no parent, we're root - return just our segment
	if s.parent == nil {
		return segment
	}

	// TODO: HOTFIX - Skip parent path to avoid deadlock.
	//
	// Deadlock root cause:
	// 1. Parent supervisor's tick() holds s.mu lock
	// 2. tick() → reconcileChildren() → creates child supervisor
	// 3. childSupervisor.AddWorker() calls getHierarchyPathUnlocked() (line ~158)
	// 4. getHierarchyPathUnlocked() would call s.parent.GetHierarchyPath()
	// 5. GetHierarchyPath() tries to acquire s.mu.RLock() on parent
	// 6. DEADLOCK - parent's lock is already held by tick()!
	//
	// PROPER FIX (requires interface change):
	// 1. Add GetHierarchyPathUnlocked() to SupervisorInterface (types.go)
	// 2. Have all supervisors implement it (this method already exists, just not on interface)
	// 3. Call s.parent.GetHierarchyPathUnlocked() instead of GetHierarchyPath()
	//
	// For now, we skip parent path computation to unblock development.
	// This means child supervisor logs will show "childID(childType)" instead of
	// "parentID(parentType)/childID(childType)" - less context but no deadlock.
	return segment
}

// GetCurrentStateName returns the current FSM state name for this supervisor's worker.
// Returns "unknown" if no worker or state is set.
// Used by parent supervisors to track children's health status.
func (s *Supervisor[TObserved, TDesired]) GetCurrentStateName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get the first (and typically only) worker's current state
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

// GetWorkerType returns the type of workers this supervisor manages.
// Example: "examplechild", "exampleparent", "application"
func (s *Supervisor[TObserved, TDesired]) GetWorkerType() string {
	return s.workerType
}
