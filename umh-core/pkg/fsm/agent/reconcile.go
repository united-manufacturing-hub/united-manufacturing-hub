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

package agent

import (
	"context"
	"fmt"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// Constants for agent FSM
const (
	// AgentUpdateObservedStateTimeout defines the timeout for updating observed state
	AgentUpdateObservedStateTimeout = time.Millisecond * 5
)

// Reconcile examines the AgentInstance and, in three steps:
//  1. Check if a previous transition failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
func (a *AgentInstance) Reconcile(ctx context.Context, tick uint64) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "reconcile", time.Since(start))
		if err != nil {
			a.baseFSMInstance.GetLogger().Errorf("Error reconciling agent: %v", err)
			a.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(ComponentAgentInstance, "agent")
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if a.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		err := a.baseFSMInstance.GetBackoffError(tick)
		a.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for agent: %v", err)

		// if it is a permanent error, start the removal process and reset the error
		if backoff.IsPermanentFailureError(err) {
			// If already in a terminal state with permanent error, force remove
			if a.IsRemoved() || a.IsRemoving() || a.IsMonitoringStopped() {
				a.baseFSMInstance.GetLogger().Errorf("Agent is already in a terminal state, force removing it")
				// In a real implementation, there might be additional cleanup here
				return err, false
			} else {
				a.baseFSMInstance.GetLogger().Errorf("Agent is not in a terminal state, resetting state and removing it")
				a.baseFSMInstance.ResetState()
				a.Remove(ctx)
				return nil, false // Try to reconcile towards a monitoring_stopped/removed state
			}
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err := a.reconcileExternalChanges(ctx, tick); err != nil {
		a.baseFSMInstance.SetError(err, tick)
		a.baseFSMInstance.GetLogger().Errorf("Error reconciling external changes: %s", err)
		return nil, false // Continue reconciling
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = a.reconcileStateTransition(ctx)
	if err != nil {
		// If the instance is removed, we don't want to return an error
		if err == fsm.ErrInstanceRemoved {
			return nil, false
		}

		a.baseFSMInstance.SetError(err, tick)
		a.baseFSMInstance.GetLogger().Errorf("Error reconciling state: %s", err)
		return nil, false
	}

	// Everything went well, reset any errors
	a.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the AgentInstance service status has changed
// externally and updates the observed state.
func (a *AgentInstance) reconcileExternalChanges(ctx context.Context, tick uint64) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "reconcileExternalChanges", time.Since(start))
	}()

	// Create a timeout context to prevent blocking for too long
	observedStateCtx, cancel := context.WithTimeout(ctx, AgentUpdateObservedStateTimeout)
	defer cancel()

	// First update agent's observed state and check for config changes
	configErr := a.updateAgentObservedState(observedStateCtx, tick)
	if configErr != nil {
		// Log the error for observability
		a.baseFSMInstance.GetLogger().Errorf("Failed to update agent observed state: %v", configErr)

		// If we're in active state and hit config errors, transition to degraded
		currentState := a.baseFSMInstance.GetCurrentFSMState()
		if currentState == OperationalStateActive {
			a.baseFSMInstance.GetLogger().Warnf("Transitioning agent to degraded state due to configuration errors")
			a.baseFSMInstance.SendEvent(ctx, EventDegrade)
			// We still return the error so the reconciliation loop can handle it with backoff
			return fmt.Errorf("configuration error triggered degraded state: %w", configErr)
		}

		// Return the error for handling in the reconcile loop
		return configErr
	}

	// Now check if the agent health status has changed, AFTER updating observed state
	healthy, err := a.checkAgentHealth(ctx)
	if err != nil {
		return fmt.Errorf("failed to check agent health: %w", err)
	}

	// Handle health state transitions based on observed state
	currentState := a.baseFSMInstance.GetCurrentFSMState()

	// If we're active but not healthy, transition to degraded
	if currentState == OperationalStateActive && !healthy {
		a.baseFSMInstance.GetLogger().Infof("Detected agent degradation, will transition to degraded state")
		a.baseFSMInstance.SendEvent(ctx, EventDegrade)
		return nil
	}

	// If we're degraded but now healthy, transition to active
	if currentState == OperationalStateDegraded && healthy {
		a.baseFSMInstance.GetLogger().Infof("Detected agent recovery, will transition to active state")
		a.baseFSMInstance.SendEvent(ctx, EventRecover)
		return nil
	}

	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and sends events to drive the FSM from the current to the desired state.
func (a *AgentInstance) reconcileStateTransition(ctx context.Context) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "reconcileStateTransition", time.Since(start))
	}()

	currentState := a.baseFSMInstance.GetCurrentFSMState()
	desiredState := a.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) || currentState == LifecycleStateCreate || currentState == LifecycleStateRemove {
		err, reconciled := a.reconcileLifecycleStates(ctx, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if isOperationalState(currentState) {
		err, reconciled := a.reconcileOperationalStates(ctx, currentState, desiredState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (create/remove)
func (a *AgentInstance) reconcileLifecycleStates(ctx context.Context, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "reconcileLifecycleStates", time.Since(start))
	}()

	// Independent of the desired state, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated, LifecycleStateCreate:
		// In a real implementation, there might be creation logic here
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateCreating:
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving, LifecycleStateRemove:
		// In a real implementation, there might be removal logic here
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations
func (a *AgentInstance) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(ComponentAgentInstance, "reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return a.reconcileTransitionToActive(ctx, currentState)
	case OperationalStateMonitoringStopped:
		return a.reconcileTransitionToMonitoringStopped(ctx, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
func (a *AgentInstance) reconcileTransitionToActive(ctx context.Context, currentState string) (err error, reconciled bool) {
	switch currentState {
	case OperationalStateMonitoringStopped:
		// Directly activate monitoring from stopped state
		if err := a.initiateAgentActivation(ctx); err != nil {
			return err, false
		}
		return a.baseFSMInstance.SendEvent(ctx, EventStart), true
	case OperationalStateDegraded:
		// If we're in degraded state but desired is active, check health
		// This is actually handled in reconcileExternalChanges, but we include it here for completeness
		healthy, err := a.checkAgentHealth(ctx)
		if err != nil {
			return err, false
		}
		if healthy {
			return a.baseFSMInstance.SendEvent(ctx, EventRecover), true
		}
		// If not healthy, stay degraded for now
		return nil, false
	}

	return nil, false
}

// reconcileTransitionToMonitoringStopped handles transitions when the desired state is MonitoringStopped.
func (a *AgentInstance) reconcileTransitionToMonitoringStopped(ctx context.Context, currentState string) (err error, reconciled bool) {
	switch currentState {
	case OperationalStateActive, OperationalStateDegraded:
		// Stop monitoring from active or degraded state
		if err := a.initiateAgentMonitoringStopped(ctx); err != nil {
			return err, false
		}
		return a.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	return nil, false
}

// isOperationalState returns true if the given state is an operational state.
func isOperationalState(state string) bool {
	switch state {
	case OperationalStateMonitoringStopped, OperationalStateDegraded, OperationalStateActive:
		return true
	default:
		return false
	}
}
