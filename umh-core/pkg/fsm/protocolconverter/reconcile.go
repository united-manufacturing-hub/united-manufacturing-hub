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

package protocolconverter

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the DataflowComponentInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsep.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff periop.
func (p *ProtocolConverterInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	protocolConverterInstanceName := p.baseFSMInstance.GetID()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, protocolConverterInstanceName, time.Since(start))

		if err != nil {
			p.baseFSMInstance.GetLogger().Errorf("error reconciling protocolconverter instance %s: %v", protocolConverterInstanceName, err)
			p.PrintState()
			// Add metrics for error
			metrics.IncErrorCountAndLog(metrics.ComponentProtocolConverterInstance, protocolConverterInstanceName, err, p.baseFSMInstance.GetLogger())
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		if p.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "start of reconciliation") {
			return nil, false
		}

		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if p.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := p.baseFSMInstance.GetBackoffError(snapshot.Tick)
		p.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Protocolconverter pipeline %s: %v", protocolConverterInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return p.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return p.IsRemoved() || p.IsRemoving() || p.IsStopping() || p.IsStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					// Use Remove() instead of RemoveInstance() to ensure proper FSM state management.
					// Remove() triggers FSM state transitions via baseFSMInstance.Remove(),
					// while RemoveInstance() bypasses FSM and directly performs file operations.
					return p.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return p.service.ForceRemoveProtocolConverter(ctx, services.GetFileSystem(), protocolConverterInstanceName)
				},
			)
		}

		return nil, false
	}

	// Step 2: Detect external changes - skip during removal
	if p.baseFSMInstance.IsRemoving() {
		// Skip external changes detection during removal - config files may be deleted
		p.baseFSMInstance.GetLogger().Debugf("Skipping external changes detection during removal")
	} else {
		if err = p.reconcileExternalChanges(ctx, services, snapshot); err != nil {
			if p.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileExternalChanges") {
				return nil, false
			}

			// Log the error but always continue reconciling - we need reconcileStateTransition to run
			// to restore services after restart, even if we can't read their status yet
			p.baseFSMInstance.GetLogger().Warnf("failed to update observed state (continuing reconciliation): %s", err)

			// For all other errors, just continue reconciling without setting backoff
			err = nil
		}
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check

	err, reconciled = p.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		if p.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileStateTransition") {
			return nil, false
		}

		// Enhanced error logging with state context
		currentState := p.baseFSMInstance.GetCurrentFSMState()
		desiredState := p.baseFSMInstance.GetDesiredFSMState()
		p.baseFSMInstance.GetLogger().Errorf("error reconciling state transition: current_state='%s', desired_state='%s', error: %s",
			currentState, desiredState, err)

		p.baseFSMInstance.SetError(err, snapshot.Tick)

		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the manager
	managerErr, managerReconciled := p.service.ReconcileManager(ctx, services, snapshot.Tick)
	if managerErr != nil {
		if errors.Is(managerErr, context.DeadlineExceeded) {
			// Context deadline exceeded should be retried with backoff, not ignored
			p.baseFSMInstance.SetError(managerErr, snapshot.Tick)
			p.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in manager reconciliation, will retry with backoff")

			return nil, false
		}

		p.baseFSMInstance.SetError(managerErr, snapshot.Tick)
		p.baseFSMInstance.GetLogger().Errorf("error reconciling manager: %s", managerErr)

		return nil, false
	}

	// If either Dataflowcomponent state or manager state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the available time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || managerReconciled

	// It went all right, so clear the error
	p.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the DataflowComponentInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed).
func (p *ProtocolConverterInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.ProtocolConverterUpdateObservedStateTimeout)
	defer cancel()

	err := p.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
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
func (p *ProtocolConverterInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := p.baseFSMInstance.GetCurrentFSMState()
	desiredState := p.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID(), currentState, desiredState)

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := p.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, p.CreateInstance, p.RemoveInstance, p.CheckForCreation)
		if err != nil {
			return err, false
		}

		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := p.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}

		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping).
