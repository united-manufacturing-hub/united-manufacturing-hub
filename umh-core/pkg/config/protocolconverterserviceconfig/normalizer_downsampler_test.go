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

package protocolconverterserviceconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
)

var _ = Describe("Downsampler Integration Tests", func() {
	Describe("Real-world Protocol Converter Scenarios", func() {
		var normalizer *protocolconverterserviceconfig.Normalizer

		BeforeEach(func() {
			normalizer = protocolconverterserviceconfig.NewNormalizer()
		})

		It("should inject downsampler for vibration sensor protocol converter", func() {
			// This mimics the vibration-sensor-pc from the example config
			spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Template: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "{{ .HOST }}",
							Port:   "{{ .PORT }}",
						},
					},
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"count":    0,
									"interval": "1s",
									"mapping":  "root = random_int(min:10, max:20)",
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"tag_processor": map[string]any{
											"defaults": `
												msg.meta.location_path = "{{ .location_path }}";
												msg.meta.data_contract = "_historian";
												msg.meta.tag_name = "vibration_level";
												msg.meta.ds_algorithm = "deadband";
												msg.meta.ds_threshold = 0;
												msg.meta.ds_max_time = "30m";
												return msg;
											`,
											"conditions": []any{
												map[string]any{
													"if": "msg.payload > 50",
													"then": `
														msg.meta.ds_algorithm = "swinging_door";
														msg.meta.ds_threshold = 5.0;
														msg.meta.ds_min_time = "1s";
														msg.meta.ds_max_time = "5m";
														return msg;
													`,
												},
											},
										},
									},
								},
							},
							Output: map[string]any{
								"uns": map[string]any{},
							},
						},
					},
				},
			}

			result := normalizer.NormalizeConfig(spec)

			// Verify downsampler was injected
			processors := result.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(2), "Should have tag_processor + injected downsampler")

			// Check tag_processor is first
			tagProc := processors[0].(map[string]any)
			Expect(tagProc).To(HaveKey("tag_processor"))

			// Check downsampler is second (injected)
			downsampler := processors[1].(map[string]any)
			Expect(downsampler).To(HaveKey("downsampler"))

			// Verify it's an empty downsampler (metadata-driven)
			downsamplerConfig := downsampler["downsampler"].(map[string]any)
			Expect(downsamplerConfig).To(BeEmpty(), "Injected downsampler should have empty config")
		})

		It("should not inject downsampler when manually configured", func() {
			// This mimics the manual-downsampler-pc from the example config
			spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Template: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"opcua": map[string]any{
									"endpoint": "opc.tcp://192.168.1.200:4840",
									"nodeIDs":  []string{"ns=2;s=PressureData"},
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"tag_processor": map[string]any{
											"defaults": `
												msg.meta.location_path = "{{ .location_path }}";
												msg.meta.data_contract = "_historian";
												msg.meta.tag_name = "pressure";
												return msg;
											`,
										},
									},
									map[string]any{
										"downsampler": map[string]any{
											"default": map[string]any{
												"deadband": map[string]any{
													"threshold": 2.0,
													"max_time":  "10m",
												},
											},
											"overrides": []any{
												map[string]any{
													"pattern":     "*.pressure",
													"late_policy": "drop",
													"swinging_door": map[string]any{
														"threshold": 1.0,
														"min_time":  "5s",
														"max_time":  "30m",
													},
												},
											},
										},
									},
								},
							},
							Output: map[string]any{
								"uns": map[string]any{},
							},
						},
					},
				},
			}

			result := normalizer.NormalizeConfig(spec)

			// Verify no additional downsampler was injected
			processors := result.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(2), "Should still have 2 processors (no injection)")

			// Check tag_processor is first
			tagProc := processors[0].(map[string]any)
			Expect(tagProc).To(HaveKey("tag_processor"))

			// Check manual downsampler is second (preserved)
			downsampler := processors[1].(map[string]any)
			Expect(downsampler).To(HaveKey("downsampler"))

			// Verify it's the user's manual configuration (not empty)
			downsamplerConfig := downsampler["downsampler"].(map[string]any)
			Expect(downsamplerConfig).To(HaveKey("default"), "Should preserve user's manual config")
			Expect(downsamplerConfig).To(HaveKey("overrides"), "Should preserve user's manual config")
		})

		It("should not inject downsampler for non-time-series data flows", func() {
			// Protocol converter without tag_processor (pure data transformation)
			spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Template: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topics": []string{"raw/data"},
									"urls":   []string{"tcp://mqtt:1883"},
								},
							},
							Pipeline: map[string]any{
								"processors": []any{
									map[string]any{
										"mapping": "root = this.upper()",
									},
									map[string]any{
										"http": map[string]any{
											"url": "http://api.example.com/webhook",
										},
									},
								},
							},
							Output: map[string]any{
								"kafka": map[string]any{
									"addresses": []string{"kafka:9092"},
									"topic":     "processed-data",
								},
							},
						},
					},
				},
			}

			result := normalizer.NormalizeConfig(spec)

			// Verify no downsampler was injected
			processors := result.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(2), "Should have same number of processors")

			// Verify no downsampler exists
			for _, proc := range processors {
				procMap := proc.(map[string]any)
				Expect(procMap).NotTo(HaveKey("downsampler"))
			}
		})

		It("should handle empty processors gracefully", func() {
			spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Template: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"generate": map[string]any{
									"mapping": "root = {}",
								},
							},
							Pipeline: map[string]any{
								"processors": []any{},
							},
							Output: map[string]any{
								"drop": map[string]any{},
							},
						},
					},
				},
			}

			result := normalizer.NormalizeConfig(spec)

			// Verify processors remain empty
			processors := result.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(0), "Should remain empty")
		})
	})
})
