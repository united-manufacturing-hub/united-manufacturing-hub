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

package nmap

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the NmapInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (n *NmapInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	instanceName := n.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, instanceName, time.Since(start))
		if err != nil {
			n.baseFSMInstance.GetLogger().Errorf("error reconciling nmap instance %s: %s", instanceName, err)
			n.PrintState()
			// Add metrics for error
			metrics.IncErrorCountAndLog(metrics.ComponentNmapInstance, instanceName, err, n.baseFSMInstance.GetLogger())
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		if n.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "start of reconciliation") {
			return nil, false
		}
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if n.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		backErr := n.baseFSMInstance.GetBackoffError(snapshot.Tick)
		if backoff.IsPermanentFailureError(backErr) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return n.baseFSMInstance.HandlePermanentError(
				ctx,
				backErr,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return n.IsRemoved() || n.IsRemoving() || n.IsStopped() || n.IsStopping() || n.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return n.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal as a last resort when normal state transitions can't work
					// This directly removes files and resources
					return n.monitorService.ForceRemoveNmap(ctx, services.GetFileSystem(), instanceName)
				},
			)
		}
		n.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for nmap monitor %s: %v", instanceName, backErr)
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = n.reconcileExternalChanges(ctx, services, snapshot); err != nil {
		if n.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileExternalChanges") {
			return nil, false
		}

		// Log the error but always continue reconciling - we need reconcileStateTransition to run
		// to restore services after restart, even if we can't read their status yet
		n.baseFSMInstance.GetLogger().Warnf("failed to update observed state (continuing reconciliation): %s", err)
		
		// For all other errors, just continue reconciling without setting backoff
		err = nil
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = n.reconcileStateTransition(ctx, services, snapshot.SnapshotTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		if n.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileStateTransition") {
			return nil, false
		}

		n.baseFSMInstance.SetError(err, snapshot.Tick)
		n.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the s6Manager
	s6Err, s6Reconciled := n.monitorService.ReconcileManager(ctx, services, snapshot.Tick)
	if s6Err != nil {
		if n.baseFSMInstance.IsDeadlineExceededAndHandle(s6Err, snapshot.Tick, "monitorService reconciliation") {
			return nil, false
		}
		n.baseFSMInstance.SetError(s6Err, snapshot.Tick)
		n.baseFSMInstance.GetLogger().Errorf("error reconciling monitorService: %s", s6Err)
		return nil, false
	}

	// If either Nmap state or S6 state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || s6Reconciled

	// It went all right, so clear the error
	n.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the Nmap service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (n *NmapInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.NmapUpdateObservedStateTimeout)
	defer cancel()

	err := n.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (n *NmapInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := n.baseFSMInstance.GetCurrentFSMState()
	desiredState := n.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID(), currentState, desiredState)

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := n.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, n.CreateInstance, n.RemoveInstance, n.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := n.reconcileOperationalStates(ctx, currentState, desiredState, services, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles operational (i.e. start/stop and sub-state)
func (n *NmapInstance) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateOpen: // "running" – user wants active monitoring
		return n.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return n.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (n *NmapInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := n.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return n.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return n.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return n.reconcileRunningStates(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return n.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// Transition sequence for running state:
//
//	from any running state (open, filtered, closed, degraded) -> stopping (EventStop)
//	then from stopping -> stopped (EventStopped)
func (n *NmapInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	// If not yet stopped and not already stopping, stop instance.
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		if err := n.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, true
		}
		return n.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	// If currently stopping, verify that it is now fully stopped.
	if currentState == OperationalStateStopping {
		if !n.IsStopped() {
			return n.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
		return nil, false
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (n *NmapInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		return n.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (n *NmapInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateOpen:
		// If we're in open, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateFiltered:
		// If we're in filtered, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateClosed:
		// If we're in closed, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateUnfiltered:
		// If we're in closed, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateOpenFiltered:
		// If we're in closed, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateClosedFiltered:
		// If we're in closed, we need to check whether it is degraded
		if !n.isNmapHealthy(currentTime) {
			return n.baseFSMInstance.SendEvent(ctx, EventNmapExecutionFailed), true
		}

		// if we don't go into degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			// We don't set reconciled to true here, since it's intended for larger
			// actions to stop the control loop and wait for the next tick.
			// Switching between different port states should be considered as a
			// meaningless action here.
			return n.baseFSMInstance.SendEvent(ctx, event), false
		}

		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover with Nmap port state
		// if we don't stay in degraded state, check if the last reported state was
		// open/filtered/closed and then set it's state accordingly
		changedPortState, event := n.checkPortState(currentState)
		if changedPortState {
			return n.baseFSMInstance.SendEvent(ctx, event), true
		}

		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// isNmapHealthy decides if the nmap health is Active
func (n *NmapInstance) isNmapHealthy(currentTime time.Time) bool {
	status := n.ObservedState.ServiceInfo.NmapStatus

	// If there is no LastScan and no Logs we consider Nmap to be unhealthy
	if status.LastScan == nil && len(status.Logs) == 0 {
		return false
	}

	// If there occured any error in the LastScan during parseScanLogs
	if status.LastScan.Error != "" {
		return false
	}

	// Check that the last scan was not too long ago
	if currentTime.Sub(status.LastScan.Timestamp) > constants.NmapScanTimeout {
		return false
	}

	// Only consider nmap healthy if the overall health status is running
	return n.IsNmapRunning()
}

// checkPortState checks on the port state and if it differs to the currentState
// it returns true and the corresponding event
func (n *NmapInstance) checkPortState(currentState string) (bool, string) {
	if n.ObservedState.ServiceInfo.NmapStatus.LastScan == nil {
		return false, ""
	}

	portState := PortState(strings.ToLower(n.ObservedState.ServiceInfo.NmapStatus.LastScan.PortResult.State))

	switch portState {
	case PortStateOpen:
		if currentState == OperationalStateOpen {
			return false, ""
		}
		return true, EventPortOpen
	case PortStateFiltered:
		if currentState == OperationalStateFiltered {
			return false, ""
		}
		return true, EventPortFiltered
	case PortStateClosed:
		if currentState == OperationalStateClosed {
			return false, ""
		}
		return true, EventPortClosed
	case PortStateUnfiltered:
		if currentState == OperationalStateUnfiltered {
			return false, ""
		}
		return true, EventPortUnfiltered
	case PortStateOpenFiltered:
		if currentState == OperationalStateOpenFiltered {
			return false, ""
		}
		return true, EventPortOpenFiltered
	case PortStateClosedFiltered:
		if currentState == OperationalStateClosedFiltered {
			return false, ""
		}
		return true, EventPortClosedFiltered
	default:
		return false, ""
	}
}
