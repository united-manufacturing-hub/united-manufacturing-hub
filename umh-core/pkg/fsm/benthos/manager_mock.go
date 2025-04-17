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

package benthos

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

// NewBenthosManagerWithMockedServices creates a BenthosManager with fully mocked services
// that never touch the filesystem or real network ports. Use this for testing manager logic
// without real Benthos or S6 interactions.
func NewBenthosManagerWithMockedServices(name string) (*BenthosManager, *benthossvc.MockBenthosService) {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosManager, name)

	// Create a single shared mock service that will be used by all instances
	mockService := benthossvc.NewMockBenthosService()

	archiveStorage := storage.NewArchiveEventStorage(100)

	// Initialize state maps if they don't exist
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	}

	baseManager := public_fsm.NewBaseFSMManager[config.BenthosConfig](
		managerName,
		"/dev/null", // Prevent any real filesystem writes
		// Extract Benthos configs from full config - same as original
		func(fullConfig config.FullConfig) ([]config.BenthosConfig, error) {
			return fullConfig.Internal.Benthos, nil
		},
		// Get name from Benthos config - same as original
		func(cfg config.BenthosConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from Benthos config - same as original
		func(cfg config.BenthosConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Benthos instance from config - with mock service
		func(cfg config.BenthosConfig) (public_fsm.FSMInstance, error) {
			// Create an instance with the basic config
			instance := NewBenthosInstance(cfg, archiveStorage)

			// Create a mock S6 service and attach it to the Benthos mock
			s6MockService := s6svc.NewMockService()

			// Configure mock S6 service
			s6MockService.ServiceExistsResult = true
			s6MockService.StatusResult = s6svc.ServiceInfo{
				Status:    s6svc.ServiceUp, // Start in UP state
				Pid:       12345,
				Uptime:    60,
				IsReady:   true,
				WantUp:    true,
				ReadyTime: 30,
			}

			// Replace S6 service in Benthos mock
			mockService.S6Service = s6MockService

			// Setup default config result
			mockService.GetConfigResult = cfg.BenthosServiceConfig

			// Attach the shared mock service to this instance
			instance.service = mockService

			return instance, nil
		},
		// Compare Benthos configs - always return true to avoid unnecessary recreation
		func(instance public_fsm.FSMInstance, cfg config.BenthosConfig) (bool, error) {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a BenthosInstance")
			}

			// For tests, just update the config instead of triggering recreation
			benthosInstance.config = cfg.BenthosServiceConfig

			// If we have a mock service, update its config too
			if mockSvc, ok := benthosInstance.service.(*benthossvc.MockBenthosService); ok {
				mockSvc.GetConfigResult = cfg.BenthosServiceConfig
			}

			// Always return true in mocked tests to avoid unnecessary recreation
			return true, nil
		},
		// Set Benthos config - update mock as well
		func(instance public_fsm.FSMInstance, cfg config.BenthosConfig) error {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return fmt.Errorf("instance is not a BenthosInstance")
			}

			// Update the instance config
			benthosInstance.config = cfg.BenthosServiceConfig

			// Update the mock service config too
			if mockSvc, ok := benthosInstance.service.(*benthossvc.MockBenthosService); ok {
				mockSvc.GetConfigResult = cfg.BenthosServiceConfig
			}

			return nil
		},
		// Get expected max p95 execution time per instance - same as original
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			return constants.BenthosExpectedMaxP95ExecutionTimePerInstance, nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentBenthosManager, name)

	// Use a mock port manager to avoid real port allocation
	portManager := portmanager.NewMockPortManager()

	manager := &BenthosManager{
		BaseFSMManager: baseManager,
		portManager:    portManager,
	}

	return manager, mockService
}
