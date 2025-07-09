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
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	dataflowcomponentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the DataflowComponentInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (d *DataflowComponentInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
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
	if d.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := d.baseFSMInstance.GetBackoffError(snapshot.Tick)
		d.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Dataflowcomponent pipeline %s: %v", dataflowComponentInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return d.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return d.IsRemoved() || d.IsRemoving() || d.IsStopping() || d.IsStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					// Use Remove() instead of RemoveInstance() to ensure proper FSM state management.
					// Remove() triggers FSM state transitions via baseFSMInstance.Remove(),
					// while RemoveInstance() bypasses FSM and directly performs file operations.
					return d.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return d.service.ForceRemoveDataFlowComponent(ctx, services.GetFileSystem(), dataflowComponentInstanceName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	err = d.reconcileExternalChanges(ctx, services, snapshot)
	if err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, dataflowcomponentservice.ErrServiceNotExists) && !errors.Is(err, s6.ErrServiceNotExist) {
			// errors.Is(err, s6.ErrServiceNotExist)
			// Consider a special case for DFC FSM here
			// While creating for the first time, reconcileExternalChanges function will throw an error such as
			// s6 service not found in the path since DFC fsm is relying on BenthosFSM and Benthos in turn relies on S6 fsm
			// Inorder for DFC fsm to start, benthosManager.Reconcile should be called and this is called at the end of the function
			// So set the err to nil in this case
			// An example error: "failed to update observed state: failed to get observed DataflowComponent config: failed to get benthos config: failed to get benthos config file for service benthos-dataflow-hello-world-dfc: service does not exist"

			d.baseFSMInstance.SetError(err, snapshot.Tick)
			d.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)

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

		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = d.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		d.baseFSMInstance.SetError(err, snapshot.Tick)
		d.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the benthosManager
	benthosErr, benthosReconciled := d.service.ReconcileManager(ctx, services, snapshot.Tick)
	if benthosErr != nil {
		d.baseFSMInstance.SetError(benthosErr, snapshot.Tick)
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
func (d *DataflowComponentInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.DataflowComponentUpdateObservedStateTimeout)
	defer cancel()

	err := d.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
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
func (d *DataflowComponentInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := d.baseFSMInstance.GetCurrentFSMState()
	desiredState := d.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID(), currentState, desiredState)

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := d.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, d.CreateInstance, d.RemoveInstance, d.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := d.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (d *DataflowComponentInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return d.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return d.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (d *DataflowComponentInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := d.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return d.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return d.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return d.reconcileRunningStates(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return d.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (d *DataflowComponentInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// 1. Has the instance *ever* failed to start before?
		//    ──► yes:  transition permanently to StartingFailed.
		didFail, reason := d.DidDFCAlreadyFailedBefore(ctx)
		if didFail {
			d.ObservedState.ServiceInfo.StatusReason = reason
			return d.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		// 2. Is Benthos already up (race-condition where the service was started
		//    outside the FSM or recovered very quickly)?
		//    ──► yes:  mark start successful (StartDone) and proceed.
		if d.IsDataflowComponentBenthosRunning() {
			d.ObservedState.ServiceInfo.StatusReason = "started up"
			return d.baseFSMInstance.SendEvent(ctx, EventStartDone), true
		}

		// 3. Have we waited longer than the grace period without a successful start?
		//    ──► yes:  declare the start attempt failed.
		didExceedGracePeriod, reason := d.IsStartingPeriodGracePeriodExceeded(ctx, currentTime)
		if didExceedGracePeriod {
			d.ObservedState.ServiceInfo.StatusReason = reason
			return d.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}
		benthosStatusReason := d.ObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.StatusReason
		if benthosStatusReason == "" {
			benthosStatusReason = "not existing"
		}

		// 4. Otherwise remain in OperationalStateStarting and try again on next tick.
		d.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting - waiting for benthos to be up: %s", benthosStatusReason)
		return nil, false
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

// reconcileRunningStates handles the various running states when transitioning to Active.
func (d *DataflowComponentInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		degraded, reasonDegraded := d.IsDataflowComponentDegraded()
		hasActivity, reasonActivity := d.IsDataflowComponentWithProcessingActivity()
		if degraded {
			d.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reasonDegraded)
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosDegraded), true
		} else if !hasActivity { // if there is no activity, we move to Idle
			d.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idling: %s", reasonActivity)
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosNoDataReceived), true
		}
		d.ObservedState.ServiceInfo.StatusReason = "" // if everything is fine, reset the status reason
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		degraded, reasonDegraded := d.IsDataflowComponentDegraded()
		hasActivity, reasonActivity := d.IsDataflowComponentWithProcessingActivity()
		if degraded {
			d.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reasonDegraded)
			return d.baseFSMInstance.SendEvent(ctx, EventBenthosDegraded), true
		} else if !hasActivity { // if there is no activity, we stay in Idle
			d.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idle: %s", reasonActivity)
			return nil, false
		}
		d.ObservedState.ServiceInfo.StatusReason = "active" // if everything is fine, reset the status reason
		return d.baseFSMInstance.SendEvent(ctx, EventBenthosDataReceived), true
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		degraded, reason := d.IsDataflowComponentDegraded()
		if degraded { // if it is still degraded, we do not do anything
			d.ObservedState.ServiceInfo.StatusReason = reason
			return nil, false
		}

		// if it is not degraded, we move to Idle
		d.ObservedState.ServiceInfo.StatusReason = "recovering" // if everything is fine, reset the status reason
		return d.baseFSMInstance.SendEvent(ctx, EventBenthosRecovered), true
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (d *DataflowComponentInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, d.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do more
		d.ObservedState.ServiceInfo.StatusReason = "stopped"
		return nil, false
	case OperationalStateStopping:
		if d.IsDataflowComponentBenthosStopped() {
			// Transition from Stopping to Stopped
			d.ObservedState.ServiceInfo.StatusReason = "stopped"
			return d.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
		d.ObservedState.ServiceInfo.StatusReason = "stopping"
		return nil, false
	default:
		if err := d.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		d.ObservedState.ServiceInfo.StatusReason = "stopping"
		return d.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
