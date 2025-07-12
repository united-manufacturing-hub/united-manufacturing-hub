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

package streamprocessor

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpandasvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockService is a mock implementation of the IStreamProcessorService interface for testing
type MockService struct {
	// Tracks calls to methods
	GenerateConfigCalled     bool
	GetConfigCalled          bool
	StatusCalled             bool
	AddToManagerCalled       bool
	UpdateInManagerCalled    bool
	RemoveFromManagerCalled  bool
	StartCalled              bool
	StopCalled               bool
	ForceRemoveCalled        bool
	ServiceExistsCalled      bool
	ReconcileManagerCalled   bool
	BuildRuntimeConfigCalled bool

	// Return values for each method
	GenerateConfigResultDFC    dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	GenerateConfigError        error
	GetConfigResult            streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime
	GetConfigError             error
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
	ConverterStates    map[string]*ServiceInfo
	ExistingComponents map[string]bool
	dfcConfigs         []config.DataFlowComponentConfig

	// State control for FSM testing
	stateFlags map[string]*StateFlags

	DfcService *dataflowcomponent.MockDataFlowComponentService
}

// Ensure MockService implements IStreamProcessorService
var _ IStreamProcessorService = (*MockService)(nil)

// StateFlags contains all the state flags needed for FSM testing
type StateFlags struct {
	IsDFCRunning       bool
	IsConnectionUp     bool
	IsRedpandaRunning  bool
	DfcFSMReadState    string
	DfcFSMWriteState   string
	ConnectionFSMState string
	RedpandaFSMState   string
}

// NewMockService creates a new mock DataFlowComponent service
func NewMockService() *MockService {
	return &MockService{
		ConverterStates:    make(map[string]*ServiceInfo),
		ExistingComponents: make(map[string]bool),
		dfcConfigs:         make([]config.DataFlowComponentConfig, 0),
		stateFlags:         make(map[string]*StateFlags),
		DfcService:         dataflowcomponent.NewMockDataFlowComponentService(),
	}
}

// SetProcessorState updates **both** underlying mocks so their FSMs
func (m *MockService) SetProcessorState(
	spName string,
	flags StateFlags,
) {
	// Ensure service exists in mock
	m.ExistingComponents[spName] = true

	// 1. Forward to DFC mock
	dfcFlags := ConverterToDFCFlags(flags)
	m.DfcService.SetComponentState(spName, dfcFlags)

	// 3. Build a *single* aggregate ServiceInfo for Status()
	m.ConverterStates[spName] = BuildServiceInfo(
		spName, flags, m.DfcService,
	)

	// Store the flags for backward compatibility
	m.stateFlags[spName] = &flags
}

// SetComponentState is kept for backward compatibility but now delegates to SetConverterState
func (m *MockService) SetComponentState(spName string, flags StateFlags) {
	m.SetProcessorState(spName, flags)
}

// GetProcessorState gets the state flags for a stream processor
func (m *MockService) GetProcessorState(spName string) *StateFlags {
	if flags, exists := m.stateFlags[spName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &StateFlags{}
	m.stateFlags[spName] = flags
	return flags
}

// ConverterToDFCFlags converts high-level ConverterStateFlags into the
// exact flag struct used by the DFC mock.
func ConverterToDFCFlags(src StateFlags) dataflowcomponent.ComponentStateFlags {
	return dataflowcomponent.ComponentStateFlags{
		IsBenthosRunning:                 src.IsDFCRunning,
		BenthosFSMState:                  src.DfcFSMReadState,
		IsBenthosProcessingMetricsActive: src.IsDFCRunning && src.DfcFSMReadState == dfcfsm.OperationalStateActive,
	}
}

// BuildServiceInfo builds an aggregated ServiceInfo from the sub-mocks
func BuildServiceInfo(
	name string,
	flags StateFlags,
	dfcMock *dataflowcomponent.MockDataFlowComponentService,
) *ServiceInfo {
	// Get observed states from the sub-mocks
	var dfcInfo dataflowcomponent.ServiceInfo

	if dfcMock.ComponentStates[name] != nil {
		dfcInfo = *dfcMock.ComponentStates[name]
	}

	// Build the aggregate ServiceInfo
	return &ServiceInfo{
		DFCFSMState: flags.DfcFSMReadState,
		DFCObservedState: dfcfsm.DataflowComponentObservedState{
			ServiceInfo: dfcInfo,
		},
		RedpandaFSMState: flags.RedpandaFSMState,
		RedpandaObservedState: redpandafsm.RedpandaObservedState{
			ServiceInfo: redpandasvc.ServiceInfo{
				RedpandaStatus: redpandasvc.RedpandaStatus{
					HealthCheck: redpandasvc.HealthCheck{
						IsReady: flags.IsRedpandaRunning,
						IsLive:  flags.IsRedpandaRunning,
					},
				},
			},
		},
	}
}

// GetConfig mocks getting the actual deployed StreamProcessor config
func (m *MockService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) (streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime, error) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}, m.GetConfigError
	}

	// Return the StreamProcessor config
	return m.GetConfigResult, nil
}

