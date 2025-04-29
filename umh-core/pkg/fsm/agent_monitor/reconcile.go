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

package agent_monitor

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Reconcile periodically checks if the FSM needs state transitions based on metrics
// The filesystemService parameter allows for filesystem operations during reconciliation,
// enabling the method to read configuration or state information from the filesystem.
// Currently not used in this implementation but added for consistency with the interface.
func (a *AgentInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	instanceName := a.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, instanceName, time.Since(start))
		if err != nil {
			a.baseFSMInstance.GetLogger().Errorf("error reconciling agent instance %s: %s", instanceName, err)
			a.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentAgentMonitor, instanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if a.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		backErr := a.baseFSMInstance.GetBackoffError(snapshot.Tick)
		if backoff.IsPermanentFailureError(backErr) {
			// If permanent, we want to remove the instance or at least stop it
			// For now, let's just remove it from the manager:
			if a.IsRemoved() || a.IsRemoving() || a.IsStopping() || a.IsStopped() {
				a.baseFSMInstance.GetLogger().Errorf("Permanent error on agent monitor %s but it is already in a terminal/removing state", instanceName)
				return backErr, false
			} else {
				a.baseFSMInstance.GetLogger().Errorf("Permanent error on agent monitor %s => removing it", instanceName)
				a.baseFSMInstance.ResetState() // clear the error
				_ = a.Remove(ctx)              // attempt removal
				return nil, false
			}
		}
		a.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for agent monitor %s: %v", instanceName, backErr)
		return nil, false
	}

	// 2) Update observed state (i.e., fetch agent metrics) with a timeout
	updateCtx, cancel := context.WithTimeout(ctx, constants.AgentMonitorUpdateObservedStateTimeout)
	defer cancel()

	if err := a.updateObservedState(updateCtx, snapshot); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Updating the observed state can sometimes take longer,
			// resulting in context.DeadlineExceeded errors. In this case, we want to
			// mark the reconciliation as complete for this tick since we've likely
			// already consumed significant time.
			a.baseFSMInstance.GetLogger().Warnf("Timeout while updating observed state for agent instance %s", instanceName)
			return nil, true
		}

		// For other errors, set the error for backoff
		a.baseFSMInstance.SetError(err, snapshot.Tick)
		return nil, false
	}

	// Print system state every 10 ticks
	if snapshot.Tick%10 == 0 {
		a.printSystemState(instanceName, snapshot.Tick)
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = a.reconcileStateTransition(ctx, services.GetFileSystem())
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

		a.baseFSMInstance.SetError(err, snapshot.Tick)
		a.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// It went all right, so clear the error
	a.baseFSMInstance.ResetState()

	return nil, reconciled
}

// printSystemState prints the full system state in a human-readable format
func (a *AgentInstance) printSystemState(instanceName string, tick uint64) {
	logger := a.baseFSMInstance.GetLogger()
	status := a.ObservedState.ServiceInfo

	logger.Infof("======= Agent Instance State: %s (tick: %d) =======", instanceName, tick)
	logger.Infof("FSM States: Current=%s, Desired=%s", a.baseFSMInstance.GetCurrentFSMState(), a.baseFSMInstance.GetDesiredFSMState())

	if status == nil {
		logger.Infof("Agent Status: No data available")
	} else {
		logger.Infof("Health: Overall=%s, Latency=%s, Release=%s",
			healthCategoryToString(status.OverallHealth),
			healthCategoryToString(status.LatencyHealth),
			healthCategoryToString(status.ReleaseHealth))

		if status.Location != nil {
			logger.Infof("Location: %v", status.Location)
		}

		if status.Latency != nil {
			logger.Infof("Latency: %v", status.Latency)
		}

		if status.Release != nil {
			logger.Infof("Release: Channel=%s, Version=%s", status.Release.Channel, status.Release.Version)
			if len(status.Release.Versions) > 0 {
				logger.Infof("Component Versions: %v", status.Release.Versions)
			}
		}
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
func (a *AgentInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := a.baseFSMInstance.GetCurrentFSMState()
	desiredState := a.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do.
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := a.reconcileLifecycleStates(ctx, filesystemService, currentState)
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
		err, reconciled := a.reconcileOperationalStates(ctx, filesystemService, currentState, desiredState, time.Now())
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

// updateObservedState queries agent_monitor.Service for new metrics
func (a *AgentInstance) updateObservedState(ctx context.Context, snapshot fsm.SystemSnapshot) error {
	// get the config from the config manager
	status, err := a.monitorService.Status(ctx, snapshot) // TODO: where to get the config? it needs to be passed somehow from the manager
	if err != nil {
		return fmt.Errorf("failed to get agent metrics: %w", err)
	}
	// Save to observed state
	a.ObservedState.ServiceInfo = status
	return nil
}

// reconcileLifecycleStates handles to_be_created, creating, removing, removed
func (a *AgentInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (error, bool) {
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		// do creation
		if err := a.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true

	case internal_fsm.LifecycleStateCreating:
		// We can assume creation is done immediately (no real action)
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true

	case internal_fsm.LifecycleStateRemoving:
		if err := a.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return a.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true

	case internal_fsm.LifecycleStateRemoved:
		// The manager will clean this up eventually
		return fsm.ErrInstanceRemoved, true

	default:
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (a *AgentInstance) reconcileOperationalStates(ctx context.Context, filesystemService filesystem.Service, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return a.reconcileTransitionToActive(ctx, filesystemService, currentState, currentTime)
	case OperationalStateStopped:
		return a.reconcileTransitionToStopped(ctx, filesystemService, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (a *AgentInstance) reconcileTransitionToActive(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		// nothing to start here, just for consistency with other fsms
		err := a.StartInstance(ctx, filesystemService)
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return a.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return a.reconcileStartingStates(ctx, filesystemService, currentState, currentTime)
	case IsRunningState(currentState):
		return a.reconcileRunningStates(ctx, filesystemService, currentState, currentTime)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to a running state
// no big startup process here
func (a *AgentInstance) reconcileStartingStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:

		// nothing to verify here, just for consistency with other fsms
		return a.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (a *AgentInstance) reconcileRunningStates(ctx context.Context, filesystemService filesystem.Service, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if !a.areAllMetricsHealthy() {
			return a.baseFSMInstance.SendEvent(ctx, EventMetricsNotOK), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Active
		if a.areAllMetricsHealthy() {
			return a.baseFSMInstance.SendEvent(ctx, EventMetricsAllOK), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (a *AgentInstance) reconcileTransitionToStopped(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, a.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStopped:
		// Already stopped, nothing to do
		return nil, false
	case OperationalStateStopping:
		// If already stopping, verify if the instance is completely stopped
		// no verification, always go to stopped
		// Unlike other FSMs, we don't need to verify the stopping state for agent monitoring
		// because there's no external service or process that needs to be checked - we can
		// immediately transition to stopped state
		return a.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	default:
		// For any other state, initiate stop
		err := a.StopInstance(ctx, filesystemService)
		if err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return a.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
