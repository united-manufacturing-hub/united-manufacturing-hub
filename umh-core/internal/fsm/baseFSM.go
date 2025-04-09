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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// BaseFSMInstance implements the public fsm.FSM interface
type BaseFSMInstance struct {
	cfg BaseFSMInstanceConfig

	// mu is a mutex for protecting concurrent access to fields
	mu sync.RWMutex

	// fsm is the finite state machine that manages instance state
	fsm *fsm.FSM

	// Callbacks for state transitions
	callbacks map[string]fsm.Callback

	// backoffManager is the backoff manager for handling error retries and permanent failures
	backoffManager *backoff.BackoffManager

	// logger is the logger for the FSM
	logger *zap.SugaredLogger
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
}

// NewBaseFSMInstance creates a new FSM instance
func NewBaseFSMInstance(cfg BaseFSMInstanceConfig, logger *zap.SugaredLogger) *BaseFSMInstance {

	baseInstance := &BaseFSMInstance{
		cfg:       cfg,
		callbacks: make(map[string]fsm.Callback),
		logger:    logger,
	}

	// Initialize backoff manager with appropriate configuration
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)
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

// SendEvent sends an event to the FSM and returns whether the event was processed
func (s *BaseFSMInstance) SendEvent(ctx context.Context, eventName string, args ...interface{}) error {
	return s.fsm.Event(ctx, eventName, args...)
}

// ClearError clears any error state and resets the backoff
func (s *BaseFSMInstance) ClearError() {
	s.backoffManager.Reset()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
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

// StandardReconcile provides a standardized reconciliation flow that specific implementations can use
func (s *BaseFSMInstance) StandardReconcile(ctx context.Context,
	actions struct {
		Create              func(ctx context.Context, filesystemService filesystem.Service) error
		Remove              func(ctx context.Context, filesystemService filesystem.Service) error
		Start               func(ctx context.Context, filesystemService filesystem.Service) error
		Stop                func(ctx context.Context, filesystemService filesystem.Service) error
		UpdateObservedState func(ctx context.Context, filesystemService filesystem.Service, tick uint64) error
	},
	filesystemService filesystem.Service,
	tick uint64) (error, bool) {

	// Skip if we're in backoff mode
	if s.ShouldSkipReconcileBecauseOfError(tick) {
		return s.GetBackoffError(tick), false
	}

	// First update observed state to get current information
	err := actions.UpdateObservedState(ctx, filesystemService, tick)
	if err != nil {
		s.SetError(err, tick)
		return err, false
	}

	// Get current and desired states
	currentState := s.GetCurrentFSMState()
	desiredState := s.GetDesiredFSMState()

	// Handle lifecycle states first (to_be_created, creating, removing, removed)
	if IsLifecycleState(currentState) {
		return s.handleLifecycleState(ctx, actions, filesystemService, currentState, desiredState)
	}

	// Handle operational states
	return s.handleOperationalState(ctx, actions, filesystemService, currentState, desiredState)
}

// Helper methods for reconciliation
func (s *BaseFSMInstance) handleLifecycleState(ctx context.Context,
	actions struct {
		Create              func(ctx context.Context, filesystemService filesystem.Service) error
		Remove              func(ctx context.Context, filesystemService filesystem.Service) error
		Start               func(ctx context.Context, filesystemService filesystem.Service) error
		Stop                func(ctx context.Context, filesystemService filesystem.Service) error
		UpdateObservedState func(ctx context.Context, filesystemService filesystem.Service, tick uint64) error
	},
	filesystemService filesystem.Service,
	currentState, desiredState string) (error, bool) {

	switch currentState {
	case LifecycleStateToBeCreated:
		return s.handleToBeCreated(ctx, filesystemService)
	case LifecycleStateCreating:
		return s.handleCreating(ctx, actions, filesystemService)
	case LifecycleStateRemoving:
		return s.handleRemoving(ctx, actions, filesystemService)
	case LifecycleStateRemoved:
		return nil, false // Nothing to do
	default:
		return fmt.Errorf("unknown lifecycle state: %s", currentState), false
	}
}

// Helper methods for specific state transitions
func (s *BaseFSMInstance) handleToBeCreated(ctx context.Context, filesystemService filesystem.Service) (error, bool) {
	err := s.SendEvent(ctx, LifecycleEventCreate)
	if err != nil {
		return fmt.Errorf("failed to send create event: %w", err), false
	}
	return nil, true
}

func (s *BaseFSMInstance) handleCreating(ctx context.Context,
	actions struct {
		Create              func(ctx context.Context, filesystemService filesystem.Service) error
		Remove              func(ctx context.Context, filesystemService filesystem.Service) error
		Start               func(ctx context.Context, filesystemService filesystem.Service) error
		Stop                func(ctx context.Context, filesystemService filesystem.Service) error
		UpdateObservedState func(ctx context.Context, filesystemService filesystem.Service, tick uint64) error
	},
	filesystemService filesystem.Service) (error, bool) {

	err := actions.Create(ctx, filesystemService)
	if err != nil {
		s.SetError(err, uint64(time.Now().Unix()))
		return err, false
	}

	err = s.SendEvent(ctx, LifecycleEventCreateDone)
	if err != nil {
		return fmt.Errorf("failed to send create_done event: %w", err), false
	}
	return nil, true
}

func (s *BaseFSMInstance) handleRemoving(ctx context.Context,
	actions struct {
		Create              func(ctx context.Context, filesystemService filesystem.Service) error
		Remove              func(ctx context.Context, filesystemService filesystem.Service) error
		Start               func(ctx context.Context, filesystemService filesystem.Service) error
		Stop                func(ctx context.Context, filesystemService filesystem.Service) error
		UpdateObservedState func(ctx context.Context, filesystemService filesystem.Service, tick uint64) error
	},
	filesystemService filesystem.Service) (error, bool) {

	err := actions.Remove(ctx, filesystemService)
	if err != nil {
		s.SetError(err, uint64(time.Now().Unix()))
		return err, false
	}

	err = s.SendEvent(ctx, LifecycleEventRemoveDone)
	if err != nil {
		return fmt.Errorf("failed to send remove_done event: %w", err), false
	}
	return nil, true
}

// handleOperationalState manages transitions between operational states
// This is a skeleton implementation that should be overridden by concrete implementations
func (s *BaseFSMInstance) handleOperationalState(ctx context.Context,
	actions struct {
		Create              func(ctx context.Context, filesystemService filesystem.Service) error
		Remove              func(ctx context.Context, filesystemService filesystem.Service) error
		Start               func(ctx context.Context, filesystemService filesystem.Service) error
		Stop                func(ctx context.Context, filesystemService filesystem.Service) error
		UpdateObservedState func(ctx context.Context, filesystemService filesystem.Service, tick uint64) error
	},
	filesystemService filesystem.Service,
	currentState, desiredState string) (error, bool) {

	// This should be implemented by concrete implementations
	return fmt.Errorf("operational state handling not implemented for %s", s.cfg.ID), false
}
