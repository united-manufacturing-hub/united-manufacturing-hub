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
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of DFC YAML configurations
// BoilerPlateCode
type Generator struct {
}

// NewGenerator creates a new YAML generator for DFC configurations
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a ProtocolConverter YAML configuration from a ProtocolConverterServiceConfig
func (g *Generator) RenderConfig(cfg ProtocolConverterServiceConfig) (string, error) {

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

// configToMap converts a DataFlowComponentServiceConfig to a raw map for YAML generation
func (g *Generator) configToMap(cfg ProtocolConverterServiceConfig) map[string]any {
	// use generator to create a valid dfcConfigMap & connectionConfigMap
	dfcGenerator := dataflowcomponentserviceconfig.NewGenerator()
	connectionGenerator := connectionserviceconfig.NewGenerator()

	dfcReadConfigMap := dfcGenerator.ConfigToMap(cfg.DataflowComponentReadServiceConfig)
	dfcWriteConfigMap := dfcGenerator.ConfigToMap(cfg.DataflowComponentWriteServiceConfig)
	connectionConfigMap := connectionGenerator.ConfigToMap(cfg.ConnectionServiceConfig)

	configMap := make(map[string]any)

	// indent the config by 1
	configMap["dataflowcomponent_read"] = dfcReadConfigMap
	configMap["dataflowcomponent_write"] = dfcWriteConfigMap
	configMap["connection"] = connectionConfigMap

	return configMap
}

// normalizeConfig does not need to adjust anything here
func normalizeConfig(raw map[string]any) map[string]any {
	normalized := make(map[string]any)

	// extract and check the dfc config
	dfcReadConfig, ok := raw["dataflowcomponent_read"].(map[string]any)
	if !ok {
		dfcReadConfig = raw
	}

	dfcWriteConfig, ok := raw["dataflowcomponent_write"].(map[string]any)
	if !ok {
		dfcWriteConfig = raw
	}

	// extract and check the connection config
	connectionConfig, ok := raw["connection"].(map[string]any)
	if !ok {
		connectionConfig = raw
	}

	normalizedDFCReadConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcReadConfig)
	normalizedDFCWriteConfig := dataflowcomponentserviceconfig.NormalizeConfig(dfcWriteConfig)
	normalizedConnectionConfig := connectionserviceconfig.NormalizeConfig(connectionConfig)
	normalized["dataflowcomponent_read"] = normalizedDFCReadConfig
	normalized["dataflowcomponent_write"] = normalizedDFCWriteConfig
	normalized["connection"] = normalizedConnectionConfig
	return normalized
}
