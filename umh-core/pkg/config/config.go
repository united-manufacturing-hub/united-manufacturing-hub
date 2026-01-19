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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"

	"github.com/tiendc/go-deepcopy"
)

type FullConfig struct {
	Templates         TemplatesConfig           `yaml:"templates,omitempty"`         // Templates section with enforced structure for protocol converter
	PayloadShapes     map[string]PayloadShape   `yaml:"payloadShapes,omitempty"`     // PayloadShapes section with enforced structure for payload shapes
	Internal          InternalConfig            `yaml:"internal,omitempty"`          // Internal config, not to be used by the user, only to be used for testing internal components
	DataModels        []DataModelsConfig        `yaml:"dataModels,omitempty"`        // DataModels section with enforced structure for data models
	DataContracts     []DataContractsConfig     `yaml:"dataContracts,omitempty"`     // DataContracts section with enforced structure for data contracts
	DataFlow          []DataFlowComponentConfig `yaml:"dataFlow,omitempty"`          // DataFlow components to manage, can be updated while running
	ProtocolConverter []ProtocolConverterConfig `yaml:"protocolConverter,omitempty"` // ProtocolConverter config, can be updated while runnnig
	StreamProcessor   []StreamProcessorConfig   `yaml:"streamProcessor,omitempty"`   // StreamProcessor config, can be updated while running
	Agent             AgentConfig               `yaml:"agent"`                       // Agent config, requires restart to take effect
}

// TemplatesConfig defines the structure for the templates section.
type TemplatesConfig struct {
	ProtocolConverter map[string]interface{} `yaml:"protocolConverter,omitempty"` // Array of protocol converter templates
	StreamProcessor   map[string]interface{} `yaml:"streamProcessor,omitempty"`   // Array of stream processor templates
}

// DataModelsConfig defines the structure for the data models section.
type DataModelsConfig struct {
	Versions    map[string]DataModelVersion `yaml:"version"`               // version of the data model (1, 2, etc.)
	Name        string                      `yaml:"name"`                  // name of the data model
	Description string                      `yaml:"description,omitempty"` // description of the data model
}

// DataContractsConfig defines the structure for the data contracts section.
type DataContractsConfig struct {
	Name           string                   `yaml:"name"`                      // name of the data contract
	Model          *ModelRef                `yaml:"model,omitempty"`           // reference to the data model
	DefaultBridges []map[string]interface{} `yaml:"default_bridges,omitempty"` // placeholder for default bridges configuration
}

type DataModelVersion struct {
	Structure map[string]Field `yaml:"structure"` // structure of the data model (fields)
}

// ModelRef represents a reference to another data model.
type ModelRef struct {
	Name    string `yaml:"name"`    // name of the referenced data model
	Version string `yaml:"version"` // version of the referenced data model
}

type Field struct {
	ModelRef     *ModelRef        `yaml:"_refModel,omitempty"`     // this is a special field that is used to reference another data model to be used as a type for this field
	Subfields    map[string]Field `yaml:",inline"`                 // subfields of the field (allow recursive definition of fields)
	PayloadShape string           `yaml:"_payloadshape,omitempty"` // type of the field (timeseries only for now)
}

type InternalConfig struct {
	Services        []S6FSMConfig           `yaml:"services,omitempty"`        // Services to manage, can be updated while running
	Benthos         []BenthosConfig         `yaml:"benthos,omitempty"`         // Benthos services to manage, can be updated while running
	Nmap            []NmapConfig            `yaml:"nmap,omitempty"`            // Nmap services to manage, can be updated while running
	BenthosMonitor  []BenthosMonitorConfig  `yaml:"benthosMonitor,omitempty"`  // BenthosMonitor config, can be updated while running
	Connection      []ConnectionConfig      `yaml:"connection,omitempty"`      // Connection services to manage, can be updated while running
	RedpandaMonitor []RedpandaMonitorConfig `yaml:"redpandaMonitor,omitempty"` // RedpandaMonitor config, can be updated while running
	TopicBrowser    TopicBrowserConfig      `yaml:"topicbrowser,omitempty"`
	Redpanda        RedpandaConfig          `yaml:"redpanda,omitempty"` // Redpanda config, can be updated while running
}

