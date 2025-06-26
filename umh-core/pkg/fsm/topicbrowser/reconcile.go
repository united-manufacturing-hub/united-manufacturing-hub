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

package topicbrowser

import (
	"context"
	"errors"
	"fmt"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// Reconcile examines the ConnectionInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (i *Instance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	tbInstanceName := i.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, tbInstanceName, time.Since(start))
		if err != nil {
			i.baseFSMInstance.GetLogger().Errorf("error reconciling topic browser instance %s: %v", tbInstanceName, err)
			i.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentTopicBrowserInstance, tbInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if i.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := i.baseFSMInstance.GetBackoffError(snapshot.Tick)
		i.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Topic Browser pipeline %s: %v", tbInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return i.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return i.IsRemoved() || i.IsRemoving() || i.IsStopping() || i.IsStopped() || i.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					// Use Remove() instead of RemoveInstance() to ensure proper FSM state management.
					// Remove() triggers FSM state transitions via baseFSMInstance.Remove(),
					// while RemoveInstance() bypasses FSM and directly performs file operations.
					return i.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return i.service.ForceRemove(ctx, services, tbInstanceName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = i.reconcileExternalChanges(ctx, services, snapshot); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, topicbrowsersvc.ErrServiceNotExist) && !errors.Is(err, s6.ErrServiceNotExist) {
			// Consider a special case for TopicBrowser FSM here
			// While creating for the first time, reconcileExternalChanges function will throw an error such as
			// s6 service not found in the path since TopicBrowser fsm is relying on BenthosFSM and Benthos in turn relies on S6 fsm
			// Inorder for TopicBrowser fsm to start, benthosManager.Reconcile should be called and this is called at the end of the function
			// So set the err to nil in this case
			// An example error: "failed to update observed state: failed to get observed DataflowComponent config: failed to get benthos config: failed to get benthos config file for service benthos-dataflow-hello-world-dfc: service does not exist"

			i.baseFSMInstance.SetError(err, snapshot.Tick)
			i.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
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
	err, reconciled = i.reconcileStateTransition(ctx, services, currentTime, snapshot)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		i.baseFSMInstance.SetError(err, snapshot.Tick)
		i.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the underlying Manager
	managerErr, managerReconciled := i.service.ReconcileManager(ctx, services, snapshot.Tick)
	if managerErr != nil {
		i.baseFSMInstance.SetError(managerErr, snapshot.Tick)
		i.baseFSMInstance.GetLogger().Errorf("error reconciling Topic Browser manager: %s", managerErr)
		return nil, false
	}

	// If either Connection state or Nmap state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || managerReconciled

	// It went all right, so clear the error
	i.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the ConnectionInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (i *Instance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.TopicBrowserUpdateObservedStateTimeout)
	defer cancel()

	err := i.UpdateObservedStateOfInstance(observedStateCtx, services, snapshot)
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
func (i *Instance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := i.baseFSMInstance.GetCurrentFSMState()
	desiredState := i.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled = i.baseFSMInstance.ReconcileLifecycleStates(
			ctx,
			services,
			currentState,
			i.CreateInstance,
			i.RemoveInstance,
			i.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled = i.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime, snapshot)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (i *Instance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	i.baseFSMInstance.GetLogger().Debugf("reconciling operational states from state %s to state %s", currentState, desiredState)

	switch desiredState {
	case OperationalStateActive:
		return i.reconcileTransitionToActive(ctx, services, currentState, currentTime, snapshot)
	case OperationalStateStopped:
		return i.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (i *Instance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	i.baseFSMInstance.GetLogger().Debugf("reconciling transition to active from state %s", currentState)

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := i.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return i.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return i.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return i.reconcileRunningStates(ctx, services, currentState, currentTime, snapshot)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return i.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (i *Instance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
	}()

	i.baseFSMInstance.GetLogger().Debugf("reconciling starting states from state %s", currentState)

	switch currentState {
	case OperationalStateStarting:
		return i.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (i *Instance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time, snapshot fsm.SystemSnapshot) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	i.baseFSMInstance.GetLogger().Debugf("reconciling running states from state %s", currentState)

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if degraded, reason := i.isTopicBrowserDegraded(); degraded {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("degraded: %s", reason)
			return i.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to check whether it is recovered
		if recovered := i.shouldRecoverFromDegraded(); recovered {
			i.ObservedState.ServiceInfo.StatusReason = ""
			return i.baseFSMInstance.SendEvent(ctx, EventRecovered), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (i *Instance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	i.baseFSMInstance.GetLogger().Debugf("reconciling transition to stopped from state %s", currentState)

	// If we're in any operational state except Stopped or Stopping, initiate stop
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		// Attempt to initiate a stop
		if err := i.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		i.ObservedState.ServiceInfo.StatusReason = "stopping"
		return i.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	// If already stopping, verify if the instance is completely stopped
	isStopped, reason := i.isTopicBrowserStopped()
	if currentState == OperationalStateStopping {
		if !isStopped {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("stopping: %s", reason)
			return nil, false
		}
		// Transition from Stopping to Stopped
		i.ObservedState.ServiceInfo.StatusReason = ""
		return i.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	}

	return nil, false
}
