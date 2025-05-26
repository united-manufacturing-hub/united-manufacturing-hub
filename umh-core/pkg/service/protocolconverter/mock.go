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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpandasvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockProtocolConverterService is a mock implementation of the IProtocolConverterService interface for testing
//
// Why delegation?
//
// The Protocol-Converter FSM is *by definition* the union of the
// Connection FSM and *two* DFC FSMs (read + write).  Re-encoding all
// low-level fields (e.g. PortState, BenthosMetrics.IsActive) in a new
// "converter flag bag" risks drifting out-of-sync with the canonical
// mocks that the Connection / DFC teams already maintain.
//
// Instead we embed their mocks and translate the high-level intent
// (Idle, Active, Down, …) into the exact flag structs those mocks
// expect.  One source of truth, less duplication, and all helper
// fns (`SetupConnectionServiceState`, `TransitionToDataflowComponentState`,
// …) remain reusable.
type MockProtocolConverterService struct {
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
	GenerateConfigResultDFC        dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	GenerateConfigResultConnection connectionserviceconfig.ConnectionServiceConfig
	GenerateConfigError            error
	GetConfigResult                protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime
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

	/*
	   Each protocol-converter *instance* is backed by
	     • one Connection  (talks to PLC / OPC-UA, etc.)
	     • one READ DFC    (Benthos pipeline → Kafka, …)

	   Instead of re-implementing state flags here, we embed the
	   **already battle-tested** mocks used by those FSMs.  That gives
	   us full fidelity (e.g. Nmap scan results, Benthos metrics-active
	   flag) and keeps all FSM helpers DRY.
	*/
	DfcService  *dataflowcomponent.MockDataFlowComponentService
	ConnService *connection.MockConnectionService
}

// Ensure MockProtocolConverterService implements IProtocolConverterService
var _ IProtocolConverterService = (*MockProtocolConverterService)(nil)

// ConverterStateFlags contains all the state flags needed for FSM testing
type ConverterStateFlags struct {
	IsDFCRunning       bool
	IsConnectionUp     bool
	IsRedpandaRunning  bool
	DfcFSMReadState    string
	DfcFSMWriteState   string
	ConnectionFSMState string
	RedpandaFSMState   string
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
		ConnService:        connection.NewMockConnectionService(),
	}
}

// SetConverterState updates **both** underlying mocks so their FSMs
// look exactly like a real system in the requested Protocol-Converter
// high-level state.  It then synthesises a single ServiceInfo snapshot
// (`ConverterStates`) so the ProtocolConverterService.Status method can
// still return aggregated data without peeking into the sub-services.
func (m *MockProtocolConverterService) SetConverterState(
	protConvName string,
	flags ConverterStateFlags,
) {
	// Ensure service exists in mock
	m.ExistingComponents[protConvName] = true

	// 1. Forward to DFC mock
	dfcFlags := ConverterToDFCFlags(flags)
	m.DfcService.SetComponentState(protConvName, dfcFlags)

	// 2. Forward to Connection mock
	connFlags := ConverterToConnFlags(flags)
	m.ConnService.SetConnectionState(protConvName, connFlags)

	// 3. Build a *single* aggregate ServiceInfo for Status()
	m.ConverterStates[protConvName] = BuildProtocolConverterServiceInfo(
		protConvName, flags, m.DfcService, m.ConnService,
	)

	// Store the flags for backward compatibility
	m.stateFlags[protConvName] = &flags
}

// SetComponentState is kept for backward compatibility but now delegates to SetConverterState
func (m *MockProtocolConverterService) SetComponentState(protConvName string, flags ConverterStateFlags) {
	m.SetConverterState(protConvName, flags)
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

// ConverterToDFCFlags converts high-level ConverterStateFlags into the
// exact flag struct used by the DFC mock.
func ConverterToDFCFlags(src ConverterStateFlags) dataflowcomponent.ComponentStateFlags {
	return dataflowcomponent.ComponentStateFlags{
		IsBenthosRunning:                 src.IsDFCRunning,
		BenthosFSMState:                  src.DfcFSMReadState,
		IsBenthosProcessingMetricsActive: src.IsDFCRunning && src.DfcFSMReadState == dfcfsm.OperationalStateActive,
	}
}

// ConverterToConnFlags converts high-level ConverterStateFlags into the
// flag struct the Connection mock expects.
func ConverterToConnFlags(src ConverterStateFlags) connection.ConnectionStateFlags {
	return connection.ConnectionStateFlags{
		IsNmapRunning: src.IsConnectionUp,
		NmapFSMState:  src.ConnectionFSMState,
		IsFlaky:       false, // Default to not flaky
	}
}

// BuildProtocolConverterServiceInfo builds an aggregated ServiceInfo from the sub-mocks
func BuildProtocolConverterServiceInfo(
	name string,
	flags ConverterStateFlags,
	dfcMock *dataflowcomponent.MockDataFlowComponentService,
	connMock *connection.MockConnectionService,
) *ServiceInfo {
	// Get observed states from the sub-mocks
	var dfcInfo dataflowcomponent.ServiceInfo
	var connInfo connection.ServiceInfo

	if dfcMock.ComponentStates[name] != nil {
		dfcInfo = *dfcMock.ComponentStates[name]
	}

	if connMock.ConnectionStates[name] != nil {
		connInfo = *connMock.ConnectionStates[name]
	}

	// Build the aggregate ServiceInfo
	return &ServiceInfo{
		DataflowComponentReadFSMState: flags.DfcFSMReadState,
		DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
			ServiceInfo: dfcInfo,
		},
		DataflowComponentWriteFSMState: flags.DfcFSMWriteState,
		DataflowComponentWriteObservedState: dfcfsm.DataflowComponentObservedState{
			ServiceInfo: dataflowcomponent.ServiceInfo{}, // TODO: Add write DFC support
		},
		ConnectionFSMState: flags.ConnectionFSMState,
		ConnectionObservedState: connfsm.ConnectionObservedState{
			ServiceInfo: connInfo,
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

// GetConfig mocks getting the ProtocolConverter configuration
func (m *MockProtocolConverterService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) (
	protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
	error,
) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, m.GetConfigError
	}

	// If a result is preset, return it
	return m.GetConfigResult, nil
}

// Status mocks getting the status of a ProtocolConverter
func (m *MockProtocolConverterService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
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
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
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
	for _, connConfig := range m.connConfigs {
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
			DesiredFSMState: connfsm.OperationalStateUp,
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
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
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

// StartProtocolConverter mocks starting a ProtocolConverter
func (m *MockProtocolConverterService) StartProtocolConverter(ctx context.Context, filesystemService filesystem.Service, protConvName string) error {
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

// StopProtocolConverter mocks stopping a ProtocolConverter
func (m *MockProtocolConverterService) StopProtocolConverter(
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
			m.connConfigs[i].DesiredFSMState = connfsm.OperationalStateStopped
			connFound = true
			break
		}
	}

	if !dfcFound || !connFound {
		return ErrServiceNotExist
	}

	return m.StopError
}

// ForceRemoveProtocolConverter mocks force removing a ProtocolConverter
func (m *MockProtocolConverterService) ForceRemoveProtocolConverter(ctx context.Context, filesystemService filesystem.Service, protConvName string) error {
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
