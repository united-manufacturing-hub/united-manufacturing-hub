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

package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/looplab/fsm"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// NewConnection creates a new Connection instance with the given configuration
func NewConnection(config ConnectionConfig) *Connection {
	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			{Name: EventTest, Src: []string{OperationalStateStopped}, Dst: OperationalStateTesting},

			// Testing can result in success or failure
			{Name: EventTestDone, Src: []string{OperationalStateTesting}, Dst: OperationalStateSuccess},
			{Name: EventTestFailed, Src: []string{OperationalStateTesting}, Dst: OperationalStateFailure},

			// From any operational state except Stopped, we can transition to Stopping
			{Name: EventStop, Src: []string{OperationalStateTesting, OperationalStateSuccess, OperationalStateFailure}, Dst: OperationalStateStopping},

			// Final transition for stopping
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},
		},
	}

	instance := &Connection{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, logger.For(config.Name)),
		Config:          config,
		ObservedState:   ConnectionObservedState{},
	}

	instance.registerCallbacks()

	return instance
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected maximum p95 execution time per instance
// This is used by the manager to determine if there's enough time in the current reconcile cycle to process this instance
func (c *Connection) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	// For connection testing, we set a relatively short expected execution time
	// since most operations are fast except for the actual nmap test
	return 500 * time.Millisecond
}

// SetDesiredFSMState safely updates the desired state
func (c *Connection) SetDesiredFSMState(state string) error {
	// For Connection, we only allow setting Stopped or a testing state
	if state != OperationalStateStopped &&
		state != OperationalStateTesting {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateTesting)
	}

	c.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// Remove starts the removal process
