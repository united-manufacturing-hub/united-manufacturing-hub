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
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// MockConfigManager is a mock implementation of ConfigManager for testing
type MockConfigManager struct {
	GetConfigCalled                     bool
	AddDataflowcomponentCalled          bool
	DeleteDataflowcomponentCalled       bool
	EditDataflowcomponentCalled         bool
	AtomicAddProtocolConverterCalled    bool
	AtomicEditProtocolConverterCalled   bool
	AtomicDeleteProtocolConverterCalled bool
	AtomicAddDataModelCalled            bool
	AtomicEditDataModelCalled           bool
	AtomicDeleteDataModelCalled         bool
	Config                              FullConfig
	ConfigError                         error
	AddDataflowcomponentError           error
	DeleteDataflowcomponentError        error
	EditDataflowcomponentError          error
	AtomicAddProtocolConverterError     error
	AtomicEditProtocolConverterError    error
	AtomicDeleteProtocolConverterError  error
	AtomicAddDataModelError             error
	AtomicEditDataModelError            error
	AtomicDeleteDataModelError          error
	ConfigAsString                      string
	GetConfigAsStringError              error
	GetConfigAsStringCalled             bool
	ConfigDelay                         time.Duration
	mutexReadOrWrite                    sync.Mutex
	mutexReadAndWrite                   sync.Mutex
	MockFileSystem                      *filesystem.MockFileSystem
	CacheModTime                        time.Time
	logger                              *zap.SugaredLogger
}

