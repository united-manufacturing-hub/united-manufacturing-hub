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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
)

func NewManagerWithMockedServices(name string) (*Manager, *spsvc.MockService) {

	mockSvc := spsvc.NewMockService()

	// Create a new manager instance
	// Lets create a mock manager here
	mockFSMManager := public_fsm.NewBaseFSMManager[config.StreamProcessorConfig](
		name,
		"/dev/null",
		func(config config.FullConfig) ([]config.StreamProcessorConfig, error) {
			return config.StreamProcessor, nil
		},
		func(config config.StreamProcessorConfig) (string, error) {
			return config.Name, nil
		},
		// Get desired state for StreamProcessor config
		func(cfg config.StreamProcessorConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create StreamProcessor instance from config
		func(cfg config.StreamProcessorConfig) (public_fsm.FSMInstance, error) {
			instance := NewInstance("/dev/null", cfg)
			dfcServiceMock := dfcsvc.NewMockDataFlowComponentService()

			mockSvc.DfcService = dfcServiceMock

			// TODO: potentially pre-configure these mock services here

			instance.service = mockSvc
			return instance, nil
		},
		// Compare StreamProcessor configs
		func(instance public_fsm.FSMInstance, cfg config.StreamProcessorConfig) (bool, error) {
			streamProcessorInstance, ok := instance.(*Instance)
			if !ok {
				return false, fmt.Errorf("instance is not a StreamProcessorInstance")
			}

			// Perform actual comparison - return true if configs are equal
			configsEqual := streamprocessorserviceconfig.ConfigsEqual(streamProcessorInstance.specConfig, cfg.StreamProcessorServiceConfig)

			// Only update config if configs are different (for mock service)
			if !configsEqual {
				streamProcessorInstance.specConfig = cfg.StreamProcessorServiceConfig
				if mockSvc, ok := streamProcessorInstance.service.(*spsvc.MockService); ok {
					runtimeConfig := streamprocessorserviceconfig.SpecToRuntime(cfg.StreamProcessorServiceConfig)
					mockSvc.GetConfigResult = runtimeConfig
				}
			}

			return configsEqual, nil
		},
		// Set StreamProcessor config
		func(instance public_fsm.FSMInstance, cfg config.StreamProcessorConfig) error {
			streamProcessorInstance, ok := instance.(*Instance)
			if !ok {
				return fmt.Errorf("instance is not a StreamProcessorInstance")
			}
			streamProcessorInstance.specConfig = cfg.StreamProcessorServiceConfig
			if mockSvc, ok := streamProcessorInstance.service.(*spsvc.MockService); ok {
				runtimeConfig := streamprocessorserviceconfig.SpecToRuntime(cfg.StreamProcessorServiceConfig)
				mockSvc.GetConfigResult = runtimeConfig
			}
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			streamProcessorInstance, ok := instance.(*Instance)
			if !ok {
				return 0, fmt.Errorf("instance is not a StreamProcessorInstance")
			}
			return streamProcessorInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	mockManager := &Manager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
