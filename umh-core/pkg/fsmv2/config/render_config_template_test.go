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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("RenderConfigTemplate", func() {
	Describe("Empty template handling", func() {
		It("should return empty string for empty template (no error)", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
			}

			result, err := config.RenderConfigTemplate("", vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(""))
		})
	})

	Describe("User variables at top-level", func() {
		It("should access User variables at top-level ({{ .IP }}, {{ .PORT }})", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
			}
			tmpl := "modbus://{{ .IP }}:{{ .PORT }}"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("modbus://192.168.1.100:502"))
		})
	})

	Describe("Global variables nested under 'global'", func() {
		It("should access Global variables under 'global' prefix ({{ .global.api_endpoint }})", func() {
			vars := config.VariableBundle{
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
					"cluster_id":   "prod-123",
				},
			}
			tmpl := "endpoint: {{ .global.api_endpoint }}, cluster: {{ .global.cluster_id }}"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("endpoint: https://api.example.com, cluster: prod-123"))
		})
	})

	Describe("Internal variables nested under 'internal'", func() {
		It("should access Internal variables under 'internal' prefix ({{ .internal.id }})", func() {
			vars := config.VariableBundle{
				Internal: map[string]any{
					"id":        "worker-abc-123",
					"timestamp": 1234567890,
				},
			}
			tmpl := "worker_id: {{ .internal.id }}, ts: {{ .internal.timestamp }}"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("worker_id: worker-abc-123, ts: 1234567890"))
		})
	})

	Describe("Strict mode enforcement", func() {
		It("should return error when variable is missing (strict mode)", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
			}
			tmpl := "{{ .IP }}:{{ .MISSING_PORT }}"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("MISSING_PORT"),
				ContainSubstring("execute template"),
			))
			Expect(result).To(Equal(""))
		})

		It("should return error when accessing missing nested variable", func() {
			vars := config.VariableBundle{
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
			}
			tmpl := "{{ .global.nonexistent_key }}"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(""))
		})
	})

	Describe("All three namespaces together", func() {
		It("should combine User, Global, and Internal variables in single template", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id": "worker-xyz",
				},
			}
			tmpl := `{
  "address": "{{ .IP }}:{{ .PORT }}",
  "api": "{{ .global.api_endpoint }}",
  "worker": "{{ .internal.id }}"
}`

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`{
  "address": "192.168.1.100:502",
  "api": "https://api.example.com",
  "worker": "worker-xyz"
}`))
		})
	})

	Describe("Complex templates with multiple substitutions", func() {
		It("should render complex benthos-style config template", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"name":          "modbus-plc",
					"IP":            "10.0.0.50",
					"PORT":          502,
					"location_path": "enterprise.site.area",
				},
				Global: map[string]any{
					"kafka_broker": "localhost:9092",
					"environment":  "production",
				},
				Internal: map[string]any{
					"id":         "bridge-12345",
					"created_at": "2025-01-13T10:00:00Z",
				},
			}
			tmpl := `input:
  label: "{{ .name }}_input"
  modbus_tcp:
    address: "{{ .IP }}:{{ .PORT }}"

output:
  kafka:
    addresses:
      - "{{ .global.kafka_broker }}"
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"

# Bridge ID: {{ .internal.id }}
# Environment: {{ .global.environment }}
# Created: {{ .internal.created_at }}`

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(`label: "modbus-plc_input"`))
			Expect(result).To(ContainSubstring(`address: "10.0.0.50:502"`))
			Expect(result).To(ContainSubstring(`- "localhost:9092"`))
			Expect(result).To(ContainSubstring(`topic: "umh.v1.enterprise.site.area.modbus-plc"`))
			Expect(result).To(ContainSubstring(`# Bridge ID: bridge-12345`))
			Expect(result).To(ContainSubstring(`# Environment: production`))
			Expect(result).To(ContainSubstring(`# Created: 2025-01-13T10:00:00Z`))
		})

		It("should handle repeated variable usage in template", func() {
			vars := config.VariableBundle{
				User: map[string]any{
					"name": "sensor-1",
				},
			}
			tmpl := "{{ .name }}_input, {{ .name }}_output, {{ .name }}_processor"

			result, err := config.RenderConfigTemplate(tmpl, vars)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("sensor-1_input, sensor-1_output, sensor-1_processor"))
		})
	})
})
