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
	"errors"
	"fmt"
	"time"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	agentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// Reconcile examines the AgentInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (a *AgentInstance) Reconcile(ctx context.Context, tick uint64) (err error, reconciled bool) {
	start := time.Now()
	agentInstanceName := a.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, agentInstanceName, time.Since(start))
		if err != nil {
			a.baseFSMInstance.GetLogger().Errorf("error reconciling Agent instance %s: %s", a.baseFSMInstance.GetID(), err)
			a.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentAgentInstance, agentInstanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if a.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		err := a.baseFSMInstance.GetBackoffError(tick)
		a.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Agent %s: %s", a.baseFSMInstance.GetID(), err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// if it is already in stopped, stopping, removing states, and it again returns a permanent error,
			// we need to throw it to the manager as the instance itself here cannot fix it anymore
			if a.IsRemoved() || a.IsMonitoringStopped() {
				a.baseFSMInstance.GetLogger().Errorf("Agent instance %s is already in a terminal state, force removing it", a.baseFSMInstance.GetID())
				// force remove the agent
				a.service.Remove(ctx, a.agentID)
				return err, false
			} else {
				a.baseFSMInstance.GetLogger().Errorf("Agent instance %s is not in a terminal state, resetting state and removing it", a.baseFSMInstance.GetID())
				a.baseFSMInstance.ResetState()
				a.Remove(ctx)
				return nil, false // let's try to at least reconcile towards a stopped / removed state
			}
		}

		return nil, false
	}

	// Step 2: Detect external changes.
	if err := a.reconcileExternalChanges(ctx); err != nil {
		// If the agent is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, agentservice.ErrAgentNotExist) {
			a.baseFSMInstance.SetError(err, tick)
			a.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		err = nil // The agent does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = a.reconcileStateTransition(ctx)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, fsm.ErrInstanceRemoved) {
			return nil, false
		}

		if errors.Is(err, context.DeadlineExceeded) {
			// Updating the observed state can sometimes take longer,
			// resulting in context.DeadlineExceeded errors. In this case, we want to
			// mark the reconciliation as complete for this tick since we've likely
			// already consumed significant time. We return reconciled=true to prevent
			// further reconciliation attempts in the current tick.
			return nil, true // We don't want to return an error here, as this can happen in normal operations
		}

		a.baseFSMInstance.SetError(err, tick)
		a.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// It went all right, so clear the error
	a.baseFSMInstance.ResetState()

	return err, reconciled
}

// reconcileExternalChanges checks if the AgentInstance status has changed
// externally (e.g., if an agent has disconnected, or if it's reporting errors)
func (a *AgentInstance) reconcileExternalChanges(ctx context.Context) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	observedStateCtx, cancel := context.WithTimeout(ctx, constants.S6UpdateObservedStateTimeout)
	defer cancel()
	err := a.updateObservedState(observedStateCtx)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}

	// Check if parent core state has changed
	if a.parentCore != nil {
		if a.parentCore.IsMonitoringStopped() && !a.IsMonitoringStopped() {
			// If parent core is stopped, agent should also be stopped
			a.baseFSMInstance.GetLogger().Infof("Parent core is monitoring_stopped, stopping agent %s", a.baseFSMInstance.GetID())
			a.SetDesiredFSMState(OperationalStateMonitoringStopped)
		}

		if a.parentCore.IsRemoved() && !a.IsRemoved() {
			// If parent core is removed, agent should also be removed
			a.baseFSMInstance.GetLogger().Infof("Parent core is removed, removing agent %s", a.baseFSMInstance.GetID())
			a.Remove(ctx)
		}
	}

	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
func (a *AgentInstance) reconcileStateTransition(ctx context.Context) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := a.baseFSMInstance.GetCurrentFSMState()
	desiredState := a.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do.
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := a.reconcileLifecycleStates(ctx, currentState)
		if err != nil {
			return err, false
		}
		if reconciled {
			return nil, true
		} else {
			return nil, false
		}
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := a.reconcileOperationalStates(ctx, currentState, desiredState)
		if err != nil {
			return err, false
		}
		if reconciled {
			return nil, true
		} else {
			return nil, false
		}
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (creating/removing)
func (a *AgentInstance) reconcileLifecycleStates(ctx context.Context, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		// Nothing to create for an agent, it's just a virtual entity
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Nothing to wait for when creating an agent
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		if err := a.initiateAgentRemove(ctx); err != nil {
			return err, true
		}
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (monitoring/stopping)
func (a *AgentInstance) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return a.reconcileTransitionToActive(ctx, currentState)
	case OperationalStateMonitoringStopped:
		return a.reconcileTransitionToMonitoringStopped(ctx, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false // its simply an error, but we did not take any action
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from MonitoringStopped to Active.
func (a *AgentInstance) reconcileTransitionToActive(ctx context.Context, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	if currentState == OperationalStateMonitoringStopped {
		// Attempt to initiate start
		if err := a.initiateAgentStart(ctx); err != nil {
			return err, true
		}
		// Send event to transition from MonitoringStopped to Active
		return a.baseFSMInstance.SendEvent(ctx, EventStart), true
	} else if currentState == OperationalStateDegraded {
		// From degraded, we can recover to active
		// In a real implementation, we might need additional recovery logic here
		return a.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	return fmt.Errorf("cannot transition from %s to active", currentState), false
}

// reconcileTransitionToMonitoringStopped handles transitions when the desired state is MonitoringStopped.
// It deals with moving from Active/Degraded to MonitoringStopped.
func (a *AgentInstance) reconcileTransitionToMonitoringStopped(ctx context.Context, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, a.baseFSMInstance.GetID()+".reconcileTransitionToMonitoringStopped", time.Since(start))
	}()

	if currentState == OperationalStateActive || currentState == OperationalStateDegraded {
		// Attempt to initiate stop
		if err := a.initiateAgentStop(ctx); err != nil {
			return err, true
		}
		// Send event to transition to MonitoringStopped
		return a.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	return fmt.Errorf("cannot transition from %s to monitoring_stopped", currentState), false
}
