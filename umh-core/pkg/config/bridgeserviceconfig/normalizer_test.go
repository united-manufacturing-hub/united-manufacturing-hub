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

package bridgeserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var _ = Describe("Bridge YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := ConfigSpec{}
			normalizer := NewNormalizer()

			config = normalizer.NormalizeConfig(config)

			Expect(config.Config.DFCReadConfig.BenthosConfig).NotTo(BeNil())
			Expect(config.Config.DFCReadConfig.BenthosConfig.Output).NotTo(BeNil())
			Expect(config.Config.DFCReadConfig.BenthosConfig.Pipeline).NotTo(BeNil())
		})

		It("should preserve existing values", func() {
			config := ConfigSpec{
				Config: ConfigTemplate{
					ConnectionConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
					DFCReadConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
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
					},
				},
			}

			normalizer := NewNormalizer()
			config = normalizer.NormalizeConfig(config)

			// Check input preserved
			inputMqtt := config.Config.DFCReadConfig.BenthosConfig.Input["mqtt"].(map[string]any)
			Expect(inputMqtt["topic"]).To(Equal("test/topic"))

			// Check output preserved
			outputUns := config.Config.DFCReadConfig.BenthosConfig.Output["uns"].(map[string]any) // note that this is NOT kafka, but uns
			Expect(outputUns["bridged_by"]).To(Equal("{{ .internal.bridged_by }}"))

			// Check pipeline processors preserved
			processors := config.Config.DFCReadConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(1))
			processor := processors[0].(map[string]any)
			processorText := processor["text"].(map[string]any)
			Expect(processorText["operator"]).To(Equal("to_upper"))
		})

		It("should normalize maps by ensuring they're not nil", func() {
			config := ConfigSpec{
				Config: ConfigTemplate{
					DFCReadConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							// Input is nil
							// Output is nil
							Pipeline: map[string]any{}, // Empty but not nil
							// Buffer is nil
							// CacheResources is nil
							// RateLimitResources is nil
						},
					},
				},
			}

			normalizer := NewNormalizer()
			config = normalizer.NormalizeConfig(config)

			Expect(config.Config.DFCReadConfig.BenthosConfig.Input).NotTo(BeNil())
			Expect(config.Config.DFCReadConfig.BenthosConfig.Output).NotTo(BeNil())
			// processor subfield should exist in the pipeline field
			Expect(config.Config.DFCReadConfig.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(config.Config.DFCReadConfig.BenthosConfig.Pipeline["processors"]).To(BeEmpty())

			// Check write-side configuration
			Expect(config.Config.DFCWriteConfig.BenthosConfig).NotTo(BeNil())
			Expect(config.Config.DFCWriteConfig.BenthosConfig.Input).NotTo(BeNil())
			Expect(config.Config.DFCWriteConfig.BenthosConfig.Output).NotTo(BeNil())
			// processor subfield should exist in the pipeline field
			Expect(config.Config.DFCWriteConfig.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(config.Config.DFCWriteConfig.BenthosConfig.Pipeline["processors"]).To(BeEmpty())

			// Buffer should have the none buffer set
			Expect(config.Config.DFCReadConfig.BenthosConfig.Buffer).To(HaveKey("none"))

			// These should be empty
			Expect(config.Config.DFCReadConfig.BenthosConfig.CacheResources).To(BeEmpty())
			Expect(config.Config.DFCReadConfig.BenthosConfig.RateLimitResources).To(BeEmpty())
		})
	})

	// Test the package-level function
	Describe("NormalizeDataFlowComponentConfig package function", func() {
		It("should use the default normalizer", func() {
			config1 := ConfigSpec{}
			config2 := ConfigSpec{}

			// Use package-level function
			config1 = NormalizeConfig(config1)

			// Use normalizer directly
			normalizer := NewNormalizer()
			config2 = normalizer.NormalizeConfig(config2)

			// Results should be the same
			Expect(config1).To(Equal(config2))
		})
	})
})
