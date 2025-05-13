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
)

var _ = Describe("DFC YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := DataflowComponentServiceConfig{}
			normalizer := NewNormalizer()

			config = normalizer.NormalizeConfig(config)

			Expect(config.BenthosConfig).NotTo(BeNil())
			Expect(config.BenthosConfig.Output).NotTo(BeNil())
			Expect(config.BenthosConfig.Pipeline).NotTo(BeNil())
		})

		It("should preserve existing values", func() {
			config := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
					Output: map[string]any{
						"kafka": map[string]any{
							"topic": "test-output",
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
				},
			}

			normalizer := NewNormalizer()
			config = normalizer.NormalizeConfig(config)

			// Check input preserved
			inputMqtt := config.BenthosConfig.Input["mqtt"].(map[string]any)
			Expect(inputMqtt["topic"]).To(Equal("test/topic"))

			// Check output preserved
			outputKafka := config.BenthosConfig.Output["kafka"].(map[string]any)
			Expect(outputKafka["topic"]).To(Equal("test-output"))

			// Check pipeline processors preserved
			processors := config.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(1))
			processor := processors[0].(map[string]any)
			processorText := processor["text"].(map[string]any)
			Expect(processorText["operator"]).To(Equal("to_upper"))
		})

		It("should normalize maps by ensuring they're not nil", func() {
			config := DataflowComponentServiceConfig{
				BenthosConfig{
					// Input is nil
					// Output is nil
					Pipeline: map[string]any{}, // Empty but not nil
					// Buffer is nil
					// CacheResources is nil
					// RateLimitResources is nil
				},
			}

			normalizer := NewNormalizer()
			config = normalizer.NormalizeConfig(config)

			Expect(config.BenthosConfig.Input).NotTo(BeNil())
			Expect(config.BenthosConfig.Output).NotTo(BeNil())
			Expect(config.BenthosConfig.Pipeline).NotTo(BeNil())

			// Buffer should have the none buffer set
			Expect(config.BenthosConfig.Buffer).To(HaveKey("none"))

			// These should be empty
			Expect(config.BenthosConfig.CacheResources).To(BeEmpty())
			Expect(config.BenthosConfig.RateLimitResources).To(BeEmpty())
		})
	})

	// Test the package-level function
	Describe("NormalizeDataFlowComponentConfig package function", func() {
		It("should use the default normalizer", func() {
			config1 := DataflowComponentServiceConfig{}
			config2 := DataflowComponentServiceConfig{}

			// Use package-level function
			NormalizeDataFlowComponentConfig(config1)

			// Use normalizer directly
			normalizer := NewNormalizer()
			normalizer.NormalizeConfig(config2)

			// Results should be the same
			Expect(config1.BenthosConfig).To(Equal(config2.BenthosConfig))
		})
	})
})
