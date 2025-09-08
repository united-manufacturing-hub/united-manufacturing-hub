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

package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/looplab/fsm"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// BaseFSMInstance implements the public fsm.FSM interface.
type BaseFSMInstance struct {

	// fsm is the finite state machine that manages instance state
	fsm *fsm.FSM

	// Callbacks for state transitions
	callbacks map[string]fsm.Callback

	// backoffManager is the backoff manager for handling error retries and permanent failures
	backoffManager *backoff.BackoffManager

	// logger is the logger for the FSM
	logger *zap.SugaredLogger

	// lastObservedLifecycleState is the last state that was observed by the FSM
	// Note: this is only temporary and should be replaced by a generalized implementation of the archive storage
	lastObservedLifecycleState string
	cfg                        BaseFSMInstanceConfig

	// transientStreakCounter is the number of ticks a FSM has remained in a transient state
	transientStreakCounter uint64

	// mu is a mutex for protecting concurrent access to fields
	mu sync.RWMutex
}

type BaseFSMInstanceConfig struct {
	ID              string
	DesiredFSMState string

	// FSM

	// OperationalStateAfterCreate is the operational state after the create event
	OperationalStateAfterCreate string
	// OperationalStateBeforeRemove is the operational state before the remove event
	// The lifecycle state removing is only allowed from this state
	OperationalStateBeforeRemove string
	// OperationalTransitions are the transitions that are allowed in the operational state
	OperationalTransitions []fsm.EventDesc

	// Fallback for Transient States
	// If a FSM remains in one of these states for too long, it will cause a Remove / ForceRemove
	// This is to prevent an fsm to be stuck forever in an intermediate state

	// MaxTicksToRemainInTransientState is the maximum number of ticks a FSM can remain in a transient state
	// Note: currently only used for lifecycle states
	MaxTicksToRemainInTransientState uint64
}

// NewBaseFSMInstance creates a new FSM instance.
func NewBaseFSMInstance(cfg BaseFSMInstanceConfig, backoffConfig backoff.Config, logger *zap.SugaredLogger) *BaseFSMInstance {
	// Set default max ticks to remain in transient state if not set
	if cfg.MaxTicksToRemainInTransientState == 0 {
		cfg.MaxTicksToRemainInTransientState = constants.DefaultMaxTicksToRemainInTransientState
	}

	baseInstance := &BaseFSMInstance{
		cfg:       cfg,
		callbacks: make(map[string]fsm.Callback),
		logger:    logger,
	}

	// Initialize backoff manager with appropriate configuration
	baseInstance.backoffManager = backoff.NewBackoffManager(backoffConfig)

	// Combine lifecycle and operational transitions
	events := []fsm.EventDesc{
		// Lifecycle transitions
		{Name: LifecycleEventCreate, Src: []string{LifecycleStateToBeCreated}, Dst: LifecycleStateCreating},
		{Name: LifecycleEventCreateDone, Src: []string{LifecycleStateCreating}, Dst: cfg.OperationalStateAfterCreate},
		{Name: LifecycleEventRemove, Src: []string{cfg.OperationalStateBeforeRemove}, Dst: LifecycleStateRemoving},
		{Name: LifecycleEventRemoveDone, Src: []string{LifecycleStateRemoving}, Dst: LifecycleStateRemoved},
	}
	events = append(events, cfg.OperationalTransitions...)

	//
	baseInstance.fsm = fsm.NewFSM(
		LifecycleStateToBeCreated,
		fsm.Events(events),
		fsm.Callbacks{
			"enter_state": func(ctx context.Context, e *fsm.Event) {
				// Call registered callback for this state if exists
				if cb, ok := baseInstance.callbacks["enter_"+e.Dst]; ok {
					cb(ctx, e)
				}
			},
		},
	)

	// Register default lifecycle callbacks

	baseInstance.AddCallback("enter_"+LifecycleStateRemoved, func(ctx context.Context, e *fsm.Event) {
		baseInstance.logger.Debugf("Entering removed state for FSM %s", baseInstance.cfg.ID)
	})

	baseInstance.AddCallback("enter_"+LifecycleStateCreating, func(ctx context.Context, e *fsm.Event) {
		baseInstance.logger.Debugf("Entering creating state for FSM %s", baseInstance.cfg.ID)
	})

	baseInstance.AddCallback("enter_"+LifecycleStateToBeCreated, func(ctx context.Context, e *fsm.Event) {
		baseInstance.logger.Debugf("Entering to be created state for FSM %s", baseInstance.cfg.ID)
	})

	baseInstance.AddCallback("enter_"+LifecycleStateRemoving, func(ctx context.Context, e *fsm.Event) {
		baseInstance.logger.Debugf("Entering removing state for FSM %s", baseInstance.cfg.ID)
	})

	return baseInstance
}

