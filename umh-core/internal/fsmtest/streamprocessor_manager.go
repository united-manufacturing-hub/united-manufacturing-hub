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
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// streamProcessorInstancePrefix is the consistent prefix used for stream processor instance IDs.
const streamProcessorInstancePrefix = "streamprocessor"

// WaitForStreamProcessorManagerStable waits for the manager to reach a stable state with all instances.
func WaitForStreamProcessorManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *spfsm.Manager,
	services serviceregistry.Provider,
) (uint64, error) {
	tick := snapshot.Tick
	maxAttempts := 10

	for range maxAttempts {
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

// WaitForStreamProcessorManagerInstanceState waits for an instance to reach a specific state.
func WaitForStreamProcessorManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *spfsm.Manager,
	services serviceregistry.Provider,
	instanceName string,
	expectedState string,
	maxAttempts int,
) (uint64, error) {
	// Same pattern as in other similar functions
	tick := snapshot.Tick

	for range maxAttempts {
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
		instance, found := manager.GetInstance(fmt.Sprintf("%s-%s", streamProcessorInstancePrefix, instanceName))
		if found && instance.GetCurrentFSMState() == expectedState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s didn't reach expected state: %s", instanceName, expectedState)
}

// WaitForStreamProcessorManagerInstanceRemoval waits for an instance to be removed.
func WaitForStreamProcessorManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *spfsm.Manager,
	services serviceregistry.Provider,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for range maxAttempts {
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

// WaitForStreamProcessorManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForStreamProcessorManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *spfsm.Manager,
	services serviceregistry.Provider,
	desiredMap map[string]string, // e.g. { "proc1": "idle", "proc2": "active" }
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for range maxAttempts {
		// Create a copy of the snapshot with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}

		tick++

		allMatched := true

		for proc, desired := range desiredMap {
			inst, found := manager.GetInstance(fmt.Sprintf("%s-%s", streamProcessorInstancePrefix, proc))
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

// SetupServiceInStreamProcessorManager adds a service to the manager and configures it.
func SetupServiceInStreamProcessorManager(
	manager *spfsm.Manager,
	mockService *spsvc.MockService,
	processorName string,
	desiredState string,
	services serviceregistry.Provider,
) {
	// Create a properly configured instance
	instance := spfsm.NewInstance("", CreateStreamProcessorTestConfig(processorName, desiredState))

	// Add it to the manager
	manager.AddInstanceForTest(fmt.Sprintf("%s-%s", streamProcessorInstancePrefix, processorName), instance)

	// Make sure the service exists in the mock service
	mockService.ExistingComponents[processorName] = true

	// Ensure we have a service info initialized
	if mockService.States[processorName] == nil {
		mockService.States[processorName] = &spsvc.ServiceInfo{}
	}
}

// CreateMockStreamProcessorManager creates a StreamProcessor manager with a mock service for testing.
func CreateMockStreamProcessorManager(name string) (*spfsm.Manager, *spsvc.MockService) {
	mockManager, mockService := spfsm.NewManagerWithMockedServices(name)

	return mockManager, mockService
}

// ConfigureStreamProcessorManagerForState sets up the mock service in a StreamProcessorManager to facilitate
// a state transition for a specific instance.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - processorName: The name of the processor to configure
//   - targetState: The desired state to configure the service for
func ConfigureStreamProcessorManagerForState(
	mockService *spsvc.MockService,
	processorName string,
	targetState string,
) {
	// Configure the service for the desired state
	TransitionToStreamProcessorState(mockService, processorName, targetState)
}

// ReconcileOnceStreamProcessorManager performs a single reconciliation cycle on the manager.
func ReconcileOnceStreamProcessorManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *spfsm.Manager,
	services serviceregistry.Provider,
) (uint64, error, bool) {
	err, reconciled := manager.Reconcile(ctx, snapshot, services)

	return snapshot.Tick + 1, err, reconciled
}
