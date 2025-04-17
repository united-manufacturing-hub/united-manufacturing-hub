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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

func NewDataflowComponentManagerWithMockedServices(name string) (*DataflowComponentManager, *dataflowcomponentsvc.MockDataFlowComponentService) {

	mockSvc := dataflowcomponentsvc.NewMockDataFlowComponentService()

	// Configure the mock S6 service
	mockBenthosService := mockSvc.BenthosService.(*benthossvc.MockBenthosService)
	s6MockService := mockBenthosService.S6Service.(*s6svc.MockService)

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
	// Create a new manager instance
	archiveStorage := storage.NewArchiveEventStorage(100)
	// Lets create a mock manager here
	mockFSMManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		name,
		"/dev/null",
		func(config config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			return config.DataFlow, nil
		},
		func(config config.DataFlowComponentConfig) (string, error) {
			return fmt.Sprintf("dataflow-%s", config.Name), nil
		},
		// Get desired state for Dataflowcomponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Dataflowcomponent instance from config
		func(cfg config.DataFlowComponentConfig) (public_fsm.FSMInstance, error) {
			instance := NewDataflowComponentInstance("/dev/null", cfg, archiveStorage)
			benthosMockService := benthossvc.NewMockBenthosService()
			s6MockService := s6svc.NewMockService()
			s6MockService.ServiceExistsResult = true
			s6MockService.StatusResult = s6svc.ServiceInfo{
				Status: s6svc.ServiceUp,
				Pid:    12345, // Fake PID
				Uptime: 60,    // Fake uptime in seconds
			}
			benthosMockService.S6Service = s6MockService
			mockSvc.BenthosService = benthosMockService
			instance.service = mockSvc
			return instance, nil
		},
		// Compare Dataflowcomponent configs
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			dataflowComponentInstance.config = cfg.DataFlowComponentConfig
			if mockSvc, ok := dataflowComponentInstance.service.(*dataflowcomponentsvc.MockDataFlowComponentService); ok {
				mockSvc.GetConfigResult = cfg.DataFlowComponentConfig
			}
			return true, nil
		},
		// Set DataflowComponent config
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) error {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			dataflowComponentInstance.config = cfg.DataFlowComponentConfig
			if mockSvc, ok := dataflowComponentInstance.service.(*dataflowcomponentsvc.MockDataFlowComponentService); ok {
				mockSvc.GetConfigResult = cfg.DataFlowComponentConfig
			}
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			return dataflowComponentInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	mockManager := &DataflowComponentManager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
