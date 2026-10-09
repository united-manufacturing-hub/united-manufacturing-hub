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

package examplebridge_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
)

func TestExamplebridge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Bridge Suite")
}

var _ = Describe("ExamplebridgeWorker", func() {
	Describe("DeriveDesiredState", func() {
		It("builds every name a flow template can use and keeps the flow templates unrendered", func() {
			logger := deps.NewNopFSMLogger()
			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{
				Config: `name: b1
connection:
  host: 10.0.0.5
  port: 502
location:
  1: hall
  3: cell
variables:
  SLAVE_ID: 7
read: "{{ .connection.host }}"
`,
				ConfigValues: map[string]any{
					"agent": map[string]any{
						"location": map[string]any{"0": "acme", "1": ""},
					},
					"historian": map[string]any{
						"timescale": map[string]any{"host": "db"},
					},
				},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)
			Expect(err).ToNot(HaveOccurred())

			desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[examplebridge.ExamplebridgeConfig])
			Expect(ok).To(BeTrue())

			names := desired.Config.Names
			Expect(names).ToNot(BeNil())

			By("the entry's fields, by their YAML path")
			Expect(names["name"]).To(Equal("b1"))
			connection, ok := names["connection"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(connection["host"]).To(Equal("10.0.0.5"))

			By("the IP and PORT shortcuts")
			Expect(names["IP"]).To(Equal("10.0.0.5"))
			portRendered, err := fsmv2types.Template("{{ .PORT }}").Render("probe", names)
			Expect(err).ToNot(HaveOccurred())
			Expect(portRendered).To(Equal("502"))

			By("the variables section and its shortcut")

			varRendered, err := fsmv2types.Template(`{{ .variables.SLAVE_ID }}`).Render("probe", names)
			Expect(err).ToNot(HaveOccurred())
			Expect(varRendered).To(Equal("7"))

			shortcutRendered, err := fsmv2types.Template(`{{ .SLAVE_ID }}`).Render("probe", names)
			Expect(err).ToNot(HaveOccurred())
			Expect(shortcutRendered).To(Equal("7"))

			By("the merged location and its path, ready-made")
			Expect(names["location_path"]).To(Equal("acme.hall.unknown.cell"))

			location, ok := names["location"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(location["0"]).To(Equal("acme"))
			Expect(location["2"]).To(Equal("unknown"))
			Expect(location["3"]).To(Equal("cell"))

			By("the historian shortcut")

			historian, ok := names["historian"].(map[string]any)
			Expect(ok).To(BeTrue())

			timescale, ok := historian["timescale"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(timescale["host"]).To(Equal("db"))

			By("the flow template stays unrendered")
			Expect(string(desired.Config.Read)).To(Equal(`{{ .connection.host }}`))
		})

		It("hands both flow templates down and keeps the write template unrendered", func() {
			logger := deps.NewNopFSMLogger()
			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{
				Config: `name: b1
read: "{{ .connection.host }}"
write: "{{ .IP }}:{{ .PORT }}"
`,
				ConfigValues: map[string]any{
					"agent": map[string]any{
						"location": map[string]any{"0": "acme", "1": ""},
					},
					"historian": map[string]any{
						"timescale": map[string]any{"host": "db"},
					},
				},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)
			Expect(err).ToNot(HaveOccurred())

			desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[examplebridge.ExamplebridgeConfig])
			Expect(ok).To(BeTrue())
			Expect(string(desired.Config.Read)).To(Equal(`{{ .connection.host }}`))
			Expect(string(desired.Config.Write)).To(Equal(`{{ .IP }}:{{ .PORT }}`))
		})

		Describe("RenderFlows", func() {
			flowNames := map[string]any{
				"name": "b1",
				"IP":   "10.0.0.5",
				"PORT": 502,
			}
			cfg := examplebridge.ExamplebridgeConfig{
				Name:  "b1",
				Read:  `{{ .connection.host }}:"quoted: text"`,
				Write: `{{ .IP }}:{{ .PORT }}`,
				Names: flowNames,
			}

			It("renders one child per non-empty flow template with the exact template text", func() {
				specs, err := examplebridge.RenderFlows("b1", cfg)
				Expect(err).ToNot(HaveOccurred())
				Expect(specs).To(HaveLen(2))

				byName := map[string]fsmv2types.ChildSpec{}
				for _, spec := range specs {
					byName[spec.Name] = spec
				}
				Expect(byName).To(HaveKey("b1-read"))
				Expect(byName).To(HaveKey("b1-write"))

				for _, flow := range []string{"read", "write"} {
					spec := byName["b1-"+flow]

					Expect(spec.WorkerType).To(Equal("exampleflow"))
					Expect(spec.Enabled).To(BeTrue())

					Expect(spec.UserSpec.Names).To(Equal(flowNames))

					var decoded struct {
						Flow string `yaml:"flow"`
					}
					err := yaml.Unmarshal([]byte(spec.UserSpec.Config), &decoded)
					Expect(err).ToNot(HaveOccurred())
					Expect(decoded.Flow).To(Equal(string(cfg.Template(flow))))
				}
			})

			It("renders one child when only read is set", func() {
				onlyRead := cfg
				onlyRead.Write = ""

				specs, err := examplebridge.RenderFlows("b1", onlyRead)
				Expect(err).ToNot(HaveOccurred())
				Expect(specs).To(HaveLen(1))
				Expect(specs[0].Name).To(Equal("b1-read"))
			})

			It("names two bridges' children apart, so their store keys stay distinct", func() {
				b2 := cfg
				b2.Name = "b2"
				b2.Connection.Host = "10.0.0.6"

				b1Specs, err := examplebridge.RenderFlows("b1", cfg)
				Expect(err).ToNot(HaveOccurred())
				b2Specs, err := examplebridge.RenderFlows("b2", b2)
				Expect(err).ToNot(HaveOccurred())

				b1Names := map[string]bool{}
				for _, spec := range b1Specs {
					b1Names[spec.Name] = true
				}

				for _, spec := range b2Specs {
					Expect(b1Names).NotTo(HaveKey(spec.Name))
				}
			})
		})

		It("yields the default desired state for a nil spec", func() {
			logger := deps.NewNopFSMLogger()

			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			desiredIface, err := worker.DeriveDesiredState(nil)
			Expect(err).ToNot(HaveOccurred())

			desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[examplebridge.ExamplebridgeConfig])
			Expect(ok).To(BeTrue())
			Expect(desired.Config.Names).To(BeNil())
		})

		It("rejects a connection that is not a mapping", func() {
			logger := deps.NewNopFSMLogger()

			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{Config: `name: b1
connection: 10.0.0.5:502
`}
			// A typed Connection field makes yaml itself reject the scalar:
			// the decode failure is a *ConfigError keeping yaml's text.
			_, err = worker.DeriveDesiredState(spec)

			var cfgErr *fsmv2types.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Error()).To(ContainSubstring("examplebridge.Connection"))
		})

		It("rejects an agent that is not a mapping", func() {
			logger := deps.NewNopFSMLogger()

			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{
				Config: `name: b1
`,
				ConfigValues: map[string]any{"agent": "acme"},
			}

			_, err = worker.DeriveDesiredState(spec)

			var cfgErr *fsmv2types.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal("agent"))
		})

		It("rejects a variable named like a connection shortcut", func() {
			logger := deps.NewNopFSMLogger()

			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{Config: `name: b1
connection:
  host: 10.0.0.5
variables:
  IP: 1.2.3.4
`}

			_, err = worker.DeriveDesiredState(spec)

			var cfgErr *fsmv2types.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal("IP"))
			Expect(cfgErr.Error()).To(ContainSubstring(`"IP" is both the connection shortcut and a variable`))
		})

		It("rejects an unknown desired state", func() {
			logger := deps.NewNopFSMLogger()

			worker, err := examplebridge.NewExamplebridgeWorker(
				deps.Identity{ID: "test-bridge", Name: "Test Bridge"},
				logger,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2types.UserSpec{Config: `name: b1
state: paused
`}

			_, err = worker.DeriveDesiredState(spec)

			var cfgErr *fsmv2types.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal("state"))
		})
	})
})
