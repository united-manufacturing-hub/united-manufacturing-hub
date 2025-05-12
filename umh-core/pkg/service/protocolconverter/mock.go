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

package protocolconverter

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	connfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	benthosservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockProtocolConverterService is a mock implementation of the IProtocolConverterService interface for testing
type MockProtocolConverterService struct {
	// Tracks calls to methods
	GenerateConfigCalled    bool
	GetConfigCalled         bool
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
	GenerateConfigResultDFC        dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	GenerateConfigResultConnection connectionserviceconfig.ConnectionServiceConfig
	GenerateConfigError            error
	GetConfigResult                protocolconverterserviceconfig.ProtocolConverterServiceConfig
	GetConfigError                 error
	StatusResult                   ServiceInfo
	StatusError                    error
	AddToManagerError              error
	UpdateInManagerError           error
	RemoveFromManagerError         error
	StartError                     error
	StopError                      error
	ForceRemoveError               error
	ServiceExistsResult            bool
	ReconcileManagerError          error
	ReconcileManagerReconciled     bool

	// For more complex testing scenarios
	ConverterStates    map[string]*ServiceInfo
	ExistingComponents map[string]bool
	dfcConfigs         []config.DataFlowComponentConfig
	connConfigs        []config.ConnectionConfig

	// State control for FSM testing
	stateFlags map[string]*ConverterStateFlags

	// service mocks
	DfcService  dataflowcomponent.IDataFlowComponentService
	ConnService connection.IConnectionService
}

// Ensure MockProtocolConverterService implements IProtocolConverterService
var _ IProtocolConverterService = (*MockProtocolConverterService)(nil)

// ConverterStateFlags contains all the state flags needed for FSM testing
type ConverterStateFlags struct {
	IsDFCRunning       bool
	IsConnectionUp     bool
	dfcFSMState        string
	connectionFSMState string
	PortState          nmapfsm.PortState
}

// NewMockProtocolConverterService creates a new mock DataFlowComponent service
func NewMockProtocolConverterService() *MockProtocolConverterService {
	return &MockProtocolConverterService{
		ConverterStates:    make(map[string]*ServiceInfo),
		ExistingComponents: make(map[string]bool),
		dfcConfigs:         make([]config.DataFlowComponentConfig, 0),
		connConfigs:        make([]config.ConnectionConfig, 0),
		stateFlags:         make(map[string]*ConverterStateFlags),
		DfcService:         dataflowcomponent.NewMockDataFlowComponentService(),
	}
}

// SetComponentState sets all state flags for a protocolConverter at once
func (m *MockProtocolConverterService) SetComponentState(protConvName string, flags ConverterStateFlags) {
	dfcObservedState := &dfcfsm.DataflowComponentObservedState{
		ServiceInfo: dataflowcomponent.ServiceInfo{
			BenthosObservedState: benthosfsmmanager.BenthosObservedState{
				ServiceInfo: benthosservice.ServiceInfo{
					BenthosStatus: benthosservice.BenthosStatus{
						BenthosMetrics: benthos_monitor.BenthosMetrics{
							MetricsState: &benthos_monitor.BenthosMetricsState{
								IsActive: flags.IsDFCRunning,
							},
						},
					},
				},
			},
		},
	}

	connObservedState := &connfsm.ConnectionObservedState{
		ServiceInfo: connection.ServiceInfo{
			NmapObservedState: nmapfsm.NmapObservedState{
				ServiceInfo: nmap.ServiceInfo{
					NmapStatus: nmap.NmapServiceInfo{
						IsRunning: flags.IsConnectionUp,
						LastScan: &nmap.NmapScanResult{
							PortResult: nmap.PortResult{
								State: string(flags.PortState),
							},
						},
					},
				},
			},
		},
	}
	// Ensure ServiceInfo exists for this component
	if _, exists := m.ConverterStates[protConvName]; !exists {
		m.ConverterStates[protConvName] = &ServiceInfo{
			DataflowComponentFSMState:      flags.dfcFSMState,
			DataflowComponentObservedState: *dfcObservedState,
			ConnectionFSMState:             flags.connectionFSMState,
			ConnectionObservedState:        *connObservedState,
		}
	} else {
		m.ConverterStates[protConvName].DataflowComponentObservedState = *dfcObservedState
		m.ConverterStates[protConvName].DataflowComponentFSMState = flags.dfcFSMState
		m.ConverterStates[protConvName].ConnectionObservedState = *connObservedState
		m.ConverterStates[protConvName].ConnectionFSMState = flags.connectionFSMState
	}

	// Store the flags
	m.stateFlags[protConvName] = &flags
}

// GetConverterState gets the state flags for a protocol converter
func (m *MockProtocolConverterService) GetConverterState(protConvName string) *ConverterStateFlags {
	if flags, exists := m.stateFlags[protConvName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &ConverterStateFlags{}
	m.stateFlags[protConvName] = flags
	return flags
}

// GenerateConfig mocks generating connection & dfc config for a ProtocolConverter
func (m *MockProtocolConverterService) GenerateConfig(
	protConvConfig *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	protConvName string,
) (
	connectionserviceconfig.ConnectionServiceConfig,
	dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	error,
) {
	m.GenerateConfigCalled = true
	return m.GenerateConfigResultConnection, m.GenerateConfigResultDFC, m.GenerateConfigError
}

// GetConfig mocks getting the ProtocolConverter configuration
func (m *MockProtocolConverterService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) (
	protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	error,
) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfig{}, m.GetConfigError
	}

	// If a result is preset, return it
	return m.GetConfigResult, nil
}

