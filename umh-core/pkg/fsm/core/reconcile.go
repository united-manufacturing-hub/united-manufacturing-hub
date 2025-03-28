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

package core

import (
	"context"
	"fmt"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// Reconcile implements the FSMInstance Reconcile method
// It moves the instance toward its desired state by checking the current status
// and performing the appropriate actions to reach the desired state
func (c *CoreInstance) Reconcile(ctx context.Context, tick uint64) (error, bool) {
	// Start timing the reconciliation process
	start := time.Now()
	coreInstanceName := c.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentCoreInstance, coreInstanceName, time.Since(start))
	}()

	// Check for previous error conditions and skip if necessary
	if c.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		c.baseFSMInstance.GetLogger().Debugf("Skipping reconcile due to backoff")
		metrics.IncErrorCount(metrics.ComponentCoreInstance, coreInstanceName)
		return c.baseFSMInstance.GetError(), false
	}

	// Current and desired state
	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	changed := false

	// Handle LifecycleState transitions first if in a lifecycle state
	if IsLifecycleState(currentState) {
		err, stateChanged := c.reconcileLifecycleStates(ctx)
		if err != nil {
			c.baseFSMInstance.SetError(err, tick)
			metrics.IncErrorCount(metrics.ComponentCoreInstance, coreInstanceName)
			return err, stateChanged
		}
		changed = changed || stateChanged
	}

	// Then handle OperationalState transitions if in an operational state
	if IsOperationalState(currentState) {
		err, stateChanged := c.reconcileOperationalStates(ctx, desiredState)
		if err != nil {
			c.baseFSMInstance.SetError(err, tick)
			metrics.IncErrorCount(metrics.ComponentCoreInstance, coreInstanceName)
			return err, stateChanged
		}
		changed = changed || stateChanged
	}

	// If we've successfully reconciled with no errors, reset any error state
	c.baseFSMInstance.ResetState()
	return nil, changed
}

// reconcileLifecycleStates handles transitions between lifecycle states
func (c *CoreInstance) reconcileLifecycleStates(ctx context.Context) (error, bool) {
	currentState := c.baseFSMInstance.GetCurrentFSMState()
	c.baseFSMInstance.GetLogger().Debugf("Reconciling lifecycle state: %s", currentState)

	switch currentState {
	case LifecycleStateCreate:
		// Handle creation logic
		if err := c.baseFSMInstance.SendEvent(ctx, EventCreate); err != nil {
			return fmt.Errorf("failed to send create event: %w", err), false
		}
		return nil, true

	case LifecycleStateRemove:
		// Handle removal logic
		// Perform any cleanup needed before removal

		// Since the state machine is in the remove state, we can indicate it's removed
		// by using the lifecycle event to transition to a removed state
		if err := c.baseFSMInstance.SendEvent(ctx, internalfsm.LifecycleEventRemoveDone); err != nil {
			return fmt.Errorf("failed to send remove done event: %w", err), false
		}
		return nil, true
	}

	return nil, false
}

// reconcileOperationalStates handles transitions between operational states
func (c *CoreInstance) reconcileOperationalStates(ctx context.Context, desiredState string) (error, bool) {
	currentState := c.baseFSMInstance.GetCurrentFSMState()
	c.baseFSMInstance.GetLogger().Debugf("Reconciling operational state: %s (desired: %s)", currentState, desiredState)

	// Check if desired state requires a state transition
	switch currentState {
	case OperationalStateMonitoringStopped:
		if desiredState == OperationalStateActive {
			// Transition to active state
			return c.reconcileTransitionToActive(ctx)
		}

	case OperationalStateActive:
		// If we're in active state but desire is stopped, transition to stopped
		if desiredState == OperationalStateMonitoringStopped {
			return c.reconcileTransitionToStopped(ctx)
		}

		// Check if we need to transition to degraded
		if c.shouldTransitionToDegraded() {
			if err := c.baseFSMInstance.SendEvent(ctx, EventDegraded); err != nil {
				return fmt.Errorf("failed to send degraded event: %w", err), false
			}
			return nil, true
		}

	case OperationalStateDegraded:
		// If we're in degraded state but desire is stopped, transition to stopped
		if desiredState == OperationalStateMonitoringStopped {
			return c.reconcileTransitionToStopped(ctx)
		}

		// Check if we can recover back to active
		if c.canRecoverFromDegraded() {
			if err := c.baseFSMInstance.SendEvent(ctx, EventStart); err != nil {
				return fmt.Errorf("failed to send start event from degraded: %w", err), false
			}
			return nil, true
		}
	}

	return nil, false
}

// reconcileTransitionToActive handles the transition to an active state
func (c *CoreInstance) reconcileTransitionToActive(ctx context.Context) (error, bool) {
	// Start monitoring logic
	c.baseFSMInstance.GetLogger().Infof("Starting monitoring for %s", c.baseFSMInstance.GetID())

	// Start monitoring
	if err := c.startMonitoring(); err != nil {
		return fmt.Errorf("failed to start monitoring: %w", err), false
	}

	// Send the event to transition to active state
	if err := c.baseFSMInstance.SendEvent(ctx, EventStart); err != nil {
		return fmt.Errorf("failed to send start event: %w", err), false
	}

	return nil, true
}

// reconcileTransitionToStopped handles the transition to a stopped state
func (c *CoreInstance) reconcileTransitionToStopped(ctx context.Context) (error, bool) {
	// Stop monitoring logic
	c.baseFSMInstance.GetLogger().Infof("Stopping monitoring for %s", c.baseFSMInstance.GetID())

	// Stop monitoring
	if err := c.stopMonitoring(); err != nil {
		return fmt.Errorf("failed to stop monitoring: %w", err), false
	}

	// Send the event to transition to stopped state
	if err := c.baseFSMInstance.SendEvent(ctx, EventStop); err != nil {
		return fmt.Errorf("failed to send stop event: %w", err), false
	}

	return nil, true
}

// shouldTransitionToDegraded checks if the instance should transition to a degraded state
func (c *CoreInstance) shouldTransitionToDegraded() bool {
	// Implement criteria for degradation
	// For example, check error count thresholds
	return c.ObservedState.ErrorCount > 5
}

// canRecoverFromDegraded checks if the instance can recover from a degraded state
func (c *CoreInstance) canRecoverFromDegraded() bool {
	// Implement recovery criteria
	// For example, check if error count has decreased or issues resolved
	return c.ObservedState.ErrorCount <= 2
}

// startMonitoring starts the monitoring process
func (c *CoreInstance) startMonitoring() error {
	// Implement actual monitoring logic
	// This might involve starting goroutines, setting up watches, etc.
	c.baseFSMInstance.GetLogger().Debugf("Starting monitoring for component %s", c.componentName)
	c.ObservedState.IsMonitoring = true
	return nil
}

// stopMonitoring stops the monitoring process
func (c *CoreInstance) stopMonitoring() error {
	// Implement actual monitoring termination logic
	// This might involve cancelling contexts, stopping goroutines, etc.
	c.baseFSMInstance.GetLogger().Debugf("Stopping monitoring for component %s", c.componentName)
	c.ObservedState.IsMonitoring = false
	return nil
}
