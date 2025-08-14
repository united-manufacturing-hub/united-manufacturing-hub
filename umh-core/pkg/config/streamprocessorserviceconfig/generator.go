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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of StreamProcessor YAML configurations.
type Generator struct {
}

// NewGenerator creates a new YAML generator for StreamProcessor configurations.
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a StreamProcessor YAML configuration from a StreamProcessorServiceConfigSpec.
func (g *Generator) RenderConfig(cfg StreamProcessorServiceConfigSpec) (string, error) {
	// Convert the config to a normalized map
	configMap := g.configToMap(cfg)
	normalizedMap := normalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal StreamProcessor config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// configToMap converts a StreamProcessorServiceConfigSpec to a raw map for YAML generation.
func (g *Generator) configToMap(cfg StreamProcessorServiceConfigSpec) map[string]any {
	// use generator to create a valid variableBundleConfigMap
	variableBundleGenerator := variables.NewGenerator()

	// Get the template configs
	variableBundleConfigMap := variableBundleGenerator.ConfigToMap(cfg.Variables)

	configMap := make(map[string]any)

	// Create the template structure
	templateMap := make(map[string]any)
	templateMap["model"] = cfg.Config.Model
	templateMap["sources"] = cfg.Config.Sources
	templateMap["mapping"] = cfg.Config.Mapping

	// Add template and variables to the root config
	configMap["template"] = templateMap
	configMap["variables"] = variableBundleConfigMap
	configMap["location"] = cfg.Location

	return configMap
}

// normalizeConfig normalizes the configuration by applying defaults and ensuring consistency.
func normalizeConfig(raw map[string]any) map[string]any {
	normalized := make(map[string]any)

	// Extract template and variables
	template, isValidTemplate := raw["template"].(map[string]any)
	if !isValidTemplate {
		template = raw
	}

	rawVariables, isValidVariables := raw["variables"].(map[string]any)
	if !isValidVariables {
		rawVariables = make(map[string]any)
	}

	// Process location map correctly to ensure it's a proper map[string]string
	// This handles converting location keys (like level numbers) to the correct format
	locationMap := make(map[string]string)
	if rawLocation, ok := raw["location"].(map[string]any); ok {
		// Convert map[string]any to map[string]string
		for k, v := range rawLocation {
			// Use fmt.Sprint to convert any value type to string
			// This handles all basic types (string, int, float, bool) with appropriate formatting
			locationMap[k] = fmt.Sprint(v)
		}
	} else if typedLocation, ok := raw["location"].(map[string]string); ok {
		// If already in the right format, just use it directly
		locationMap = typedLocation
	}

	// Extract and normalize template components
	modelConfig, exists := template["model"].(map[string]any)
	if !exists {
		modelConfig = make(map[string]any)
	}

	sourcesConfig, exists := template["sources"].(map[string]any)
	if !exists {
		sourcesConfig = make(map[string]any)
	}

	mappingConfig, exists := template["mapping"].(map[string]any)
	if !exists {
		mappingConfig = make(map[string]any)
	}

	// Variables don't need normalization, they are just key-value pairs
	normalizedVariables := variables.NormalizeConfig(rawVariables)

	// Reconstruct the normalized template
	normalizedTemplate := make(map[string]any)
	normalizedTemplate["model"] = modelConfig
	normalizedTemplate["sources"] = sourcesConfig
	normalizedTemplate["mapping"] = mappingConfig

	// Set the normalized template and variables
	normalized["template"] = normalizedTemplate
	normalized["variables"] = normalizedVariables
	normalized["location"] = locationMap // Use our correctly processed location map

	return normalized
}