// AddCallback adds a callback for a given event name.
func (s *BaseFSMInstance) AddCallback(eventName string, callback fsm.Callback) {
	s.callbacks[eventName] = callback
}

// GetError returns the last error that occurred during a transition.
func (s *BaseFSMInstance) GetError() error {
	return s.backoffManager.GetLastError()
}

// SetError sets the last error that occurred during a transition
// and returns true if the error is considered a permanent failure.
func (s *BaseFSMInstance) SetError(err error, tick uint64) bool {
	isPermanent := s.backoffManager.SetError(err, tick)
	if isPermanent {
		sentry.ReportFSMErrorf(s.logger, s.cfg.ID, "BaseFSM", "permanent_failure", "FSM has reached permanent failure state: %v", err)
	}

	return isPermanent
}

// setDesiredFSMState safely updates the desired state
// but does not check if the desired state is valid.
func (s *BaseFSMInstance) SetDesiredFSMState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cfg.DesiredFSMState = state
	s.logger.Infof("Setting desired state of FSM %s to %s", s.cfg.ID, state)
}

// GetDesiredFSMState returns the desired state of the FSM.
func (s *BaseFSMInstance) GetDesiredFSMState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.cfg.DesiredFSMState
}

// GetCurrentFSMState returns the current state of the FSM.
func (s *BaseFSMInstance) GetCurrentFSMState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.fsm.Current()
}

// SetCurrentFSMState sets the current state of the FSM
// This should only be called in tests.
func (s *BaseFSMInstance) SetCurrentFSMState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.fsm.SetState(state)
}

// SendEvent sends an event to the FSM and returns whether the event was processed.
//
// Problem: Context expiration during FSM transitions can lead to deadlocks:
// - When a context expires mid-transition, the FSM's internal transition state remains set
// - This causes future events to fail with "event X inappropriate because previous transition did not complete"
// - After multiple retries, the backoff manager marks the instance as permanently failed
// - The entire instance then gets unnecessarily removed from the system
//
// Solution: This method implements protective measures to prevent deadlocks:
// 1. Rejects event sending if the context is already cancelled
// 2. Refuses to start transitions when insufficient time remains before a deadline
//
// By ensuring transitions only start with sufficient context lifetime, we avoid the
// cascade of failures that would otherwise occur when a transition is interrupted.
func (s *BaseFSMInstance) SendEvent(ctx context.Context, eventName string, args ...interface{}) error {
	// Context protection: We verify two distinct conditions before proceeding:
	// 1. Context must not be cancelled - An already expired context will lead to immediate failure
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 2. Context must have sufficient time remaining - Context expiration during a transition
	//    is worse than failing to start, as it leaves the FSM in an inconsistent state
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < constants.ExpectedMaxP95ExecutionTimePerEvent {
			return errors.New("context deadline exceeded")
		}
	}

	// Capture state information for better error reporting
	currentState := s.fsm.Current()
	desiredState := s.GetDesiredFSMState()
	instanceID := s.GetID()

	// Log the transition attempt for debugging
	s.logger.Debugf("FSM %s attempting transition: current_state='%s' -> event='%s' (desired_state='%s')",
		instanceID, currentState, eventName, desiredState)

	// CRITICAL: Always give FSM a fresh context to prevent "previous transition did not complete" errors
	// This ensures the FSM can complete even under CPU throttling (cgroups throttle for ~100ms periods)
	// We ignore the parent context deadline to guarantee the transition completes
	// See Linear ticket ENG-3419 for full context and analysis

	fsmCtx, cancel := context.WithTimeout(context.Background(), constants.FSMTransitionTimeout)
	defer cancel()

	// Execute the FSM transition with guaranteed time to complete
	err := s.fsm.Event(fsmCtx, eventName, args...)
	
	// Check if parent context expired while we were executing
	if err == nil && ctx.Err() != nil {
		s.logger.Warnf("FSM transition completed successfully but parent context expired - prevented stuck transition but now outside of cycle time (preventing bigger impact)")
	}
	
	if err != nil {
		// Enhanced error message with state context
		enhancedErr := fmt.Errorf("FSM %s failed transition: current_state='%s' -> event='%s' (desired_state='%s'): %w",
			instanceID, currentState, eventName, desiredState, err)

		// Log the failure with full context
		s.logger.Errorf("FSM transition failed (test): %s", enhancedErr.Error())

		return enhancedErr
	}

	// Log successful transition
	newState := s.fsm.Current()
	if newState != currentState {
		s.logger.Debugf("FSM %s successful transition: '%s' -> '%s' via event='%s' (desired_state='%s')",
			instanceID, currentState, newState, eventName, desiredState)
	}

	return nil
}

