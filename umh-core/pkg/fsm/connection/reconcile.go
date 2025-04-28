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

package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Reconcile examines the ConnectionInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (c *ConnectionInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	connectionInstanceName := c.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, connectionInstanceName, time.Since(start))
		if err != nil {
			c.baseFSMInstance.GetLogger().Errorf("error reconciling connection instance %s: %v", connectionInstanceName, err)
			c.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentConnectionInstance, connectionInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if c.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := c.baseFSMInstance.GetBackoffError(snapshot.Tick)
		c.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Connection pipeline %s: %v", connectionInstanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return c.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return c.IsRemoved() || c.IsRemoving() || c.IsStopping() || c.IsStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return c.RemoveInstance(ctx, filesystemService)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return c.service.ForceRemoveConnection(ctx, filesystemService, connectionInstanceName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = c.reconcileExternalChanges(ctx, filesystemService, snapshot.Tick); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, connection.ErrServiceNotExist) {
			c.baseFSMInstance.SetError(err, snapshot.Tick)
			c.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)

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
	err, reconciled = c.reconcileStateTransition(ctx, filesystemService, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, fsm.ErrInstanceRemoved) {
			return nil, false
		}

		c.baseFSMInstance.SetError(err, snapshot.Tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the benthosManager
	nmapErr, nmapReconciled := c.service.ReconcileManager(ctx, filesystemService, snapshot.Tick)
	if nmapErr != nil {
		c.baseFSMInstance.SetError(nmapErr, snapshot.Tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling nmapManager: %s", nmapErr)
		return nil, false
	}

	// If either Connection state or Nmap state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || nmapReconciled

	// It went all right, so clear the error
	c.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the ConnectionInstance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (c *ConnectionInstance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, constants.ConnectionUpdateObservedStateTimeout)
	defer cancel()

	err := c.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick, start)
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
func (c *ConnectionInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := c.reconcileLifecycleStates(ctx, filesystemService, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := c.reconcileOperationalStates(ctx, filesystemService, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (creating/removing)
func (c *ConnectionInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		if err := c.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Check if the service is created
		// For now, we'll assume it's created immediately after initiating creation
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		if err := c.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (c *ConnectionInstance) reconcileOperationalStates(ctx context.Context, filesystemService filesystem.Service, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateUp:
		return c.reconcileTransitionToActive(ctx, filesystemService, currentState, currentTime)
	case OperationalStateStopped:
		return c.reconcileTransitionToStopped(ctx, filesystemService, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (c *ConnectionInstance) reconcileTransitionToActive(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	// If we're stopped, we need to start first
	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := c.StartInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return c.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	// Handle starting phase states
	if IsStartingState(currentState) {
		return c.reconcileStartingStates(ctx, filesystemService, currentState, currentTime)
	} else if IsRunningState(currentState) {
		return c.reconcileRunningState(ctx, filesystemService, currentState, currentTime)
	}

	return nil, false
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (c *ConnectionInstance) reconcileStartingStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentDataflowComponentInstance, c.baseFSMInstance.GetID()+".reconcileStartingState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:
		// ensure that Nmap is in a running state before considering the Connection
		// FSM as started
		if c.IsConnectionNmapRunning() {
			return c.baseFSMInstance.SendEvent(ctx, EventStartDone), true
		}
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
	return nil, false
}

// reconcileRunningState handles the various running states when transitioning to Active.
func (c *ConnectionInstance) reconcileRunningState(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileRunningState", time.Since(start))
	}()

	switch currentState {
	case OperationalStateUp:
		// If we're in Up, we need to check whether it is degraded or flaky
		if c.IsConnectionDegraded() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeFlaky), true
		}

		if c.IsConnectionNmapDown() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeDown), true
		}

		return nil, false
	case OperationalStateDown:
		// If we're in Down, we need to check whether it is degraded or up
		if c.IsConnectionDegraded() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeFlaky), true
		}

		if c.IsConnectionNmapUp() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeUp), true
		}

		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Idle
		if c.IsConnectionNmapUp() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeUp), true
		}

		if c.IsConnectionNmapDown() {
			return c.baseFSMInstance.SendEvent(ctx, EventProbeDown), true
		}

		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (c *ConnectionInstance) reconcileTransitionToStopped(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do more
		return nil, false
	case OperationalStateStopping:
		if c.IsConnectionNmapStopped() {
			// Transition from Stopping to Stopped
			return c.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
	default:
		if err := c.StopInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return c.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	return nil, false
}
