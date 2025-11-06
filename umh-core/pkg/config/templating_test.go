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
							"s7comm": map[string]any{
								"tcpDevice":  "192.168.0.1",
								"rack":       0,
								"slot":       1,
								"addresses":  []any{"{{ range .ADDRESSES }}{{ . }}\n{{ end }}"},
								"batchMaxSize": 480,
								"timeout":    10,
							},
						},
					},
				}

				scope := map[string]any{
					"ADDRESSES": []string{"DB1.DW20", "DB1.S30.10", "DB3.X0.0"},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Inputs).To(HaveLen(1))

				addressesSlice := result.Inputs[0]["s7comm"].(map[string]any)["addresses"].([]any)
				addressesStr := addressesSlice[0].(string)
				Expect(addressesStr).To(ContainSubstring("DB1.DW20"))
				Expect(addressesStr).To(ContainSubstring("DB1.S30.10"))
				Expect(addressesStr).To(ContainSubstring("DB3.X0.0"))
			})

			It("should render template with Modbus slave IDs array using range", func() {
				type ModbusConfig struct {
					Inputs []map[string]any `yaml:"inputs"`
				}

				template := ModbusConfig{
					Inputs: []map[string]any{
						{
							"label": "modbus_reader",
							"modbus": map[string]any{
								"controller":       "tcp://localhost:502",
								"transmissionMode": "TCP",
								"slaveIDs":         "{{ range .SLAVE_IDS }}{{ . }}\n{{ end }}",
								"timeout":          "1s",
								"addresses": []map[string]any{
									{
										"name":     "firstFlagOfDiscreteInput",
										"register": "discrete",
										"address":  1,
										"type":     "BIT",
										"output":   "BOOL",
									},
								},
							},
						},
					},
				}

				scope := map[string]any{
					"SLAVE_IDS": []int{1, 2, 3, 4, 5},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Inputs).To(HaveLen(1))

				slaveIDs := result.Inputs[0]["modbus"].(map[string]any)["slaveIDs"].(string)
				Expect(slaveIDs).To(ContainSubstring("1"))
				Expect(slaveIDs).To(ContainSubstring("2"))
				Expect(slaveIDs).To(ContainSubstring("5"))
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
					"DNS_SERVERS": []string{"8.8.8.8", "8.8.4.4"},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Primary).To(Equal("8.8.8.8"))
				Expect(result.Secondary).To(Equal("8.8.4.4"))
			})

			It("should render template with mixed array types", func() {
				type MixedConfig struct {
					Addresses []string       `yaml:"addresses"`
					Config    map[string]any `yaml:"config"`
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
					Protocol: "s7comm",
					Inputs: []map[string]any{
						{
							"label": "s7_primary",
							"s7comm": map[string]any{
								"tcpDevice": "192.168.0.1",
								"rack":      "{{ .RACK }}",
								"slot":      "{{ .SLOT }}",
								"addresses": []any{"{{ index .ADDRESSES 0 }}"},
							},
						},
						{
							"label": "s7_all",
							"s7comm": map[string]any{
								"tcpDevice": "192.168.0.1",
								"rack":      0,
								"slot":      1,
								"addresses": []any{"{{ range .ADDRESSES }}{{ . }}\n{{ end }}"},
							},
						},
					},
				}

				scope := map[string]any{
					"ADDRESSES": []string{"DB1.DW20", "DB1.S30.10", "DB3.X0.0"},
					"RACK":      0,
					"SLOT":      1,
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Protocol).To(Equal("s7comm"))
				Expect(result.Inputs).To(HaveLen(2))

				primaryInput := result.Inputs[0]["s7comm"].(map[string]any)
				primaryAddresses := primaryInput["addresses"].([]any)
				Expect(primaryAddresses[0]).To(Equal("DB1.DW20"))
				Expect(primaryInput["rack"]).To(Equal("0"))
				Expect(primaryInput["slot"]).To(Equal("1"))

				allInput := result.Inputs[1]["s7comm"].(map[string]any)
				allAddresses := allInput["addresses"].([]any)
				allAddressesStr := allAddresses[0].(string)
				Expect(allAddressesStr).To(ContainSubstring("DB1.DW20"))
				Expect(allAddressesStr).To(ContainSubstring("DB1.S30.10"))
				Expect(allAddressesStr).To(ContainSubstring("DB3.X0.0"))
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
					"ADDRESSES": []string{"DB1.DW20"},
				}

				result, err := RenderTemplate(template, scope)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Address).To(Equal("DB1.DW20"))
			})
		})
	})
})
