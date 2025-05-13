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

package protocolconverterserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ProtocolConverterServiceConfig represents the configuration for a ProtocolConverter
type ProtocolConverterServiceConfig struct {
	ConnectionServiceConfig        connectionserviceconfig.ConnectionServiceConfig               `yaml:"connection"`
	DataflowComponentServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig `yaml:"dataflowcomponent"`
}

// Equal checks if two ProtocolConverterServiceConfigs are equal
func (c ProtocolConverterServiceConfig) Equal(other ProtocolConverterServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderProtocolConverterYAML is a package-level function for easy YAML generation
func RenderProtocolConverterYAML(
	connection connectionserviceconfig.ConnectionServiceConfig,
	dfc dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
) (string, error) {
	// Create a config object from the individual components
	cfg := ProtocolConverterServiceConfig{
		ConnectionServiceConfig:        connection,
		DataflowComponentServiceConfig: dfc,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeProtocolConverterConfig is a package-level function for easy config normalization
func NormalizeProtocolConverterConfig(cfg ProtocolConverterServiceConfig) ProtocolConverterServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed ProtocolConverterServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed ProtocolConverterServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
