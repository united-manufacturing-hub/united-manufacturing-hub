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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmtype "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateTopicBrowserTestConfig creates a standard TopicBrowser config for testing
func CreateTopicBrowserTestConfig(name string, desiredState string) config.TopicBrowserConfig {
	return config.TopicBrowserConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ServiceConfig: topicbrowserserviceconfig.Config{},
	}
}

// SetupTopicBrowserServiceState configures the mock service state for TopicBrowser instance tests
func SetupTopicBrowserServiceState(
	mockService *topicbrowsersvc.MockService,
	serviceName string,
	flags topicbrowsersvc.StateFlags,
) {
	// Ensure service exists in mock
	mockService.Existing[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.States[serviceName] == nil {
		mockService.States[serviceName] = &topicbrowsersvc.ServiceInfo{}
	}

	// Set the Benthos FSM state
	if flags.BenthosFSMState != "" {
		mockService.States[serviceName].BenthosFSMState = flags.BenthosFSMState
	}

	// Store the service state flags directly
	mockService.SetState(serviceName, flags)
}

// ConfigureTopicBrowserServiceConfig configures the mock service with a default TopicBrowser config
func ConfigureTopicBrowserServiceConfig(mockService *topicbrowsersvc.MockService) {
	// TODO: add default config, probably from generate config
	// mockService.GetConfigResult = topicbrowserserviceconfig.Config{}
}

// TransitionToTopicBrowserState is a helper to configure a service for a given high-level state
func TransitionToTopicBrowserState(mockService *topicbrowsersvc.MockService, serviceName string, state string) {
	switch state {
	case dataflowcomponentfsm.OperationalStateStopped:
		SetupTopicBrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsmtype.OperationalStateStopped,
			RedpandaFSMState:      redpandafsm.OperationalStateStopped,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	case dataflowcomponentfsm.OperationalStateStarting:
		SetupTopicBrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsmtype.OperationalStateStarting,
			RedpandaFSMState:      redpandafsm.OperationalStateStarting,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	case dataflowcomponentfsm.OperationalStateActive:
		SetupTopicBrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsmtype.OperationalStateActive,
			RedpandaFSMState:      redpandafsm.OperationalStateActive,
			HasProcessingActivity: true,
			HasBenthosOutput:      true,
		})
	case dataflowcomponentfsm.OperationalStateDegraded:
		SetupTopicBrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsmtype.OperationalStateDegraded,
			RedpandaFSMState:      redpandafsm.OperationalStateActive,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	case dataflowcomponentfsm.OperationalStateStopping:
		SetupTopicBrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsmtype.OperationalStateStopping,
			RedpandaFSMState:      redpandafsm.OperationalStateActive,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	}
}

// SetupTopicBrowserInstance creates and configures a TopicBrowser instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupTopicBrowserInstance(serviceName string, desiredState string) (*topicbrowserfsm.Instance, *topicbrowsersvc.MockService, config.TopicBrowserConfig) {
	// Create test config
	cfg := CreateTopicBrowserTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := topicbrowsersvc.NewMockService()

	// Set up initial service states
	mockService.Existing = make(map[string]bool)
	mockService.States = make(map[string]*topicbrowsersvc.ServiceInfo)

	// Configure service with default config
	ConfigureTopicBrowserServiceConfig(mockService)

	// Add default service info
	mockService.States[serviceName] = &topicbrowsersvc.ServiceInfo{}

	// Add mock service registry
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	// Create new instance
	instance := setUpMockTopicBrowserInstance(cfg, mockService, mockSvcRegistry)

	return instance, mockService, cfg
}

// setUpMockTopicBrowserInstance creates a TopicBrowserInstance with a mock service
// This is an internal helper function used by SetupTopicBrowserInstance
func setUpMockTopicBrowserInstance(
	cfg config.TopicBrowserConfig,
	mockService *topicbrowsersvc.MockService,
	mockSvcRegistry *serviceregistry.Registry,
) *topicbrowserfsm.Instance {
	// Create the instance
	instance := topicbrowserfsm.NewInstance("", cfg)

	// Set the mock service
	instance.SetService(mockService)
	return instance
}

// TestTopicBrowserStateTransition tests a transition from one state to another.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicBrowserInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry to use
//   - serviceName: The name of the service instance
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestTopicBrowserStateTransition(
	ctx context.Context,
	instance *topicbrowserfsm.Instance,
	mockService *topicbrowsersvc.MockService,
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
	TransitionToTopicBrowserState(mockService, serviceName, toState)

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

// VerifyTopicBrowserStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The TopicBrowserInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry to use
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyTopicBrowserStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *topicbrowserfsm.Instance,
	mockService *topicbrowsersvc.MockService,
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
	TransitionToTopicBrowserState(mockService, serviceName, expectedState)

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
func ResetTopicBrowserInstanceError(mockService *topicbrowsersvc.MockService) {
	// Clear any error conditions in the mock
	mockService.AddToManagerError = nil
	mockService.StartError = nil
	mockService.StopError = nil
	mockService.UpdateInManagerError = nil
	mockService.RemoveFromManagerError = nil
	mockService.ForceRemoveError = nil
}

// CreateMockTopicBrowserInstance creates a TopicBrowser instance for testing.
// It sets up a new instance with a mock service.
func CreateMockTopicBrowserInstance(
	serviceName string,
	mockService topicbrowsersvc.ITopicBrowserService,
	desiredState string,
	services serviceregistry.Provider,
) *topicbrowserfsm.Instance {
	cfg := CreateTopicBrowserTestConfig(serviceName, desiredState)
	instance := topicbrowserfsm.NewInstance("", cfg)
	instance.SetService(mockService)
	return instance
}

// StabilizeTopicBrowserInstance ensures the TopicBrowser instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The TopicBrowserInstance to stabilize
//   - mockService: The mock service to use
//   - services: The service registry to use
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeTopicBrowserInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *topicbrowserfsm.Instance,
	mockService *topicbrowsersvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToTopicBrowserState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyTopicBrowserStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForTopicBrowserDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The TopicBrowserInstance to monitor
//   - services: The service registry to use
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForTopicBrowserDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *topicbrowserfsm.Instance,
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

// ReconcileTopicBrowserUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The TopicBrowserInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - services: The service registry to use
//   - serviceName: The name of the service
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileTopicBrowserUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *topicbrowserfsm.Instance,
	mockService *topicbrowsersvc.MockService,
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
