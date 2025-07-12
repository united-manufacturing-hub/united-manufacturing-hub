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

// GetDFCServiceConfig converts the component config to a full ProtocolConverterServiceConfig
// For a read DFC, the user is not allowed to set its own output config, so we "enforce" the output config
// to be the UNS output config. This ensures protocol converters always write to the unified namespace.
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
		"stream_processor": map[string]any{
			"mode":         "timeseries",
			"model":        c.Config.Model,
			"sources":      enhancedSources, // Use enhanced sources with umh.v1. prefix
			"mapping":      c.Config.Mapping,
			"output_topic": outputTopic,
		},
	}

	// Always set UNS output
	dfcReadConfig.BenthosConfig.Output = map[string]any{
		"uns": map[string]any{},
	}

	return dfcReadConfig
}

// FromDFCServiceConfig creates a StreamProcessorServiceConfigRuntime from a DFC config
// This is used by the service's GetConfig method to reconstruct the StreamProcessor config
// from the underlying DFC deployment
func FromDFCServiceConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig) StreamProcessorServiceConfigRuntime {
	// Extract the stream processor config from the DFC pipeline section
	var model ModelRef
	var sources SourceMapping
	var mapping map[string]any

	if pipeline, ok := dfcConfig.BenthosConfig.Pipeline["stream_processor"].(map[string]any); ok {
		// Convert model from map[string]any to ModelRef
		if modelData, ok := pipeline["model"].(map[string]any); ok {
			if name, ok := modelData["name"].(string); ok {
				model.Name = name
			}
			if version, ok := modelData["version"].(string); ok {
				model.Version = version
			}
		}

		// Convert sources from map[string]any to SourceMapping
		if sourcesData, ok := pipeline["sources"].(map[string]any); ok {
			sources = make(SourceMapping)
			for key, value := range sourcesData {
				if strValue, ok := value.(string); ok {
					// Strip the "umh.v1." prefix to get the clean user-facing format
					cleanValue := strValue
					if strings.HasPrefix(strValue, "umh.v1.") {
						cleanValue = strings.TrimPrefix(strValue, "umh.v1.")
					}
					sources[key] = cleanValue
				}
			}
		}

		if mappingData, ok := pipeline["mapping"].(map[string]any); ok {
			mapping = mappingData
		}
	}

	return StreamProcessorServiceConfigRuntime{
		Model:   model,
		Sources: sources,
		Mapping: mapping,
	}
}