func (p *ProtocolConverterInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return p.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return p.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (p *ProtocolConverterInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	// If we're stopped, we need to start first
	if currentState == OperationalStateStopped {
		// Send event to transition from Stopped to StartingConnection
		// The connection will be started by StartConnectionInstance in the starting_connection state
		// This prevents Benthos from connecting and sending data when the connection is flaky or filtered
		p.ObservedState.ServiceInfo.StatusReason = "starting_connection"

		return p.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	// Handle starting phase states
	switch {
	case IsStartingState(currentState):
		return p.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return p.reconcileRunningState(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return p.reconcileTransitionToStopped(ctx, services, currentState)
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (p *ProtocolConverterInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStartingConnection:
		// Start the connection component first
		if err := p.StartConnectionInstance(ctx, services.GetFileSystem()); err != nil {
			p.baseFSMInstance.GetLogger().Debugf("Failed to start connection: %v", err)

			return err, false
		}

		// Check if connection is up before proceeding
		running, reason := p.IsConnectionUp()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return nil, false
		}

		return p.baseFSMInstance.SendEvent(ctx, EventStartConnectionUp), true
	case OperationalStateStartingRedpanda:
		// If the connection is not up, we need to go back to starting
		running, reason := p.IsConnectionUp()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return p.baseFSMInstance.SendEvent(ctx, EventStartRetry), false // a previous succeeding check failed, so let's retry the whole start process
		}

		// Now check whether redpanda is healthy
		running, reason = p.IsRedpandaHealthy()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return nil, false
		}

		return p.baseFSMInstance.SendEvent(ctx, EventStartRedpandaUp), true
	case OperationalStateStartingDFC:
		// If the connection is not up, we need to go back to starting
		running, reason := p.IsConnectionUp()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return p.baseFSMInstance.SendEvent(ctx, EventStartRetry), false // a previous succeeding check failed, so let's retry the whole start process
		}

		// If the redpanda is not healthy, we need to go back to starting
		running, reason = p.IsRedpandaHealthy()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return p.baseFSMInstance.SendEvent(ctx, EventStartRetry), false // a previous succeeding check failed, so let's retry the whole start process
		}

		// If neither read not write DFC is existing, then go into OperationalStateStartingFailedDFCMissing
		existing, reason := p.IsDFCExisting()
		if !existing {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return p.baseFSMInstance.SendEvent(ctx, EventStartFailedDFCMissing), true
		}

		// Start the DFC components now that prerequisites are met
		if err := p.StartDFCInstance(ctx, services.GetFileSystem()); err != nil {
			p.baseFSMInstance.GetLogger().Debugf("Failed to start DFC: %v", err)

			return err, false
		}

		// Now check whether the DFC is healthy
		running, reason = p.IsDFCHealthy()
		if !running {
			p.ObservedState.ServiceInfo.StatusReason = "starting: " + reason

			return nil, false
		}

		return p.baseFSMInstance.SendEvent(ctx, EventStartDFCUp), true
	case OperationalStateStartingFailedDFCMissing:
		// For OperationalStateStartingFailedDFCMissing, check if a DFC is now available and retry starting
		// CRITICAL FIX: Recovery mechanism for OperationalStateStartingFailedDFCMissing
		//
		// PROBLEM DESCRIPTION:
		// The Protocol Converter FSM had a design flaw where instances would get permanently
		// stuck in OperationalStateStartingFailedDFCMissing when created without DFC configuration
		// and later updated to include DFCs. This created an unrecoverable state that required
		// manual intervention or instance recreation.
		//
		// ROOT CAUSE ANALYSIS:
		// 1. INITIAL STATE FLOW:
		//    - User creates Protocol Converter without DFC configuration
		//    - FSM transitions: stopped → starting_connection → starting_redpanda → starting_dfc
		//    - In starting_dfc state, IsDFCExisting() returns false (no DFC configured)
		//    - FSM transitions to OperationalStateStartingFailedDFCMissing via EventStartFailedDFCMissing
		//
		// 2. CONFIGURATION UPDATE FLOW:
		//    - User adds DFC to configuration
		//    - Base manager detects config change and calls setConfig()
		//    - UpdateObservedStateOfInstance() successfully creates DFC via EvaluateDFCDesiredStates()
		//    - BUT the Protocol Converter FSM remains stuck in OperationalStateStartingFailedDFCMissing
		//
		// 3. WHY IT STAYED STUCK:
		//    - Original code had no recovery logic: "Do not do anything here"
		//    - Comment stated: "only way to get out is to be removed and recreated by manager"
		//    - However, Protocol Converters do NOT call Remove() on config changes (unlike S6 instances)
		//    - They only call UpdateInManager() to update configuration in-place
		//    - Base manager only calls Remove() when instance is completely removed from config
		//    - Result: Instance permanently stuck with no automatic recovery path
		//
		// 4. COMPARISON WITH OTHER FSM TYPES:
		//    - S6 instances: Call Remove() on config changes → get recreated → automatic recovery
		//    - Protocol Converters: Call UpdateInManager() → stay in same instance → no recovery
		//    - This architectural difference created the recovery gap
		//
		// SOLUTION IMPLEMENTED:
		// For OperationalStateStartingFailedDFCMissing specifically:
		// 1. Check if DFC is now available using existing IsDFCExisting() logic
		// 2. If DFC exists, send EventStartRetry to restart the startup sequence
		// 3. EventStartRetry transitions back to OperationalStateStartingConnection
		// 4. Normal startup flow continues: connection → redpanda → dfc → idle/active
		// 5. If DFC still missing, remain in failed state with clear status reason
		//
		existing, reason := p.IsDFCExisting()
		if existing {
			// DFC is now available, retry the start process
			p.ObservedState.ServiceInfo.StatusReason = "retrying start: DFC now available"

			return p.baseFSMInstance.SendEvent(ctx, EventStartRetry), true
		}
		// Still no DFC available, stay in failed state
		p.ObservedState.ServiceInfo.StatusReason = "starting failed: " + reason

		return nil, false

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
func (p *ProtocolConverterInstance) reconcileRunningState(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		connectionUp, reasonConnection := p.IsConnectionUp()
		redpandaHealthy, reasonRedpanda := p.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := p.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := p.IsOtherDegraded()

		hasActivity, reasonActivity := p.IsDataflowComponentWithProcessingActivity()
		switch {
		case otherDegraded:
			p.ObservedState.ServiceInfo.StatusReason = "other degraded: " + reasonOtherDegraded
			if currentState != OperationalStateDegradedOther {
				return p.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true
			}

			return nil, false
		case !connectionUp:
			p.ObservedState.ServiceInfo.StatusReason = "connection degraded: " + reasonConnection
			if currentState != OperationalStateDegradedConnection {
				return p.baseFSMInstance.SendEvent(ctx, EventConnectionUnhealthy), true
			}

			return nil, false
		case !redpandaHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "redpanda degraded: " + reasonRedpanda
			if currentState != OperationalStateDegradedRedpanda {
				return p.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}

			return nil, false
		case !dfcHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "DFC degraded: " + reasonDFC
			if currentState != OperationalStateDegradedDFC {
				return p.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}

			return nil, false
		case !hasActivity: // if there is no activity, we move to Idle
			p.ObservedState.ServiceInfo.StatusReason = "idling: " + reasonActivity

			return p.baseFSMInstance.SendEvent(ctx, EventDFCIdle), true
		}

		p.ObservedState.ServiceInfo.StatusReason = "" // if everything is fine, reset the status reason

		return nil, false
	case OperationalStateIdle:
		// If we're in Idle, we need to check whether it is degraded
		// If we're in Active, we need to check whether it is degraded
		connectionUp, reasonConnection := p.IsConnectionUp()
		redpandaHealthy, reasonRedpanda := p.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := p.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := p.IsOtherDegraded()

		hasActivity, reasonActivity := p.IsDataflowComponentWithProcessingActivity()
		switch {
		case otherDegraded:
			p.ObservedState.ServiceInfo.StatusReason = "other degraded: " + reasonOtherDegraded
			if currentState != OperationalStateDegradedOther {
				return p.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true
			}

			return nil, false
		case !connectionUp:
			p.ObservedState.ServiceInfo.StatusReason = "connection degraded: " + reasonConnection
			if currentState != OperationalStateDegradedConnection {
				return p.baseFSMInstance.SendEvent(ctx, EventConnectionUnhealthy), true
			}

			return nil, false
		case !redpandaHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "redpanda degraded: " + reasonRedpanda
			if currentState != OperationalStateDegradedRedpanda {
				return p.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}

			return nil, false
		case !dfcHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "DFC degraded: " + reasonDFC
			if currentState != OperationalStateDegradedDFC {
				return p.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}

			return nil, false
		case !hasActivity: // if there is no activity, we stay in idle
			p.ObservedState.ServiceInfo.StatusReason = "idling: " + reasonActivity

			return nil, false
		}

		p.ObservedState.ServiceInfo.StatusReason = "active" // if everything is fine, reset the status reason

		return p.baseFSMInstance.SendEvent(ctx, EventDFCActive), true
	case OperationalStateDegradedConnection,
		OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther:
		// If we're in Degraded, we need to recover to move to Idle
		connectionUp, reasonConnection := p.IsConnectionUp()
		redpandaHealthy, reasonRedpanda := p.IsRedpandaHealthy()
		dfcHealthy, reasonDFC := p.IsDFCHealthy()
		otherDegraded, reasonOtherDegraded := p.IsOtherDegraded()

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

		switch {
		case otherDegraded:
			p.ObservedState.ServiceInfo.StatusReason = "other degraded: " + reasonOtherDegraded // Always set status reason
			if currentState != OperationalStateDegradedOther {
				return p.baseFSMInstance.SendEvent(ctx, EventDegradedOther), true // Send event for NEW degraded issue
			}

			return nil, false // Stay in current degraded state (same issue persists)
		case !connectionUp:
			p.ObservedState.ServiceInfo.StatusReason = "connection degraded: " + reasonConnection
			if currentState != OperationalStateDegradedConnection {
				return p.baseFSMInstance.SendEvent(ctx, EventConnectionUnhealthy), true
			}

			return nil, false
		case !redpandaHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "redpanda degraded: " + reasonRedpanda
			if currentState != OperationalStateDegradedRedpanda {
				return p.baseFSMInstance.SendEvent(ctx, EventRedpandaDegraded), true
			}

			return nil, false
		case !dfcHealthy:
			p.ObservedState.ServiceInfo.StatusReason = "DFC degraded: " + reasonDFC
			if currentState != OperationalStateDegradedDFC {
				return p.baseFSMInstance.SendEvent(ctx, EventDFCDegraded), true
			}

			return nil, false
		}

		// If we reach here, all issues are resolved - recover to Idle
		p.ObservedState.ServiceInfo.StatusReason = "recovering"

		return p.baseFSMInstance.SendEvent(ctx, EventRecovered), true
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stoppep.
// It deals with moving from any operational state to Stopping and then to Stoppep.
func (p *ProtocolConverterInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do more
		p.ObservedState.ServiceInfo.StatusReason = "stopped"

		return nil, false
	case OperationalStateStopping:
		stopped, reason := p.IsProtocolConverterStopped()
		if stopped {
			// Transition from Stopping to Stopped
			p.ObservedState.ServiceInfo.StatusReason = "stopped: " + reason

			return p.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}

		p.ObservedState.ServiceInfo.StatusReason = "stopping"

		return nil, false
	default:
		if err := p.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		p.ObservedState.ServiceInfo.StatusReason = "stopping"

		return p.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
