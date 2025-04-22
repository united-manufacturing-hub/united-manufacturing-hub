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

package connectionserviceconfig

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// ConnectionServiceConfig represents the configuration for a DataFlowComponent
type ConnectionServiceConfig struct {
	NmapServiceConfig nmapserviceconfig.NmapServiceConfig `yaml:"nmap"`
}

// Equal checks if two ConnectionServiceConfigs are equal
func (c ConnectionServiceConfig) Equal(other ConnectionServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderConnectionYAML is a package-level function for easy YAML generation
func RenderConnectionYAML(nmap nmapserviceconfig.NmapServiceConfig) (string, error) {
	// Create a config object from the individual components
	cfg := ConnectionServiceConfig{
		NmapServiceConfig: nmap,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeConnectionConfig is a package-level function for easy config normalization
func NormalizeConnectionConfig(cfg ConnectionServiceConfig) ConnectionServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed ConnectionServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed ConnectionServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
