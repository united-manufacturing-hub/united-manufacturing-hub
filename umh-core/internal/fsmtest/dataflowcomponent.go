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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmtype "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateDataflowComponentTestConfig creates a standard DataflowComponent config for testing
func CreateDataflowComponentTestConfig(name string, desiredState string) config.DataFlowComponentConfig {
	return config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"generate": map[string]interface{}{
						"mapping":  "root = {\"message\":\"hello world\"}",
						"interval": "1s",
					},
				},
				Output: map[string]interface{}{
					"drop": map[string]interface{}{},
				},
			},
		},
	}
}

// SetupDataflowComponentServiceState configures the mock service state for DataflowComponent instance tests
func SetupDataflowComponentServiceState(
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	serviceName string,
	flags dataflowcomponentsvc.ComponentStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingComponents[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ComponentStates[serviceName] == nil {
		mockService.ComponentStates[serviceName] = &dataflowcomponentsvc.ServiceInfo{}
	}

	// Set the Benthos FSM state
	if flags.BenthosFSMState != "" {
		mockService.ComponentStates[serviceName].BenthosFSMState = flags.BenthosFSMState
	}

	// Store the service state flags directly
	mockService.SetComponentState(serviceName, flags)
}

// ConfigureDataflowComponentServiceConfig configures the mock service with a default DataflowComponent config
func ConfigureDataflowComponentServiceConfig(mockService *dataflowcomponentsvc.MockDataFlowComponentService) {
	mockService.GetConfigResult = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = {\"message\":\"hello world\"}",
					"interval": "1s",
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
		},
	}
}

// TransitionToDataflowComponentState is a helper to configure a service for a given high-level state
func TransitionToDataflowComponentState(mockService *dataflowcomponentsvc.MockDataFlowComponentService, serviceName string, state string) {
	switch state {
	case dataflowcomponentfsm.OperationalStateStopped:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 false,
			BenthosFSMState:                  benthosfsmtype.OperationalStateStopped,
			IsBenthosProcessingMetricsActive: false,
		})
	case dataflowcomponentfsm.OperationalStateStarting:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 false,
			BenthosFSMState:                  benthosfsmtype.OperationalStateStarting,
			IsBenthosProcessingMetricsActive: false,
		})
	case dataflowcomponentfsm.OperationalStateIdle:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 true,
			BenthosFSMState:                  benthosfsmtype.OperationalStateIdle,
			IsBenthosProcessingMetricsActive: false,
		})
	case dataflowcomponentfsm.OperationalStateActive:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 true,
			BenthosFSMState:                  benthosfsmtype.OperationalStateActive,
			IsBenthosProcessingMetricsActive: true,
		})
	case dataflowcomponentfsm.OperationalStateDegraded:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 true,
			BenthosFSMState:                  benthosfsmtype.OperationalStateDegraded,
			IsBenthosProcessingMetricsActive: false,
		})
	case dataflowcomponentfsm.OperationalStateStopping:
		SetupDataflowComponentServiceState(mockService, serviceName, dataflowcomponentsvc.ComponentStateFlags{
			IsBenthosRunning:                 false,
			BenthosFSMState:                  benthosfsmtype.OperationalStateStopping,
			IsBenthosProcessingMetricsActive: false,
		})
	}
}

// SetupDataflowComponentInstance creates and configures a DataflowComponent instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupDataflowComponentInstance(serviceName string, desiredState string) (*dataflowcomponentfsm.DataflowComponentInstance, *dataflowcomponentsvc.MockDataFlowComponentService, config.DataFlowComponentConfig) {
	// Create test config
	cfg := CreateDataflowComponentTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := dataflowcomponentsvc.NewMockDataFlowComponentService()

	// Set up initial service states
	mockService.ExistingComponents = make(map[string]bool)
	mockService.ComponentStates = make(map[string]*dataflowcomponentsvc.ServiceInfo)

	// Configure service with default config
	ConfigureDataflowComponentServiceConfig(mockService)

	// Add default service info
	mockService.ComponentStates[serviceName] = &dataflowcomponentsvc.ServiceInfo{}

	// Create new instance
	instance := setUpMockDataflowComponentInstance(cfg, mockService)

	return instance, mockService, cfg
}

// setUpMockDataflowComponentInstance creates a DataflowComponentInstance with a mock service
// This is an internal helper function used by SetupDataflowComponentInstance
func setUpMockDataflowComponentInstance(
	cfg config.DataFlowComponentConfig,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
) *dataflowcomponentfsm.DataflowComponentInstance {
	// Create the instance
	instance := dataflowcomponentfsm.NewDataflowComponentInstance("", cfg)

	// Set the mock service
	instance.SetService(mockService)
	return instance
}