// ClearError clears any error state and resets the backoff.
func (s *BaseFSMInstance) ClearError() {
	s.backoffManager.Reset()
}

// Remove starts the graceful-removal sequence.
//
//  1. Always point the desired state at <OperationalStateBeforeRemove> (usually "stopped").
//  2. If – and only if – the current FSM state already *is* that state (→
//     the service has finished stopping) fire the "remove" transition.
//  3. If we are already removing / removed, do nothing.
//
// This lets the manager call Remove() every tick without having to
// micromanage intermediate states.
func (s *BaseFSMInstance) Remove(ctx context.Context) error {
	// (1) Already done or in progress?
	if s.IsRemoved() || s.IsRemoving() {
		return nil
	}

	preRemove := s.cfg.OperationalStateBeforeRemove // usually "stopped"
	s.SetDesiredFSMState(preRemove)                 // converge towards it

	// (2) Are we *already* in <preRemove>?   fire the remove transition
	if s.GetCurrentFSMState() == preRemove && s.fsm.Can(LifecycleEventRemove) {
		return s.SendEvent(ctx, LifecycleEventRemove)
	}

	// (3) Not there yet – arm a one-shot callback that will fire the
	//     event the moment we enter <preRemove>.
	const cb = "auto_remove_once"
	if _, ok := s.callbacks[cb]; !ok {
		s.AddCallback("enter_"+preRemove, func(ctx context.Context, e *fsm.Event) {
			err := s.SendEvent(ctx, LifecycleEventRemove)
			if err != nil {
				// ── Wrap as *permanent* to force immediate escalation ─────────
				perr := fmt.Errorf("%s: remove transition failed: %w",
					backoff.PermanentFailureError, err)

				// Record it; tick value is irrelevant here (we pass 0)
				s.SetError(perr, 0)
				sentry.ReportFSMErrorf(s.logger, s.cfg.ID, "BaseFSM", "permanent_failure", "auto-remove of %s failed: %v", s.cfg.ID, err)
			}

			delete(s.callbacks, cb) // detach – run only once
		})
	}

	return nil
}

// IsRemoved returns true if the instance has been removed.
func (s *BaseFSMInstance) IsRemoved() bool {
	return s.fsm.Current() == LifecycleStateRemoved
}

// IsRemoving returns true if the instance is in the removing state.
func (s *BaseFSMInstance) IsRemoving() bool {
	return s.fsm.Current() == LifecycleStateRemoving
}

// ShouldSkipReconcileBecauseOfError returns true if the reconcile should be skipped
// because of an error that occurred in the last reconciliation and the backoff
// period has not yet elapsed, or if the FSM is in permanent failure state.
func (s *BaseFSMInstance) ShouldSkipReconcileBecauseOfError(tick uint64) bool {
	return s.backoffManager.ShouldSkipOperation(tick)
}

// ResetState clears the error and backoff after a successful reconcile.
func (s *BaseFSMInstance) ResetState() {
	s.backoffManager.Reset()
}

