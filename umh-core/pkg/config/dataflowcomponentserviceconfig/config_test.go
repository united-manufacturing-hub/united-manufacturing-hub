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

package dataflowcomponentserviceconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

var _ = Describe("DataFlowComponentConfig", func() {
	Context("Conversion between BenthosConfig and BenthosServiceConfig", func() {
		It("should convert BenthosConfig to BenthosServiceConfig with default advanced settings", func() {
			// Create a simple BenthosConfig
			benthos := dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"kafka": map[string]interface{}{
						"addresses": []string{"localhost:9092"},
						"topics":    []string{"test-topic"},
					},
				},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"log": map[string]interface{}{
								"message": "Processing message",
							},
						},
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			}

			// Convert to BenthosServiceConfig
			fullConfig := benthos.ToBenthosServiceConfig()

			// Verify conversion
			Expect(fullConfig.Input).To(Equal(benthos.Input))
			Expect(fullConfig.Pipeline).To(Equal(benthos.Pipeline))
			Expect(fullConfig.Output).To(Equal(benthos.Output))
			Expect(fullConfig.MetricsPort).To(Equal(uint16(0)))                     // Default value
			Expect(fullConfig.LogLevel).To(Equal(constants.DefaultBenthosLogLevel)) // Default value
		})

		It("should convert BenthosServiceConfig to BenthosConfig, ignoring advanced fields", func() {
			// Create a full BenthosServiceConfig
			fullConfig := benthosserviceconfig.BenthosServiceConfig{
				Input: map[string]interface{}{
					"kafka": map[string]interface{}{
						"addresses": []string{"localhost:9092"},
						"topics":    []string{"test-topic"},
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
				MetricsPort: 8080,    // This should be ignored in conversion
				LogLevel:    "DEBUG", // This should be ignored in conversion
			}

			// Convert to simplified BenthosConfig
			simplified := dataflowcomponentserviceconfig.FromBenthosServiceConfig(fullConfig)

			// Verify conversion
			Expect(simplified.BenthosConfig.Input).To(Equal(fullConfig.Input))
			Expect(simplified.BenthosConfig.Output).To(Equal(fullConfig.Output))

			// Convert back to BenthosServiceConfig
			convertedBack := simplified.GetBenthosServiceConfig()

			// Verify advanced fields use defaults, not original values
			Expect(convertedBack.MetricsPort).To(Equal(uint16(0)))                     // Default, not 8080
			Expect(convertedBack.LogLevel).To(Equal(constants.DefaultBenthosLogLevel)) // Default, not DEBUG
		})
	})

	Context("Utility functions", func() {
		It("should correctly compare identical configs", func() {
			// Create two identical configs
			configA := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"test-topic"},
						},
					},
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
				},
			}

			configB := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"test-topic"},
						},
					},
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
				},
			}

			Expect(dataflowcomponentserviceconfig.ConfigsEqual(configA, configB)).To(BeTrue())
		})

		It("should correctly compare different configs", func() {
			// Create two different configs
			configA := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"test-topic"},
						},
					},
				},
			}

			configB := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"different-topic"},
						},
					},
				},
			}

			Expect(dataflowcomponentserviceconfig.ConfigsEqual(configA, configB)).To(BeFalse())
			diff := dataflowcomponentserviceconfig.ConfigDiff(configA, configB)
			Expect(diff).To(ContainSubstring("Input config differences"))
		})

		It("should normalize config correctly", func() {
			config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"test-topic"},
						},
					},
					// Output is intentionally missing to test normalization
				},
			}

			// Normalize the config
			normalizer := dataflowcomponentserviceconfig.NewNormalizer()
			config = normalizer.NormalizeConfig(config)

			// Output and pipeline should have been added
			Expect(config.BenthosConfig.Output).NotTo(BeNil())
			Expect(config.BenthosConfig.Pipeline).NotTo(BeNil())
		})

		It("should generate valid YAML", func() {
			config := &dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topics":    []string{"test-topic"},
						},
					},
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
				},
			}

			yaml, err := dataflowcomponentserviceconfig.RenderDataFlowComponentYAML(config.GetBenthosServiceConfig())
			Expect(err).NotTo(HaveOccurred())
			Expect(string(yaml)).To(ContainSubstring("kafka"))
			Expect(string(yaml)).To(ContainSubstring("test-topic"))
			Expect(string(yaml)).To(ContainSubstring("stdout"))
		})
	})
})
