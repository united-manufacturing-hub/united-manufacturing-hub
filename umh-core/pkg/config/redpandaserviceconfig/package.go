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
// TopicConfig represents the topic-related configuration for Redpanda
type TopicConfig struct {
	DefaultTopicCompressionAlgorithm string `yaml:"defaultTopicCompressionAlgorithm,omitempty"`
	DefaultTopicCleanupPolicy        string `yaml:"defaultTopicCleanupPolicy,omitempty"`
	DefaultTopicRetentionMs          int64  `yaml:"defaultTopicRetentionMs,omitempty"`
	DefaultTopicRetentionBytes       int64  `yaml:"defaultTopicRetentionBytes,omitempty"`
	DefaultTopicSegmentMs            int64  `yaml:"defaultTopicSegmentMs,omitempty"`
}

// ResourcesConfig represents the resource-related configuration for Redpanda
type ResourcesConfig struct {
	MaxCores             int `yaml:"maxCores,omitempty"`
	MemoryPerCoreInBytes int `yaml:"memoryPerCoreInBytes,omitempty"`
}

// RedpandaServiceConfig represents the configuration for a Redpanda service
type RedpandaServiceConfig struct {
	BaseDir string `yaml:"baseDir,omitempty"`
	// Redpanda-specific configuration
	Topic     TopicConfig     `yaml:"topic,omitempty"`
	Resources ResourcesConfig `yaml:"resources,omitempty"`
}

// Equal checks if two RedpandaServiceConfigs are equal
func (c RedpandaServiceConfig) Equal(other RedpandaServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderRedpandaYAML is a package-level function for easy YAML generation
func RenderRedpandaYAML(defaultTopicRetentionMs int64, defaultTopicRetentionBytes int64, defaultTopicCompressionAlgorithm string, defaultTopicCleanupPolicy string, defaultTopicSegmentMs int64) (string, error) {
	// Create a config object from the individual components
	cfg := RedpandaServiceConfig{}
	cfg.Topic.DefaultTopicRetentionMs = defaultTopicRetentionMs
	cfg.Topic.DefaultTopicRetentionBytes = defaultTopicRetentionBytes
	cfg.Topic.DefaultTopicCompressionAlgorithm = defaultTopicCompressionAlgorithm
	cfg.Topic.DefaultTopicCleanupPolicy = defaultTopicCleanupPolicy
	cfg.Topic.DefaultTopicSegmentMs = defaultTopicSegmentMs

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
