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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// factoryRegistryAdapter provides an adapter for config validation
type factoryRegistryAdapter struct{}

func (f *factoryRegistryAdapter) ListRegisteredTypes() []string {
	return factory.ListRegisteredTypes()
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor[TObserved, TDesired]) tickWorker(ctx context.Context, workerID string) error {
	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquire",
		"mutex_name", "supervisor.mu",
		"lock_type", "read",
		"worker_type", s.workerType,
		"worker_id", workerID)

	s.mu.RLock()

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquired",
		"mutex_name", "supervisor.mu",
		"worker_type", s.workerType,
		"worker_id", workerID)

	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_unlock",
		"mutex_name", "supervisor.mu",
		"worker_type", s.workerType,
		"worker_id", workerID)

	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	// Skip if tick already in progress
	if !workerCtx.tickInProgress.CompareAndSwap(false, true) {
		s.logTrace("lifecycle",
			"lifecycle_event", "tick_skip",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"reason", "previous_tick_in_progress")

		return nil
	}
	defer workerCtx.tickInProgress.Store(false)

	s.logTrace("lifecycle",
		"lifecycle_event", "tick_start",
		"worker_type", s.workerType,
		"worker_id", workerID)

	workerCtx.mu.RLock()

	currentStateStr := "nil"
	if workerCtx.currentState != nil {
		currentStateStr = workerCtx.currentState.String()
	}

	s.logTrace("tick_worker",
		"worker_type", s.workerType,
		"worker_id", workerID,
		"current_state", currentStateStr)
	workerCtx.mu.RUnlock()

	// Load latest snapshot from database
	s.logTrace("loading_snapshot",
		"worker_type", s.workerType,
		"worker_id", workerID,
		"stage", "data_freshness")

	storageSnapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
	if err != nil {
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
		return fmt.Errorf("failed to load typed observed state: %w", err)
	}

	// Load typed desired state
	var desired TDesired

	err = s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
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
		s.logTrace("observation_timestamp_loaded",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"stage", "data_freshness",
			"timestamp", observationTimestamp.Format(time.RFC3339Nano))
	} else {
		s.logTrace("observation_no_timestamp",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"stage", "data_freshness",
			"type", fmt.Sprintf("%T", observed))
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
		s.logger.Infow("collector_recovered",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"restart_attempts", s.collectorHealth.restartCount)
		s.collectorHealth.restartCount = 0
	}

	// I16: Validate ObservedState is not nil before progressing FSM
	if snapshot.Observed == nil {
		panic("Invariant I16 violated: attempted to progress FSM with nil ObservedState for worker " + workerID)
	}

	// Action-observation gating: wait for fresh observation after action enqueued.
	// This prevents duplicate action execution when ticker fires faster than collector.
	workerCtx.mu.RLock()
	actionPending := workerCtx.actionPending
	lastActionObsTime := workerCtx.lastActionObsTime
	workerCtx.mu.RUnlock()

	if actionPending {
		// Observation must be NEWER than last action to unlock gating
		var currentObsTime time.Time

		if timestampProvider, ok := any(observed).(interface{ GetTimestamp() time.Time }); ok {
			currentObsTime = timestampProvider.GetTimestamp()
		}

		if currentObsTime.After(lastActionObsTime) {
			workerCtx.mu.Lock()
			workerCtx.actionPending = false
			workerCtx.lastActionObsTime = time.Time{}
			workerCtx.mu.Unlock()

			s.logger.Debugw("action_gating_cleared",
				"worker_type", s.workerType,
				"worker_id", workerID,
				"last_action_time", lastActionObsTime.Format(time.RFC3339Nano),
				"new_observation_time", currentObsTime.Format(time.RFC3339Nano))
		} else {
			s.logTrace("action_gating_blocked",
				"worker_type", s.workerType,
				"worker_id", workerID,
				"last_action_time", lastActionObsTime.Format(time.RFC3339Nano),
				"current_observation_time", currentObsTime.Format(time.RFC3339Nano))

			return nil
		}
	}

	workerCtx.mu.RLock()
	currentState := workerCtx.currentState
	workerCtx.mu.RUnlock()

	if currentState == nil {
		if _, alreadyLogged := s.noStateMachineLoggedOnce.LoadOrStore(workerID, true); !alreadyLogged {
			s.logger.Infow("worker_has_no_state_machine",
				"worker_type", s.workerType,
				"worker_id", workerID,
				"info", "This worker operates as an orchestrator without its own FSM")
		}

		return nil
	}

	nextState, signal, action := currentState.Next(*snapshot)

	hasAction := action != nil
	// Per-tick log moved to TRACE for scalability
	s.logTrace("state_evaluation",
		"worker_type", s.workerType,
		"worker_id", workerID,
		"next_state", nextState.String(),
		"signal", int(signal),
		"has_action", hasAction)

	// VALIDATION: Cannot switch state AND emit action simultaneously
	if nextState != currentState && action != nil {
		panic(fmt.Sprintf("invalid state transition: state %s tried to switch to %s AND emit action %s",
			currentState.String(), nextState.String(), action.Name()))
	}

	// Execute action if present
	if action != nil {
		actionID := action.Name()

		if workerCtx.executor.HasActionInProgress(actionID) {
			s.logTrace("action_skipped",
				"worker_type", s.workerType,
				"action_id", actionID,
				"worker_id", workerID,
				"reason", "already_in_progress")

			return nil
		}

		// Get dependencies from worker if it implements DependencyProvider
		var deps any
		if provider, ok := workerCtx.worker.(fsmv2.DependencyProvider); ok {
			deps = provider.GetDependenciesAny()
		}

		// Action enqueued at DEBUG - low signal for operators
		s.logger.Debugw("action_enqueued",
			"worker_type", s.workerType,
			"action_id", actionID,
			"worker_id", workerID)

		if err := workerCtx.executor.EnqueueAction(actionID, action, deps); err != nil {
			return fmt.Errorf("failed to enqueue action: %w", err)
		}

		// Set action gating: block state.Next() until fresh observation arrives
		workerCtx.mu.Lock()
		workerCtx.actionPending = true
		var currentObsTime time.Time
		if timestampProvider, ok := any(observed).(interface{ GetTimestamp() time.Time }); ok {
			currentObsTime = timestampProvider.GetTimestamp()
		}
		workerCtx.lastActionObsTime = currentObsTime
		workerCtx.mu.Unlock()

		s.logger.Debugw("action_gating_pending",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"action_id", actionID,
			"observation_time", currentObsTime.Format(time.RFC3339Nano))
	}

	// Transition to next state
	if nextState != currentState {
		s.logTrace("lifecycle",
			"lifecycle_event", "state_transition",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"from_state", currentState.String(),
			"to_state", nextState.String(),
			"reason", nextState.Reason())

		s.logger.Infow("state_transition",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"from_state", currentState.String(),
			"to_state", nextState.String(),
			"reason", nextState.Reason())

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_lock_acquire",
			"mutex_name", "workerCtx.mu",
			"lock_type", "write",
			"worker_type", s.workerType,
			"worker_id", workerID)

		workerCtx.mu.Lock()

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_lock_acquired",
			"mutex_name", "workerCtx.mu",
			"worker_type", s.workerType,
			"worker_id", workerID)

		workerCtx.currentState = nextState
		workerCtx.mu.Unlock()

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_unlock",
			"mutex_name", "workerCtx.mu",
			"worker_type", s.workerType,
			"worker_id", workerID)
	} else {
		s.logTrace("state_unchanged",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"state", currentState.String())
	}

	// Process signal
	if err := s.processSignal(ctx, workerID, signal); err != nil {
		return fmt.Errorf("signal processing failed: %w", err)
	}

	s.logTrace("lifecycle",
		"lifecycle_event", "tick_complete",
		"worker_type", s.workerType,
		"worker_id", workerID,
		"final_state", workerCtx.currentState.String())

	return nil
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
// PHASE 2: Async Action Execution
//   - State machine evaluates transitions and enqueues actions when needed
//   - Actions execute in global worker pool (non-blocking)
//   - Timeouts and retries handled automatically by ActionExecutor
//
// PERFORMANCE: The complete tick loop is non-blocking and completes in <10ms,
// making it safe to call at high frequency (100Hz+) without impacting system performance.
func (s *Supervisor[TObserved, TDesired]) tick(ctx context.Context) error {
	// Increment tick counter and log heartbeat periodically
	s.tickCount++
	if s.tickCount%heartbeatTickInterval == 0 {
		s.logHeartbeat()
	}

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
				"worker_type", s.workerType,
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
				"worker_type", s.workerType,
				"failed_child", childErr.ChildName,
				"retry_attempt", attempts,
				"max_attempts", s.healthChecker.maxAttempts,
				"elapsed_downtime", s.healthChecker.backoff.GetTotalDowntime().String(),
				"next_retry_in", nextDelay.String(),
				"recovery_status", s.getRecoveryStatus())

			if attempts == 4 {
				s.logger.Warn("WARNING: One retry attempt remaining before escalation",
					"worker_type", s.workerType,
					"child_name", childErr.ChildName,
					"attempts_remaining", 1,
					"total_downtime", s.healthChecker.backoff.GetTotalDowntime().String())
			}

			if attempts >= 5 {
				s.logger.Error("ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.",
					"worker_type", s.workerType,
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
		s.logger.Infow("circuit_breaker_closed",
			"worker_type", s.workerType,
			"reason", "infrastructure_recovered",
			"total_downtime", downtime.String())
		metrics.RecordCircuitOpen(s.workerType, false)
		metrics.RecordInfrastructureRecovery(s.workerType, downtime)
	}

	s.circuitOpen.Store(false)

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
	// Per-tick log moved to TRACE for scalability
	s.logTrace("variables_propagated",
		"worker_type", s.workerType,
		"worker_id", firstWorkerID,
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
			"worker_type", s.workerType,
			"worker_id", firstWorkerID,
			"error", err.Error(),
			"duration_ms", templateDuration.Milliseconds())
		metrics.RecordTemplateRenderingDuration(s.workerType, "error", templateDuration)
		metrics.RecordTemplateRenderingError(s.workerType, "derivation_failed")

		return fmt.Errorf("failed to derive desired state: %w", err)
	}

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

	// CRITICAL: Preserve ShutdownRequested field if it was set by requestShutdown
	// DeriveDesiredState returns user-derived config, but shutdown is a supervisor operation
	// that must override user config. We load the existing desired state as typed struct
	// and check via DesiredState interface.
	var existingDesiredTyped TDesired
	if err := s.store.LoadDesiredTyped(ctx, s.workerType, firstWorkerID, &existingDesiredTyped); err == nil {
		// Check if existing state had shutdown requested via interface
		if ds, ok := any(existingDesiredTyped).(fsmv2.DesiredState); ok {
			if ds.IsShutdownRequested() {
				desiredDoc["ShutdownRequested"] = true
			}
		}
	}

	_, err = s.store.SaveDesired(ctx, s.workerType, firstWorkerID, desiredDoc)
	if err != nil {
		// Log the error but continue with the tick - the system can recover on the next tick
		// The tickWorker will use the previously saved desired state
		s.logger.Warnf("failed to save derived desired state (will use previous state): %v", err)
	} else {
		// Per-tick log moved to TRACE for scalability
		s.logTrace("derived_desired_state_saved",
			"worker_type", s.workerType,
			"worker_id", firstWorkerID)
	}

	// Validate ChildrenSpecs before reconciliation
	if len(desired.ChildrenSpecs) > 0 {
		registry := &factoryRegistryAdapter{}

		if err := config.ValidateChildSpecs(desired.ChildrenSpecs, registry); err != nil {
			s.logger.Error("child spec validation failed",
				"worker_type", s.workerType,
				"worker_id", firstWorkerID,
				"error", err.Error())

			return fmt.Errorf("invalid child specifications: %w", err)
		}

		// Per-tick log moved to TRACE for scalability
		s.logTrace("child_specs_validated",
			"worker_type", s.workerType,
			"worker_id", firstWorkerID,
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
	s.logTrace("lifecycle",
		"lifecycle_event", "signal_processing",
		"worker_type", s.workerType,
		"worker_id", workerID,
		"signal", int(signal))

	switch signal {
	case fsmv2.SignalNone:
		// Normal operation
		return nil
	case fsmv2.SignalNeedsRemoval:
		s.logger.Infow("worker_removal_signaled",
			"worker_type", s.workerType,
			"worker_id", workerID)

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

		// GRACEFUL CHILD SHUTDOWN: Request graceful shutdown and wait for children
		// to complete their state machine shutdown (e.g., disconnect actions) before
		// proceeding with parent removal. This ensures proper cleanup ordering.
		//
		// Flow:
		// 1. Request graceful shutdown on all children (sets shutdownRequested=true)
		// 2. Wait for children's workers to process their state machines
		//    (e.g., Connected → TryingToDisconnect → disconnect action → Disconnected → SignalNeedsRemoval)
		// 3. After workers are removed, call Shutdown() to clean up supervisor resources
		// 4. Remove from parent's children map

		// TODO: RequestGracefulShutdown method not yet implemented
		_ = ctx

		// Now shutdown child supervisors and clean up
		for name, child := range childrenToCleanup {
			s.logger.Debugw("child_supervisor_shutdown",
				"worker_type", s.workerType,
				"worker_id", workerID,
				"child_name", name,
				"context", "post_graceful_cleanup")
			child.Shutdown()

			// Wait for child supervisor to fully stop
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

		s.logger.Infow("worker_removed_successfully",
			"worker_type", s.workerType,
			"worker_id", workerID,
			"children_cleaned", childCount)

		return nil
	case fsmv2.SignalNeedsRestart:
		s.logger.Infow("worker_restart_signaled",
			"worker_type", s.workerType,
			"worker_id", workerID)

		if err := s.restartCollector(ctx, workerID); err != nil {
			return fmt.Errorf("failed to restart collector for worker %s: %w", workerID, err)
		}

		return nil
	default:
		return fmt.Errorf("unknown signal: %d", signal)
	}
}

// getString extracts a string value from a document map with a default fallback.
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

func (s *Supervisor[TObserved, TDesired]) restartCollector(ctx context.Context, workerID string) error {
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

	// ObservedState interface requires GetTimestamp() - no Document fallback needed
	if timestampProvider, ok := snapshot.Observed.(interface{ GetTimestamp() time.Time }); ok {
		collectedAt = timestampProvider.GetTimestamp()
		hasTimestamp = true
	}

	if !hasTimestamp {
		s.logger.Warn("Snapshot.Observed does not implement GetTimestamp(), cannot check freshness")

		return true
	}

	age = time.Since(collectedAt)

	// During shutdown, log at DEBUG instead of WARN to avoid noisy logs
	// when collectors are already stopped and data is expected to be stale.
	isShuttingDown := !s.started.Load()

	if s.freshnessChecker.IsTimeout(snapshot) {
		if isShuttingDown {
			s.logger.Debugw("data_timeout_during_shutdown",
				"age", age,
				"threshold", s.collectorHealth.timeout)
		} else {
			s.logger.Warnf("Data timeout: observation is %v old (threshold: %v)", age, s.collectorHealth.timeout)
		}

		return false
	}

	if !s.freshnessChecker.Check(snapshot) {
		if isShuttingDown {
			s.logger.Debugw("data_stale_during_shutdown",
				"age", age,
				"threshold", s.collectorHealth.staleThreshold)
		} else {
			s.logger.Warnf("Data stale: observation is %v old (threshold: %v)", age, s.collectorHealth.staleThreshold)
		}

		return false
	}

	return true
}

func (s *Supervisor[TObserved, TDesired]) logHeartbeat() {
	s.mu.RLock()
	workerCount := len(s.workers)
	childCount := len(s.children)

	workerStates := make(map[string]string, workerCount)
	for id, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		if workerCtx.currentState != nil {
			workerStates[id] = workerCtx.currentState.String()
		} else {
			workerStates[id] = "no_state_machine"
		}
		workerCtx.mu.RUnlock()
	}
	s.mu.RUnlock()

	activeActions := 0

	s.logger.Infow("supervisor_heartbeat",
		"worker_type", s.workerType,
		"tick", s.tickCount,
		"workers", workerCount,
		"children", childCount,
		"worker_states", workerStates,
		"active_actions", activeActions)
}

func (s *Supervisor[TObserved, TDesired]) getRecoveryStatus() string {
	attempts := s.healthChecker.backoff.GetAttempts()
	if attempts < 3 {
		return "attempting_recovery"
	} else if attempts < 5 {
		return "persistent_failure"
	}

	return "escalation_imminent"
}

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

func (s *Supervisor[TObserved, TDesired]) reconcileChildren(specs []config.ChildSpec) error {
	startTime := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	var addedCount, updatedCount, removedCount int

	specNames := make(map[string]bool)
	for _, spec := range specs {
		specNames[spec.Name] = true

		if child, exists := s.children[spec.Name]; exists {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_update",
				"child_name", spec.Name,
				"parent_worker_type", s.workerType)

			child.updateUserSpec(spec.UserSpec)
			child.setStateMapping(spec.StateMapping)

			updatedCount++
		} else {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_add_start",
				"child_name", spec.Name,
				"child_worker_type", spec.WorkerType,
				"parent_worker_type", s.workerType)

			s.logger.Infow("child_adding",
				"worker_type", s.workerType,
				"child_name", spec.Name,
				"child_worker_type", spec.WorkerType)

			addedCount++

			childConfig := Config{
				WorkerType:              spec.WorkerType,
				Store:                   s.store,
				Logger:                  s.logger,
				TickInterval:            s.tickInterval,
				GracefulShutdownTimeout: s.gracefulShutdownTimeout,
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
			if _, err := s.store.SaveDesired(childDesiredCtx, spec.WorkerType, childIdentity.ID, desiredDoc); err != nil {
				s.logger.Warnf("Failed to save initial desired state for child %s: %v", spec.Name, err)
			}

			cancel()

			s.children[spec.Name] = childSupervisor

			s.logTrace("lifecycle",
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
			s.logTrace("lifecycle",
				"lifecycle_event", "child_remove_start",
				"child_name", name,
				"parent_worker_type", s.workerType)

			s.logger.Infow("child_removing",
				"worker_type", s.workerType,
				"child_name", name)

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

			s.logTrace("lifecycle",
				"lifecycle_event", "child_remove_complete",
				"child_name", name,
				"parent_worker_type", s.workerType)

			removedCount++
		}
	}

	duration := time.Since(startTime)
	// Log at INFO when topology changes (add/remove), DEBUG for updates only
	if addedCount > 0 || removedCount > 0 {
		// Topology changed - INFO level for operators
		s.logger.Infow("child_reconciliation_completed",
			"worker_type", s.workerType,
			"added", addedCount,
			"updated", updatedCount,
			"removed", removedCount,
			"duration_ms", duration.Milliseconds())
	} else if updatedCount > 0 {
		// Updates only - DEBUG level (per-tick noise)
		s.logger.Debugw("child_reconciliation_completed",
			"worker_type", s.workerType,
			"updated", updatedCount,
			"duration_ms", duration.Milliseconds())
	}
	metrics.RecordReconciliation(s.workerType, "success", duration)

	return nil
}

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
		s.logTrace("state_mapped",
			"worker_type", s.workerType,
			"child_name", childName,
			"parent_state", parentState,
			"mapped_state", mappedState)
	}
}
