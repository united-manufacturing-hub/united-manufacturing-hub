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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// Reconcile examines the NmapInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (n *NmapInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	instanceName := n.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, instanceName, time.Since(start))
		if err != nil {
			n.baseFSMInstance.GetLogger().Errorf("error reconciling nmap instance %s: %s", instanceName, err)
			n.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentNmapInstance, instanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
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
					return n.monitorService.ForceRemoveNmap(ctx, filesystemService, instanceName)
				},
			)
		}
		n.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for nmap monitor %s: %v", instanceName, backErr)
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = n.reconcileExternalChanges(ctx, filesystemService, snapshot.Tick, start); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, nmap_service.ErrServiceNotExist) {
			n.baseFSMInstance.SetError(err, snapshot.Tick)
			n.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)

			// We want to return the error here, to stop reconciling since this would
			// lead the fsm going into degraded. But Nmap-scans are triggered once per second
			// and each tick is 100ms, therefore it could fail, because it doesn't get
			// complete logs from which it parses.
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}
		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition}
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now()
	err, reconciled = n.reconcileStateTransition(ctx, filesystemService, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, fsm.ErrInstanceRemoved) {
			return nil, false
		}

		n.baseFSMInstance.SetError(err, snapshot.Tick)
		n.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the s6Manager
	s6Err, s6Reconciled := n.monitorService.ReconcileManager(ctx, filesystemService, snapshot.Tick)
	if s6Err != nil {
		n.baseFSMInstance.SetError(s6Err, snapshot.Tick)
		n.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
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
func (n *NmapInstance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	observedStateCtx, cancel := context.WithTimeout(ctx, constants.S6UpdateObservedStateTimeout)
	defer cancel()
	err := n.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick, loopStartTime)
	if err != nil {
		if errors.Is(err, nmap_service.ErrScanFailed) {
			return err
		}
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (n *NmapInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := n.baseFSMInstance.GetCurrentFSMState()
	desiredState := n.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := n.reconcileLifecycleStates(ctx, filesystemService, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := n.reconcileOperationalStates(ctx, currentState, desiredState, filesystemService, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles to_be_created, creating, removing, removed
func (n *NmapInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		// do creation
		if err := n.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return n.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true

	case internal_fsm.LifecycleStateCreating:
		// We can assume creation is done immediately (no real action)
		return n.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true

	case internal_fsm.LifecycleStateRemoving:
		if err := n.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return n.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true

	case internal_fsm.LifecycleStateRemoved:
		// The manager will clean this up eventually
		return fsm.ErrInstanceRemoved, true

	default:
		return nil, false
	}
}

// reconcileOperationalStates handles operational (i.e. start/stop and sub-state)
func (n *NmapInstance) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateOpen: // "running" – user wants active monitoring
		return n.reconcileTransitionToActive(ctx, currentState, filesystemService, currentTime)
	case OperationalStateStopped:
		return n.reconcileTransitionToStopped(ctx, currentState, filesystemService)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (n *NmapInstance) reconcileTransitionToActive(ctx context.Context, currentState string, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		// Attempt to start instance
		if err := n.StartInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return n.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return n.reconcileStartingStates(ctx, filesystemService, currentState, currentTime)
	case IsRunningState(currentState):
		return n.reconcileRunningStates(ctx, filesystemService, currentState, currentTime)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// Transition sequence for running state:
//
//	from any running state (open, filtered, closed, degraded) -> stopping (EventStop)
//	then from stopping -> stopped (EventStopped)
func (n *NmapInstance) reconcileTransitionToStopped(ctx context.Context, currentState string, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	// If not yet stopped and not already stopping, stop instance.
	if currentState != OperationalStateStopped && currentState != OperationalStateStopping {
		if err := n.StopInstance(ctx, filesystemService); err != nil {
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
func (n *NmapInstance) reconcileStartingStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
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
func (n *NmapInstance) reconcileRunningStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateOpen:
		// If we're in open, we need to check whether it is degraded
		if !n.isNmapHealthy() {
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
		if !n.isNmapHealthy() {
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
		if !n.isNmapHealthy() {
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
		if !n.isNmapHealthy() {
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
		if !n.isNmapHealthy() {
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
		if !n.isNmapHealthy() {
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
func (n *NmapInstance) isNmapHealthy() bool {
	status := n.ObservedState.ServiceInfo.NmapStatus

	// If there is no LastScan and no Logs we consider Nmap to be unhealthy
	if status.LastScan == nil && len(status.Logs) == 0 {
		return false
	}

	// If there occured any error in the LastScan during parseScanLogs
	if status.LastScan.Error != "" {
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
