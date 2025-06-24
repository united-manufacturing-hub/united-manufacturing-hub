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

package topicbrowser

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockService is a mock implementation of the ITopicBrowserService interface for testing
type MockService struct {
	// Tracks calls to methods
	GenerateConfigCalled    bool
	StatusCalled            bool
	AddToManagerCalled      bool
	UpdateInManagerCalled   bool
	RemoveFromManagerCalled bool
	StartCalled             bool
	StopCalled              bool
	ForceRemoveCalled       bool
	ServiceExistsCalled     bool
	ReconcileManagerCalled  bool

	// Return values for each method
	GenerateConfigResult       benthosserviceconfig.BenthosServiceConfig
	GenerateConfigError        error
	StatusResult               ServiceInfo
	StatusError                error
	AddToManagerError          error
	UpdateInManagerError       error
	RemoveFromManagerError     error
	StartError                 error
	StopError                  error
	ForceRemoveError           error
	ServiceExistsResult        bool
	ReconcileManagerError      error
	ReconcileManagerReconciled bool

	// For more complex testing scenarios
	States   map[string]*ServiceInfo
	Existing map[string]bool

	// State control for FSM testing
	stateFlags map[string]*StateFlags

	// Benthos service mock
	BenthosService benthossvc.IBenthosService
}

// Ensure MockService implements ITopicBrowserService
var _ ITopicBrowserService = (*MockService)(nil)

// StateFlags contains all the state flags needed for FSM testing
type StateFlags struct {
	BenthosFSMState       string
	RedpandaFSMState      string
	HasProcessingActivity bool
}

// NewMockDataFlowComponentService creates a new mock DataFlowComponent service
func NewMockDataFlowComponentService() *MockService {
	return &MockService{
		States:         make(map[string]*ServiceInfo),
		Existing:       make(map[string]bool),
		stateFlags:     make(map[string]*StateFlags),
		BenthosService: benthossvc.NewMockBenthosService(),
	}
}

// SetComponentState sets all state flags for a component at once
func (m *MockService) SetComponentState(componentName string, flags StateFlags) {
	observedState := &benthosfsm.BenthosObservedState{
		ServiceInfo: benthossvc.ServiceInfo{
			BenthosStatus: benthossvc.BenthosStatus{
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
	if _, exists := m.States[componentName]; !exists {
		m.States[componentName] = &ServiceInfo{
			BenthosFSMState:      flags.BenthosFSMState,
			BenthosObservedState: *observedState,
		}
	} else {
		m.States[componentName].BenthosObservedState = *observedState
		m.States[componentName].BenthosFSMState = flags.BenthosFSMState
	}

	// Store the flags
	m.stateFlags[componentName] = &flags
}

// GetComponentState gets the state flags for a component
func (m *MockService) GetComponentState(componentName string) *StateFlags {
	if flags, exists := m.stateFlags[componentName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &StateFlags{}
	m.stateFlags[componentName] = flags
	return flags
}

// GenerateBenthosConfigForDataFlowComponent mocks generating Benthos config for a DataFlowComponent
func (m *MockService) GenerateConfig(tbName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	m.GenerateConfigCalled = true
	return m.GenerateConfigResult, m.GenerateConfigError
}

// Status mocks getting the status of a DataFlowComponent
func (m *MockService) Status(ctx context.Context, services serviceregistry.Provider, tbName string) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the component exists in the ExistingComponents map
	if exists, ok := m.Existing[tbName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.States[tbName]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddDataFlowComponentToBenthosManager mocks adding a DataFlowComponent to the Benthos manager
func (m *MockService) AddDataFlowComponentToBenthosManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	m.AddToManagerCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	// Check whether the component already exists
	for _, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the component to the list of existing components
	m.Existing[componentName] = true

	// Create a BenthosConfig for this component
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            benthosName,
			DesiredFSMState: benthosfsm.OperationalStateActive,
		},
		BenthosServiceConfig: m.GenerateConfigResult,
	}

	// Add the BenthosConfig to the list of BenthosConfigs
	m.BenthosConfigs = append(m.BenthosConfigs, benthosConfig)

	return m.AddToManagerError
}

// UpdateDataFlowComponentInBenthosManager mocks updating a DataFlowComponent in the Benthos manager
func (m *MockService) UpdateDataFlowComponentInBenthosManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, componentName string) error {
	m.UpdateInManagerCalled = true

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
		BenthosServiceConfig: m.GenerateConfigResult,
	}

	return m.UpdateInManagerError
}

// RemoveDataFlowComponentFromBenthosManager mocks removing a DataFlowComponent from the Benthos manager
func (m *MockService) RemoveDataFlowComponentFromBenthosManager(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.RemoveFromManagerCalled = true

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
	delete(m.Existing, componentName)
	delete(m.States, componentName)

	return m.RemoveFromManagerError
}

// StartDataFlowComponent mocks starting a DataFlowComponent
func (m *MockService) StartDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.StartCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	found := false

	// Set the desired state to active for the given component
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return m.StartError
}

// StopDataFlowComponent mocks stopping a DataFlowComponent
func (m *MockService) StopDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.StopCalled = true

	benthosName := fmt.Sprintf("dataflow-%s", componentName)

	found := false

	// Set the desired state to stopped for the given component
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExists
	}

	return m.StopError
}

// ForceRemoveDataFlowComponent mocks force removing a DataFlowComponent
func (m *MockService) ForceRemoveDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, componentName string) error {
	m.ForceRemoveCalled = true
	return m.ForceRemoveError
}

// ServiceExists mocks checking if a DataFlowComponent exists
func (m *MockService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, componentName string) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the DataFlowComponent manager
func (m *MockService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// boolToInt64 converts a boolean to int64 (1 for true, 0 for false)
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
