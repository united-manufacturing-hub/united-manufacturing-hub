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
							Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.enterprise.site.*"},
							Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
								Protocol: "kafka",
								Code:     "addresses:\n- kafka:9092\ntopic: output-topic\n",
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
					"  source:",
					"    topics: umh.v1.enterprise.site.*",
					"  destination:",
					"    protocol: kafka",
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
							Processing: dataflowcomponentserviceconfig.WriteConfigProcessing{
								Type: "nodered_js",
								Code: "msg.payload.foo = 'bar';\nreturn msg;",
							},
							Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
								Protocol: "http_client",
								Code:     "url: http://api.example.com/ingest\n",
							},
						},
					},
				},
				expected: []string{
					"dataflowcomponent_write:",
					"  processing:",
					"    type: nodered_js",
					"  destination:",
					"    protocol: http_client",
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
							Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
								Protocol: "redis_pubsub",
								Code:     "url: redis://redis:6379\nchannel: processed_data\n",
							},
							Extra: &dataflowcomponentserviceconfig.WriteConfigExtra{
								Code: "buffer:\n  none: {}",
							},
						},
					},
				},
				expected: []string{
					"dataflowcomponent_write:",
					"  destination:",
					"    protocol: redis_pubsub",
					"  buffer:",
					"    none: {}",
				},
				notExpected: []string{},
			}),
	)
})
