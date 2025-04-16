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

	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// WaitForBenthosManagerStable is a simple helper that calls manager.Reconcile once
// and checks if there was an error. It's used for scenarios like an empty config
// where we just want to do one pass. Then we might call it multiple times if needed.
func WaitForBenthosManagerStable(
	ctx context.Context,
	currentSnapshot snapshot.SystemSnapshot,
	manager *benthosfsm.BenthosManager,

	filesystemService filesystem.Service,
) (uint64, error) {
	err, _ := manager.Reconcile(ctx, currentSnapshot, filesystemService)
	if err != nil {
		return currentSnapshot.Tick, err
	}
	// We might do more checks if needed, e.g. manager has zero instances?
	return currentSnapshot.Tick + 1, nil
}

// WaitForBenthosManagerInstanceState repeatedly calls manager.Reconcile until
// we see the specified 'desiredState' or we hit maxAttempts.
func WaitForBenthosManagerInstanceState(
	ctx context.Context,
	currentSnapshot snapshot.SystemSnapshot,
	manager *benthosfsm.BenthosManager,

	filesystemService filesystem.Service,
	instanceName string,
	desiredState string,
	maxAttempts int,
) (uint64, error) {
	tick := currentSnapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, currentSnapshot, filesystemService)
		if err != nil {
			return tick, err
		}
		tick++

		inst, found := manager.GetInstance(instanceName)
		if found && inst.GetCurrentFSMState() == desiredState {
			return tick, nil
		}
	}
	return tick, fmt.Errorf("instance %s did not reach state %s after %d attempts",
		instanceName, desiredState, maxAttempts)
}

// WaitForBenthosManagerInstanceRemoval waits until the given serviceName is removed
// from the manager's instance map.
func WaitForBenthosManagerInstanceRemoval(
	ctx context.Context,
	currentSnapshot snapshot.SystemSnapshot,
	manager *benthosfsm.BenthosManager,

	filesystemService filesystem.Service,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := currentSnapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, currentSnapshot, filesystemService)
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

// WaitForBenthosManagerMultiState can check multiple instances at once:
// e.g. map[serviceName]desiredState = ...
func WaitForBenthosManagerMultiState(
	ctx context.Context,
	currentSnapshot snapshot.SystemSnapshot,
	manager *benthosfsm.BenthosManager,

	filesystemService filesystem.Service,
	desiredMap map[string]string, // e.g. { "svc1": "idle", "svc2": "active" }
	maxAttempts int,
) (uint64, error) {
	tick := currentSnapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, currentSnapshot, filesystemService)
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

// SetupServiceInManager adds a service to the manager and configures it with the mock service.
// It creates an instance automatically and adds it to the manager.
// Note: Since this cannot directly set the service field in the instance after creation,
// the instance in the manager will be using the default service, not the mock service.
// Tests should rely on the manager.Reconcile method instead, which uses the manager's own service.
func SetupServiceInManager(
	manager *benthosfsm.BenthosManager,
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	desiredState string,
) {
	// Create a properly configured instance
	instance := benthosfsm.NewBenthosInstance(CreateBenthosTestConfig(serviceName, desiredState))

	// Add it to the manager
	manager.BaseFSMManager.AddInstanceForTest(serviceName, instance)

	// Make sure the service exists in the mock service
	mockService.ExistingServices[serviceName] = true

	// Ensure we have a service info initialized
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}
}

// CreateMockBenthosManager creates a Benthos manager with a mock service for testing
func CreateMockBenthosManager(name string) (*benthosfsm.BenthosManager, *benthossvc.MockBenthosService) {
	// Use the specialized constructor that sets up fully mocked services
	manager, mockService := benthosfsm.NewBenthosManagerWithMockedServices(name)

	// Set up the mock service to prevent real filesystem operations
	s6MockService := mockService.S6Service.(*s6svc.MockService)

	// Configure default responses to prevent real filesystem operations
	s6MockService.CreateError = nil
	s6MockService.RemoveError = nil
	s6MockService.StartError = nil
	s6MockService.StopError = nil
	s6MockService.ForceRemoveError = nil

	// Set up the mock to say services exist after creation
	s6MockService.ServiceExistsResult = true

	// Configure default successful statuses
	s6MockService.StatusResult = s6svc.ServiceInfo{
		Status: s6svc.ServiceUp,
		Pid:    12345, // Fake PID
		Uptime: 60,    // Fake uptime in seconds
	}

	return manager, mockService
}

// ConfigureBenthosManagerForState sets up the mock service in a BenthosManager to facilitate
// a state transition for a specific instance. This should be called before starting
// reconciliation if you want to ensure state transitions happen correctly.
//
// Parameters:
//   - mockService: The mock service from the manager
//   - serviceName: The name of the service to configure
//   - targetState: The desired state to configure the service for
func ConfigureBenthosManagerForState(
	mockService *benthossvc.MockBenthosService,
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
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	}
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToBenthosState(mockService, serviceName, targetState)
}

// ReconcileOnceBenthosManager calls manager.Reconcile(...) exactly once,
// increments 'tick' by 1, and returns the new tick plus any error & the
// manager's 'reconciled' bool.
func ReconcileOnceBenthosManager(
	ctx context.Context,
	currentSnapshot snapshot.SystemSnapshot,
	manager *benthosfsm.BenthosManager,

	filesystemService filesystem.Service,
) (newTick uint64, err error, reconciled bool) {
	err, rec := manager.Reconcile(ctx, currentSnapshot, filesystemService)
	return currentSnapshot.Tick + 1, err, rec
}
