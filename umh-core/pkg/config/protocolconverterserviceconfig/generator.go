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

package protocolconverterserviceconfig

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of DFC YAML configurations
// BoilerPlateCode.
type Generator struct {
}

// NewGenerator creates a new YAML generator for DFC configurations.
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a ProtocolConverter YAML configuration from a ProtocolConverterServiceConfigSpec.
func (g *Generator) RenderConfig(cfg ProtocolConverterServiceConfigSpec) (string, error) {
	configMap, err := g.configToMap(cfg)
	if err != nil {
		return "", err
	}
	normalizedMap := normalizeConfig(configMap)

	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ProtocolConverter config: %w", err)
	}

	return string(yamlBytes), nil
}

// configToMap converts a ProtocolConverterServiceConfigSpec to a raw map for YAML generation.
func (g *Generator) configToMap(cfg ProtocolConverterServiceConfigSpec) (map[string]any, error) {
	dfcGenerator := dataflowcomponentserviceconfig.NewGenerator()
	connectionGenerator := connectionserviceconfig.NewGenerator()
	variableBundleGenerator := variables.NewGenerator()

	dfcReadConfigMap := dfcGenerator.ConfigToMap(cfg.Config.DataflowComponentReadServiceConfig)

	dfcWriteConfigMap, err := writeConfigToMap(cfg.Config.DataflowComponentWriteServiceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to convert write DFC config: %w", err)
	}

	connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(cfg.Config.ConnectionServiceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to convert connection template to runtime: %w", err)
	}

	connectionConfigMap := connectionGenerator.ConfigToMap(connRuntime)
	variableBundleConfigMap := variableBundleGenerator.ConfigToMap(cfg.Variables)

	templateMap := map[string]any{
		"connection":             connectionConfigMap,
		"dataflowcomponent_read": dfcReadConfigMap,
		"dataflowcomponent_write": dfcWriteConfigMap,
	}

	configMap := map[string]any{
		"template":  templateMap,
		"variables": variableBundleConfigMap,
		"location":  cfg.Location,
	}

	return configMap, nil
}

// normalizeConfig normalizes the configuration by applying defaults and ensuring consistency.
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
	dfcReadConfig, ok := template["dataflowcomponent_read"].(map[string]any)
	if !ok {
		dfcReadConfig = template
	}

	dfcWriteConfig, ok := template["dataflowcomponent_write"].(map[string]any)
	if !ok {
		dfcWriteConfig = template
	}

	connectionConfig, ok := template["connection"].(map[string]any)
	if !ok {
		connectionConfig = template
	}

	// Normalize each component.
	// Read DFC uses free-form benthos maps that need benthos normalization.
	// Write DFC uses a typed struct marshaled to a plain map; pass it through unchanged.
	normalizedDFCReadConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcReadConfig)
	normalizedDFCWriteConfig := dfcWriteConfig
	normalizedConnectionConfig := connectionserviceconfig.NormalizeConfig(connectionConfig)

	// Variables don't need normalization, they are just key-value pairs
	normalizedVariables := variables.NormalizeConfig(rawVariables)

	// Reconstruct the normalized template
	normalizedTemplate := make(map[string]any)
	normalizedTemplate["dataflowcomponent_read"] = normalizedDFCReadConfig
	normalizedTemplate["dataflowcomponent_write"] = normalizedDFCWriteConfig
	normalizedTemplate["connection"] = normalizedConnectionConfig

	// Set the normalized template and variables
	normalized["template"] = normalizedTemplate
	normalized["variables"] = normalizedVariables
	normalized["location"] = locationMap // Use our correctly processed location map

	return normalized
}

// writeConfigToMap marshals a DataflowComponentWriteConfigInput to a plain map[string]any
// for inclusion in the YAML generator output.
func writeConfigToMap(cfg dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput) (map[string]any, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("writeConfigToMap: failed to marshal write DFC config: %w", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("writeConfigToMap: failed to unmarshal write DFC config: %w", err)
	}
	return m, nil
}
