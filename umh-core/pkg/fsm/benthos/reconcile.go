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

package benthos

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
	benthos_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the BenthosInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (b *BenthosInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	benthosInstanceName := b.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, benthosInstanceName, time.Since(start))
		if err != nil {
			b.baseFSMInstance.GetLogger().Errorf("error reconciling Benthos instance %s: %v", benthosInstanceName, err)
			b.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentBenthosInstance, benthosInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if b.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := b.baseFSMInstance.GetBackoffError(snapshot.Tick)
		b.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Benthos pipeline %s: %v", benthosInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return b.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return b.IsRemoved() || b.IsRemoving() || b.IsStopping() || b.IsStopped() || b.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return b.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return b.service.ForceRemoveBenthos(ctx, services.GetFileSystem(), benthosInstanceName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err := b.reconcileExternalChanges(ctx, services, snapshot.Tick, start); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, benthos_service.ErrServiceNotExist) {

			if errors.Is(err, context.DeadlineExceeded) {
				// Healthchecks occasionally take longer (sometimes up to 70ms),
				// resulting in context.DeadlineExceeded errors. In this case, we want to
				// mark the reconciliation as complete for this tick since we've likely
				// already consumed significant time. We return reconciled=true to prevent
				// further reconciliation attempts in the current tick.
				return nil, true // We don't want to return an error here, as this can happen in normal operations
			}

			b.baseFSMInstance.SetError(err, snapshot.Tick)
			b.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		//nolint:ineffassign
		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = b.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		b.baseFSMInstance.SetError(err, snapshot.Tick)
		b.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the s6Manager
	s6Err, s6Reconciled := b.service.ReconcileManager(ctx, services, snapshot.Tick)
	if s6Err != nil {
		b.baseFSMInstance.SetError(s6Err, snapshot.Tick)
		b.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
		return nil, false
	}
	// If either Benthos state or S6 state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || s6Reconciled

	// It went all right, so clear the error
	b.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the BenthosInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (b *BenthosInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, tick uint64, loopStartTime time.Time) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.BenthosUpdateObservedStateTimeout)
	defer cancel()

	err := b.UpdateObservedStateOfInstance(observedStateCtx, services, tick, loopStartTime)
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
func (b *BenthosInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := b.baseFSMInstance.GetCurrentFSMState()
	desiredState := b.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do.
	// This is wrong, as there could be a degradation
	// if currentState == desiredState {
	// 	return nil, false
	// }

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := b.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, b.CreateInstance, b.RemoveInstance, b.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := b.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (b *BenthosInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return b.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return b.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (b *BenthosInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	// If we're stopped, we need to start first
	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := b.StartInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return b.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	// Handle starting phase states
	if IsStartingState(currentState) {
		return b.reconcileStartingStates(ctx, services, currentState, currentTime)
	} else if IsRunningState(currentState) {
		return b.reconcileRunningStates(ctx, services, currentState, currentTime)
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (b *BenthosInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// First we need to ensure the S6 service is started
		running, reason := b.IsBenthosS6Running()
		if !running {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("starting: %s", reason)
			return nil, false
		}

		return b.baseFSMInstance.SendEvent(ctx, EventS6Started), true
	case OperationalStateStartingConfigLoading:
		// Check if config has been loaded

		// If the S6 is not running, go back to starting
		running, reason := b.IsBenthosS6Running()
		if !running {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		// Now check whether benthos has loaded the config
		loaded, reason := b.IsBenthosConfigLoaded()
		if !loaded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("waiting for config to be loaded: %s", reason)
			return nil, false
		}

		return b.baseFSMInstance.SendEvent(ctx, EventConfigLoaded), true
	case OperationalStateStartingWaitingForHealthchecks:
		// If the S6 is not running, go back to starting
		running, reason := b.IsBenthosS6Running()
		if !running {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		loaded, reason := b.IsBenthosConfigLoaded()
		if !loaded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		// Check if healthchecks have passed
		passed, reason := b.IsBenthosHealthchecksPassed()
		if !passed {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("waiting for healthchecks to pass: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		return b.baseFSMInstance.SendEvent(ctx, EventHealthchecksPassed), true
	case OperationalStateStartingWaitingForServiceToRemainRunning:
		// If the S6 is not running, go back to starting
		running, reason := b.IsBenthosS6Running()
		if !running {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		loaded, reason := b.IsBenthosConfigLoaded()
		if !loaded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		passed, reason := b.IsBenthosHealthchecksPassed()
		if !passed {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("start failed: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventStartFailed), true
		}

		// Check if service has been running stably for some time
		running, reason = b.IsBenthosRunningForSomeTimeWithoutErrors(currentTime, constants.BenthosLogWindow)
		if !running {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("waiting for service to remain running: %s", reason)
			return nil, false
		}

		return b.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (b *BenthosInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		degraded, reasonDegraded := b.IsBenthosDegraded(currentTime, constants.BenthosLogWindow)
		processing, reasonProcessing := b.IsBenthosWithProcessingActivity()
		if degraded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("degrading: %s", reasonDegraded)
			return b.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if !processing { // if there is no activity, we move to Idle
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("idling: %s", reasonProcessing)
			return b.baseFSMInstance.SendEvent(ctx, EventNoDataTimeout), true
		}
		// if we are active, send no status reason
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		degraded, reasonDegraded := b.IsBenthosDegraded(currentTime, constants.BenthosLogWindow)
		processing, reasonProcessing := b.IsBenthosWithProcessingActivity()
		if degraded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("degrading: %s", reasonDegraded)
			return b.baseFSMInstance.SendEvent(ctx, EventDegraded), true
		} else if processing { // if there is activity, we move to Active
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("active: %s", reasonProcessing)
			return b.baseFSMInstance.SendEvent(ctx, EventDataReceived), true
		}
		b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("idle: %s", reasonProcessing)
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		degraded, reason := b.IsBenthosDegraded(currentTime, constants.BenthosLogWindow)
		if !degraded {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("recovering: %s", reason)
			return b.baseFSMInstance.SendEvent(ctx, EventRecovered), true
		}
		b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("degraded: %s", reason)
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (b *BenthosInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	// If we're in any operational state except Stopped or Stopping, initiate stop
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		// Attempt to initiate a stop
		if err := b.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = "stopping"
		return b.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	// If already stopping, verify if the instance is completely stopped
	isStopped, reason := b.IsBenthosS6Stopped()
	if currentState == OperationalStateStopping {
		if !isStopped {
			b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = fmt.Sprintf("stopping: %s", reason)
			return nil, false
		}
		b.ObservedState.ServiceInfo.BenthosStatus.StatusReason = ""
		return b.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	}

	return nil, false
}