type AgentConfig struct {
	Location                    map[int]string `yaml:"location,omitempty"`
	ReleaseChannel              ReleaseChannel `yaml:"releaseChannel,omitempty"`
	CommunicatorConfig          `yaml:"communicator,omitempty"`
	GraphQLConfig               GraphQLConfig `yaml:"graphql,omitempty"` // GraphQL server configuration
	MetricsPort                 int           `yaml:"metricsPort"`       // Port to expose metrics on
	Simulator                   bool          `yaml:"simulator,omitempty"`
	EnableResourceLimitBlocking bool          `yaml:"enableResourceLimitBlocking"` // Feature flag for resource-based bridge blocking

	// FSMv2 feature flags for incremental migration
	EnableFSMv2               bool   `yaml:"enableFSMv2,omitempty"`               // Master switch: starts ApplicationSupervisor
	FSMv2StorePrefix          string `yaml:"fsmv2StorePrefix,omitempty"`          // State isolation prefix (default: "fsmv2_")
	UseFSMv2ProtocolConverter bool   `yaml:"useFSMv2ProtocolConverter,omitempty"` // Migrate Protocol Converter to FSMv2
}

// ValidateFSMv2Flags validates FSMv2 feature flags and auto-enables EnableFSMv2
// if any component flag is set. Returns true if EnableFSMv2 was auto-enabled.
func (c *AgentConfig) ValidateFSMv2Flags() (autoEnabled bool) {
	hasComponentFlag := c.UseFSMv2Transport || c.UseFSMv2ProtocolConverter

	if hasComponentFlag && !c.EnableFSMv2 {
		c.EnableFSMv2 = true

		return true
	}

	return false
}

type CommunicatorConfig struct {
	APIURL            string `yaml:"apiUrl,omitempty"`
	AuthToken         string `yaml:"authToken,omitempty"`
	AllowInsecureTLS  bool   `yaml:"allowInsecureTLS,omitempty"`  // Allow TLS connections without verifying the certificate.
	UseFSMv2Transport bool   `yaml:"useFSMv2Transport,omitempty"` // Feature flag: use FSMv2 communicator instead of legacy Puller/Pusher.
}

// FSMInstanceConfig is the config for a FSM instance.
type FSMInstanceConfig struct {
	// Name of the service, we use omitempty here, as some services like redpanda will ignore this name, therefore writing the "empty" name back to the config file will cause confusion for the users
	Name            string `yaml:"name,omitempty"`
	DesiredFSMState string `yaml:"desiredState,omitempty"`
}

// ContainerConfig is the config for a container instance.
type ContainerConfig struct {
	Name            string `yaml:"name"`
	DesiredFSMState string `yaml:"desiredState"`
}

// AgentMonitorConfig is the config for an agent monitor instance.
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

// S6FSMConfig contains configuration for creating a service.
type S6FSMConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	S6ServiceConfig s6serviceconfig.S6ServiceConfig `yaml:"s6ServiceConfig"`
}

// BenthosMonitorConfig contains configuration for creating a benthos monitor.
type BenthosMonitorConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	MetricsPort uint16 `yaml:"metricsPort"` // Port to expose metrics on
}

// BenthosConfig contains configuration for creating a Benthos service.
type BenthosConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Benthos service
	BenthosServiceConfig benthosserviceconfig.BenthosServiceConfig `yaml:"benthosServiceConfig"`
}

// DataFlowComponentConfig contains configuration for creating a DataFlowComponent.
type DataFlowComponentConfig struct {

	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	DataFlowComponentServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataFlowComponentConfig"`

	// private marker – not (un)marshalled
	// explanation see templating.go
	hasAnchors bool `yaml:"-"`
}

// HasAnchors returns true if the DataFlowComponentConfig has anchors, see templating.go.
func (d *DataFlowComponentConfig) HasAnchors() bool { return d.hasAnchors }

// ProtocolConverterConfig contains configuration for creating a ProtocolConverter.
type ProtocolConverterConfig struct {

	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	anchorName string `yaml:"-"`

	ProtocolConverterServiceConfig protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec `yaml:"protocolConverterServiceConfig"`

	// private marker – not (un)marshalled
	// explanation see templating.go
	hasAnchors bool `yaml:"-"`
}

// HasAnchors returns true if the ProtocolConverterConfig has anchors, see templating.go.
func (d *ProtocolConverterConfig) HasAnchors() bool { return d.hasAnchors }

