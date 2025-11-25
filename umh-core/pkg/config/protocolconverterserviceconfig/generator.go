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
	// Convert the config to a normalized map
	configMap := g.configToMap(cfg)
	normalizedMap := normalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ProtocolConverter config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// configToMap converts a DataFlowComponentServiceConfig to a raw map for YAML generation.
func (g *Generator) configToMap(cfg ProtocolConverterServiceConfigSpec) map[string]any {
	// use generator to create a valid dfcConfigMap & connectionConfigMap
	dfcGenerator := dataflowcomponentserviceconfig.NewGenerator()
	connectionGenerator := connectionserviceconfig.NewGenerator()
	variableBundleGenerator := variables.NewGenerator()

	// Get the template configs with DebugLevel properly set
	// Priority: DFC level (most specific) > Spec level (least specific)
	dfcReadConfig := cfg.Config.DataflowComponentReadServiceConfig
	dfcReadConfig.DebugLevel = dfcReadConfig.DebugLevel || cfg.DebugLevel
	dfcReadConfigMap := dfcGenerator.ConfigToMap(dfcReadConfig)

	dfcWriteConfig := cfg.Config.DataflowComponentWriteServiceConfig
	dfcWriteConfig.DebugLevel = dfcWriteConfig.DebugLevel || cfg.DebugLevel
	dfcWriteConfigMap := dfcGenerator.ConfigToMap(dfcWriteConfig)
	// Convert template to runtime for config map generation
	connRuntime, err := connectionserviceconfig.ConvertTemplateToRuntime(cfg.Config.ConnectionServiceConfig)
	if err != nil {
		// If conversion fails, use empty config to avoid breaking YAML generation
		connRuntime = connectionserviceconfig.ConnectionServiceConfigRuntime{}
	}

	connectionConfigMap := connectionGenerator.ConfigToMap(connRuntime)
	variableBundleConfigMap := variableBundleGenerator.ConfigToMap(cfg.Variables)

	configMap := make(map[string]any)

	// Create the template structure
	templateMap := make(map[string]any)
	templateMap["connection"] = connectionConfigMap
	templateMap["dataflowcomponent_read"] = dfcReadConfigMap
	templateMap["dataflowcomponent_write"] = dfcWriteConfigMap

	// Add template and variables to the root config
	configMap["template"] = templateMap
	configMap["variables"] = variableBundleConfigMap

	configMap["location"] = cfg.Location
	configMap["debug_level"] = cfg.DebugLevel

	return configMap
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

	// Normalize each component
	normalizedDFCReadConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcReadConfig)
	normalizedDFCWriteConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcWriteConfig)
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

	// Preserve debug_level if it exists in raw config
	if debugLevel, ok := raw["debug_level"]; ok {
		normalized["debug_level"] = debugLevel
	}

	return normalized
}
