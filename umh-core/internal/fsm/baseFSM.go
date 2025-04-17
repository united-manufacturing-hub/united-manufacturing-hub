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
	"fmt"
	"sync"
	"time"

	"github.com/looplab/fsm"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// BaseFSMInstance implements the shared logic for all FSM-based services.
// Concrete managers (e.g., Benthos, Redpanda) embed or wrap this to handle their domain logic.
type BaseFSMInstance struct {
	cfg BaseFSMInstanceConfig

	// mu is a mutex for protecting concurrent access to fields
	mu sync.RWMutex

	// fsm is the finite state machine that manages instance state
	fsm *fsm.FSM

	// Registered "enter_state" callbacks, purely for logging or minor side-effects.
	callbacks map[string]fsm.Callback

	// Handles exponential backoff for repeated transient errors,
	// culminating in a "permanent failure" if max retries are exceeded.
	backoffManager *backoff.BackoffManager

	// logger is the logger for the FSM
	logger *zap.SugaredLogger

	// Each concrete instance must provide these actions
	FSMInstanceActions

	// Each concrete manager must provide these reconcile methods
	FSMInstanceReconcile
}

// BaseFSMInstanceManagerInterface is the interface that each concrete manager must implement
// We are not using the FSMInstanceManagerInterface here because we want to avoid a circular dependency
type BaseFSMInstanceManagerInterface interface {
	Reconcile(ctx context.Context, currentSnapshot snapshot.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool)
}

// BaseFSMInstanceConfig holds parameters for setting up the base FSM.
type BaseFSMInstanceConfig struct {
	ID              string
	DesiredFSMState string

	// FSM

	// OperationalStateAfterCreate is the operational state after the create event
	OperationalStateAfterCreate string
	// OperationalStateBeforeRemove is the operational state before the remove event
	// The lifecycle state removing is only allowed from this state
	// This is usually "stopped"
	OperationalStateBeforeRemove string
	// OperationalStateBeforeStopping is the operational state before OperationalStateBeforeRemove
	// This is usually "stopping"
	// If there is no equivalent here, then it can be left empty
	OperationalStateBeforeBeforeRemove string

	// OperationalTransitions are the transitions that are allowed in the operational state
	OperationalTransitions []fsm.EventDesc

	// Timeouts

	UpdateObservedStateTimeout time.Duration

	// If you want to specify certain states in which certain errors are "ignored" or "permanent," you can do so:
	// IgnoreErrorsInStates is a map of states telling in what state which errors are ignored
	IgnoreErrorsInStates map[string][]error
	// PermanentErrorsInStates is a map of states telling in what state which errors are considered permanent
	PermanentErrorsInStates map[string][]error

	// ManagersToReconcile is a list of managers that should be reconciled
	ManagersToReconcile []BaseFSMInstanceManagerInterface
}

// NewBaseFSMInstance sets up a new FSM with the standard lifecycle transitions plus
// your operational transitions.
func NewBaseFSMInstance(cfg BaseFSMInstanceConfig, logger *zap.SugaredLogger) *BaseFSMInstance {

	baseInstance := &BaseFSMInstance{
		cfg:       cfg,
		callbacks: make(map[string]fsm.Callback),
		logger:    logger,
	}

	// Create a default backoff manager (exponential) with limited retries.
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)
	baseInstance.backoffManager = backoff.NewBackoffManager(backoffConfig)

	// Lifecycle transitions + user-supplied operational transitions
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

// AddCallback adds a callback for a given event name
func (s *BaseFSMInstance) AddCallback(eventName string, callback fsm.Callback) {
	s.callbacks[eventName] = callback
}

// GetError returns the last error that occurred during a transition
func (s *BaseFSMInstance) GetError() error {
	return s.backoffManager.GetLastError()
}

// SetError sets the last error that occurred during a transition
// and returns true if the error is considered a permanent failure
func (s *BaseFSMInstance) SetError(err error, tick uint64) bool {
	isPermanent := s.backoffManager.SetError(err, tick)
	if isPermanent {
		sentry.ReportFSMErrorf(s.logger, s.cfg.ID, "BaseFSM", "permanent_failure", "FSM has reached permanent failure state: %v", err)
	}
	return isPermanent
}