// AnchorName returns the anchor name of the ProtocolConverterConfig, see templating.go.
func (d *ProtocolConverterConfig) AnchorName() string { return d.anchorName }

// StreamProcessorConfig contains configuration for creating a StreamProcessor.
type StreamProcessorConfig struct {
	StreamProcessorServiceConfig streamprocessorserviceconfig.StreamProcessorServiceConfigSpec `yaml:"streamProcessorServiceConfig"`

	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	anchorName string `yaml:"-"`

	// private marker – not (un)marshalled
	// explanation see templating.go
	hasAnchors bool `yaml:"-"`
}

// HasAnchors returns true if the StreamProcessorConfig has anchors, see templating.go.
func (d *StreamProcessorConfig) HasAnchors() bool { return d.hasAnchors }

// AnchorName returns the anchor name of the StreamProcessorConfig, see templating.go.
func (d *StreamProcessorConfig) AnchorName() string { return d.anchorName }

// NmapConfig contains configuration for creating a Nmap service.
type NmapConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Nmap service
	NmapServiceConfig nmapserviceconfig.NmapServiceConfig `yaml:"nmapServiceConfig"`
}

// RedpandaMonitorConfig contains configuration for creating a Redpanda monitor.
type RedpandaMonitorConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`
}

// RedpandaConfig contains configuration for a Redpanda service.
type RedpandaConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Redpanda service
	RedpandaServiceConfig redpandaserviceconfig.RedpandaServiceConfig `yaml:"redpandaServiceConfig,omitempty"`
}

// ConnectionConfig contains configuration for creating a Connection service.
type ConnectionConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Connection service
	ConnectionServiceConfig connectionserviceconfig.ConnectionServiceConfig `yaml:"connectionServiceConfig"`
}

type GraphQLConfig struct {
	CORSOrigins  []string `yaml:"corsOrigins,omitempty"`  // CORS allowed origins (default: ["*"])
	Port         int      `yaml:"port"`                   // Port to expose GraphQL on (default: 8090)
	Enabled      bool     `yaml:"enabled"`                // Enable/disable GraphQL server
	Debug        bool     `yaml:"debug,omitempty"`        // Enable GraphiQL playground and debug logging
	AuthRequired bool     `yaml:"authRequired,omitempty"` // Require authentication (future use)
}

// TopicBrowserConfig contains configuration for creating a Topic Browser service.
type TopicBrowserConfig struct {
	// For the FSM
	FSMInstanceConfig `yaml:",inline"`

	// For the Connection service
	TopicBrowserServiceConfig topicbrowserserviceconfig.Config `yaml:"serviceConfig,omitempty"`
}

// PayloadShape defines the structure for a single payload shape.
type PayloadShape struct {
	Fields      map[string]PayloadField `yaml:"fields"`                // fields of the payload shape
	Description string                  `yaml:"description,omitempty"` // description of the payload shape
}

// PayloadField represents a field within a payload shape with recursive structure support.
type PayloadField struct {
	Subfields map[string]PayloadField `yaml:",inline"`         // subfields for recursive definition (inline to allow direct field access)
	Type      string                  `yaml:"_type,omitempty"` // type of the field (number, string, etc.)
}

// Clone creates a deep copy of FullConfig.
func (c FullConfig) Clone() FullConfig {
	clone := FullConfig{
		Agent:             c.Agent,
		PayloadShapes:     make(map[string]PayloadShape),
		DataModels:        make([]DataModelsConfig, len(c.DataModels)),
		DataContracts:     make([]DataContractsConfig, len(c.DataContracts)),
		DataFlow:          make([]DataFlowComponentConfig, len(c.DataFlow)),
		ProtocolConverter: make([]ProtocolConverterConfig, len(c.ProtocolConverter)),
		StreamProcessor:   make([]StreamProcessorConfig, len(c.StreamProcessor)),
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

	err = deepcopy.Copy(&clone.PayloadShapes, &c.PayloadShapes)
	if err != nil {
		return FullConfig{}
	}

	err = deepcopy.Copy(&clone.DataModels, &c.DataModels)
	if err != nil {
		return FullConfig{}
	}

	err = deepcopy.Copy(&clone.DataContracts, &c.DataContracts)
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

	err = deepcopy.Copy(&clone.StreamProcessor, &c.StreamProcessor)
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
