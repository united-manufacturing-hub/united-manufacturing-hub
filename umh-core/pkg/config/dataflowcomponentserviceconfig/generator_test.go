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

package dataflowcomponentserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
)

var _ = Describe("DFC YAML Generator", func() {
	type testCase struct {
		config      *DataflowComponentServiceConfig
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
				config: &DataflowComponentServiceConfig{
					BenthosConfig{
						Output: map[string]any{
							"stdout": map[string]any{},
						},
					},
				},
				// NOTE: We expect port 0 here, since ot comes out of ToBenthosServiceConfig()
				expected: []string{
					"benthos:",
					"  output:",
					"    stdout: {}",
					"  http:",
					"    address: 0.0.0.0:0",
					"  logger:",
					"    level: INFO",
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render configured output correctly",
			testCase{
				config: &DataflowComponentServiceConfig{
					BenthosConfig{
						Output: map[string]any{
							"stdout": map[string]any{
								"codec": "lines",
							},
						},
					},
				},
				expected: []string{
					"benthos:",
					"  output:",
					"    stdout:",
					"      codec: lines",
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render empty input correctly",
			testCase{
				config: &DataflowComponentServiceConfig{
					BenthosConfig{
						Input: map[string]any{},
						Output: map[string]any{
							"stdout": map[string]any{},
						},
					},
				},
				expected: []string{
					"benthos:",
					"  output:",
					"    stdout: {}",
				},
				notExpected: []string{
					"input: {}", // Empty input should not render as an empty object
				},
			}),
		Entry("should render Kafka input with processor and AWS S3 output",
			testCase{
				config: &DataflowComponentServiceConfig{
					BenthosConfig{
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
				expected: []string{
					"benthos:",
					"  input:",
					"    kafka:",
					"      addresses:",
					"      - kafka:9092",
					"      topics:",
					"      - benthos_topic",
					"      consumer_group:",
					"        session_timeout: 6s",
					"  pipeline:",
					"    processors:",
					"    - text:",
					"        operator: to_upper",
					"  output:",
					"    aws_s3:",
					"      bucket: example-bucket",
					"      path: ${!count:files}-${!timestamp_unix_nano}.txt",
				},
				notExpected: []string{
					"stdin",
				},
			}),
	)

	// Add package-level function test
	Describe("RenderDataFlowComponentYAML package function", func() {
		It("should produce the same output as the Generator", func() {
			// Setup test config
			benthosConfig := benthosserviceconfig.BenthosServiceConfig{
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
			}

			// Use package-level function
			yamlStr1, err := RenderDataFlowComponentYAML(benthosConfig)
			Expect(err).NotTo(HaveOccurred())

			// Use Generator directly
			cfg := DataflowComponentServiceConfig{
				BenthosConfig{
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
			generator := NewGenerator()
			yamlStr2, err := generator.RenderConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Both should produce the same result
			Expect(yamlStr1).To(Equal(yamlStr2))
		})
	})
})
