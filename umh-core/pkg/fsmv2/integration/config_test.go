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


package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("Phase 0.5 Integration Tests", func() {
	Describe("Scenario 1: Variable Flattening", func() {
		It("should promote User variables to top-level and nest Global and Internal", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
					"cluster_id":   "cluster-123",
				},
				Internal: map[string]any{
					"id":        "worker-456",
					"timestamp": "2025-11-04T10:00:00Z",
				},
			}

			flattened := bundle.Flatten()

			Expect(flattened["IP"]).To(Equal("192.168.1.100"))
			Expect(flattened["PORT"]).To(Equal(502))

			globalMap, ok := flattened["global"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(globalMap["api_endpoint"]).To(Equal("https://api.example.com"))
			Expect(globalMap["cluster_id"]).To(Equal("cluster-123"))

			internalMap, ok := flattened["internal"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(internalMap["id"]).To(Equal("worker-456"))
			Expect(internalMap["timestamp"]).To(Equal("2025-11-04T10:00:00Z"))
		})
	})

	Describe("Scenario 2: Template Rendering", func() {
		It("should generate config from template with substituted variables", func() {
			template := `host: {{ .IP }}
port: {{ .PORT }}
name: {{ .name }}`

			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
					"name": "PLC-Connection",
				},
			}

			flattened := bundle.Flatten()
			rendered, err := config.RenderTemplate(template, flattened)

			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("host: 192.168.1.100"))
			Expect(rendered).To(ContainSubstring("port: 502"))
			Expect(rendered).To(ContainSubstring("name: PLC-Connection"))
			Expect(rendered).NotTo(ContainSubstring("{{"))
			Expect(rendered).NotTo(ContainSubstring("}}"))
		})
	})

	Describe("Scenario 3: Location Computation", func() {
		It("should merge parent and child locations into complete ISA-95 path", func() {
			parentLocation := []config.LocationLevel{
				{Type: "enterprise", Value: "ACME"},
				{Type: "site", Value: "Factory-1"},
			}

			childLocation := []config.LocationLevel{
				{Type: "line", Value: "Line-A"},
				{Type: "cell", Value: "Cell-5"},
			}

			merged := config.MergeLocations(parentLocation, childLocation)
			Expect(merged).To(HaveLen(4))

			filled := config.NormalizeHierarchyLevels(merged)
			Expect(filled).To(HaveLen(5))

			Expect(filled[0].Type).To(Equal("enterprise"))
			Expect(filled[0].Value).To(Equal("ACME"))
			Expect(filled[1].Type).To(Equal("site"))
			Expect(filled[1].Value).To(Equal("Factory-1"))
			Expect(filled[2].Type).To(Equal("area"))
			Expect(filled[2].Value).To(Equal(""))
			Expect(filled[3].Type).To(Equal("line"))
			Expect(filled[3].Value).To(Equal("Line-A"))
			Expect(filled[4].Type).To(Equal("cell"))
			Expect(filled[4].Value).To(Equal("Cell-5"))

			path := config.ComputeLocationPath(filled)
			Expect(path).To(Equal("ACME.Factory-1.Line-A.Cell-5"))
			Expect(path).NotTo(ContainSubstring(".."))
		})
	})

	Describe("Scenario 4: Variable Propagation", func() {
		It("should make parent variables available to child workers via UserSpec", func() {
			parentVariables := config.VariableBundle{
				User: map[string]any{
					"parent_ip":   "192.168.1.1",
					"parent_port": 1883,
				},
				Global: map[string]any{
					"cluster_id": "prod-cluster",
				},
			}

			childSpec := config.ChildSpec{
				Name:       "mqtt-connection",
				WorkerType: "mqtt_client",
				UserSpec: config.UserSpec{
					Config: "url: tcp://{{ .parent_ip }}:{{ .parent_port }}",
					Variables: config.VariableBundle{
						User: map[string]any{
							"parent_ip":   parentVariables.User["parent_ip"],
							"parent_port": parentVariables.User["parent_port"],
							"child_var":   "child-value",
						},
						Global: parentVariables.Global,
					},
				},
			}

			Expect(childSpec.UserSpec.Variables.User["parent_ip"]).To(Equal("192.168.1.1"))
			Expect(childSpec.UserSpec.Variables.User["parent_port"]).To(Equal(1883))
			Expect(childSpec.UserSpec.Variables.User["child_var"]).To(Equal("child-value"))
			Expect(childSpec.UserSpec.Variables.Global["cluster_id"]).To(Equal("prod-cluster"))

			flattened := childSpec.UserSpec.Variables.Flatten()
			rendered, err := config.RenderTemplate(childSpec.UserSpec.Config, flattened)

			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("url: tcp://192.168.1.1:1883"))
		})
	})

	Describe("Scenario 5: Strict Mode Enforcement", func() {
		It("should return error when template references missing variable", func() {
			template := "host: {{ .IP }}\nport: {{ .MISSING_VAR }}"

			bundle := config.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
			}

			flattened := bundle.Flatten()
			_, err := config.RenderTemplate(template, flattened)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MISSING_VAR"))
		})
	})

	Describe("Scenario 6: Nested Variables", func() {
		It("should access Global and Internal via namespace prefixes", func() {
			template := `user_ip: {{ .IP }}
global_api: {{ .global.api_endpoint }}
internal_id: {{ .internal.id }}`

			bundle := config.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id": "worker-789",
				},
			}

			flattened := bundle.Flatten()
			rendered, err := config.RenderTemplate(template, flattened)

			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("user_ip: 192.168.1.100"))
			Expect(rendered).To(ContainSubstring("global_api: https://api.example.com"))
			Expect(rendered).To(ContainSubstring("internal_id: worker-789"))
		})
	})

	Describe("Scenario 7: Empty Locations", func() {
		It("should fill missing ISA-95 levels with empty strings and skip in path", func() {
			sparseLocation := []config.LocationLevel{
				{Type: "enterprise", Value: "ACME"},
				{Type: "line", Value: "Line-A"},
			}

			filled := config.NormalizeHierarchyLevels(sparseLocation)

			Expect(filled).To(HaveLen(5))
			Expect(filled[0].Type).To(Equal("enterprise"))
			Expect(filled[0].Value).To(Equal("ACME"))
			Expect(filled[1].Type).To(Equal("site"))
			Expect(filled[1].Value).To(Equal(""))
			Expect(filled[2].Type).To(Equal("area"))
			Expect(filled[2].Value).To(Equal(""))
			Expect(filled[3].Type).To(Equal("line"))
			Expect(filled[3].Value).To(Equal("Line-A"))
			Expect(filled[4].Type).To(Equal("cell"))
			Expect(filled[4].Value).To(Equal(""))

			path := config.ComputeLocationPath(filled)
			Expect(path).To(Equal("ACME.Line-A"))
			Expect(path).NotTo(ContainSubstring(".."))
		})
	})
})