// IsPermanentlyFailed returns true if the FSM has reached a permanent failure state
// after exceeding the maximum retry attempts.
func (s *BaseFSMInstance) IsPermanentlyFailed() bool {
	return s.backoffManager.IsPermanentlyFailed()
}

// GetBackoffError returns a structured error that includes backoff information
// This will return a permanent failure error or a temporary backoff error
// depending on the current state.
func (s *BaseFSMInstance) GetBackoffError(tick uint64) error {
	return s.backoffManager.GetBackoffError(tick)
}

func (s *BaseFSMInstance) GetID() string {
	return s.cfg.ID
}

func (s *BaseFSMInstance) GetLogger() *zap.SugaredLogger {
	return s.logger
}

func (s *BaseFSMInstance) GetLastError() error {
	return s.backoffManager.GetLastError()
}

// Create provides a default implementation that can be overridden.
func (s *BaseFSMInstance) Create(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("create action not implemented for %s", s.cfg.ID)
}

// Remove for FSMActions interface provides a default implementation that can be overridden
// It's a separate implementation from the existing Remove method which handles removing through state transitions.
func (s *BaseFSMInstance) RemoveAction(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("remove action not implemented for %s", s.cfg.ID)
}

// Start provides a default implementation that can be overridden.
func (s *BaseFSMInstance) Start(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("start action not implemented for %s", s.cfg.ID)
}

// Stop provides a default implementation that can be overridden.
func (s *BaseFSMInstance) Stop(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("stop action not implemented for %s", s.cfg.ID)
}

// UpdateObservedState provides a default implementation that can be overridden.
func (s *BaseFSMInstance) UpdateObservedState(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	return fmt.Errorf("updateObservedState action not implemented for %s", s.cfg.ID)
}

// HandlePermanentError handles the case where a permanent error has occurred during reconciliation.
// It centralizes the error handling logic that was previously duplicated across FSM implementations.
//
// The function implements the following workflow:
//  1. If the FSM is in a shutdown state (e.g., stopped, stopping, removing, removed)
//     and can't recover normally, it attempts to forcefully remove the instance.
//  2. If the FSM is not in a shutdown state, it resets the error state and
//     attempts a normal removal first. If that fails, it tries force removal.
//
// Parameters:
//   - ctx: The context for the operation
//   - err: The permanent error that triggered this handling
//   - isInShutdownState: Function that returns true if the FSM instance is already in a shutdown state
//     (typically checks if it's removed, removing, stopped, or stopping) where normal recovery isn't possible
//   - normalRemove: Function that performs a graceful removal of the instance through proper state transitions
//   - forceRemove: Function that performs a forceful removal when graceful removal fails, bypassing state transitions
//
// Returns:
// - The original error (or nil if handled) and whether reconciliation should continue.
func (s *BaseFSMInstance) HandlePermanentError(
	ctx context.Context,
	err error,
	isInShutdownState func() bool,
	normalRemove func(ctx context.Context) error,
	forceRemove func(ctx context.Context) error,
) (error, bool) {
	instanceID := s.GetID()
	logger := s.GetLogger()

	// If it's already in a shutdown state (stopped, stopping, removing, removed)
	// we can't expect it to transition normally, so force remove it
	if isInShutdownState() {
		logger.Errorf("%s instance %s is already in a shutdown state, force removing it", s.cfg.ID, instanceID)

		// Force delete everything - this is the last resort when the instance
		// is already in a state where normal removal isn't possible
		forceErr := forceRemove(ctx)
		if forceErr != nil {
			// If even the force removing doesn't work, the base-manager should delete the instance
			// due to a permanent error
			logger.Errorf("error force removing %s instance %s: %v", s.cfg.ID, instanceID, forceErr)

			return fmt.Errorf("failed to force remove the %s instance: %s : %w", s.cfg.ID, backoff.PermanentFailureError, forceErr), false
		}

		return err, true
	} else {
		// Not in a shutdown state yet, so try normal removal first
		logger.Errorf("%s instance %s is not in a shutdown state, resetting state and removing it", s.cfg.ID, instanceID)
		s.ResetState()

		// Attempt normal removal through the state transition system
		removeErr := normalRemove(ctx)
		if removeErr != nil {
			// If removing doesn't work because the FSM isn't in the correct state,
			// try force removal as a last resort
			logger.Errorf("error removing %s instance %s: %v", s.cfg.ID, instanceID, removeErr)

			forceErr := forceRemove(ctx)
			if forceErr != nil {
				// If even the force removing doesn't work, the base-manager should delete the instance
				// due to a permanent error
				logger.Errorf("error force removing %s instance %s: %v", s.cfg.ID, instanceID, forceErr)

				return fmt.Errorf("failed to force remove the %s instance: %s : %w", s.cfg.ID, backoff.PermanentFailureError, forceErr), false
			}
		}

		return nil, false // Let's try to at least reconcile towards a stopped/removed state
	}
}

