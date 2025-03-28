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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent    AgentConfig     `yaml:"agent"`    // Agent config, requires restart to take effect
	Services []S6FSMConfig   `yaml:"services"` // Services to manage, can be updated while running
	Benthos  []BenthosConfig `yaml:"benthos"`  // Benthos services to manage, can be updated while running
	Nmap     []NmapConfig    `yaml:"nmap"`     // Nmap services to manage, can be updated while running
}

type AgentConfig struct {
	MetricsPort        int `yaml:"metricsPort"` // Port to expose metrics on
	CommunicatorConfig `yaml:"communicator"`
	ReleaseChannel     ReleaseChannel `yaml:"releaseChannel"`
	Location           map[int]string `yaml:"location"`
}

type CommunicatorConfig struct {
	APIURL    string `yaml:"apiUrl"`
	AuthToken string `yaml:"authToken"`
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

// Clone creates a deep copy of FullConfig
func (c FullConfig) Clone() FullConfig {
	clone := FullConfig{
		Agent:    c.Agent,
		Services: make([]S6FSMConfig, len(c.Services)),
		Benthos:  make([]BenthosConfig, len(c.Benthos)),
		Nmap:     make([]NmapConfig, len(c.Nmap)),
	}
	deepcopy.Copy(&clone.Services, &c.Services)
	deepcopy.Copy(&clone.Benthos, &c.Benthos)
	deepcopy.Copy(&clone.Nmap, &c.Nmap)
	return clone
}
