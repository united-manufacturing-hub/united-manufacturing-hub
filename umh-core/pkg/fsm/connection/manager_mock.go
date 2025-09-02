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

package connection

import (
	"errors"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
)

func NewConnectionManagerWithMockedServices(name string) (*ConnectionManager, *connection.MockConnectionService) {
	mockSvc := connection.NewMockConnectionService()

	// Create a new manager instance
	// Lets create a mock manager here
	mockFSMManager := public_fsm.NewBaseFSMManager[config.ConnectionConfig](
		name,
		"/dev/null",
		func(config config.FullConfig) ([]config.ConnectionConfig, error) {
			return config.Internal.Connection, nil
		},
		func(config config.ConnectionConfig) (string, error) {
			return "connection-" + config.Name, nil
		},
		// Get desired state for Connection config
		func(cfg config.ConnectionConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Connection instance from config
		func(cfg config.ConnectionConfig) (public_fsm.FSMInstance, error) {
			instance := NewConnectionInstance("/dev/null", cfg)
			nmapMockService := nmap.NewMockNmapService()
			s6MockService := s6_default.NewMockService()
			s6MockService.ServiceExistsResult = true
			s6MockService.StatusResult = s6_shared.ServiceInfo{
				Status: s6_default.ServiceUp,
				Pid:    12345, // Fake PID
				Uptime: 60,    // Fake uptime in seconds
			}
			nmapMockService.S6Service = s6MockService
			mockSvc.NmapService = nmapMockService
			instance.service = mockSvc

			return instance, nil
		},
		// Compare Connection configs
		func(instance public_fsm.FSMInstance, cfg config.ConnectionConfig) (bool, error) {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return false, errors.New("instance is not a ConnectionInstance")
			}

			connectionInstance.config = cfg.ConnectionServiceConfig
			if mockSvc, ok := connectionInstance.service.(*connection.MockConnectionService); ok {
				mockSvc.GetConfigResult = cfg.ConnectionServiceConfig
			}

			return true, nil
		},
		// Set Connection config
		func(instance public_fsm.FSMInstance, cfg config.ConnectionConfig) error {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return errors.New("instance is not a ConnectionInstance")
			}

			connectionInstance.config = cfg.ConnectionServiceConfig
			if mockSvc, ok := connectionInstance.service.(*connection.MockConnectionService); ok {
				mockSvc.GetConfigResult = cfg.ConnectionServiceConfig
			}

			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return 0, errors.New("instance is not a ConnectionInstance")
			}

			return connectionInstance.GetMinimumRequiredTime(), nil
		},
	)

	mockManager := &ConnectionManager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
