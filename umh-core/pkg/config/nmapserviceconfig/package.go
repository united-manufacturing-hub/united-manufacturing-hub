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

package nmapserviceconfig

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// NmapServiceConfig represents the configuration for a Nmap service
type NmapServiceConfig struct {
	// Target to scan (hostname or IP)
	Target string `yaml:"target"`
	// Port to scan (single port number)
	Port uint16 `yaml:"port"`
}

// Equal checks if two BenthosServiceConfigs are equal
func (c NmapServiceConfig) Equal(other NmapServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderNmapYAML is a package-level function for easy YAML generation
func RenderNmapYAML(target string, port uint16) (string, error) {
	// Create a config object from the individual components
	cfg := NmapServiceConfig{
		Target: target,
		Port:   port,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeNmapConfig is a package-level function for easy config normalization
func NormalizeNmapConfig(cfg NmapServiceConfig) NmapServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed NmapServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed NmapServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
