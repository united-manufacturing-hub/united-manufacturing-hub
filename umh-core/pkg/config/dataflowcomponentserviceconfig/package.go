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

package dataflowcomponentserviceconfig

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// BenthosConfig contains only the essential Benthos configuration fields
// that should be exposed to DataFlowComponent users
type BenthosConfig struct {
	Input              map[string]any   `yaml:"input"`
	Pipeline           map[string]any   `yaml:"pipeline"`
	Output             map[string]any   `yaml:"output"`
	CacheResources     []map[string]any `yaml:"cache_resources,omitempty"`
	RateLimitResources []map[string]any `yaml:"rate_limit_resources,omitempty"`
	Buffer             map[string]any   `yaml:"buffer,omitempty"`
}

// DataflowComponentServiceConfig represents the configuration for a DataFlowComponent
type DataflowComponentServiceConfig struct {
	BenthosConfig BenthosConfig `yaml:"benthos"`
}

// Equal checks if two DataFlowComponentServiceConfigs are equal
func (c DataflowComponentServiceConfig) Equal(other DataflowComponentServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderDataFlowComponentYAML is a package-level function for easy YAML generation
func RenderDataFlowComponentYAML(benthos benthosserviceconfig.BenthosServiceConfig) (string, error) {
	// Create a config object from the individual components
	cfg := DataflowComponentServiceConfig{
		BenthosConfig: FromBenthosServiceConfig(benthos).BenthosConfig,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeDataFlowComponentConfig is a package-level function for easy config normalization
func NormalizeDataFlowComponentConfig(cfg DataflowComponentServiceConfig) {
	defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed DataflowComponentServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed DataflowComponentServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
