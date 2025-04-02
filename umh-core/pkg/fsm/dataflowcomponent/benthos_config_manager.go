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

package dataflowcomponent

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// BenthosConfigFile represents the structure of the benthos config file
type BenthosConfigFile struct {
	Agent   map[string]interface{} `yaml:"agent,omitempty"`
	Benthos []BenthosEntry         `yaml:"benthos,omitempty"`
}

// BenthosEntry represents an entry in the benthos config file
type BenthosEntry struct {
	Name                 string                                    `yaml:"name"`
	DesiredState         string                                    `yaml:"desiredState"`
	BenthosServiceConfig benthosserviceconfig.BenthosServiceConfig `yaml:"benthosServiceConfig"`
}

// DefaultBenthosConfigManager implements the BenthosConfigManager interface
type DefaultBenthosConfigManager struct {
	configFilePath string
	mutex          sync.Mutex
	logger         *zap.SugaredLogger
	fs             filesystem.Service
}

// NewBenthosConfigManager creates a new DefaultBenthosConfigManager
func NewBenthosConfigManager(configFilePath string) *DefaultBenthosConfigManager {
	logger := logger.For("BenthosConfigManager")
	logger.Debugf("Creating new BenthosConfigManager with config file path: %s", configFilePath)
	return &DefaultBenthosConfigManager{
		configFilePath: configFilePath,
		logger:         logger,
		fs:             filesystem.NewDefaultService(),
	}
}

// readBenthosConfig reads the benthos config file
func (m *DefaultBenthosConfigManager) readBenthosConfig(ctx context.Context) (*BenthosConfigFile, error) {
	m.logger.Debugf("Reading Benthos config from: %s", m.configFilePath)

	// Ensure config file directory exists
	configDir := filepath.Dir(m.configFilePath)
	m.logger.Debugf("Ensuring config directory exists: %s", configDir)
	if err := m.fs.EnsureDirectory(ctx, configDir); err != nil {
		m.logger.Errorf("Failed to create config directory %s: %v", configDir, err)
		return nil, fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	// Check if config file exists
	exists, err := m.fs.FileExists(ctx, m.configFilePath)
	if err != nil {
		m.logger.Errorf("Failed to check if config file exists: %v", err)
		return nil, fmt.Errorf("failed to check if config file exists: %w", err)
	}

	if !exists {
		// Config file doesn't exist yet, create a new empty one
		m.logger.Infof("Config file %s doesn't exist, creating new empty config", m.configFilePath)
		return &BenthosConfigFile{
			Agent:   map[string]interface{}{"metricsPort": 8080},
			Benthos: []BenthosEntry{},
		}, nil
	}

	// Read config file
	m.logger.Debugf("Reading config file: %s", m.configFilePath)
	data, err := m.fs.ReadFile(ctx, m.configFilePath)
	if err != nil {
		m.logger.Errorf("Failed to read config file %s: %v", m.configFilePath, err)
		return nil, fmt.Errorf("failed to read config file %s: %w", m.configFilePath, err)
	}
	m.logger.Debugf("Read %d bytes from config file", len(data))

	// Parse config file
	var config BenthosConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		m.logger.Errorf("Failed to parse config file %s: %v", m.configFilePath, err)
		return nil, fmt.Errorf("failed to parse config file %s: %w", m.configFilePath, err)
	}

	m.logger.Debugf("Successfully parsed config file with %d Benthos entries", len(config.Benthos))
	return &config, nil
}

// writeBenthosConfig writes the benthos config file
func (m *DefaultBenthosConfigManager) writeBenthosConfig(ctx context.Context, config *BenthosConfigFile) error {
	m.logger.Debugf("Writing Benthos config to: %s with %d entries", m.configFilePath, len(config.Benthos))

	// Marshal config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		m.logger.Errorf("Failed to marshal config to YAML: %v", err)
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}
	m.logger.Debugf("Marshaled config to %d bytes of YAML data", len(data))

	// Write config file
	m.logger.Debugf("Writing data to file: %s", m.configFilePath)
	if err := m.fs.WriteFile(ctx, m.configFilePath, data, 0644); err != nil {
		m.logger.Errorf("Failed to write config file %s: %v", m.configFilePath, err)
		return fmt.Errorf("failed to write config file %s: %w", m.configFilePath, err)
	}

	m.logger.Infof("Successfully wrote Benthos config to %s", m.configFilePath)
	return nil
}

