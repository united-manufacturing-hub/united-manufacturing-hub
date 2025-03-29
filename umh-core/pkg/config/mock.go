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
	"sync"
	"time"
)

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	GetConfigCalled bool
	Config          FullConfig
	ConfigError     error
	ConfigDelay     time.Duration
	mutex           sync.Mutex
}

// NewMockConfigManager creates a new MockConfigManager instance
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{}
}

// GetConfig implements the ConfigManager interface
func (m *MockConfigManager) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
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
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.GetConfigCalled = false
}
