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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Template Rendering with Array Variables", func() {
	Describe("RenderTemplate with array variables", func() {
		Context("when using range to iterate over array", func() {
			It("should render template with S7 address array using range", func() {
				type S7Config struct {
					Inputs []map[string]any `yaml:"inputs"`
				}

				template := S7Config{
					Inputs: []map[string]any{
						{
							"label": "s7_reader",
							"s7": map[string]any{
								"addresses": "{{ range .ADDRESSES }}{{ . }}\n{{ end }}",
							},
						},
					},
				}

				scope := map[string]any{
					"ADDRESSES": []string{
						"DB3.X0.0",
						"DB3.X0.1",
						"DB3.X0.2",
					},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Inputs).To(HaveLen(1))
				s7Config := result.Inputs[0]["s7"].(map[string]any)
				addresses := s7Config["addresses"].(string)

				Expect(addresses).To(ContainSubstring("DB3.X0.0"))
				Expect(addresses).To(ContainSubstring("DB3.X0.1"))
				Expect(addresses).To(ContainSubstring("DB3.X0.2"))
			})

			It("should render template with Modbus slave IDs array using range", func() {
				type ModbusConfig struct {
					Outputs []map[string]any `yaml:"outputs"`
				}

				template := ModbusConfig{
					Outputs: []map[string]any{
						{
							"label": "modbus_writer",
							"modbus_tcp": map[string]any{
								"slaves": "{{ range .SLAVE_IDS }}{{ . }}\n{{ end }}",
							},
						},
					},
				}

				scope := map[string]any{
					"SLAVE_IDS": []int{1, 2, 3, 4, 5},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Outputs).To(HaveLen(1))
				modbusConfig := result.Outputs[0]["modbus_tcp"].(map[string]any)
				slaves := modbusConfig["slaves"].(string)

				Expect(slaves).To(ContainSubstring("1"))
				Expect(slaves).To(ContainSubstring("2"))
				Expect(slaves).To(ContainSubstring("5"))
			})
		})

		Context("when using index to access array elements", func() {
			It("should render template accessing array elements by index", func() {
				type NetworkConfig struct {
					Primary   string `yaml:"primary"`
					Secondary string `yaml:"secondary"`
				}

				template := NetworkConfig{
					Primary:   "{{ index .DNS_SERVERS 0 }}",
					Secondary: "{{ index .DNS_SERVERS 1 }}",
				}

				scope := map[string]any{
					"DNS_SERVERS": []string{
						"8.8.8.8",
						"8.8.4.4",
					},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Primary).To(Equal("8.8.8.8"))
				Expect(result.Secondary).To(Equal("8.8.4.4"))
			})

			It("should render template with mixed array types", func() {
				type MixedConfig struct {
					Addresses []string         `yaml:"addresses"`
					Config    map[string]any   `yaml:"config"`
				}

				template := MixedConfig{
					Addresses: []string{
						"{{ index .IPS 0 }}:{{ index .PORTS 0 }}",
						"{{ index .IPS 1 }}:{{ index .PORTS 1 }}",
					},
					Config: map[string]any{
						"retry_count": "{{ index .RETRY_COUNTS 0 }}",
					},
				}

				scope := map[string]any{
					"IPS":          []string{"192.168.1.100", "192.168.1.101"},
					"PORTS":        []int{502, 503},
					"RETRY_COUNTS": []int{3, 5, 10},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Addresses).To(HaveLen(2))
				Expect(result.Addresses[0]).To(Equal("192.168.1.100:502"))
				Expect(result.Addresses[1]).To(Equal("192.168.1.101:503"))
				Expect(result.Config["retry_count"]).To(Equal("3"))
			})
		})

		Context("when combining range and index access", func() {
			It("should render complex template with both range iteration and index access", func() {
				type ComplexConfig struct {
					Protocol string           `yaml:"protocol"`
					Inputs   []map[string]any `yaml:"inputs"`
				}

				template := ComplexConfig{
					Protocol: "s7",
					Inputs: []map[string]any{
						{
							"label": "s7_primary",
							"s7": map[string]any{
								"address": "{{ index .ADDRESSES 0 }}",
								"rack":    "{{ .RACK }}",
								"slot":    "{{ .SLOT }}",
							},
						},
						{
							"label": "s7_all",
							"s7": map[string]any{
								"addresses": "{{ range .ADDRESSES }}{{ . }}\n{{ end }}",
							},
						},
					},
				}

				scope := map[string]any{
					"ADDRESSES": []string{
						"DB3.X0.0",
						"DB3.X0.1",
						"DB3.X0.2",
					},
					"RACK": 0,
					"SLOT": 1,
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Protocol).To(Equal("s7"))
				Expect(result.Inputs).To(HaveLen(2))

				primaryInput := result.Inputs[0]["s7"].(map[string]any)
				Expect(primaryInput["address"]).To(Equal("DB3.X0.0"))
				Expect(primaryInput["rack"]).To(Equal("0"))
				Expect(primaryInput["slot"]).To(Equal("1"))

				allInput := result.Inputs[1]["s7"].(map[string]any)
				addresses := allInput["addresses"].(string)
				Expect(addresses).To(ContainSubstring("DB3.X0.0"))
				Expect(addresses).To(ContainSubstring("DB3.X0.1"))
				Expect(addresses).To(ContainSubstring("DB3.X0.2"))
			})
		})

		Context("edge cases", func() {
			It("should handle empty array", func() {
				type EmptyArrayConfig struct {
					Items string `yaml:"items"`
				}

				template := EmptyArrayConfig{
					Items: "{{ range .ITEMS }}{{ . }}\n{{ end }}",
				}

				scope := map[string]any{
					"ITEMS": []string{},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Items).To(BeEmpty())
			})

			It("should handle single element array", func() {
				type SingleConfig struct {
					Address string `yaml:"address"`
				}

				template := SingleConfig{
					Address: "{{ range .ADDRESSES }}{{ . }}{{ end }}",
				}

				scope := map[string]any{
					"ADDRESSES": []string{"DB3.X0.0"},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Address).To(Equal("DB3.X0.0"))
			})
		})
	})
})
