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
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/panicutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// ErrPanicCircuitOpen is returned when the tick is suppressed because the panic circuit breaker is open.
// Returning an error (instead of nil) ensures the supervisor knows the worker is unhealthy
// and continues ticking it, allowing the circuit breaker to auto-reset once the sliding window
// of recent panics drains (panics older than the window are forgotten).
var ErrPanicCircuitOpen = errors.New("panic circuit breaker open")

// ErrInfraCircuitOpen is returned when the tick is suppressed because the infrastructure circuit breaker is open.
var ErrInfraCircuitOpen = errors.New("infrastructure circuit breaker open")

// factoryRegistryAdapter provides an adapter for config validation.
type factoryRegistryAdapter struct{}

func (f *factoryRegistryAdapter) ListRegisteredTypes() []string {
	return factory.ListRegisteredTypes()
}

// tickWorker performs one FSM tick for a specific worker.
func (s *Supervisor[TObserved, TDesired]) tickWorker(ctx context.Context, workerID string) error {
	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "mutex_lock_acquire"),
		deps.String("mutex_name", "supervisor.mu"),
		deps.String("lock_type", "read"))

	s.mu.RLock()

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "mutex_lock_acquired"),
		deps.String("mutex_name", "supervisor.mu"))

	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "mutex_unlock"),
		deps.String("mutex_name", "supervisor.mu"))

	if !exists {
		// Worker was removed via SignalNeedsRemoval during concurrent tick.
		// This race can happen during:
		// 1. Shutdown() (s.started == false)
		// 2. RequestShutdown() from reconcileChildren (s.started == true)
		// WorkerIDs are always obtained from iterating s.workers, so
		// "not found" is only possible due to this benign race condition.
		s.logger.Debug("tick_worker_skip",
			deps.Reason("worker_removed_during_shutdown"),
			deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
			deps.String("target_worker_id", workerID))

		return nil
	}

	// Skip if tick already in progress
	if !workerCtx.tickInProgress.CompareAndSwap(false, true) {
		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "tick_skip"),
			deps.Reason("previous_tick_in_progress"))

		return nil
	}
	defer workerCtx.tickInProgress.Store(false)

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "tick_start"))

	workerCtx.mu.RLock()

	currentStateStr := "nil"
	if workerCtx.currentState != nil {
		currentStateStr = workerCtx.currentState.String()
	}

	s.logTrace("tick_worker",
		deps.String("current_state", currentStateStr))
	workerCtx.mu.RUnlock()

	s.logTrace("loading_snapshot",
		deps.String("stage", "data_freshness"))

	storageSnapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "snapshot_load_failed")

		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// I16: Validate ObservedState is not nil from storage
	if storageSnapshot.Observed == nil {
		panic("Invariant I16 violated: storage returned nil ObservedState for worker " + workerID)
	}

	var observed TObserved

	err = s.store.LoadObservedTyped(ctx, s.workerType, workerID, &observed)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "observed_state_load_failed")

		return fmt.Errorf("failed to load typed observed state: %w", err)
	}

	var desired TDesired

	err = s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "desired_state_load_failed")

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

		// Cache for IsObservationStale() - called by parent supervisor when building ChildInfo
		workerCtx.mu.Lock()
		workerCtx.lastObservationCollectedAt = observationTimestamp
		workerCtx.mu.Unlock()

		s.logTrace("observation_timestamp_loaded",
			deps.String("stage", "data_freshness"),
			deps.String("timestamp", observationTimestamp.Format(time.RFC3339Nano)))
	} else {
		s.logTrace("observation_no_timestamp",
			deps.String("stage", "data_freshness"),
			deps.String("type", fmt.Sprintf("%T", observed)))
	}

	// Cache observed state name for GetObservedStateName() - used by parent for lifecycle-based health checks
	// State implements LifecyclePhase(), construct name from phase + state.String()
	workerCtx.mu.RLock()
	currentStateForPhase := workerCtx.currentState
	workerCtx.mu.RUnlock()

	if currentStateForPhase != nil {
		phase := currentStateForPhase.LifecyclePhase()

		workerCtx.mu.Lock()
		workerCtx.lastObservedStateName = buildObservedStateName(phase, currentStateForPhase)
		workerCtx.lastLifecyclePhase = phase
		workerCtx.mu.Unlock()
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
				maxAttemptsErr := fmt.Errorf("collector unresponsive after %d restart attempts", s.collectorHealth.maxRestartAttempts)
				s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), maxAttemptsErr, "collector_unresponsive_max_attempts",
					deps.Attempts(s.collectorHealth.maxRestartAttempts))

				if shutdownErr := s.requestShutdown(ctx, workerID, maxAttemptsErr.Error()); shutdownErr != nil {
					s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, shutdownErr, "shutdown_request_failed")
				}

				return errors.New("collector unresponsive, shutdown requested")
			}

			// I4: Safe to restart (restartCount < maxRestartAttempts)
			// RestartCollector will panic if invariant violated (defensive check)
			if err := s.restartCollector(ctx, workerID); err != nil {
				s.mu.RLock()
				restartAttempt := s.collectorHealth.restartCount
				maxAttempts := s.collectorHealth.maxRestartAttempts
				s.mu.RUnlock()

				s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "collector_restart_failed",
					deps.Int("restart_attempt", restartAttempt),
					deps.Int("max_attempts", maxAttempts))

				return fmt.Errorf("failed to restart collector: %w", err)
			}
		}

		s.logger.Debug("Pausing FSM due to stale/timeout data")

		return nil
	}

	// Check and reset restartCount under lock to prevent data races
	s.mu.Lock()

	restartCount := s.collectorHealth.restartCount
	if restartCount > 0 {
		s.collectorHealth.restartCount = 0
	}

	s.mu.Unlock()

	if restartCount > 0 {
		s.logger.Info("collector_recovered",
			deps.Attempts(restartCount))
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
			blockedCount := workerCtx.gatingBlockedCount
			workerCtx.actionPending = false
			workerCtx.lastActionObsTime = time.Time{}
			workerCtx.gatingExplainedOnce = false
			workerCtx.gatingBlockedCount = 0
			workerCtx.mu.Unlock()

			s.logger.Debug("action_gating_cleared",
				deps.Int64("waited_ticks", blockedCount),
				deps.String("last_action_time", lastActionObsTime.Format(time.RFC3339Nano)),
				deps.String("new_observation_time", currentObsTime.Format(time.RFC3339Nano)))
		} else {
			workerCtx.mu.Lock()
			explained := workerCtx.gatingExplainedOnce
			workerCtx.gatingBlockedCount++

			blockedCount := workerCtx.gatingBlockedCount
			if !explained {
				workerCtx.gatingExplainedOnce = true
			}

			workerCtx.mu.Unlock()

			if !explained {
				s.logger.Debug("fsm_progression_paused",
					deps.Reason("waiting_for_fresh_observation"),
					deps.String("explanation", "FSM waits for fresh observation after action to prevent duplicate execution. "+
						"This log appears once per gating cycle."))
			}

			s.logTrace("action_gating_blocked",
				deps.Int64("blocked_count", blockedCount),
				deps.String("last_action_time", lastActionObsTime.Format(time.RFC3339Nano)),
				deps.String("current_observation_time", currentObsTime.Format(time.RFC3339Nano)))

			return nil
		}
	}

	workerCtx.mu.RLock()
	currentState := workerCtx.currentState
	workerCtx.mu.RUnlock()

	if currentState == nil {
		if _, alreadyLogged := s.noStateMachineLoggedOnce.LoadOrStore(workerID, true); !alreadyLogged {
			s.logger.Info("worker_has_no_state_machine",
				deps.String("info", "This worker manages child workers without its own FSM"))
		}

		return nil
	}

	result := currentState.Next(*snapshot)

	hasAction := result.Action != nil
	// Per-tick log moved to TRACE for scalability
	s.logTrace("state_evaluation",
		deps.String("next_state", result.State.String()),
		deps.Int("signal", int(result.Signal)),
		deps.Bool("has_action", hasAction))

	// FSM invariant: state transition and action emission are mutually exclusive
	if result.State != currentState && result.Action != nil {
		panic(fmt.Sprintf("invalid state transition: state %s tried to switch to %s AND emit action %s",
			currentState.String(), result.State.String(), result.Action.Name()))
	}

	if result.Action != nil {
		actionID := result.Action.Name()

		if workerCtx.executor.HasActionInProgress(actionID) {
			s.logTrace("action_skipped",
				deps.String("action_id", actionID),
				deps.Reason("already_in_progress"))

			return nil
		}

		// Get dependencies from worker if it implements DependencyProvider
		var workerDeps any
		if provider, ok := workerCtx.worker.(fsmv2.DependencyProvider); ok {
			workerDeps = provider.GetDependenciesAny()
		}

		// Action enqueued at DEBUG - low signal for operators
		s.logger.Debug("action_enqueued",
			deps.String("action_id", actionID))

		if err := workerCtx.executor.EnqueueAction(actionID, result.Action, workerDeps); err != nil {
			s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "action_enqueue_failed",
				deps.String("action_id", actionID))

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

		s.logger.Debug("action_gating_pending",
			deps.String("action_id", actionID),
			deps.String("observation_time", currentObsTime.Format(time.RFC3339Nano)))
	}

	if result.State != currentState {
		fromState := currentState.String()
		toState := result.State.String()
		now := time.Now()

		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "state_transition"),
			deps.String("from_state", fromState),
			deps.String("to_state", toState),
			deps.Reason(result.Reason))

		s.logger.Info("state_transition",
			deps.String("from_state", fromState),
			deps.String("to_state", toState),
			deps.Reason(result.Reason))

		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "mutex_lock_acquire"),
			deps.String("mutex_name", "workerCtx.mu"),
			deps.String("lock_type", "write"))

		workerCtx.mu.Lock()

		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "mutex_lock_acquired"),
			deps.String("mutex_name", "workerCtx.mu"))

		if !workerCtx.stateEnteredAt.IsZero() {
			timeInState := now.Sub(workerCtx.stateEnteredAt)
			workerCtx.stateDurations[fromState] += timeInState
		}

		workerCtx.stateTransitions[toState]++
		workerCtx.totalTransitions++
		workerCtx.currentState = result.State

		// Exposed via FrameworkMetrics and GetCurrentStateNameAndReason
		workerCtx.currentStateReason = result.Reason
		workerCtx.stateEnteredAt = now

		// Update cached lifecycle phase and observed state name AFTER state transition.
		// CRITICAL: This must happen AFTER currentState is updated, not before.
		// Parent supervisors call GetLifecyclePhase() which returns lastLifecyclePhase.
		// If we update this before the transition, parent sees stale health status.
		newPhase := result.State.LifecyclePhase()
		workerCtx.lastObservedStateName = buildObservedStateName(newPhase, result.State)
		workerCtx.lastLifecyclePhase = newPhase

		workerCtx.mu.Unlock()

		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "mutex_unlock"),
			deps.String("mutex_name", "workerCtx.mu"))

		// Record Prometheus metric AFTER lock release
		metrics.RecordStateTransition(s.GetHierarchyPathUnlocked(), fromState, toState)
	} else {
		s.logTrace("state_unchanged",
			deps.String("state", currentState.String()))
	}

	workerCtx.mu.RLock()

	if workerCtx.currentState != nil && !workerCtx.stateEnteredAt.IsZero() {
		metrics.RecordStateDuration(
			s.GetHierarchyPathUnlocked(),
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

	if err := s.processSignal(ctx, workerID, result.Signal); err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "signal_processing_failed",
			deps.Int("signal", int(result.Signal)))

		return fmt.Errorf("signal processing failed: %w", err)
	}

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "tick_complete"),
		deps.String("final_state", finalState))

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
//   - Timeouts enforced by ActionExecutor; retries via tick-based re-evaluation
//
// PERFORMANCE: The complete tick loop is non-blocking and completes in <10ms,
// making it safe to call at high frequency (100Hz+) without impacting system performance.
func (s *Supervisor[TObserved, TDesired]) tick(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					err = fmt.Errorf("tick panic (recovery handler also panicked: %v): %v", r2, r)

					s.panicCircuitOpen.Store(true)
					func() {
						defer func() { _ = recover() }()

						s.logger.SentryError(deps.FeatureFSMv2, "unknown", err, "tick_double_panic",
							deps.String("stack", string(debug.Stack())))
					}()
				}
			}()

			panicType, panicErr := panicutil.ClassifyPanic(r)
			err = fmt.Errorf("tick panic: %w", panicErr)

			hierarchyPath := s.GetHierarchyPathUnlocked()
			metrics.RecordPanicRecovery(hierarchyPath, panicType)

			s.logger.SentryError(deps.FeatureFSMv2, hierarchyPath, err, "tick_panic",
				deps.WorkerType(s.workerType),
				deps.Field{Key: "panic_type", Value: panicType},
				deps.Field{Key: "stack_trace", Value: string(debug.Stack())})

			if s.panicTracker.RecordPanic() {
				s.panicCircuitOpen.Store(true)
				panicCount := s.panicTracker.PanicCount()
				s.logger.SentryWarn(deps.FeatureFSMv2, hierarchyPath, "panic_circuit_open",
					deps.WorkerType(s.workerType),
					deps.Field{Key: "panic_count", Value: panicCount})
			}
		}
	}()

	// tick() is called from a single goroutine (tickLoop or parent's tick). No concurrent ticks occur.
	if s.panicCircuitOpen.Load() {
		if s.panicTracker.PanicCount() == 0 {
			s.panicCircuitOpen.Store(false)
			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "panic_circuit_auto_reset",
				deps.WorkerType(s.workerType))
		} else {
			s.logger.Debug("tick_suppressed_panic_circuit_open",
				deps.HierarchyPath(s.GetHierarchyPathUnlocked()))

			return ErrPanicCircuitOpen
		}
	}

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

		// Extract child error info early for logging context.
		var childErr *ChildHealthError
		errors.As(err, &childErr)

		if !wasOpen {
			// Build fields with failed_child info if available.
			logFields := []deps.Field{
				deps.String("error_scope", "infrastructure"),
				deps.String("impact", "all_workers"),
			}
			if childErr != nil {
				logFields = append(logFields, deps.String("failed_child", childErr.ChildName))
			}

			s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "circuit_breaker_opened",
				logFields...)
			metrics.RecordCircuitOpen(s.GetHierarchyPathUnlocked(), true)
		}

		if childErr != nil {
			attempts := s.healthChecker.backoff.GetAttempts()
			nextDelay := s.healthChecker.backoff.NextDelay()

			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "circuit_breaker_retry_scheduled",
				deps.String("failed_child", childErr.ChildName),
				deps.Attempts(attempts),
				deps.Int("max_attempts", s.healthChecker.maxAttempts),
				deps.String("elapsed_downtime", s.healthChecker.backoff.GetTotalDowntime().String()),
				deps.String("next_retry_in", nextDelay.String()),
				deps.String("recovery_status", s.getRecoveryStatus()))

			if attempts == 4 {
				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "escalation_warning_one_retry_remaining",
					deps.String("child_name", childErr.ChildName),
					deps.Int("attempts_remaining", 1),
					deps.String("total_downtime", s.healthChecker.backoff.GetTotalDowntime().String()))
			}

			if attempts >= 5 {
				s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), childErr, "escalation_required",
					deps.String("child_name", childErr.ChildName),
					deps.Int("max_attempts", 5),
					deps.String("total_downtime", s.healthChecker.backoff.GetTotalDowntime().String()),
					deps.String("runbook_url", "https://docs.umh.app/runbooks/supervisor-escalation"),
					deps.String("manual_steps", s.getEscalationSteps(childErr.ChildName)))
			}
		}

		return ErrInfraCircuitOpen
	}

	if s.circuitOpen.Load() {
		downtime := time.Since(s.healthChecker.backoff.GetStartTime())
		s.logger.Info("circuit_breaker_closed",
			deps.Reason("infrastructure_recovered"),
			deps.String("total_downtime", downtime.String()))
		metrics.RecordCircuitOpen(s.GetHierarchyPathUnlocked(), false)
		metrics.RecordInfrastructureRecovery(s.GetHierarchyPathUnlocked(), downtime)
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
		if !s.started.Load() {
			return nil
		}

		// Zero workers on a started supervisor is a transient state (e.g., during child
		// restart). The supervisor self-heals on the next reconcileChildren cycle.
		// Log at Info, not Warn/Error, to avoid Sentry noise.
		if s.noWorkersWarnedOnce.CompareAndSwap(false, true) {
			s.logger.Info("tick_skipped_no_workers",
				deps.HierarchyPath(s.GetHierarchyPathUnlocked()))
		}

		return nil
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

	// Deep copy globalVars to prevent race with SetGlobalVariables().
	// The map reference could be replaced while we use it for template expansion.
	s.mu.RLock()

	globalVarsCopy := make(map[string]any, len(s.globalVars))
	for k, v := range s.globalVars {
		globalVarsCopy[k] = v
	}

	globalVarCount := len(globalVarsCopy)

	s.mu.RUnlock()

	userSpecWithVars.Variables.Global = globalVarsCopy

	userSpecWithVars.Variables.Internal = map[string]any{
		FieldID:                firstWorkerID,
		storage.FieldCreatedAt: s.createdAt,
		FieldParentID:          s.parentID,
	}

	userVarCount := len(userSpecWithVars.Variables.User)
	// Per-tick log moved to TRACE for scalability
	s.logTrace("variables_propagated",
		deps.Int("user_vars", userVarCount),
		deps.Int("global_vars", globalVarCount))
	metrics.RecordVariablePropagation(s.GetHierarchyPathUnlocked())

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
			deps.String("hash", currentHash[:8]+"..."))
	} else {
		// Cache miss - call DeriveDesiredState
		templateStart := time.Now()

		var err error

		desired, err = worker.DeriveDesiredState(userSpecWithVars)
		templateDuration := time.Since(templateStart)

		if err != nil {
			s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "template_rendering_failed",
				deps.DurationMs(templateDuration.Milliseconds()))
			metrics.RecordTemplateRenderingDuration(s.GetHierarchyPathUnlocked(), "error", templateDuration)
			metrics.RecordTemplateRenderingError(s.GetHierarchyPathUnlocked(), "derivation_failed")

			// Don't cache errors - will retry next tick
			return fmt.Errorf("failed to derive desired state: %w", err)
		}

		// Validate DesiredState.State is a valid lifecycle state ("stopped" or "running")
		// This catches both developer mistakes (hardcoded wrong values) and user config mistakes
		// Use GetState() method from fsmv2.DesiredState interface
		if valErr := config.ValidateDesiredState(desired.GetState()); valErr != nil {
			s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), valErr, "invalid_desired_state",
				deps.String("state", desired.GetState()),
				deps.WorkerID(firstWorkerID))
			metrics.RecordTemplateRenderingDuration(s.GetHierarchyPathUnlocked(), "error", templateDuration)
			metrics.RecordTemplateRenderingError(s.GetHierarchyPathUnlocked(), "invalid_state_value")

			// Don't cache validation errors - will retry next tick
			return fmt.Errorf("failed to derive desired state: %w", valErr)
		}

		// Update cache
		s.mu.Lock()
		s.lastUserSpecHash = currentHash
		s.cachedDesiredState = desired
		s.mu.Unlock()

		s.logTrace("derive_desired_state_computed",
			deps.String("hash", currentHash[:8]+"..."),
			deps.DurationMs(templateDuration.Milliseconds()))
		metrics.RecordTemplateRenderingDuration(s.GetHierarchyPathUnlocked(), "success", templateDuration)
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
		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "derived_desired_state_save_failed",
			deps.Err(err))
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
			hash, err := spec.Hash()
			if err != nil {
				// If hashing fails, always validate to be safe
				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "spec_hash_failed",
					deps.Err(err),
					deps.String("spec", spec.Name))
				specsToValidate = append(specsToValidate, spec)

				continue
			}

			if cachedHash, exists := s.validatedSpecHashes[spec.Name]; !exists || cachedHash != hash {
				specsToValidate = append(specsToValidate, spec)
			}
		}

		if len(specsToValidate) > 0 {
			if err := config.ValidateChildSpecs(specsToValidate, registry); err != nil {
				s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "child_spec_validation_failed")

				return fmt.Errorf("invalid child specifications: %w", err)
			}

			// Update cache for validated specs
			for _, spec := range specsToValidate {
				hash, err := spec.Hash()
				if err != nil {
					// Skip caching if hash fails - will revalidate next time
					s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "spec_hash_cache_failed",
						deps.Err(err),
						deps.String("spec", spec.Name))

					continue
				}

				s.validatedSpecHashes[spec.Name] = hash
			}

			s.logTrace("child_specs_validated",
				deps.Int("validated_count", len(specsToValidate)),
				deps.Int("total_count", len(childrenSpecs)))
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
			s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "child_tick_failed")
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
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// processSignal handles signals from states.
func (s *Supervisor[TObserved, TDesired]) processSignal(ctx context.Context, workerID string, signal fsmv2.Signal) error {
	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "signal_processing"),
		deps.Int("signal", int(signal)))

	switch signal {
	case fsmv2.SignalNone:
		// Normal operation
		return nil
	case fsmv2.SignalNeedsRemoval:
		s.logger.Debug("worker_removal_signaled")

		// Check if this worker should be restarted instead of removed
		s.mu.Lock()

		shouldRestart := s.pendingRestart[workerID]
		if shouldRestart {
			delete(s.pendingRestart, workerID)
			delete(s.restartRequestedAt, workerID)
		}

		s.mu.Unlock()

		if shouldRestart {
			s.logger.Info("worker_restarting",
				deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
				deps.String("target_worker_id", workerID),
				deps.Reason("restart requested after graceful shutdown"))

			return s.handleWorkerRestart(ctx, workerID)
		}

		// Original removal flow continues below...
		s.mu.Lock()

		workerCtx, exists := s.workers[workerID]
		if !exists {
			s.mu.Unlock()

			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "worker_removal_not_found",
				deps.String("target_worker_id", workerID))

			return errors.New("worker not found in registry")
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

			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "worker_removal_has_children",
				deps.Int("child_count", childCount),
				deps.Any("children", childNames))
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

		for name, child := range childrenToCleanup {
			s.logger.Debug("child_supervisor_shutdown",
				deps.String("child_name", name),
				deps.String("context", "post_graceful_cleanup"))
			child.Shutdown()

			// Wait for child supervisor to fully stop (with timeout)
			if done, exists := doneChannels[name]; exists {
				s.waitForChildDone(done, name, s.GetHierarchyPath(), "worker_removal_cleanup")
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

			if err := workerCtx.collector.CollectFinalObservation(collectCtx); err != nil {
				s.logger.Debug("final_observation_failed",
					deps.Err(err),
					deps.HierarchyPath(workerCtx.identity.HierarchyPath))
			}

			cancel()
		}

		workerCtx.collector.Stop(ctx)
		workerCtx.executor.Shutdown()

		s.logger.Debug("worker_removed_successfully",
			deps.Int("children_cleaned", childCount))

		return nil
	case fsmv2.SignalNeedsRestart:
		s.logger.Info("worker_restart_requested",
			deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
			deps.String("target_worker_id", workerID),
			deps.Reason("worker signaled unrecoverable error"))

		// Mark for restart (will be checked when SignalNeedsRemoval is received)
		s.mu.Lock()
		s.pendingRestart[workerID] = true
		s.restartRequestedAt[workerID] = time.Now()
		s.mu.Unlock()

		// Request graceful shutdown - worker will go through cleanup states
		if err := s.requestShutdown(ctx, workerID, "restart_requested"); err != nil {
			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "restart_shutdown_request_failed",
				deps.Err(err),
				deps.String("target_worker_id", workerID))
			// Continue anyway - we want to restart even if request fails
		}

		return nil
	default:
		unknownSignalErr := fmt.Errorf("unknown signal: %d", signal)
		s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), unknownSignalErr, "unknown_signal_received",
			deps.String("target_worker_id", workerID),
			deps.Int("signal", int(signal)))

		return unknownSignalErr
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
			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "restart_graceful_timeout",
				deps.String("target_worker_id", workerID),
				deps.Duration("timeout", DefaultGracefulRestartTimeout),
				deps.Duration("waited", time.Since(requestedAt)))

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

			s.logger.Info("worker_restart_force_reset",
				deps.HierarchyPath(workerCtx.identity.HierarchyPath),
				deps.String("from_state", fromState))

			workerCtx.currentState = workerCtx.worker.GetInitialState()

			toState := "nil"
			if workerCtx.currentState != nil {
				toState = workerCtx.currentState.String()
			}

			workerCtx.mu.Unlock()

			if workerCtx.collector != nil {
				workerCtx.collector.TriggerNow()
			}

			// Clear pending restart
			delete(s.pendingRestart, workerID)
			delete(s.restartRequestedAt, workerID)

			s.logger.Info("worker_restart_force_complete",
				deps.HierarchyPath(workerCtx.identity.HierarchyPath),
				deps.String("to_state", toState))
		}
	}
}

