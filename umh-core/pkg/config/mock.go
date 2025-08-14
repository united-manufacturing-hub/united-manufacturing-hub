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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// MockConfigManager is a mock implementation of ConfigManager for testing.
type MockConfigManager struct {
	CacheModTime                        time.Time
	ConfigError                         error
	AddDataflowcomponentError           error
	DeleteDataflowcomponentError        error
	EditDataflowcomponentError          error
	AtomicAddProtocolConverterError     error
	AtomicEditProtocolConverterError    error
	AtomicDeleteProtocolConverterError  error
	AtomicAddStreamProcessorError       error
	AtomicEditStreamProcessorError      error
	AtomicDeleteStreamProcessorError    error
	AtomicAddDataModelError             error
	AtomicEditDataModelError            error
	AtomicDeleteDataModelError          error
	AtomicAddDataContractError          error
	GetConfigAsStringError              error
	MockFileSystem                      *filesystem.MockFileSystem
	logger                              *zap.SugaredLogger
	ConfigAsString                      string
	Config                              FullConfig
	ConfigDelay                         time.Duration
	mutexReadOrWrite                    sync.Mutex
	mutexReadAndWrite                   sync.Mutex
	GetConfigCalled                     int32 // Use atomic int32 for thread safety (0=false, 1=true)
	AddDataflowcomponentCalled          bool
	DeleteDataflowcomponentCalled       bool
	EditDataflowcomponentCalled         bool
	AtomicAddProtocolConverterCalled    bool
	AtomicEditProtocolConverterCalled   bool
	AtomicDeleteProtocolConverterCalled bool
	AtomicAddStreamProcessorCalled      bool
	AtomicEditStreamProcessorCalled     bool
	AtomicDeleteStreamProcessorCalled   bool
	AtomicAddDataModelCalled            bool
	AtomicEditDataModelCalled           bool
	AtomicDeleteDataModelCalled         bool
	AtomicAddDataContractCalled         bool
	GetConfigAsStringCalled             bool
}

// NewMockConfigManager creates a new MockConfigManager instance.
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		MockFileSystem: filesystem.NewMockFileSystem(),
		logger:         logger.For(logger.ComponentConfigManager),
	}
}

// GetDataFlowConfig returns the DataFlow component configurations.
func (m *MockConfigManager) GetDataFlowConfig() []DataFlowComponentConfig {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	return m.Config.DataFlow
}

// getConfigInternal returns the config without locking - for use by atomic methods that already hold the lock.
func (m *MockConfigManager) getConfigInternal(ctx context.Context) (FullConfig, error) {
	atomic.StoreInt32(&m.GetConfigCalled, 1)

	if m.ConfigDelay > 0 {
		select {
		case <-time.After(m.ConfigDelay):
			// Delay completed
		case <-ctx.Done():
			return FullConfig{}, ctx.Err()
		}
	}

	// Return a deep copy of the config to avoid data races
	// The atomic operations should work on a copy and only update the original via writeConfig
	configCopy := m.Config
	configCopy.DataFlow = make([]DataFlowComponentConfig, len(m.Config.DataFlow))
	copy(configCopy.DataFlow, m.Config.DataFlow)

	return configCopy, m.ConfigError
}

// GetConfig implements the ConfigManager interface.
func (m *MockConfigManager) GetConfig(ctx context.Context, tick uint64) (FullConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	return m.getConfigInternal(ctx)
}

// GetFileSystemService returns the mock filesystem service.
func (m *MockConfigManager) GetFileSystemService() filesystem.Service {
	return m.MockFileSystem
}

// WriteConfig implements the ConfigManager interface
// all the functions that call MockConfigManager.writeConfig must hold the mutexReadAndWrite mutex.
func (m *MockConfigManager) writeConfig(ctx context.Context, cfg FullConfig) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Create the directory if it doesn't exist (using mock filesystem)
	dir := filepath.Dir(DefaultConfigPath)

	err := m.MockFileSystem.EnsureDirectory(ctx, dir)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Convert spec to YAML using the same logic as the real implementation
	yamlConfig, err := convertSpecToYaml(cfg, ctx)
	if err != nil {
		return fmt.Errorf("failed to convert spec to yaml: %w", err)
	}

	// Marshal the config to YAML
	data, err := yaml.Marshal(yamlConfig) //nolint:musttag // yamlConfig is dynamically generated interface{}
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the file via mock filesystem (give everybody read & write access)
	configPath := DefaultConfigPath

	err = m.MockFileSystem.WriteFile(ctx, configPath, data, 0666)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update the cache to reflect the new config (simulate file stat)
	m.Config = cfg
	m.CacheModTime = time.Now()
	m.ConfigAsString = string(data) // Update raw config cache too

	return nil
}

