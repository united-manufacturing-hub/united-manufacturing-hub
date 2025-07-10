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
	"strings"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	redpanda_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the RedpandaInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (r *RedpandaInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
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

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return r.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return r.IsRemoved() || r.IsRemoving() || r.IsStopping() || r.IsStopped() || r.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return r.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return r.service.ForceRemoveRedpanda(ctx, services.GetFileSystem(), r.baseFSMInstance.GetID())
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	var externalReconciled bool
	err, externalReconciled = r.reconcileExternalChanges(ctx, services, snapshot)
	if err != nil {
		// I am using strings.Contains as i cannot get it working with errors.Is
		isExpectedError := strings.Contains(err.Error(), redpanda_monitor_service.ErrServiceNotExist.Error()) ||
			strings.Contains(err.Error(), redpanda_monitor_service.ErrServiceNoLogFile.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceConnectionRefused.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceConnectionTimedOut.Error()) ||
			strings.Contains(err.Error(), redpanda_monitor_service.ErrServiceNoSectionsFound.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceStopped.Error()) // This is expected when the service is stopped or stopping, no need to fetch logs, metrics, etc.

		if !isExpectedError {

			if errors.Is(err, context.DeadlineExceeded) {
				// Context deadline exceeded should be retried with backoff, not ignored
				r.baseFSMInstance.SetError(err, snapshot.Tick)
				r.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in reconcileExternalChanges, will retry with backoff")
				err = nil // Clear error so reconciliation continues
				return nil, false
			}

			r.baseFSMInstance.SetError(err, snapshot.Tick)
			r.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		//nolint:ineffassign
		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = r.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		r.baseFSMInstance.SetError(err, snapshot.Tick)
		r.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the s6Manager
	s6Err, s6Reconciled := r.service.ReconcileManager(ctx, services, snapshot)
	if s6Err != nil {
		// Check if the error is from schema registry
		if redpanda_service.IsSchemaRegistryError(s6Err) {
			// For schema registry errors, only set the error if we're in running states
			if r.IsRunning() {
				r.baseFSMInstance.SetError(s6Err, snapshot.Tick)
				r.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
				return nil, false
			}
			// If not in running state, just log the error but don't set it in the FSM
			r.baseFSMInstance.GetLogger().Debugf("schema registry error while not in running state (%s), ignoring: %s", r.GetCurrentFSMState(), s6Err)
		} else {
			// For non-schema registry errors, always set the error
			r.baseFSMInstance.SetError(s6Err, snapshot.Tick)
			r.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
			return nil, false
		}
	}

	// If either Redpanda state, S6 state or the internal redpanda state via the Admin API was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the available time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || s6Reconciled || externalReconciled

	// It went all right, so clear the error
	r.baseFSMInstance.ResetState()
	return nil, reconciled
}

// reconcileExternalChanges checks if the RedpandaInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (r *RedpandaInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.RedpandaUpdateObservedStateTimeout)
	defer cancel()

	err = r.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err), false
	}
	return nil, false
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ObservedState.
// This is to ensure full testability of the FSM.
func (r *RedpandaInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := r.baseFSMInstance.GetCurrentFSMState()
	desiredState := r.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID(), currentState, desiredState)

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := r.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, r.CreateInstance, r.RemoveInstance, r.CheckForCreation)
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
		err, reconciled := r.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (r *RedpandaInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return r.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return r.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (r *RedpandaInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := r.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return r.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return r.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return r.reconcileRunningStates(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return r.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (r *RedpandaInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// First we need to ensure the S6 service is started
		running, reason := r.IsRedpandaS6Running()
		if !running {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting: %s", reason)
			return nil, false
		}

		// Check if "Successfully started Redpanda!" is found in logs
		started, reasonStarted := r.IsRedpandaStarted()
		if !started {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting: %s", reasonStarted)
			return nil, false
		}

		// Set the transition time when we detect successful start
		r.transitionToRunningTime = currentTime

		r.PreviousObservedState.ServiceInfo.StatusReason = ""
		return r.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (r *RedpandaInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		degraded, reasonDegraded := r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow)
		processingActivity, reasonProcessingActivity := r.IsRedpandaWithProcessingActivity()
		if degraded {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reasonDegraded)
			return r.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if !processingActivity { // if there is no activity, we move to Idle
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idling: %s", reasonProcessingActivity)
			return r.baseFSMInstance.SendEvent(ctx, EventNoDataTimeout), true
		}
		// If we're in Active,  send no status reason
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		degraded, reasonDegraded := r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow)
		processingActivity, reasonProcessingActivity := r.IsRedpandaWithProcessingActivity()

		if degraded {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reasonDegraded)
			return r.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if processingActivity { // if there is activity, we move to Active
			r.PreviousObservedState.ServiceInfo.StatusReason = ""
			return r.baseFSMInstance.SendEvent(ctx, EventDataReceived), true
		}
		r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idle: %s", reasonProcessingActivity)
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		degraded, reason := r.IsRedpandaDegraded(currentTime, constants.RedpandaLogWindow)
		if !degraded {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("recovering: %s", reason)
			return r.baseFSMInstance.SendEvent(ctx, EventRecovered), true
		}

		// CRITICAL FIX: If degraded because S6 is not running, attempt to restart it
		s6Running, _ := r.IsRedpandaS6Running()
		if !s6Running {
			r.baseFSMInstance.GetLogger().Debugf("S6 service stopped while in degraded state, attempting to restart")
			err := r.StartInstance(ctx, services.GetFileSystem())
			if err != nil {
				r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: failed to restart service: %v", err)
				return err, false
			}
			r.PreviousObservedState.ServiceInfo.StatusReason = "degraded: restarting service"
			return nil, false // Don't transition yet, wait for restart to take effect
		}

		r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reason)
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (r *RedpandaInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	// If we're in any operational state except Stopped or Stopping, initiate stop
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		// Attempt to initiate a stop
		if err := r.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		r.PreviousObservedState.ServiceInfo.StatusReason = "stopping"
		return r.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	// If already stopping, verify if the instance is completely stopped
	isStopped, reason := r.IsRedpandaS6Stopped()
	if currentState == OperationalStateStopping {
		if !isStopped {
			r.PreviousObservedState.ServiceInfo.StatusReason = fmt.Sprintf("stopping: %s", reason)
			return nil, false
		}
		// Transition from Stopping to Stopped
		r.PreviousObservedState.ServiceInfo.StatusReason = ""
		return r.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	}

	return nil, false
}
