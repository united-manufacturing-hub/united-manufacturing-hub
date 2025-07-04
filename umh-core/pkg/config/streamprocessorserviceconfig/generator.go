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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of DFC YAML configurations
// BoilerPlateCode
type Generator struct{}

// NewGenerator creates a new YAML generator for DFC configurations
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a Stream Processor YAML configuration from a ConfigSpec
func (g *Generator) RenderConfig(cfg ConfigSpec) (string, error) {
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

// configToMap converts a DataFlowComponentServiceConfig to a raw map for YAML generation
func (g *Generator) configToMap(cfg ConfigSpec) map[string]any {
	// use generator to create a valid dfcConfigMap & connectionConfigMap
	dfcGenerator := dataflowcomponentserviceconfig.NewGenerator()
	variableBundleGenerator := variables.NewGenerator()

	// Get the template configs
	dfcConfigMap := dfcGenerator.ConfigToMap(cfg.Config.DFCConfig)
	// Convert template to runtime for config map generation
	variableBundleConfigMap := variableBundleGenerator.ConfigToMap(cfg.Variables)

	configMap := make(map[string]any)

	// Create the template structure
	templateMap := make(map[string]any)
	templateMap["dataflowcomponent"] = dfcConfigMap

	// Add template and variables to the root config
	configMap["template"] = templateMap
	configMap["variables"] = variableBundleConfigMap
	configMap["location"] = cfg.Location
	return configMap
}

// normalizeConfig normalizes the configuration by applying defaults and ensuring consistency
func normalizeConfig(raw map[string]any) map[string]any {
	normalized := make(map[string]any)

	// Extract template and variables
	template, ok := raw["template"].(map[string]any)
	if !ok {
		template = raw
	}

	rawVariables, ok := raw["variables"].(map[string]any)
	if !ok {
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
	dfcConfig, ok := template["dataflowcomponent"].(map[string]any)
	if !ok {
		dfcConfig = template
	}

	// Normalize each component
	normalizedDFCConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcConfig)

	// Variables don't need normalization, they are just key-value pairs
	normalizedVariables := variables.NormalizeConfig(rawVariables)

	// Reconstruct the normalized template
	normalizedTemplate := make(map[string]any)
	normalizedTemplate["dataflowcomponent"] = normalizedDFCConfig

	// Set the normalized template and variables
	normalized["template"] = normalizedTemplate
	normalized["variables"] = normalizedVariables
	normalized["location"] = locationMap // Use our correctly processed location map
	return normalized
}
