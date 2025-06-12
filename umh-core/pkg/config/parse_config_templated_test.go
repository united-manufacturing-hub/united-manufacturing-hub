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

package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseConfigTemplated", func() {
	Context("when the config is valid and anchors are used", func() {
		It("should parse the config correctly", func() {
			cfg, err := os.ReadFile("../../examples/example-config-protocolconverter-templated.yaml")
			Expect(err).To(BeNil())

			parsedConfig, anchorMap, err := parseConfig(cfg, false)

			Expect(err).To(BeNil())

			Expect(parsedConfig.ProtocolConverter).To(HaveLen(3))
			Expect(parsedConfig.ProtocolConverter[0].Name).To(Equal("temperature-sensor-pc"))
			Expect(parsedConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Variables.User).To(HaveKeyWithValue("IP", "10.0.1.50"))
			Expect(parsedConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Variables.User).To(HaveKeyWithValue("PORT", "4840"))

			generatedConfigGeneratePart := parsedConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Template.DataflowComponentReadServiceConfig.BenthosConfig.Input["opcua"]
			Expect(generatedConfigGeneratePart).ToNot(BeNil())

			Expect(anchorMap).To(HaveKeyWithValue("temperature-sensor-pc", "opcua_http"))
			Expect(parsedConfig.Templates).To(HaveLen(1))
			Expect(parsedConfig.Templates[0]).To(Equal(map[string]any{
				"connection": map[string]any{
					"nmap": map[string]any{
						"target": "{{ .IP }}",
						"port":   "{{ .PORT }}",
					},
				},
				"dataflowcomponent_read": map[string]any{
					"benthos": map[string]any{
						"input": map[string]any{
							"opcua": map[string]any{
								"address": "opc.tcp://{{ .IP }}:{{ .PORT }}",
							},
						},
					},
				},
				"dataflowcomponent_write": map[string]any{
					"benthos": map[string]any{
						"output": map[string]any{
							"http_client": map[string]any{
								"url": "http://collector.local/ingest",
							},
						},
					},
				},
			}))

		})
	})

	Context("when the config is valid and no anchors are used", func() {
		It("should parse the config correctly", func() {
			cfg, err := os.ReadFile("../../examples/example-config-protocolconverter.yaml")
			Expect(err).To(BeNil())

			parsedConfig, anchorMap, err := parseConfig(cfg, false)
			Expect(err).To(BeNil())

			Expect(anchorMap).To(BeEmpty())

			Expect(parsedConfig.ProtocolConverter).To(HaveLen(1))
			Expect(parsedConfig.ProtocolConverter[0].Name).To(Equal("vibration-sensor-pc"))
			Expect(parsedConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Template.ConnectionServiceConfig.NmapTemplate).ToNot(BeNil())
			Expect(parsedConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Template.DataflowComponentReadServiceConfig.BenthosConfig.Input["generate"]).ToNot(BeNil())

			Expect(anchorMap).To(BeEmpty())
		})
	})

	Context("when parsing a config with and without anchors", func() {
		It("should parse both equally", func() {
			cfgTemplated, err := os.ReadFile("../../examples/example-config-protocolconverter-templated.yaml")
			Expect(err).To(BeNil())

			cfgUntemplated, err := os.ReadFile("../../examples/example-config-protocolconverter-untemplated.yaml")
			Expect(err).To(BeNil())

			parsedConfigTemplated, anchorMapTemplated, err := parseConfig(cfgTemplated, false)
			Expect(err).To(BeNil())
			Expect(anchorMapTemplated).ToNot(BeEmpty())

			parsedConfigUntemplated, anchorMapUntemplated, err := parseConfig(cfgUntemplated, false)
			Expect(err).To(BeNil())
			Expect(anchorMapUntemplated).To(BeEmpty())

			Expect(parsedConfigTemplated.ProtocolConverter).To(Equal(parsedConfigUntemplated.ProtocolConverter))
		})
	})
})
