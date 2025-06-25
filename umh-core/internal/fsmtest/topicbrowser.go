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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	rpfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateTopicbrowserTestConfig creates a standard Topicbrowser config for testing
func CreateTopicbrowserTestConfig(name string, desiredState string) config.TopicBrowserConfig {
	return config.TopicBrowserConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ServiceConfig: topicbrowserserviceconfig.Config{},
	}
}

// SetupTopicbrowserServiceState configures the mock service state for Topicbrowser instance tests
func SetupTopicbrowserServiceState(
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

	// Store the service state flags directly
	mockService.SetComponentState(serviceName, flags)
}

// TransitionToTopicbrowserState is a helper to configure a service for a given high-level state
func TransitionToTopicbrowserState(mockService *topicbrowsersvc.MockService, serviceName string, state string) {
	switch state {
	case topicbrowserfsm.OperationalStateStopped:
		SetupTopicbrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsm.OperationalStateStopped,
			RedpandaFSMState:      rpfsm.OperationalStateActive,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	case topicbrowserfsm.OperationalStateStarting:
		SetupTopicbrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsm.OperationalStateStarting,
			RedpandaFSMState:      rpfsm.OperationalStateActive,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	case topicbrowserfsm.OperationalStateActive:
		SetupTopicbrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsm.OperationalStateActive,
			RedpandaFSMState:      rpfsm.OperationalStateActive,
			HasProcessingActivity: true,
			HasBenthosOutput:      true,
		})
	case topicbrowserfsm.OperationalStateDegraded:
		SetupTopicbrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsm.OperationalStateDegraded,
			RedpandaFSMState:      rpfsm.OperationalStateActive,
			HasProcessingActivity: true,
			HasBenthosOutput:      false,
		})
	case topicbrowserfsm.OperationalStateStopping:
		SetupTopicbrowserServiceState(mockService, serviceName, topicbrowsersvc.StateFlags{
			BenthosFSMState:       benthosfsm.OperationalStateStopping,
			RedpandaFSMState:      rpfsm.OperationalStateActive,
			HasProcessingActivity: false,
			HasBenthosOutput:      false,
		})
	}
}

// SetupTopicbrowserInstance creates and configures a Topicbrowser instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupTopicbrowserInstance(serviceName string, desiredState string) (*topicbrowserfsm.Instance, *topicbrowsersvc.MockService, config.TopicBrowserConfig) {
	// Create test config
	cfg := CreateTopicbrowserTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := topicbrowsersvc.NewMockDataFlowComponentService()

	// Set up initial service states
	mockService.Existing = make(map[string]bool)
	mockService.States = make(map[string]*topicbrowsersvc.ServiceInfo)

	// Add default service info
	mockService.States[serviceName] = &topicbrowsersvc.ServiceInfo{}

	// Create new instance directly using the specialized constructor
	instance := setUpMockTopicbrowserInstance(cfg, mockService)

	return instance, mockService, cfg
}

// setUpMockTopicbrowserInstance creates a TopicbrowserInstance with a mock service
// This is an internal helper function used by SetupTopicbrowserInstance
func setUpMockTopicbrowserInstance(cfg config.TopicBrowserConfig, mockService *topicbrowsersvc.MockService) *topicbrowserfsm.Instance {
	// First create the instance normally
	instance := topicbrowserfsm.NewInstance("", cfg)

	// Set the mock service using the test utility method
	instance.SetService(mockService)

	return instance
}

// TestTopicbrowserStateTransition tests a transition from one state to another without directly calling Reconcile.
// Instead, it sets up the proper mock service conditions and then calls Reconcile in a controlled way.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicbrowserInstance to reconcile
//   - mockService: The mock service to use (since we can't get it from the instance)
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestTopicbrowserStateTransition(
	ctx context.Context,
	instance *topicbrowserfsm.Instance,
	mockService *topicbrowsersvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	fromState string,
	toState string,
	maxAttempts int,
	startTick uint64,
	startTime time.Time,
) (uint64, error) {
	// 1. Verify the instance is in the expected starting state
	if instance.GetCurrentFSMState() != fromState {
		return startTick, fmt.Errorf("instance is in state %s, not in expected starting state %s",
			instance.GetCurrentFSMState(), fromState)
	}

	// 2. Set up the mock service for the target state
	TransitionToTopicbrowserState(mockService, serviceName, toState)

	// 4. Reconcile until we reach the target state or exhaust attempts
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == toState {
			return tick, nil // Success!
		}

		// Perform a reconcile cycle
		// the current time is is the start time * the amount of ticks that have passed
		currentTime := startTime.Add(time.Duration(tick) * constants.DefaultTickerTime)
		_, _ = instance.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick, SnapshotTime: currentTime}, services)
		tick++
	}

	// Did not reach the target state
	return startTick + uint64(maxAttempts), fmt.Errorf(
		"failed to transition from %s to %s after %d attempts; current state: %s",
		fromState, toState, maxAttempts, instance.GetCurrentFSMState())
}

// VerifyTopicbrowserStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicbrowserInstance to reconcile
//   - mockService: The mock service to use (since we can't get it from the instance)
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyTopicbrowserStableState(
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
	TransitionToTopicbrowserState(mockService, serviceName, expectedState)

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

// ResetTopicbrowserInstanceError resets the error and backoff state of a TopicbrowserInstance.
// This is useful in tests to clear error conditions.
//
// Parameters:
//   - mockService: The mock service to reset
func ResetTopicbrowserInstanceError(mockService *topicbrowsersvc.MockService) {
	// Clear any error conditions in the mock
	mockService.AddToManagerError = nil
	mockService.StartError = nil
	mockService.StopError = nil
	mockService.UpdateInManagerError = nil
	mockService.RemoveFromManagerError = nil
	mockService.ForceRemoveError = nil
}

// CreateMockTopicbrowserInstance creates a Topicbrowser instance for testing.
// Note: This creates a standard instance without replacing the service component.
// In actual tests, the pattern is to use the manager's functionality rather than
// working with individual instances.
func CreateMockTopicbrowserInstance(serviceName string, mockService topicbrowsersvc.ITopicBrowserService, desiredState string) *topicbrowserfsm.Instance {
	cfg := CreateTopicbrowserTestConfig(serviceName, desiredState)
	return topicbrowserfsm.NewInstance("", cfg)
}

// StabilizeTopicbrowserInstance ensures the Topicbrowser instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicbrowserInstance to stabilize
//   - mockService: The mock service to use
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeTopicbrowserInstance(
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
	TransitionToTopicbrowserState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyTopicbrowserStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForTopicbrowserDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicbrowserInstance to monitor
//   - startTick: The starting tick value for reconciliation
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForTopicbrowserDesiredState(
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

// ReconcileTopicbrowserUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The TopicbrowserInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - serviceName: The name of the service
//   - startTick: The starting tick value for reconciliation
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileTopicbrowserUntilError(
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

// === MANAGER TEST HELPERS ===

// CreateMockTopicBrowserManager creates a TopicBrowser manager with a mock service for testing
func CreateMockTopicBrowserManager(managerName string) (*topicbrowserfsm.Manager, *topicbrowsersvc.MockService) {
	manager := topicbrowserfsm.NewTopicBrowserManager(managerName)
	mockService := topicbrowsersvc.NewMockDataFlowComponentService()
	return manager, mockService
}

// ConfigureTopicBrowserManagerForState configures the mock service for manager-level testing
func ConfigureTopicBrowserManagerForState(mockService *topicbrowsersvc.MockService, tbName string, targetState string) {
	// For manager tests, we configure the service to respond as if it's in the target state
	TransitionToTopicbrowserState(mockService, tbName, targetState)
}

// WaitForTopicBrowserManagerStable waits for the manager to reach a stable state with no reconciliation activity
func WaitForTopicBrowserManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
	services serviceregistry.Provider,
) (uint64, error) {
	maxAttempts := 10
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, reconciled := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// If no reconciliation happened, we're stable
		if !reconciled {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("manager did not stabilize after %d attempts", maxAttempts)
}

// WaitForTopicBrowserManagerInstanceState waits for a specific instance to reach a target state
func WaitForTopicBrowserManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
	services serviceregistry.Provider,
	tbName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if the instance exists and is in the target state
		if instance, exists := manager.GetInstance(tbName); exists {
			if instance.GetCurrentFSMState() == targetState {
				return tick, nil
			}
		}
	}

	// Get current state for error message
	currentState := "not_found"
	if instance, exists := manager.GetInstance(tbName); exists {
		currentState = instance.GetCurrentFSMState()
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, currentState)
}

// WaitForTopicBrowserManagerInstanceRemoval waits for an instance to be completely removed from the manager
func WaitForTopicBrowserManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
	services serviceregistry.Provider,
	tbName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if the instance no longer exists
		if _, exists := manager.GetInstance(tbName); !exists {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s was not removed after %d attempts", tbName, maxAttempts)
}

// WaitForTopicBrowserManagerMultiState waits for multiple instances to reach their desired states
func WaitForTopicBrowserManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
	services serviceregistry.Provider,
	desiredStates map[string]string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if all instances are in their desired states
		allReached := true
		for tbName, targetState := range desiredStates {
			if instance, exists := manager.GetInstance(tbName); !exists || instance.GetCurrentFSMState() != targetState {
				allReached = false
				break
			}
		}

		if allReached {
			return tick, nil
		}
	}

	// Build error message with current states
	var currentStates []string
	for tbName, targetState := range desiredStates {
		currentState := "not_found"
		if instance, exists := manager.GetInstance(tbName); exists {
			currentState = instance.GetCurrentFSMState()
		}
		currentStates = append(currentStates, fmt.Sprintf("%s: %s (wanted %s)", tbName, currentState, targetState))
	}

	return tick, fmt.Errorf("not all instances reached desired states after %d attempts: %v", maxAttempts, currentStates)
}
