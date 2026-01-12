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

package benthosserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Benthos YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := BenthosServiceConfig{}
			normalizer := NewNormalizer()

			normalizedConfig := normalizer.NormalizeConfig(config)
			normalizedInput := normalizedConfig.Input
			normalizedOutput := normalizedConfig.Output
			normalizedPipeline := normalizedConfig.Pipeline
			normalizedMetricsPort := normalizedConfig.MetricsPort
			normalizedDebugLevel := normalizedConfig.DebugLevel

			Expect(normalizedInput).NotTo(BeNil())
			Expect(normalizedOutput).NotTo(BeNil())
			Expect(normalizedPipeline).NotTo(BeNil())
			Expect(normalizedMetricsPort).To(Equal(uint16(4195)))
			Expect(normalizedDebugLevel).To(BeFalse())
		})

		It("should preserve existing values", func() {
			config := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "test/topic",
					},
				},
				Output: map[string]interface{}{
					"kafka": map[string]interface{}{
						"topic": "test-output",
					},
				},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"text": map[string]interface{}{
								"operator": "to_upper",
							},
						},
					},
				},
				MetricsPort: uint16(8000),
				DebugLevel:  true,
			}

			normalizer := NewNormalizer()
			normalizedConfig := normalizer.NormalizeConfig(config)
			normalizedInput := normalizedConfig.Input
			normalizedOutput := normalizedConfig.Output
			normalizedPipeline := normalizedConfig.Pipeline
			normalizedMetricsPort := normalizedConfig.MetricsPort
			normalizedDebugLevel := normalizedConfig.DebugLevel

			Expect(normalizedMetricsPort).To(Equal(uint16(8000)))
			Expect(normalizedDebugLevel).To(BeTrue())

			// Check input preserved
			inputMqtt := normalizedInput["mqtt"].(map[string]interface{})
			Expect(inputMqtt["topic"]).To(Equal("test/topic"))

			// Check output preserved
			outputKafka := normalizedOutput["kafka"].(map[string]interface{})
			Expect(outputKafka["topic"]).To(Equal("test-output"))

			// Check pipeline processors preserved
			processors := normalizedPipeline["processors"].([]interface{})
			Expect(processors).To(HaveLen(1))
			processor := processors[0].(map[string]interface{})
			processorText := processor["text"].(map[string]interface{})
			Expect(processorText["operator"]).To(Equal("to_upper"))
		})

		It("should normalize maps by ensuring they're not nil", func() {
			config := BenthosServiceConfig{
				// Input is nil
				// Output is nil
				Pipeline: map[string]interface{}{}, // Empty but not nil
				// Buffer is nil
				// CacheResources is nil
				// RateLimitResources is nil
				MetricsPort: 4195,
				DebugLevel:  false,
			}

			normalizer := NewNormalizer()
			normalizedConfig := normalizer.NormalizeConfig(config)
			normalizedInput := normalizedConfig.Input
			normalizedOutput := normalizedConfig.Output
			normalizedPipeline := normalizedConfig.Pipeline
			normalizedBuffer := normalizedConfig.Buffer
			normalizedCacheResources := normalizedConfig.CacheResources
			normalizedRateLimitResources := normalizedConfig.RateLimitResources

			Expect(normalizedInput).NotTo(BeNil())
			Expect(normalizedOutput).NotTo(BeNil())
			Expect(normalizedPipeline).NotTo(BeNil())

			// Buffer should have the none buffer set
			Expect(normalizedBuffer).To(HaveKey("none"))

			// These should be empty
			Expect(normalizedCacheResources).To(BeEmpty())
			Expect(normalizedRateLimitResources).To(BeEmpty())
		})
	})

	// Test the package-level function
	Describe("NormalizeBenthosConfig package function", func() {
		It("should use the default normalizer", func() {
			config := BenthosServiceConfig{}

			// Use package-level function
			normalizedConfig1 := NormalizeBenthosConfig(config)

			// Use normalizer directly
			normalizer := NewNormalizer()
			normalizedConfig2 := normalizer.NormalizeConfig(config)

			// Results should be the same
			Expect(normalizedConfig1.MetricsPort).To(Equal(normalizedConfig2.MetricsPort))
			Expect(normalizedConfig1.DebugLevel).To(Equal(normalizedConfig2.DebugLevel))
			Expect(normalizedConfig1.Input).NotTo(BeNil())
			Expect(normalizedConfig1.Output).NotTo(BeNil())
		})
	})
})
