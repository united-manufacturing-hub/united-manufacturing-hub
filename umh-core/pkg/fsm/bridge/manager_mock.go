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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	bridgesvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

func NewManagerWithMockServices(name string) (*Manager, *bridgesvc.MockService) {
	mockSvc := bridgesvc.NewMockService()

	// Create a new manager instance
	// Lets create a mock manager here
	mockFSMManager := public_fsm.NewBaseFSMManager[config.BridgeConfig](
		name,
		"/dev/null",
		func(config config.FullConfig) ([]config.BridgeConfig, error) {
			return config.Bridge, nil
		},
		func(config config.BridgeConfig) (string, error) {
			return fmt.Sprintf("bridge-%s", config.Name), nil
		},
		// Get desired state for Bridge config
		func(cfg config.BridgeConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Bridge instance from config
		func(cfg config.BridgeConfig) (public_fsm.FSMInstance, error) {
			instance := NewInstance("/dev/null", cfg)
			connectionServiceMock := connection.NewMockConnectionService()
			dfcServiceMock := dfcsvc.NewMockDataFlowComponentService()

			mockSvc.ConnService = connectionServiceMock
			mockSvc.DfcService = dfcServiceMock

			// TODO: potentially pre-configure these mock services here

			instance.service = mockSvc
			return instance, nil
		},
		// Compare Bridge configs
		func(instance public_fsm.FSMInstance, cfg config.BridgeConfig) (bool, error) {
			bridgeInstance, ok := instance.(*Instance)
			if !ok {
				return false, fmt.Errorf("instance is not a Bridge Instance")
			}

			// Perform actual comparison - return true if configs are equal
			configsEqual := bridgeserviceconfig.ConfigsEqual(bridgeInstance.configSpec, cfg.ServiceConfig)

			// Only update config if configs are different (for mock service)
			if !configsEqual {
				bridgeInstance.configSpec = cfg.ServiceConfig
				if mockSvc, ok := bridgeInstance.service.(*bridgesvc.MockService); ok {
					runtimeConfig, err := bridgeserviceconfig.SpecToRuntime(cfg.ServiceConfig)
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
		// Set Bridge config
		func(instance public_fsm.FSMInstance, cfg config.BridgeConfig) error {
			bridgeInstance, ok := instance.(*Instance)
			if !ok {
				return fmt.Errorf("instance is not a Bridge Instance")
			}
			bridgeInstance.configSpec = cfg.ServiceConfig
			if mockSvc, ok := bridgeInstance.service.(*bridgesvc.MockService); ok {
				runtimeConfig, err := bridgeserviceconfig.SpecToRuntime(cfg.ServiceConfig)
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
			brInstance, ok := instance.(*Instance)
			if !ok {
				return 0, fmt.Errorf("instance is not a Bridge Instance")
			}
			return brInstance.GetMinimumRequiredTime(), nil
		},
	)

	mockManager := &Manager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
