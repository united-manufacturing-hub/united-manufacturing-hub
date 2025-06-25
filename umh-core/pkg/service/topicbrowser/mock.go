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
	benthossvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	rpfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	rpsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	rpmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
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
	GenerateConfigResult       benthossvccfg.BenthosServiceConfig
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
	States         map[string]*ServiceInfo
	Existing       map[string]bool
	BenthosConfigs []config.BenthosConfig

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
	HasBenthosOutput      bool
}

// NewMockService creates a new mock topic browser service
func NewMockService() *MockService {
	return &MockService{
		States:         make(map[string]*ServiceInfo),
		Existing:       make(map[string]bool),
		stateFlags:     make(map[string]*StateFlags),
		BenthosService: benthossvc.NewMockBenthosService(),
	}
}

// SetState sets all state flags for a topic browser at once
func (m *MockService) SetState(tbName string, flags StateFlags) {
	bytesOut := 1024
	batchSent := 1

	if !flags.HasBenthosOutput {
		batchSent = 0
	}
	benthosObservedState := &benthosfsm.BenthosObservedState{
		ServiceInfo: benthossvc.ServiceInfo{
			BenthosStatus: benthossvc.BenthosStatus{
				BenthosMetrics: benthos_monitor.BenthosMetrics{
					Metrics: benthos_monitor.Metrics{
						Output: benthos_monitor.OutputMetrics{
							ConnectionUp: 1,
							BatchSent:    int64(batchSent),
						},
					},
				},
			},
		},
	}

	if !flags.HasProcessingActivity {
		bytesOut = 0
	}

	rpObservedState := &rpfsm.RedpandaObservedState{
		ServiceInfo: rpsvc.ServiceInfo{
			RedpandaStatus: rpsvc.RedpandaStatus{
				RedpandaMetrics: rpmonitor.RedpandaMetrics{
					Metrics: rpmonitor.Metrics{
						Throughput: rpmonitor.ThroughputMetrics{
							BytesOut: int64(bytesOut),
						},
					},
				},
			},
		},
	}
	// Ensure ServiceInfo exists for this topic browser
	if _, exists := m.States[tbName]; !exists {
		m.States[tbName] = &ServiceInfo{
			BenthosFSMState:       flags.BenthosFSMState,
			RedpandaFSMState:      flags.RedpandaFSMState,
			RedpandaObservedState: *rpObservedState,
			BenthosObservedState:  *benthosObservedState,
		}
	} else {
		m.States[tbName].BenthosObservedState = *benthosObservedState
		m.States[tbName].RedpandaObservedState = *rpObservedState
		m.States[tbName].BenthosFSMState = flags.BenthosFSMState
		m.States[tbName].RedpandaFSMState = flags.RedpandaFSMState
	}

	// Store the flags
	m.stateFlags[tbName] = &flags
}

// GetState gets the state flags for a topic browser
func (m *MockService) GetState(tbName string) *StateFlags {
	if flags, exists := m.stateFlags[tbName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &StateFlags{}
	m.stateFlags[tbName] = flags
	return flags
}

// GenerateConfig mocks generating Benthos config for a topic browser
func (m *MockService) GenerateConfig(tbName string) (benthossvccfg.BenthosServiceConfig, error) {
	m.GenerateConfigCalled = true
	return m.GenerateConfigResult, m.GenerateConfigError
}

// Status mocks getting the status of a topic browser
func (m *MockService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	tbName string,
	snapshot fsm.SystemSnapshot,
) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the topic browser exists in the Existing map
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

// AddToManager mocks adding a TopicBrowser to the Benthos manager
func (m *MockService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *benthossvccfg.BenthosServiceConfig,
	tbName string,
) error {
	m.AddToManagerCalled = true

	benthosName := fmt.Sprintf("topicbrowser-%s", tbName)

	// Check whether the topic browser already exists
	for _, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the topic browser to the list of existing ones
	m.Existing[tbName] = true

	// Create a BenthosConfig for this topic browser
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

// UpdateInManager mocks updating a TopicBrowser in the Benthos manager
func (m *MockService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *benthossvccfg.BenthosServiceConfig,
	tbName string,
) error {
	m.UpdateInManagerCalled = true

	benthosName := fmt.Sprintf("topicbrowser-%s", tbName)

	// Check if the topic browser exists
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
		return ErrServiceNotExist
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

// RemoveFromManager mocks removing a TopicBrowser from the Benthos manager
func (m *MockService) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	m.RemoveFromManagerCalled = true

	benthosName := fmt.Sprintf("topicbrowser-%s", tbName)

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
		return ErrServiceNotExist
	}

	// Remove the topic browser from the list of existing ones
	delete(m.Existing, tbName)
	delete(m.States, tbName)

	return m.RemoveFromManagerError
}

// Start mocks starting a Topic Browser
func (m *MockService) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	m.StartCalled = true

	benthosName := fmt.Sprintf("topicbrowser-%s", tbName)

	found := false

	// Set the desired state to active for the given topic browser
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateActive
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StartError
}

// Stop mocks stopping a Topic Browser
func (m *MockService) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	m.StopCalled = true

	benthosName := fmt.Sprintf("topicbrowser-%s", tbName)

	found := false

	// Set the desired state to stopped for the given topic browser
	for i, benthosConfig := range m.BenthosConfigs {
		if benthosConfig.Name == benthosName {
			m.BenthosConfigs[i].DesiredFSMState = benthosfsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StopError
}

// ForceRemove mocks force removing a Topic Browser
func (m *MockService) ForceRemove(
	ctx context.Context,
	services serviceregistry.Provider,
	tbname string,
) error {
	m.ForceRemoveCalled = true
	return m.ForceRemoveError
}

// ServiceExists mocks checking if a topic browser exists
func (m *MockService) ServiceExists(
	ctx context.Context,
	services serviceregistry.Provider,
	tbName string,
) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the topic browser manager
func (m *MockService) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}
