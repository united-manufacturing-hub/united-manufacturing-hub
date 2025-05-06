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
	"strings"
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
	if cfg.Topic.DefaultTopicRetentionBytes == 0 {
		cfg.Topic.DefaultTopicRetentionBytes = 0
	}

	if cfg.Topic.DefaultTopicRetentionMs == 0 {
		cfg.Topic.DefaultTopicRetentionMs = 604800000 // Redpanda by default sets this to 7 days when set to 0, therefore we just set it to 7 days to keep the code a bit cleaner
	}

	if cfg.BaseDir == "" {
		cfg.BaseDir = "/data"
	}

	// Strip trailing / from basedir (if present)
	cfg.BaseDir = strings.TrimSuffix(cfg.BaseDir, "/")

	// Resources.MaxCores & Resources.MemoryPerCoreInBytes are not used in the template, but directly passed to the redpanda binary

	// Render the template
	var rendered bytes.Buffer
	if err := g.tmpl.Execute(&rendered, cfg); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return rendered.String(), nil
}

var simplifiedTemplate = `# Redpanda configuration file

redpanda:
  data_directory: "{{.BaseDir}}/redpanda"

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
  log_retention_ms: {{if eq .Topic.DefaultTopicRetentionMs 0}}-1{{else}}{{.Topic.DefaultTopicRetentionMs}}{{end}}
  retention_bytes: {{if eq .Topic.DefaultTopicRetentionBytes 0}}null{{else}}{{.Topic.DefaultTopicRetentionBytes}}{{end}}

  # Set the default number of partitions for new topics
  default_topic_partitions: 1

  # Enable auto topic creation
  auto_create_topics_enabled: true

pandaproxy: {}

schema_registry: {}

rpk:
  coredump_dir: "{{.BaseDir}}/redpanda/coredump"
`
