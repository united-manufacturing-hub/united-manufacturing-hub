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

var _ = Describe("DFC YAML Comparator", func() {
	Describe("ConfigsEqual", func() {
		It("should consider identical configs equal", func() {
			config1 := DataflowComponentServiceConfig{
				BenthosConfig: BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
					Output: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/output",
						},
					},
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig: BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
					Output: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/output",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})

		It("should consider configs with different input not equal", func() {
			config1 := DataflowComponentServiceConfig{
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
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "different/topic",
						},
					},
					Output: map[string]any{
						"kafka": map[string]any{
							"topic": "test-output",
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

			config1 := DataflowComponentServiceConfig{
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
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
					Output: map[string]any{
						"kafka": map[string]any{
							"topic": "different-output",
						},
					},
				},
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)
			Expect(equal).To(BeFalse())

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("Output config differences"))
			Expect(diff).To(ContainSubstring("Output.kafka differs"))
		})
	})

	Describe("ConfigDiff", func() {
		It("should generate readable diff for different configs", func() {
			config1 := DataflowComponentServiceConfig{
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
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "different/topic",
						},
					},
					Output: map[string]any{
						"kafka": map[string]any{
							"topic": "different-output",
						},
					},
				},
			}

			comparator := NewComparator()
			diff := comparator.ConfigDiff(config1, config2)

			Expect(diff).To(ContainSubstring("Input config differences"))
			Expect(diff).To(ContainSubstring("Input.mqtt differs"))

			Expect(diff).To(ContainSubstring("Output config differences"))
			Expect(diff).To(ContainSubstring("Output.kafka differs"))
		})
	})

	// Test package-level functions
	Describe("Package-level functions", func() {
		It("ConfigsEqual should use default comparator", func() {
			config1 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
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
			config1 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "test/topic",
						},
					},
				},
			}

			config2 := DataflowComponentServiceConfig{
				BenthosConfig{
					Input: map[string]any{
						"mqtt": map[string]any{
							"topic": "different/topic",
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
