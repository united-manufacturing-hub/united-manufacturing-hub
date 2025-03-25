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

package fsmtest

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// CreateBenthosTestConfig creates a standard Benthos config for testing
func CreateBenthosTestConfig(name string, desiredState string) config.BenthosConfig {
	return config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		BenthosServiceConfig: config.BenthosServiceConfig{
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
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
			IsLive:  true,
			IsReady: true,
		}
	} else {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
			IsLive:  false,
			IsReady: false,
		}
	}

	// Setup metrics state if needed
	if flags.HasProcessingActivity {
		mockService.ServiceStates[serviceName].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
			IsActive: true,
		}
	} else if mockService.ServiceStates[serviceName].BenthosStatus.MetricsState == nil {
		mockService.ServiceStates[serviceName].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
			IsActive: false,
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// ConfigureBenthosServiceConfig configures the mock service with a default Benthos config
func ConfigureBenthosServiceConfig(mockService *benthossvc.MockBenthosService) {
	mockService.GetConfigResult = config.BenthosServiceConfig{
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

// WaitForBenthosManagerStable is a simple helper that calls manager.Reconcile once
// and checks if there was an error. It's used for scenarios like an empty config
// where we just want to do one pass. Then we might call it multiple times if needed.
func WaitForBenthosManagerStable(
	ctx context.Context,
	manager *benthosfsm.BenthosManager,
	fullCfg config.FullConfig,
	startTick uint64,
) (uint64, error) {
	err, _ := manager.Reconcile(ctx, fullCfg, startTick)
	if err != nil {
		return startTick, err
	}
	// We might do more checks if needed, e.g. manager has zero instances?
	return startTick + 1, nil
}

// WaitForBenthosManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForBenthosManagerInstanceState(
	ctx context.Context,
	manager *benthosfsm.BenthosManager,
	fullCfg config.FullConfig,
	instanceName string,
	desiredState string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, fullCfg, tick)
		if err != nil {
			return tick, err
		}
		tick++

		inst, found := manager.GetInstance(instanceName)
		if found && inst.GetCurrentFSMState() == desiredState {
			return tick, nil
		}
	}
	return tick, fmt.Errorf("instance %s did not reach state %s after %d attempts",
		instanceName, desiredState, maxAttempts)
}

// WaitForBenthosManagerInstanceRemoval waits until the given serviceName is removed
// from the manager's instance map.
func WaitForBenthosManagerInstanceRemoval(
	ctx context.Context,
	manager *benthosfsm.BenthosManager,
	fullCfg config.FullConfig,
	instanceName string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, fullCfg, tick)
		if err != nil {
			return tick, err
		}
		tick++

		if _, found := manager.GetInstance(instanceName); !found {
			// success
			return tick, nil
		}
	}
	return tick, fmt.Errorf("instance %s not removed after %d attempts", instanceName, maxAttempts)
}

// WaitForBenthosManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForBenthosManagerMultiState(
	ctx context.Context,
	manager *benthosfsm.BenthosManager,
	fullCfg config.FullConfig,
	desiredMap map[string]string, // e.g. { "svc1": "idle", "svc2": "active" }
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, fullCfg, tick)
		if err != nil {
			return tick, err
		}
		tick++

		allMatched := true
		for svc, desired := range desiredMap {
			inst, found := manager.GetInstance(svc)
			if !found || inst.GetCurrentFSMState() != desired {
				allMatched = false
				break
			}
		}
		if allMatched {
			return tick, nil
		}
	}
	return tick, fmt.Errorf("not all instances reached desired states after %d attempts", maxAttempts)
}

