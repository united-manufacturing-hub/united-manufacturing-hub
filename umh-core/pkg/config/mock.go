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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	GetConfigCalled               bool
	AddDataflowcomponentCalled    bool
	DeleteDataflowcomponentCalled bool
	EditDataflowcomponentCalled   bool
	Config                        FullConfig
	ConfigError                   error
	AddDataflowcomponentError     error
	DeleteDataflowcomponentError  error
	EditDataflowcomponentError    error
	ConfigDelay                   time.Duration
	mutexReadOrWrite              sync.Mutex
	mutexReadAndWrite             sync.Mutex
}

// NewMockConfigManager creates a new MockConfigManager instance
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{}
}

// GetDataFlowConfig returns the DataFlow component configurations
func (m *MockConfigManager) GetDataFlowConfig() []DataFlowComponentConfig {
	return m.Config.DataFlow
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

// WithAddDataflowcomponentError configures the mock to return the given error when AtomicAddDataflowcomponent is called
func (m *MockConfigManager) WithAddDataflowcomponentError(err error) *MockConfigManager {
	m.AddDataflowcomponentError = err
	return m
}

// WithDeleteDataflowcomponentError configures the mock to return the given error when AtomicDeleteDataflowcomponent is called
func (m *MockConfigManager) WithDeleteDataflowcomponentError(err error) *MockConfigManager {
	m.DeleteDataflowcomponentError = err
	return m
}

// WithEditDataflowcomponentError configures the mock to return the given error when AtomicEditDataflowcomponent is called
func (m *MockConfigManager) WithEditDataflowcomponentError(err error) *MockConfigManager {
	m.EditDataflowcomponentError = err
	return m
}

// ResetCalls clears the called flags for testing multiple calls
func (m *MockConfigManager) ResetCalls() {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()
	m.GetConfigCalled = false
	m.AddDataflowcomponentCalled = false
	m.DeleteDataflowcomponentCalled = false
	m.EditDataflowcomponentCalled = false
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

	m.AddDataflowcomponentCalled = true

	if m.AddDataflowcomponentError != nil {
		return m.AddDataflowcomponentError
	}

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

// AtomicDeleteDataflowcomponent implements the ConfigManager interface
func (m *MockConfigManager) AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.DeleteDataflowcomponentCalled = true

	if m.DeleteDataflowcomponentError != nil {
		return m.DeleteDataflowcomponentError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find and remove the component with matching UUID
	found := false
	filteredComponents := make([]DataFlowComponentConfig, 0, len(config.DataFlow))

	for _, component := range config.DataFlow {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID != componentUUID {
			filteredComponents = append(filteredComponents, component)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("dataflow component with UUID %s not found", componentUUID)
	}

	// Update config with filtered components
	config.DataFlow = filteredComponents

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditDataflowcomponent implements the ConfigManager interface
func (m *MockConfigManager) AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.EditDataflowcomponentCalled = true

	if m.EditDataflowcomponentError != nil {
		return DataFlowComponentConfig{}, m.EditDataflowcomponentError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// Find the component with matching UUID
	found := false
	var oldConfig DataFlowComponentConfig
	for i, component := range config.DataFlow {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID == componentUUID {
			// Found the component to edit, update it
			oldConfig = component
			config.DataFlow[i] = dfc
			found = true
			break
		}
	}

	if !found {
		return DataFlowComponentConfig{}, fmt.Errorf("dataflow component with UUID %s not found", componentUUID)
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicAddProtocolConverter implements the ConfigManager interface
func (m *MockConfigManager) AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// add the protocol converter
	config.ProtocolConverter = append(config.ProtocolConverter, pc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
