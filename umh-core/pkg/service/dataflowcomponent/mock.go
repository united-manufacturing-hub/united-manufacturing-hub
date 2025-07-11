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
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthosservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockDataFlowComponentService is a mock implementation of the IDataFlowComponentService interface for testing
type MockDataFlowComponentService struct {
	// Mutex to protect concurrent access to shared state
	mu sync.RWMutex

	// Tracks calls to methods
	GenerateBenthosConfigForDataFlowComponentCalled bool
	GetConfigCalled                                 bool
	StatusCalled                                    bool
	AddDataFlowComponentToBenthosManagerCalled      bool
	UpdateDataFlowComponentInBenthosManagerCalled   bool
	RemoveDataFlowComponentFromBenthosManagerCalled bool
	StartDataFlowComponentCalled                    bool
	StopDataFlowComponentCalled                     bool
	ForceRemoveDataFlowComponentCalled              bool
	ServiceExistsCalled                             bool
	ReconcileManagerCalled                          bool

	// Return values for each method
	GenerateBenthosConfigForDataFlowComponentResult benthosserviceconfig.BenthosServiceConfig
	GenerateBenthosConfigForDataFlowComponentError  error
	GetConfigResult                                 dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	GetConfigError                                  error
	StatusResult                                    ServiceInfo
	StatusError                                     error
	AddDataFlowComponentToBenthosManagerError       error
	UpdateDataFlowComponentInBenthosManagerError    error
	RemoveDataFlowComponentFromBenthosManagerError  error
	StartDataFlowComponentError                     error
	StopDataFlowComponentError                      error
	ForceRemoveDataFlowComponentError               error
	ServiceExistsResult                             bool
	ReconcileManagerError                           error
	ReconcileManagerReconciled                      bool

	// For more complex testing scenarios
	ComponentStates    map[string]*ServiceInfo
	ExistingComponents map[string]bool
	BenthosConfigs     []config.BenthosConfig

	// State control for FSM testing
	stateFlags map[string]*ComponentStateFlags

	// Benthos service mock
	BenthosService benthosservice.IBenthosService
}

// Ensure MockDataFlowComponentService implements IDataFlowComponentService
var _ IDataFlowComponentService = (*MockDataFlowComponentService)(nil)

// ComponentStateFlags contains all the state flags needed for FSM testing
type ComponentStateFlags struct {
	IsBenthosRunning                 bool
	BenthosFSMState                  string
	IsBenthosProcessingMetricsActive bool
}

// NewMockDataFlowComponentService creates a new mock DataFlowComponent service
func NewMockDataFlowComponentService() *MockDataFlowComponentService {
	return &MockDataFlowComponentService{
		ComponentStates:    make(map[string]*ServiceInfo),
		ExistingComponents: make(map[string]bool),
		BenthosConfigs:     make([]config.BenthosConfig, 0),
		stateFlags:         make(map[string]*ComponentStateFlags),
		BenthosService:     benthosservice.NewMockBenthosService(),
	}
}

// SetComponentState sets all state flags for a component at once
func (m *MockDataFlowComponentService) SetComponentState(componentName string, flags ComponentStateFlags) {
	m.mu.Lock()
	defer m.mu.Unlock()

	observedState := &benthosfsmmanager.BenthosObservedState{
		ServiceInfo: benthosservice.ServiceInfo{
			BenthosStatus: benthosservice.BenthosStatus{
				BenthosMetrics: benthos_monitor.BenthosMetrics{
					MetricsState: &benthos_monitor.BenthosMetricsState{
						IsActive: flags.IsBenthosProcessingMetricsActive,
					},
					Metrics: benthos_monitor.Metrics{
						Input: benthos_monitor.InputMetrics{
							ConnectionUp:   boolToInt64(flags.IsBenthosProcessingMetricsActive),
							ConnectionLost: 0,
						},
						Output: benthos_monitor.OutputMetrics{
							ConnectionUp: 1,
						},
					},
				},
			},
		},
	}
	// Ensure ServiceInfo exists for this component
	if _, exists := m.ComponentStates[componentName]; !exists {
		m.ComponentStates[componentName] = &ServiceInfo{
			BenthosFSMState:      flags.BenthosFSMState,
			BenthosObservedState: *observedState,
		}
	} else {
		m.ComponentStates[componentName].BenthosObservedState = *observedState
		m.ComponentStates[componentName].BenthosFSMState = flags.BenthosFSMState
	}

	// Store the flags
	m.stateFlags[componentName] = &flags
}

