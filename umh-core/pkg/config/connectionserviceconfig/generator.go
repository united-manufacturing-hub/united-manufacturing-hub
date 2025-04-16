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
	"text/template"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"gopkg.in/yaml.v3"
)

// Generator handles the generation of Nmap YAML configurations
type Generator struct {
	tmpl *template.Template
}

// NewGenerator creates a new YAML generator for Benthos configurations
func NewGenerator() *Generator {
	return &Generator{
		tmpl: template.Must(template.New("connection").Parse(simplifiedTemplate)),
	}
}

// RenderConfig generates a Connection YAML configuration from a ConnectionServiceConfig
func (g *Generator) RenderConfig(cfg ConnectionServiceConfig) (string, error) {

	// Convert the config to a normalized map
	configMap := g.configToMap(cfg)
	normalizedMap := normalizeConfig(configMap)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(normalizedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Connection config: %w", err)
	}

	yamlStr := string(yamlBytes)

	return yamlStr, nil
}

// configToMap converts a ConnectionServiceConfig to a raw map for YAML generation
func (g *Generator) configToMap(cfg ConnectionServiceConfig) map[string]any {
	configMap := make(map[string]any)

	// Add all sections
	if cfg.NmapServiceConfig.Target != "" && len(cfg.NmapServiceConfig.Target) > 0 {
		configMap["target"] = cfg.NmapServiceConfig.Target
	}

	if cfg.NmapServiceConfig.Port != 0 {
		configMap["port"] = cfg.NmapServiceConfig.Port
	}

	// We need indent this since it should look like this:
	// connection:
	//    nmap:
	//      target: "127.0.0.1"
	//      port: 443
	connectionConfigMap := make(map[string]any)
	connectionConfigMap["nmap"] = configMap

	return connectionConfigMap
}

// normalizeConfig does not need to adjust anything here
func normalizeConfig(raw map[string]any) map[string]any {
	return raw
}

// templateData represents the data structure expected by the simplified Connection YAML template
type templateData struct {
	NmapServiceConfig *nmapserviceconfig.NmapServiceConfig
}

// simplifiedTemplate is a much simpler template that just places pre-rendered YAML blocks
var simplifiedTemplate = `
nmap:
  target: {{.Target}}
  port: {{.Port}}
`
