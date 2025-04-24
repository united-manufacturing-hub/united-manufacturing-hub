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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// CreateBenthosTestConfig creates a standard Benthos config for testing
func CreateBenthosTestConfig(name string, desiredState string) config.BenthosConfig {
	return config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		BenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = {\"message\":\"hello world\"}",
					"interval": "1s",
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
			MetricsPort: 9000,
		},
	}
}

// SetupBenthosServiceState configures the mock service state for Benthos instance tests
func SetupBenthosServiceState(
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	flags benthossvc.ServiceStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingServices[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Set S6 FSM state
	if flags.S6FSMState != "" {
		mockService.ServiceStates[serviceName].S6FSMState = flags.S6FSMState
	}

	// Update S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceUp,
			Uptime: 10, // Set uptime to 10s to simulate config loaded
			Pid:    1234,
		}
	} else {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceDown,
		}
	}

	// Update health check status
	if flags.IsHealthchecksPassed {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthos_monitor.HealthCheck{
			IsLive:  true,
			IsReady: true,
		}
	} else {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthos_monitor.HealthCheck{
			IsLive:  false,
			IsReady: false,
		}
	}

	// Setup metrics state if needed
	if flags.HasProcessingActivity {
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState = &benthos_monitor.BenthosMetricsState{
			IsActive: true,
		}
	} else if mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState == nil {
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState = &benthos_monitor.BenthosMetricsState{
			IsActive: false,
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// ConfigureBenthosServiceConfig configures the mock service with a default Benthos config
func ConfigureBenthosServiceConfig(mockService *benthossvc.MockBenthosService) {
	mockService.GetConfigResult = benthosserviceconfig.BenthosServiceConfig{
		Input: map[string]interface{}{
			"generate": map[string]interface{}{
				"mapping":  "root = {\"message\":\"hello world\"}",
				"interval": "1s",
			},
		},
		Output: map[string]interface{}{
			"drop": map[string]interface{}{},
		},
		MetricsPort: 9000,
	}
}

// TransitionToBenthosState is a helper to configure a service for a given high-level state
func TransitionToBenthosState(mockService *benthossvc.MockBenthosService, serviceName string, state string) {
	switch state {
	case benthosfsm.OperationalStateStopped:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStarting:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStartingConfigLoading:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          true,
			S6FSMState:           s6fsm.OperationalStateRunning,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateIdle:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
		})
	case benthosfsm.OperationalStateActive:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateDegraded:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   false,
			IsRunningWithoutErrors: false,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateStopping:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopping,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	}
}

// SetupBenthosInstance creates and configures a Benthos instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupBenthosInstance(serviceName string, desiredState string) (*benthosfsm.BenthosInstance, *benthossvc.MockBenthosService, config.BenthosConfig) {
	// Create test config
	cfg := CreateBenthosTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := benthossvc.NewMockBenthosService()

	// Set up initial service states
	mockService.ExistingServices = make(map[string]bool)
	mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)

	// Configure service with default config
	ConfigureBenthosServiceConfig(mockService)

	// Add default service info
	mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}

	// Create new instance directly using the specialized constructor
	instance := setUpMockBenthosInstance(cfg, mockService)

	return instance, mockService, cfg
}

// setUpMockBenthosInstance creates a BenthosInstance with a mock service
// This is an internal helper function used by SetupBenthosInstance
func setUpMockBenthosInstance(cfg config.BenthosConfig, mockService *benthossvc.MockBenthosService) *benthosfsm.BenthosInstance {
	// First create the instance normally
	instance := benthosfsm.NewBenthosInstance(cfg)

	// Set the mock service using the test utility method
	instance.SetService(mockService)

	return instance
}

// TestBenthosStateTransition tests a transition from one state to another without directly calling Reconcile.
// Instead, it sets up the proper mock service conditions and then calls Reconcile in a controlled way.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The BenthosInstance to reconcile
//   - mockService: The mock service to use (since we can't get it from the instance)
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestBenthosStateTransition(
	ctx context.Context,
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	filesystemService filesystem.Service,
	serviceName string,
	fromState string,
	toState string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	// 1. Verify the instance is in the expected starting state
	if instance.GetCurrentFSMState() != fromState {
		return startTick, fmt.Errorf("instance is in state %s, not in expected starting state %s",
			instance.GetCurrentFSMState(), fromState)
	}

	// 2. Set up the mock service for the target state
	TransitionToBenthosState(mockService, serviceName, toState)

	// 4. Reconcile until we reach the target state or exhaust attempts
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == toState {
			return tick, nil // Success!
		}

		// Perform a reconcile cycle
		_, _ = instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, filesystemService)
		tick++
	}

	// Did not reach the target state
	return startTick + uint64(maxAttempts), fmt.Errorf(
		"failed to transition from %s to %s after %d attempts; current state: %s",
		fromState, toState, maxAttempts, instance.GetCurrentFSMState())
}

// VerifyBenthosStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The BenthosInstance to reconcile
//   - mockService: The mock service to use (since we can't get it from the instance)
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyBenthosStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	filesystemService filesystem.Service,
	serviceName string,
	expectedState string,
	numCycles int,
) (uint64, error) {
	// Initial state check
	if instance.GetCurrentFSMState() != expectedState {
		return snapshot.Tick, fmt.Errorf("instance is not in expected state %s; actual: %s",
			expectedState, instance.GetCurrentFSMState())
	}

	// Ensure the mock service stays configured for the expected state
	TransitionToBenthosState(mockService, serviceName, expectedState)

	// Execute reconcile cycles and check state stability
	tick := snapshot.Tick
	for i := 0; i < numCycles; i++ {
		_, _ = instance.Reconcile(ctx, snapshot, filesystemService)
		tick++

		if instance.GetCurrentFSMState() != expectedState {
			return tick, fmt.Errorf(
				"state changed from %s to %s during cycle %d/%d",
				expectedState, instance.GetCurrentFSMState(), i+1, numCycles)
		}
	}

	return tick, nil
}

// ResetBenthosInstanceError resets the error and backoff state of a BenthosInstance.
// This is useful in tests to clear error conditions.
//
// Parameters:
//   - mockService: The mock service to reset
func ResetBenthosInstanceError(mockService *benthossvc.MockBenthosService) {
	// Clear any error conditions in the mock
	mockService.AddBenthosToS6ManagerError = nil
	mockService.StartBenthosError = nil
	mockService.StopBenthosError = nil
	mockService.UpdateBenthosInS6ManagerError = nil
	mockService.RemoveBenthosFromS6ManagerError = nil
	mockService.ForceRemoveBenthosError = nil
}

// CreateMockBenthosInstance creates a Benthos instance for testing.
// Note: This creates a standard instance without replacing the service component.
// In actual tests, the pattern is to use the manager's functionality rather than
// working with individual instances.
func CreateMockBenthosInstance(serviceName string, mockService benthossvc.IBenthosService, desiredState string) *benthosfsm.BenthosInstance {
	cfg := CreateBenthosTestConfig(serviceName, desiredState)
	return benthosfsm.NewBenthosInstance(cfg)
}

// StabilizeBenthosInstance ensures the Benthos instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The BenthosInstance to stabilize
//   - mockService: The mock service to use
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeBenthosInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	filesystemService filesystem.Service,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToBenthosState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyBenthosStableState(ctx, snapshot, instance, mockService, filesystemService, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, filesystemService)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForBenthosDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The BenthosInstance to monitor
//   - startTick: The starting tick value for reconciliation
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForBenthosDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *benthosfsm.BenthosInstance,
	filesystemService filesystem.Service,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Check if we've reached the target desired state
		if instance.GetDesiredFSMState() == targetState {
			return tick, nil
		}

		// Run a reconcile cycle
		err, _ := instance.Reconcile(ctx, snapshot, filesystemService)
		if err != nil {
			return tick, err
		}

		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach desired state %s after %d attempts; current desired state: %s",
		targetState, maxAttempts, instance.GetDesiredFSMState())
}

// ReconcileBenthosUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The BenthosInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - serviceName: The name of the service
//   - startTick: The starting tick value for reconciliation
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileBenthosUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	filesystemService filesystem.Service,
	serviceName string,
	maxAttempts int,
) (uint64, error, bool) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Perform a reconcile cycle and capture the error and reconciled status
		err, reconciled := instance.Reconcile(ctx, snapshot, filesystemService)
		tick++

		if err != nil {
			// Error found, return it along with the current tick
			return tick, err, reconciled
		}
	}

	// No error found after maxAttempts
	return tick, nil, true
}
