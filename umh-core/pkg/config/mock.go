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

package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	GetConfigCalled   bool
	Config            FullConfig
	ConfigError       error
	ConfigDelay       time.Duration
	mutexReadOrWrite  sync.Mutex
	mutexReadAndWrite sync.Mutex
}

// NewMockConfigManager creates a new MockConfigManager instance
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{}
}

// GetConfig implements the ConfigManager interface
func (m *MockConfigManager) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()
	m.GetConfigCalled = true

	if m.ConfigDelay > 0 {
		select {
		case <-time.After(m.ConfigDelay):
			// Delay completed
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		}
	}

	return m.Config, m.ConfigError
}

// WriteConfig implements the ConfigManager interface
func (m *MockConfigManager) writeConfig(ctx context.Context, cfg FullConfig) error {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()

	m.Config = cfg
	return nil
}

// WithConfig configures the mock to return the given config
func (m *MockConfigManager) WithConfig(cfg FullConfig) *MockConfigManager {
	m.Config = cfg
	return m
}

// WithConfigError configures the mock to return the given error
func (m *MockConfigManager) WithConfigError(err error) *MockConfigManager {
	m.ConfigError = err
	return m
}

// WithConfigDelay configures the mock to delay for the given duration
func (m *MockConfigManager) WithConfigDelay(delay time.Duration) *MockConfigManager {
	m.ConfigDelay = delay
	return m
}

// ResetCalls clears the called flags for testing multiple calls
func (m *MockConfigManager) ResetCalls() {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()
	m.GetConfigCalled = false
}

// atomic set location
func (m *MockConfigManager) AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	config.Agent.Location = make(map[int]string)
	config.Agent.Location[0] = location.Enterprise
	if location.Site != nil {
		config.Agent.Location[1] = *location.Site
	}
	if location.Area != nil {
		config.Agent.Location[2] = *location.Area
	}
	if location.Line != nil {
		config.Agent.Location[3] = *location.Line
	}
	if location.WorkCell != nil {
		config.Agent.Location[4] = *location.WorkCell
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// atomic add dataflowcomponent
func (m *MockConfigManager) AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// edit the config
	config.DataFlow = append(config.DataFlow, dfc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
