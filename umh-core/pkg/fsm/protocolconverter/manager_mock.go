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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

func NewProtocolConverterManagerWithMockedServices(name string) (*ProtocolConverterManager, *protocolconvertersvc.MockProtocolConverterService) {
	mockSvc := protocolconvertersvc.NewMockProtocolConverterService()

	// Create a new manager instance
	// Lets create a mock manager here
	mockFSMManager := public_fsm.NewBaseFSMManager[config.ProtocolConverterConfig](
		name,
		"/dev/null",
		func(config config.FullConfig) ([]config.ProtocolConverterConfig, error) {
			return config.ProtocolConverter, nil
		},
		func(config config.ProtocolConverterConfig) (string, error) {
			return fmt.Sprintf("protocolconverter-%s", config.Name), nil
		},
		// Get desired state for ProtocolConverter config
		func(cfg config.ProtocolConverterConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create ProtocolConverter instance from config
		func(cfg config.ProtocolConverterConfig) (public_fsm.FSMInstance, error) {
			instance := NewProtocolConverterInstance("/dev/null", cfg)
			connectionServiceMock := connection.NewMockConnectionService()
			dfcServiceMock := dfcsvc.NewMockDataFlowComponentService()

			mockSvc.ConnService = connectionServiceMock
			mockSvc.DfcService = dfcServiceMock

			// TODO: potentially pre-configure these mock services here

			instance.service = mockSvc
			return instance, nil
		},
		// Compare ProtocolConverter configs
		func(instance public_fsm.FSMInstance, cfg config.ProtocolConverterConfig) (bool, error) {
			protocolConverterInstance, ok := instance.(*ProtocolConverterInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a ProtocolConverterInstance")
			}

			// Perform actual comparison - return true if configs are equal
			configsEqual := bridgeserviceconfig.ConfigsEqual(protocolConverterInstance.specConfig, cfg.ProtocolConverterServiceConfig)

			// Only update config if configs are different (for mock service)
			if !configsEqual {
				protocolConverterInstance.specConfig = cfg.ProtocolConverterServiceConfig
				if mockSvc, ok := protocolConverterInstance.service.(*protocolconvertersvc.MockProtocolConverterService); ok {
					runtimeConfig, err := bridgeserviceconfig.SpecToRuntime(cfg.ProtocolConverterServiceConfig)
					if err != nil {
						// For invalid configs, don't update the mock service but don't fail the comparison
						// This matches the behavior of the real manager where invalid configs are handled gracefully
						return configsEqual, nil
					}
					mockSvc.GetConfigResult = runtimeConfig
				}
			}

			return configsEqual, nil
		},
		// Set ProtocolConverter config
		func(instance public_fsm.FSMInstance, cfg config.ProtocolConverterConfig) error {
			protocolConverterInstance, ok := instance.(*ProtocolConverterInstance)
			if !ok {
				return fmt.Errorf("instance is not a ProtocolConverterInstance")
			}
			protocolConverterInstance.specConfig = cfg.ProtocolConverterServiceConfig
			if mockSvc, ok := protocolConverterInstance.service.(*protocolconvertersvc.MockProtocolConverterService); ok {
				runtimeConfig, err := bridgeserviceconfig.SpecToRuntime(cfg.ProtocolConverterServiceConfig)
				if err != nil {
					// For invalid configs, don't update the mock service but don't fail the set operation
					// This matches the behavior of the real manager where invalid configs are handled gracefully
					return nil
				}
				mockSvc.GetConfigResult = runtimeConfig
			}
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			protocolConverterInstance, ok := instance.(*ProtocolConverterInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a ProtocolConverterInstance")
			}
			return protocolConverterInstance.GetMinimumRequiredTime(), nil
		},
	)

	mockManager := &ProtocolConverterManager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
