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

package config

import (
	"bytes"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"gopkg.in/yaml.v3"
)

// since we use this function in runtime_config_test to best cover the functionality, we export it
func ParseConfig(data []byte, allowUnknownFields bool) (FullConfig, error) {
	var rawConfig FullConfig

	// First decode the YAML into the raw config structure using standard YAML functions
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(!allowUnknownFields) // Only reject unknown keys if allowUnknownFields is false
	if err := dec.Decode(&rawConfig); err != nil {
		return FullConfig{}, fmt.Errorf("failed to decode config: %w", err)
	}

	// Process templateRef resolution for protocol converters
	processedConfig, err := resolveProtocolConverterTemplateRefs(rawConfig)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to resolve protocol converter template references: %w", err)
	}

	return processedConfig, nil
}

// resolveProtocolConverterTemplateRefs processes protocol converter configs to resolve templateRef fields
// This translates between the "unrendered" config (with templateRef) and "rendered" config (with actual template content)
func resolveProtocolConverterTemplateRefs(config FullConfig) (FullConfig, error) {
	// Create a copy to avoid mutating the original
	processedConfig := config.Clone()

	// Build a map of available protocol converter templates for quick lookup
	templateMap := make(map[string]protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate)

	// Process protocol converter templates from the enforced structure
	for templateName, templateContent := range config.Templates.ProtocolConverter {
		// Convert the template content to the proper structure
		templateBytes, err := yaml.Marshal(templateContent)
		if err != nil {
			return FullConfig{}, fmt.Errorf("failed to marshal template %s: %w", templateName, err)
		}

		var template protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate
		if err := yaml.Unmarshal(templateBytes, &template); err != nil {
			return FullConfig{}, fmt.Errorf("failed to unmarshal template %s: %w", templateName, err)
		}

		templateMap[templateName] = template
	}

	// Process each protocol converter to resolve templateRef
	for i, pc := range processedConfig.ProtocolConverter {
		// Only resolve templateRef if it's not empty/null and there's no inline config
		if pc.ProtocolConverterServiceConfig.TemplateRef != "" {
			// Resolve the template reference
			templateName := pc.ProtocolConverterServiceConfig.TemplateRef
			template, exists := templateMap[templateName]
			if !exists {
				return FullConfig{}, fmt.Errorf("template reference %q not found for protocol converter %s", templateName, pc.Name)
			}

			// Create a new spec with the resolved template
			resolvedSpec := pc.ProtocolConverterServiceConfig
			resolvedSpec.Config = template
			resolvedSpec.TemplateRef = "" // Clear the reference since it's now resolved

			// Update the config
			processedConfig.ProtocolConverter[i].ProtocolConverterServiceConfig = resolvedSpec
		}
		// If templateRef is empty/null, use the inline config as-is
	}

	return processedConfig, nil
}