// setDesiredFSMState safely updates the desired state
// but does not check if the desired state is valid
func (s *BaseFSMInstance) SetDesiredFSMState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.DesiredFSMState = state
	s.logger.Infof("Setting desired state of FSM %s to %s", s.cfg.ID, state)
}

// GetDesiredFSMState returns the desired state of the FSM
func (s *BaseFSMInstance) GetDesiredFSMState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.DesiredFSMState
}

// GetCurrentFSMState returns the current state of the FSM
func (s *BaseFSMInstance) GetCurrentFSMState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fsm.Current()
}

// SetCurrentFSMState sets the current state of the FSM
// This should only be called in tests
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
			return fmt.Errorf("context deadline exceeded")
		}
	}

	return s.fsm.Event(ctx, eventName, args...)
}

// ClearError clears any error state and resets the backoff
func (s *BaseFSMInstance) ClearError() {
	s.backoffManager.Reset()
}

// Remove attempts a normal removal via the lifecycle. If we see a permanent error
// or if removal fails, we can do a forced removal (skipping graceful steps).
func (s *BaseFSMInstance) Remove(ctx context.Context) error {
	// Set the desired state to the state before remove
	s.SetDesiredFSMState(s.cfg.OperationalStateBeforeRemove)
	return s.SendEvent(ctx, LifecycleEventRemove)
}

// IsRemoved returns true if the instance has been removed
func (s *BaseFSMInstance) IsRemoved() bool {
	return s.fsm.Current() == LifecycleStateRemoved
}

// IsRemoving returns true if the instance is in the removing state
func (s *BaseFSMInstance) IsRemoving() bool {
	return s.fsm.Current() == LifecycleStateRemoving
}

// IsStopping returns true if the FSM is in a stopping state
func (s *BaseFSMInstance) IsStopping() bool {
	return s.fsm.Current() == s.cfg.OperationalStateBeforeBeforeRemove
}

// IsStopped returns true if the FSM is in a stopped state
func (s *BaseFSMInstance) IsStopped() bool {
	return s.fsm.Current() == s.cfg.OperationalStateBeforeRemove
}

// ShouldSkipReconcileBecauseOfError returns true if the reconcile should be skipped
// because of an error that occurred in the last reconciliation and the backoff
// period has not yet elapsed, or if the FSM is in permanent failure state
func (s *BaseFSMInstance) ShouldSkipReconcileBecauseOfError(tick uint64) bool {
	return s.backoffManager.ShouldSkipOperation(tick)
}

// ResetState clears the error and backoff after a successful reconcile
func (s *BaseFSMInstance) ResetState() {
	s.backoffManager.Reset()
}

// IsPermanentlyFailed returns true if the FSM has reached a permanent failure state
// after exceeding the maximum retry attempts
func (s *BaseFSMInstance) IsPermanentlyFailed() bool {
	return s.backoffManager.IsPermanentlyFailed()
}

// GetBackoffError returns a structured error that includes backoff information
// This will return a permanent failure error or a temporary backoff error
// depending on the current state
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

// Create provides a default implementation that can be overridden
func (s *BaseFSMInstance) Create(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("create action not implemented for %s", s.cfg.ID)
}

// Remove for FSMActions interface provides a default implementation that can be overridden
// It's a separate implementation from the existing Remove method which handles removing through state transitions
func (s *BaseFSMInstance) RemoveAction(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("remove action not implemented for %s", s.cfg.ID)
}

// Start provides a default implementation that can be overridden
func (s *BaseFSMInstance) Start(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("start action not implemented for %s", s.cfg.ID)
}

// Stop provides a default implementation that can be overridden
func (s *BaseFSMInstance) Stop(ctx context.Context, filesystemService filesystem.Service) error {
	return fmt.Errorf("stop action not implemented for %s", s.cfg.ID)
}

// UpdateObservedState provides a default implementation that can be overridden
func (s *BaseFSMInstance) UpdateObservedState(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	return fmt.Errorf("updateObservedState action not implemented for %s", s.cfg.ID)
}
