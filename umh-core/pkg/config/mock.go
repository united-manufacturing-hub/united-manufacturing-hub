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
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	GetConfigCalled                   bool
	AddDataflowcomponentCalled        bool
	DeleteDataflowcomponentCalled     bool
	EditDataflowcomponentCalled       bool
	AtomicAddProtocolConverterCalled  bool
	AtomicEditProtocolConverterCalled bool
	Config                            FullConfig
	ConfigError                       error
	AddDataflowcomponentError         error
	DeleteDataflowcomponentError      error
	EditDataflowcomponentError        error
	AtomicAddProtocolConverterError   error
	AtomicEditProtocolConverterError  error
	ConfigAsString                    string
	GetConfigAsStringError            error
	GetConfigAsStringCalled           bool
	ConfigDelay                       time.Duration
	mutexReadOrWrite                  sync.Mutex
	mutexReadAndWrite                 sync.Mutex
	MockFileSystem                    *filesystem.MockFileSystem
	CacheModTime                      time.Time
}

// NewMockConfigManager creates a new MockConfigManager instance
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		MockFileSystem: filesystem.NewMockFileSystem(),
	}
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

// GetFileSystemService returns the mock filesystem service
func (m *MockConfigManager) GetFileSystemService() filesystem.Service {
	return m.MockFileSystem
}

// WriteConfig implements the ConfigManager interface
// all the functions that call MockConfigManager.writeConfig must hold the mutexReadAndWrite mutex
func (m *MockConfigManager) writeConfig(ctx context.Context, cfg FullConfig) error {
	m.Config = cfg
	m.CacheModTime = time.Now()
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

// WithAtomicAddProtocolConverterError configures the mock to return the given error when AtomicAddProtocolConverter is called
func (m *MockConfigManager) WithAtomicAddProtocolConverterError(err error) *MockConfigManager {
	m.AtomicAddProtocolConverterError = err
	return m
}

// WithAtomicEditProtocolConverterError configures the mock to return the given error when AtomicEditProtocolConverter is called
func (m *MockConfigManager) WithAtomicEditProtocolConverterError(err error) *MockConfigManager {
	m.AtomicEditProtocolConverterError = err
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
	m.AtomicAddProtocolConverterCalled = false
	m.AtomicEditProtocolConverterCalled = false
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

	m.AtomicAddProtocolConverterCalled = true

	if m.AtomicAddProtocolConverterError != nil {
		return m.AtomicAddProtocolConverterError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add (similar to real implementation)
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == pc.Name {
			return fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// Generate template name from protocol converter name (simulate real implementation)
	templateName := generateMockTemplateAnchorName(pc.Name)

	// Create a simple template to simulate template creation
	templateWithAnchor := map[string]interface{}{
		templateName: map[string]interface{}{
			"connection": map[string]interface{}{
				"ip":   "{{ .IP }}",
				"port": "{{ .PORT }}",
			},
		},
		"_anchor": templateName, // Metadata to indicate this should be an anchor
	}
	config.Templates = append(config.Templates, templateWithAnchor)

	// add the protocol converter
	config.ProtocolConverter = append(config.ProtocolConverter, pc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditProtocolConverter implements the ConfigManager interface
func (m *MockConfigManager) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditProtocolConverterCalled = true

	if m.AtomicEditProtocolConverterError != nil {
		return ProtocolConverterConfig{}, m.AtomicEditProtocolConverterError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// Find the protocol converter with matching UUID
	found := false
	var oldConfig ProtocolConverterConfig
	for i, cmp := range config.ProtocolConverter {
		cmpID := dataflowcomponentserviceconfig.GenerateUUIDFromName(cmp.Name)
		if cmpID == componentUUID {
			// Found the protocol converter to edit, update it
			oldConfig = cmp
			config.ProtocolConverter[i] = pc
			found = true
			break
		}
	}

	if !found {
		return ProtocolConverterConfig{}, fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// generateMockTemplateAnchorName creates a valid YAML anchor name from a protocol converter name
// This mirrors the real implementation for testing
func generateMockTemplateAnchorName(pcName string) string {
	// Replace non-alphanumeric characters with underscores and add template suffix
	// YAML anchors must contain only alphanumeric characters
	result := ""
	for _, r := range pcName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else {
			result += "_"
		}
	}
	return result + "_template"

}

// GetConfigAsString implements the ConfigManager interface
func (m *MockConfigManager) GetConfigAsString(ctx context.Context) (string, error) {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()
	m.GetConfigAsStringCalled = true

	if m.ConfigDelay > 0 {
		select {
		case <-time.After(m.ConfigDelay):
			// Delay completed
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if m.GetConfigAsStringError != nil {
		return "", m.GetConfigAsStringError
	}

	// If ConfigAsString is set, return it
	if m.ConfigAsString != "" {
		return m.ConfigAsString, nil
	}

	// Otherwise, read the file from the mock filesystem
	data, err := m.MockFileSystem.ReadFile(ctx, DefaultConfigPath)
	return string(data), err
}

// WithConfigAsString configures the mock to return the given string when GetConfigAsString is called
func (m *MockConfigManager) WithConfigAsString(content string) *MockConfigManager {
	m.ConfigAsString = content
	return m
}

// WithGetConfigAsStringError configures the mock to return the given error when GetConfigAsString is called
func (m *MockConfigManager) WithGetConfigAsStringError(err error) *MockConfigManager {
	m.GetConfigAsStringError = err
	return m
}

// GetCacheModTimeWithoutUpdate returns the modification time from the cache without updating it
func (m *MockConfigManager) GetCacheModTimeWithoutUpdate() time.Time {
	return m.CacheModTime
}

// UpdateAndGetCacheModTime updates the cache and returns the modification time
func (m *MockConfigManager) UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error) {
	return m.CacheModTime, nil
}

// WithCacheModTime configures the mock to return the given modification time when GetCacheModTime is called
func (m *MockConfigManager) WithCacheModTime(modTime time.Time) *MockConfigManager {
	m.CacheModTime = modTime
	return m
}

// WriteConfigFromString implements the ConfigManager interface
func (m *MockConfigManager) WriteConfigFromString(ctx context.Context, config string, expectedModTime string) error {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()

	// If expectedModTime is provided, check for concurrent modification
	if expectedModTime != "" {
		expectedTime, err := time.Parse(time.RFC3339, expectedModTime)
		if err != nil {
			return fmt.Errorf("invalid expected modification time format: %w", err)
		}
		if !m.CacheModTime.Equal(expectedTime) {
			return fmt.Errorf("concurrent modification detected: file modified at %v, expected %v",
				m.CacheModTime.Format(time.RFC3339), expectedModTime)
		}
	}

	// First parse the config with strict validation to detect syntax errors and schema problems
	parsedConfig, err := parseConfig([]byte(config), false)
	if err != nil {
		// If strict parsing fails, try again with allowUnknownFields=true
		// This allows YAML anchors and other custom fields
		parsedConfig, err = parseConfig([]byte(config), true)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// write the config
	if err := m.writeConfig(ctx, parsedConfig); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// update the cache mod time
	m.CacheModTime = time.Now()
	m.ConfigAsString = config

	return nil
}
