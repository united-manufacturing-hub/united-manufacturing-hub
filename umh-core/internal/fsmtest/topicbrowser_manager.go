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

package fsmtest

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// topicBrowserInstancePrefix is the consistent prefix used for protocol converter instance IDs
const topicBrowserInstancePrefix = "topicbrowser"

// WaitForTopicBrowserManagerStable waits for the manager to reach a stable state with all instances
func WaitForTopicBrowserManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
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

// WaitForTopicBrowserManagerInstanceState waits for an instance to reach a specific state
func WaitForTopicBrowserManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
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
		instance, found := manager.GetInstance(fmt.Sprintf("%s-%s", topicBrowserInstancePrefix, instanceName))
		if found && instance.GetCurrentFSMState() == expectedState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s didn't reach expected state: %s", instanceName, expectedState)
}

// WaitForTopicBrowserManagerInstanceRemoval waits for an instance to be removed
func WaitForTopicBrowserManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
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

// WaitForTopicBrowserManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForTopicBrowserManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
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
			inst, found := manager.GetInstance(fmt.Sprintf("%s-%s", topicBrowserInstancePrefix, conv))
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

// SetupServiceInTopicBrowserManager adds a service to the manager and configures it
func SetupServiceInTopicBrowserManager(
	manager *topicbrowserfsm.Manager,
	mockService *topicbrowsersvc.MockService,
	converterName string,
	desiredState string,
	services serviceregistry.Provider,
) {
	// Create a properly configured instance
	instance := topicbrowserfsm.NewInstance("", CreateTopicBrowserTestConfig(converterName, desiredState))

	// Add it to the manager
	manager.BaseFSMManager.AddInstanceForTest(fmt.Sprintf("%s-%s", topicBrowserInstancePrefix, converterName), instance)

	// Make sure the service exists in the mock service
	mockService.Existing[converterName] = true

	// Ensure we have a service info initialized
	if mockService.States[converterName] == nil {
		mockService.States[converterName] = &topicbrowsersvc.ServiceInfo{}
	}
}

// CreateMockTopicBrowserManager creates a TopicBrowser manager with a mock service for testing
func CreateMockTopicBrowserManager(name string) (*topicbrowserfsm.Manager, *topicbrowsersvc.MockService) {
	mockManager, mockService := topicbrowserfsm.NewTopicBrowserManagerWithMockedServices(name)

	return mockManager, mockService
}

// ConfigureTopicBrowserManagerForState sets up the mock service in a TopicBrowserManager to facilitate
// a state transition for a specific instance.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - converterName: The name of the converter to configure
//   - targetState: The desired state to configure the service for
func ConfigureTopicBrowserManagerForState(
	mockService *topicbrowsersvc.MockService,
	converterName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.Existing == nil {
		mockService.Existing = make(map[string]bool)
	}
	mockService.Existing[converterName] = true

	// Make sure service state is initialized
	if mockService.States == nil {
		mockService.States = make(map[string]*topicbrowsersvc.ServiceInfo)
	}
	if mockService.States[converterName] == nil {
		mockService.States[converterName] = &topicbrowsersvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToTopicBrowserState(mockService, converterName, targetState)
}

// ReconcileOnceTopicBrowserManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceTopicBrowserManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *topicbrowserfsm.Manager,
	services serviceregistry.Provider,
) (newTick uint64, err error, reconciled bool) {
	err, rec := manager.Reconcile(ctx, snapshot, services)
	return snapshot.Tick + 1, err, rec
}
