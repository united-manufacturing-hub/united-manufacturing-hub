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

package container

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Reconcile periodically checks if the FSM needs state transitions based on metrics
func (c *ContainerInstance) Reconcile(ctx context.Context, tick uint64) (err error, reconciled bool) {
	start := time.Now()
	instanceName := c.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, instanceName, time.Since(start))
		if err != nil {
			c.baseFSMInstance.GetLogger().Errorf("error reconciling container instance %s: %s", instanceName, err)
			c.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentContainerMonitor, instanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// 2) Check if we should skip due to a recent error with backoff
	if c.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		backErr := c.baseFSMInstance.GetBackoffError(tick)
		if backoff.IsPermanentFailureError(backErr) {
			// If permanent, we want to remove the instance or at least stop it
			// For now, let's just remove it from the manager:
			if c.IsRemoved() || c.IsRemoving() || c.IsStopped() {
				c.baseFSMInstance.GetLogger().Errorf("Permanent error on container monitor %s but it is already in a terminal/removing state", instanceName)
				return backErr, false
			} else {
				c.baseFSMInstance.GetLogger().Errorf("Permanent error on container monitor %s => removing it", instanceName)
				c.baseFSMInstance.ResetState() // clear the error
				_ = c.Remove(ctx)              // attempt removal
				return nil, false
			}
		}
		c.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for container monitor %s: %v", instanceName, backErr)
		return nil, false
	}

	// 2) Update observed state (i.e., fetch container metrics) with a timeout
	updateCtx, cancel := context.WithTimeout(ctx, constants.ContainerMonitorUpdateObservedStateTimeout)
	defer cancel()

	if err := c.updateObservedState(updateCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Updating the observed state can sometimes take longer,
			// resulting in context.DeadlineExceeded errors. In this case, we want to
			// mark the reconciliation as complete for this tick since we've likely
			// already consumed significant time.
			c.baseFSMInstance.GetLogger().Warnf("Timeout while updating observed state for container instance %s", instanceName)
			return nil, true
		}

		// For other errors, set the error for backoff
		c.baseFSMInstance.SetError(err, tick)
		return nil, false
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = c.reconcileStateTransition(ctx)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		// Also this should not
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

		c.baseFSMInstance.SetError(err, tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// It went all right, so clear the error
	c.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (c *ContainerInstance) reconcileStateTransition(ctx context.Context) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, c.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do.
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := c.reconcileLifecycleStates(ctx, currentState)
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
		err, reconciled := c.reconcileOperationalStates(ctx, currentState, desiredState)
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

// updateObservedState queries container_monitor.Service for new metrics
func (c *ContainerInstance) updateObservedState(ctx context.Context) error {
	status, err := c.monitorService.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container metrics: %w", err)
	}
	// Save to observed state
	c.ObservedState.ContainerStatus = status
	return nil
}

// reconcileLifecycleStates handles to_be_created, creating, removing, removed
func (c *ContainerInstance) reconcileLifecycleStates(ctx context.Context, currentState string) (error, bool) {
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		// do creation
		if err := c.initiateContainerCreate(ctx); err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true

	case internal_fsm.LifecycleStateCreating:
		// We can assume creation is done immediately (no real action)
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true

	case internal_fsm.LifecycleStateRemoving:
		if err := c.initiateContainerRemove(ctx); err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true

	case internal_fsm.LifecycleStateRemoved:
		// The manager will clean this up eventually
		return fsm.ErrInstanceRemoved, true

	default:
		return nil, false
	}
}

// reconcileOperationalStates checks the desired state (active or stopped) and the observed metrics
func (c *ContainerInstance) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string) (error, bool) {
	current := c.GetCurrentFSMState()
	desired := c.GetDesiredFSMState()

	// 1) If desired is "stopped" and we are not "monitoring_stopped", we should eventStop
	if desired == MonitoringStateStopped && current != MonitoringStateStopped {
		err := c.disableMonitoring(ctx)
		if err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, EventStop), false // it is inconsitent with the control fsms, but as we actually not do anything here we can save some ticks and allow other fsms to reconcile after us
	}

	// 2) If desired is "active" and we are "monitoring_stopped", we should do eventStart -> goes to degraded
	if desired == MonitoringStateActive && current == MonitoringStateStopped {
		err := c.enableMonitoring(ctx)
		if err != nil {
			return err, false
		}
		return c.baseFSMInstance.SendEvent(ctx, EventStart), false // it is inconsitent with the control fsms, but as we actually not do anything here we can save some ticks and allow other fsms to reconcile after us
	}

	// 3) If we are in "degraded" or "active" (i.e. running monitoring), check metrics
	if current == MonitoringStateDegraded || current == MonitoringStateActive {
		// Evaluate the container metrics from c.ObservedState
		if c.areAllMetricsHealthy() {
			// If currently degraded, we go to active
			if current == MonitoringStateDegraded {
				return c.baseFSMInstance.SendEvent(ctx, EventMetricsAllOK), false // it is inconsitent with the control fsms, but as we actually not do anything here we can save some ticks and allow other fsms to reconcile after us
			}
		} else {
			// If currently active, we degrade
			if current == MonitoringStateActive {
				return c.baseFSMInstance.SendEvent(ctx, EventMetricsNotOK), false // it is inconsitent with the control fsms, but as we actually not do anything here we can save some ticks and allow other fsms to reconcile after us
			}
		}
	}

	// no changes
	return nil, false
}

// areAllMetricsHealthy decides if the container health is Active
func (c *ContainerInstance) areAllMetricsHealthy() bool {
	status := c.ObservedState.ContainerStatus
	if status == nil {
		// If we have no data, let's consider it not healthy
		return false
	}

	// Only consider container healthy if the overall health category is Active
	return status.OverallHealth == models.Active
}
