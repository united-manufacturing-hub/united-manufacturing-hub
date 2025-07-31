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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile periodically checks if the FSM needs state transitions based on metrics
// The services parameter provides access to core services including filesystem operations during reconciliation,
// enabling the method to read configuration or state information from the filesystem.
// Currently not used in this implementation but added for consistency with the interface.
func (c *ContainerInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	instanceName := c.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, instanceName, time.Since(start))
		if err != nil {
			c.baseFSMInstance.GetLogger().Errorf("error reconciling container instance %s: %s", instanceName, err)
			c.PrintState()
			// Add metrics for error
			metrics.IncErrorCountAndLog(metrics.ComponentContainerMonitor, instanceName, err, c.baseFSMInstance.GetLogger())
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		if c.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "start of reconciliation") {
			return nil, false
		}
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if c.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		backErr := c.baseFSMInstance.GetBackoffError(snapshot.Tick)
		if backoff.IsPermanentFailureError(backErr) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return c.baseFSMInstance.HandlePermanentError(
				ctx,
				backErr,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return c.IsRemoved() || c.IsRemoving() || c.IsStopping() || c.IsStopped() || c.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return c.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Container implementation doesn't have a ForceRemove method
					// Instead, we signal permanent failure so the manager can clean it up
					// This acts as the last resort when normal removal isn't possible
					return fmt.Errorf("%s : %w", backoff.PermanentFailureError, backErr)
				},
			)
		}
		c.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for container monitor %s: %v", instanceName, backErr)
		return nil, false
	}

	// Step 2: Detect external changes
	if err = c.reconcileExternalChanges(ctx, services, snapshot); err != nil {
		if c.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileExternalChanges") {
			return nil, false
		}

		// For other errors, set the error for backoff
		c.baseFSMInstance.SetError(err, snapshot.Tick)
		return nil, false
	}

	// Print system state every 100 ticks
	if snapshot.Tick%100 == 0 {
		c.printSystemState(instanceName, snapshot.Tick)
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = c.reconcileStateTransition(ctx, services)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		// Also this should not
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		if c.baseFSMInstance.IsDeadlineExceededAndHandle(err, snapshot.Tick, "reconcileStateTransition") {
			return nil, false
		}

		c.baseFSMInstance.SetError(err, snapshot.Tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// It went all right, so clear the error
	c.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the ContainerInstance service status has changed
// externally and updates the observed state accordingly
func (c *ContainerInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.ContainerUpdateObservedStateTimeout)
	defer cancel()

	err := c.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

// printSystemState prints the full system state in a human-readable format
// TODO: move this into status reason as well to be shown centrally in the system snapshot logger
func (c *ContainerInstance) printSystemState(instanceName string, tick uint64) {
	logger := c.baseFSMInstance.GetLogger()
	status := c.ObservedState.ServiceInfo

	logger.Infof("======= Container Instance State: %s (tick: %d) =======", instanceName, tick)
	logger.Infof("FSM States: Current=%s, Desired=%s", c.baseFSMInstance.GetCurrentFSMState(), c.baseFSMInstance.GetDesiredFSMState())

	if status == nil {
		logger.Infof("Container Status: No data available")
	} else {
		logger.Infof("Health: Overall=%s, CPU=%s, Memory=%s, Disk=%s",
			healthCategoryToString(status.OverallHealth),
			healthCategoryToString(status.CPUHealth),
			healthCategoryToString(status.MemoryHealth),
			healthCategoryToString(status.DiskHealth))

		if status.CPU != nil {
			logger.Infof("CPU: Usage=%.2fm cores, Cores=%d", status.CPU.TotalUsageMCpu, status.CPU.CoreCount)
		}

		if status.Memory != nil {
			usedMB := float64(status.Memory.CGroupUsedBytes) / 1024 / 1024
			totalMB := float64(status.Memory.CGroupTotalBytes) / 1024 / 1024
			usagePercent := 0.0
			if status.Memory.CGroupTotalBytes > 0 {
				usagePercent = float64(status.Memory.CGroupUsedBytes) / float64(status.Memory.CGroupTotalBytes) * 100
			}
			logger.Infof("Memory: Used=%.2f MB, Total=%.2f MB, Usage=%.2f%%", usedMB, totalMB, usagePercent)
		}

		if status.Disk != nil {
			usedGB := float64(status.Disk.DataPartitionUsedBytes) / 1024 / 1024 / 1024
			totalGB := float64(status.Disk.DataPartitionTotalBytes) / 1024 / 1024 / 1024
			usagePercent := 0.0
			if status.Disk.DataPartitionTotalBytes > 0 {
				usagePercent = float64(status.Disk.DataPartitionUsedBytes) / float64(status.Disk.DataPartitionTotalBytes) * 100
			}
			logger.Infof("Disk: Used=%.2f GB, Total=%.2f GB, Usage=%.2f%%", usedGB, totalGB, usagePercent)
		}

		logger.Infof("Architecture: %s, HWID: %s", status.Architecture, status.Hwid)
	}
	logger.Infof("=================================================")
}

// healthCategoryToString converts a HealthCategory to a human-readable string
func healthCategoryToString(category models.HealthCategory) string {
	switch category {
	case models.Neutral:
		return "Neutral"
	case models.Active:
		return "Active"
	case models.Degraded:
		return "Degraded"
	default:
		return fmt.Sprintf("Unknown(%d)", category)
	}
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (c *ContainerInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	// Report current and desired state metrics
	metrics.UpdateServiceState(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID(), currentState, desiredState)

	// If already in the desired state, nothing to do.
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := c.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, c.CreateInstance, c.RemoveInstance, c.CheckForCreation)
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
		err, reconciled := c.reconcileOperationalStates(ctx, services, currentState, desiredState, time.Now())
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

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (c *ContainerInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return c.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return c.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (c *ContainerInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := c.StartInstance(ctx, services)
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

// reconcileStartingStates handles the various starting phase states when transitioning to a running state
// no big startup process here
func (c *ContainerInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:

		// nothing to verify here, just for consistency with other fsms
		return c.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (c *ContainerInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if !c.areAllMetricsHealthy() {
			return c.baseFSMInstance.SendEvent(ctx, EventMetricsNotOK), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Active
		if c.areAllMetricsHealthy() {
			return c.baseFSMInstance.SendEvent(ctx, EventMetricsAllOK), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (c *ContainerInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentContainerMonitor, c.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do
		return nil, false
	case OperationalStateStopping:
		// If already stopping, verify if the instance is completely stopped
		// no verification, always go to stopped
		// Unlike other FSMs, we don't need to verify the stopping state for container monitoring
		// because there's no external service or process that needs to be checked - we can
		// immediately transition to stopped state
		return c.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	default:
		// For any other state, initiate stop
		err := c.StopInstance(ctx, services)
		if err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return c.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