// Status mocks getting the status of a ProtocolConverter
func (m *MockProtocolConverterService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	protConvName string,
	tick uint64,
) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the component exists in the ExistingComponents map
	if exists, ok := m.ExistingComponents[protConvName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.ConverterStates[protConvName]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddToManager mocks adding a ProtocolConverter to the Connection & DFC manager
func (m *MockProtocolConverterService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	protConvName string,
) error {
	m.AddToManagerCalled = true

	underlyingName := fmt.Sprintf("protocolconverter-%s", protConvName)

	// Check whether the component already exists
	for _, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			return ErrServiceAlreadyExists
		}
	}

	// Check whether the component already exists
	for _, connConfig := range m.dfcConfigs {
		if connConfig.Name == underlyingName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the component to the list of existing components
	m.ExistingComponents[protConvName] = true

	// Create a dfcConfig for this component
	dfcConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: dfcfsm.OperationalStateActive,
		},
		DataFlowComponentServiceConfig: m.GenerateConfigResultDFC,
	}

	// Create a dfcConfig for this component
	connConfig := config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: dfcfsm.OperationalStateActive,
		},
		ConnectionServiceConfig: m.GenerateConfigResultConnection,
	}

	// Add the dfcConfig to the list of dfcConfigs
	m.dfcConfigs = append(m.dfcConfigs, dfcConfig)
	m.connConfigs = append(m.connConfigs, connConfig)

	return m.AddToManagerError
}

// UpdateInManager mocks updating a ProtocolConverter in Connection & DFC manager
func (m *MockProtocolConverterService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	protConvName string,
) error {
	m.UpdateInManagerCalled = true

	underlyingName := fmt.Sprintf("protocolconverter-%s", protConvName)

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

	// Check if the connection exists
	connFound := false
	connIndex := -1
	for i, connConfig := range m.connConfigs {
		if connConfig.Name == underlyingName {
			connFound = true
			connIndex = i
			break
		}
	}

	if !dfcFound || !connFound {
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

	// Update the DFCConfig
	currentDesiredStateConn := m.connConfigs[connIndex].DesiredFSMState
	m.connConfigs[connIndex] = config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: currentDesiredStateConn,
		},
		ConnectionServiceConfig: m.GenerateConfigResultConnection,
	}

	return m.UpdateInManagerError
}

// RemoveFromManager mocks removing a DataFlowComponent from the Benthos manager
func (m *MockProtocolConverterService) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) error {
	m.RemoveFromManagerCalled = true

	underlyingName := fmt.Sprintf("protocolconverter-%s", protConvName)

	dfcFound := false

	// Remove the DfcConfig from the list of DfcConfigs
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs = append(m.dfcConfigs[:i], m.dfcConfigs[i+1:]...)
			dfcFound = true
			break
		}
	}

	connFound := false

	for i, connConfig := range m.connConfigs {
		if connConfig.Name == underlyingName {
			m.connConfigs = append(m.connConfigs[:i], m.connConfigs[i+1:]...)
			connFound = true
			break
		}
	}

	if !dfcFound || !connFound {
		return ErrServiceNotExist
	}

	// Remove the component from the list of existing components
	delete(m.ExistingComponents, protConvName)
	delete(m.ConverterStates, protConvName)

	return m.RemoveFromManagerError
}

// Start mocks starting a ProtocolConverter
func (m *MockProtocolConverterService) Start(ctx context.Context, filesystemService filesystem.Service, protConvName string) error {
	m.StartCalled = true

	underlyingName := fmt.Sprintf("protocolconverter-%s", protConvName)

	dfcFound := false

	// Set the desired state to active for the given component
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateActive
			dfcFound = true
			break
		}
	}

	connFound := false

	// Set the desired state to active for the given component
	for i, connConfig := range m.connConfigs {
		if connConfig.Name == underlyingName {
			m.connConfigs[i].DesiredFSMState = connfsm.OperationalStateUp
			connFound = true
			break
		}
	}

	if !dfcFound || !connFound {
		return ErrServiceNotExist
	}

	return m.StartError
}

// Stop mocks stopping a ProtocolConverter
func (m *MockProtocolConverterService) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) error {
	m.StopCalled = true

	underlyingName := fmt.Sprintf("protocolconverter-%s", protConvName)

	dfcFound := false

	// Set the desired state to stopped for the given component
	for i, dfcConfig := range m.dfcConfigs {
		if dfcConfig.Name == underlyingName {
			m.dfcConfigs[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			dfcFound = true
			break
		}
	}

	connFound := false

	// Set the desired state to stopped for the given component
	for i, connConfig := range m.connConfigs {
		if connConfig.Name == underlyingName {
			m.connConfigs[i].DesiredFSMState = connfsm.OperationalStateUp
			connFound = true
			break
		}
	}

	if !dfcFound || !connFound {
		return ErrServiceNotExist
	}

	return m.StopError
}

// ForceRemove mocks force removing a ProtocolConverter
func (m *MockProtocolConverterService) ForceRemove(ctx context.Context, filesystemService filesystem.Service, protConvName string) error {
	m.ForceRemoveCalled = true
	return m.ForceRemoveError
}

// ServiceExists mocks checking if a ProtocolConverter exists
func (m *MockProtocolConverterService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, protConvName string) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the ProtocolConverter manager
func (m *MockProtocolConverterService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}
