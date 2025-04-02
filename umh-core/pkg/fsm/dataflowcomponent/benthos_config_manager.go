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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// BenthosConfigFile represents the structure of the benthos config file
type BenthosConfigFile struct {
	Agent   map[string]interface{} `yaml:"agent,omitempty"`
	Benthos []BenthosEntry         `yaml:"benthos,omitempty"`
}

// BenthosEntry represents an entry in the benthos config file
type BenthosEntry struct {
	Name                 string      `yaml:"name"`
	DesiredState         string      `yaml:"desiredState"`
	BenthosServiceConfig interface{} `yaml:"benthosServiceConfig"`
}

// DefaultBenthosConfigManager implements the BenthosConfigManager interface
type DefaultBenthosConfigManager struct {
	configFilePath string
	mutex          sync.Mutex
	logger         *zap.SugaredLogger
}

// NewBenthosConfigManager creates a new DefaultBenthosConfigManager
func NewBenthosConfigManager(configFilePath string) *DefaultBenthosConfigManager {
	return &DefaultBenthosConfigManager{
		configFilePath: configFilePath,
		logger:         logger.For("BenthosConfigManager"),
	}
}

// readBenthosConfig reads the benthos config file
func (m *DefaultBenthosConfigManager) readBenthosConfig() (*BenthosConfigFile, error) {
	// Ensure config file directory exists
	configDir := filepath.Dir(m.configFilePath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	// Check if config file exists
	if _, err := os.Stat(m.configFilePath); os.IsNotExist(err) {
		// Config file doesn't exist yet, create a new empty one
		return &BenthosConfigFile{
			Agent:   map[string]interface{}{"metricsPort": 8080},
			Benthos: []BenthosEntry{},
		}, nil
	}

	// Read config file
	data, err := ioutil.ReadFile(m.configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", m.configFilePath, err)
	}

	// Parse config file
	var config BenthosConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", m.configFilePath, err)
	}

	return &config, nil
}

// writeBenthosConfig writes the benthos config file
func (m *DefaultBenthosConfigManager) writeBenthosConfig(config *BenthosConfigFile) error {
	// Marshal config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Write config file
	if err := ioutil.WriteFile(m.configFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", m.configFilePath, err)
	}

	return nil
}

// AddComponentToBenthosConfig adds a component to the benthos config
func (m *DefaultBenthosConfigManager) AddComponentToBenthosConfig(component DataFlowComponentConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig()
	if err != nil {
		return err
	}

	// Check if component already exists
	for i, entry := range config.Benthos {
		if entry.Name == component.Name {
			// Component already exists, update it
			config.Benthos[i].DesiredState = component.DesiredState
			config.Benthos[i].BenthosServiceConfig = component.ServiceConfig
			m.logger.Debugf("Component %s already exists in config, updating", component.Name)
			return m.writeBenthosConfig(config)
		}
	}

	// Component doesn't exist, add it
	config.Benthos = append(config.Benthos, BenthosEntry{
		Name:                 component.Name,
		DesiredState:         component.DesiredState,
		BenthosServiceConfig: component.ServiceConfig,
	})

	m.logger.Debugf("Adding component %s to config", component.Name)
	return m.writeBenthosConfig(config)
}

// RemoveComponentFromBenthosConfig removes a component from the benthos config
func (m *DefaultBenthosConfigManager) RemoveComponentFromBenthosConfig(componentName string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig()
	if err != nil {
		return err
	}

	// Find and remove the component
	found := false
	newBenthos := []BenthosEntry{}
	for _, entry := range config.Benthos {
		if entry.Name == componentName {
			found = true
			m.logger.Debugf("Removing component %s from config", componentName)
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
	return m.writeBenthosConfig(config)
}

// UpdateComponentInBenthosConfig updates a component in the benthos config
func (m *DefaultBenthosConfigManager) UpdateComponentInBenthosConfig(component DataFlowComponentConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig()
	if err != nil {
		return err
	}

	// Find and update the component
	found := false
	for i, entry := range config.Benthos {
		if entry.Name == component.Name {
			config.Benthos[i].DesiredState = component.DesiredState
			config.Benthos[i].BenthosServiceConfig = component.ServiceConfig
			found = true
			m.logger.Debugf("Updating component %s in config", component.Name)
			break
		}
	}

	if !found {
		// Component doesn't exist, add it
		return m.AddComponentToBenthosConfig(component)
	}

	return m.writeBenthosConfig(config)
}

// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
func (m *DefaultBenthosConfigManager) ComponentExistsInBenthosConfig(componentName string) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Read config file
	config, err := m.readBenthosConfig()
	if err != nil {
		return false, err
	}

	// Check if component exists
	for _, entry := range config.Benthos {
		if entry.Name == componentName {
			return true, nil
		}
	}

	return false, nil
}
