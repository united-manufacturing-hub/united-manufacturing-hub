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

package dataflowcomponent

import (
	"context"
	"fmt"

	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// MockBenthosManagerAdapter adapts the mocked BenthosManager and MockBenthosService
// to implement the BenthosConfigManager interface for testing
type MockBenthosManagerAdapter struct {
	// These are kept for reference but we'll primarily use the component maps below
	BenthosManager *benthosfsm.BenthosManager
	MockService    *benthossvc.MockBenthosService

	// Component tracking
	components     map[string]DataFlowComponentConfig
	observedStates map[string]*benthosfsm.BenthosObservedState

	// Error simulation
	shouldFailGet    bool
	shouldFailAdd    bool
	shouldFailUpdate bool
	shouldFailRemove bool
	shouldFailExists bool

	// Permanent error simulation
	PermanentError error
}

// NewMockBenthosManagerAdapter creates a new adapter using the provided BenthosManager and MockBenthosService
func NewMockBenthosManagerAdapter(manager *benthosfsm.BenthosManager, mockService *benthossvc.MockBenthosService) *MockBenthosManagerAdapter {
	return &MockBenthosManagerAdapter{
		BenthosManager: manager,
		MockService:    mockService,
		components:     make(map[string]DataFlowComponentConfig),
		observedStates: make(map[string]*benthosfsm.BenthosObservedState),
	}
}

// AddComponentToBenthosConfig adds a component to the benthos config
func (m *MockBenthosManagerAdapter) AddComponentToBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	if m.shouldFailAdd {
		return fmt.Errorf("simulated error adding component")
	}

	// Store the component in our local map
	m.components[component.Name] = component

	// Set up the mock service to know about this component
	m.MockService.ExistingServices[component.Name] = true

	// Initialize an observed state for this component if it doesn't exist
	if _, exists := m.observedStates[component.Name]; !exists {
		m.observedStates[component.Name] = &benthosfsm.BenthosObservedState{
			ServiceInfo: benthossvc.ServiceInfo{
				S6FSMState: benthosfsm.OperationalStateStopped,
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  true,
						IsReady: true,
					},
				},
			},
			ObservedBenthosServiceConfig: component.ServiceConfig,
		}
	}

	return nil
}

// RemoveComponentFromBenthosConfig removes a component from the benthos config
func (m *MockBenthosManagerAdapter) RemoveComponentFromBenthosConfig(ctx context.Context, componentName string) error {
	if m.shouldFailRemove {
		return fmt.Errorf("simulated error removing component")
	}

	// Remove from our component map
	delete(m.components, componentName)

	// Update mock service to remove this component
	m.MockService.ExistingServices[componentName] = false

	return nil
}

// UpdateComponentInBenthosConfig updates a component in the benthos config
func (m *MockBenthosManagerAdapter) UpdateComponentInBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	if m.shouldFailUpdate {
		return fmt.Errorf("simulated error updating component")
	}

	// Update our component map
	m.components[component.Name] = component

	// Update the observed state config to match the new component config
	if state, exists := m.observedStates[component.Name]; exists {
		state.ObservedBenthosServiceConfig = component.ServiceConfig
	} else {
		// Initialize if it doesn't exist
		m.observedStates[component.Name] = &benthosfsm.BenthosObservedState{
			ServiceInfo: benthossvc.ServiceInfo{
				S6FSMState: benthosfsm.OperationalStateStopped,
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  true,
						IsReady: true,
					},
				},
			},
			ObservedBenthosServiceConfig: component.ServiceConfig,
		}
	}

	return nil
}

// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
func (m *MockBenthosManagerAdapter) ComponentExistsInBenthosConfig(ctx context.Context, componentName string) (bool, error) {
	if m.shouldFailExists {
		return false, fmt.Errorf("simulated error checking existence")
	}

	// Check our component map
	_, exists := m.components[componentName]
	return exists, nil
}

// GetComponentBenthosObservedState retrieves the observed state of a component from the mocked BenthosManager
func (m *MockBenthosManagerAdapter) GetComponentBenthosObservedState(ctx context.Context, componentName string) (*benthosfsm.BenthosObservedState, error) {
	if m.PermanentError != nil {
		return nil, m.PermanentError
	}

	if m.shouldFailGet {
		return nil, fmt.Errorf("simulated error getting observed state")
	}

	// Check if the component exists in our map
	if _, exists := m.components[componentName]; !exists {
		return nil, fmt.Errorf("component %s does not exist", componentName)
	}

	// Return the stored observed state for this component, or create a new one
	state, exists := m.observedStates[componentName]
	if !exists {
		// This should rarely happen since we initialize it in Add/Update, but just in case
		state = &benthosfsm.BenthosObservedState{
			ServiceInfo: benthossvc.ServiceInfo{
				S6FSMState: benthosfsm.OperationalStateStopped,
				BenthosStatus: benthossvc.BenthosStatus{
					HealthCheck: benthossvc.HealthCheck{
						IsLive:  true,
						IsReady: true,
					},
				},
			},
			ObservedBenthosServiceConfig: m.components[componentName].ServiceConfig,
		}
		m.observedStates[componentName] = state
	}

	return state, nil
}

// SetComponentState allows setting a specific state for a component in tests
func (m *MockBenthosManagerAdapter) SetComponentState(componentName, state string, isHealthy bool) {
	if _, exists := m.observedStates[componentName]; !exists {
		// Create a default observed state if it doesn't exist
		m.observedStates[componentName] = &benthosfsm.BenthosObservedState{
			ServiceInfo: benthossvc.ServiceInfo{},
		}
	}

	// Update the service state in the observed state
	m.observedStates[componentName].ServiceInfo.S6FSMState = state

	// Update health status in BenthosStatus field
	m.observedStates[componentName].ServiceInfo.BenthosStatus.HealthCheck.IsLive = isHealthy
	m.observedStates[componentName].ServiceInfo.BenthosStatus.HealthCheck.IsReady = isHealthy
}

// ConfigureFailure allows configuring failures for different operations
func (m *MockBenthosManagerAdapter) ConfigureFailure(operation string, shouldFail bool) {
	switch operation {
	case "add":
		m.shouldFailAdd = shouldFail
	case "remove":
		m.shouldFailRemove = shouldFail
	case "update":
		m.shouldFailUpdate = shouldFail
	case "exists":
		m.shouldFailExists = shouldFail
	case "getState":
		m.shouldFailGet = shouldFail
	}
}
