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
)

var _ = Describe("ProtocolConverter YAML Generator", func() {
	type testCase struct {
		config      *ProtocolConverterServiceConfigSpec
		expected    []string
		notExpected []string
	}

	DescribeTable("generator rendering",
		func(testCaseValue testCase) {
			generator := NewGenerator()
			yamlStr, err := generator.RenderConfig(*testCaseValue.config)
			Expect(err).NotTo(HaveOccurred())

			// Check for expected strings
			for _, exp := range testCaseValue.expected {
				Expect(yamlStr).To(ContainSubstring(exp))
			}

			// Check for strings that should not be present
			for _, notExp := range testCaseValue.notExpected {
				Expect(yamlStr).NotTo(ContainSubstring(notExp))
			}
		},
		Entry("should render empty stdout output correctly",
			testCase{
				config: &ProtocolConverterServiceConfigSpec{
					Config: ProtocolConverterServiceConfigTemplate{
						ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
							NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
								Target: "127.0.0.1",
								Port:   "443",
							},
						},
						DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Output: map[string]any{
									"stdout": map[string]any{},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Input: map[string]any{
									"stdin": map[string]any{},
								},
								Output: map[string]any{
									"kafka": map[string]any{
										"addresses": []string{"kafka:9092"},
										"topic":     "output-topic",
									},
								},
							},
						},
					},
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
					"  benthos:",
					"    input:",
					"      stdin: {}",
					"    output:",
					"      kafka:",
					"        addresses:",
					"        - kafka:9092",
					"        topic: output-topic",
					"    http:",
					"      address: 0.0.0.0:0",
					"    logger:",
					"      level: INFO",
				},
				notExpected: []string{},
			}),
		Entry("should render configured output correctly",
			testCase{
				config: &ProtocolConverterServiceConfigSpec{
					Config: ProtocolConverterServiceConfigTemplate{
						ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
							NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
								Target: "127.0.0.1",
								Port:   "443",
							},
						},
						DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Output: map[string]any{
									"stdout": map[string]any{
										"codec": "lines",
									},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Input: map[string]any{
									"http_server": map[string]any{
										"path": "/data",
									},
								},
								Output: map[string]any{
									"http_client": map[string]any{
										"url": "http://api.example.com/ingest",
									},
								},
							},
						},
					},
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
					"  benthos:",
					"    input:",
					"      http_server:",
					"        path: /data",
					"    output:",
					"      http_client:",
					"        url: http://api.example.com/ingest",
				},
				notExpected: []string{},
			}),
		Entry("should render empty input correctly",
			testCase{
				config: &ProtocolConverterServiceConfigSpec{
					Config: ProtocolConverterServiceConfigTemplate{
						ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
							NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
								Target: "127.0.0.1",
								Port:   "443",
							},
						},
						DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Input: map[string]any{},
								Output: map[string]any{
									"stdout": map[string]any{},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Input: map[string]any{
									"file": map[string]any{
										"paths": []string{"/tmp/input.txt"},
									},
								},
								Output: map[string]any{
									"file": map[string]any{
										"path": "/tmp/output.txt",
									},
								},
							},
						},
					},
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
					"  benthos:",
					"    input:",
					"      file:",
					"        paths:",
					"        - /tmp/input.txt",
					"    output:",
					"      file:",
					"        path: /tmp/output.txt",
				},
				notExpected: []string{
					"input: {}", // Empty input should not render as an empty object
				},
			}),
		Entry("should render Kafka input with processor and AWS S3 output",
			testCase{
				config: &ProtocolConverterServiceConfigSpec{
					Config: ProtocolConverterServiceConfigTemplate{
						ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
							NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
								Target: "127.0.0.1",
								Port:   "443",
							},
						},
						DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Input: map[string]any{
									"mqtt": map[string]any{
										"urls":   []string{"tcp://mqtt-broker:1883"},
										"topics": []string{"sensor/data"},
									},
								},
								Pipeline: map[string]any{
									"processors": []any{
										map[string]any{
											"json": map[string]any{
												"operator": "select",
												"path":     "payload",
											},
										},
									},
								},
								Output: map[string]any{
									"redis_pubsub": map[string]any{
										"url":     "redis://redis:6379",
										"channel": "processed_data",
									},
								},
							},
						},
					},
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
					"  benthos:",
					"    input:",
					"      mqtt:",
					"        urls:",
					"        - tcp://mqtt-broker:1883",
					"        topics:",
					"        - sensor/data",
					"    pipeline:",
					"      processors:",
					"      - json:",
					"          operator: select",
					"          path: payload",
					"    output:",
					"      redis_pubsub:",
					"        url: redis://redis:6379",
					"        channel: processed_data",
				},
				notExpected: []string{},
			}),
	)
})
