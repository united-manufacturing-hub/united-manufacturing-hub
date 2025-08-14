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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	connectionsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// WaitForConnectionManagerStable is a simple helper that calls manager.Reconcile once
// and checks if there was an error. It's used for scenarios like an empty config
// where we just want to do one pass. Then we might call it multiple times if needed.
func WaitForConnectionManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *connectionfsm.ConnectionManager,
	services serviceregistry.Provider,
) (uint64, error) {
	err, _ := manager.Reconcile(ctx, snapshot, services)
	if err != nil {
		return snapshot.Tick, err
	}
	// We might do more checks if needed, e.g. manager has zero instances?
	return snapshot.Tick + 1, nil
}

// WaitForConnectionManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForConnectionManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *connectionfsm.ConnectionManager,
	services serviceregistry.Provider,
	instanceName string,
	desiredState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	currentState := ""

	for range maxAttempts {
		snapshot.Tick = tick

		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}

		tick++

		inst, found := manager.GetInstance("connection-" + instanceName)
		if found {
			currentState = inst.GetCurrentFSMState()
			if currentState == desiredState {
				return tick, nil
			}
		}
	}

	return tick, fmt.Errorf("instance %s did not reach state %s after %d attempts. Current state: %s",
		instanceName, desiredState, maxAttempts, currentState)
}

// WaitForConnectionManagerInstanceRemoval waits until the given serviceName is removed
// from the manager's instance map.
func WaitForConnectionManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *connectionfsm.ConnectionManager,
	services serviceregistry.Provider,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for range maxAttempts {
		err, _ := manager.Reconcile(ctx, snapshot, services)
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

// WaitForConnectionManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForConnectionManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *connectionfsm.ConnectionManager,
	services serviceregistry.Provider,
	desiredMap map[string]string, // e.g. { "comp1": "idle", "comp2": "active" }
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for range maxAttempts {
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}

		tick++

		allMatched := true

		for comp, desired := range desiredMap {
			inst, found := manager.GetInstance("connection-" + comp)
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

// SetupServiceInConnectionManager adds a service to the manager and configures it.
func SetupServiceInConnectionManager(
	manager *connectionfsm.ConnectionManager,
	mockService *connectionsvc.MockConnectionService,
	connectionName string,
	desiredState string,
) {
	// Create a properly configured instance
	instance := connectionfsm.NewConnectionInstance("", CreateConnectionTestConfig(connectionName, desiredState))

	// Add it to the manager
	manager.AddInstanceForTest(connectionName, instance)

	// Make sure the service exists in the mock service
	mockService.ExistingConnections[connectionName] = true

	// Ensure we have a service info initialized
	if mockService.ConnectionStates[connectionName] == nil {
		mockService.ConnectionStates[connectionName] = &connectionsvc.ServiceInfo{}
	}
}

// CreateMockConnectionManager creates a ConnectionManager with a mock service for testing.
func CreateMockConnectionManager(name string) (*connectionfsm.ConnectionManager, *connectionsvc.MockConnectionService) {
	mockManager, mockService := connectionfsm.NewConnectionManagerWithMockedServices(name)

	return mockManager, mockService
}

// ConfigureConnectionManagerForState sets up the mock service in a ConnectionManager to facilitate
// a state transition for a specific instance.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - connectionName: The name of the connection to configure
//   - targetState: The desired state to configure the service for
func ConfigureConnectionManagerForState(
	mockService *connectionsvc.MockConnectionService,
	connectionName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingConnections == nil {
		mockService.ExistingConnections = make(map[string]bool)
	}

	mockService.ExistingConnections[connectionName] = true

	// Make sure service state is initialized
	if mockService.ConnectionStates == nil {
		mockService.ConnectionStates = make(map[string]*connectionsvc.ServiceInfo)
	}

	if mockService.ConnectionStates[connectionName] == nil {
		mockService.ConnectionStates[connectionName] = &connectionsvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToConnectionState(mockService, connectionName, targetState)
}

// ReconcileOnceConnectionManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceConnectionManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *connectionfsm.ConnectionManager,
	services serviceregistry.Provider,
) (uint64, error, bool) {
	err, rec := manager.Reconcile(ctx, snapshot, services)

	return snapshot.Tick + 1, err, rec
}
