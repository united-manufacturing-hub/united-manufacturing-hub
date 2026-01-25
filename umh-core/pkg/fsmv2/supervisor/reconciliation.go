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
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// factoryRegistryAdapter provides an adapter for config validation.
type factoryRegistryAdapter struct{}

func (f *factoryRegistryAdapter) ListRegisteredTypes() []string {
	return factory.ListRegisteredTypes()
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor[TObserved, TDesired]) tickWorker(ctx context.Context, workerID string) error {
	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquire",
		"mutex_name", "supervisor.mu",
		"lock_type", "read")

	s.mu.RLock()

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquired",
		"mutex_name", "supervisor.mu")

	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_unlock",
		"mutex_name", "supervisor.mu")

	if !exists {
		// Worker was removed via SignalNeedsRemoval during concurrent tick.
		// This race can happen during:
		// 1. Shutdown() (s.started == false)
		// 2. RequestShutdown() from reconcileChildren (s.started == true)
		// WorkerIDs are always obtained from iterating s.workers, so
		// "not found" is only possible due to this benign race condition.
		s.logger.Debugw("tick_worker_skip",
			"reason", "worker_removed_during_shutdown",
			"worker_id", workerID)

		return nil
	}

	// Skip if tick already in progress
	if !workerCtx.tickInProgress.CompareAndSwap(false, true) {
		s.logTrace("lifecycle",
			"lifecycle_event", "tick_skip",
			"reason", "previous_tick_in_progress")

		return nil
	}
	defer workerCtx.tickInProgress.Store(false)

	s.logTrace("lifecycle",
		"lifecycle_event", "tick_start")

	workerCtx.mu.RLock()

	currentStateStr := "nil"
	if workerCtx.currentState != nil {
		currentStateStr = workerCtx.currentState.String()
	}

	s.logTrace("tick_worker",
		"current_state", currentStateStr)
	workerCtx.mu.RUnlock()

	s.logTrace("loading_snapshot",
		"stage", "data_freshness")

	storageSnapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// I16: Validate ObservedState is not nil from storage
	if storageSnapshot.Observed == nil {
		panic("Invariant I16 violated: storage returned nil ObservedState for worker " + workerID)
	}

	var observed TObserved

	err = s.store.LoadObservedTyped(ctx, s.workerType, workerID, &observed)
	if err != nil {
		return fmt.Errorf("failed to load typed observed state: %w", err)
	}

	var desired TDesired

	err = s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
		return fmt.Errorf("failed to load typed desired state: %w", err)
	}

	snapshot := &fsmv2.Snapshot{
		Identity: deps.Identity{
			ID:            workerID,
			Name:          getString(storageSnapshot.Identity, FieldName, workerID),
			WorkerType:    workerCtx.identity.WorkerType,
			HierarchyPath: workerCtx.identity.HierarchyPath,
		},
		Observed: observed,
		Desired:  desired,
	}

	if timestampProvider, ok := any(observed).(fsmv2.TimestampProvider); ok {
		observationTimestamp := timestampProvider.GetTimestamp()
		s.logTrace("observation_timestamp_loaded",
			"stage", "data_freshness",
			"timestamp", observationTimestamp.Format(time.RFC3339Nano))
	} else {
		s.logTrace("observation_no_timestamp",
			"stage", "data_freshness",
			"type", fmt.Sprintf("%T", observed))
	}

	// Shutdown bypass: Allow FSM to process shutdown even with stale data.
	// During graceful shutdown, child supervisors shut down first (correct order),
	// which causes parent's observation to become stale (collectors can't observe gone children).
	// The shutdown transition only needs the ShutdownRequested flag, not fresh observation data.
	var isShutdownRequested bool
	if ds, ok := snapshot.Desired.(fsmv2.DesiredState); ok {
		isShutdownRequested = ds.IsShutdownRequested()
	}

	// I3: Check data freshness BEFORE calling state.Next()
	// This is the trust boundary: states assume data is always fresh
	// EXCEPT when shutdown is requested - shutdown doesn't need fresh observation
	if !isShutdownRequested && !s.checkDataFreshness(snapshot) {
		if s.freshnessChecker.IsTimeout(snapshot) {
			// I4: Check if we've exhausted restart attempts
			// Acquire lock to read mutable collectorHealth fields (restartCount)
			// Note: maxRestartAttempts is immutable (set in constructor), no lock needed
			s.mu.RLock()
			restartCount := s.collectorHealth.restartCount
			s.mu.RUnlock()

			if restartCount >= s.collectorHealth.maxRestartAttempts {
				// Max attempts reached - escalate to shutdown (Layer 3)
				s.logger.Errorw("collector_unresponsive_max_attempts",
					"restart_attempts", s.collectorHealth.maxRestartAttempts)

				if shutdownErr := s.requestShutdown(ctx, workerID,
					fmt.Sprintf("collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)); shutdownErr != nil {
					s.logger.Errorw("shutdown_request_failed",
						"error", shutdownErr)
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

	// Check and reset restartCount under lock to prevent data races
	s.mu.Lock()

	if s.collectorHealth.restartCount > 0 {
		restartCount := s.collectorHealth.restartCount
		s.collectorHealth.restartCount = 0
		s.mu.Unlock()

		s.logger.Infow("collector_recovered",
			"restart_attempts", restartCount)
	} else {
		s.mu.Unlock()
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

		if timestampProvider, ok := any(observed).(fsmv2.TimestampProvider); ok {
			currentObsTime = timestampProvider.GetTimestamp()
		}

		if currentObsTime.After(lastActionObsTime) {
			workerCtx.mu.Lock()
			workerCtx.actionPending = false
			workerCtx.lastActionObsTime = time.Time{}
			workerCtx.mu.Unlock()

			s.logger.Debugw("action_gating_cleared",
				"last_action_time", lastActionObsTime.Format(time.RFC3339Nano),
				"new_observation_time", currentObsTime.Format(time.RFC3339Nano))
		} else {
			s.logTrace("action_gating_blocked",
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
				"info", "This worker manages child workers without its own FSM")
		}

		return nil
	}

	// NOTE: FrameworkMetrics injection is now handled automatically by the Collector
	// via FrameworkMetricsProvider callback. See collector.go collectAndSaveObservedState().
	// The provider is set up in api.go AddWorker() when creating the CollectorConfig.

	nextState, signal, action := currentState.Next(*snapshot)

	hasAction := action != nil
	// Per-tick log moved to TRACE for scalability
	s.logTrace("state_evaluation",
		"next_state", nextState.String(),
		"signal", int(signal),
		"has_action", hasAction)

	// FSM invariant: state transition and action emission are mutually exclusive
	if nextState != currentState && action != nil {
		panic(fmt.Sprintf("invalid state transition: state %s tried to switch to %s AND emit action %s",
			currentState.String(), nextState.String(), action.Name()))
	}

	if action != nil {
		actionID := action.Name()

		if workerCtx.executor.HasActionInProgress(actionID) {
			s.logTrace("action_skipped",
				"action_id", actionID,
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
			"action_id", actionID)

		if err := workerCtx.executor.EnqueueAction(actionID, action, deps); err != nil {
			return fmt.Errorf("failed to enqueue action: %w", err)
		}

		// Block FSM progression until fresh observation confirms action effect
		workerCtx.mu.Lock()
		workerCtx.actionPending = true

		var currentObsTime time.Time

		if timestampProvider, ok := any(observed).(fsmv2.TimestampProvider); ok {
			currentObsTime = timestampProvider.GetTimestamp()
		}

		workerCtx.lastActionObsTime = currentObsTime
		workerCtx.mu.Unlock()

		s.logger.Debugw("action_gating_pending",
			"action_id", actionID,
			"observation_time", currentObsTime.Format(time.RFC3339Nano))
	}

	if nextState != currentState {
		fromState := currentState.String()
		toState := nextState.String()
		now := time.Now()

		s.logTrace("lifecycle",
			"lifecycle_event", "state_transition",
			"from_state", fromState,
			"to_state", toState,
			"reason", nextState.Reason())

		s.logger.Infow("state_transition",
			"from_state", fromState,
			"to_state", toState,
			"reason", nextState.Reason())

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_lock_acquire",
			"mutex_name", "workerCtx.mu",
			"lock_type", "write")

		workerCtx.mu.Lock()

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_lock_acquired",
			"mutex_name", "workerCtx.mu")

		if !workerCtx.stateEnteredAt.IsZero() {
			timeInState := now.Sub(workerCtx.stateEnteredAt)
			workerCtx.stateDurations[fromState] += timeInState
		}

		workerCtx.stateTransitions[toState]++
		workerCtx.totalTransitions++
		workerCtx.currentState = nextState

		// Exposed via FrameworkMetrics and GetCurrentStateNameAndReason
		workerCtx.currentStateReason = nextState.Reason()
		workerCtx.stateEnteredAt = now

		workerCtx.mu.Unlock()

		s.logTrace("lifecycle",
			"lifecycle_event", "mutex_unlock",
			"mutex_name", "workerCtx.mu")

		// Record Prometheus metric AFTER lock release
		metrics.RecordStateTransition(s.workerType, fromState, toState)
	} else {
		s.logTrace("state_unchanged",
			"state", currentState.String())
	}

	workerCtx.mu.RLock()

	if workerCtx.currentState != nil && !workerCtx.stateEnteredAt.IsZero() {
		metrics.RecordStateDuration(
			s.workerType,
			workerID,
			workerCtx.currentState.String(),
			time.Since(workerCtx.stateEnteredAt),
		)
	}

	// Capture final state while holding lock for logging after processSignal
	finalState := "nil"
	if workerCtx.currentState != nil {
		finalState = workerCtx.currentState.String()
	}

	workerCtx.mu.RUnlock()

	if err := s.processSignal(ctx, workerID, signal); err != nil {
		return fmt.Errorf("signal processing failed: %w", err)
	}

	s.logTrace("lifecycle",
		"lifecycle_event", "tick_complete",
		"final_state", finalState)

	return nil
}

// Tick performs one supervisor tick cycle, integrating all FSMv2 phases.
//
// ARCHITECTURE: This method executes four phases in priority order:
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
	tickCount := atomic.AddUint64(&s.tickCount, 1)
	if tickCount%heartbeatTickInterval == 0 {
		s.logHeartbeat()
	}

	s.checkRestartTimeouts(ctx)

	// PHASE 1: Infrastructure health check
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
				"failed_child", childErr.ChildName,
				"retry_attempt", attempts,
				"max_attempts", s.healthChecker.maxAttempts,
				"elapsed_downtime", s.healthChecker.backoff.GetTotalDowntime().String(),
				"next_retry_in", nextDelay.String(),
				"recovery_status", s.getRecoveryStatus())

			if attempts == 4 {
				s.logger.Warn("Warning: One retry attempt remaining before escalation",
					"child_name", childErr.ChildName,
					"attempts_remaining", 1,
					"total_downtime", s.healthChecker.backoff.GetTotalDowntime().String())
			}

			if attempts >= 5 {
				s.logger.Error("Escalation required: Infrastructure failure after max retry attempts. Manual intervention needed.",
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
		// During shutdown (started=false), having no workers is expected.
		// Return nil to allow tick loop to exit gracefully when context is cancelled.
		if !s.started.Load() {
			return nil
		}

		return errors.New("no workers in supervisor")
	}

	if worker == nil {
		return errors.New("worker is nil")
	}

	// PHASE 0.5: Variable Injection
	// Inject variables BEFORE DeriveDesiredState() so they're available for template expansion
	// Use getUserSpec() to avoid race with parent calling updateUserSpec()
	userSpecWithVars := s.getUserSpec()

	if userSpecWithVars.Variables.User == nil {
		userSpecWithVars.Variables.User = make(map[string]any)
	}

	s.mu.RLock()
	userSpecWithVars.Variables.Global = s.globalVars
	globalVarCount := len(s.globalVars)
	s.mu.RUnlock()

	userSpecWithVars.Variables.Internal = map[string]any{
		FieldID:                firstWorkerID,
		storage.FieldCreatedAt: s.createdAt,
		FieldParentID:          s.parentID,
	}

	userVarCount := len(userSpecWithVars.Variables.User)
	// Per-tick log moved to TRACE for scalability
	s.logTrace("variables_propagated",
		"user_vars", userVarCount,
		"global_vars", globalVarCount)
	metrics.RecordVariablePropagation(s.workerType)

	// PHASE 0: Hierarchical Composition
	// 1. DeriveDesiredState (with caching)

	currentHash := config.ComputeUserSpecHash(userSpecWithVars)

	s.mu.RLock()
	lastHash := s.lastUserSpecHash
	cachedState := s.cachedDesiredState
	s.mu.RUnlock()

	// desired is fsmv2.DesiredState interface - workers return their typed DesiredState
	var desired fsmv2.DesiredState

	if lastHash == currentHash && cachedState != nil {
		// Cache hit - reuse previous result
		desired = cachedState

		s.logTrace("derive_desired_state_cached",
			"hash", currentHash[:8]+"...")
	} else {
		// Cache miss - call DeriveDesiredState
		templateStart := time.Now()

		var err error

		desired, err = worker.DeriveDesiredState(userSpecWithVars)
		templateDuration := time.Since(templateStart)

		if err != nil {
			s.logger.Error("template rendering failed",
				"error", err.Error(),
				"duration_ms", templateDuration.Milliseconds())
			metrics.RecordTemplateRenderingDuration(s.workerType, "error", templateDuration)
			metrics.RecordTemplateRenderingError(s.workerType, "derivation_failed")

			// Don't cache errors - will retry next tick
			return fmt.Errorf("failed to derive desired state: %w", err)
		}

		// Validate DesiredState.State is a valid lifecycle state ("stopped" or "running")
		// This catches both developer mistakes (hardcoded wrong values) and user config mistakes
		// Use GetState() method from fsmv2.DesiredState interface
		if valErr := config.ValidateDesiredState(desired.GetState()); valErr != nil {
			s.logger.Error("invalid desired state from DeriveDesiredState",
				"state", desired.GetState(),
				"worker_id", firstWorkerID,
				"error", valErr)
			metrics.RecordTemplateRenderingDuration(s.workerType, "error", templateDuration)
			metrics.RecordTemplateRenderingError(s.workerType, "invalid_state_value")

			// Don't cache validation errors - will retry next tick
			return fmt.Errorf("failed to derive desired state: %w", valErr)
		}

		// Update cache
		s.mu.Lock()
		s.lastUserSpecHash = currentHash
		s.cachedDesiredState = desired
		s.mu.Unlock()

		s.logTrace("derive_desired_state_computed",
			"hash", currentHash[:8]+"...",
			"duration_ms", templateDuration.Milliseconds())
		metrics.RecordTemplateRenderingDuration(s.workerType, "success", templateDuration)
	}

	// Save before tickWorker, which loads the freshest desired state from snapshot
	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal derived desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal derived desired state to document: %w", err)
	}

	desiredDoc[FieldID] = firstWorkerID

	// Preserve ShutdownRequested: shutdown is a supervisor operation that overrides DeriveDesiredState
	var existingDesiredTyped TDesired
	if err := s.store.LoadDesiredTyped(ctx, s.workerType, firstWorkerID, &existingDesiredTyped); err == nil {
		// Check if existing state had shutdown requested via interface
		if ds, ok := any(existingDesiredTyped).(fsmv2.DesiredState); ok {
			if ds.IsShutdownRequested() {
				desiredDoc[FieldShutdownRequested] = true
			}
		}
	}

	_, err = s.store.SaveDesired(ctx, s.workerType, firstWorkerID, desiredDoc)
	if err != nil {
		// Log the error but continue with the tick - the system can recover on the next tick
		// The tickWorker will use the previously saved desired state
		s.logger.Warnw("derived_desired_state_save_failed",
			"error", err)
	} else {
		// Per-tick log moved to TRACE for scalability
		s.logTrace("derived_desired_state_saved")
	}

	var childrenSpecs []config.ChildSpec
	if provider, ok := desired.(config.ChildSpecProvider); ok {
		childrenSpecs = provider.GetChildrenSpecs()
	}

	// Validate only changed/new ChildrenSpecs before reconciliation (incremental validation)
	if len(childrenSpecs) > 0 {
		registry := &factoryRegistryAdapter{}
		specsToValidate := make([]config.ChildSpec, 0)

		for _, spec := range childrenSpecs {
			hash := spec.Hash()
			if cachedHash, exists := s.validatedSpecHashes[spec.Name]; !exists || cachedHash != hash {
				specsToValidate = append(specsToValidate, spec)
			}
		}

		if len(specsToValidate) > 0 {
			if err := config.ValidateChildSpecs(specsToValidate, registry); err != nil {
				s.logger.Error("child spec validation failed",
					"error", err.Error())

				return fmt.Errorf("invalid child specifications: %w", err)
			}

			// Update cache for validated specs
			for _, spec := range specsToValidate {
				s.validatedSpecHashes[spec.Name] = spec.Hash()
			}

			s.logTrace("child_specs_validated",
				"validated_count", len(specsToValidate),
				"total_count", len(childrenSpecs))
		}

		// Clean up removed specs from cache
		currentNames := make(map[string]bool)
		for _, spec := range childrenSpecs {
			currentNames[spec.Name] = true
		}

		for name := range s.validatedSpecHashes {
			if !currentNames[name] {
				delete(s.validatedSpecHashes, name)
			}
		}
	}

	// Tick worker before creating children to progress FSM state first
	if err := s.tickWorker(ctx, firstWorkerID); err != nil {
		return fmt.Errorf("failed to tick worker: %w", err)
	}

	// When shutting down, pass nil to trigger graceful child shutdown instead of re-creation
	if desiredDoc[FieldShutdownRequested] == true {
		childrenSpecs = nil
	}

	if err := s.reconcileChildren(childrenSpecs); err != nil {
		return fmt.Errorf("failed to reconcile children: %w", err)
	}

	s.applyStateMapping()

	// Tick children; errors logged but don't fail parent
	s.mu.RLock()

	childrenToTick := make([]SupervisorInterface, 0, len(s.children))
	for _, child := range s.children {
		childrenToTick = append(childrenToTick, child)
	}

	s.mu.RUnlock()

	for _, child := range childrenToTick {
		if err := child.tick(ctx); err != nil {
			s.logger.Errorw("child_tick_failed",
				"error", err)
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
		"signal", int(signal))

	switch signal {
	case fsmv2.SignalNone:
		// Normal operation
		return nil
	case fsmv2.SignalNeedsRemoval:
		s.logger.Debugw("worker_removal_signaled")

		// Check if this worker should be restarted instead of removed
		s.mu.Lock()

		shouldRestart := s.pendingRestart[workerID]
		if shouldRestart {
			delete(s.pendingRestart, workerID)
			delete(s.restartRequestedAt, workerID)
		}

		s.mu.Unlock()

		if shouldRestart {
			s.logger.Infow("worker_restarting",
				"worker", workerID,
				"reason", "restart requested after graceful shutdown")

			return s.handleWorkerRestart(ctx, workerID)
		}

		// Original removal flow continues below...
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

			s.logger.Warnw("worker_removal_has_children",
				"child_count", childCount,
				"children", childNames)
		}

		delete(s.workers, workerID)
		s.mu.Unlock()

		// Clean up children outside parent lock to avoid deadlock with GetChildren/calculateHierarchySize
		doneChannels := make(map[string]<-chan struct{})

		s.mu.Lock()

		for name := range childrenToCleanup {
			if done, exists := s.childDoneChans[name]; exists {
				doneChannels[name] = done
			}
		}

		s.mu.Unlock()

		// TODO: Use context-aware RequestGracefulShutdown instead of immediate Shutdown
		_ = ctx

		for name, child := range childrenToCleanup {
			s.logger.Debugw("child_supervisor_shutdown",
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

		// Capture terminal state (e.g., "Stopped") before shutdown
		if workerCtx.collector.IsRunning() {
			collectCtx, cancel := context.WithTimeout(ctx, s.collectorHealth.observationTimeout)
			_ = workerCtx.collector.CollectFinalObservation(collectCtx)

			cancel()
		}

		workerCtx.collector.Stop(ctx)
		workerCtx.executor.Shutdown()

		s.logger.Debugw("worker_removed_successfully",
			"children_cleaned", childCount)

		return nil
	case fsmv2.SignalNeedsRestart:
		s.logger.Infow("worker_restart_requested",
			"worker", workerID,
			"reason", "worker signaled unrecoverable error")

		// Mark for restart (will be checked when SignalNeedsRemoval is received)
		s.mu.Lock()
		s.pendingRestart[workerID] = true
		s.restartRequestedAt[workerID] = time.Now()
		s.mu.Unlock()

		// Request graceful shutdown - worker will go through cleanup states
		if err := s.requestShutdown(ctx, workerID, "restart_requested"); err != nil {
			s.logger.Warnw("restart_shutdown_request_failed",
				"worker", workerID, "error", err)
			// Continue anyway - we want to restart even if request fails
		}

		return nil
	default:
		return fmt.Errorf("unknown signal: %d", signal)
	}
}

// checkRestartTimeouts checks for workers that have been pending restart too long
// and force-resets them. Called from tick() to handle stuck restart requests.
func (s *Supervisor[TObserved, TDesired]) checkRestartTimeouts(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for workerID := range s.pendingRestart {
		requestedAt, exists := s.restartRequestedAt[workerID]
		if !exists {
			continue
		}

		if time.Since(requestedAt) > DefaultGracefulRestartTimeout {
			s.logger.Warnw("restart_graceful_timeout",
				"worker", workerID,
				"timeout", DefaultGracefulRestartTimeout,
				"waited", time.Since(requestedAt))

			workerCtx, workerExists := s.workers[workerID]
			if !workerExists {
				delete(s.pendingRestart, workerID)
				delete(s.restartRequestedAt, workerID)

				continue
			}

			// Force reset (skip waiting for SignalNeedsRemoval)
			// Acquire workerCtx.mu to safely access/modify currentState.
			// Lock order: Supervisor.mu (already held) -> WorkerContext.mu
			workerCtx.mu.Lock()

			fromState := "nil"
			if workerCtx.currentState != nil {
				fromState = workerCtx.currentState.String()
			}

			s.logger.Infow("worker_restart_force_reset",
				"worker", workerID,
				"from_state", fromState)

			workerCtx.currentState = workerCtx.worker.GetInitialState()

			toState := "nil"
			if workerCtx.currentState != nil {
				toState = workerCtx.currentState.String()
			}

			workerCtx.mu.Unlock()

			if workerCtx.collector != nil {
				workerCtx.collector.Restart()
			}

			// Clear pending restart
			delete(s.pendingRestart, workerID)
			delete(s.restartRequestedAt, workerID)

			s.logger.Infow("worker_restart_force_complete",
				"worker", workerID,
				"to_state", toState)
		}
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
	// Acquire lock to access mutable collectorHealth fields (restartCount, lastRestart)
	// Note: maxRestartAttempts is immutable (set in constructor), no lock needed for read
	s.mu.Lock()

	if s.collectorHealth.restartCount >= s.collectorHealth.maxRestartAttempts {
		s.mu.Unlock()
		panic(fmt.Sprintf("supervisor bug: RestartCollector called with restartCount=%d >= maxRestartAttempts=%d (should have escalated to shutdown)",
			s.collectorHealth.restartCount, s.collectorHealth.maxRestartAttempts))
	}

	// Non-blocking backoff: check if sufficient time has elapsed since last restart.
	// This allows the tick loop to complete quickly (<100ms) instead of blocking.
	// The backoff increases with each restart attempt: 2s, 4s, 6s, etc.
	backoff := time.Duration((s.collectorHealth.restartCount+1)*2) * time.Second

	// If this is not the first restart, check if backoff period has elapsed
	if !s.collectorHealth.lastRestart.IsZero() {
		elapsed := time.Since(s.collectorHealth.lastRestart)
		if elapsed < backoff {
			restartCount := s.collectorHealth.restartCount
			s.mu.Unlock()

			// Backoff not yet elapsed - skip this restart attempt (non-blocking)
			s.logger.Debugw("collector_restart_backoff",
				"restart_attempt", restartCount,
				"backoff_remaining", backoff-elapsed)

			return nil
		}
	}

	// Proceed with restart
	s.collectorHealth.restartCount++
	s.collectorHealth.lastRestart = time.Now()

	// Capture values for logging after unlock
	restartCount := s.collectorHealth.restartCount
	maxRestartAttempts := s.collectorHealth.maxRestartAttempts
	s.mu.Unlock()

	s.logger.Warnw("collector_restarting",
		"restart_attempt", restartCount,
		"max_attempts", maxRestartAttempts,
		"backoff", backoff)

	s.mu.RLock()
	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	workerCtx.mu.Lock()
	workerCtx.collectorRestarts++
	workerCtx.mu.Unlock()

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
	if timestampProvider, ok := snapshot.Observed.(fsmv2.TimestampProvider); ok {
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
			s.logger.Warnw("data_timeout",
				"age", age,
				"threshold", s.collectorHealth.timeout)
		}

		return false
	}

	if !s.freshnessChecker.Check(snapshot) {
		if isShuttingDown {
			s.logger.Debugw("data_stale_during_shutdown",
				"age", age,
				"threshold", s.collectorHealth.staleThreshold)
		} else {
			s.logger.Warnw("data_stale",
				"age", age,
				"threshold", s.collectorHealth.staleThreshold)
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
		"tick", atomic.LoadUint64(&s.tickCount),
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

			// Clear pendingRemoval if child was marked for removal but re-appeared in specs
			// This handles the case where a child disappears temporarily (e.g., during restart)
			// and then re-appears in the desired spec before shutdown completes
			if s.pendingRemoval[spec.Name] {
				s.logger.Debugw("child_reappeared_clearing_pending_removal",
					"child_name", spec.Name)
				delete(s.pendingRemoval, spec.Name)
			}

			// Merge parent User variables with child's (child overrides parent)
			// Direct access is safe here - reconcileChildren holds s.mu.Lock()
			childUserSpec := spec.UserSpec
			childUserSpec.Variables = config.Merge(s.userSpec.Variables, spec.UserSpec.Variables)
			child.updateUserSpec(childUserSpec)
			child.setChildStartStates(spec.ChildStartStates)

			updatedCount++
		} else {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_add_start",
				"child_name", spec.Name,
				"child_worker_type", spec.WorkerType,
				"parent_worker_type", s.workerType)

			s.logger.Infow("child_adding",
				"child_name", spec.Name,
				"child_worker_type", spec.WorkerType)

			addedCount++

			// Merge parent dependencies with child's (child overrides parent)
			mergedDeps := config.MergeDependencies(s.deps, spec.Dependencies)

			s.logTrace("lifecycle",
				"lifecycle_event", "dependencies_merged",
				"child_name", spec.Name,
				"parent_dep_count", len(s.deps),
				"child_dep_count", len(spec.Dependencies),
				"merged_dep_count", len(mergedDeps))

			childConfig := Config{
				WorkerType:              spec.WorkerType,
				Store:                   s.store,
				Logger:                  s.baseLogger, // Important: Use un-enriched logger to prevent duplicate "worker" fields
				TickInterval:            s.tickInterval,
				GracefulShutdownTimeout: s.gracefulShutdownTimeout,
				Dependencies:            mergedDeps,
			}

			// Use factory to create child supervisor with proper type
			rawSupervisor, err := factory.NewSupervisorByType(spec.WorkerType, childConfig)
			if err != nil {
				s.logger.Errorw("child_supervisor_creation_failed",
					"child_name", spec.Name,
					"error", err)

				continue
			}

			childSupervisor, ok := rawSupervisor.(SupervisorInterface)
			if !ok {
				s.logger.Errorw("factory_invalid_supervisor_type",
					"child_name", spec.Name)

				continue
			}

			// Merge parent User variables with child's (child overrides parent)
			// Direct access is safe here - reconcileChildren holds s.mu.Lock()
			childUserSpec := spec.UserSpec
			childUserSpec.Variables = config.Merge(s.userSpec.Variables, spec.UserSpec.Variables)
			childSupervisor.updateUserSpec(childUserSpec)
			childSupervisor.setChildStartStates(spec.ChildStartStates)
			childSupervisor.setParent(s, s.workerType)

			// Compute child's hierarchy path: parent path + child segment
			// Format: "parentID(parentType)/childID(childType)"
			childID := spec.Name + "-001"

			// Get parent's full hierarchy path from first worker's stored identity
			// (uses stored identity for consistency with how paths are assigned)
			var parentPath string

			for _, wCtx := range s.workers {
				if wCtx.identity.HierarchyPath != "" {
					parentPath = wCtx.identity.HierarchyPath

					break
				}
			}

			childPath := fmt.Sprintf("%s/%s(%s)", parentPath, childID, spec.WorkerType)

			// Create worker identity with hierarchy path for logging context
			childIdentity := deps.Identity{
				ID:            childID,
				Name:          spec.Name,
				WorkerType:    spec.WorkerType,
				HierarchyPath: childPath,
			}

			// Use factory to create worker instance with un-enriched logger
			// Important: Pass baseLogger to prevent duplicate "worker" fields
			// Use mergedDeps to include both parent and child-specific dependencies
			childWorker, err := factory.NewWorkerByType(spec.WorkerType, childIdentity, s.baseLogger, s.store, mergedDeps)
			if err != nil {
				s.logger.Errorw("child_worker_creation_failed",
					"child_name", spec.Name,
					"error", err)

				continue
			}

			// Add worker to child supervisor
			if err := childSupervisor.AddWorker(childIdentity, childWorker); err != nil {
				s.logger.Errorw("child_supervisor_add_worker_failed",
					"child_name", spec.Name,
					"error", err)

				continue
			}

			// Save initial desired state for child (empty document to avoid nil on first tick)
			childDesiredCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			desiredDoc := persistence.Document{
				FieldID:                childIdentity.ID,
				FieldShutdownRequested: false,
			}
			if _, err := s.store.SaveDesired(childDesiredCtx, spec.WorkerType, childIdentity.ID, desiredDoc); err != nil {
				s.logger.Warnw("child_initial_desired_state_save_failed",
					"child_name", spec.Name,
					"error", err)
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
					s.logger.Warnw("child_start_skipped_context_cancelled",
						"child_name", spec.Name)
				}
			}
		}
	}

	// Phase 1: Request shutdown for children not in specs (mark as pendingRemoval)
	for name := range s.children {
		if !specNames[name] && !s.pendingRemoval[name] {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_shutdown_requested",
				"child_name", name,
				"parent_worker_type", s.workerType)

			s.logger.Debugw("child_shutdown_requesting",
				"child_name", name)

			// Mark child as pending removal
			s.pendingRemoval[name] = true

			child := s.children[name]
			if child != nil {
				// Request shutdown - sets ShutdownRequested=true on child's workers
				// Child continues ticking and will emit SignalNeedsRemoval when ready
				ctx := context.Background()
				if err := child.RequestShutdown(ctx, "removed_from_specs"); err != nil {
					s.logger.Warnw("child_shutdown_request_failed",
						"child_name", name,
						"error", err)
				}
			}
			// DON'T delete here - wait for child's workers to complete shutdown
		}
	}

	// Phase 2: Complete removal of children that have finished shutdown
	for name := range s.pendingRemoval {
		child := s.children[name]
		if child == nil {
			delete(s.pendingRemoval, name)

			continue
		}

		// Check if child has no more workers (all emitted SignalNeedsRemoval)
		if len(child.ListWorkers()) == 0 {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_remove_start",
				"child_name", name,
				"parent_worker_type", s.workerType)

			s.logger.Debugw("child_shutdown_complete",
				"child_name", name)

			// Now safe to fully shut down and remove
			child.Shutdown()

			if done, exists := s.childDoneChans[name]; exists {
				<-done
				delete(s.childDoneChans, name)
			}

			delete(s.children, name)
			delete(s.pendingRemoval, name)

			s.logTrace("lifecycle",
				"lifecycle_event", "child_remove_complete",
				"child_name", name,
				"parent_worker_type", s.workerType)

			removedCount++
		}
	}

	duration := time.Since(startTime)
	// Log at DEBUG for all reconciliation completions
	if addedCount > 0 || removedCount > 0 {
		// Topology changed
		s.logger.Debugw("child_reconciliation_completed",
			"added", addedCount,
			"updated", updatedCount,
			"removed", removedCount,
			"duration_ms", duration.Milliseconds())
	} else if updatedCount > 0 {
		// Updates only - DEBUG level (per-tick noise)
		s.logger.Debugw("child_reconciliation_completed",
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
		mappedState := s.computeMappedState(parentState, child)

		child.setMappedParentState(mappedState)
		s.logTrace("state_mapped",
			"child_name", childName,
			"parent_state", parentState,
			"mapped_state", mappedState)
	}
}

// computeMappedState determines the desired state for a child based on parent's current state.
//
// ChildStartStates logic:
//   - If empty: child always runs (follows parent's DesiredState.State)
//   - If parentState is in the list: child should run (returns "running")
//   - Otherwise: child should stop (returns "stopped")
func (s *Supervisor[TObserved, TDesired]) computeMappedState(parentState string, child SupervisorInterface) string {
	childStartStates := child.getChildStartStates()

	// Empty ChildStartStates = child always runs (follows parent's desired state)
	if len(childStartStates) == 0 {
		return config.DesiredStateRunning
	}

	// Check if parent state is in the list of states where child should run
	for _, state := range childStartStates {
		if state == parentState {
			return config.DesiredStateRunning
		}
	}

	return config.DesiredStateStopped
}
