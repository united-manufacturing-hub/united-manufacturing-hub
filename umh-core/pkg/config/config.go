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
	"reflect"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent    AgentConfig     `yaml:"agent"`    // Agent config, requires restart to take effect
	Services []S6FSMConfig   `yaml:"services"` // Services to manage, can be updated while running
	Benthos  []BenthosConfig `yaml:"benthos"`  // Benthos services to manage, can be updated while running
	Redpanda RedpandaConfig  `yaml:"redpanda"` // Redpanda config, can be updated while running
}

type AgentConfig struct {
	MetricsPort int `yaml:"metricsPort"` // Port to expose metrics on
}

// FSMInstanceConfig is the config for a FSM instance
type FSMInstanceConfig struct {
	Name            string `yaml:"name"`
	DesiredFSMState string `yaml:"desiredState"`
}

// S6FSMConfig contains configuration for creating a service
type S6FSMConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the S6 service
	S6ServiceConfig S6ServiceConfig `yaml:"s6ServiceConfig"`
}

// S6ServiceConfig contains configuration for creating a service
type S6ServiceConfig struct {
	Command     []string          `yaml:"command"`
	Env         map[string]string `yaml:"env"`
	ConfigFiles map[string]string `yaml:"configFiles"`
	MemoryLimit int64             `yaml:"memoryLimit"` // 0 means no memory limit, see also https://skarnet.org/software/s6/s6-softlimit.html
}

// Equal checks if two S6ServiceConfigs are equal
func (c S6ServiceConfig) Equal(other S6ServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}

// BenthosConfig contains configuration for creating a Benthos service
type BenthosConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Benthos service
	BenthosServiceConfig BenthosServiceConfig `yaml:"benthosServiceConfig"`
}

// BenthosServiceConfig represents the configuration for a Benthos service
type BenthosServiceConfig struct {
	// Benthos-specific configuration
	Input              map[string]interface{}   `yaml:"input"`
	Pipeline           map[string]interface{}   `yaml:"pipeline"`
	Output             map[string]interface{}   `yaml:"output"`
	CacheResources     []map[string]interface{} `yaml:"cache_resources"`
	RateLimitResources []map[string]interface{} `yaml:"rate_limit_resources"`
	Buffer             map[string]interface{}   `yaml:"buffer"`

	// Advanced configuration
	MetricsPort int    `yaml:"metrics_port"`
	LogLevel    string `yaml:"log_level"`
}

// Equal checks if two BenthosServiceConfigs are equal
func (c BenthosServiceConfig) Equal(other BenthosServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}

// RedpandaConfig contains configuration for a Redpanda service
type RedpandaConfig struct {
	RedpandaServiceConfig RedpandaServiceConfig `yaml:"redpandaServiceConfig"`
}

type RedpandaServiceConfig struct {
	Command    string `yaml:"command"`
	ConfigPath string `yaml:"config_path"`
}

// Equal checks if two RedpandaServiceConfigs are equal
func (c RedpandaServiceConfig) Equal(other RedpandaServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}

// Clone creates a deep copy of FullConfig
func (c FullConfig) Clone() FullConfig {
	clone := FullConfig{
		Agent:    c.Agent,
		Services: make([]S6FSMConfig, len(c.Services)),
		Benthos:  make([]BenthosConfig, len(c.Benthos)),
	}
	deepcopy.Copy(&clone.Services, &c.Services)
	deepcopy.Copy(&clone.Benthos, &c.Benthos)
	return clone
}
