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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"gopkg.in/yaml.v3"
)

// RedpandaBuilder is used to build configuration with Redpanda services
type RedpandaBuilder struct {
	full config.FullConfig
	// Map to track which services are active by name
	activeRedpanda map[string]bool
}

// NewRedpandaBuilder creates a new builder for Redpanda configurations
func NewRedpandaBuilder() *RedpandaBuilder {
	return &RedpandaBuilder{
		full: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
			},
			Services: []config.S6FSMConfig{},
			Redpanda: config.RedpandaConfig{},
		},
		activeRedpanda: make(map[string]bool),
	}
}

// AddGoldenRedpanda adds a Redpanda service that serves HTTP requests on port 8082
func (b *RedpandaBuilder) AddGoldenRedpanda() *RedpandaBuilder {
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
	redpandaConfig.RedpandaServiceConfig.Resources.MemoryPerCoreInBytes = 1024 * 1024 * 1024 // 1GB

	// Add to configuration
	b.full.Redpanda = redpandaConfig
	b.activeRedpanda["golden-redpanda"] = true
	return b
}

// StartRedpanda sets a Redpanda service to active state
func (b *RedpandaBuilder) StartRedpanda(name string) *RedpandaBuilder {
	b.full.Redpanda.FSMInstanceConfig.DesiredFSMState = "active"
	b.activeRedpanda[name] = true
	return b
}

// StopRedpanda sets a Redpanda service to inactive state
func (b *RedpandaBuilder) StopRedpanda(name string) *RedpandaBuilder {
	b.full.Redpanda.FSMInstanceConfig.DesiredFSMState = "stopped"
	b.activeRedpanda[name] = false
	return b
}

// BuildYAML converts the configuration to YAML format
func (b *RedpandaBuilder) BuildYAML() string {
	out, _ := yaml.Marshal(b.full)
	return string(out)
}
