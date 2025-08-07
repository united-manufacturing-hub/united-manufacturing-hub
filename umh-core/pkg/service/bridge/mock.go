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

package bridge

import (
	"context"
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
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

// MockService is a mock implementation of the IService interface for testing
//
// Why delegation?
//
// The Bridge FSM is by definition the union of the
// Connection FSM and two DFC FSMs (read + write).  Re-encoding all
// low-level fields (e.g. PortState, BenthosMetrics.IsActive) in a new
// "converter flag bag" risks drifting out-of-sync with the canonical
// mocks that the Connection / DFC teams already maintain.
//
// Instead we embed their mocks and translate the high-level intent
// (Idle, Active, Down, …) into the exact flag structs those mocks
// expect.  One source of truth, less duplication, and all helper
// fns (`SetupConnectionServiceState`, `TransitionToDataflowComponentState`,
// …) remain reusable.
type MockService struct {
	GenerateConfigError    error
	GetConfigError         error
	StatusError            error
	AddToManagerError      error
	UpdateInManagerError   error
	RemoveFromManagerError error
	StartError             error
	StopError              error
	ForceRemoveError       error
	ReconcileManagerError  error

	// For more complex testing scenarios
	States             map[string]*ServiceInfo
	ExistingComponents map[string]bool

	// State control for FSM testing
	stateFlags map[string]*StateFlags

	/*
	   Each bridge *instance* is backed by
	     • one Connection  (talks to PLC / OPC-UA, etc.)
	     • one READ DFC    (Benthos pipeline → Kafka, …)

	   Instead of re-implementing state flags here, we embed the
	   **already battle-tested** mocks used by those FSMs.  That gives
	   us full fidelity (e.g. Nmap scan results, Benthos metrics-active
	   flag) and keeps all FSM helpers DRY.
	*/
	DfcService      *dataflowcomponent.MockDataFlowComponentService
	ConnService     *connection.MockConnectionService
	GetConfigResult bridgeserviceconfig.ConfigRuntime

	// Return values for each method
	GenerateConfigResultDFC        dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	GenerateConfigResultConnection connectionserviceconfig.ConnectionServiceConfig
	dfcConfigs                     []config.DataFlowComponentConfig
	connConfigs                    []config.ConnectionConfig

	StatusResult ServiceInfo

	// mu protects concurrent access to ExistingComponents and ConverterStates maps
	mu sync.RWMutex
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

	ServiceExistsResult        bool
	ReconcileManagerReconciled bool
}

// Ensure MockService implements IService
var _ IService = (*MockService)(nil)

// StateFlags contains all the state flags needed for FSM testing
type StateFlags struct {
	DfcFSMReadState    string
	DfcFSMWriteState   string
	ConnectionFSMState string
	RedpandaFSMState   string
	PortState          nmapfsm.PortState
	IsDFCRunning       bool
	IsConnectionUp     bool
	IsRedpandaRunning  bool
}

// NewMockService creates a new mock service
func NewMockService() *MockService {
	return &MockService{
		mu:                 sync.RWMutex{},
		States:             make(map[string]*ServiceInfo),
		ExistingComponents: make(map[string]bool),
		dfcConfigs:         make([]config.DataFlowComponentConfig, 0),
		connConfigs:        make([]config.ConnectionConfig, 0),
		stateFlags:         make(map[string]*StateFlags),
		DfcService:         dataflowcomponent.NewMockDataFlowComponentService(),
		ConnService:        connection.NewMockConnectionService(),
	}
}

// SetState updates both underlying mocks so their FSMs
// look exactly like a real system in the requested Bridge
// high-level state.  It then synthesises a single ServiceInfo snapshot
// (`States`) so the Service.Status method can
// still return aggregated data without peeking into the sub-services.
func (m *MockService) SetState(
	name string,
	flags StateFlags,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure service exists in mock
	m.ExistingComponents[name] = true

	// 1. Forward to DFC mock
	dfcFlags := BridgeToDFCFlags(flags)
	m.DfcService.SetComponentState(name, dfcFlags)

	// 2. Forward to Connection mock
	connFlags := BridgeToConnFlags(flags)
	m.ConnService.SetConnectionState(name, connFlags)

	// 3. Build a *single* aggregate ServiceInfo for Status()
	m.States[name] = BuildServiceInfo(
		name, flags, m.DfcService, m.ConnService,
	)

	// Store the flags for backward compatibility
	m.stateFlags[name] = &flags
}

// SetComponentState is kept for backward compatibility but now delegates to SetState
func (m *MockService) SetComponentState(name string, flags StateFlags) {
	m.SetState(name, flags)
}

// GetState gets the state flags for a bridge
func (m *MockService) GetState(name string) *StateFlags {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if flags, exists := m.stateFlags[name]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &StateFlags{}
	m.stateFlags[name] = flags
	return flags
}

// BridgeToDFCFlags converts high-level StateFlags into the
// exact flag struct used by the DFC mock.
func BridgeToDFCFlags(src StateFlags) dataflowcomponent.ComponentStateFlags {
	return dataflowcomponent.ComponentStateFlags{
		IsBenthosRunning:                 src.IsDFCRunning,
		BenthosFSMState:                  src.DfcFSMReadState,
		IsBenthosProcessingMetricsActive: src.IsDFCRunning && src.DfcFSMReadState == dfcfsm.OperationalStateActive,
	}
}

// BridgeToConnFlags converts high-level StateFlags into the
// flag struct the Connection mock expects.
func BridgeToConnFlags(src StateFlags) connection.ConnectionStateFlags {
	return connection.ConnectionStateFlags{
		IsNmapRunning: src.IsConnectionUp,
		NmapFSMState:  src.ConnectionFSMState,
		IsFlaky:       false, // Default to not flaky
	}
}

// BuildServiceInfo builds an aggregated ServiceInfo from the sub-mocks
func BuildServiceInfo(
	name string,
	flags StateFlags,
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
		DFCReadFSMState: flags.DfcFSMReadState,
		DFCReadObservedState: dfcfsm.DataflowComponentObservedState{
			ServiceInfo: dfcInfo,
		},
		DFCWriteFSMState: flags.DfcFSMWriteState,
		DFCWriteObservedState: dfcfsm.DataflowComponentObservedState{
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

// GetConfig mocks getting the Bridge configuration
func (m *MockService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) (
	bridgeserviceconfig.ConfigRuntime,
	error,
) {
	m.mu.Lock()
	m.GetConfigCalled = true
	m.mu.Unlock()

	// If error is set, return it
	if m.GetConfigError != nil {
		return bridgeserviceconfig.ConfigRuntime{}, m.GetConfigError
	}

	// If a result is preset, return it
	return m.GetConfigResult, nil
}

// Status mocks getting the status of a Bridge
func (m *MockService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
	name string,
) (ServiceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StatusCalled = true

	// Check if the component exists in the ExistingComponents map
	if exists, ok := m.ExistingComponents[name]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.States[name]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddToManager mocks adding a Bridge to the Connection & DFC manager
func (m *MockService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *bridgeserviceconfig.ConfigRuntime,
	name string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.AddToManagerCalled = true

	underlyingName := fmt.Sprintf("bridge-%s", name)

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
	m.ExistingComponents[name] = true

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

// UpdateInManager mocks updating a Bridge in Connection & DFC manager
func (m *MockService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *bridgeserviceconfig.ConfigRuntime,
	name string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateInManagerCalled = true

	underlyingName := fmt.Sprintf("bridge-%s", name)

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

	// Update the ConnConfig
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
func (m *MockService) RemoveFromManager(
	ctx context.Context,
	fsService filesystem.Service,
	name string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RemoveFromManagerCalled = true

	underlyingName := fmt.Sprintf("bridge-%s", name)

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
	delete(m.ExistingComponents, name)
	delete(m.States, name)

	return m.RemoveFromManagerError
}

// Start mocks starting a Bridge
func (m *MockService) Start(
	ctx context.Context,
	fsService filesystem.Service,
	name string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StartCalled = true

	underlyingName := fmt.Sprintf("bridge-%s", name)

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

// Stop mocks stopping a Bridge
func (m *MockService) Stop(
	ctx context.Context,
	fsService filesystem.Service,
	name string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StopCalled = true

	underlyingName := fmt.Sprintf("bridge-%s", name)

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

// ForceRemove mocks force removing a Bridge
func (m *MockService) ForceRemove(
	ctx context.Context,
	fsService filesystem.Service,
	name string,
) error {
	m.mu.Lock()
	m.ForceRemoveCalled = true
	m.mu.Unlock()

	return m.ForceRemoveError
}

// ServiceExists mocks checking if a Bridge exists
func (m *MockService) ServiceExists(
	ctx context.Context,
	fsService filesystem.Service,
	name string,
) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ServiceExistsCalled = true

	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the Bridge manager
func (m *MockService) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	m.mu.Lock()
	m.ReconcileManagerCalled = true
	m.mu.Unlock()

	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// EvaluateDFCDesiredStates mocks the DFC state evaluation logic.
// This method exists because bridge must re-evaluate DFC states
// when configs change during reconciliation (unlike other FSMs that set states once).
func (m *MockService) EvaluateDFCDesiredStates(
	name string,
	desiredState string,
) error {
	// Mock implementation - just update the configs like the real implementation would
	underlyingReadName := fmt.Sprintf("read-bridge-%s", name)
	underlyingWriteName := fmt.Sprintf("write-bridge-%s", name)

	// Find and update read DFC config
	for i, config := range m.dfcConfigs {
		if config.Name == underlyingReadName {
			if desiredState == "stopped" {
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

	// Find and update write DFC config
	for i, config := range m.dfcConfigs {
		if config.Name == underlyingWriteName {
			if desiredState == "stopped" {
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
