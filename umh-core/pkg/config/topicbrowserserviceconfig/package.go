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

package topicbrowserserviceconfig

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// Config represents the configuration for a Topic Browser service
type Config struct {
	// to be filled
}

// Equal checks if two BenthosServiceConfigs are equal
func (c Config) Equal(other Config) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderYAML is a package-level function for easy YAML generation
func RenderYAML(target string, port uint16) (string, error) {
	// Create a config object from the individual components
	cfg := Config{
		// to be filled
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeNmapConfig is a package-level function for easy config normalization
func NormalizeNmapConfig(cfg Config) Config {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed Config) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed Config) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
