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

package streamprocessorserviceconfig

import (
	"encoding/hex"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"gopkg.in/yaml.v3"
)

// GetDFCServiceConfig converts the component config to a full DFCServiceConfig
// it enforces the output to uns-plugin, the input to uns-plugin and prefixes the
// given sources and umh-topics accordingly with it's location path.
func (c StreamProcessorServiceConfigSpec) GetDFCServiceConfig(spName string) dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	// create dfc config
	dfcReadConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}

	// Extract topics from sources for input configuration
	// These are template strings that will be rendered with variables later
	var umhTopics []string

	enhancedSources := make(map[string]string)

	for alias, topic := range c.Config.Sources {
		// Topics in sources are template strings like "{{ .location_path }}._raw.{{ .abc }}"
		// We need to prefix them with "umh.v1." to make them valid UNS topics
		// But check if they already have the prefix to avoid double prefixing
		umhTopic := topic
		if !strings.HasPrefix(topic, "umh.v1.") {
			umhTopic = "umh.v1." + topic
		}

		umhTopics = append(umhTopics, umhTopic)
		enhancedSources[alias] = umhTopic
	}

	hexName := hex.EncodeToString([]byte(spName))

	// Set UNS input with topics from sources
	dfcReadConfig.BenthosConfig.Input = map[string]any{
		"uns": map[string]any{
			"umh_topics":     umhTopics,
			"consumer_group": hexName,
		},
	}

	// Build the output topic - this should be the base location path without _raw suffix
	// Use template variable since this will be rendered later
	outputTopic := "umh.v1.{{ .location_path }}"

	dfcReadConfig.BenthosConfig.Pipeline = map[string]any{
		"processors": []any{
			map[string]any{
				"stream_processor": StreamProcessorBenthosConfig{
					Model:       c.Config.Model,
					Mode:        "timeseries",
					Sources:     enhancedSources,
					Mapping:     c.Config.Mapping,
					OutputTopic: outputTopic,
				},
			},
		},
	}

	// Always set UNS output
	dfcReadConfig.BenthosConfig.Output = map[string]any{
		"uns": map[string]any{
			"bridged_by": "{{ .internal.bridged_by }}",
		},
	}

	return dfcReadConfig
}

type StreamProcessorBenthosConfig struct {
	Model       ModelRef       `yaml:"model"`
	Mode        string         `yaml:"mode"`
	Sources     SourceMapping  `yaml:"sources"`
	Mapping     map[string]any `yaml:"mapping"`
	OutputTopic string         `yaml:"output_topic"`
}

// FromDFCServiceConfig creates a StreamProcessorServiceConfigRuntime from a DFC config.
func FromDFCServiceConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig) StreamProcessorServiceConfigRuntime {
	// Extract the stream processor config from the DFC pipeline section
	var (
		model   ModelRef
		sources SourceMapping
		mapping map[string]any
	)

	if processor, ok := dfcConfig.BenthosConfig.Pipeline["processors"].([]any); ok {
		for _, v := range processor {
			if processor, ok := v.(map[string]any); ok {
				if streamProcessorMap, ok := processor["stream_processor"].(map[string]any); ok {
					// Use YAML marshal/unmarshal to convert map to struct to avoid nesting pain
					yamlBytes, err := yaml.Marshal(streamProcessorMap)
					if err == nil {
						var streamProcessor StreamProcessorBenthosConfig
						if err := yaml.Unmarshal(yamlBytes, &streamProcessor); err == nil {
							model = streamProcessor.Model
							sources = streamProcessor.Sources
							mapping = streamProcessor.Mapping
						}
					}
				}
			}
		}
	}

	// Strip umh.v1. prefix from sources to match original configuration format
	originalSources := make(SourceMapping)
	for alias, topic := range sources {
		originalSources[alias] = strings.TrimPrefix(topic, "umh.v1.")
	}

	return StreamProcessorServiceConfigRuntime{
		Model:   model,
		Sources: originalSources,
		Mapping: mapping,
	}
}
