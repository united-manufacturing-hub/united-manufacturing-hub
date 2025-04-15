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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Reconcile examines the S6Instance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (s *S6Instance) Reconcile(ctx context.Context, snapshot fsm.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
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
			// if it is already in stopped, stopping, removing states, and it again returns a permanent error,
			// we need to throw it to the manager as the instance itself here cannot fix it anymore
			if s.IsRemoved() || s.IsRemoving() || s.IsStopping() || s.IsStopped() {
				s.baseFSMInstance.GetLogger().Errorf("S6 instance %s is already in a terminal state, force removing it", s.baseFSMInstance.GetID())
				// force delete everything from the s6 file directory
				forceErr := s.service.ForceRemove(ctx, s.servicePath, filesystemService)
				if forceErr != nil {
					s.baseFSMInstance.GetLogger().Errorf("ForceRemove failed: %v", forceErr)
				}
				return err, false
			} else {
				s.baseFSMInstance.GetLogger().Errorf("S6 instance %s is not in a terminal state, resetting state and removing it", s.baseFSMInstance.GetID())
				s.baseFSMInstance.ResetState()
				s.Remove(ctx)
				return nil, false // let's try to at least reconcile towards a stopped / removed state
			}
		}

		return nil, false
	}

	// Step 2: Detect external changes.
	if err := s.reconcileExternalChanges(ctx, filesystemService, snapshot.Tick); err != nil {
		// If the service is not running, we don't want to return an error here, because we want to continue reconciling
		if !errors.Is(err, s6service.ErrServiceNotExist) {
			s.baseFSMInstance.SetError(err, snapshot.Tick)
			s.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
			return nil, false // We don't want to return an error here, because we want to continue reconciling
		}

		err = nil // The service does not exist, which is fine as this happens in the reconcileStateTransition
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = s.reconcileStateTransition(ctx, filesystemService)
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
func (s *S6Instance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	observedStateCtx, cancel := context.WithTimeout(ctx, constants.S6UpdateObservedStateTimeout)
	defer cancel()
	err := s.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick)
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
func (s *S6Instance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service) (err error, reconciled bool) {
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
		err, reconciled := s.reconcileLifecycleStates(ctx, filesystemService, currentState)
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
		err, reconciled := s.reconcileOperationalStates(ctx, filesystemService, currentState, desiredState)
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
func (s *S6Instance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		if err := s.CreateInstance(ctx, filesystemService); err != nil {
			return err, true
		}
		return s.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Check if the s6 service has its supervision directory set up
		servicePath := s.servicePath
		ready, err := s.service.EnsureSupervision(ctx, servicePath, filesystemService)
		if err != nil {
			s.baseFSMInstance.GetLogger().Warnf("Failed to ensure service supervision: %v", err)
			return nil, false // Don't transition state yet, retry next reconcile
		}

		// Only transition if the supervise directory actually exists
		if !ready {
			s.baseFSMInstance.GetLogger().Debugf("Waiting for s6-svscan to create supervise directory")
			return nil, false // Don't transition state yet, retry next reconcile
		}

		// If we get here, supervision is confirmed set up correctly
		s.baseFSMInstance.GetLogger().Debugf("Service supervision confirmed, transitioning to Created state")
		return s.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		if err := s.RemoveInstance(ctx, filesystemService); err != nil {
			return err, true
		}
		return s.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations (starting/stopping)
func (s *S6Instance) reconcileOperationalStates(ctx context.Context, filesystemService filesystem.Service, currentState string, desiredState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileOperationalStates", time.Since(start))
	}()

	switch desiredState {
	case OperationalStateRunning:
		return s.reconcileTransitionToRunning(ctx, filesystemService, currentState)
	case OperationalStateStopped:
		return s.reconcileTransitionToStopped(ctx, filesystemService, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false // its simply an error, but we did not take any action
	}
}

// reconcileTransitionToRunning handles transitions when the desired state is Running.
// It deals with moving from Stopped/Failed to Starting and then to Running.
func (s *S6Instance) reconcileTransitionToRunning(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileTransitionToRunning", time.Since(start))
	}()

	if currentState == OperationalStateStopped {
		// Attempt to initiate start
		if err := s.StartInstance(ctx, filesystemService); err != nil {
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
func (s *S6Instance) reconcileTransitionToStopped(ctx context.Context, filesystemService filesystem.Service, currentState string) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Instance, s.baseFSMInstance.GetID()+".reconcileTransitionToStopped", time.Since(start))
	}()

	if currentState == OperationalStateRunning || currentState == OperationalStateStarting {
		// Attempt to initiate a stop
		if err := s.StopInstance(ctx, filesystemService); err != nil {
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