// TestDataflowComponentStateTransition tests a transition from one state to another.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The DataflowComponentInstance to reconcile
//   - mockService: The mock service to use
//   - filesystemService: The filesystem service to use
//   - serviceName: The name of the service instance
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestDataflowComponentStateTransition(
	ctx context.Context,
	instance *dataflowcomponentfsm.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	services serviceregistry.Provider,
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
	TransitionToDataflowComponentState(mockService, serviceName, toState)

	// 3. Reconcile until we reach the target state or exhaust attempts
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == toState {
			return tick, nil // Success!
		}

		// Perform a reconcile cycle
		_, _ = instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, services)
		tick++
	}

	// Did not reach the target state
	return startTick + uint64(maxAttempts), fmt.Errorf(
		"failed to transition from %s to %s after %d attempts; current state: %s",
		fromState, toState, maxAttempts, instance.GetCurrentFSMState())
}

// VerifyDataflowComponentStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The DataflowComponentInstance to reconcile
//   - mockService: The mock service to use
//   - filesystemService: The filesystem service to use
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyDataflowComponentStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *dataflowcomponentfsm.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	services serviceregistry.Provider,
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
	TransitionToDataflowComponentState(mockService, serviceName, expectedState)

	// Execute reconcile cycles and check state stability
	tick := snapshot.Tick
	for i := 0; i < numCycles; i++ {
		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++

		if instance.GetCurrentFSMState() != expectedState {
			return tick, fmt.Errorf(
				"state changed from %s to %s during cycle %d/%d",
				expectedState, instance.GetCurrentFSMState(), i+1, numCycles)
		}
	}

	return tick, nil
}

// ResetDataflowComponentInstanceError resets the error and backoff state of a DataflowComponentInstance.
// This is useful in tests to clear error conditions.
//
// Parameters:
//   - mockService: The mock service to reset
func ResetDataflowComponentInstanceError(mockService *dataflowcomponentsvc.MockDataFlowComponentService) {
	// Clear any error conditions in the mock
	mockService.AddDataFlowComponentToBenthosManagerError = nil
	mockService.StartDataFlowComponentError = nil
	mockService.StopDataFlowComponentError = nil
	mockService.UpdateDataFlowComponentInBenthosManagerError = nil
	mockService.RemoveDataFlowComponentFromBenthosManagerError = nil
	mockService.ForceRemoveDataFlowComponentError = nil
}

// CreateMockDataflowComponentInstance creates a DataflowComponent instance for testing.
// It sets up a new instance with a mock service.
func CreateMockDataflowComponentInstance(
	serviceName string,
	mockService dataflowcomponentsvc.IDataFlowComponentService,
	desiredState string,
) *dataflowcomponentfsm.DataflowComponentInstance {
	cfg := CreateDataflowComponentTestConfig(serviceName, desiredState)
	instance := dataflowcomponentfsm.NewDataflowComponentInstance("", cfg)
	instance.SetService(mockService)
	return instance
}

// StabilizeDataflowComponentInstance ensures the DataflowComponent instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The DataflowComponentInstance to stabilize
//   - mockService: The mock service to use
//   - filesystemService: The filesystem service to use
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeDataflowComponentInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *dataflowcomponentfsm.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToDataflowComponentState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyDataflowComponentStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForDataflowComponentDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The DataflowComponentInstance to monitor
//   - filesystemService: The filesystem service to use
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForDataflowComponentDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *dataflowcomponentfsm.DataflowComponentInstance,
	services serviceregistry.Provider,
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
		err, _ := instance.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}

		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach desired state %s after %d attempts; current desired state: %s",
		targetState, maxAttempts, instance.GetDesiredFSMState())
}

// ReconcileDataflowComponentUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The DataflowComponentInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - filesystemService: The filesystem service to use
//   - serviceName: The name of the service
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileDataflowComponentUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *dataflowcomponentfsm.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	services serviceregistry.Provider,
	serviceName string,
	maxAttempts int,
) (uint64, error, bool) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Perform a reconcile cycle and capture the error and reconciled status
		err, reconciled := instance.Reconcile(ctx, snapshot, services)
		tick++

		if err != nil {
			// Error found, return it along with the current tick
			return tick, err, reconciled
		}
	}

	// No error found after maxAttempts
	return tick, nil, true
}
