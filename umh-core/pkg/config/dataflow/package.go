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

package dataflow

import (
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
)

// DataFlowComponentConfig represents a data flow component configuration.
type DataFlowComponentConfig struct {
	// Name of the component.
	Name string `yaml:"name" json:"name"`

	// DesiredState of the component (e.g., "active", "stopped").
	DesiredState string `yaml:"desiredState" json:"desiredState"`

	// ServiceConfig contains the Benthos service configuration.
	ServiceConfig *benthosserviceconfig.BenthosServiceConfig `yaml:"serviceConfig" json:"serviceConfig"`

	// ConnectionConfig contains the connection configuration.
	// TODO: ADD ME ConnectionConfig *connectionconfig.ConnectionConfig `yaml:"connectionConfig" json:"connectionConfig"`
}

// DataFlowConfig represents the configuration for data flow components
type DataFlowConfig struct {
	// DataFlowComponents is a list of data flow component configurations
	DataFlowComponents []DataFlowComponentConfig `yaml:"dataFlowComponents" json:"dataFlowComponent"`
}

// GetDataFlowComponentByName returns a data flow component configuration by name
func (c *DataFlowConfig) GetDataFlowComponentByName(name string) *DataFlowComponentConfig {
	for i := range c.DataFlowComponents {
		if c.DataFlowComponents[i].Name == name {
			return &c.DataFlowComponents[i]
		}
	}
	return nil
}

// AddDataFlowComponent adds a data flow component configuration
func (c *DataFlowConfig) AddDataFlowComponent(config DataFlowComponentConfig) {
	// Check if component already exists
	for i := range c.DataFlowComponents {
		if c.DataFlowComponents[i].Name == config.Name {
			// Update existing component
			c.DataFlowComponents[i] = config
			return
		}
	}
	// Add new component
	c.DataFlowComponents = append(c.DataFlowComponents, config)
}

// RemoveDataFlowComponent removes a data flow component configuration by name
func (c *DataFlowConfig) RemoveDataFlowComponent(name string) bool {
	for i := range c.DataFlowComponents {
		if c.DataFlowComponents[i].Name == name {
			// Remove component by replacing it with the last element and truncating the slice
			c.DataFlowComponents[i] = c.DataFlowComponents[len(c.DataFlowComponents)-1]
			c.DataFlowComponents = c.DataFlowComponents[:len(c.DataFlowComponents)-1]
			return true
		}
	}
	return false
}
