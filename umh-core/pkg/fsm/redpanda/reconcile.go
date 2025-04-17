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

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

// Reconcile examines the RedpandaInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (r *RedpandaInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	redpandaInstanceName := r.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, redpandaInstanceName, time.Since(start))
		if err != nil {
			r.baseFSMInstance.GetLogger().Errorf("error reconciling Redpanda instance %s: %w", redpandaInstanceName, err)
			r.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentRedpandaInstance, redpandaInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if r.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := r.baseFSMInstance.GetBackoffError(snapshot.Tick)
		r.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Redpanda pipeline %s: %w", redpandaInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {

			// if it is already in stopped, stopping, removing states, and it again returns a permanent error,
			// we need to throw it to the manager as the instance itself here cannot fix it anymore
			if r.IsRemoved() || r.IsRemoving() || r.IsStopping() || r.IsStopped() {
				r.baseFSMInstance.GetLogger().Errorf("Redpanda instance %s is already in a terminal state, force removing it", redpandaInstanceName)
				// force delete everything from the s6 file directory
				r.service.ForceRemoveRedpanda(ctx, filesystemService)
				return err, false
			} else {
				r.baseFSMInstance.GetLogger().Errorf("Redpanda instance %s is not in a terminal state, resetting state and removing it", redpandaInstanceName)
				r.baseFSMInstance.ResetState()
				r.Remove(ctx)
				return nil, false // let's try to at least reconcile towards a stopped / removed state
			}
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err := r.reconcileExternalChanges(ctx, filesystemService, snapshot.Tick, start); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, redpanda_service.ErrServiceNotExist) {
			r.baseFSMInstance.SetError(err, snapshot.Tick)
			r.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)

			if errors.Is(err, context.DeadlineExceeded) {
				// Healthchecks occasionally take longer (sometimes up to 70ms),
				// resulting in context.DeadlineExceeded errors. In this case, we want to
				// mark the reconciliation as complete for this tick since we've likely
				// already consumed significant time. We return reconciled=true to prevent
				// further reconciliation attempts in the current tick.
				return nil, true // We don't want to return an error here, as this can happen in normal operations
			}
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		//nolint:ineffassign // This is intentionally modifying the named return value accessed in defer
		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = r.reconcileStateTransition(ctx, filesystemService, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, fsm.ErrInstanceRemoved) {
			return nil, false
		}

		r.baseFSMInstance.SetError(err, snapshot.Tick)
		r.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the s6Manager
	s6Err, s6Reconciled := r.service.ReconcileManager(ctx, filesystemService, snapshot.Tick)
	if s6Err != nil {
		r.baseFSMInstance.SetError(s6Err, snapshot.Tick)
		r.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
		return nil, false
	}

	// If either Redpanda state or S6 state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || s6Reconciled

	// It went all right, so clear the error
	r.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the RedpandaInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (r *RedpandaInstance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.RedpandaUpdateObservedStateTimeout)
	defer cancel()

	err := r.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick, loopStartTime)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ObservedState.
// This is to ensure full testability of the FSM.
func (r *RedpandaInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := r.baseFSMInstance.GetCurrentFSMState()
	desiredState := r.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := r.reconcileLifecycleStates(ctx, filesystemService, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// If both current and desired state are stopped, we don't need to do anything
	if currentState == OperationalStateStopped && desiredState == OperationalStateStopped {
		return nil, false
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := r.reconcileOperationalStates(ctx, filesystemService, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (creating/removing)
func (r *RedpandaInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		if err := r.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return r.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Check if the service is created
		// For now, we'll assume it's created immediately after initiating creation
		return r.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		if err := r.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return r.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (r *RedpandaInstance) reconcileOperationalStates(ctx context.Context, filesystemService filesystem.Service, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return r.reconcileTransitionToActive(ctx, filesystemService, currentState, currentTime)
	case OperationalStateStopped:
		return r.reconcileTransitionToStopped(ctx, filesystemService, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (r *RedpandaInstance) reconcileTransitionToActive(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	// If we're stopped, we need to start first
	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := r.StartInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return r.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	// Handle starting phase states
	if IsStartingState(currentState) {
		return r.reconcileStartingStates(ctx, filesystemService, currentState, currentTime)
	} else if IsRunningState(currentState) {
		return r.reconcileRunningStates(ctx, filesystemService, currentState, currentTime)
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (r *RedpandaInstance) reconcileStartingStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// First we need to ensure the S6 service is started
		if !r.IsRedpandaS6Running() {
			return nil, false
		}

		// Check if "Successfully started Redpanda!" is found in logs
		if r.IsRedpandaStarted() {
			return r.baseFSMInstance.SendEvent(ctx, EventStartDone), true
		}

		return nil, false
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (r *RedpandaInstance) reconcileRunningStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow) {
			return r.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if !r.IsRedpandaWithProcessingActivity() { // if there is no activity, we move to Idle
			return r.baseFSMInstance.SendEvent(ctx, EventNoDataTimeout), true
		}
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		if r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow) {
			return r.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if r.IsRedpandaWithProcessingActivity() { // if there is activity, we move to Active
			return r.baseFSMInstance.SendEvent(ctx, EventDataReceived), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		if !r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow) {
			return r.baseFSMInstance.SendEvent(ctx, EventRecovered), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (r *RedpandaInstance) reconcileTransitionToStopped(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	// If we're in any operational state except Stopped or Stopping, initiate stop
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		// Attempt to initiate a stop
		if err := r.StopInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return r.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	// If already stopping, verify if the instance is completely stopped
	if currentState == OperationalStateStopping && r.IsRedpandaS6Stopped() {
		// Transition from Stopping to Stopped
		return r.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	}

	return nil, false
}
