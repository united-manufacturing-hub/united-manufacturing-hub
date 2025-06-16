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

var _ = Describe("ProtocolConverter YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := ProtocolConverterServiceConfigSpec{}
			normalizer := NewNormalizer()

			config = normalizer.NormalizeConfig(config)

			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig).NotTo(BeNil())
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Output).NotTo(BeNil())
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline).NotTo(BeNil())
		})

		It("should preserve existing values", func() {
			config := ProtocolConverterServiceConfigSpec{
				Template: ProtocolConverterServiceConfigTemplate{
					ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
						NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
							Target: "127.0.0.1",
							Port:   "443",
						},
					},
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
			inputMqtt := config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Input["mqtt"].(map[string]any)
			Expect(inputMqtt["topic"]).To(Equal("test/topic"))

			// Check output preserved
			outputUns := config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Output["uns"].(map[string]any) // note that this is NOT kafka, but uns
			Expect(outputUns["bridged_by"]).To(Equal("{{ .internal.bridged_by }}"))

			// Check pipeline processors preserved
			processors := config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(1))
			processor := processors[0].(map[string]any)
			processorText := processor["text"].(map[string]any)
			Expect(processorText["operator"]).To(Equal("to_upper"))
		})

		It("should normalize maps by ensuring they're not nil", func() {
			config := ProtocolConverterServiceConfigSpec{
				Template: ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
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

			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Input).NotTo(BeNil())
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Output).NotTo(BeNil())
			// processor subfield should exist in the pipeline field
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"]).To(BeEmpty())

			// Check write-side configuration
			Expect(config.Template.DataflowComponentWriteServiceConfig.BenthosConfig).NotTo(BeNil())
			Expect(config.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Input).NotTo(BeNil())
			Expect(config.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Output).NotTo(BeNil())
			// processor subfield should exist in the pipeline field
			Expect(config.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(config.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Pipeline["processors"]).To(BeEmpty())

			// Buffer should have the none buffer set
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Buffer).To(HaveKey("none"))

			// These should be empty
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.CacheResources).To(BeEmpty())
			Expect(config.Template.DataflowComponentReadServiceConfig.BenthosConfig.RateLimitResources).To(BeEmpty())
		})
	})

	// Test the package-level function
	Describe("NormalizeDataFlowComponentConfig package function", func() {
		It("should use the default normalizer", func() {
			config1 := ProtocolConverterServiceConfigSpec{}
			config2 := ProtocolConverterServiceConfigSpec{}

			// Use package-level function
			config1 = NormalizeProtocolConverterConfig(config1)

			// Use normalizer directly
			normalizer := NewNormalizer()
			config2 = normalizer.NormalizeConfig(config2)

			// Results should be the same
			Expect(config1).To(Equal(config2))
		})
	})

	// Test downsampler injection functionality
	Describe("Downsampler Injection", func() {
		var normalizer *Normalizer

		BeforeEach(func() {
			normalizer = NewNormalizer()
		})

		Describe("injectDefaultDownsampler", func() {
			It("should inject downsampler after tag_processor", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
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
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(3), "Should have 3 processors after injection")

				// Check first processor is still tag_processor
				tagProc := processors[0].(map[string]any)
				Expect(tagProc).To(HaveKey("tag_processor"))

				// Check second processor is the injected downsampler
				downsampler := processors[1].(map[string]any)
				Expect(downsampler).To(HaveKey("downsampler"))
				downsamplerConfig := downsampler["downsampler"].(map[string]any)
				Expect(downsamplerConfig).To(BeEmpty(), "Downsampler should have empty config (metadata-driven)")

				// Check third processor is the original mapping
				mapping := processors[2].(map[string]any)
				Expect(mapping).To(HaveKey("mapping"))
			})

			It("should not inject if no tag_processor exists", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: map[string]any{
							"processors": []any{
								map[string]any{
									"mapping": "root = this.upper()",
								},
								map[string]any{
									"http": map[string]any{
										"url": "http://example.com",
									},
								},
							},
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(2), "Should have same number of processors")

				// Verify no downsampler was added
				for _, proc := range processors {
					procMap := proc.(map[string]any)
					Expect(procMap).NotTo(HaveKey("downsampler"))
				}
			})

			It("should not inject if downsampler already exists", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
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
												"threshold": 1.0,
											},
										},
									},
								},
							},
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(2), "Should not add another downsampler")

				// Verify existing downsampler config is preserved
				downsampler := processors[1].(map[string]any)["downsampler"].(map[string]any)
				Expect(downsampler).To(HaveKey("default"), "Should preserve user's downsampler config")
			})

			It("should inject after first tag_processor when multiple exist", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: map[string]any{
							"processors": []any{
								map[string]any{
									"tag_processor": map[string]any{
										"defaults": "msg.meta.tag_name = 'first'; return msg;",
									},
								},
								map[string]any{
									"mapping": "root = this.upper()",
								},
								map[string]any{
									"tag_processor": map[string]any{
										"defaults": "msg.meta.tag_name = 'second'; return msg;",
									},
								},
							},
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(4), "Should have 4 processors after injection")

				// Check downsampler is injected after first tag_processor
				downsampler := processors[1].(map[string]any)
				Expect(downsampler).To(HaveKey("downsampler"))

				// Check second tag_processor is still at position 3
				secondTagProc := processors[3].(map[string]any)
				Expect(secondTagProc).To(HaveKey("tag_processor"))
			})

			It("should handle tag_processor as last processor", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: map[string]any{
							"processors": []any{
								map[string]any{
									"mapping": "root = this.upper()",
								},
								map[string]any{
									"tag_processor": map[string]any{
										"defaults": "msg.meta.tag_name = 'test'; return msg;",
									},
								},
							},
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(3), "Should have 3 processors after injection")

				// Check downsampler is appended at the end
				downsampler := processors[2].(map[string]any)
				Expect(downsampler).To(HaveKey("downsampler"))
			})

			It("should not inject if no processors exist", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: map[string]any{
							"processors": []any{},
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				processors := result.BenthosConfig.Pipeline["processors"].([]any)
				Expect(processors).To(HaveLen(0), "Should remain empty")
			})

			It("should not inject if pipeline is nil", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: nil,
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				Expect(result.BenthosConfig.Pipeline).To(BeNil(), "Pipeline should remain nil")
			})

			It("should not inject if pipeline has no processors key", func() {
				config := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Pipeline: map[string]any{
							"threads": 4,
						},
					},
				}

				result := normalizer.injectDefaultDownsampler(config)

				Expect(result.BenthosConfig.Pipeline).To(HaveKey("threads"))
				Expect(result.BenthosConfig.Pipeline).NotTo(HaveKey("processors"))
			})
		})

		Describe("Integration with full normalization", func() {
			It("should inject downsampler in read component but not write component", func() {
				spec := ProtocolConverterServiceConfigSpec{
					Template: ProtocolConverterServiceConfigTemplate{
						DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Pipeline: map[string]any{
									"processors": []any{
										map[string]any{
											"tag_processor": map[string]any{
												"defaults": "msg.meta.tag_name = 'test'; return msg;",
											},
										},
									},
								},
							},
						},
						DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
								Pipeline: map[string]any{
									"processors": []any{
										map[string]any{
											"tag_processor": map[string]any{
												"defaults": "msg.meta.tag_name = 'test'; return msg;",
											},
										},
									},
								},
							},
						},
					},
				}

				result := normalizer.NormalizeConfig(spec)

				// Check read component has downsampler injected
				readProcessors := result.Template.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
				Expect(readProcessors).To(HaveLen(2), "Read component should have downsampler injected")

				readDownsampler := readProcessors[1].(map[string]any)
				Expect(readDownsampler).To(HaveKey("downsampler"))

				// Check write component does NOT have downsampler injected
				writeProcessors := result.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Pipeline["processors"].([]any)
				Expect(writeProcessors).To(HaveLen(1), "Write component should not have downsampler injected")

				// Verify write component only has the original tag_processor
				writeTagProc := writeProcessors[0].(map[string]any)
				Expect(writeTagProc).To(HaveKey("tag_processor"))
			})
		})

		Describe("Helper Functions", func() {
			Describe("isTagProcessor", func() {
				It("should detect tag_processor correctly", func() {
					tagProc := map[string]any{
						"tag_processor": map[string]any{
							"defaults": "return msg;",
						},
					}
					Expect(isTagProcessor(tagProc)).To(BeTrue())
				})

				It("should not detect other processors as tag_processor", func() {
					mapping := map[string]any{
						"mapping": "root = this.upper()",
					}
					Expect(isTagProcessor(mapping)).To(BeFalse())
				})

				It("should handle non-map inputs gracefully", func() {
					Expect(isTagProcessor("not a map")).To(BeFalse())
					Expect(isTagProcessor(nil)).To(BeFalse())
				})
			})

			Describe("hasDownsampler", func() {
				It("should detect existing downsampler", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
						map[string]any{"downsampler": map[string]any{}},
					}
					Expect(hasDownsampler(processors)).To(BeTrue())
				})

				It("should return false when no downsampler exists", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
						map[string]any{"tag_processor": map[string]any{}},
					}
					Expect(hasDownsampler(processors)).To(BeFalse())
				})
			})

			Describe("insertProcessorAfter", func() {
				It("should insert processor after specified index", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
						map[string]any{"tag_processor": map[string]any{}},
					}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, 1, newProcessor)

					Expect(result).To(HaveLen(3))
					Expect(result[0]).To(Equal(processors[0]))
					Expect(result[1]).To(Equal(processors[1]))
					Expect(result[2]).To(Equal(newProcessor))
				})

				It("should insert processor after first index", func() {
					processors := []any{
						map[string]any{"tag_processor": map[string]any{}},
						map[string]any{"mapping": "root = this"},
					}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, 0, newProcessor)

					Expect(result).To(HaveLen(3))
					Expect(result[0]).To(Equal(processors[0]))
					Expect(result[1]).To(Equal(newProcessor))
					Expect(result[2]).To(Equal(processors[1]))
				})

				It("should return original slice unchanged for negative index", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
						map[string]any{"tag_processor": map[string]any{}},
					}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, -1, newProcessor)

					Expect(result).To(Equal(processors))
					Expect(result).To(HaveLen(2))
				})

				It("should return original slice unchanged for index >= len(processors)", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
						map[string]any{"tag_processor": map[string]any{}},
					}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, 2, newProcessor)

					Expect(result).To(Equal(processors))
					Expect(result).To(HaveLen(2))
				})

				It("should return original slice unchanged for index way out of bounds", func() {
					processors := []any{
						map[string]any{"mapping": "root = this"},
					}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, 100, newProcessor)

					Expect(result).To(Equal(processors))
					Expect(result).To(HaveLen(1))
				})

				It("should handle empty processor slice gracefully", func() {
					processors := []any{}
					newProcessor := map[string]any{"downsampler": map[string]any{}}

					result := insertProcessorAfter(processors, 0, newProcessor)

					Expect(result).To(Equal(processors))
					Expect(result).To(HaveLen(0))
				})
			})
		})
	})
})