// SetupBenthosInstance creates and configures a Benthos instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupBenthosInstance(serviceName string, desiredState string) (*benthosfsm.BenthosInstance, *benthossvc.MockBenthosService, config.BenthosConfig) {
	// Create test config
	cfg := CreateBenthosTestConfig(serviceName, desiredState)

	// Create new instance directly using the correct constructor signature
	instance := benthosfsm.NewBenthosInstance(constants.S6BaseDir, cfg)

	// Create mock service
	mockService := benthossvc.NewMockBenthosService()

	// Set up initial service states
	mockService.ExistingServices = make(map[string]bool)
	mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)

	// Configure service with default config
	ConfigureBenthosServiceConfig(mockService)

	// Add default service info
	mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}

	// Replace the real service
	// Note: This requires creating a new instance in test code since we can't
	// access private fields directly. In the tests, we'll create the instance with
	// a mock service from the beginning.

	return instance, mockService, cfg
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
		_, _ = instance.Reconcile(ctx, tick)
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
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	expectedState string,
	numCycles int,
	startTick uint64,
) (uint64, error) {
	// Initial state check
	if instance.GetCurrentFSMState() != expectedState {
		return startTick, fmt.Errorf("instance is not in expected state %s; actual: %s",
			expectedState, instance.GetCurrentFSMState())
	}

	// Ensure the mock service stays configured for the expected state
	TransitionToBenthosState(mockService, serviceName, expectedState)

	// Execute reconcile cycles and check state stability
	tick := startTick
	for i := 0; i < numCycles; i++ {
		_, _ = instance.Reconcile(ctx, tick)
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

// SetupServiceInManager adds a service to the manager and configures it with the mock service.
// It creates an instance automatically and adds it to the manager.
// Note: Since this cannot directly set the service field in the instance after creation,
// the instance in the manager will be using the default service, not the mock service.
// Tests should rely on the manager.Reconcile method instead, which uses the manager's own service.
func SetupServiceInManager(
	manager *benthosfsm.BenthosManager,
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	desiredState string,
) {
	// Create a properly configured instance
	instance := benthosfsm.NewBenthosInstance(constants.S6BaseDir, CreateBenthosTestConfig(serviceName, desiredState))

	// Add it to the manager
	manager.BaseFSMManager.AddInstanceForTest(serviceName, instance)

	// Make sure the service exists in the mock service
	mockService.ExistingServices[serviceName] = true

	// Ensure we have a service info initialized
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}
}

// CreateMockBenthosInstance creates a Benthos instance for testing.
// Note: This creates a standard instance without replacing the service component.
// In actual tests, the pattern is to use the manager's functionality rather than
// working with individual instances.
func CreateMockBenthosInstance(serviceName string, mockService benthossvc.IBenthosService, desiredState string) *benthosfsm.BenthosInstance {
	cfg := CreateBenthosTestConfig(serviceName, desiredState)
	return benthosfsm.NewBenthosInstance(constants.S6BaseDir, cfg)
}

// CreateMockBenthosManager creates a Benthos manager with a mock service for testing
func CreateMockBenthosManager(name string) (*benthosfsm.BenthosManager, *benthossvc.MockBenthosService) {
	// Use the specialized constructor that sets up fully mocked services
	manager, mockService := benthosfsm.NewBenthosManagerWithMockedServices(name)

	// Set up the mock service to prevent real filesystem operations
	s6MockService := mockService.S6Service.(*s6svc.MockService)

	// Configure default responses to prevent real filesystem operations
	s6MockService.CreateError = nil
	s6MockService.RemoveError = nil
	s6MockService.StartError = nil
	s6MockService.StopError = nil
	s6MockService.ForceRemoveError = nil

	// Set up the mock to say services exist after creation
	s6MockService.ServiceExistsResult = true

	// Configure default successful statuses
	s6MockService.StatusResult = s6svc.ServiceInfo{
		Status: s6svc.ServiceUp,
		Pid:    12345, // Fake PID
		Uptime: 60,    // Fake uptime in seconds
	}

	return manager, mockService
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
	instance *benthosfsm.BenthosInstance,
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	targetState string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToBenthosState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyBenthosStableState(ctx, instance, mockService, serviceName, targetState, 3, tick)
		}

		_, _ = instance.Reconcile(ctx, tick)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// ConfigureBenthosManagerForState sets up the mock service in a BenthosManager to facilitate
// a state transition for a specific instance. This should be called before starting
// reconciliation if you want to ensure state transitions happen correctly.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - serviceName: The name of the service to configure
//   - targetState: The desired state to configure the service for
func ConfigureBenthosManagerForState(
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}
	mockService.ExistingServices[serviceName] = true

	// Make sure service state is initialized
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	}
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToBenthosState(mockService, serviceName, targetState)
}
