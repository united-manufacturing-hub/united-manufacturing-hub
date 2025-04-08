package dataflowcomponentconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// BenthosConfig contains only the essential Benthos configuration fields
// that should be exposed to DataFlowComponent users
type BenthosConfig struct {
	Input              map[string]interface{}   `yaml:"input"`
	Pipeline           map[string]interface{}   `yaml:"pipeline"`
	Output             map[string]interface{}   `yaml:"output"`
	CacheResources     []map[string]interface{} `yaml:"cache_resources,omitempty"`
	RateLimitResources []map[string]interface{} `yaml:"rate_limit_resources,omitempty"`
	Buffer             map[string]interface{}   `yaml:"buffer,omitempty"`
}

// DataFlowComponentConfig represents the configuration for a DataFlowComponent
type DataFlowComponentConfig struct {
	BenthosConfig BenthosConfig `yaml:"benthos"`
}

// ToBenthosServiceConfig converts the simplified BenthosConfig to a full BenthosServiceConfig
// with default advanced configuration
func (bc *BenthosConfig) ToBenthosServiceConfig() benthosserviceconfig.BenthosServiceConfig {
	return benthosserviceconfig.BenthosServiceConfig{
		Input:              bc.Input,
		Pipeline:           bc.Pipeline,
		Output:             bc.Output,
		CacheResources:     bc.CacheResources,
		RateLimitResources: bc.RateLimitResources,
		Buffer:             bc.Buffer,
		// Default values for advanced configuration
		MetricsPort: 0, // Will be assigned dynamically by the port manager
		LogLevel:    constants.DefaultBenthosLogLevel,
	}
}

// GetBenthosServiceConfig converts the component config to a full BenthosServiceConfig
func (c *DataFlowComponentConfig) GetBenthosServiceConfig() benthosserviceconfig.BenthosServiceConfig {
	return c.BenthosConfig.ToBenthosServiceConfig()
}

// FromBenthosServiceConfig creates a DataFlowComponentConfig from a BenthosServiceConfig,
// ignoring advanced configuration fields
func FromBenthosServiceConfig(benthos benthosserviceconfig.BenthosServiceConfig) DataFlowComponentConfig {
	return DataFlowComponentConfig{
		BenthosConfig: BenthosConfig{
			Input:              benthos.Input,
			Pipeline:           benthos.Pipeline,
			Output:             benthos.Output,
			CacheResources:     benthos.CacheResources,
			RateLimitResources: benthos.RateLimitResources,
			Buffer:             benthos.Buffer,
		},
	}
}
