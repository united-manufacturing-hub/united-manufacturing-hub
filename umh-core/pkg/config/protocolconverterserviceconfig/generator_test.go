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
		Entry("should render write DFC with output and topics",
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
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
							InputTopics: "umh.v1.enterprise.site.*",
							Output: map[string]any{
								"kafka": map[string]any{
									"addresses": []string{"kafka:9092"},
									"topic":     "output-topic",
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
					"  input_topics: umh.v1.enterprise.site.*",
					"  output:",
					"    kafka:",
					"      addresses:",
					"      - kafka:9092",
					"      topic: output-topic",
				},
				notExpected: []string{},
			}),
		Entry("should render write DFC with processing code",
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
									"stdout": map[string]any{"codec": "lines"},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
							ProcessingNoderedJS: "msg.payload.foo = 'bar';\nreturn msg;",
							Output: map[string]any{
								"http_client": map[string]any{
									"url": "http://api.example.com/ingest",
								},
							},
						},
					},
				},
				expected: []string{
					"dataflowcomponent_write:",
					"  processing_nodered_js:",
					"  output:",
					"    http_client:",
					"      url: http://api.example.com/ingest",
				},
				notExpected: []string{},
			}),
		Entry("should render empty write DFC",
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
								Input:  map[string]any{},
								Output: map[string]any{"stdout": map[string]any{}},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{},
					},
				},
				expected: []string{
					"connection:",
					"dataflowcomponent_read:",
					"  benthos:",
					"    output:",
					"      stdout: {}",
				},
				notExpected: []string{},
			}),
		Entry("should render write DFC with buffer",
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
									},
								},
								Output: map[string]any{
									"aws_s3": map[string]any{
										"bucket": "example-bucket",
									},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
							Output: map[string]any{
								"redis_pubsub": map[string]any{
									"url":     "redis://redis:6379",
									"channel": "processed_data",
								},
							},
							Buffer: map[string]any{"none": map[string]any{}},
						},
					},
				},
				expected: []string{
					"dataflowcomponent_write:",
					"  output:",
					"    redis_pubsub:",
					"      url: redis://redis:6379",
					"      channel: processed_data",
					"  buffer:",
					"    none: {}",
				},
				notExpected: []string{},
			}),
	)
})
