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

//go:build test
// +build test

package fsmtest

import (
	"context"
	"fmt"
	"path/filepath"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Common test states
const (
	s6RunningState  = s6fsm.OperationalStateRunning
	s6StoppedState  = s6fsm.OperationalStateStopped
	s6StartingState = s6fsm.OperationalStateStarting
	s6StoppingState = s6fsm.OperationalStateStopping
)

// CreateS6TestConfig creates a minimal S6 service config for testing
func CreateS6TestConfig(name string, desiredState string) config.S6FSMConfig {
	return config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
			Command:     []string{"/bin/sh", "-c", "echo test"},
			Env:         map[string]string{},
			ConfigFiles: map[string]string{},
		},
	}
}

// ConfigureServiceForState sets up a mock S6 service to return the given state
func ConfigureServiceForState(mockService *s6service.MockService, servicePath string, state string) {
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]s6service.ServiceInfo)
	}
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}

	// Configure service state based on FSM state
	switch state {
	case internal_fsm.LifecycleStateToBeCreated:
		// Service doesn't exist for to_be_created state
		delete(mockService.ExistingServices, servicePath)
		delete(mockService.ServiceStates, servicePath)
	case internal_fsm.LifecycleStateCreating:
		// Service directory exists but isn't running
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}
	case internal_fsm.LifecycleStateRemoving:
		// Service exists but is about to be removed
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}
	case internal_fsm.LifecycleStateRemoved:
		// Service doesn't exist anymore
		delete(mockService.ExistingServices, servicePath)
		delete(mockService.ServiceStates, servicePath)
	case s6RunningState:
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceUp,
			Uptime: 10,
			Pid:    12345,
		}
	case s6StoppedState:
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}
	case s6StartingState:
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceRestarting, // Use as proxy for "starting"
		}
	case s6StoppingState:
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceDown, // Use as proxy for "stopping"
		}
	default:
		mockService.ExistingServices[servicePath] = true
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceUnknown,
		}
	}
}

// ConfigureS6ServiceConfig configures the mock service with default config
func ConfigureS6ServiceConfig(mockService *s6service.MockService) {
	mockService.GetConfigResult = s6serviceconfig.S6ServiceConfig{
		Command:     []string{"/bin/sh", "-c", "echo test"},
		Env:         map[string]string{},
		ConfigFiles: map[string]string{},
	}
}

// ConfigureS6State sets up both the instance and its mock service for a specific state
func ConfigureS6State(instance *s6fsm.S6Instance, targetState string) error {
	mockService, ok := instance.GetService().(*s6service.MockService)
	if !ok {
		return fmt.Errorf("instance doesn't use a MockService")
	}
	servicePath := instance.GetServicePath()

	// Configure service state
	ConfigureServiceForState(mockService, servicePath, targetState)

	return nil
}