func (c *Connection) Remove(ctx context.Context) error {
	return c.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (c *Connection) IsRemoved() bool {
	return c.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (c *Connection) IsRemoving() bool {
	return c.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (c *Connection) IsStopping() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (c *Connection) IsStopped() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (c *Connection) PrintState() {
	c.baseFSMInstance.GetLogger().Debugf("Current state: %s", c.baseFSMInstance.GetCurrentFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Desired state: %s", c.baseFSMInstance.GetDesiredFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", c.ObservedState)
}

// Reconcile examines the Connection and:
//  1. Check if a previous transition failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., results from nmap tests).
//  3. Attempt the required state transition by sending the appropriate event.
func (c *Connection) Reconcile(ctx context.Context, tick uint64) (err error, reconciled bool) {
	componentName := c.baseFSMInstance.GetID()
	defer func() {
		if err != nil {
			c.baseFSMInstance.GetLogger().Errorf("error reconciling Connection %s: %v", componentName, err)
			c.PrintState()
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Step 1: If there's a lastError, see if we've waited enough.
	if c.baseFSMInstance.ShouldSkipReconcileBecauseOfError(tick) {
		err := c.baseFSMInstance.GetBackoffError(tick)
		c.baseFSMInstance.GetLogger().Debugf("Skipping reconcile for Connection %s: %v", componentName, err)
		return nil, false
	}

	// Step 2: Detect external changes.
	if err := c.reconcileExternalChanges(ctx, tick); err != nil {
		c.baseFSMInstance.SetError(err, tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling external changes: %s", err)
		return nil, false
	}

	// Step 3: Attempt to reconcile the state.
	err, reconciled = c.reconcileStateTransition(ctx)
	if err != nil {
		// If the instance is removed, we don't want to return an error here
		if errors.Is(err, public_fsm.ErrInstanceRemoved) {
			return nil, false
		}

		c.baseFSMInstance.SetError(err, tick)
		c.baseFSMInstance.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false
	}

	// It went all right, so clear the error
	c.baseFSMInstance.ResetState()

	return nil, reconciled
}

// reconcileExternalChanges checks if the component's status has changed externally
// This will be where we check for nmap results or initiate the test if needed
func (c *Connection) reconcileExternalChanges(ctx context.Context, tick uint64) error {
	currentState := c.baseFSMInstance.GetCurrentFSMState()

	// If we're in the testing state, check if we need to start a test or check results
	if currentState == OperationalStateTesting {
		// If the test hasn't been started yet (no LastTestTime), start it
		if c.ObservedState.LastTestTime.IsZero() {
			if err := c.initiateConnectionTest(ctx); err != nil {
				return fmt.Errorf("failed to initiate connection test: %w", err)
			}
			return nil
		}

		// Check if the test has completed
		completed, err := c.checkNmapTestResult(ctx)
		if err != nil {
			return fmt.Errorf("failed to check nmap test result: %w", err)
		}

		// If the test is completed, update the parent DFC with the results
		if completed {
			if err := c.updateDFCWithConnectionStatus(ctx); err != nil {
				return fmt.Errorf("failed to update parent DFC: %w", err)
			}

			// Trigger the appropriate event based on test result
			if c.ObservedState.TestSuccessful {
				if err := c.baseFSMInstance.SendEvent(ctx, EventTestDone); err != nil {
					return fmt.Errorf("failed to send test done event: %w", err)
				}
			} else {
				if err := c.baseFSMInstance.SendEvent(ctx, EventTestFailed); err != nil {
					return fmt.Errorf("failed to send test failed event: %w", err)
				}
			}
		}
	}

	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
func (c *Connection) reconcileStateTransition(ctx context.Context) (err error, reconciled bool) {
	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if internal_fsm.IsLifecycleState(currentState) {
		err, reconciled := c.reconcileLifecycleStates(ctx, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// Handle operational states
	if IsOperationalState(currentState) {
		err, reconciled := c.reconcileOperationalStates(ctx, currentState, desiredState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	return fmt.Errorf("invalid state: %s", currentState), false
}

// reconcileLifecycleStates handles states related to instance lifecycle (creating/removing)
func (c *Connection) reconcileLifecycleStates(ctx context.Context, currentState string) (err error, reconciled bool) {
	// Basic lifecycle handling
	switch currentState {
	case internal_fsm.LifecycleStateToBeCreated:
		// Creation just involves setting up the state machine, no external resources needed
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreate), true
	case internal_fsm.LifecycleStateCreating:
		// Creation is immediately complete
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventCreateDone), true
	case internal_fsm.LifecycleStateRemoving:
		// Nothing to clean up
		return c.baseFSMInstance.SendEvent(ctx, internal_fsm.LifecycleEventRemoveDone), true
	case internal_fsm.LifecycleStateRemoved:
		return public_fsm.ErrInstanceRemoved, true
	default:
		// If we are not in a lifecycle state, just continue
		return nil, false
	}
}

// reconcileOperationalStates handles states related to instance operations
func (c *Connection) reconcileOperationalStates(ctx context.Context, currentState string, desiredState string) (err error, reconciled bool) {
	switch desiredState {
	case OperationalStateTesting:
		return c.reconcileTransitionToTesting(ctx, currentState)
	case OperationalStateStopped:
		return c.reconcileTransitionToStopped(ctx, currentState)
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState), false
	}
}

// reconcileTransitionToTesting handles transitions when the desired state is Testing
func (c *Connection) reconcileTransitionToTesting(ctx context.Context, currentState string) (err error, reconciled bool) {
	switch currentState {
	case OperationalStateStopped:
		// The connection test is stopped but should be testing, so start it
		return c.baseFSMInstance.SendEvent(ctx, EventTest), true

	case OperationalStateTesting:
		// The connection test is in progress, handled by reconcileExternalChanges
		return nil, false

	case OperationalStateSuccess, OperationalStateFailure:
		// Already in a terminal testing state, no action needed
		return nil, false

	case OperationalStateStopping:
		// The connection test is stopping, but we want it testing
		// Since we can't go directly from stopping to testing, we need to wait until it's stopped
		return nil, false

	default:
		return fmt.Errorf("unexpected current state: %s", currentState), false
	}
}

// reconcileTransitionToStopped handles transitions when the desired state is Stopped
func (c *Connection) reconcileTransitionToStopped(ctx context.Context, currentState string) (err error, reconciled bool) {
	switch currentState {
	case OperationalStateStopped:
		// The connection test is already stopped, nothing to do
		return nil, false

	case OperationalStateTesting, OperationalStateSuccess, OperationalStateFailure:
		// The connection test is in a non-stopped state but should be stopped
		return c.baseFSMInstance.SendEvent(ctx, EventStop), true

	case OperationalStateStopping:
		// The connection test is already in the process of stopping, check if it's done
		// Since there's no external process to stop, we can complete immediately
		return c.baseFSMInstance.SendEvent(ctx, EventStopDone), true

	default:
		return fmt.Errorf("unexpected current state: %s", currentState), false
	}
}
