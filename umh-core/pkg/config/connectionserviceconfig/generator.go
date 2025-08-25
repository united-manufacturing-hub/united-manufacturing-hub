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

package connectionserviceconfig

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of Nmap YAML configurations.
type Generator struct {
}

// NewGenerator creates a new YAML generator for Benthos configurations.
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a Connection YAML configuration from a ConnectionServiceConfig.
func (g *Generator) RenderConfig(cfg ConnectionServiceConfig) (string, error) {
	// Convert the config to a normalized map
	configMap := g.ConfigToMap(cfg)
	normalizedMap := NormalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Connection config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// ConfigToMap converts a ConnectionServiceConfig to a raw map for YAML generation.
func (g *Generator) ConfigToMap(cfg ConnectionServiceConfig) map[string]any {
	// use generator to create a valid nmapConfigMap
	generator := nmapserviceconfig.NewGenerator()
	nmapConfigMap := generator.ConfigToMap(cfg.NmapServiceConfig)

	configMap := make(map[string]any)
	// We need indent this since it should look like this:
	// connection:
	//    nmap:
	//      target: "127.0.0.1"
	//      port: 443
	configMap["nmap"] = nmapConfigMap

	return configMap
}

// NormalizeConfig does not need to adjust anything here.
func NormalizeConfig(raw map[string]any) map[string]any {
	return raw
}
