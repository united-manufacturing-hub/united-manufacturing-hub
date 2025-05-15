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

package protocolconverterserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
)

var _ = Describe("ProtocolConverter YAML Generator", func() {
	type testCase struct {
		config      *ProtocolConverterServiceConfig
		expected    []string
		notExpected []string
	}

	DescribeTable("generator rendering",
		func(tc testCase) {
			generator := NewGenerator()
			yamlStr, err := generator.RenderConfig(*tc.config)
			Expect(err).NotTo(HaveOccurred())

			// Check for expected strings
			for _, exp := range tc.expected {
				Expect(yamlStr).To(ContainSubstring(exp))
			}

			// Check for strings that should not be present
			for _, notExp := range tc.notExpected {
				Expect(yamlStr).NotTo(ContainSubstring(notExp))
			}
		},
		Entry("should render empty stdout output correctly",
			testCase{
				config: &ProtocolConverterServiceConfig{
					connectionserviceconfig.ConnectionServiceConfig{
						NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
							Target: "127.0.0.1",
							Port:   443,
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, //TODO: Add write DFC
				},
				// NOTE: We expect port 0 here, since ot comes out of ToBenthosServiceConfig()
				expected: []string{
					"connection:",
					"  target: 127.0.0.1",
					"  port: 443",
					"dataflowcomponent_read:",
					"  benthos:",
					"    output:",
					"      stdout: {}",
					"    http:",
					"      address: 0.0.0.0:0",
					"    logger:",
					"      level: INFO",
					"dataflowcomponent_write:",
					// TODO: Add write DFC
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render configured output correctly",
			testCase{
				config: &ProtocolConverterServiceConfig{
					connectionserviceconfig.ConnectionServiceConfig{
						NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
							Target: "127.0.0.1",
							Port:   443,
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Output: map[string]any{
								"stdout": map[string]any{
									"codec": "lines",
								},
							},
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, //TODO: Add write DFC
				},
				expected: []string{
					"connection:",
					"  target: 127.0.0.1",
					"  port: 443",
					"dataflowcomponent_read:",
					"  benthos:",
					"    output:",
					"      stdout:",
					"        codec: lines",
					"dataflowcomponent_write:",
					// TODO: Add write DFC
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render empty input correctly",
			testCase{
				config: &ProtocolConverterServiceConfig{
					connectionserviceconfig.ConnectionServiceConfig{
						NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
							Target: "127.0.0.1",
							Port:   443,
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, //TODO: Add write DFC
				},
				expected: []string{
					"connection:",
					"  target: 127.0.0.1",
					"  port: 443",
					"dataflowcomponent_read:",
					"  benthos:",
					"    output:",
					"      stdout: {}",
					"dataflowcomponent_write:",
					// TODO: Add write DFC
				},
				notExpected: []string{
					"input: {}", // Empty input should not render as an empty object
				},
			}),
		Entry("should render Kafka input with processor and AWS S3 output",
			testCase{
				config: &ProtocolConverterServiceConfig{
					connectionserviceconfig.ConnectionServiceConfig{
						NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
							Target: "127.0.0.1",
							Port:   443,
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"kafka": map[string]any{
									"addresses": []string{"kafka:9092"},
									"topics":    []string{"benthos_topic"},
									"consumer_group": map[string]any{
										"session_timeout": "6s",
									},
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"text": map[string]any{
											"operator": "to_upper",
										},
									},
								},
							},
							Output: map[string]any{
								"aws_s3": map[string]any{
									"bucket": "example-bucket",
									"path":   "${!count:files}-${!timestamp_unix_nano}.txt",
								},
							},
						},
					},
					dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, //TODO: Add write DFC
				},
				expected: []string{
					"connection:",
					"  target: 127.0.0.1",
					"  port: 443",
					"dataflowcomponent_read:",
					"  benthos:",
					"    input:",
					"      kafka:",
					"        addresses:",
					"        - kafka:9092",
					"        topics:",
					"        - benthos_topic",
					"        consumer_group:",
					"          session_timeout: 6s",
					"    pipeline:",
					"      processors:",
					"      - text:",
					"          operator: to_upper",
					"    output:",
					"      aws_s3:",
					"        bucket: example-bucket",
					"        path: ${!count:files}-${!timestamp_unix_nano}.txt",
					"dataflowcomponent_write:",
					// TODO: Add write DFC
				},
				notExpected: []string{
					"stdin",
				},
			}),
	)

	// Add package-level function test
	Describe("RenderProtocolConverterYAML package function", func() {
		It("should produce the same output as the Generator", func() {
			// Setup test configs
			connectionConfig := connectionserviceconfig.ConnectionServiceConfig{
				NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
					Target: "127.0.0.1",
					Port:   443,
				},
			}

			dfcReadConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]any{
						"kafka": map[string]any{
							"addresses": []string{"kafka:9092"},
							"topics":    []string{"test_topic"},
						},
					},
					Output: map[string]any{
						"kafka": map[string]any{
							"addresses": []string{"kafka:9092"},
							"topic":     "output_topic",
						},
					},
				},
			}

			dfcWriteConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}

			// Use package-level function
			yamlStr1, err := RenderProtocolConverterYAML(connectionConfig, dfcReadConfig, dfcWriteConfig)
			Expect(err).NotTo(HaveOccurred())

			// Use Generator directly
			cfg := ProtocolConverterServiceConfig{
				ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfig{
					NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
						Target: "127.0.0.1",
						Port:   443,
					},
				},
				DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]any{
							"kafka": map[string]any{
								"addresses": []string{"kafka:9092"},
								"topics":    []string{"test_topic"},
							},
						},
						Output: map[string]any{
							"kafka": map[string]any{
								"addresses": []string{"kafka:9092"},
								"topic":     "output_topic",
							},
						},
					},
				},
				DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			}
			generator := NewGenerator()
			yamlStr2, err := generator.RenderConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Both should produce the same result
			Expect(yamlStr1).To(Equal(yamlStr2))
		})
	})
})
