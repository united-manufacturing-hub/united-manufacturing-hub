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

package topicbrowserserviceconfig

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Generator handles the generation of Nmap YAML configurations
// Currently BoilerPlate-Code
type Generator struct{}

// NewGenerator creates a new YAML generator for Nmap configurations
// Currently BoilerPlate-Code

func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig generates a Topic Browser YAML configuration from a Config
func (g *Generator) RenderConfig(cfg Config) (string, error) {
	// Convert the config to a normalized map
	configMap := g.ConfigToMap(cfg)
	normalizedMap := normalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Nmap config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// ConfigToMap converts a Config to a raw map for YAML generation
func (g *Generator) ConfigToMap(cfg Config) map[string]any {
	configMap := make(map[string]any)

	// Add all sections

	return configMap
}

// normalizeConfig applies nothing since Nmap has no default settings
func normalizeConfig(raw map[string]any) map[string]any {
	return raw
}
