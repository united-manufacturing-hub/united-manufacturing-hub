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

package integration_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"gopkg.in/yaml.v3"
)

// DataFlowComponentBuilder is used to build configuration with DataFlowComponent entries
type DataFlowComponentBuilder struct {
	full                config.FullConfig
	activeDataFlowComps map[string]bool
}

// NewDataFlowComponentBuilder creates a new builder for DataFlowComponent configurations
func NewDataFlowComponentBuilder() *DataFlowComponentBuilder {
	return &DataFlowComponentBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Services:           []config.S6FSMConfig{},
			Benthos:            []config.BenthosConfig{},
			DataFlowComponents: []config.DataFlowComponentConfig{},
			Redpanda: config.RedpandaConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            "redpanda",
					DesiredFSMState: "stopped",
				},
				RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{
					Topic: struct {
						DefaultTopicRetentionMs    int `yaml:"defaultTopicRetentionMs"`
						DefaultTopicRetentionBytes int `yaml:"defaultTopicRetentionBytes"`
					}{
						DefaultTopicRetentionMs:    604800000,
						DefaultTopicRetentionBytes: 0,
					},
					Resources: struct {
						MaxCores             int `yaml:"maxCores"`
						MemoryPerCoreInBytes int `yaml:"memoryPerCoreInBytes"`
					}{
						MaxCores:             1,
						MemoryPerCoreInBytes: 2147483648,
					},
				},
			},
		},
		activeDataFlowComps: make(map[string]bool),
	}
}

// AddGoldenDataFlowComponent adds a basic hello-world DataFlowComponent
func (b *DataFlowComponentBuilder) AddGoldenDataFlowComponent() *DataFlowComponentBuilder {
	// Create DataFlowComponent config
	dataFlowComp := config.DataFlowComponentConfig{
		Name:         "golden-data-flow",
		DesiredState: "active",
		ServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  `root = "hello world from golden-data-flow!"`,
					"interval": "1s",
					"count":    0, // Unlimited
				},
			},
			Output: map[string]interface{}{
				"stdout": map[string]interface{}{},
			},
			LogLevel: "DEBUG",
		},
	}

	// Add to configuration
	b.full.DataFlowComponents = append(b.full.DataFlowComponents, dataFlowComp)
	b.activeDataFlowComps["golden-data-flow"] = true
	return b
}

// AddDataFlowComponent adds a custom DataFlowComponent
func (b *DataFlowComponentBuilder) AddDataFlowComponent(name string, interval string) *DataFlowComponentBuilder {
	// Create DataFlowComponent config
	dataFlowComp := config.DataFlowComponentConfig{
		Name:         name,
		DesiredState: "active",
		ServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  `root = "message from ` + name + `"`,
					"interval": interval,
					"count":    0, // Unlimited
				},
			},
			Output: map[string]interface{}{
				"stdout": map[string]interface{}{},
			},
			LogLevel: "INFO",
		},
	}

	// Add to configuration
	b.full.DataFlowComponents = append(b.full.DataFlowComponents, dataFlowComp)
	b.activeDataFlowComps[name] = true
	return b
}

// UpdateDataFlowComponent updates the interval of an existing DataFlowComponent
func (b *DataFlowComponentBuilder) UpdateDataFlowComponent(name string, newInterval string) *DataFlowComponentBuilder {
	// Find and update the component
	for i, comp := range b.full.DataFlowComponents {
		if comp.Name == name {
			// Update interval in the service config
			if input, ok := comp.ServiceConfig.Input["generate"].(map[string]interface{}); ok {
				input["interval"] = newInterval
				b.full.DataFlowComponents[i].ServiceConfig.Input["generate"] = input
			}
			break
		}
	}
	return b
}

// StartDataFlowComponent sets a DataFlowComponent to active state
func (b *DataFlowComponentBuilder) StartDataFlowComponent(name string) *DataFlowComponentBuilder {
	for i, comp := range b.full.DataFlowComponents {
		if comp.Name == name {
			b.full.DataFlowComponents[i].DesiredState = "active"
			b.activeDataFlowComps[name] = true
			break
		}
	}
	return b
}

// StopDataFlowComponent sets a DataFlowComponent to stopped state
func (b *DataFlowComponentBuilder) StopDataFlowComponent(name string) *DataFlowComponentBuilder {
	for i, comp := range b.full.DataFlowComponents {
		if comp.Name == name {
			b.full.DataFlowComponents[i].DesiredState = "stopped"
			b.activeDataFlowComps[name] = false
			break
		}
	}
	return b
}

// CountActiveDataFlowComponents returns the number of active DataFlowComponents
func (b *DataFlowComponentBuilder) CountActiveDataFlowComponents() int {
	count := 0
	for _, isActive := range b.activeDataFlowComps {
		if isActive {
			count++
		}
	}
	return count
}

// EnableRedpanda enables the Redpanda service in the configuration
func (b *DataFlowComponentBuilder) EnableRedpanda() *DataFlowComponentBuilder {
	b.full.Redpanda.DesiredFSMState = "active"
	return b
}

// AddBenthosService adds a Benthos service to verify co-existence with DataFlowComponents
func (b *DataFlowComponentBuilder) AddBenthosService(name string) *DataFlowComponentBuilder {
	// Create Benthos config
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		BenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  `root = "message from Benthos service ` + name + `"`,
					"interval": "2s",
					"count":    0, // Unlimited
				},
			},
			Output: map[string]interface{}{
				"stdout": map[string]interface{}{},
			},
			LogLevel: "INFO",
		},
	}

	// Add to configuration
	b.full.Benthos = append(b.full.Benthos, benthosConfig)
	return b
}

// BuildYAML converts the configuration to YAML format
func (b *DataFlowComponentBuilder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}
