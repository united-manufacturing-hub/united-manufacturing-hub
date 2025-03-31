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

package redpandaserviceconfig

import (
	"bytes"
	"fmt"
	"text/template"
)

// Generator handles the generation of Redpanda YAML configurations
type Generator struct {
	tmpl *template.Template
}

// NewGenerator creates a new YAML generator for Redpanda configurations
func NewGenerator() *Generator {
	return &Generator{
		tmpl: template.Must(template.New("redpanda").Parse(simplifiedTemplate)),
	}
}

// RenderConfig generates a Redpanda YAML configuration from a RedpandaServiceConfig
func (g *Generator) RenderConfig(cfg RedpandaServiceConfig) (string, error) {
	if cfg.DefaultTopicRetentionMs == 0 {
		cfg.DefaultTopicRetentionMs = 0
	}

	if cfg.DefaultTopicRetentionBytes == 0 {
		cfg.DefaultTopicRetentionBytes = 0
	}

	if cfg.MaxCores == 0 {
		cfg.MaxCores = 1
	}

	// Render the template
	var rendered bytes.Buffer
	if err := g.tmpl.Execute(&rendered, cfg); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return rendered.String(), nil
}

var simplifiedTemplate = `# Redpanda configuration file

redpanda:
  data_directory: "/data/redpanda"

  seed_servers: []

  rpc_server:
    address: "0.0.0.0"
    port: 33145

  advertised_rpc_api:
    address: "127.0.0.1"
    port: 33145

  kafka_api:
  - address: "0.0.0.0"
    port: 9092

  advertised_kafka_api:
  - address: "127.0.0.1"
    port: 9092

  admin:
    address: "0.0.0.0"
    port: 9644

  developer_mode: true

  # Default topic retention configuration:
  log_retention_ms: {{if eq .DefaultTopicRetentionMs 0}}-1{{else}}{{.DefaultTopicRetentionMs}}{{end}}
  retention_bytes: {{if eq .DefaultTopicRetentionBytes 0}}null{{else}}{{.DefaultTopicRetentionBytes}}{{end}}

  # Performance configuration:
  smp: {{.MaxCores}}

  # Auto topic creation configuration:
  auto_create_topics_enabled: true  # Enable automatic topic creation

pandaproxy: {}

schema_registry: {}

rpk:
  coredump_dir: "/data/redpanda/coredump"
`
