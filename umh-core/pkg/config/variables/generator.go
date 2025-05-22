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

package variables

import (
	"gopkg.in/yaml.v3"
)

// Generator handles YAML generation for VariableBundle
type Generator struct{}

// NewGenerator creates a new Generator instance
func NewGenerator() *Generator {
	return &Generator{}
}

// RenderConfig converts a VariableBundle to YAML
func (g *Generator) RenderConfig(vb VariableBundle) (string, error) {
	bytes, err := yaml.Marshal(vb)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ConfigToMap converts a VariableBundle to a raw map for YAML generation
func (g *Generator) ConfigToMap(cfg VariableBundle) map[string]any {
	configMap := make(map[string]any)

	// Add user variables if present
	if len(cfg.User) > 0 {
		configMap["user"] = cfg.User
	}

	// Add global variables if present
	if len(cfg.Global) > 0 {
		configMap["global"] = cfg.Global
	}

	// Add internal variables if present
	if len(cfg.Internal) > 0 {
		configMap["internal"] = cfg.Internal
	}

	return configMap
}

// normalizeConfig applies nothing since Variables has no default settings
func NormalizeConfig(raw map[string]any) map[string]any {
	return raw
}