// NewMockConfigManager creates a new MockConfigManager instance
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		MockFileSystem: filesystem.NewMockFileSystem(),
		logger:         logger.For(logger.ComponentConfigManager),
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
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Create the directory if it doesn't exist (using mock filesystem)
	dir := filepath.Dir(DefaultConfigPath)
	if err := m.MockFileSystem.EnsureDirectory(ctx, dir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Convert spec to YAML using the same logic as the real implementation
	yamlConfig, err := convertSpecToYaml(cfg)
	if err != nil {
		return fmt.Errorf("failed to convert spec to yaml: %w", err)
	}

	// Marshal the config to YAML
	data, err := yaml.Marshal(yamlConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the file via mock filesystem (give everybody read & write access)
	configPath := DefaultConfigPath
	if err := m.MockFileSystem.WriteFile(ctx, configPath, data, 0666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update the cache to reflect the new config (simulate file stat)
	m.Config = cfg
	m.CacheModTime = time.Now()
	m.ConfigAsString = string(data) // Update raw config cache too

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

// WithAtomicDeleteProtocolConverterError configures the mock to return the given error when AtomicDeleteProtocolConverter is called
func (m *MockConfigManager) WithAtomicDeleteProtocolConverterError(err error) *MockConfigManager {
	m.AtomicDeleteProtocolConverterError = err
	return m
}

// WithAtomicAddDataModelError configures the mock to return the given error when AtomicAddDataModel is called
func (m *MockConfigManager) WithAtomicAddDataModelError(err error) *MockConfigManager {
	m.AtomicAddDataModelError = err
	return m
}

// WithAtomicEditDataModelError configures the mock to return the given error when AtomicEditDataModel is called
func (m *MockConfigManager) WithAtomicEditDataModelError(err error) *MockConfigManager {
	m.AtomicEditDataModelError = err
	return m
}

// WithAtomicDeleteDataModelError configures the mock to return the given error when AtomicDeleteDataModel is called
func (m *MockConfigManager) WithAtomicDeleteDataModelError(err error) *MockConfigManager {
	m.AtomicDeleteDataModelError = err
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
	m.AtomicDeleteProtocolConverterCalled = false
	m.AtomicAddDataModelCalled = false
	m.AtomicEditDataModelCalled = false
	m.AtomicDeleteDataModelCalled = false
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

	// check for duplicate name before add
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == pc.Name {
			return fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// If it's a child (TemplateRef is non-empty and != Name), verify that a root with that TemplateRef exists
	if pc.ProtocolConverterServiceConfig.TemplateRef != "" && pc.ProtocolConverterServiceConfig.TemplateRef != pc.Name {
		templateRef := pc.ProtocolConverterServiceConfig.TemplateRef
		rootExists := false

		// Scan existing protocol converters to find a root with matching name
		for _, existing := range config.ProtocolConverter {
			if existing.Name == templateRef && existing.ProtocolConverterServiceConfig.TemplateRef == existing.Name {
				rootExists = true
				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, pc.Name)
		}
	}

	// Add the protocol converter - let convertSpecToYAML handle template generation
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

	// Find target index via GenerateUUIDFromName(Name) == componentUUID
	targetIndex := -1
	var oldConfig ProtocolConverterConfig
	for i, component := range config.ProtocolConverter {
		curComponentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if curComponentID == componentUUID {
			targetIndex = i
			oldConfig = config.ProtocolConverter[i]
			break
		}
	}

	if targetIndex == -1 {
		return ProtocolConverterConfig{}, fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// Duplicate-name check (exclude the edited one)
	for i, cmp := range config.ProtocolConverter {
		if i != targetIndex && cmp.Name == pc.Name {
			return ProtocolConverterConfig{}, fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	newIsRoot := pc.ProtocolConverterServiceConfig.TemplateRef != "" &&
		pc.ProtocolConverterServiceConfig.TemplateRef == pc.Name
	oldIsRoot := oldConfig.ProtocolConverterServiceConfig.TemplateRef != "" &&
		oldConfig.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name

	// Handle root rename - propagate to children
	if oldIsRoot && newIsRoot && oldConfig.Name != pc.Name {
		// Update all children that reference the old root name
		for i, inst := range config.ProtocolConverter {
			if i != targetIndex && inst.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name {
				inst.ProtocolConverterServiceConfig.TemplateRef = pc.Name
				config.ProtocolConverter[i] = inst
			}
		}
	}

	// If it's a child (TemplateRef is non-empty and not a root), validate that the template reference exists
	if !newIsRoot && pc.ProtocolConverterServiceConfig.TemplateRef != "" {
		templateRef := pc.ProtocolConverterServiceConfig.TemplateRef
		rootExists := false

		// Scan existing protocol converters to find a root with matching name
		// Note: we check the updated slice which may include renamed roots
		for i, inst := range config.ProtocolConverter {
			// Skip the instance being edited since it's not committed yet
			if i == targetIndex {
				continue
			}
			if inst.Name == templateRef && inst.ProtocolConverterServiceConfig.TemplateRef == inst.Name {
				rootExists = true
				break
			}
		}

		// Also check if the new instance itself becomes the root for this template
		if pc.Name == templateRef && newIsRoot {
			rootExists = true
		}

		if !rootExists {
			return ProtocolConverterConfig{}, fmt.Errorf("template %q not found for child %s", templateRef, pc.Name)
		}
	}

	// Commit the edit
	config.ProtocolConverter[targetIndex] = pc

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicDeleteProtocolConverter implements the ConfigManager interface
func (m *MockConfigManager) AtomicDeleteProtocolConverter(ctx context.Context, componentUUID uuid.UUID) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteProtocolConverterCalled = true

	if m.AtomicDeleteProtocolConverterError != nil {
		return m.AtomicDeleteProtocolConverterError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find the target protocol converter by UUID
	targetIndex := -1
	var targetConverter ProtocolConverterConfig
	for i, converter := range config.ProtocolConverter {
		converterID := dataflowcomponentserviceconfig.GenerateUUIDFromName(converter.Name)
		if converterID == componentUUID {
			targetIndex = i
			targetConverter = converter
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// Determine if target is a root
	isRoot := targetConverter.ProtocolConverterServiceConfig.TemplateRef != "" &&
		targetConverter.ProtocolConverterServiceConfig.TemplateRef == targetConverter.Name

	// If it's a root, check for dependent children
	if isRoot {
		childCount := 0
		for i, converter := range config.ProtocolConverter {
			// Skip the target itself
			if i == targetIndex {
				continue
			}
			// Count children that reference this root
			if converter.ProtocolConverterServiceConfig.TemplateRef == targetConverter.Name {
				childCount++
			}
		}

		if childCount > 0 {
			return fmt.Errorf("cannot delete root %q; %d dependent converters exist", targetConverter.Name, childCount)
		}
	}

	// Build new slice omitting the target
	filteredConverters := make([]ProtocolConverterConfig, 0, len(config.ProtocolConverter)-1)
	for i, converter := range config.ProtocolConverter {
		if i != targetIndex {
			filteredConverters = append(filteredConverters, converter)
		}
	}

	// Update config with filtered converters
	config.ProtocolConverter = filteredConverters

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddDataModel implements the ConfigManager interface
func (m *MockConfigManager) AtomicAddDataModel(ctx context.Context, name string, dmVersion DataModelVersion) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddDataModelCalled = true

	if m.AtomicAddDataModelError != nil {
		return m.AtomicAddDataModelError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, dmc := range config.DataModels {
		if dmc.Name == name {
			return fmt.Errorf("another data model with name %q already exists – choose a unique name", name)
		}
	}

	// add the data model to the config
	config.DataModels = append(config.DataModels, DataModelsConfig{
		Name: name,
		Versions: map[uint64]DataModelVersion{
			1: dmVersion,
		},
	})

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditDataModel implements the ConfigManager interface
func (m *MockConfigManager) AtomicEditDataModel(ctx context.Context, name string, dmVersion DataModelVersion) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditDataModelCalled = true

	if m.AtomicEditDataModelError != nil {
		return m.AtomicEditDataModelError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	targetIndex := -1
	// find the data model to edit
	for i, dmc := range config.DataModels {
		if dmc.Name == name {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// get the current data model
	currentDataModel := config.DataModels[targetIndex]

	// Find the highest version number to ensure we don't overwrite existing versions
	var maxVersion uint64 = 0
	for versionKey := range currentDataModel.Versions {
		if versionKey > maxVersion {
			maxVersion = versionKey
		}
	}

	// append the new version to the data model
	nextVersion := maxVersion + 1
	currentDataModel.Versions[nextVersion] = dmVersion

	// edit the data model in the config
	config.DataModels[targetIndex] = currentDataModel

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicDeleteDataModel implements the ConfigManager interface
func (m *MockConfigManager) AtomicDeleteDataModel(ctx context.Context, name string) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteDataModelCalled = true

	if m.AtomicDeleteDataModelError != nil {
		return m.AtomicDeleteDataModelError
	}

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// find the data model to delete
	targetIndex := -1
	for i, dmc := range config.DataModels {
		if dmc.Name == name {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// delete the data model from the config
	config.DataModels = append(config.DataModels[:targetIndex], config.DataModels[targetIndex+1:]...)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
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

// WriteYAMLConfigFromString implements the ConfigManager interface
func (m *MockConfigManager) WriteYAMLConfigFromString(ctx context.Context, config string, expectedModTime string) error {
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
	parsedConfig, err := ParseConfig([]byte(config), false)
	if err != nil {
		// If strict parsing fails, try again with allowUnknownFields=true
		// This allows YAML anchors and other custom fields
		parsedConfig, err = ParseConfig([]byte(config), true)
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
