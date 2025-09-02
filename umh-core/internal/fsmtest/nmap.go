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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateNmapTestConfig creates a standard Nmap config for testing
func CreateNmapTestConfig(name string, desiredState string) config.NmapConfig {
	return config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: "localhost",
			Port:   443,
		},
	}
}

// SetupNmapServiceState configures the mock service state for Nmap instance tests
func SetupNmapServiceState(
	mockService *nmap.MockNmapService,
	serviceName string,
	flags nmap.ServiceStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingServices[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}
	}

	// Set S6 FSM state
	if flags.S6FSMState != "" {
		mockService.ServiceStates[serviceName].S6FSMState = flags.S6FSMState
	}

	// Update S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6_shared.ServiceInfo{
			Status: s6_shared.ServiceUp,
			Uptime: 10, // Set uptime to 10s to simulate config loaded
			Pid:    1234,
		}
	} else {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6_shared.ServiceInfo{
			Status: s6_shared.ServiceDown,
		}
	}

	// Update health check status
	if flags.IsRunning {
		mockService.ServiceStates[serviceName].NmapStatus.IsRunning = true
	} else {
		mockService.ServiceStates[serviceName].NmapStatus.IsRunning = false
	}

	if flags.PortState != "" {
		mockService.ServiceStates[serviceName].NmapStatus.LastScan = &nmap.NmapScanResult{
			PortResult: nmap.PortResult{
				State: flags.PortState,
			},
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// ConfigureNmapServiceConfig configures the mock service with a default Nmap config
func ConfigureNmapServiceConfig(mockService *nmap.MockNmapService) {
	mockService.GetConfigResult = nmapserviceconfig.NmapServiceConfig{
		Target: "localhost",
		Port:   443,
	}
}

// TransitionToNmapState is a helper to configure a service for a given high-level state
func TransitionToNmapState(mockService *nmap.MockNmapService, serviceName string, state string) {
	switch state {
	case nmapfsm.OperationalStateStopped,
		nmapfsm.OperationalStateStarting:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: false,
			S6FSMState:  s6fsm.OperationalStateStopped,
		})
	case nmapfsm.OperationalStateDegraded:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   false,
			S6FSMState:  s6fsm.OperationalStateRunning,
			IsDegraded:  true,
			PortState:   "",
		})
	case nmapfsm.OperationalStateOpen:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateOpen),
		})
	case nmapfsm.OperationalStateOpenFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateOpenFiltered),
		})
	case nmapfsm.OperationalStateFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateFiltered),
		})
	case nmapfsm.OperationalStateUnfiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateUnfiltered),
		})
	case nmapfsm.OperationalStateClosed:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateClosed),
		})
	case nmapfsm.OperationalStateClosedFiltered:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: true,
			IsRunning:   true,
			S6FSMState:  s6fsm.OperationalStateRunning,
			PortState:   string(nmapfsm.PortStateClosedFiltered),
		})
	case nmapfsm.OperationalStateStopping:
		SetupNmapServiceState(mockService, serviceName, nmap.ServiceStateFlags{
			IsS6Running: false,
			IsRunning:   false,
			S6FSMState:  s6fsm.OperationalStateStopping,
		})
	}
}

// SetupNmapInstance creates and configures a Nmap instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupNmapInstance(serviceName string, desiredState string) (*nmapfsm.NmapInstance, *nmap.MockNmapService, config.NmapConfig) {
	// Create test config
	cfg := CreateNmapTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := nmap.NewMockNmapService()

	// Set up initial service states
	mockService.ExistingServices = make(map[string]bool)
	mockService.ServiceStates = make(map[string]*nmap.ServiceInfo)

	// Configure service with default config
	ConfigureNmapServiceConfig(mockService)

	// Add default service info
	mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}

	// Create new instance directly using the specialized constructor
	instance := setUpMockNmapInstance(cfg, mockService)

	return instance, mockService, cfg
}

// setUpMockNmapInstance creates a NmapInstance with a mock service
func setUpMockNmapInstance(cfg config.NmapConfig, mockService *nmap.MockNmapService) *nmapfsm.NmapInstance {
	// First create the instance normally
	instance := nmapfsm.NewNmapInstance(cfg)

	// Set the mock service using the test utility method
	instance.SetService(mockService)

	return instance
}

// TestNmapStateTransition tests a transition from one state to another without directly calling Reconcile.
// Instead, it sets up the proper mock service conditions and then calls Reconcile in a controlled way.
func TestNmapStateTransition(
	ctx context.Context,
	instance *nmapfsm.NmapInstance,
	mockService *nmap.MockNmapService,
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
	TransitionToNmapState(mockService, serviceName, toState)

	// 4. Reconcile until we reach the target state or exhaust attempts
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

// VerifyNmapStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
func VerifyNmapStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *nmapfsm.NmapInstance,
	mockService *nmap.MockNmapService,
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
	TransitionToNmapState(mockService, serviceName, expectedState)

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

// ResetNmapInstanceError resets the error and backoff state of a NmapInstance.
func ResetNmapInstanceError(mockService *nmap.MockNmapService) {
	mockService.AddServiceError = nil
	mockService.StartServiceError = nil
	mockService.StopServiceError = nil
	mockService.UpdateServiceError = nil
	mockService.RemoveServiceError = nil
	mockService.ForceRemoveNmapError = nil
}

// CreateMockNmapInstance creates a Nmap instance for testing.
// Note: This creates a standard instance without replacing the service component.
// In actual tests, the pattern is to use the manager's functionality rather than
// working with individual instances.
func CreateMockNmapInstance(serviceName string, mockService nmap.INmapService, desiredState string) *nmapfsm.NmapInstance {
	cfg := CreateNmapTestConfig(serviceName, desiredState)
	return nmapfsm.NewNmapInstance(cfg)
}

// StabilizeNmapInstance ensures the Nmap instance reaches and remains in a stable state.
func StabilizeNmapInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *nmapfsm.NmapInstance,
	mockService *nmap.MockNmapService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToNmapState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyNmapStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		_, _ = instance.Reconcile(ctx, snapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForNmapDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
func WaitForNmapDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *nmapfsm.NmapInstance,
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

// ReconcileNmapUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
func ReconcileNmapUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *nmapfsm.NmapInstance,
	mockService *nmap.MockNmapService,
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
