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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	connectionsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateConnectionTestConfig creates a standard Connection config for testing
func CreateConnectionTestConfig(name string, desiredState string) config.ConnectionConfig {
	return config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfig{
			NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
				Target: "localhost",
				Port:   443,
			},
		},
	}
}

// SetupConnectionServiceState configures the mock service state for Connection instance tests
func SetupConnectionServiceState(
	mockService *connectionsvc.MockConnectionService,
	serviceName string,
	flags connectionsvc.ConnectionStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingConnections[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ConnectionStates[serviceName] == nil {
		mockService.ConnectionStates[serviceName] = &connectionsvc.ServiceInfo{}
	}

	// Set the Benthos FSM state
	if flags.NmapFSMState != "" {
		mockService.ConnectionStates[serviceName].NmapFSMState = flags.NmapFSMState
	}

	// Store the service state flags directly
	mockService.SetConnectionState(serviceName, flags)
}

// ConfigureConnectionServiceConfig configures the mock service with a default Connection config
func ConfigureConnectionServiceConfig(mockService *connectionsvc.MockConnectionService) {
	mockService.GetConfigResult = connectionserviceconfig.ConnectionServiceConfig{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: "localhost",
			Port:   443,
		},
	}
}

// TransitionToConnectionState is a helper to configure a service for a given high-level state
func TransitionToConnectionState(mockService *connectionsvc.MockConnectionService, serviceName string, state string) {
	switch state {
	case connectionfsm.OperationalStateStopped:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: false,
			NmapFSMState:  nmapfsm.OperationalStateStopped,
		})
	case connectionfsm.OperationalStateStarting:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: false,
			NmapFSMState:  nmapfsm.OperationalStateStarting,
		})
	case connectionfsm.OperationalStateUp:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: true,
			NmapFSMState:  nmapfsm.OperationalStateOpen,
			IsFlaky:       false,
		})
	case connectionfsm.OperationalStateDegraded:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: true,
			NmapFSMState:  nmapfsm.OperationalStateOpen,
			IsFlaky:       true,
		})
	case connectionfsm.OperationalStateDown:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: true,
			NmapFSMState:  nmapfsm.OperationalStateClosed,
			IsFlaky:       false,
		})
	case connectionfsm.OperationalStateStopping:
		SetupConnectionServiceState(mockService, serviceName, connectionsvc.ConnectionStateFlags{
			IsNmapRunning: false,
			NmapFSMState:  nmapfsm.OperationalStateStopping,
			IsFlaky:       false,
		})
	}
}

// SetupConnectionInstance creates and configures a Connection instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupConnectionInstance(serviceName string, desiredState string) (*connectionfsm.ConnectionInstance, *connectionsvc.MockConnectionService, config.ConnectionConfig) {
	// Create test config
	cfg := CreateConnectionTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := connectionsvc.NewMockConnectionService()

	// Set up initial service states
	mockService.ExistingConnections = make(map[string]bool)
	mockService.ConnectionStates = make(map[string]*connectionsvc.ServiceInfo)

	// Configure service with default config
	ConfigureConnectionServiceConfig(mockService)

	// Add default service info
	mockService.ConnectionStates[serviceName] = &connectionsvc.ServiceInfo{}

	// Create new instance
	instance := setUpMockConnectionInstance(cfg, mockService)

	return instance, mockService, cfg
}

// setUpMockConnectionInstance creates a ConnectionInstance with a mock service
// This is an internal helper function used by SetupConnectionInstance
func setUpMockConnectionInstance(
	cfg config.ConnectionConfig,
	mockService *connectionsvc.MockConnectionService,
) *connectionfsm.ConnectionInstance {
	// Create the instance
	instance := connectionfsm.NewConnectionInstance("", cfg)

	// Set the mock service
	instance.SetService(mockService)
	return instance
}

// TestConnectionStateTransition tests a transition from one state to another.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The ConnectionInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry provider for accessing core services including filesystem
//   - serviceName: The name of the service instance
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestConnectionStateTransition(
	ctx context.Context,
	instance *connectionfsm.ConnectionInstance,
	mockService *connectionsvc.MockConnectionService,
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
	TransitionToConnectionState(mockService, serviceName, toState)

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

// VerifyConnectionStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ConnectionInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry provider for accessing core services including filesystem
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyConnectionStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *connectionfsm.ConnectionInstance,
	mockService *connectionsvc.MockConnectionService,
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
	TransitionToConnectionState(mockService, serviceName, expectedState)

	// Execute reconcile cycles and check state stability
	tick := snapshot.Tick
	for i := 0; i < numCycles; i++ {
		snapshot.Tick = tick
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

// ResetConnectionInstanceError resets the error and backoff state of a ConnectionInstance.
// This is useful in tests to clear error conditions.
//
// Parameters:
//   - mockService: The mock service to reset
func ResetConnectionInstanceError(mockService *connectionsvc.MockConnectionService) {
	// Clear any error conditions in the mock
	mockService.AddConnectionToNmapManagerError = nil
	mockService.StartConnectionError = nil
	mockService.StopConnectionError = nil
	mockService.UpdateConnectionInNmapManagerError = nil
	mockService.RemoveConnectionFromNmapManagerError = nil
	mockService.ForceRemoveConnectionError = nil
}

// CreateMockConnectionInstance creates a Connection instance for testing.
// It sets up a new instance with a mock service.
func CreateMockConnectionInstance(
	serviceName string,
	mockService connectionsvc.IConnectionService,
	desiredState string,
) *connectionfsm.ConnectionInstance {
	cfg := CreateConnectionTestConfig(serviceName, desiredState)
	instance := connectionfsm.NewConnectionInstance("", cfg)
	instance.SetService(mockService)
	return instance
}

// StabilizeConnectionInstance ensures the Connection instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ConnectionInstance to stabilize
//   - mockService: The mock service to use
//   - services: The service registry provider for accessing core services including filesystem
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeConnectionInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *connectionfsm.ConnectionInstance,
	mockService *connectionsvc.MockConnectionService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToConnectionState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		snapshot.Tick = tick
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyConnectionStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForConnectionDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ConnectionInstance to monitor
//   - services: The service registry provider for accessing core services including filesystem
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForConnectionDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *connectionfsm.ConnectionInstance,
	services serviceregistry.Provider,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		snapshot.Tick = tick
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

// ReconcileConnectionUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ConnectionInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - services: The service registry provider for accessing core services including filesystem
//   - serviceName: The name of the service
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileConnectionUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *connectionfsm.ConnectionInstance,
	mockService *connectionsvc.MockConnectionService,
	servies serviceregistry.Provider,
	serviceName string,
	maxAttempts int,
) (uint64, error, bool) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Perform a reconcile cycle and capture the error and reconciled status
		snapshot.Tick = tick
		err, reconciled := instance.Reconcile(ctx, snapshot, servies)
		tick++

		if err != nil {
			// Error found, return it along with the current tick
			return tick, err, reconciled
		}
	}

	// No error found after maxAttempts
	return tick, nil, true
}
