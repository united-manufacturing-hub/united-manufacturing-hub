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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent             AgentConfig               `yaml:"agent"`                       // Agent config, requires restart to take effect
	DataFlow          []DataFlowComponentConfig `yaml:"dataFlow,omitempty"`          // DataFlow components to manage, can be updated while running
	ProtocolConverter []ProtocolConverterConfig `yaml:"protocolConverter,omitempty"` // ProtocolConverter config, can be updated while runnnig
	Internal          InternalConfig            `yaml:"internal,omitempty"`          // Internal config, not to be used by the user, only to be used for testing internal components
	Templates         []map[string]interface{}  `yaml:"templates,omitempty"`         // proof of concept for general yaml templates, where anchor can be placed, see also examples/example-config-dataflow-templated.yaml
}

type InternalConfig struct {
	Services        []S6FSMConfig           `yaml:"services,omitempty"`        // Services to manage, can be updated while running
	Benthos         []BenthosConfig         `yaml:"benthos,omitempty"`         // Benthos services to manage, can be updated while running
	Nmap            []NmapConfig            `yaml:"nmap,omitempty"`            // Nmap services to manage, can be updated while running
	Redpanda        RedpandaConfig          `yaml:"redpanda,omitempty"`        // Redpanda config, can be updated while running
	BenthosMonitor  []BenthosMonitorConfig  `yaml:"benthosMonitor,omitempty"`  // BenthosMonitor config, can be updated while running
	Connection      []ConnectionConfig      `yaml:"connection,omitempty"`      // Connection services to manage, can be updated while running
	RedpandaMonitor []RedpandaMonitorConfig `yaml:"redpandaMonitor,omitempty"` // RedpandaMonitor config, can be updated while running
}

type AgentConfig struct {
	MetricsPort        int `yaml:"metricsPort"` // Port to expose metrics on
	CommunicatorConfig `yaml:"communicator,omitempty"`
	ReleaseChannel     ReleaseChannel `yaml:"releaseChannel,omitempty"`
	Location           map[int]string `yaml:"location,omitempty"`
}

type CommunicatorConfig struct {
	APIURL           string `yaml:"apiUrl,omitempty"`
	AuthToken        string `yaml:"authToken,omitempty"`
	AllowInsecureTLS bool   `yaml:"allowInsecureTLS,omitempty"` // Allow TLS connections without verifying the certificate.
}

// FSMInstanceConfig is the config for a FSM instance
type FSMInstanceConfig struct {
	// Name of the service, we use omitempty here, as some services like redpanda will ignore this name, therefore writing the "empty" name back to the config file will cause confusion for the users
	Name            string `yaml:"name,omitempty"`
	DesiredFSMState string `yaml:"desiredState,omitempty"`
}

// ContainerConfig is the config for a container instance
type ContainerConfig struct {
	Name            string `yaml:"name"`
	DesiredFSMState string `yaml:"desiredState"`
}

// AgentMonitorConfig is the config for an agent monitor instance
type AgentMonitorConfig struct {
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

// BenthosMonitorConfig contains configuration for creating a benthos monitor
type BenthosMonitorConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	MetricsPort uint16 `yaml:"metricsPort"` // Port to expose metrics on
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

	DataFlowComponentServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataFlowComponentConfig"`

	// private marker – not (un)marshalled
	// explanation see templating.go
	hasAnchors bool `yaml:"-"`
}

// HasAnchors returns true if the DataFlowComponentConfig has anchors, see templating.go
func (d *DataFlowComponentConfig) HasAnchors() bool { return d.hasAnchors }

// ProtocolConverterConfig contains configuration for creating a ProtocolConverter
type ProtocolConverterConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	ProtocolConverterServiceConfig protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec `yaml:"protocolConverterServiceConfig"`

	// private marker – not (un)marshalled
	// explanation see templating.go
	hasAnchors bool `yaml:"-"`
}

// HasAnchors returns true if the ProtocolConverterConfig has anchors, see templating.go
func (d *ProtocolConverterConfig) HasAnchors() bool { return d.hasAnchors }

// NmapConfig contains configuration for creating a Nmap service
type NmapConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Nmap service
	NmapServiceConfig nmapserviceconfig.NmapServiceConfig `yaml:"nmapServiceConfig"`
}

// RedpandaMonitorConfig contains configuration for creating a Redpanda monitor
type RedpandaMonitorConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`
}

// RedpandaConfig contains configuration for a Redpanda service
type RedpandaConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Redpanda service
	RedpandaServiceConfig redpandaserviceconfig.RedpandaServiceConfig `yaml:"redpandaServiceConfig,omitempty"`
}

// ConnectionConfig contains configuration for creating a Connection service
type ConnectionConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Connection service
	ConnectionServiceConfig connectionserviceconfig.ConnectionServiceConfig `yaml:"connectionServiceConfig"`
}

// TemplateVariable is a variable that can be used in templating
type TemplateVariable struct {
	Name  string `yaml:"name"`
	Value any    `yaml:"value"`
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
	err := deepcopy.Copy(&clone.Agent, &c.Agent)
	if err != nil {
		return FullConfig{}
	}
	err = deepcopy.Copy(&clone.DataFlow, &c.DataFlow)
	if err != nil {
		return FullConfig{}
	}
	err = deepcopy.Copy(&clone.Internal, &c.Internal)
	if err != nil {
		return FullConfig{}
	}
	return clone
}