// SetupS6InstanceWithState creates and configures an S6Instance in a specific state
func SetupS6InstanceWithState(
	baseDir string,
	name string,
	initialState string,
	desiredState string,
) (*s6fsm.S6Instance, error) {
	instance, _, _ := SetupS6Instance(baseDir, name, desiredState)

	// Configure for initial state if different from to_be_created
	if initialState != internal_fsm.LifecycleStateToBeCreated {
		err := ConfigureS6State(instance, initialState)
		if err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// waitForInstanceState is an internal helper that waits for an instance to reach a specific state
// through repeated reconciliation. Consider using TestS6StateTransition or StabilizeS6Instance instead.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The S6Instance to reconcile
//   - filesystemService: The filesystem service to use for reconciliation
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: Any error that occurred during reconciliation
func waitForInstanceState(
	ctx context.Context,
	instance *s6fsm.S6Instance,
	filesystemService filesystem.Service,
	targetState string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	var tick = startTick
	var currentState string

	for i := 0; i < maxAttempts; i++ {
		currentState = instance.GetCurrentFSMState()
		if currentState == targetState {
			return tick, nil
		}

		// Call Reconcile to progress the state
		_, _ = instance.Reconcile(ctx, filesystemService, tick)
		tick++
	}

	return tick, fmt.Errorf("failed to reach target state %s, current: %s after %d attempts",
		targetState, currentState, maxAttempts)
}

// ResetInstanceError resets the error and backoff state of an S6Instance.
// This is especially useful in tests to clear error conditions without
// direct access to baseFSMInstance.
//
// Parameters:
//   - instance: The S6Instance to reset error state for
func ResetInstanceError(instance *s6fsm.S6Instance, filesystemService filesystem.Service) {
	// Force a reconcile cycle with empty errors by simulating
	// a successful operation cycle. We rely on the fact that
	// each instance.Reconcile call will reset the error state
	// when operations succeed.
	mockService, ok := instance.GetService().(*s6service.MockService)
	if !ok {
		return
	}

	// Clear any error conditions in the mock
	mockService.StartError = nil
	mockService.StopError = nil
	mockService.CreateError = nil
	mockService.RemoveError = nil

	// Force a reconcile with ctx.Done to just reset internal state
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel to avoid actual operations
	instance.Reconcile(ctx, filesystemService, 0)
}

// StabilizeS6Instance is a simplified helper that configures the service and waits for an S6 instance
// to reach the desired state. This is the recommended way to stabilize an S6 instance for testing.
// Consider using TestS6StateTransition for testing full transitions.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The S6Instance to stabilize
//   - filesystemService: The filesystem service to use for reconciliation
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeS6Instance(
	ctx context.Context,
	instance *s6fsm.S6Instance,
	filesystemService filesystem.Service,
	targetState string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	// Get the mock service and service path from the instance
	mockService, ok := instance.GetService().(*s6service.MockService)
	if !ok {
		return startTick, fmt.Errorf("instance doesn't use a MockService")
	}

	servicePath := instance.GetServicePath()

	// Configure the mock service for the target state
	ConfigureServiceForState(mockService, servicePath, targetState)

	// Wait for the instance to reach the target state
	return waitForInstanceState(ctx, instance, filesystemService, targetState, maxAttempts, startTick)
}

// TestS6StateTransition tests a transition between two states using reconciliation.
// This is the main testing API for S6 instance state transitions.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The S6Instance to test
//   - filesystemService: The filesystem service to use for reconciliation
//   - fromState: The initial state to configure
//   - toState: The target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: (Optional) The starting tick value for reconciliation, defaults to 0 if not provided
//   - skipSetupDesiredState: (Optional) If true, won't set the desired state automatically
//
// Returns:
//   - uint64: The final tick value after all reconciliations
//   - error: Any error that occurred during the process
func TestS6StateTransition(
	ctx context.Context,
	instance *s6fsm.S6Instance,
	filesystemService filesystem.Service,
	fromState string,
	toState string,
	maxAttempts int,
	startTick uint64,
	skipSetupDesiredState ...bool,
) (uint64, error) {
	// Configure initial state
	err := ConfigureS6State(instance, fromState)
	if err != nil {
		return startTick, err
	}

	// Setup the desired state if it's an operational state and skipSetupDesiredState is not true
	skipDesired := len(skipSetupDesiredState) > 0 && skipSetupDesiredState[0]
	if !skipDesired && (toState == s6fsm.OperationalStateRunning || toState == s6fsm.OperationalStateStopped) {
		if err := instance.SetDesiredFSMState(toState); err != nil {
			return startTick, err
		}
	}

	// Use the provided tick value
	var tick = startTick

	// Run initial reconcile to trigger transition
	_, _ = instance.Reconcile(ctx, filesystemService, tick)
	tick++

	// Now stabilize to the target state
	return StabilizeS6Instance(ctx, instance, filesystemService, toState, maxAttempts, tick)
}

// SetupS6Instance creates a new S6Instance with a mock service
// This is a helper function for tests to quickly set up an instance for testing
func SetupS6Instance(
	baseDir string,
	name string,
	desiredState string,
) (*s6fsm.S6Instance, *s6service.MockService, string) {
	mockService := s6service.NewMockService()

	// Create config
	instanceConfig := CreateS6TestConfig(name, desiredState)

	// Create instance
	instance, err := s6fsm.NewS6InstanceWithService(
		baseDir,
		instanceConfig,
		mockService,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create S6Instance: %v", err))
	}

	servicePath := filepath.Join(baseDir, name)

	// Configure mock service with the test config
	mockService.GetConfigResult = instanceConfig.S6ServiceConfig

	return instance, mockService, servicePath
}

// VerifyStableState reconciles the instance multiple times and verifies it remains in the expected state.
// Unlike StabilizeS6Instance, this function always performs the specified number of reconciliations
// and verifies the instance stays in the expected state throughout.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The S6Instance to verify
//   - filesystemService: The filesystem service to use for reconciliation
//   - expectedState: The state the instance should remain in
//   - reconcileCount: Number of reconciliation cycles to perform
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after reconciliations
//   - error: Any error that occurred during verification
func VerifyStableState(
	ctx context.Context,
	instance *s6fsm.S6Instance,
	filesystemService filesystem.Service,
	expectedState string,
	reconcileCount int,
	startTick uint64,
) (uint64, error) {
	currentTick := startTick

	// Verify starting state
	if currentState := instance.GetCurrentFSMState(); currentState != expectedState {
		return currentTick, fmt.Errorf("instance not in expected state: got %s, want %s",
			currentState, expectedState)
	}

	// Perform specified number of reconciliations
	for i := 0; i < reconcileCount; i++ {
		// Reconcile and advance tick
		err, _ := instance.Reconcile(ctx, filesystemService, currentTick)
		if err != nil {
			return currentTick, fmt.Errorf("reconcile failed at tick %d: %w", currentTick, err)
		}
		currentTick++

		// Verify state hasn't changed
		if currentState := instance.GetCurrentFSMState(); currentState != expectedState {
			return currentTick, fmt.Errorf("unexpected state transition from %s to %s at tick %d",
				expectedState, currentState, currentTick)
		}
	}

	return currentTick, nil
}