// WithConfig configures the mock to return the given config.
func (m *MockConfigManager) WithConfig(cfg FullConfig) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.Config = cfg

	return m
}

// WithConfigError configures the mock to return the given error.
func (m *MockConfigManager) WithConfigError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.ConfigError = err

	return m
}

// WithConfigDelay configures the mock to delay for the given duration.
func (m *MockConfigManager) WithConfigDelay(delay time.Duration) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.ConfigDelay = delay

	return m
}

// WithAddDataflowcomponentError configures the mock to return the given error when AtomicAddDataflowcomponent is called.
func (m *MockConfigManager) WithAddDataflowcomponentError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AddDataflowcomponentError = err

	return m
}

// WithDeleteDataflowcomponentError configures the mock to return the given error when AtomicDeleteDataflowcomponent is called.
func (m *MockConfigManager) WithDeleteDataflowcomponentError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.DeleteDataflowcomponentError = err

	return m
}

// WithEditDataflowcomponentError configures the mock to return the given error when AtomicEditDataflowcomponent is called.
func (m *MockConfigManager) WithEditDataflowcomponentError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.EditDataflowcomponentError = err

	return m
}

// WithAtomicAddProtocolConverterError configures the mock to return the given error when AtomicAddProtocolConverter is called.
func (m *MockConfigManager) WithAtomicAddProtocolConverterError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddProtocolConverterError = err

	return m
}

// WithAtomicEditProtocolConverterError configures the mock to return the given error when AtomicEditProtocolConverter is called.
func (m *MockConfigManager) WithAtomicEditProtocolConverterError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditProtocolConverterError = err

	return m
}

// WithAtomicDeleteProtocolConverterError configures the mock to return the given error when AtomicDeleteProtocolConverter is called.
func (m *MockConfigManager) WithAtomicDeleteProtocolConverterError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteProtocolConverterError = err

	return m
}

// WithAtomicAddStreamProcessorError configures the mock to return the given error when AtomicAddStreamProcessor is called.
func (m *MockConfigManager) WithAtomicAddStreamProcessorError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddStreamProcessorError = err

	return m
}

// WithAtomicEditStreamProcessorError configures the mock to return the given error when AtomicEditStreamProcessor is called.
func (m *MockConfigManager) WithAtomicEditStreamProcessorError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditStreamProcessorError = err

	return m
}

// WithAtomicDeleteStreamProcessorError configures the mock to return the given error when AtomicDeleteStreamProcessor is called.
func (m *MockConfigManager) WithAtomicDeleteStreamProcessorError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteStreamProcessorError = err

	return m
}

// WithAtomicAddDataModelError configures the mock to return the given error when AtomicAddDataModel is called.
func (m *MockConfigManager) WithAtomicAddDataModelError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddDataModelError = err

	return m
}

// WithAtomicEditDataModelError configures the mock to return the given error when AtomicEditDataModel is called.
func (m *MockConfigManager) WithAtomicEditDataModelError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditDataModelError = err

	return m
}

// WithAtomicDeleteDataModelError configures the mock to return the given error when AtomicDeleteDataModel is called.
func (m *MockConfigManager) WithAtomicDeleteDataModelError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteDataModelError = err

	return m
}

// WithAtomicAddDataContractError configures the mock to return the given error when AtomicAddDataContract is called.
func (m *MockConfigManager) WithAtomicAddDataContractError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddDataContractError = err

	return m
}