// Status mocks getting the status of a StreamProcessor
func (m *MockService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
	spName string,
) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the component exists in the ExistingComponents map
	if exists, ok := m.ExistingComponents[spName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.ConverterStates[spName]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddToManager mocks adding a StreamProcessor to the  DFC manager
func (m *MockService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	spName string,
) error {
	m.AddToManagerCalled = true

	underlyingName := fmt.Sprintf("streamprocessor-%s", spName)

	// Check whether the component already exists
	for _, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the component to the list of existing components
	m.ExistingComponents[spName] = true

	// Create a dfcConfig for this component
	dfcConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: dfcfsm.OperationalStateActive,
		},
		DataFlowComponentServiceConfig: m.GenerateConfigResultDFC,
	}

	// Add the dfcConfig to the list of dfcConfigs
	m.dfcConfigs = append(m.dfcConfigs, dfcConfig)

	return m.AddToManagerError
}

// UpdateInManager mocks updating a Stream Processor  DFC manager
func (m *MockService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	spName string,
) error {
	m.UpdateInManagerCalled = true

	underlyingName := fmt.Sprintf("streamprocessor-%s", spName)

	// Check if the component exists
	dfcFound := false
	dfcIndex := -1
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			dfcFound = true
			dfcIndex = i
			break
		}
	}

	if !dfcFound {
		return ErrServiceNotExist
	}

	// Update the DFCConfig
	currentDesiredStateDFC := m.dfcConfigs[dfcIndex].DesiredFSMState
	m.dfcConfigs[dfcIndex] = config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: currentDesiredStateDFC,
		},
		DataFlowComponentServiceConfig: m.GenerateConfigResultDFC,
	}

	return m.UpdateInManagerError
}

// RemoveFromManager mocks removing a Stream Processor from the DFC manager
func (m *MockService) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	m.RemoveFromManagerCalled = true

	underlyingName := fmt.Sprintf("streamprocessor-%s", spName)

	dfcFound := false

	// Remove the DfcConfig from the list of DfcConfigs
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs = append(m.dfcConfigs[:i], m.dfcConfigs[i+1:]...)
			dfcFound = true
			break
		}
	}

	if !dfcFound {
		return ErrServiceNotExist
	}

	// Remove the component from the list of existing components
	delete(m.ExistingComponents, spName)
	delete(m.ConverterStates, spName)

	return m.RemoveFromManagerError
}

// Start mocks starting a Steram Processor
func (m *MockService) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	m.StartCalled = true

	underlyingName := fmt.Sprintf("streamprocessor-%s", spName)

	dfcFound := false

	// Set the desired state to active for the given processor
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateActive
			dfcFound = true
			break
		}
	}

	if !dfcFound {
		return ErrServiceNotExist
	}

	return m.StartError
}

// Stop mocks stopping a StreamProcessor
func (m *MockService) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	m.StopCalled = true

	underlyingName := fmt.Sprintf("streamprocessor-%s", spName)

	dfcFound := false

	// Set the desired state to stopped for the given component
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			dfcFound = true
			break
		}
	}

	if !dfcFound {
		return ErrServiceNotExist
	}

	return m.StopError
}

// ForceRemove mocks force removing a StreamProcessor
func (m *MockService) ForceRemove(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	m.ForceRemoveCalled = true
	return m.ForceRemoveError
}

// ServiceExists mocks checking if a StreamProcessor exists
func (m *MockService) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	spname string,
) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the StreamProcessor manager
func (m *MockService) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// EvaluateDFCDesiredStates mocks the DFC state evaluation logic.
// This method exists because stream processors must re-evaluate DFC states
// when configs change during reconciliation (unlike other FSMs that set states once).
func (m *MockService) EvaluateDFCDesiredStates(
	spName string,
	spDesiredState string,
) error {
	// Mock implementation - just update the configs like the real implementation would
	underlyingReadName := fmt.Sprintf("streamprocessor-%s", spName)

	// Find and update read DFC config
	for i, config := range m.dfcConfigs {
		if config.Name == underlyingReadName {
			if spDesiredState == "stopped" {
				m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			} else {
				// Only start the DFC, if it has been configured
				if len(m.dfcConfigs[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
					m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateActive
				} else {
					m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateStopped
				}
			}
			break
		}
	}

	return nil
}
