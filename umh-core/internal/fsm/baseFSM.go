package fsm

import (
	"context"
	"sync"

	"github.com/looplab/fsm"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/backoff"
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
		s.logger.Errorf("FSM %s has reached permanent failure state", s.cfg.ID)
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
	return s.cfg.DesiredFSMState
}

// GetCurrentFSMState returns the current state of the FSM
func (s *BaseFSMInstance) GetCurrentFSMState() string {
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
