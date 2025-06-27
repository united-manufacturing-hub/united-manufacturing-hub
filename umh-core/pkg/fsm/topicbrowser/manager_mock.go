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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// NewTopicBrowserManagerWithMockedServices creates a TopicBrowserManager with fully mocked services
func NewTopicBrowserManagerWithMockedServices(name string) (*Manager, *topicbrowsersvc.MockService) {

	mockSvc := topicbrowsersvc.NewMockService()

	// Create a new manager instance
	mockFSMManager := public_fsm.NewBaseFSMManager[config.TopicBrowserConfig](
		name,
		"/dev/null",
		func(fullConfig config.FullConfig) ([]config.TopicBrowserConfig, error) {
			tbConfig := fullConfig.Internal.TopicBrowser
			return []config.TopicBrowserConfig{tbConfig}, nil
		},
		func(config config.TopicBrowserConfig) (string, error) {
			return fmt.Sprintf("topicbrowser-%s", config.Name), nil
		},
		// Get desired state for TopicBrowser config
		func(cfg config.TopicBrowserConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create TopicBrowser instance from config
		func(cfg config.TopicBrowserConfig) (public_fsm.FSMInstance, error) {
			instance := NewInstance("/dev/null", cfg)

			// Attach the shared mock service to this instance
			instance.SetService(mockSvc)
			return instance, nil
		},
		// Compare TopicBrowser configs
		func(instance public_fsm.FSMInstance, cfg config.TopicBrowserConfig) (bool, error) {
			topicBrowserInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a TopicBrowserInstance")
			}

			// Perform actual comparison - return true if configs are equal
			configsEqual := topicBrowserInstance.GetConfig().Equal(cfg.TopicBrowserServiceConfig)

			// Only update config if configs are different (for mock service)
			if !configsEqual {
				topicBrowserInstance.config = cfg.TopicBrowserServiceConfig
				// No need to update mock service config as TopicBrowser doesn't use it the same way
			}

			return configsEqual, nil
		},
		// Set TopicBrowser config
		func(instance public_fsm.FSMInstance, cfg config.TopicBrowserConfig) error {
			topicBrowserInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return fmt.Errorf("instance is not a TopicBrowserInstance")
			}
			topicBrowserInstance.config = cfg.TopicBrowserServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			topicBrowserInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a TopicBrowserInstance")
			}
			return topicBrowserInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	mockManager := &Manager{
		BaseFSMManager: mockFSMManager,
	}

	return mockManager, mockSvc
}
