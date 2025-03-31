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

package redpandaserviceconfig

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// RedpandaServiceConfig represents the configuration for a Redpanda service
type RedpandaServiceConfig struct {
	// Redpanda-specific configuration
	DefaultTopicRetentionMs    int    `yaml:"defaultTopicRetentionMs"`
	DefaultTopicRetentionBytes int    `yaml:"defaultTopicRetentionBytes"`
	MaxCores                   int    `yaml:"maxCores"`
	MemoryPerCoreInBytes       int    `yaml:"memoryPerCoreInBytes"`
	BaseDir                    string `yaml:"baseDir"`
}

// Equal checks if two RedpandaServiceConfigs are equal
func (c RedpandaServiceConfig) Equal(other RedpandaServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderRedpandaYAML is a package-level function for easy YAML generation
func RenderRedpandaYAML(defaultTopicRetentionMs int, defaultTopicRetentionBytes int) (string, error) {
	// Create a config object from the individual components
	cfg := RedpandaServiceConfig{
		DefaultTopicRetentionMs:    defaultTopicRetentionMs,
		DefaultTopicRetentionBytes: defaultTopicRetentionBytes,
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeRedpandaConfig is a package-level function for easy config normalization
func NormalizeRedpandaConfig(cfg RedpandaServiceConfig) RedpandaServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison
func ConfigsEqual(desired, observed RedpandaServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation
func ConfigDiff(desired, observed RedpandaServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
