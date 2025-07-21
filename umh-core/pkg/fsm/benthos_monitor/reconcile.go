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

package benthos_monitor

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
	benthos_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile periodically checks if the FSM needs state transitions based on metrics
// The filesystemService parameter allows for filesystem operations during reconciliation,
// enabling the method to read configuration or state information from the filesystem.
// Currently not used in this implementation but added for consistency with the interface.
func (b *BenthosMonitorInstance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	instanceName := b.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, instanceName, time.Since(start))
		if err != nil {
			b.baseFSMInstance.GetLogger().Errorf("error reconciling benthos monitor instance %s: %s", instanceName, err)
			b.PrintState()
			// Add metrics for error
			metrics.IncErrorCountAndLog(metrics.ComponentBenthosMonitor, instanceName, err, b.baseFSMInstance.GetLogger())
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		if b.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "start of reconciliation") {
			return nil, false
		}
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if b.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := b.baseFSMInstance.GetBackoffError(snapshot.Tick)
		b.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Benthos monitor %s: %v", instanceName, err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return b.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return b.IsRemoved() || b.IsRemoving() || b.IsStopping() || b.IsStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return b.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal when other approaches fail - bypasses state transitions
					// and directly deletes files and resources
					return b.monitorService.ForceRemoveBenthosMonitor(ctx, services)
				},
			)
		}
		return nil, false
	}

	// Step 2: Detect external changes.
	if err = b.reconcileExternalChanges(ctx, services, snapshot); err != nil {

		// I am using strings.Contains as i cannot get it working with errors.Is
		isExpectedError := strings.Contains(err.Error(), benthos_monitor_service.ErrServiceNotExist.Error()) ||
			strings.Contains(err.Error(), benthos_monitor_service.ErrServiceNoLogFile.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceConnectionRefused.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceConnectionTimedOut.Error()) ||
			strings.Contains(err.Error(), benthos_monitor_service.ErrServiceNoSectionsFound.Error()) ||
			strings.Contains(err.Error(), monitor.ErrServiceStopped.Error()) // This is expected when the service is stopped or stopping, no need to fetch logs, metrics, etc.

		if !isExpectedError {

			if errors.Is(err, context.DeadlineExceeded) {
				// Context deadline exceeded should be retried with backoff, not ignored
				b.baseFSMInstance.SetError(err, snapshot.Tick)
				b.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in reconcileExternalChanges, will retry with backoff")
				err = nil // Clear error so reconciliation continues
				return nil, false
			}

			b.baseFSMInstance.SetError(err, snapshot.Tick)
			b.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)

			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		err = nil // The service does not exist or has troubles fetching the observed state (which will cause the service to be in degraded state)
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = b.reconcileStateTransition(ctx, services)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		// Also this should not
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
			return nil, false
		}

		if errors.Is(err, context.DeadlineExceeded) {
			// Context deadline exceeded should be retried with backoff, not ignored
			b.baseFSMInstance.SetError(err, snapshot.Tick)
			b.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in reconcileStateTransition, will retry with backoff")
			err = nil // Clear error so reconciliation continues
			return nil, false
		}

		b.baseFSMInstance.SetError(err, snapshot.Tick)
		b.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	s6Err, s6Reconciled := b.monitorService.ReconcileManager(ctx, services, snapshot.Tick)
	if s6Err != nil {
		if errors.Is(s6Err, context.DeadlineExceeded) {
			// Context deadline exceeded should be retried with backoff, not ignored
			b.baseFSMInstance.SetError(s6Err, snapshot.Tick)
			b.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in s6Manager reconciliation, will retry with backoff")
			return nil, false
		}
		b.baseFSMInstance.SetError(s6Err, snapshot.Tick)
		b.baseFSMInstance.GetLogger().Errorf("error reconciling s6Manager: %s", s6Err)
		return nil, false
	}

	// If either benthos monitor state or S6 state was reconciled, we return reconciled so that nothing happens anymore in this tick
	// nothing should happen as we might have already taken up some significant time of the avaialble time per tick, so better
	// to be on the safe side and let the rest handle in another tick
	reconciled = reconciled || s6Reconciled

	// It went all right, so clear the error
	b.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the Benthos monitor service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (b *BenthosMonitorInstance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.BenthosMonitorUpdateObservedStateTimeout)
	defer cancel()

	err := b.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (b *BenthosMonitorInstance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := b.baseFSMInstance.GetCurrentFSMState()
	desiredState := b.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := b.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, b.CreateInstance, b.RemoveInstance, b.CheckForCreation)
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
		err, reconciled := b.reconcileOperationalStates(ctx, services, currentState, desiredState, time.Now())
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
func (b *BenthosMonitorInstance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateActive:
		return b.reconcileTransitionToActive(ctx, services, currentState, currentTime)
	case OperationalStateStopped:
		return b.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToActive handles transitions when the desired state is Active.
// It deals with moving from various states to the Active state.
func (b *BenthosMonitorInstance) reconcileTransitionToActive(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileTransitionToActive", time.Since(start))
	}()

	switch {
	// If we're stopped, we need to start first
	case currentState == OperationalStateStopped:
		err := b.StartInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition from Stopped to Starting
		return b.baseFSMInstance.SendEvent(ctx, EventStart), true
	case IsStartingState(currentState):
		return b.reconcileStartingStates(ctx, services, currentState, currentTime)
	case IsRunningState(currentState):
		return b.reconcileRunningStates(ctx, services, currentState, currentTime)
	case currentState == OperationalStateStopping:
		// There can be the edge case where an fsm is set to stopped, and then a cycle later again to active
		// It will cause the stopping process to start, but then the deisred state is again active, so it will land up in reconcileTransitionToActive
		// if it is stopping, we will first finish the stopping process and then we will go to active
		return b.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid current state: %s", currentState), false
	}
}

// reconcileStartingStates handles the various starting phase states when transitioning to a running state
// no big startup process here
func (b *BenthosMonitorInstance) reconcileStartingStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileStartingStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateStarting:

		// nothing to verify here, just for consistency with other fsms
		return b.baseFSMInstance.SendEvent(ctx, EventStartDone), true
	default:
		return fmt.Errorf("invalid starting state: %s", currentState), false
	}
}

// reconcileRunningStates handles the various running states when transitioning to Active.
func (b *BenthosMonitorInstance) reconcileRunningStates(ctx context.Context, services serviceregistry.Provider, currentState string, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileRunningStates", time.Since(start))
	}()

	switch currentState {
	case OperationalStateActive:
		// If we're in Active, we need to check whether it is degraded
		if !b.isMonitorHealthy(currentTime) {
			return b.baseFSMInstance.SendEvent(ctx, EventMetricsNotOK), true
		}
		return nil, false
	case OperationalStateDegraded:
		// If we're in Degraded, we need to recover to move to Active
		if b.isMonitorHealthy(currentTime) {
			return b.baseFSMInstance.SendEvent(ctx, EventMetricsAllOK), true
		}
		return nil, false
	default:
		return fmt.Errorf("invalid running state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from any operational state to Stopping and then to Stopped.
func (b *BenthosMonitorInstance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBenthosMonitor, b.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
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
		return b.baseFSMInstance.SendEvent(ctx, EventStopDone), true
	default:
		// For any other state, initiate stop
		err := b.StopInstance(ctx, services.GetFileSystem())
		if err != nil {
			return err, false
		}
		// Send event to transition to Stopping
		return b.baseFSMInstance.SendEvent(ctx, EventStop), true
	}
}
