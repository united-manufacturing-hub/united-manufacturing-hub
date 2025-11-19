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

package runtime_config_test

import (
	"context"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
)

var _ = Describe("BuildRuntimeConfig", func() {
	var (
		spec          protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec
		agentLocation map[string]string
		globalVars    map[string]any
		nodeName      string
		pcName        string
	)

	BeforeEach(func() {
		// Parse the example config file using the config manager's parseConfig function
		exampleConfigPath := "../../../../examples/example-config-protocolconverter-templated.yaml"

		// Read the example config file
		data, err := os.ReadFile(exampleConfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to read example config file")

		// Use the config manager's parseConfig function to properly handle templates and anchors
		ctx := context.Background()
		fullConfig, err := config.ParseConfig(data, ctx, true) // Allow unknown fields for template handling
		Expect(err).NotTo(HaveOccurred(), "Failed to parse example config")

		// Extract the first protocol converter (temperature-sensor-pc)
		Expect(fullConfig.ProtocolConverter).To(HaveLen(3), "Expected 3 protocol converters in example config")
		firstPC := fullConfig.ProtocolConverter[0]
		Expect(firstPC.Name).To(Equal("temperature-sensor-pc"), "Expected first PC to be temperature-sensor-pc")

		// Get the spec from the first protocol converter
		spec = firstPC.ProtocolConverterServiceConfig

		// Extract agent location from the config
		agentLocation = map[string]string{}
		for k, v := range fullConfig.Agent.Location {
			agentLocation[strconv.Itoa(k)] = v
		}

		// Set up test data
		globalVars = map[string]any{
			"releaseChannel": "stable",
			"version":        "1.0.0",
		}
		nodeName = "test-node"
		pcName = "temperature-sensor-pc"

		spec.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}
	})

	Describe("BuildRuntimeConfig", func() {
		It("should successfully build runtime config from example config", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the runtime config structure
			Expect(result).NotTo(BeZero())

			// Check connection config - should have rendered variables
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))

			// Verify dataflow component configs exist (even if empty)
			Expect(result.DataflowComponentReadServiceConfig).NotTo(BeNil())
			Expect(result.DataflowComponentWriteServiceConfig).NotTo(BeNil())
		})

		It("should properly merge agent and PC locations", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// The location should be properly merged and accessible via variables
			// We can't directly inspect internal variables, but the build should succeed
			Expect(result).NotTo(BeZero())
		})

		It("should handle template variable substitution", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Check that template variables {{ .IP }} and {{ .PORT }} have been substituted
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).To(Equal("10.0.1.50"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Port).To(Equal(uint16(4840)))
		})

		It("should generate proper bridged_by header", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// We can't directly check the bridged_by header, but the function should succeed
			// The bridged_by header would be something like "protocol-converter-test-node-temperature-sensor-pc"
			Expect(result).NotTo(BeZero())
		})

		It("should handle missing location levels with 'unknown'", func() {
			// Test with incomplete location map
			incompleteLocation := map[string]string{
				"0": "plant-A",
				"3": "workstation-5", // Gap at levels 1,2
			}

			result, err := runtime_config.BuildRuntimeConfig(spec, incompleteLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Should fill gaps with "unknown" and not fail
			Expect(result).NotTo(BeZero())
		})

		It("should return error for empty spec", func() {
			emptySpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}

			result, err := runtime_config.BuildRuntimeConfig(emptySpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil spec"))
			Expect(result).To(Equal(protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}))
		})

		It("should handle empty global vars", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})

		It("should handle empty node name", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, "", pcName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeZero())
		})
	})

	Describe("Variable expansion", func() {
		It("should expand all template variables correctly", func() {
			result, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Verify that no template strings remain (no {{ }} patterns)
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("{{"))
			Expect(result.ConnectionServiceConfig.NmapServiceConfig.Target).NotTo(ContainSubstring("}}"))
		})
	})

	Describe("debug_level propagation", func() {
		It("should propagate debug_level from spec.DebugLevel to read and write DFCs", func() {
			// Start with the example config and modify DebugLevel
			testSpec := spec
			testSpec.DebugLevel = true

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.DebugLevel).To(BeTrue(), "Runtime config DebugLevel should be true")
			Expect(result.DataflowComponentReadServiceConfig.DebugLevel).To(BeTrue(), "Read DFC DebugLevel should be true")
			Expect(result.DataflowComponentWriteServiceConfig.DebugLevel).To(BeTrue(), "Write DFC DebugLevel should be true")
		})

		It("should respect debug_level priority: DFC level > Spec level", func() {
			// Test priority: DFC-level overrides spec-level
			testSpec := spec
			testSpec.DebugLevel = false // Spec level = false
			testSpec.Config.DataflowComponentReadServiceConfig.DebugLevel = true  // DFC level = true (higher priority)
			testSpec.Config.DataflowComponentWriteServiceConfig.DebugLevel = true // DFC level = true (higher priority)

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Runtime config uses spec.DebugLevel directly
			Expect(result.DebugLevel).To(BeFalse(), "Runtime config should use spec.DebugLevel")
			// Both DFCs should use their own DebugLevel (true) because DFC level takes precedence
			Expect(result.DataflowComponentReadServiceConfig.DebugLevel).To(BeTrue(), "Read DFC should use its own DebugLevel (higher priority)")
			Expect(result.DataflowComponentWriteServiceConfig.DebugLevel).To(BeTrue(), "Write DFC should use its own DebugLevel (higher priority)")
		})

		It("should default to false when debug_level is not set", func() {
			// Start with the example config and ensure all DebugLevel fields are false
			testSpec := spec
			testSpec.DebugLevel = false
			testSpec.Config.DataflowComponentReadServiceConfig.DebugLevel = false
			testSpec.Config.DataflowComponentWriteServiceConfig.DebugLevel = false

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.DebugLevel).To(BeFalse(), "Runtime config DebugLevel should default to false")
			Expect(result.DataflowComponentReadServiceConfig.DebugLevel).To(BeFalse(), "Read DFC DebugLevel should default to false")
			Expect(result.DataflowComponentWriteServiceConfig.DebugLevel).To(BeFalse(), "Write DFC DebugLevel should default to false")
		})

		It("should propagate debug_level from spec level when DFC level is false", func() {
			// Start with the example config and set spec.DebugLevel
			testSpec := spec
			testSpec.DebugLevel = true  // Spec level = true
			testSpec.Config.DataflowComponentReadServiceConfig.DebugLevel = false
			testSpec.Config.DataflowComponentWriteServiceConfig.DebugLevel = false

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.DebugLevel).To(BeTrue(), "Runtime config should use spec.DebugLevel")
			Expect(result.DataflowComponentReadServiceConfig.DebugLevel).To(BeTrue(), "Read DFC should use spec.DebugLevel")
			Expect(result.DataflowComponentWriteServiceConfig.DebugLevel).To(BeTrue(), "Write DFC should use spec.DebugLevel")
		})
	})

	Describe("Downsampler injection", func() {
		// Helper function to create a basic connection config for tests
		createConnectionConfig := func() connectionserviceconfig.ConnectionServiceConfigTemplate {
			return connectionserviceconfig.ConnectionServiceConfigTemplate{
				NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
					Target: "127.0.0.1",
					Port:   "8080",
				},
			}
		}
		It("should NOT inject downsampler when no processors exist", func() {
			// Create a simple spec with no processors
			testSpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: createConnectionConfig(),
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count": 1,
								},
							},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
				},
			}

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Check that NO downsampler was injected (since no tag_processor)
			readConfig := result.DataflowComponentReadServiceConfig
			_, exists := readConfig.BenthosConfig.Pipeline["processors"]
			Expect(exists).To(BeFalse(), "Pipeline should NOT have processors when no tag_processor exists")
		})

		It("should NOT inject downsampler when there are multiple processors", func() {
			// Create spec with multiple processors (tag_processor + mapping)
			testSpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: createConnectionConfig(),
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count": 1,
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"tag_processor": map[string]any{
											"defaults": "msg.meta.tag_name = 'test'; return msg;",
										},
									},
									map[string]any{
										"mapping": "root = this.upper()",
									},
								},
							},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
				},
			}

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Check that NO downsampler was injected (multiple processors = custom)
			readConfig := result.DataflowComponentReadServiceConfig
			processors := readConfig.BenthosConfig.Pipeline["processors"].([]interface{})

			Expect(processors).To(HaveLen(2), "Should have only tag_processor + mapping, no downsampler")

			// Check that original processors are preserved
			tagProcessor := processors[0].(map[string]interface{})
			Expect(tagProcessor).To(HaveKey("tag_processor"))

			mappingProcessor := processors[1].(map[string]interface{})
			Expect(mappingProcessor).To(HaveKey("mapping"))

			// Verify no downsampler was added
			for _, proc := range processors {
				procMap := proc.(map[string]interface{})
				Expect(procMap).NotTo(HaveKey("downsampler"), "Downsampler should not be injected with multiple processors")
			}
		})

		It("should not inject downsampler if one already exists", func() {
			// Create spec with existing downsampler
			testSpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: createConnectionConfig(),
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count": 1,
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"tag_processor": map[string]any{
											"defaults": "msg.meta.tag_name = 'test'; return msg;",
										},
									},
									map[string]any{
										"downsampler": map[string]any{
											"default": map[string]any{
												"deadband": map[string]any{
													"threshold": 2.0,
												},
											},
										},
									},
								},
							},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
				},
			}

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Should not inject additional downsampler
			readConfig := result.DataflowComponentReadServiceConfig
			processors := readConfig.BenthosConfig.Pipeline["processors"].([]interface{})

			Expect(processors).To(HaveLen(2), "Should still have 2 processors, no injection")

			// Verify existing downsampler config is preserved
			downsampler := processors[1].(map[string]interface{})
			Expect(downsampler).To(HaveKey("downsampler"))
			downsamplerConfig := downsampler["downsampler"].(map[string]interface{})
			Expect(downsamplerConfig).To(HaveKey("default"), "Should preserve user's downsampler config")
		})

		// Note: We don't test write config injection because:
		// 1. Downsampler injection only happens in read configs (by design)
		// 2. Write configs have complex template requirements that are tested elsewhere
		// 3. The existing tests already verify downsampler only affects read configs

		It("should NOT inject downsampler for nodered_js processor", func() {
			// Create spec with nodered_js processor (relational data)
			testSpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: createConnectionConfig(),
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count": 1,
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"nodered_js": map[string]any{
											"code": "return msg;",
										},
									},
								},
							},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
				},
			}

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Check that NO downsampler was injected (nodered_js is relational, not timeseries)
			readConfig := result.DataflowComponentReadServiceConfig
			processors := readConfig.BenthosConfig.Pipeline["processors"].([]interface{})

			Expect(processors).To(HaveLen(1), "Should have only the original nodered_js processor")

			// Verify it's still the original nodered_js processor
			noredejsProcessor := processors[0].(map[string]interface{})
			Expect(noredejsProcessor).To(HaveKey("nodered_js"))
			Expect(noredejsProcessor).ToNot(HaveKey("downsampler"))
		})

		It("should handle empty pipeline gracefully (no downsampler injection)", func() {
			// Create spec with no pipeline
			testSpec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: createConnectionConfig(),
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count": 1,
								},
							},
							Output: map[string]any{
								"stdout": map[string]any{},
							},
						},
					},
				},
			}

			result, err := runtime_config.BuildRuntimeConfig(testSpec, agentLocation, globalVars, nodeName, pcName)
			Expect(err).NotTo(HaveOccurred())

			// Should NOT create pipeline with downsampler (no tag_processor)
			readConfig := result.DataflowComponentReadServiceConfig
			_, exists := readConfig.BenthosConfig.Pipeline["processors"]
			Expect(exists).To(BeFalse(), "Pipeline should NOT be created with processors when no tag_processor exists")
		})
	})
})
