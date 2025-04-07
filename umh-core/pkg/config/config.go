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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent    AgentConfig    `yaml:"agent"`    // Agent config, requires restart to take effect
	Internal InternalConfig `yaml:"internal"` // Internal config, not to be used by the user, only to be used for testing internal components
}

type InternalConfig struct {
	Services []S6FSMConfig   `yaml:"services"` // Services to manage, can be updated while running
	Benthos  []BenthosConfig `yaml:"benthos"`  // Benthos services to manage, can be updated while running
	Nmap     []NmapConfig    `yaml:"nmap"`     // Nmap services to manage, can be updated while running
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

// DataFlowComponentConfig contains configuration for a DataFlowComponent
type DataFlowComponentConfig struct {
	// Basic component configuration
	Name         string `yaml:"name"`
	DesiredState string `yaml:"desiredState"`

	// Service configuration similar to BenthosServiceConfig
	ServiceConfig benthosserviceconfig.BenthosServiceConfig `yaml:"serviceConfig"`
}

// Clone creates a deep copy of FullConfig
func (c FullConfig) Clone() FullConfig {
	var clone FullConfig
	deepcopy.Copy(&clone.Agent, &c.Agent)
	deepcopy.Copy(&clone.Internal, &c.Internal)
	return clone
}