// buildObservedStateName constructs the observed state name from a lifecycle phase and state string.
// Format: phase.Prefix() + lowercase(state.String()), except PhaseStopped which has no suffix.
func buildObservedStateName(phase config.LifecyclePhase, state fsmv2.State[any, any]) string {
	prefix := phase.Prefix()
	if phase == config.PhaseStopped {
		return prefix
	}

	return prefix + strings.ToLower(state.String())
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
		// Capture values before unlocking to avoid data race in panic message
		restartCount := s.collectorHealth.restartCount
		maxRestartAttempts := s.collectorHealth.maxRestartAttempts
		s.mu.Unlock()
		panic(fmt.Sprintf("supervisor bug: RestartCollector called with restartCount=%d >= maxRestartAttempts=%d (should have escalated to shutdown)",
			restartCount, maxRestartAttempts))
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
			s.logger.Debug("collector_restart_backoff",
				deps.Int("restart_attempt", restartCount),
				deps.Duration("backoff_remaining", backoff-elapsed))

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

	// Tiered escalation: use SentryError when 50%+ of max attempts used
	if restartCount >= maxRestartAttempts/2 {
		escalationRisk := "high"
		if restartCount == maxRestartAttempts-1 {
			escalationRisk = "imminent"
		}

		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "collector_restarting",
			deps.Err(fmt.Errorf("collector restart attempt %d of %d", restartCount, maxRestartAttempts)),
			deps.Int("restart_attempt", restartCount),
			deps.Int("max_attempts", maxRestartAttempts),
			deps.Duration("backoff", backoff),
			deps.String("escalation_risk", escalationRisk))
	} else {
		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "collector_restarting",
			deps.Int("restart_attempt", restartCount),
			deps.Int("max_attempts", maxRestartAttempts),
			deps.Duration("backoff", backoff))
	}

	s.mu.RLock()
	workerCtx, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		notFoundErr := errors.New("worker not found")
		s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), notFoundErr, "collector_restart_worker_not_found",
			deps.String("target_worker_id", workerID))

		return notFoundErr
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
		s.logger.SentryWarn(deps.FeatureFSMv2, snapshot.Identity.HierarchyPath, "snapshot_missing_timestamp",
			deps.Reason("Snapshot.Observed does not implement GetTimestamp()"),
			deps.String("impact", "cannot check freshness"))

		return true
	}

	age = time.Since(collectedAt)

	// During shutdown, log at DEBUG instead of WARN to avoid noisy logs
	// when collectors are already stopped and data is expected to be stale.
	isShuttingDown := !s.started.Load()

	if s.freshnessChecker.IsTimeout(snapshot) {
		s.logFreshnessWarning(isShuttingDown, snapshot.Identity.HierarchyPath, "data_timeout", age, s.collectorHealth.timeout)

		return false
	}

	if !s.freshnessChecker.Check(snapshot) {
		s.logFreshnessWarning(isShuttingDown, snapshot.Identity.HierarchyPath, "data_stale", age, s.collectorHealth.staleThreshold)

		return false
	}

	return true
}

