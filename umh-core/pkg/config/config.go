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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Agent             AgentConfig               `yaml:"agent"`                       // Agent config, requires restart to take effect
	Templates         TemplatesConfig           `yaml:"templates,omitempty"`         // Templates section with enforced structure for protocol converters
	DataModels        []DataModelsConfig        `yaml:"dataModels,omitempty"`        // DataModels section with enforced structure for data models
	DataFlow          []DataFlowComponentConfig `yaml:"dataFlow,omitempty"`          // DataFlow components to manage, can be updated while running
	ProtocolConverter []ProtocolConverterConfig `yaml:"protocolConverter,omitempty"` // ProtocolConverter config, can be updated while runnnig
	Internal          InternalConfig            `yaml:"internal,omitempty"`          // Internal config, not to be used by the user, only to be used for testing internal components
}

// TemplatesConfig defines the structure for the templates section
type TemplatesConfig struct {
	ProtocolConverter map[string]interface{} `yaml:"protocolConverter,omitempty"` // Array of protocol converter templates
}

type DataModelsConfig struct {
	Name        string           `yaml:"name"`                  // name of the data model
	Version     string           `yaml:"version"`               // version of the data model (v1, v2, etc.)
	Description string           `yaml:"description,omitempty"` // description of the data model
	Structure   map[string]Field `yaml:"structure"`             // structure of the data model (fields)
}

type Field struct {
	Type   string `yaml:"type,omitempty"`   // type of the field
	_Model string `yaml:"_model,omitempty"` // this is a special field that is used to reference another data model to be used as a type for this field
}

type InternalConfig struct {
	Services        []S6FSMConfig           `yaml:"services,omitempty"`        // Services to manage, can be updated while running
	Benthos         []BenthosConfig         `yaml:"benthos,omitempty"`         // Benthos services to manage, can be updated while running
	Nmap            []NmapConfig            `yaml:"nmap,omitempty"`            // Nmap services to manage, can be updated while running
	Redpanda        RedpandaConfig          `yaml:"redpanda,omitempty"`        // Redpanda config, can be updated while running
	BenthosMonitor  []BenthosMonitorConfig  `yaml:"benthosMonitor,omitempty"`  // BenthosMonitor config, can be updated while running
	Connection      []ConnectionConfig      `yaml:"connection,omitempty"`      // Connection services to manage, can be updated while running
	RedpandaMonitor []RedpandaMonitorConfig `yaml:"redpandaMonitor,omitempty"` // RedpandaMonitor config, can be updated while running
	TopicBrowser    TopicBrowserConfig      `yaml:"topicbrowser,omitempty"`
}

type AgentConfig struct {
	MetricsPort        int `yaml:"metricsPort"` // Port to expose metrics on
	CommunicatorConfig `yaml:"communicator,omitempty"`
	GraphQLConfig      GraphQLConfig  `yaml:"graphql,omitempty"` // GraphQL server configuration
	ReleaseChannel     ReleaseChannel `yaml:"releaseChannel,omitempty"`
	Location           map[int]string `yaml:"location,omitempty"`
	Simulator          bool           `yaml:"simulator,omitempty"`
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
	hasAnchors bool   `yaml:"-"`
	anchorName string `yaml:"-"`
}

// HasAnchors returns true if the ProtocolConverterConfig has anchors, see templating.go
func (d *ProtocolConverterConfig) HasAnchors() bool { return d.hasAnchors }

// AnchorName returns the anchor name of the ProtocolConverterConfig, see templating.go
func (d *ProtocolConverterConfig) AnchorName() string { return d.anchorName }

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

type GraphQLConfig struct {
	Enabled      bool     `yaml:"enabled"`                // Enable/disable GraphQL server
	Port         int      `yaml:"port"`                   // Port to expose GraphQL on (default: 8090)
	CORSOrigins  []string `yaml:"corsOrigins,omitempty"`  // CORS allowed origins (default: ["*"])
	Debug        bool     `yaml:"debug,omitempty"`        // Enable GraphiQL playground and debug logging
	AuthRequired bool     `yaml:"authRequired,omitempty"` // Require authentication (future use)
}

// TopicBrowserConfig contains configuration for creating a Topic Browser service
type TopicBrowserConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Connection service
	TopicBrowserServiceConfig topicbrowserserviceconfig.Config `yaml:"serviceConfig,omitempty"`
}

// Clone creates a deep copy of FullConfig
func (c FullConfig) Clone() FullConfig {
	clone := FullConfig{
		Agent:             c.Agent,
		DataFlow:          make([]DataFlowComponentConfig, len(c.DataFlow)),
		ProtocolConverter: make([]ProtocolConverterConfig, len(c.ProtocolConverter)),
		Templates:         TemplatesConfig{},
		Internal:          InternalConfig{},
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
	err = deepcopy.Copy(&clone.ProtocolConverter, &c.ProtocolConverter)
	if err != nil {
		return FullConfig{}
	}
	err = deepcopy.Copy(&clone.Templates, &c.Templates)
	if err != nil {
		return FullConfig{}
	}
	err = deepcopy.Copy(&clone.Internal, &c.Internal)
	if err != nil {
		return FullConfig{}
	}
	return clone
}
