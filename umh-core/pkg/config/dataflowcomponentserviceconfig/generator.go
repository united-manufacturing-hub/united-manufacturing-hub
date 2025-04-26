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

package dataflowcomponentserviceconfig

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
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

// RenderConfig generates a DFC YAML configuration from a DFCServiceConfig
func (g *Generator) RenderConfig(cfg DataflowComponentServiceConfig) (string, error) {

	// Convert the config to a normalized map
	configMap := g.ConfigToMap(cfg)
	normalizedMap := NormalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DataFlowComponent config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// ConfigToMap converts a DataFlowComponentServiceConfig to a raw map for YAML generation
func (g *Generator) ConfigToMap(cfg DataflowComponentServiceConfig) map[string]any {
	// use generator to create a valid benthosConfigMap
	generator := benthosserviceconfig.NewGenerator()
	// generate BenthosServiceConfig (normalized) out of BenthosConfig
	benthosServiceConfig := cfg.BenthosConfig.ToBenthosServiceConfig()
	benthosConfigMap := generator.ConfigToMap(benthosServiceConfig)

	configMap := make(map[string]any)

	// indent the config by 1
	configMap["benthos"] = benthosConfigMap

	return configMap
}

// NormalizeConfig needs to adjust underlying benthos here
func NormalizeConfig(raw map[string]any) map[string]any {
	benthosConfig, ok := raw["benthos"].(map[string]any)
	if !ok {
		benthosConfig = raw
	}

	normalized := make(map[string]any)
	normalizedBenthosConfig := benthosserviceconfig.NormalizeConfig(benthosConfig)
	normalized["benthos"] = normalizedBenthosConfig
	return normalized
}