// logFreshnessWarning logs a data freshness issue at DEBUG during shutdown (expected) or SentryWarn otherwise.
func (s *Supervisor[TObserved, TDesired]) logFreshnessWarning(isShuttingDown bool, hierarchyPath, msg string, age, threshold time.Duration) {
	if isShuttingDown {
		s.logger.Debug(msg+"_during_shutdown",
			deps.Duration("age", age),
			deps.Duration("threshold", threshold))
	} else {
		s.logger.SentryWarn(deps.FeatureFSMv2, hierarchyPath, msg,
			deps.Duration("age", age),
			deps.Duration("threshold", threshold))
	}
}

func (s *Supervisor[TObserved, TDesired]) logHeartbeat() {
	s.mu.RLock()
	workerCount := len(s.workers)
	childCount := len(s.children)

	workerStates := make(map[string]string, workerCount)
	workerReasons := make(map[string]string, workerCount)

	for id, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		if workerCtx.currentState != nil {
			workerStates[id] = workerCtx.currentState.String()
		} else {
			workerStates[id] = "no_state_machine"
		}

		if workerCtx.currentStateReason != "" {
			workerReasons[id] = workerCtx.currentStateReason
		}

		workerCtx.mu.RUnlock()
	}

	s.mu.RUnlock()

	s.logger.Info("supervisor_heartbeat",
		deps.HierarchyPath(s.GetHierarchyPathUnlocked()),
		deps.Any("tick", atomic.LoadUint64(&s.tickCount)),
		deps.Int("workers", workerCount),
		deps.Int("children", childCount),
		deps.Any("worker_states", workerStates),
		deps.Any("worker_reasons", workerReasons),
		deps.Int("active_actions", s.actionExecutor.GetActiveActionCount()))
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
				deps.String("lifecycle_event", "child_update"),
				deps.String("child_name", spec.Name),
				deps.String("parent_worker_type", s.workerType))

			// Clear pendingRemoval if child was marked for removal but re-appeared in specs
			// This handles the case where a child disappears temporarily (e.g., during restart)
			// and then re-appears in the desired spec before shutdown completes
			if s.pendingRemoval[spec.Name] {
				s.logger.Debug("child_reappeared_clearing_pending_removal",
					deps.String("child_name", spec.Name))
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
				deps.String("lifecycle_event", "child_add_start"),
				deps.String("child_name", spec.Name),
				deps.String("child_worker_type", spec.WorkerType),
				deps.String("parent_worker_type", s.workerType))

			s.logger.Info("child_adding",
				deps.String("child_name", spec.Name),
				deps.String("child_worker_type", spec.WorkerType))

			addedCount++

			// Merge parent dependencies with child's (child overrides parent)
			mergedDeps := config.MergeDependencies(s.deps, spec.Dependencies)

			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "dependencies_merged"),
				deps.String("child_name", spec.Name),
				deps.Int("parent_dep_count", len(s.deps)),
				deps.Int("child_dep_count", len(spec.Dependencies)),
				deps.Int("merged_dep_count", len(mergedDeps)))

			childConfig := Config{
				WorkerType:              spec.WorkerType,
				Store:                   s.store,
				Logger:                  s.baseLogger, // Important: Use un-enriched logger to prevent duplicate "worker" fields
				TickInterval:            s.tickInterval,
				GracefulShutdownTimeout: s.gracefulShutdownTimeout,
				ChildShutdownTimeout:    s.childShutdownTimeout,
				EnableTraceLogging:      s.enableTraceLogging,
				Dependencies:            mergedDeps,
			}

			// Use factory to create child supervisor with proper type
			rawSupervisor, err := factory.NewSupervisorByType(spec.WorkerType, childConfig)
			if err != nil {
				s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "child_supervisor_creation_failed",
					deps.String("child_name", spec.Name))

				continue
			}

			childSupervisor, ok := rawSupervisor.(SupervisorInterface)
			if !ok {
				typeErr := fmt.Errorf("factory returned %T (expected SupervisorInterface)", rawSupervisor)
				s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), typeErr, "factory_invalid_supervisor_type",
					deps.String("child_name", spec.Name))

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
				s.logger.SentryError(deps.FeatureFSMv2, childPath, err, "child_worker_creation_failed",
					deps.String("child_name", spec.Name),
					deps.WorkerType(spec.WorkerType))

				continue
			}

			// Add worker to child supervisor
			if err := childSupervisor.AddWorker(childIdentity, childWorker); err != nil {
				s.logger.SentryError(deps.FeatureFSMv2, childPath, err, "child_supervisor_add_worker_failed",
					deps.String("child_name", spec.Name))

				continue
			}

			// Save initial desired state for child (empty document to avoid nil on first tick)
			childDesiredCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			desiredDoc := persistence.Document{
				FieldID:                childIdentity.ID,
				FieldShutdownRequested: false,
			}
			if _, err := s.store.SaveDesired(childDesiredCtx, spec.WorkerType, childIdentity.ID, desiredDoc); err != nil {
				s.logger.SentryWarn(deps.FeatureFSMv2, childPath, "child_initial_desired_state_save_failed",
					deps.Err(err),
					deps.String("child_name", spec.Name))
			}

			cancel()

			s.children[spec.Name] = childSupervisor

			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_add_complete"),
				deps.String("child_name", spec.Name),
				deps.String("parent_worker_type", s.workerType))

			// Start child supervisor if parent is already started.
			// Use StartAsChild() instead of Start() - children don't have their own tick loop.
			// The parent ticks children synchronously during its own tick().
			if childCtx, started := s.getStartedContext(); started {
				if childCtx.Err() == nil {
					childSupervisor.StartAsChild(childCtx)
				} else {
					s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "child_start_skipped_context_cancelled",
						deps.String("child_name", spec.Name))
				}
			}
		}
	}

	// Phase 1: Request shutdown for children not in specs (mark as pendingRemoval)
	for name := range s.children {
		if !specNames[name] && !s.pendingRemoval[name] {
			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_shutdown_requested"),
				deps.String("child_name", name),
				deps.String("parent_worker_type", s.workerType))

			s.logger.Debug("child_shutdown_requesting",
				deps.String("child_name", name))

			// Mark child as pending removal
			s.pendingRemoval[name] = true

			child := s.children[name]
			if child != nil {
				// Request shutdown - sets ShutdownRequested=true on child's workers
				// Child continues ticking and will emit SignalNeedsRemoval when ready
				ctx := context.Background()
				if err := child.RequestShutdown(ctx, "removed_from_specs"); err != nil {
					s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), "child_shutdown_request_failed",
						deps.Err(err),
						deps.String("child_name", name))
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
				deps.String("lifecycle_event", "child_remove_start"),
				deps.String("child_name", name),
				deps.String("parent_worker_type", s.workerType))

			s.logger.Debug("child_shutdown_complete",
				deps.String("child_name", name))

			// Now safe to fully shut down and remove
			child.Shutdown()

			if done, exists := s.childDoneChans[name]; exists {
				s.waitForChildDone(done, name, s.GetHierarchyPathUnlocked(), "reconcile_children")
				delete(s.childDoneChans, name)
			}

			delete(s.children, name)
			delete(s.pendingRemoval, name)

			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_remove_complete"),
				deps.String("child_name", name),
				deps.String("parent_worker_type", s.workerType))

			removedCount++
		}
	}

	duration := time.Since(startTime)
	// Log at DEBUG for all reconciliation completions
	if addedCount > 0 || removedCount > 0 {
		// Topology changed
		s.logger.Debug("child_reconciliation_completed",
			deps.Int("added", addedCount),
			deps.Int("updated", updatedCount),
			deps.Int("removed", removedCount),
			deps.DurationMs(duration.Milliseconds()))
	} else if updatedCount > 0 {
		// Updates only - DEBUG level (per-tick noise)
		s.logger.Debug("child_reconciliation_completed",
			deps.Int("updated", updatedCount),
			deps.DurationMs(duration.Milliseconds()))
	}

	metrics.RecordReconciliation(s.GetHierarchyPathUnlocked(), "success", duration)

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
			deps.String("child_name", childName),
			deps.String("parent_state", parentState),
			deps.String("mapped_state", mappedState))
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
