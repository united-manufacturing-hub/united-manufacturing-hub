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

var _ = Describe("ProtocolConverter YAML Comparator", func() {
	Describe("ConfigsEqual", func() {
		It("should consider identical configs equal", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"kafka": map[string]any{
									"topic": "test/output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"kafka": map[string]any{
									"topic": "test/output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})

		It("should consider configs with different input not equal", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"kafka": map[string]any{
									"topic": "test/output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "different/topic",
								},
							},
							Output: map[string]any{
								"kafka": map[string]any{
									"topic": "test/output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Input config differences"))
			Expect(diff).To(ContainSubstring("Input.mqtt differs"))
		})

		It("should consider configs with different output not equal", func() {

			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic1",
								},
							},
							Output: map[string]any{ // wrong user input, read DFC will always have uns output
								"kafka": map[string]any{
									"topic": "test-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic2",
								},
							},
							Output: map[string]any{ // wrong user input, read DFC will always have uns output
								"kafka": map[string]any{
									"topic": "different-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Input config differences"))
			Expect(diff).To(ContainSubstring("Input.mqtt differs"))
		})

		It("should consider configs with different Target not equal", func() {

			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.2",
							Port:   "443",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("No significant differences"))
		})

		It("should consider configs with different Port not equal", func() {

			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "444",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("No significant differences"))
		})

		It("should consider configs with same location equal", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-A",
					"2": "Area-1",
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-A",
					"2": "Area-1",
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeTrue())
		})

		It("should consider configs with different location not equal", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-A",
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-B",
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Location:"))
			Expect(diff).To(ContainSubstring("Site-A"))
			Expect(diff).To(ContainSubstring("Site-B"))
		})

		It("should consider configs that have a different amount of locations not equal", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-A",
					"2": "Site-B",
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
				Location: map[string]string{
					"0": "Enterprise",
					"1": "Site-B",
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Location:"))
			Expect(diff).To(ContainSubstring("Site-A"))
			Expect(diff).To(ContainSubstring("Site-B"))
		})
	})

	Describe("ConfigDiff", func() {
		It("should generate readable diff for different configs", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{ // wrong user input, read DFC will always have uns output
								"kafka": map[string]any{
									"topic": "test-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "different/topic",
								},
							},
							Output: map[string]any{ // wrong user input, read DFC will always have uns output
								"kafka": map[string]any{
									"topic": "different-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "444",
						},
					},
				},
			}

			comparator := NewComparator()
			diff := comparator.ConfigDiff(config1, config2)

			Expect(diff).To(ContainSubstring("Input config differences"))
			Expect(diff).To(ContainSubstring("Input.mqtt differs"))

			Expect(diff).To(ContainSubstring("No significant differences"))
		})
	})

	Describe("Per-DFC desired state comparison", func() {
		baseConfig := func() ProtocolConverterServiceConfigSpec {
			return ProtocolConverterServiceConfigSpec{
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
								"mqtt": map[string]any{"topic": "test"},
							},
						},
					},
				},
			}
		}

		It("should consider configs equal when both DFC desired states match", func() {
			c1 := baseConfig()
			c1.ReadDFCDesiredState = "active"
			c1.WriteDFCDesiredState = "stopped"

			c2 := baseConfig()
			c2.ReadDFCDesiredState = "active"
			c2.WriteDFCDesiredState = "stopped"

			comparator := NewComparator()
			Expect(comparator.ConfigsEqual(c1, c2)).To(BeTrue())
		})

		It("should consider configs not equal when ReadDFCDesiredState differs", func() {
			c1 := baseConfig()
			c1.ReadDFCDesiredState = "active"

			c2 := baseConfig()
			c2.ReadDFCDesiredState = "stopped"

			comparator := NewComparator()
			Expect(comparator.ConfigsEqual(c1, c2)).To(BeFalse())

			diff := comparator.ConfigDiff(c1, c2)
			Expect(diff).To(ContainSubstring("ReadDFCDesiredState"))
			Expect(diff).To(ContainSubstring("active"))
			Expect(diff).To(ContainSubstring("stopped"))
		})

		It("should consider configs not equal when WriteDFCDesiredState differs", func() {
			c1 := baseConfig()
			c1.WriteDFCDesiredState = "active"

			c2 := baseConfig()
			c2.WriteDFCDesiredState = "stopped"

			comparator := NewComparator()
			Expect(comparator.ConfigsEqual(c1, c2)).To(BeFalse())

			diff := comparator.ConfigDiff(c1, c2)
			Expect(diff).To(ContainSubstring("WriteDFCDesiredState"))
			Expect(diff).To(ContainSubstring("active"))
			Expect(diff).To(ContainSubstring("stopped"))
		})

		It("should consider configs not equal when both DFC desired states differ", func() {
			c1 := baseConfig()
			c1.ReadDFCDesiredState = "active"
			c1.WriteDFCDesiredState = "active"

			c2 := baseConfig()
			c2.ReadDFCDesiredState = "stopped"
			c2.WriteDFCDesiredState = "stopped"

			comparator := NewComparator()
			Expect(comparator.ConfigsEqual(c1, c2)).To(BeFalse())

			diff := comparator.ConfigDiff(c1, c2)
			Expect(diff).To(ContainSubstring("ReadDFCDesiredState"))
			Expect(diff).To(ContainSubstring("WriteDFCDesiredState"))
		})

		It("should consider empty and non-empty DFC desired states not equal", func() {
			c1 := baseConfig()
			// c1 has empty ReadDFCDesiredState (zero value)

			c2 := baseConfig()
			c2.ReadDFCDesiredState = "active"

			comparator := NewComparator()
			Expect(comparator.ConfigsEqual(c1, c2)).To(BeFalse())
		})
	})

	// Test package-level functions
	Describe("Package-level functions", func() {
		It("ConfigsEqual should use default comparator", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"mqtt": map[string]any{
									"topic": "test-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"mqtt": map[string]any{
									"topic": "test-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			// Use package-level function
			equal1 := ConfigsEqual(config1, config2)

			// Use comparator directly
			comparator := NewComparator()
			equal2 := comparator.ConfigsEqual(config1, config2)

			Expect(equal1).To(Equal(equal2))
		})

		It("ConfigDiff should use default comparator", func() {
			config1 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "test/topic",
								},
							},
							Output: map[string]any{
								"mqtt": map[string]any{
									"topic": "test-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
				},
			}

			config2 := ProtocolConverterServiceConfigSpec{
				Config: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt": map[string]any{
									"topic": "different/topic",
								},
							},
							Output: map[string]any{
								"mqtt": map[string]any{
									"topic": "different-output",
								},
							},
						},
					},
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "444",
						},
					},
				},
			}

			// Use package-level function
			diff1 := ConfigDiff(config1, config2)

			// Use comparator directly
			comparator := NewComparator()
			diff2 := comparator.ConfigDiff(config1, config2)

			Expect(diff1).To(Equal(diff2))
		})
	})
})
