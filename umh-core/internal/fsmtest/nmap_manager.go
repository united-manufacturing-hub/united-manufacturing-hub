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
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// WaitForNmapManagerStable is a simple helper that calls manager.Reconcile once
// and checks if there was an error. It's used for scenarios like an empty config
// where we just want to do one pass. Then we might call it multiple times if needed.
func WaitForNmapManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
	services serviceregistry.Provider,
) (uint64, error) {
	err, _ := manager.Reconcile(ctx, snapshot, services)
	if err != nil {
		return snapshot.Tick, err
	}
	// We might do more checks if needed, e.g. manager has zero instances?
	return snapshot.Tick + 1, nil
}

// WaitForNmapManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForNmapManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
	services serviceregistry.Provider,
	instanceName string,
	desiredState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		inst, found := manager.GetInstance(instanceName)
		if found && inst.GetCurrentFSMState() == desiredState {
			return tick, nil
		}
		if found {
			fmt.Printf("currentState: %s\n", inst.GetCurrentFSMState())
			fmt.Printf(" found instance: %s\n", instanceName)
		}
	}
	return tick, fmt.Errorf("instance %s did not reach state %s after %d attempts",
		instanceName, desiredState, maxAttempts)
}

// WaitForNmapManagerInstanceRemoval waits until the given serviceName is removed
// from the manager's instance map.
func WaitForNmapManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
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

// WaitForNmapManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForNmapManagerMultiState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
	services serviceregistry.Provider,
	desiredMap map[string]string, // e.g. { "svc1": "idle", "svc2": "active" }
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
		for svc, desired := range desiredMap {
			inst, found := manager.GetInstance(svc)
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

// SetupNmapServiceInManager adds a service to the manager and configures it with the mock service.
// It creates an instance automatically and adds it to the manager.
// Note: Since this cannot directly set the service field in the instance after creation,
// the instance in the manager will be using the default service, not the mock service.
// Tests should rely on the manager.Reconcile method instead, which uses the manager's own service.
func SetupNmapServiceInManager(
	manager *nmapfsm.NmapManager,
	mockService *nmap.MockNmapService,
	serviceName string,
	desiredState string,
) {
	// Create a properly configured instance
	instance := nmapfsm.NewNmapInstance(CreateNmapTestConfig(serviceName, desiredState))

	// Add it to the manager
	manager.AddInstanceForTest(serviceName, instance)

	// Make sure the service exists in the mock service
	mockService.ExistingServices[serviceName] = true

	// Ensure we have a service info initialized
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}
	}
}

// CreateMockNmapManager creates a Benthos manager with a mock service for testing
func CreateMockNmapManager(name string) (*nmapfsm.NmapManager, *nmap.MockNmapService) {
	// Use the specialized constructor that sets up fully mocked services
	manager, mockService := nmapfsm.NewNmapManagerWithMockedService(name)

	// Set up the mock service to prevent real filesystem operations
	s6MockService := mockService.S6Service.(*s6_default.MockService)

	// Configure default responses to prevent real filesystem operations
	s6MockService.CreateError = nil
	s6MockService.RemoveError = nil
	s6MockService.StartError = nil
	s6MockService.StopError = nil
	s6MockService.ForceRemoveError = nil

	// Set up the mock to say services exist after creation
	s6MockService.ServiceExistsResult = true

	// Configure default successful statuses
	s6MockService.StatusResult = s6_shared.ServiceInfo{
		Status: s6_shared.ServiceUp,
		Pid:    12345, // Fake PID
		Uptime: 60,    // Fake uptime in seconds
	}

	return manager, mockService
}

// ConfigureNmapManagerForState sets up the mock service in a BenthosManager to facilitate
// a state transition for a specific instance. This should be called before starting
// reconciliation if you want to ensure state transitions happen correctly.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - serviceName: The name of the service to configure
//   - targetState: The desired state to configure the service for
func ConfigureNmapManagerForState(
	mockService *nmap.MockNmapService,
	serviceName string,
	targetState string,
) {
	// Make sure the service exists in the mock
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}
	mockService.ExistingServices[serviceName] = true

	// Make sure service state is initialized
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*nmap.ServiceInfo)
	}
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &nmap.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToNmapState(mockService, serviceName, targetState)
}

// ReconcileOnceNmapManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceNmapManager(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *nmapfsm.NmapManager,
	services serviceregistry.Provider,
) (newTick uint64, err error, reconciled bool) {
	err, rec := manager.Reconcile(ctx, snapshot, services)
	return snapshot.Tick + 1, err, rec
}
