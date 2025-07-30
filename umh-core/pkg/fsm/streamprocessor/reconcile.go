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

package streamprocessor

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the StreamProcessorInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsep.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff periop.
func (i *Instance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	spName := i.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, spName, time.Since(start))
		if err != nil {
			i.baseFSMInstance.GetLogger().Errorf("error reconciling streamprocessor instance %s: %v", spName, err)
			i.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentStreamProcessorInstance, spName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if i.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := i.baseFSMInstance.GetBackoffError(snapshot.Tick)
		i.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Stream Processor pipeline %s: %v", spName, err)

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
					return i.IsRemoved() || i.IsRemoving() || i.IsStopping() || i.IsStopped()
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
					return i.service.ForceRemove(ctx, services.GetFileSystem(), spName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if i.baseFSMInstance.IsRemoving() {
		// Skip external changes detection during removal - config files may be deleted
		i.baseFSMInstance.GetLogger().Debugf("Skipping external changes detection during removal")
	} else {
		if err = i.reconcileExternalChanges(ctx, services, snapshot); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				// Healthchecks occasionally take longer (sometimes up to 70ms),
				// resulting in context.DeadlineExceeded errors. In this case, we want to
				// mark the reconciliation as complete for this tick since we've likely
				// already consumed significant time. We return reconciled=true to prevent
				// further reconciliation attempts in the current tick.
				i.baseFSMInstance.SetError(err, snapshot.Tick)
				i.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in reconcileExternalChanges, will retry with backoff")
				return nil, true // We don't want to return an error here, as this can happen in normal operations
			}

			// Log the error but always continue reconciling - we need reconcileStateTransition to run
			// to restore services after restart, even if we can't read their status yet
			i.baseFSMInstance.GetLogger().Warnf("failed to update observed state (continuing reconciliation): %s", err)
			
			// For all other errors, just continue reconciling without setting backoff
			err = nil
		}
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = i.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		// Enhanced error logging with state context
		currentState := i.baseFSMInstance.GetCurrentFSMState()
		desiredState := i.baseFSMInstance.GetDesiredFSMState()
		i.baseFSMInstance.GetLogger().Errorf("error reconciling state transition: current_state='%s', desired_state='%s', error: %s",
			currentState, desiredState, err)

		i.baseFSMInstance.SetError(err, snapshot.Tick)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the DFC manager
	managerErr, managerReconciled := i.service.ReconcileManager(ctx, services, snapshot.Tick)
	if managerErr != nil {
		i.baseFSMInstance.SetError(managerErr, snapshot.Tick)
		i.baseFSMInstance.GetLogger().Errorf("error reconciling manager: %s", managerErr)
		return nil, false
	}

	// If either Dataflowcomponent state or manager state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || managerReconciled

	// It went all right, so clear the error
	i.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the DataflowComponentInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (i *Instance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.StreamProcessorUpdateObservedStateTimeout)
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
func (i *Instance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := i.baseFSMInstance.GetCurrentFSMState()
	desiredState := i.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID(), currentState, desiredState)

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := i.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, i.CreateInstance, i.RemoveInstance, i.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := i.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (i *Instance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return i.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return i.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (i *Instance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	// If we're stopped, we need to start first
	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := i.StartInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		i.ObservedState.ServiceInfo.StatusReason = "started"
		// Send event to transition from Stopped to Starting
		return i.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	// Handle starting phase states
	if IsStartingState(currentState) {
		return i.reconcileStartingStates(ctx, services, currentState, currentTime)
	} else if IsRunningState(currentState) {
		return i.reconcileRunningState(ctx, services, currentState, currentTime)
	} else if currentState == OperationalStateStopping {
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return i.reconcileTransitionToStopped(ctx, services, currentState)
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (i *Instance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStartingRedpanda:

		// Now check whether redpanda is healthy
		running, reason := i.IsRedpandaHealthy()
		if !running {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting: %s", reason)
			return nil, false
		}

		return i.baseFSMInstance.SendEvent(ctx, EventStartRedpandaUp), true
	case OperationalStateStartingDFC:

		// If the redpanda is not healthy, we need to go back to starting
		running, reason := i.IsRedpandaHealthy()
		if !running {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting: %s", reason)
			return i.baseFSMInstance.SendEvent(ctx, EventStartRetry), false // a previous succeeding check failed, so let's retry the whole start process
		}

		// Now check whether the DFC is healthy
		running, reason = i.IsDFCHealthy()
		if !running {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("starting: %s", reason)
			return nil, false
		}

		return i.baseFSMInstance.SendEvent(ctx, EventStartDFCUp), true
	case OperationalStateStartingFailedDFC:

		// For OperationalStateStartingFailedDFC, do not do anything here.
		// The only way to get out of this state is to be removed and recreated by the manager when there is a config change.
		// When the config is changed, the Manager will jump-in to recreate the DFC
		// So, let's be stuck in this state for sometime

	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
	return nil, false
}

// reconcileRunningState handles the various running states when transitioning to Active.
func (i *Instance) reconcileRunningState(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		redpandaHealthy, reasonRedpanda := i.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := i.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := i.IsOtherDegraded()
		hasActivity, reasonActivity := i.IsDataflowComponentWithProcessingActivity()
		if otherDegraded {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("other degraded: %s", reasonOtherDegraded)
			if currentState != OperationalStateDegradedOther {
				return i.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true
			}
			return nil, false
		} else if !redpandaHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("redpanda degraded: %s", reasonRedpanda)
			if currentState != OperationalStateDegradedRedpanda {
				return i.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}
			return nil, false
		} else if !dfcHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("DFC degraded: %s", reasonDFC)
			if currentState != OperationalStateDegradedDFC {
				return i.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}
			return nil, false
		} else if !hasActivity { // if there is no activity, we move to Idle
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idling: %s", reasonActivity)
			return i.baseFSMInstance.SendEvent(ctx, EventDFCIdle), true
		}
		i.ObservedState.ServiceInfo.StatusReason = "" // if everything is fine, reset the status reason
		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		// If we're in Active, we need to check whether it is degraded
		redpandaHealthy, reasonRedpanda := i.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := i.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := i.IsOtherDegraded()
		hasActivity, reasonActivity := i.IsDataflowComponentWithProcessingActivity()
		if otherDegraded {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("other degraded: %s", reasonOtherDegraded)
			if currentState != OperationalStateDegradedOther {
				return i.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true
			}
			return nil, false
		} else if !redpandaHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("redpanda degraded: %s", reasonRedpanda)
			if currentState != OperationalStateDegradedRedpanda {
				return i.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}
			return nil, false
		} else if !dfcHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("DFC degraded: %s", reasonDFC)
			if currentState != OperationalStateDegradedDFC {
				return i.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}
			return nil, false
		} else if !hasActivity { // if there is no activity, we stay in idle
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("idling: %s", reasonActivity)
			return nil, false
		}
		i.ObservedState.ServiceInfo.StatusReason = "active" // if everything is fine, reset the status reason
		return i.baseFSMInstance.SendEvent(ctx, EventDFCActive), true
	case OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther:
		// If we're in Degraded, we need to recover to move to Idle
		redpandaHealthy, reasonRedpanda := i.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := i.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := i.IsOtherDegraded()

		// CRITICAL: Only send degraded events for NEW degradation issues, not the current one.
		//
		// Problem: Without these currentState checks, we get stuck in "no transition" errors:
		// 1. FSM is in degraded_dfc state due to unhealthy DFC
		// 2. Reconcile loop checks !dfcHealthy → still true (DFC still unhealthy)
		// 3. Sends EventDFCDegraded again → attempts degraded_dfc → degraded_dfc transition
		// 4. looplab/fsm treats self-transitions as NoTransitionError by design
		// 5. baseFSM.SendEvent() treats any FSM error as failure → backoff → eventual permanent failure
		// 6. Instance gets stuck in permanent backoff and eventually force-removed
		//
		// Solution: Check currentState != target degraded state before sending degraded events.
		// This allows:
		// - Staying in current degraded state when the same issue persists (no event sent)
		// - Transitioning to different degraded state when new issues arise
		// - Recovering to idle when all issues resolve (EventRecovered)

		if otherDegraded {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("other degraded: %s", reasonOtherDegraded) // Always set status reason
			if currentState != OperationalStateDegradedOther {
				return i.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true // Send event for NEW degraded issue
			}
			return nil, false // Stay in current degraded state (same issue persists)
		} else if !redpandaHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("redpanda degraded: %s", reasonRedpanda)
			if currentState != OperationalStateDegradedRedpanda {
				return i.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}
			return nil, false
		} else if !dfcHealthy {
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("DFC degraded: %s", reasonDFC)
			if currentState != OperationalStateDegradedDFC {
				return i.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}
			return nil, false
		}

		// If we reach here, all issues are resolved - recover to Idle
		i.ObservedState.ServiceInfo.StatusReason = "recovering"
		return i.baseFSMInstance.SendEvent(ctx, EventRecovered), true
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stoppep.
// It deals with moving from any operational state to Stopping and then to Stoppep.
func (i *Instance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do more
		i.ObservedState.ServiceInfo.StatusReason = "stopped"
		return nil, false
	case OperationalStateStopping:
		stopped, reason := i.IsStreamProcessorStopped()
		if stopped {
			// Transition from Stopping to Stopped
			i.ObservedState.ServiceInfo.StatusReason = fmt.Sprintf("stopped: %s", reason)
			return i.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
		i.ObservedState.ServiceInfo.StatusReason = "stopping"
		return nil, false
	default:
		if err := i.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		i.ObservedState.ServiceInfo.StatusReason = "stopping"
		return i.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
