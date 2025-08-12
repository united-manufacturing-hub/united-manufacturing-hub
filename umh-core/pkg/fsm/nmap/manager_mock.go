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

package nmap

import (
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_orig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

func NewNmapManagerWithMockedService(name string) (*NmapManager, *nmap.MockNmapService) {
	managerName := fmt.Sprintf("%s_mock_%s", logger.ComponentNmapManager, name)

	// Create a single shared mock service that will be used by all instances
	mockService := nmap.NewMockNmapService()

	baseMgr := public_fsm.NewBaseFSMManager[config.NmapConfig](
		managerName,
		"/dev/null",
		// For the mock, we'll just pretend to parse from FullConfig
		func(fc config.FullConfig) ([]config.NmapConfig, error) {
			// In a real test, you'd define fc.Nmap with test data
			return fc.Internal.Nmap, nil
		},
		func(nc config.NmapConfig) (string, error) {
			return nc.Name, nil
		},
		func(nc config.NmapConfig) (string, error) {
			return nc.DesiredFSMState, nil
		},
		func(nc config.NmapConfig) (public_fsm.FSMInstance, error) {
			inst := NewNmapInstance(nc)

			// Create a mock S6 service and attach it to the Benthos mock
			s6MockService := s6svc.NewMockService()

			// Configure mock S6 service
			s6MockService.ServiceExistsResult = true
			s6MockService.StatusResult = s6_shared.ServiceInfo{
				Status:    s6_shared.ServiceUp, // Start in UP state
				Pid:       12345,
				Uptime:    60,
				IsReady:   true,
				WantUp:    true,
				ReadyTime: 30,
			}

			// Replace S6 service in Benthos mock
			mockService.S6Service = s6MockService

			// Setup default config result
			mockService.GetConfigResult = nc.NmapServiceConfig

			// Attach the shared mock service to this instance
			inst.monitorService = mockService

			return inst, nil
		},
		func(instance public_fsm.FSMInstance, cfg config.NmapConfig) (bool, error) {
			nmapInstance, ok := instance.(*NmapInstance)
			if !ok {
				return false, errors.New("instance not a NmapInstance")
			}

			nmapInstance.config.NmapServiceConfig = cfg.NmapServiceConfig
			// Compare both FSMInstanceConfig and service config
			if mockService, ok := nmapInstance.monitorService.(*nmap.MockNmapService); ok {
				mockService.GetConfigResult = cfg.NmapServiceConfig
			}

			// Always return true in mocked tests to avoid unnecessary recreation
			return true, nil
		},
		func(instance public_fsm.FSMInstance, nc config.NmapConfig) error {
			ni, ok := instance.(*NmapInstance)
			if !ok {
				return errors.New("instance not a NmapInstance")
			}

			ni.config = nc

			if mockService, ok := ni.monitorService.(*nmap.MockNmapService); ok {
				mockService.GetConfigResult = nc.NmapServiceConfig
			}

			return nil
		},
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ni, ok := instance.(*NmapInstance)
			if !ok {
				return 0, errors.New("instance not a NmapInstance")
			}

			return ni.GetMinimumRequiredTime(), nil
		},
	)

	logger.For(managerName).Info("Created NmapManager with mocked service.")

	return &NmapManager{
		BaseFSMManager: baseMgr,
	}, mockService
}
