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
	connectionsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
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
func (c *ConnectionInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
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
					// Use Remove() instead of RemoveInstance() to ensure proper FSM state management.
					// Remove() triggers FSM state transitions via baseFSMInstance.Remove(),
					// while RemoveInstance() bypasses FSM and directly performs file operations.
					return c.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return c.service.ForceRemoveConnection(ctx, services.GetFileSystem(), connectionInstanceName)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = c.reconcileExternalChanges(ctx, services, snapshot); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, connectionsvc.ErrServiceNotExist) {

			if errors.Is(err, context.DeadlineExceeded) {
				// Healthchecks occasionally take longer (sometimes up to 70ms),
				// resulting in context.DeadlineExceeded errors. In this case, we want to
				// mark the reconciliation as complete for this tick since we've likely
				// already consumed significant time. We return reconciled=true to prevent
				// further reconciliation attempts in the current tick.
				return nil, true // We don't want to return an error here, as this can happen in normal operations
			}
			c.baseFSMInstance.SetError(err, snapshot.Tick)
			c.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	currentTime := time.Now() // this is used to check if the instance is degraded and for the log check
	err, reconciled = c.reconcileStateTransition(ctx, services, currentTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		c.baseFSMInstance.SetError(err, snapshot.Tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// Reconcile the benthosManager
	nmapErr, nmapReconciled := c.service.ReconcileManager(ctx, services, snapshot.Tick)
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
func (c *ConnectionInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.ConnectionUpdateObservedStateTimeout)
	defer cancel()

	err := c.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
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
func (c *ConnectionInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled = c.baseFSMInstance.ReconcileLifecycleStates(
			ctx,
			services,
			currentState,
			c.CreateInstance,
			c.RemoveInstance,
			c.CheckForCreation)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled = c.reconcileOperationalStates(ctx, services, currentState, desiredState, currentTime)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (c *ConnectionInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateUp:
		return c.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return c.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (c *ConnectionInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := c.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return c.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return c.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return c.reconcileRunningStates(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return c.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to Active.
func (c *ConnectionInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
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

// reconcileRunningStates handles the various running states when transitioning to Active.
func (c *ConnectionInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
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
func (c *ConnectionInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
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
		if err := c.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return c.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	return nil, false
}
