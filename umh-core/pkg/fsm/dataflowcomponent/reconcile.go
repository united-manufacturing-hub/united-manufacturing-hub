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

package dataflowcomponent

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
	dataflowcomponentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Reconcile examines the DataflowComponentInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (d *DataflowComponentInstance) Reconcile(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	start := time.Now()
	dataflowComponentInstanceName := d.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, dataflowComponentInstanceName, time.Since(start))
		if err != nil {
			d.baseFSMInstance.GetLogger().Errorf("error reconciling dataflowcomponent instance %s: %v", dataflowComponentInstanceName, err)
			d.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentDataflowComponentInstance, dataflowComponentInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if d.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		err := d.baseFSMInstance.GetBackoffError(tick)
		d.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Dataflowcomponent pipeline %s: %v", dataflowComponentInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {

			// if it is already in stopped, stopping, removing states, and it again returns a permanent error,
			// we need to throw it to the manager as the instance itself here cannot fix it anymore
			if d.IsRemoved() || d.IsRemoving() || d.IsStopping() || d.IsStopped() {
				d.baseFSMInstance.GetLogger().Errorf("DataflowComponent instance %s is already in a terminal state, force removing it", dataflowComponentInstanceName)
				// force delete everything from the s6 file directory
				d.service.ForceRemoveDataFlowComponent(ctx, filesystemService, dataflowComponentInstanceName)
				return err, false
			} else {
				d.baseFSMInstance.GetLogger().Errorf("DataflowComponent instance %s is not in a terminal state, resetting state and removing it", dataflowComponentInstanceName)
				d.baseFSMInstance.ResetState()
				d.Remove(ctx)
				return nil, false // let's try to at least reconcile towards a stopped / removed state
			}
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	err = d.reconcileExternalChanges(ctx, filesystemService, tick)
	switch {
	case err == nil:
	// All good. Do nothing and continue to reconcile
	case errors.Is(err, dataflowcomponentservice.ErrServiceNotExists):
		// If service doesn't exists, it will be created in the next reconcile loop. So set the error to nil
		err = nil
	case errors.Is(err, context.DeadlineExceeded):
		// Healthchecks occasionally take longer (sometimes up to 70ms),
		// resulting in context.DeadlineExceeded errors. In this case, we want to
		// mark the reconciliation as complete for this tick since we've likely
		// already consumed significant time. We return reconciled=true to prevent
		// further reconciliation attempts in the current tick.
		return nil, true // We don't want to return an error here, as this can happen in normal operations
	default:
		// Consider a special case for DFC FSM here
		// While creating for the first time, reconcileExternalChanges function will throw an error such as
		// s6 config file not found in the path since DFC fsm is relying on BenthosFSM and Benthos in turn relies on S6 fsm
		// Inorder for DFC fsm to start, benthosManager.Reconcile should be called and this is called at the end of the function
		// So set the err to nil in this case
		// An example error: "failed to update observed state: failed to get observed DataflowComponent config: failed to get benthos config: failed to get benthos config file for service benthos-dataflow-hello-world-dfc: service does not exist"
		if strings.Contains(err.Error(), "service does not exist") {
			err = nil
		} else {
			d.baseFSMInstance.SetError(err, tick)
			d.baseFSMInstance.GetLogger().Errorf("error while reconciling external changes for dataflowcomponent fsm: %v", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = d.reconcileStateTransition(ctx, filesystemService, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, fsm.ErrInstanceRemoved) {
			return nil, false
		}

		d.baseFSMInstance.SetError(err, tick)
		d.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the benthosManager
	benthosErr, benthosReconciled := d.service.ReconcileManager(ctx, filesystemService, tick)
	if benthosErr != nil {
		d.baseFSMInstance.SetError(benthosErr, tick)
		d.baseFSMInstance.GetLogger().Errorf("error reconciling benthosManager: %s", benthosErr)
		return nil, false
	}

	// If either Dataflowcomponent state or Benthos state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || benthosReconciled

	// It went all right, so clear the error
	d.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the DataflowComponentInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (d *DataflowComponentInstance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.DataflowComponentUpdateObservedStateTimeout)
	defer cancel()

	err := d.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick, start)
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
func (d *DataflowComponentInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := d.baseFSMInstance.GetCurrentFSMState()
	desiredState := d.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := d.reconcileLifecycleStates(ctx, filesystemService, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := d.reconcileOperationalStates(ctx, filesystemService, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (creating/removing)
func (d *DataflowComponentInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		if err := d.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return d.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Check if the service is created
		// For now, we'll assume it's created immediately after initiating creation
		return d.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		if err := d.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return d.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (d *DataflowComponentInstance) reconcileOperationalStates(ctx context.Context, filesystemService filesystem.Service, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return d.reconcileTransitionToActive(ctx, filesystemService, currentState, currentTime)
	case OperationalStateStopped:
		return d.reconcileTransitionToStopped(ctx, filesystemService, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (d *DataflowComponentInstance) reconcileTransitionToActive(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		if err := d.StartInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return d.baseFSMInstance.SendEvent(ctx, EventStart), true

	case OperationalStateStarting, OperationalStateStartingFailed:
		return d.reconcileStartingState(ctx, filesystemService, currentState, currentTime)

	case OperationalStateIdle, OperationalStateActive, OperationalStateDegraded:
		return d.reconcileRunningState(ctx, filesystemService, currentState, currentTime)

	}

	return nil, false
}

// reconcileStartingState handles the various starting phase states when transitioning to Active.
func (d *DataflowComponentInstance) reconcileStartingState(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// First check if the undelying benthos is running
		if d.IsDataflowComponentBenthosRunning() {
			return d.baseFSMInstance.SendEvent(ctx, EventStartDone), true
		}

		// Check if we have exceeded the grace period since entering the Starting state
		if d.IsStartupGracePeriodExpired(currentTime, currentState) {
			return d.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		// Still within grace period, continue waiting until the next reconcile
	case OperationalStateStartingFailed:
	// Do not do anything here.
	// The only way to get out of this state is to be removed and recreated by the manager when there is a config change.
	// When the config is changed, the Manager will jump-in to recreate the DFC
	// So, let's be stuck in this state for sometime
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
	return nil, false
}

// reconcileRunningState handles the various running states when transitioning to Active.
func (d *DataflowComponentInstance) reconcileRunningState(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if d.IsDataflowComponentDegraded() {
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosDegraded), true
		} else if !d.IsDataflowComponentWithProcessingActivity() { // if there is no activity, we move to Idle
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosNoDataReceived), true
		}
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		if d.IsDataflowComponentDegraded() {
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosDegraded), true
		} else if d.IsDataflowComponentWithProcessingActivity() { // if there is activity, we move to Active
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosDataReceived), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		if !d.IsDataflowComponentDegraded() {
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosRecovered), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (d *DataflowComponentInstance) reconcileTransitionToStopped(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do more
		return nil, false
	case OperationalStateStopping:
		if d.IsDataflowComponentBenthosStopped() {
			// Transition from Stopping to Stopped
			return d.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
	default:
		if err := d.StopInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return d.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	return nil, false
}
