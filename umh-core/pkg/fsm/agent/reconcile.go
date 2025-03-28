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

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/core"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// Reconcile implements the FSMInstance Reconcile method
// It moves the instance toward its desired state by checking the current status
// and performing the appropriate actions to reach the desired state
func (a *AgentInstance) Reconcile(ctx context.Context, tick uint64) (error, bool) {
	// Start timing the reconciliation process
	start := time.Now()
	agentInstanceName := a.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentInstance, agentInstanceName, time.Since(start))
	}()

	// Check for previous error conditions and skip if necessary
	if a.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		a.baseFSMInstance.GetLogger().Debugf("Skipping reconcile due to backoff")
		metrics.IncErrorCount(metrics.ComponentAgentInstance, agentInstanceName)
		return a.baseFSMInstance.GetError(), false
	}

	// Important: check parent core state first as it may override agent's desired state
	err, parentStateChanged := a.reconcileParentCoreState(ctx)
	if err != nil {
		a.baseFSMInstance.SetError(err, tick)
		metrics.IncErrorCount(metrics.ComponentAgentInstance, agentInstanceName)
		return err, parentStateChanged
	}

	// Current and desired state
	currentState := a.baseFSMInstance.GetCurrentFSMState()
	desiredState := a.baseFSMInstance.GetDesiredFSMState()

	changed := parentStateChanged

	// Handle LifecycleState transitions first if in a lifecycle state
	if IsLifecycleState(currentState) {
		err, stateChanged := a.reconcileLifecycleStates(ctx)
		if err != nil {
			a.baseFSMInstance.SetError(err, tick)
			metrics.IncErrorCount(metrics.ComponentAgentInstance, agentInstanceName)
			return err, stateChanged
		}
		changed = changed || stateChanged
	}

	// Then handle OperationalState transitions if in an operational state
	if IsOperationalState(currentState) {
		err, stateChanged := a.reconcileOperationalStates(ctx, desiredState)
		if err != nil {
			a.baseFSMInstance.SetError(err, tick)
			metrics.IncErrorCount(metrics.ComponentAgentInstance, agentInstanceName)
			return err, stateChanged
		}
		changed = changed || stateChanged
	}

	// If we've successfully reconciled with no errors, reset any error state
	a.baseFSMInstance.ResetState()
	return nil, changed
}

// reconcileParentCoreState checks if the parent core state requires agent state changes
func (a *AgentInstance) reconcileParentCoreState(ctx context.Context) (error, bool) {
	parentCore := a.parentCore
	if parentCore == nil {
		return fmt.Errorf("parent core is nil"), false
	}

	parentState := parentCore.GetCurrentFSMState()
	currentState := a.GetCurrentFSMState()
	a.baseFSMInstance.GetLogger().Debugf("Parent core state: %s, Agent state: %s", parentState, currentState)

	changed := false

	// If parent core is removed, agent should be removed too
	if parentCore.IsRemoved() && !a.IsRemoved() {
		if err := a.Remove(ctx); err != nil {
			return fmt.Errorf("failed to remove agent: %w", err), false
		}
		changed = true
	}

	// If parent core is not active, agent cannot be active
	if parentState != core.OperationalStateActive &&
		currentState != OperationalStateMonitoringStopped &&
		!IsLifecycleState(currentState) {
		// Set desired state to monitoring_stopped
		a.SetDesiredFSMState(OperationalStateMonitoringStopped)
		changed = true
	}

	// If parent core is degraded, agent might need to adjust (optional logic)
	if parentState == core.OperationalStateDegraded && currentState == OperationalStateActive {
		// Consider transitioning agent to degraded as well
		// This is just one approach - you might want different behavior
		err := a.updateAgentStatusBasedOnParentDegradation(ctx)
		if err != nil {
			return err, false
		}
		// Don't mark as changed since we didn't directly change state
	}

	// Trigger the check_parent_state callback to perform any additional logic
	if err := a.baseFSMInstance.SendEvent(ctx, "check_parent_state"); err != nil {
		// This isn't fatal - just log it
		a.baseFSMInstance.GetLogger().Warnf("Failed to trigger parent state check: %v", err)
	}

	return nil, changed
}

// updateAgentStatusBasedOnParentDegradation handles agent state when parent is degraded
// This is called when the parent core is in a degraded state
func (a *AgentInstance) updateAgentStatusBasedOnParentDegradation(ctx context.Context) error {
	// This is an example implementation - customize based on requirements
	// For instance, you might check specific conditions to determine if agent should be degraded

	// Example: Increment error count which might lead to degraded state in future reconciles
	a.ObservedState.ErrorCount++
	a.baseFSMInstance.GetLogger().Warnf("Parent core is degraded, incrementing agent error count: %d",
		a.ObservedState.ErrorCount)

	return nil
}

