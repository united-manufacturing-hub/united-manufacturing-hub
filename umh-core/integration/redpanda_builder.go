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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"gopkg.in/yaml.v3"
)

// RedpandaBuilder is used to build configuration with Redpanda services
type RedpandaBuilder struct {
	full config.FullConfig
	// Map to track which services are active by name
	activeRedpanda map[string]bool
	activeBenthos  map[string]bool
}

// NewRedpandaBuilder creates a new builder for Redpanda configurations
func NewRedpandaBuilder() *RedpandaBuilder {
	return &RedpandaBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Internal: config.InternalConfig{
				Services: []config.S6FSMConfig{},
				Redpanda: config.RedpandaConfig{},
			},
		},
		activeRedpanda: make(map[string]bool),
		activeBenthos:  make(map[string]bool),
	}
}

// AddGoldenRedpanda adds a Redpanda service that serves HTTP requests on port 8082
func (r *RedpandaBuilder) AddGoldenRedpanda() *RedpandaBuilder {
	// Create Redpanda config with an HTTP server input
	redpandaConfig := config.RedpandaConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            "golden-redpanda",
			DesiredFSMState: "active",
		},
		RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{},
	}
	redpandaConfig.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs = 0
	redpandaConfig.RedpandaServiceConfig.Topic.DefaultTopicRetentionBytes = 0
	redpandaConfig.RedpandaServiceConfig.Resources.MaxCores = 1
	redpandaConfig.RedpandaServiceConfig.Resources.MemoryPerCoreInBytes = 1024 * 1024 * 1024 * 2 // 2GB

	// Add to configuration
	r.full.Internal.Redpanda = redpandaConfig
	r.activeRedpanda["golden-redpanda"] = true
	return r
}

// StartRedpanda sets a Redpanda service to active state
func (r *RedpandaBuilder) StartRedpanda(name string) *RedpandaBuilder {
	r.full.Internal.Redpanda.DesiredFSMState = "active"
	r.activeRedpanda[name] = true
	return r
}

// StopRedpanda sets a Redpanda service to inactive state
func (r *RedpandaBuilder) StopRedpanda(name string) *RedpandaBuilder {
	r.full.Internal.Redpanda.DesiredFSMState = "stopped"
	r.activeRedpanda[name] = false
	return r
}

// BuildYAML converts the configuration to YAML format
func (r *RedpandaBuilder) BuildYAML() string {
	out, _ := yaml.Marshal(r.full)
	return string(out)
}

// AddBenthosProducer adds a Benthos service that produces messages to a Redpanda topic
func (b *RedpandaBuilder) AddBenthosProducer(name string, productionInterval string, topic string) *RedpandaBuilder {
	// Create Benthos config with a generator input and Kafka output
	benthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		BenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			MetricsPort: 0, // Auto-assign port
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  fmt.Sprintf(`root = "Hello %s"`, name),
					"interval": productionInterval,
					"count":    0, // Unlimited
				},
			},
			Output: map[string]interface{}{
				"kafka": map[string]interface{}{
					"addresses":     []string{"localhost:9092"},
					"topic":         topic,
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
			LogLevel: "INFO",
		},
	}

	// Add to configuration
	b.full.Internal.Benthos = append(b.full.Internal.Benthos, benthosConfig)
	if b.activeBenthos == nil {
		b.activeBenthos = make(map[string]bool)
	}
	b.activeBenthos[name] = true
	return b
}

// StartBenthos sets a Benthos service to active state
func (b *RedpandaBuilder) StartBenthos(name string) *RedpandaBuilder {
	for i, benthos := range b.full.Internal.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			b.full.Internal.Benthos[i].FSMInstanceConfig.DesiredFSMState = "active"
			b.activeBenthos[name] = true
			break
		}
	}
	return b
}

// StopBenthos sets a Benthos service to inactive state
func (b *RedpandaBuilder) StopBenthos(name string) *RedpandaBuilder {
	for i, benthos := range b.full.Internal.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			b.full.Internal.Benthos[i].FSMInstanceConfig.DesiredFSMState = "stopped"
			b.activeBenthos[name] = false
			break
		}
	}
	return b
}

// CountActiveBenthos returns the number of active Benthos services
func (b *RedpandaBuilder) CountActiveBenthos() int {
	count := 0
	for _, active := range b.activeBenthos {
		if active {
			count++
		}
	}
	return count
}

// UpdateBenthosProducer updates the configuration of an existing Benthos producer
func (b *RedpandaBuilder) UpdateBenthosProducer(name string, productionInterval string) *RedpandaBuilder {
	for i, benthos := range b.full.Internal.Benthos {
		if benthos.FSMInstanceConfig.Name == name {
			// Update the interval in the input configuration
			if input, ok := benthos.BenthosServiceConfig.Input["generate"].(map[string]interface{}); ok {
				input["interval"] = productionInterval
				benthos.BenthosServiceConfig.Input["generate"] = input
				b.full.Internal.Benthos[i] = benthos
			}
			break
		}
	}
	return b
}
