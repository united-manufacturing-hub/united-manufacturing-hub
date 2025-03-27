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
	if cfg.DataDirectory == "" {
		cfg.DataDirectory = "/data/redpanda"
	}

	// Render the template
	var rendered bytes.Buffer
	if err := g.tmpl.Execute(&rendered, cfg); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return rendered.String(), nil
}

// simplifiedTemplate is a much simpler template that just places pre-rendered YAML blocks
var simplifiedTemplate = `# Redpanda configuration file

redpanda:
  # Data directory where all the files will be stored.
  # This directory MUST reside on an ext4 or xfs partition.
  data_directory: {{.DataDirectory}}

  # The initial cluster nodes addresses
  seed_servers: []

  # Redpanda server
  rpc_server:
    address: "0.0.0.0"
    port: 33145

  # Redpanda server for other nodes to connect too
  advertised_rpc_api:
    address: "127.0.0.1"
    port: 33145

  # Kafka transport
  kafka_api:
  - address: "0.0.0.0"
    port: 9092

  # Kafka transport for other nodes to connect too
  advertised_kafka_api:
  - address: "127.0.0.1"
    port: 9092

  # Admin server includes metrics, and cluster management
  admin:
    address: "0.0.0.0"
    port: 9644

  # Skips most of the checks performed at startup (i.e. memory, xfs)
  # not recomended for production use
  developer_mode: true

# Enable Pandaproxy on port 8082
pandaproxy: {}

# Enable Schema Registry on port 8081
schema_registry: {}

rpk:
  # TLS configuration.
  #tls:
    # The path to the root CA certificate (PEM)
    #truststore_file: ""

    # The path to the client certificate (PEM)
    #cert_file: ""

    # The path to the client certificate key (PEM)
    #key_file: ""

  # Available tuners
  tune_network: false
  tune_disk_scheduler: false
  tune_disk_nomerges: false
  tune_disk_irq: false
  tune_fstrim: false
  tune_cpu: false
  tune_aio_events: false
  tune_clocksource: false
  tune_swappiness: false
  enable_memory_locking: false
  tune_coredump: false

  coredump_dir: {{.DataDirectory}}/coredump
`