// AddComponentToBenthosConfig adds a component to the benthos config
func (m *DefaultBenthosConfigManager) AddComponentToBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	m.logger.Debugf("Adding component %s to Benthos config", component.Name)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig(ctx)
	if err != nil {
		m.logger.Errorf("Failed to read Benthos config: %v", err)
		return err
	}

	// Check if component already exists
	for i, entry := range config.Benthos {
		if entry.Name == component.Name {
			// Component already exists, update it
			m.logger.Debugf("Component %s already exists in config, updating", component.Name)
			config.Benthos[i].DesiredState = component.DesiredState
			config.Benthos[i].BenthosServiceConfig = component.ServiceConfig
			m.logger.Infof("Updated component %s in Benthos config", component.Name)
			return m.writeBenthosConfig(ctx, config)
		}
	}

	// Component doesn't exist, add it
	m.logger.Debugf("Component %s doesn't exist, adding as new entry", component.Name)
	config.Benthos = append(config.Benthos, BenthosEntry{
		Name:                 component.Name,
		DesiredState:         component.DesiredState,
		BenthosServiceConfig: component.ServiceConfig,
	})

	m.logger.Infof("Added component %s to Benthos config", component.Name)
	return m.writeBenthosConfig(ctx, config)
}

// RemoveComponentFromBenthosConfig removes a component from the benthos config
func (m *DefaultBenthosConfigManager) RemoveComponentFromBenthosConfig(ctx context.Context, componentName string) error {
	m.logger.Debugf("Removing component %s from Benthos config", componentName)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig(ctx)
	if err != nil {
		m.logger.Errorf("Failed to read Benthos config: %v", err)
		return err
	}

	// Find and remove the component
	found := false
	newBenthos := []BenthosEntry{}
	for _, entry := range config.Benthos {
		if entry.Name == componentName {
			found = true
			m.logger.Debugf("Found component %s in config, removing", componentName)
		} else {
			newBenthos = append(newBenthos, entry)
		}
	}

	if !found {
		m.logger.Debugf("Component %s not found in config, nothing to remove", componentName)
		return nil
	}

	// Update config
	config.Benthos = newBenthos
	m.logger.Infof("Removed component %s from Benthos config", componentName)
	return m.writeBenthosConfig(ctx, config)
}

// UpdateComponentInBenthosConfig updates a component in the benthos config
func (m *DefaultBenthosConfigManager) UpdateComponentInBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	m.logger.Debugf("Updating component %s in Benthos config", component.Name)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig(ctx)
	if err != nil {
		m.logger.Errorf("Failed to read Benthos config: %v", err)
		return err
	}

	// Find and update the component
	found := false
	for i, entry := range config.Benthos {
		if entry.Name == component.Name {
			config.Benthos[i].DesiredState = component.DesiredState
			config.Benthos[i].BenthosServiceConfig = component.ServiceConfig
			found = true
			m.logger.Debugf("Found component %s in config, updating", component.Name)
			break
		}
	}

	if !found {
		// Component doesn't exist, add it
		m.logger.Debugf("Component %s not found for update, adding instead", component.Name)
		return m.AddComponentToBenthosConfig(ctx, component)
	}

	m.logger.Infof("Updated component %s in Benthos config", component.Name)
	return m.writeBenthosConfig(ctx, config)
}

// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
func (m *DefaultBenthosConfigManager) ComponentExistsInBenthosConfig(ctx context.Context, componentName string) (bool, error) {
	m.logger.Debugf("Checking if component %s exists in Benthos config", componentName)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig(ctx)
	if err != nil {
		m.logger.Errorf("Failed to read Benthos config: %v", err)
		return false, err
	}

	// Check if component exists
	for _, entry := range config.Benthos {
		if entry.Name == componentName {
			m.logger.Debugf("Component %s found in Benthos config", componentName)
			return true, nil
		}
	}

	m.logger.Debugf("Component %s not found in Benthos config", componentName)
	return false, nil
}
