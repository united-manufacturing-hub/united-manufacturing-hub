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
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

// GetDFCServiceConfig converts the component config to a full DFCServiceConfig
// it enforces the output to uns-plugin, the input to uns-plugin and prefixes the
// given sources and umh-topics accordingly with it's location path.
func (c StreamProcessorServiceConfigSpec) GetDFCServiceConfig() dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
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

	// Set UNS input with topics from sources
	dfcReadConfig.BenthosConfig.Input = map[string]any{
		"uns": map[string]any{
			"umh_topics": umhTopics,
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

	// dfcReadConfig.BenthosConfig.Pipeline = map[string]any{
	// 	"stream_processor": map[string]any{
	// 		"mode":         "timeseries",
	// 		"model":        c.Config.Model,
	// 		"sources":      enhancedSources, // Use enhanced sources with umh.v1. prefix
	// 		"mapping":      c.Config.Mapping,
	// 		"output_topic": outputTopic,
	// 	},
	// }

	// Always set UNS output
	dfcReadConfig.BenthosConfig.Output = map[string]any{
		"uns": map[string]any{},
	}

	return dfcReadConfig
}

type StreamProcessorBenthosConfig struct {
	Model       ModelRef
	Mode        string
	Sources     SourceMapping
	Mapping     map[string]any
	OutputTopic string
}

// FromDFCServiceConfig creates a StreamProcessorServiceConfigRuntime from a DFC config
func FromDFCServiceConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig) StreamProcessorServiceConfigRuntime {
	// Extract the stream processor config from the DFC pipeline section
	var model ModelRef
	var sources SourceMapping
	var mapping map[string]any

	if processor, ok := dfcConfig.BenthosConfig.Pipeline["processors"].([]any); ok {
		for _, v := range processor {
			if processor, ok := v.(map[string]any); ok {
				if streamProcessor, ok := processor["stream_processor"].(StreamProcessorBenthosConfig); ok {
					// found a stream processor
					model = streamProcessor.Model
					sources = streamProcessor.Sources
					mapping = streamProcessor.Mapping
				}
			}
		}
	}

	return StreamProcessorServiceConfigRuntime{
		Model:   model,
		Sources: sources,
		Mapping: mapping,
	}
}
