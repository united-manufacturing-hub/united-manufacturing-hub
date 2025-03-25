package mgmtconfig

import (
	"time"

	"github.com/Masterminds/semver/v3"
)

type Network struct {
	ExternalIP              string `yaml:"externalIP"`
	ExternalGateway         string `yaml:"externalGateway"`
	ExternalDeviceInterface string `yaml:"externalDeviceInterface"`
	DNSServer               string `yaml:"dnsServer"`
}

type OS struct {
	Architecture string `yaml:"architecture"`
}

type Helm struct {
	ChartVersion *semver.Version `yaml:"chartVersion"`
	// SameTopicExperience is a feature that maps 1 MQTT topic to 1 Kafka topic
	SameTopicExperience *bool `yaml:"sameTopicExperience"`
	// SkipSafetyChecks is a feature that skips our disk and uptime checks (For testing purposes)
	SkipSafetyChecks *bool `yaml:"skipSafetyChecks"`
}

type Configurations struct {
	GrafanaIni *GrafanaIni `yaml:"grafanaIni"`
	NodeRED    *NodeRED    `yaml:"nodeRED"`
}

type GrafanaIni struct {
	Config          map[string]interface{} `yaml:"config"`
	LastUpdated     time.Time              `yaml:"lastUpdated"`
	UUID            string                 `yaml:"uuid"`
	Versions        []ConfigVersion        `yaml:"versions"`
	DeployedVersion string                 `yaml:"deployedVersion"`
}

type NodeRED struct {
	Config          map[string]interface{} `yaml:"config"`
	LastUpdated     time.Time              `yaml:"lastUpdated"`
	UUID            string                 `yaml:"uuid"`
	Versions        []ConfigVersion        `yaml:"versions"`
	DeployedVersion string                 `yaml:"deployedVersion"`
}

type ConfigVersion struct {
	UUID             string    `yaml:"uuid"`
	CreatedAt        time.Time `yaml:"createdAt"`
	CreatedBy        string    `yaml:"createdBy"`
	DeploySuccessful bool      `yaml:"deploySuccessful"`
}

type MgmtConfig struct {
	Connections          []Connection         `yaml:"connections"`
	Tls                  TlsConfig            `yaml:"tls"`
	Umh                  UmhConfig            `yaml:"umh"`
	Debug                DebugConfig          `yaml:"debug"`
	FeatureFlags         map[string]bool      `yaml:"flags,omitempty"`
	Location             Location             `yaml:"location"`
	Brokers              Brokers              `yaml:"brokers"`
	ProtocolConverterMap ProtocolConverterMap `yaml:"protocolConverterMap"`
	DataFlowComponents   DataFlowComponents   `yaml:"dataFlowComponents"`
	Network              Network              `yaml:"network"`
	OS                   OS                   `yaml:"os"`
	Helm                 Helm                 `yaml:"helm"`
	Configurations       Configurations       `yaml:"configurations"`
}
