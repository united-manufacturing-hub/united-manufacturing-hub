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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent    AgentConfig               `yaml:"agent"`              // Agent config, requires restart to take effect
	DataFlow []DataFlowComponentConfig `yaml:"dataFlow,omitempty"` // DataFlow components to manage, can be updated while running
	Internal InternalConfig            `yaml:"internal,omitempty"` // Internal config, not to be used by the user, only to be used for testing internal components
}

type InternalConfig struct {
	Services []S6FSMConfig   `yaml:"services,omitempty"` // Services to manage, can be updated while running
	Benthos  []BenthosConfig `yaml:"benthos,omitempty"`  // Benthos services to manage, can be updated while running
	Nmap     []NmapConfig    `yaml:"nmap,omitempty"`     // Nmap services to manage, can be updated while running
	Redpanda RedpandaConfig  `yaml:"redpanda,omitempty"` // Redpanda config, can be updated while running
}

type AgentConfig struct {
	MetricsPort        int `yaml:"metricsPort"` // Port to expose metrics on
	CommunicatorConfig `yaml:"communicator,omitempty"`
	ReleaseChannel     ReleaseChannel `yaml:"releaseChannel,omitempty"`
	Location           map[int]string `yaml:"location,omitempty"`
}

type CommunicatorConfig struct {
	APIURL    string `yaml:"apiUrl,omitempty"`
	AuthToken string `yaml:"authToken,omitempty"`
}

// FSMInstanceConfig is the config for a FSM instance
type FSMInstanceConfig struct {
	Name            string `yaml:"name"`
	DesiredFSMState string `yaml:"desiredState"`
}

// ContainerConfig is the config for a container instance
type ContainerConfig struct {
	Name            string `yaml:"name"`
	DesiredFSMState string `yaml:"desiredState"`
}

type ReleaseChannel string

const (
	ReleaseChannelNightly    ReleaseChannel = "nightly"
	ReleaseChannelStable     ReleaseChannel = "stable"
	ReleaseChannelEnterprise ReleaseChannel = "enterprise"
)

// S6FSMConfig contains configuration for creating a service
type S6FSMConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	S6ServiceConfig s6serviceconfig.S6ServiceConfig `yaml:"s6ServiceConfig"`
}

// BenthosConfig contains configuration for creating a Benthos service
type BenthosConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Benthos service
	BenthosServiceConfig benthosserviceconfig.BenthosServiceConfig `yaml:"benthosServiceConfig"`
}

// DataFlowComponentConfig contains configuration for creating a DataFlowComponent
type DataFlowComponentConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the DataFlowComponent
	DataFlowComponentConfig dataflowcomponentconfig.DataFlowComponentConfig `yaml:"dataFlowComponentConfig"`
}

// NmapConfig contains configuration for creating a Nmap service
type NmapConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Nmap service
	NmapServiceConfig NmapServiceConfig `yaml:"nmapServiceConfig"`
}

// NmapServiceConfig represents the configuration for a Nmap service
type NmapServiceConfig struct {
	// Target to scan (hostname or IP)
	Target string `yaml:"target"`
	// Port to scan (single port number)
	Port int `yaml:"port"`
}

// Equal checks if two NmapServiceConfigs are equal
func (c NmapServiceConfig) Equal(other NmapServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}

// RedpandaConfig contains configuration for a Redpanda service
type RedpandaConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Redpanda service
	RedpandaServiceConfig redpandaserviceconfig.RedpandaServiceConfig `yaml:"redpandaServiceConfig"`
}

// Clone creates a deep copy of FullConfig
func (c FullConfig) Clone() FullConfig {
	clone := FullConfig{
		Agent:    c.Agent,
		DataFlow: make([]DataFlowComponentConfig, len(c.DataFlow)),
		Internal: InternalConfig{},
	}
	// deep copy the location map if it exists
	if c.Agent.Location != nil {
		clone.Agent.Location = make(map[int]string)
		for k, v := range c.Agent.Location {
			clone.Agent.Location[k] = v
		}
	}
	deepcopy.Copy(&clone.Agent, &c.Agent)
	deepcopy.Copy(&clone.DataFlow, &c.DataFlow)
	deepcopy.Copy(&clone.Internal, &c.Internal)
	return clone
}
