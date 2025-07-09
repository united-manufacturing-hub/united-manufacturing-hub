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

package s6

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
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// Reconcile examines the S6Instance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (s *S6Instance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	s6InstanceName := s.baseFSMInstance.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s6InstanceName, time.Since(start))
		if err != nil {
			s.baseFSMInstance.GetLogger().Errorf("error reconciling S6 instance %s: %s", s.baseFSMInstance.GetID(), err)
			s.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentS6Instance, s6InstanceName)

		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if s.baseFSMInstance.ShouldSkipReconcileBecauseOfError(snapshot.Tick) {
		err := s.baseFSMInstance.GetBackoffError(snapshot.Tick)
		s.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for S6 service %s: %s", s.baseFSMInstance.GetID(), err)

		// if it is a permanent error, start the removal process and reset the error (so that we can reconcile towards a stopped / removed state)
		if backoff.IsPermanentFailureError(err) {
			// For permanent errors, we need special handling based on the instance's current state:
			// 1. If already in a shutdown state (removed, removing, stopping, stopped), try force removal
			// 2. If not in a shutdown state, attempt normal removal first, then force if needed
			return s.baseFSMInstance.HandlePermanentError(
				ctx,
				err,
				func() bool {
					// Determine if we're already in a shutdown state where normal removal isn't possible
					// and force removal is required
					return s.IsRemoved() || s.IsRemoving() || s.IsStopping() || s.IsStopped() || s.WantsToBeStopped()
				},
				func(ctx context.Context) error {
					// Normal removal through state transition
					return s.Remove(ctx)
				},
				func(ctx context.Context) error {
					// Force removal as a last resort when normal state transitions can't work
					// This directly removes the s6 service directory from the filesystem
					return s.service.ForceRemove(ctx, s.servicePath, services.GetFileSystem())
				},
			)
		}

		return nil, false
	}

	// Step 2: Detect external changes.
	if err := s.reconcileExternalChanges(ctx, services, snapshot); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, s6service.ErrServiceNotExist) {
			s.baseFSMInstance.SetError(err, snapshot.Tick)
			s.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		//nolint:ineffassign
		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = s.reconcileStateTransition(ctx, services)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		// Also this should not
		if errors.Is(err, standarderrors.ErrInstanceRemoved) {
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

		s.baseFSMInstance.SetError(err, snapshot.Tick)
		s.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// It went all right, so clear the error
	s.baseFSMInstance.ResetState()

	return err, reconciled
}

// reconcileExternalChanges checks if the S6Instance service status has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
func (s *S6Instance) reconcileExternalChanges(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Create context for UpdateObservedStateOfInstance with minimum timeout guarantee
	// This ensures we get either 80% of available time OR the minimum required time, whichever is larger
	updateCtx, cancel := constants.CreateUpdateObservedStateContextWithMinimum(ctx, constants.S6UpdateObservedStateTimeout)
	defer cancel()

	err := s.UpdateObservedStateOfInstance(updateCtx, services, snapshot)
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
func (s *S6Instance) reconcileStateTransition(ctx context.Context, services serviceregistry.Provider) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := s.baseFSMInstance.GetCurrentFSMState()
	desiredState := s.baseFSMInstance.GetDesiredFSMState()

	// If already in the desired state, nothing to do.
	if currentState == desiredState {
		return nil, false
	}

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := s.baseFSMInstance.ReconcileLifecycleStates(ctx, services, currentState, s.CreateInstance, s.RemoveInstance, s.CheckForCreation)
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
		err, reconciled := s.reconcileOperationalStates(ctx, services, currentState, desiredState)
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
func (s *S6Instance) reconcileOperationalStates(ctx context.Context, services serviceregistry.Provider, currentState string, desiredState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateRunning:
		return s.reconcileTransitionToRunning(ctx, services, currentState)
	case OperationalStateStopped:
		return s.reconcileTransitionToStopped(ctx, services, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false // its simply an error, but we did not take any action
	}
}

// reconcileTransitionToRunning handles transitions when the desired state is Running.
// It deals with moving from Stopped/Failed to Starting and then to Running.
func (s *S6Instance) reconcileTransitionToRunning(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileTransitionToRunning", time.Since(start))
	}()

	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := s.StartInstance(ctx, services.GetFileSystem()); err != nil {
			return err, true
		}
		// Send event to transition from Stopped/Failed to Starting
		return s.baseFSMInstance.SendEvent(ctx, EventStart), true
	}

	if currentState == OperationalStateStarting {
		// If already in the process of starting, check if the service is healthy
		if s.IsS6Running() {
			// Transition from Starting to Running
			return s.baseFSMInstance.SendEvent(ctx, EventStartDone), true
		}
		// Otherwise, wait for the next reconcile cycle
		return nil, false
	}

	return nil, false
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped.
// It deals with moving from Running/Starting/Failed to Stopping and then to Stopped.
// It returns a boolean indicating whether the instance is stopped.
func (s *S6Instance) reconcileTransitionToStopped(ctx context.Context, services serviceregistry.Provider, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	if currentState == OperationalStateRunning || currentState == OperationalStateStarting {
		// Attempt to initiate a stop
		if err := s.StopInstance(ctx, services.GetFileSystem()); err != nil {
			return err, true
		}
		// Send event to transition to Stopping
		return s.baseFSMInstance.SendEvent(ctx, EventStop), true
	}

	if currentState == OperationalStateStopping {
		// If already stopping, verify if the instance is completely stopped
		if s.IsS6Stopped() {
			// Transition from Stopping to Stopped
			return s.baseFSMInstance.SendEvent(ctx, EventStopDone), true
		}
		return nil, false
	}

	return nil, false
}