// reconcileLifecycleStates handles transitions between lifecycle states
func (a *AgentInstance) reconcileLifecycleStates(ctx context.Context) (error, bool) {
	currentState := a.baseFSMInstance.GetCurrentFSMState()
	a.baseFSMInstance.GetLogger().Debugf("Reconciling lifecycle state: %s", currentState)

	switch currentState {
	case LifecycleStateCreate:
		// Handle creation logic
		if err := a.baseFSMInstance.SendEvent(ctx, EventCreate); err != nil {
			return fmt.Errorf("failed to send create event: %w", err), false
		}
		return nil, true

	case LifecycleStateRemove:
		// Handle removal logic - cleanup any agent-specific resources

		// Notify the parent about agent removal (optional)
		if a.parentCore != nil {
			a.baseFSMInstance.GetLogger().Infof("Notifying parent core %s about agent %s removal",
				a.parentCore.GetCurrentFSMState(), a.agentID)
		}

		// Complete removal
		if err := a.baseFSMInstance.SendEvent(ctx, internalfsm.LifecycleEventRemoveDone); err != nil {
			return fmt.Errorf("failed to send remove done event: %w", err), false
		}
		return nil, true
	}

	return nil, false
}

// reconcileOperationalStates handles transitions between operational states
func (a *AgentInstance) reconcileOperationalStates(ctx context.Context, desiredState string) (error, bool) {
	currentState := a.baseFSMInstance.GetCurrentFSMState()
	a.baseFSMInstance.GetLogger().Debugf("Reconciling operational state: %s (desired: %s)", currentState, desiredState)

	// Check if desired state requires a state transition
	switch currentState {
	case OperationalStateMonitoringStopped:
		if desiredState == OperationalStateActive {
			// First verify parent core is in a state that allows this agent to be active
			if a.parentCore != nil && a.parentCore.GetCurrentFSMState() != core.OperationalStateActive {
				a.baseFSMInstance.GetLogger().Infof("Cannot transition to active: parent core not active")
				return nil, false
			}
			// Transition to active state
			return a.reconcileTransitionToActive(ctx)
		}

	case OperationalStateActive:
		// If we're in active state but desire is stopped, transition to stopped
		if desiredState == OperationalStateMonitoringStopped {
			return a.reconcileTransitionToStopped(ctx)
		}

		// Check if we need to transition to degraded
		if a.shouldTransitionToDegraded() {
			if err := a.baseFSMInstance.SendEvent(ctx, EventDegraded); err != nil {
				return fmt.Errorf("failed to send degraded event: %w", err), false
			}
			return nil, true
		}

	case OperationalStateDegraded:
		// If we're in degraded state but desire is stopped, transition to stopped
		if desiredState == OperationalStateMonitoringStopped {
			return a.reconcileTransitionToStopped(ctx)
		}

		// Check if we can recover back to active
		if a.canRecoverFromDegraded() {
			if err := a.baseFSMInstance.SendEvent(ctx, EventStart); err != nil {
				return fmt.Errorf("failed to send start event from degraded: %w", err), false
			}
			return nil, true
		}
	}

	return nil, false
}

// reconcileTransitionToActive handles the transition to an active state
func (a *AgentInstance) reconcileTransitionToActive(ctx context.Context) (error, bool) {
	// Start agent monitoring logic
	a.baseFSMInstance.GetLogger().Infof("Starting monitoring for agent %s", a.baseFSMInstance.GetID())

	// Start monitoring - implementation in actions.go
	if err := a.startMonitoring(); err != nil {
		return fmt.Errorf("failed to start agent monitoring: %w", err), false
	}

	// Send the event to transition to active state
	if err := a.baseFSMInstance.SendEvent(ctx, EventStart); err != nil {
		return fmt.Errorf("failed to send start event: %w", err), false
	}

	return nil, true
}

// reconcileTransitionToStopped handles the transition to a stopped state
func (a *AgentInstance) reconcileTransitionToStopped(ctx context.Context) (error, bool) {
	// Stop agent monitoring logic
	a.baseFSMInstance.GetLogger().Infof("Stopping monitoring for agent %s", a.baseFSMInstance.GetID())

	// Stop monitoring - implementation in actions.go
	if err := a.stopMonitoring(); err != nil {
		return fmt.Errorf("failed to stop agent monitoring: %w", err), false
	}

	// Send the event to transition to stopped state
	if err := a.baseFSMInstance.SendEvent(ctx, EventStop); err != nil {
		return fmt.Errorf("failed to send stop event: %w", err), false
	}

	return nil, true
}

// shouldTransitionToDegraded checks if the instance should transition to a degraded state
func (a *AgentInstance) shouldTransitionToDegraded() bool {
	// Implement criteria for degradation
	// For example, check error count thresholds
	return a.ObservedState.ErrorCount > 5 || !a.ObservedState.IsConnected
}

// canRecoverFromDegraded checks if the instance can recover from a degraded state
func (a *AgentInstance) canRecoverFromDegraded() bool {
	// Implement recovery criteria
	// For example, check if error count has decreased or issues resolved
	return a.ObservedState.ErrorCount <= 2 && a.ObservedState.IsConnected
}
