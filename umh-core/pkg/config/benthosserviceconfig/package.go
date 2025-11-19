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

package benthosserviceconfig

var (
	defaultGenerator  = NewGenerator()
	defaultNormalizer = NewNormalizer()
	defaultComparator = NewComparator()
)

// BenthosServiceConfig represents the configuration for a Benthos service.
type BenthosServiceConfig struct {
	// Benthos-specific configuration
	Input    map[string]interface{} `yaml:"input"`
	Pipeline map[string]interface{} `yaml:"pipeline,omitempty"`
	Output   map[string]interface{} `yaml:"output"`
	Buffer   map[string]interface{} `yaml:"buffer,omitempty"`

	CacheResources     []map[string]interface{} `yaml:"cache_resources,omitempty"`
	RateLimitResources []map[string]interface{} `yaml:"rate_limit_resources,omitempty"`

	// Advanced configuration
	MetricsPort uint16 `yaml:"metrics_port"`
	DebugLevel  bool   `yaml:"debug_level,omitempty"`
}

// DefaultTopicBrowserBenthosServiceConfig is the default Benthos service config for the topic browser
// It is considered immutable and MUST NOT be modified at runtime. (However Go does not support const for complex types).
var DefaultTopicBrowserBenthosServiceConfig = BenthosServiceConfig{
	Input: map[string]any{
		"uns": map[string]any{
			"umh_topic":      "umh.v1.*",
			"kafka_topic":    "umh.messages",
			"broker_address": "localhost:9092",
			"consumer_group": "topic-browser",
		},
	},
	Pipeline: map[string]any{
		// We need to use []interface{} here because the extractor for the comparator requires it to function correctly
		"processors": []interface{}{
			map[string]any{
				"topic_browser": map[string]any{},
			},
		},
	},
	Output: map[string]any{
		"stdout": map[string]any{},
	},
	DebugLevel: false,
}

// Equal checks if two BenthosServiceConfigs are equal.
func (c BenthosServiceConfig) Equal(other BenthosServiceConfig) bool {
	return NewComparator().ConfigsEqual(c, other)
}

// RenderBenthosYAML is a package-level function for easy YAML generation.
func RenderBenthosYAML(input, output, pipeline, cacheResources, rateLimitResources, buffer interface{}, metricsPort uint16, debugLevel bool) (string, error) {
	// Create a config object from the individual components
	cfg := BenthosServiceConfig{
		MetricsPort: metricsPort,
		DebugLevel:  debugLevel,
	}

	// Convert each section to the appropriate map
	if input != nil {
		if inputMap, ok := input.(map[string]interface{}); ok {
			cfg.Input = inputMap
		}
	}

	if output != nil {
		if outputMap, ok := output.(map[string]interface{}); ok {
			cfg.Output = outputMap
		}
	}

	if pipeline != nil {
		if pipelineMap, ok := pipeline.(map[string]interface{}); ok {
			cfg.Pipeline = pipelineMap
		}
	}

	if buffer != nil {
		if bufferMap, ok := buffer.(map[string]interface{}); ok {
			cfg.Buffer = bufferMap
		}
	}

	// Handle resources
	if cacheResources != nil {
		if cacheArray, ok := cacheResources.([]map[string]interface{}); ok {
			cfg.CacheResources = cacheArray
		} else if cacheList, ok := cacheResources.([]interface{}); ok {
			// Try to convert each item to the expected type
			for _, item := range cacheList {
				if resMap, ok := item.(map[string]interface{}); ok {
					cfg.CacheResources = append(cfg.CacheResources, resMap)
				}
			}
		}
	}

	if rateLimitResources != nil {
		if rateArray, ok := rateLimitResources.([]map[string]interface{}); ok {
			cfg.RateLimitResources = rateArray
		} else if rateList, ok := rateLimitResources.([]interface{}); ok {
			// Try to convert each item to the expected type
			for _, item := range rateList {
				if resMap, ok := item.(map[string]interface{}); ok {
					cfg.RateLimitResources = append(cfg.RateLimitResources, resMap)
				}
			}
		}
	}

	// Use the generator to render the YAML
	return defaultGenerator.RenderConfig(cfg)
}

// NormalizeBenthosConfig is a package-level function for easy config normalization.
func NormalizeBenthosConfig(cfg BenthosServiceConfig) BenthosServiceConfig {
	return defaultNormalizer.NormalizeConfig(cfg)
}

// ConfigsEqual is a package-level function for easy config comparison.
func ConfigsEqual(desired, observed BenthosServiceConfig) bool {
	return defaultComparator.ConfigsEqual(desired, observed)
}

// ConfigDiff is a package-level function for easy config diff generation.
func ConfigDiff(desired, observed BenthosServiceConfig) string {
	return defaultComparator.ConfigDiff(desired, observed)
}
