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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// WaitForDataflowComponentManagerStable is a simple helper that calls manager.Reconcile once
// and checks if there was an error. It's used for scenarios like an empty config
// where we just want to do one pass. Then we might call it multiple times if needed.
func WaitForDataflowComponentManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *dataflowcomponentfsm.DataflowComponentManager,
	services serviceregistry.Provider,
) (uint64, error) {
	err, _ := manager.Reconcile(ctx, snapshot, services)

	if err != nil {
		return snapshot.Tick, err
	}
	// We might do more checks if needed, e.g. manager has zero instances?
	return snapshot.Tick + 1, nil
}

// WaitForDataflowComponentManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForDataflowComponentManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *dataflowcomponentfsm.DataflowComponentManager,
	services serviceregistry.Provider,
	instanceName string,
	desiredState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	currentState := ""
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		inst, found := manager.GetInstance(fmt.Sprintf("dataflow-%s", instanceName))
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

// WaitForDataflowComponentManagerInstanceRemoval waits until the given serviceName is removed
// from the manager's instance map.
func WaitForDataflowComponentManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *dataflowcomponentfsm.DataflowComponentManager,
	services serviceregistry.Provider,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
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

// WaitForDataflowComponentManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForDataflowComponentManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *dataflowcomponentfsm.DataflowComponentManager,
	services serviceregistry.Provider,
	desiredMap map[string]string, // e.g. { "comp1": "idle", "comp2": "active" }
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		allMatched := true
		for comp, desired := range desiredMap {
			inst, found := manager.GetInstance(fmt.Sprintf("dataflow-%s", comp))
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

// SetupServiceInDataflowComponentManager adds a service to the manager and configures it
func SetupServiceInDataflowComponentManager(
	manager *dataflowcomponentfsm.DataflowComponentManager,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	componentName string,
	desiredState string,
) {
	// Create a properly configured instance
	instance := dataflowcomponentfsm.NewDataflowComponentInstance("", CreateDataflowComponentTestConfig(componentName, desiredState))

	// Add it to the manager
	manager.BaseFSMManager.AddInstanceForTest(componentName, instance)

	// Make sure the service exists in the mock service
	mockService.ExistingComponents[componentName] = true

	// Ensure we have a service info initialized
	if mockService.ComponentStates[componentName] == nil {
		mockService.ComponentStates[componentName] = &dataflowcomponentsvc.ServiceInfo{}
	}
}

// CreateMockDataflowComponentManager creates a DataflowComponent manager with a mock service for testing
func CreateMockDataflowComponentManager(name string) (*dataflowcomponentfsm.DataflowComponentManager, *dataflowcomponentsvc.MockDataFlowComponentService) {
	mockManager, mockService := dataflowcomponentfsm.NewDataflowComponentManagerWithMockedServices(name)

	return mockManager, mockService
}

// ConfigureDataflowComponentManagerForState sets up the mock service in a DataflowComponentManager to facilitate
// a state transition for a specific instance.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - componentName: The name of the component to configure
//   - targetState: The desired state to configure the service for
func ConfigureDataflowComponentManagerForState(
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	componentName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingComponents == nil {
		mockService.ExistingComponents = make(map[string]bool)
	}
	mockService.ExistingComponents[componentName] = true

	// Make sure service state is initialized
	if mockService.ComponentStates == nil {
		mockService.ComponentStates = make(map[string]*dataflowcomponentsvc.ServiceInfo)
	}
	if mockService.ComponentStates[componentName] == nil {
		mockService.ComponentStates[componentName] = &dataflowcomponentsvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToDataflowComponentState(mockService, componentName, targetState)
}

// ReconcileOnceDataflowComponentManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceDataflowComponentManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *dataflowcomponentfsm.DataflowComponentManager,
	services serviceregistry.Provider,
) (newTick uint64, err error, reconciled bool) {
	err, rec := manager.Reconcile(ctx, snapshot, services)
	return snapshot.Tick + 1, err, rec
}
