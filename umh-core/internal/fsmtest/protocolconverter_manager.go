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
	bridgefsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/bridge"
	bridgesvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// bridgeInstancePrefix is the consistent prefix used for bridge instance IDs
const bridgeInstancePrefix = "bridge"

// WaitForBridgeManagerStable waits for the manager to reach a stable state with all instances
func WaitForBridgeManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *bridgefsm.Manager,
	services serviceregistry.Provider,
) (uint64, error) {
	tick := snapshot.Tick
	maxAttempts := 10

	for i := 0; i < maxAttempts; i++ {
		// Create a copy of the snapshot with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++
	}

	return tick, nil
}

// WaitForBridgeInstanceState waits for an instance to reach a specific state
func WaitForBridgeInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *bridgefsm.Manager,
	services serviceregistry.Provider,
	instanceName string,
	expectedState string,
	maxAttempts int,
) (uint64, error) {
	// Same pattern as in other similar functions
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Create a new snapshot copy with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Get the instance and check its state
		instance, found := manager.GetInstance(fmt.Sprintf("%s-%s", bridgeInstancePrefix, instanceName))
		if found && instance.GetCurrentFSMState() == expectedState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s didn't reach expected state: %s", instanceName, expectedState)
}

// WaitForBridgeInstanceRemoval waits for an instance to be removed
func WaitForBridgeInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *bridgefsm.Manager,
	services serviceregistry.Provider,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Create a new snapshot copy with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if the instance is gone
		instances := manager.GetInstances()
		if _, exists := instances[instanceName]; !exists {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s was not removed after %d attempts", instanceName, maxAttempts)
}

// WaitForBridgeManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForBridgeManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *bridgefsm.Manager,
	services serviceregistry.Provider,
	desiredMap map[string]string, // e.g. { "conv1": "idle", "conv2": "active" }
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		// Create a copy of the snapshot with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		allMatched := true
		for conv, desired := range desiredMap {
			inst, found := manager.GetInstance(fmt.Sprintf("%s-%s", bridgeInstancePrefix, conv))
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

// SetupServiceInBridgeManager adds a service to the manager and configures it
func SetupServiceInBridgeManager(
	manager *bridgefsm.Manager,
	mockService *bridgesvc.MockService,
	converterName string,
	desiredState string,
	services serviceregistry.Provider,
) {
	// Create a properly configured instance
	instance := bridgefsm.NewInstance("", CreateBridgeTestConfig(converterName, desiredState))

	// Add it to the manager
	manager.AddInstanceForTest(fmt.Sprintf("%s-%s", bridgeInstancePrefix, converterName), instance)

	// Make sure the service exists in the mock service
	mockService.ExistingComponents[converterName] = true

	// Ensure we have a service info initialized
	if mockService.States[converterName] == nil {
		mockService.States[converterName] = &bridgesvc.ServiceInfo{}
	}
}

// CreateMockBridgeManager creates a Bridge manager with a mock service for testing
func CreateMockBridgeManager(name string) (*bridgefsm.Manager, *bridgesvc.MockService) {
	mockManager, mockService := bridgefsm.NewManagerWithMockServices(name)

	return mockManager, mockService
}

// ConfigureBridgeManagerForState sets up the mock service in a Bridge Manager to facilitate
// a state transition for a specific instance.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - name: The name of the bridge to configure
//   - targetState: The desired state to configure the service for
func ConfigureBridgeManagerForState(
	mockService *bridgesvc.MockService,
	name string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingComponents == nil {
		mockService.ExistingComponents = make(map[string]bool)
	}
	mockService.ExistingComponents[name] = true

	// Make sure service state is initialized
	if mockService.States == nil {
		mockService.States = make(map[string]*bridgesvc.ServiceInfo)
	}
	if mockService.States[name] == nil {
		mockService.States[name] = &bridgesvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToBridgeState(mockService, name, targetState)
}

// ReconcileOnceBridgeManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceBridgeManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *bridgefsm.Manager,
	services serviceregistry.Provider,
) (newTick uint64, err error, reconciled bool) {
	err, rec := manager.Reconcile(ctx, snapshot, services)
	return snapshot.Tick + 1, err, rec
}