// ResetCalls clears the called flags for testing multiple calls.
func (m *MockConfigManager) ResetCalls() {
	m.mutexReadOrWrite.Lock()
	defer m.mutexReadOrWrite.Unlock()

	atomic.StoreInt32(&m.GetConfigCalled, 0)
	m.AddDataflowcomponentCalled = false
	m.DeleteDataflowcomponentCalled = false
	m.EditDataflowcomponentCalled = false
	m.AtomicAddProtocolConverterCalled = false
	m.AtomicEditProtocolConverterCalled = false
	m.AtomicDeleteProtocolConverterCalled = false
	m.AtomicAddStreamProcessorCalled = false
	m.AtomicEditStreamProcessorCalled = false
	m.AtomicDeleteStreamProcessorCalled = false
	m.AtomicAddDataModelCalled = false
	m.AtomicEditDataModelCalled = false
	m.AtomicDeleteDataModelCalled = false
	m.AtomicAddDataContractCalled = false
}

// atomic set location.
func (m *MockConfigManager) AtomicSetLocation(ctx context.Context, location models.EditInstanceLocationModel) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	// get the current config
	config, err := m.getConfigInternal(ctx)
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

	// Convert the agent location to string map for use in other components
	agentLocationStr := make(map[string]string)
	for k, v := range config.Agent.Location {
		agentLocationStr[strconv.Itoa(k)] = v
	}

	// Update all ProtocolConverter locations to match the agent location
	for i := range config.ProtocolConverter {
		if config.ProtocolConverter[i].ProtocolConverterServiceConfig.Location == nil {
			config.ProtocolConverter[i].ProtocolConverterServiceConfig.Location = make(map[string]string)
		}

		// Update each level in the protocol converter location with the agent location
		// Only update levels that exist in the agent location
		for levelStr, value := range agentLocationStr {
			config.ProtocolConverter[i].ProtocolConverterServiceConfig.Location[levelStr] = value
		}
	}

	// Update all StreamProcessor locations to match the agent location
	for i := range config.StreamProcessor {
		if config.StreamProcessor[i].StreamProcessorServiceConfig.Location == nil {
			config.StreamProcessor[i].StreamProcessorServiceConfig.Location = make(map[string]string)
		}

		// Update each level in the stream processor location with the agent location
		// Only update levels that exist in the agent location
		for levelStr, value := range agentLocationStr {
			config.StreamProcessor[i].StreamProcessorServiceConfig.Location[levelStr] = value
		}
	}

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// atomic add dataflowcomponent.
func (m *MockConfigManager) AtomicAddDataflowcomponent(ctx context.Context, dfc DataFlowComponentConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AddDataflowcomponentCalled = true

	if m.AddDataflowcomponentError != nil {
		return m.AddDataflowcomponentError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// edit the config
	config.DataFlow = append(config.DataFlow, dfc)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicDeleteDataflowcomponent implements the ConfigManager interface.
func (m *MockConfigManager) AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.DeleteDataflowcomponentCalled = true

	if m.DeleteDataflowcomponentError != nil {
		return m.DeleteDataflowcomponentError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditDataflowcomponent implements the ConfigManager interface.
func (m *MockConfigManager) AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc DataFlowComponentConfig) (DataFlowComponentConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.EditDataflowcomponentCalled = true

	if m.EditDataflowcomponentError != nil {
		return DataFlowComponentConfig{}, m.EditDataflowcomponentError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
	err = m.writeConfig(ctx, config)
	if err != nil {
		return DataFlowComponentConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicAddProtocolConverter implements the ConfigManager interface.
func (m *MockConfigManager) AtomicAddProtocolConverter(ctx context.Context, protocolConverterConfig ProtocolConverterConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddProtocolConverterCalled = true

	if m.AtomicAddProtocolConverterError != nil {
		return m.AtomicAddProtocolConverterError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == protocolConverterConfig.Name {
			return fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", protocolConverterConfig.Name)
		}
	}

	// If it's a child (TemplateRef is non-empty and != Name), verify that a root with that TemplateRef exists
	if protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef != "" && protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef != protocolConverterConfig.Name {
		templateRef := protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef
		rootExists := false

		// Scan existing protocol converters to find a root with matching name
		for _, existing := range config.ProtocolConverter {
			if existing.Name == templateRef && existing.ProtocolConverterServiceConfig.TemplateRef == existing.Name {
				rootExists = true

				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, protocolConverterConfig.Name)
		}
	}

	// Add the protocol converter - let convertSpecToYAML handle template generation
	config.ProtocolConverter = append(config.ProtocolConverter, protocolConverterConfig)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditProtocolConverter implements the ConfigManager interface.
func (m *MockConfigManager) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, protocolConverterConfig ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditProtocolConverterCalled = true

	if m.AtomicEditProtocolConverterError != nil {
		return ProtocolConverterConfig{}, m.AtomicEditProtocolConverterError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
		if i != targetIndex && cmp.Name == protocolConverterConfig.Name {
			return ProtocolConverterConfig{}, fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", protocolConverterConfig.Name)
		}
	}

	newIsRoot := protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef != "" &&
		protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef == protocolConverterConfig.Name
	oldIsRoot := oldConfig.ProtocolConverterServiceConfig.TemplateRef != "" &&
		oldConfig.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name

	// Handle root rename - propagate to children
	if oldIsRoot && newIsRoot && oldConfig.Name != protocolConverterConfig.Name {
		// Update all children that reference the old root name
		for i, inst := range config.ProtocolConverter {
			if i != targetIndex && inst.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name {
				inst.ProtocolConverterServiceConfig.TemplateRef = protocolConverterConfig.Name
				config.ProtocolConverter[i] = inst
			}
		}
	}

	// If it's a child (TemplateRef is non-empty and not a root), validate that the template reference exists
	if !newIsRoot && protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef != "" {
		templateRef := protocolConverterConfig.ProtocolConverterServiceConfig.TemplateRef
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
		if protocolConverterConfig.Name == templateRef && newIsRoot {
			rootExists = true
		}

		if !rootExists {
			return ProtocolConverterConfig{}, fmt.Errorf("template %q not found for child %s", templateRef, protocolConverterConfig.Name)
		}
	}

	// Commit the edit
	config.ProtocolConverter[targetIndex] = protocolConverterConfig

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicDeleteProtocolConverter implements the ConfigManager interface.
func (m *MockConfigManager) AtomicDeleteProtocolConverter(ctx context.Context, componentUUID uuid.UUID) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteProtocolConverterCalled = true

	if m.AtomicDeleteProtocolConverterError != nil {
		return m.AtomicDeleteProtocolConverterError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find the component to delete
	var targetComponent *ProtocolConverterConfig

	targetIndex := -1

	for i, component := range config.ProtocolConverter {
		componentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if componentID == componentUUID {
			targetComponent = &component
			targetIndex = i

			break
		}
	}

	if targetComponent == nil {
		return fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// Check if this is a root component (TemplateRef == Name) that has dependent children
	isRoot := targetComponent.ProtocolConverterServiceConfig.TemplateRef == targetComponent.Name
	if isRoot {
		// Check for dependent children
		for _, component := range config.ProtocolConverter {
			if component.ProtocolConverterServiceConfig.TemplateRef == targetComponent.Name && component.Name != targetComponent.Name {
				return fmt.Errorf("cannot delete template %q: it has dependent child instances", targetComponent.Name)
			}
		}
	}

	// Remove the component (no cascading deletion)
	config.ProtocolConverter = append(config.ProtocolConverter[:targetIndex], config.ProtocolConverter[targetIndex+1:]...)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddStreamProcessor implements the ConfigManager interface.
func (m *MockConfigManager) AtomicAddStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddStreamProcessorCalled = true

	if m.AtomicAddStreamProcessorError != nil {
		return m.AtomicAddStreamProcessorError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, cmp := range config.StreamProcessor {
		if cmp.Name == streamProcessorConfig.Name {
			return fmt.Errorf("another stream processor with name %q already exists – choose a unique name", streamProcessorConfig.Name)
		}
	}

	// If it's a child (TemplateRef is non-empty and != Name), verify that a root with that TemplateRef exists
	if streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != "" && streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != streamProcessorConfig.Name {
		templateRef := streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef
		rootExists := false

		// Scan existing stream processors to find a root with matching name
		for _, existing := range config.StreamProcessor {
			if existing.Name == templateRef && existing.StreamProcessorServiceConfig.TemplateRef == existing.Name {
				rootExists = true

				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, streamProcessorConfig.Name)
		}
	}

	// Add the stream processor - let convertSpecToYAML handle template generation
	config.StreamProcessor = append(config.StreamProcessor, streamProcessorConfig)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditStreamProcessor implements the ConfigManager interface.
func (m *MockConfigManager) AtomicEditStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) (StreamProcessorConfig, error) {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditStreamProcessorCalled = true

	if m.AtomicEditStreamProcessorError != nil {
		return StreamProcessorConfig{}, m.AtomicEditStreamProcessorError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return StreamProcessorConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// Find target index by name
	targetIndex := -1

	var oldConfig StreamProcessorConfig

	for i, component := range config.StreamProcessor {
		if component.Name == streamProcessorConfig.Name {
			targetIndex = i
			oldConfig = config.StreamProcessor[i]

			break
		}
	}

	if targetIndex == -1 {
		return StreamProcessorConfig{}, fmt.Errorf("stream processor with name %s not found", streamProcessorConfig.Name)
	}

	newIsRoot := streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != "" &&
		streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef == streamProcessorConfig.Name
	oldIsRoot := oldConfig.StreamProcessorServiceConfig.TemplateRef != "" &&
		oldConfig.StreamProcessorServiceConfig.TemplateRef == oldConfig.Name

	// Handle root rename - propagate to children
	if oldIsRoot && newIsRoot && oldConfig.Name != streamProcessorConfig.Name {
		// Update all children that reference the old root name
		for i, inst := range config.StreamProcessor {
			if i != targetIndex && inst.StreamProcessorServiceConfig.TemplateRef == oldConfig.Name {
				inst.StreamProcessorServiceConfig.TemplateRef = streamProcessorConfig.Name
				config.StreamProcessor[i] = inst
			}
		}
	}

	// If it's a child (TemplateRef is non-empty and not a root), validate that the template reference exists
	if !newIsRoot && streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != "" {
		templateRef := streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef
		rootExists := false

		// Scan existing stream processors to find a root with matching name
		// Note: we check the updated slice which may include renamed roots
		for i, inst := range config.StreamProcessor {
			// Skip the instance being edited since it's not committed yet
			if i == targetIndex {
				continue
			}

			if inst.Name == templateRef && inst.StreamProcessorServiceConfig.TemplateRef == inst.Name {
				rootExists = true

				break
			}
		}

		// Also check if the new instance itself becomes the root for this template
		if streamProcessorConfig.Name == templateRef && newIsRoot {
			rootExists = true
		}

		if !rootExists {
			return StreamProcessorConfig{}, fmt.Errorf("template %q not found for child %s", templateRef, streamProcessorConfig.Name)
		}
	}

	// Commit the edit
	config.StreamProcessor[targetIndex] = streamProcessorConfig

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return StreamProcessorConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicDeleteStreamProcessor implements the ConfigManager interface.
func (m *MockConfigManager) AtomicDeleteStreamProcessor(ctx context.Context, name string) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteStreamProcessorCalled = true

	if m.AtomicDeleteStreamProcessorError != nil {
		return m.AtomicDeleteStreamProcessorError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find the component to delete
	var targetComponent *StreamProcessorConfig

	targetIndex := -1

	for i, component := range config.StreamProcessor {
		if component.Name == name {
			targetComponent = &component
			targetIndex = i

			break
		}
	}

	if targetComponent == nil {
		return fmt.Errorf("stream processor with name %s not found", name)
	}

	// Check if this is a root component (TemplateRef == Name) that has dependent children
	isRoot := targetComponent.StreamProcessorServiceConfig.TemplateRef == targetComponent.Name
	if isRoot {
		// Check for dependent children
		for _, component := range config.StreamProcessor {
			if component.StreamProcessorServiceConfig.TemplateRef == targetComponent.Name && component.Name != targetComponent.Name {
				return fmt.Errorf("cannot delete template %q: it has dependent child instances", targetComponent.Name)
			}
		}
	}

	// Remove the component (no cascading deletion)
	config.StreamProcessor = append(config.StreamProcessor[:targetIndex], config.StreamProcessor[targetIndex+1:]...)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddDataModel implements the ConfigManager interface.
func (m *MockConfigManager) AtomicAddDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddDataModelCalled = true

	if m.AtomicAddDataModelError != nil {
		return m.AtomicAddDataModelError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
		Name:        name,
		Description: description,
		Versions: map[string]DataModelVersion{
			"v1": dmVersion,
		},
	})

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditDataModel implements the ConfigManager interface.
func (m *MockConfigManager) AtomicEditDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicEditDataModelCalled = true

	if m.AtomicEditDataModelError != nil {
		return m.AtomicEditDataModelError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
	var maxVersion = 0

	for versionKey := range currentDataModel.Versions {
		if strings.HasPrefix(versionKey, "v") {
			versionNum, err := strconv.Atoi(versionKey[1:])
			if err == nil {
				if versionNum > maxVersion {
					maxVersion = versionNum
				}
			}
		}
	}

	// append the new version to the data model
	nextVersion := maxVersion + 1
	currentDataModel.Versions[fmt.Sprintf("v%d", nextVersion)] = dmVersion

	// update the description
	currentDataModel.Description = description

	// edit the data model in the config
	config.DataModels[targetIndex] = currentDataModel

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicDeleteDataModel implements the ConfigManager interface.
func (m *MockConfigManager) AtomicDeleteDataModel(ctx context.Context, name string) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicDeleteDataModelCalled = true

	if m.AtomicDeleteDataModelError != nil {
		return m.AtomicDeleteDataModelError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
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
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddDataContract implements the ConfigManager interface.
func (m *MockConfigManager) AtomicAddDataContract(ctx context.Context, dataContract DataContractsConfig) error {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.AtomicAddDataContractCalled = true

	if m.AtomicAddDataContractError != nil {
		return m.AtomicAddDataContractError
	}

	// get the current config
	config, err := m.getConfigInternal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, dcc := range config.DataContracts {
		if dcc.Name == dataContract.Name {
			return fmt.Errorf("another data contract with name %q already exists – choose a unique name", dataContract.Name)
		}
	}

	// add the data contract to the config
	config.DataContracts = append(config.DataContracts, dataContract)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetConfigAsString implements the ConfigManager interface.
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

// WithConfigAsString configures the mock to return the given string when GetConfigAsString is called.
func (m *MockConfigManager) WithConfigAsString(content string) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.ConfigAsString = content

	return m
}

// WithGetConfigAsStringError configures the mock to return the given error when GetConfigAsString is called.
func (m *MockConfigManager) WithGetConfigAsStringError(err error) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.GetConfigAsStringError = err

	return m
}

// GetCacheModTimeWithoutUpdate returns the modification time from the cache without updating it.
func (m *MockConfigManager) GetCacheModTimeWithoutUpdate() time.Time {
	return m.CacheModTime
}

// UpdateAndGetCacheModTime updates the cache and returns the modification time.
func (m *MockConfigManager) UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error) {
	return m.CacheModTime, nil
}

// WithCacheModTime configures the mock to return the given modification time when GetCacheModTime is called.
func (m *MockConfigManager) WithCacheModTime(modTime time.Time) *MockConfigManager {
	m.mutexReadAndWrite.Lock()
	defer m.mutexReadAndWrite.Unlock()

	m.CacheModTime = modTime

	return m
}

// WriteYAMLConfigFromString implements the ConfigManager interface.
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
	parsedConfig, err := ParseConfig([]byte(config), ctx, false)
	if err != nil {
		// If strict parsing fails, try again with allowUnknownFields=true
		// This allows YAML anchors and other custom fields
		parsedConfig, err = ParseConfig([]byte(config), ctx, true)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// write the config
	err = m.writeConfig(ctx, parsedConfig)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// update the cache mod time
	m.CacheModTime = time.Now()
	m.ConfigAsString = config

	return nil
}

// IsGetConfigCalled returns true if GetConfig has been called (thread-safe).
func (m *MockConfigManager) IsGetConfigCalled() bool {
	return atomic.LoadInt32(&m.GetConfigCalled) != 0
}