// GetComponentState gets the state flags for a component
func (m *MockDataFlowComponentService) GetComponentState(componentName string) *ComponentStateFlags {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if flags, exists := m.stateFlags[componentName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &ComponentStateFlags{}
	m.stateFlags[componentName] = flags
	return flags
}

// GenerateBenthosConfigForDataFlowComponent mocks generating Benthos config for a DataFlowComponent
func (m *MockDataFlowComponentService) GenerateBenthosConfigForDataFlowComponent(dataflowConfig *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	m.mu.Lock()
	m.GenerateBenthosConfigForDataFlowComponentCalled = true
	result := m.GenerateBenthosConfigForDataFlowComponentResult
	err := m.GenerateBenthosConfigForDataFlowComponentError
	m.mu.Unlock()
	return result, err
}

// GetConfig mocks getting the DataFlowComponent configuration
func (m *MockDataFlowComponentService) GetConfig(ctx context.Context, filesystemService filesystem.Service, componentName string) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	m.mu.Lock()
	m.GetConfigCalled = true
	configError := m.GetConfigError
	configResult := m.GetConfigResult
	m.mu.Unlock()

	// If error is set, return it
	if configError != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, configError
	}

	// If a result is preset, return it
	return configResult, nil
}

// Status mocks getting the status of a DataFlowComponent
func (m *MockDataFlowComponentService) Status(ctx context.Context, filesystemService filesystem.Service, componentName string, tick uint64) (ServiceInfo, error) {
	m.mu.Lock()
	m.StatusCalled = true

	// Check if the component exists in the ExistingComponents map
	if exists, ok := m.ExistingComponents[componentName]; !ok || !exists {
		m.mu.Unlock()
		return ServiceInfo{}, ErrServiceNotExists
	}

	// If we have a state already stored, return it
	if state, exists := m.ComponentStates[componentName]; exists {
		statusError := m.StatusError
		m.mu.Unlock()
		return *state, statusError
	}

	// If no state is stored, return the default mock result
	statusResult := m.StatusResult
	statusError := m.StatusError
	m.mu.Unlock()
	return statusResult, statusError
}

// AddDataFlowComponentToBenthosManager mocks adding a DataFlowComponent to the Benthos manager
func (m *MockDataFlowComponentService) AddDataFlowComponentToBenthosManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.AddDataFlowComponentToBenthosManagerCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	// Check whether the component already exists
	for _, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the component to the list of existing components
	m.ExistingComponents[componentName] = true

	// Create a BenthosConfig for this component
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: benthosfsmmanager.OperationalStateActive,
		},
		BenthosServiceConfig: m.GenerateBenthosConfigForDataFlowComponentResult,
	}

	// Add the BenthosConfig to the list of BenthosConfigs
	m.BenthosConfigs = append(m.BenthosConfigs, benthosConfig)

	return m.AddDataFlowComponentToBenthosManagerError
}

// UpdateDataFlowComponentInBenthosManager mocks updating a DataFlowComponent in the Benthos manager
func (m *MockDataFlowComponentService) UpdateDataFlowComponentInBenthosManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateDataFlowComponentInBenthosManagerCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	// Check if the component exists
	found := false
	index := -1
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	// Update the BenthosConfig
	currentDesiredState := m.BenthosConfigs[index].DesiredFSMState
	m.BenthosConfigs[index] = config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: currentDesiredState,
		},
		BenthosServiceConfig: m.GenerateBenthosConfigForDataFlowComponentResult,
	}

	return m.UpdateDataFlowComponentInBenthosManagerError
}

// RemoveDataFlowComponentFromBenthosManager mocks removing a DataFlowComponent from the Benthos manager
func (m *MockDataFlowComponentService) RemoveDataFlowComponentFromBenthosManager(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RemoveDataFlowComponentFromBenthosManagerCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	found := false

	// Remove the BenthosConfig from the list of BenthosConfigs
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs = append(m.BenthosConfigs[:i], m.BenthosConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	// Remove the component from the list of existing components
	delete(m.ExistingComponents, componentName)
	delete(m.ComponentStates, componentName)

	return m.RemoveDataFlowComponentFromBenthosManagerError
}

// StartDataFlowComponent mocks starting a DataFlowComponent
func (m *MockDataFlowComponentService) StartDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StartDataFlowComponentCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	found := false

	// Set the desired state to active for the given component
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsmmanager.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return m.StartDataFlowComponentError
}

// StopDataFlowComponent mocks stopping a DataFlowComponent
func (m *MockDataFlowComponentService) StopDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StopDataFlowComponentCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	found := false

	// Set the desired state to stopped for the given component
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsmmanager.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return m.StopDataFlowComponentError
}

// ForceRemoveDataFlowComponent mocks force removing a DataFlowComponent
func (m *MockDataFlowComponentService) ForceRemoveDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.mu.Lock()
	m.ForceRemoveDataFlowComponentCalled = true
	err := m.ForceRemoveDataFlowComponentError
	m.mu.Unlock()
	return err
}

// ServiceExists mocks checking if a DataFlowComponent exists
func (m *MockDataFlowComponentService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, componentName string) bool {
	m.mu.Lock()
	m.ServiceExistsCalled = true
	result := m.ServiceExistsResult
	m.mu.Unlock()
	return result
}

// ReconcileManager mocks reconciling the DataFlowComponent manager
func (m *MockDataFlowComponentService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool) {
	m.mu.Lock()
	m.ReconcileManagerCalled = true
	reconcileError := m.ReconcileManagerError
	reconcileReconciled := m.ReconcileManagerReconciled
	m.mu.Unlock()
	return reconcileError, reconcileReconciled
}

// boolToInt64 converts a boolean to int64 (1 for true, 0 for false)
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