// ReconcileLifecycleStates reconciles the lifecycle states of the FSM instance
// It will reconcile the lifecycle states of the FSM instance and return the error and whether the reconciliation was successful
// It will also update the transient streak counter.
func (s *BaseFSMInstance) ReconcileLifecycleStates(
	ctx context.Context,
	services serviceregistry.Provider,
	currentState string,
	createInstance func(ctx context.Context, filesystemService filesystem.Service) error,
	removeInstance func(ctx context.Context, filesystemService filesystem.Service) error,
	checkForCreation func(ctx context.Context, filesystemService filesystem.Service) bool,
) (err error, reconciled bool) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMInstance, s.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	// Transient Streak Counter
	// TODO: to be implemented in the future for also non lifecycle states
	if currentState == s.lastObservedLifecycleState {
		s.transientStreakCounter++
	} else {
		s.transientStreakCounter = 0
	}

	s.lastObservedLifecycleState = currentState

	// Independent what the desired state is, we always need to reconcile the lifecycle states first
	switch currentState {
	case LifecycleStateToBeCreated:
		if err := createInstance(ctx, services.GetFileSystem()); err != nil {
			return err, false
		}

		return s.SendEvent(ctx, LifecycleEventCreate), true
	case LifecycleStateCreating:
		// Check if the service is created
		if !checkForCreation(ctx, services.GetFileSystem()) {
			return nil, false // Don't transition state yet, retry next reconcile
		}

		return s.SendEvent(ctx, LifecycleEventCreateDone), true
	case LifecycleStateRemoving:
		if err := removeInstance(ctx, services.GetFileSystem()); err != nil {
			// Treat “removal still in progress” as a *non-error* so that the reconcile
			// loop continues; the FSM stays in `removing` until RemoveInstance returns
			// nil or a hard error.
			if errors.Is(err, standarderrors.ErrRemovalPending) {
				return nil, false
			}

			return err, false
		}

		return s.SendEvent(ctx, LifecycleEventRemoveDone), true
	case LifecycleStateRemoved:
		return standarderrors.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed.
func (s *BaseFSMInstance) IsTransientStreakCounterMaxed() bool {
	return s.transientStreakCounter >= s.cfg.MaxTicksToRemainInTransientState
}

// IsDeadlineExceededAndHandle checks if the error is a context deadline exceeded error
// and handles it appropriately by setting the error and logging a warning.
// If the error is a context deadline exceeded, it:
// 1. Sets the error on the FSM instance for backoff handling
// 2. Logs a warning message
// 3. Returns true to indicate the error was handled and reconciliation should stop
//
// If the error is not a context deadline exceeded, it returns false to continue
// with normal error handling.
//
// Parameters:
//   - err: The error to check
//   - tick: The current system tick for backoff handling
//   - location: A description of where the error occurred (for logging)
//
// Returns:
//   - bool: true if deadline exceeded (handled, return early), false otherwise (continue)
func (s *BaseFSMInstance) IsDeadlineExceededAndHandle(err error, tick uint64, location string) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		// Context deadline exceeded should be retried with backoff, not ignored
		s.SetError(err, tick)
		s.logger.Warnf("Context deadline exceeded in %s, will retry with backoff", location)

		return true // handled, return early
	}

	return false // not handled, continue
}
