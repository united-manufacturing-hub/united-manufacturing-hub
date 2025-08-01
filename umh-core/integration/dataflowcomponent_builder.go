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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"gopkg.in/yaml.v3"
)

// DataFlowComponentBuilder is used to build configuration with DataFlowComponent services
type DataFlowComponentBuilder struct {
	// Map to track which components are active by name
	activeComponents map[string]bool
	full             config.FullConfig
}

// NewDataFlowComponentBuilder creates a new builder for DataFlowComponent configurations
func NewDataFlowComponentBuilder() *DataFlowComponentBuilder {
	return &DataFlowComponentBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			DataFlow: []config.DataFlowComponentConfig{},
			Internal: config.InternalConfig{
				Redpanda: config.RedpandaConfig{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            "redpanda",
						DesiredFSMState: "stopped",
					},
				},
				TopicBrowser: config.TopicBrowserConfig{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            "topic-browser",
						DesiredFSMState: "stopped",
					},
				},
			},
		},
		activeComponents: make(map[string]bool),
	}
}

// AddGoldenDataFlowComponent adds a DataFlowComponent service that serves HTTP requests on port 8083
func (b *DataFlowComponentBuilder) AddGoldenDataFlowComponent() *DataFlowComponentBuilder {
	// Create DataFlowComponent config with an HTTP server input
	dataFlowConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-dataflow",
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"http_server": map[string]interface{}{
						"path":    "/",
						"address": "0.0.0.0:8082",
					},
				},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"bloblang": "root = content()",
						},
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			},
		},
	}

	// Add to configuration
	b.full.DataFlow = append(b.full.DataFlow, dataFlowConfig)
	b.activeComponents["golden-dataflow"] = true
	return b
}

// AddGeneratorDataFlowComponent adds a DataFlowComponent that generates messages and sends to stdout
func (b *DataFlowComponentBuilder) AddGeneratorDataFlowComponent(name string, interval string) *DataFlowComponentBuilder {
	// Create DataFlowComponent config with a generator input
	dataFlowConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"generate": map[string]interface{}{
						"mapping":  `root = "hello world from DFC!"`,
						"interval": interval,
						"count":    0, // Unlimited
					},
				},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"bloblang": "root = content()",
						},
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			},
		},
	}

	// Add to configuration
	b.full.DataFlow = append(b.full.DataFlow, dataFlowConfig)
	b.activeComponents[name] = true
	return b
}

// AddGeneratorDataFlowComponentToKafka adds a DataFlowComponent that generates messages and sends to a Kafka topic
func (b *DataFlowComponentBuilder) AddGeneratorDataFlowComponentToKafka(name string, interval string, topic string, key *string) *DataFlowComponentBuilder {
	dataFlowConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"generate": map[string]interface{}{
						"mapping":  fmt.Sprintf(`root = "hello world from DFC! %s"`, name),
						"interval": interval,
						"count":    0, // Unlimited
					},
				},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"bloblang": "root = content()",
						},
					},
				},
				Output: map[string]interface{}{
					"kafka": map[string]interface{}{
						"addresses":     []string{"localhost:9092"},
						"topic":         topic,
						"key":           key,
						"max_in_flight": 1000,
						"client_id":     name,
						"ack_replicas":  true,
						"compression":   "snappy",
						"metadata": map[string]interface{}{
							"exclude_prefixes": []string{"_"},
						},
						"batching": map[string]interface{}{
							"count":  100,
							"period": "1s",
						},
					},
				},
			},
		},
	}
	b.full.DataFlow = append(b.full.DataFlow, dataFlowConfig)
	b.activeComponents[name] = true
	return b
}

// UpdateGeneratorDataFlowComponent updates the interval of an existing generator DataFlowComponent
func (b *DataFlowComponentBuilder) UpdateGeneratorDataFlowComponent(name string, newInterval string) *DataFlowComponentBuilder {
	// Find and update the service
	for i, component := range b.full.DataFlow {
		if component.Name == name {
			// Create updated input config with new interval
			if input, ok := component.DataFlowComponentServiceConfig.BenthosConfig.Input["generate"].(map[string]interface{}); ok {
				input["interval"] = newInterval
				b.full.DataFlow[i].DataFlowComponentServiceConfig.BenthosConfig.Input["generate"] = input
			}
			break
		}
	}
	return b
}

// StartDataFlowComponent sets a DataFlowComponent to active state
func (b *DataFlowComponentBuilder) StartDataFlowComponent(name string) *DataFlowComponentBuilder {
	for i, component := range b.full.DataFlow {
		if component.Name == name {
			b.full.DataFlow[i].DesiredFSMState = "active"
			b.activeComponents[name] = true
			break
		}
	}
	return b
}

// StopDataFlowComponent sets a DataFlowComponent to stopped state
func (b *DataFlowComponentBuilder) StopDataFlowComponent(name string) *DataFlowComponentBuilder {
	for i, component := range b.full.DataFlow {
		if component.Name == name {
			b.full.DataFlow[i].DesiredFSMState = "stopped"
			b.activeComponents[name] = false
			break
		}
	}
	return b
}

// CountActiveDataFlowComponents returns the number of active DataFlowComponent services
func (b *DataFlowComponentBuilder) CountActiveDataFlowComponents() int {
	count := 0
	for _, isActive := range b.activeComponents {
		if isActive {
			count++
		}
	}
	return count
}

// BuildYAML converts the configuration to YAML format
func (b *DataFlowComponentBuilder) BuildYAML() string {
	out, err := yaml.Marshal(b.full)
	if err != nil {
		panic(fmt.Errorf("failed to marshal DataFlowComponent config: %w", err))
	}
	return string(out)
}
