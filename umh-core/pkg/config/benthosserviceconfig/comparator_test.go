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

var _ = Describe("Benthos YAML Comparator", func() {
	Describe("ConfigsEqual", func() {
		It("should consider identical configs equal", func() {
			config1 := BenthosServiceConfig{
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
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}

			config2 := BenthosServiceConfig{
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
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})

		It("should consider configs with different inputs not equal", func() {
			config1 := BenthosServiceConfig{
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
			}

			config2 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "different/topic",
					},
				},
				Output: map[string]interface{}{
					"kafka": map[string]interface{}{
						"topic": "test-output",
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

		It("should consider configs with different outputs not equal", func() {
			config1 := BenthosServiceConfig{
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
			}

			config2 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "test/topic",
					},
				},
				Output: map[string]interface{}{
					"kafka": map[string]interface{}{
						"topic": "different-output",
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

		It("should normalize configs before comparison", func() {
			// Config with default values
			config1 := BenthosServiceConfig{
				Input:       map[string]interface{}{},
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}

			// Missing fields that should get default values
			config2 := BenthosServiceConfig{}

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})
	})

	Describe("ConfigDiff", func() {
		It("should generate readable diff for different configs", func() {
			config1 := BenthosServiceConfig{
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
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}

			config2 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "different/topic",
					},
				},
				Output: map[string]interface{}{
					"kafka": map[string]interface{}{
						"topic": "different-output",
					},
				},
				MetricsPort: 5000,
				LogLevel:    "DEBUG",
			}

			comparator := NewComparator()
			diff := comparator.ConfigDiff(config1, config2)

			Expect(diff).To(ContainSubstring("MetricsPort"))
			Expect(diff).To(ContainSubstring("Want: 4195"))
			Expect(diff).To(ContainSubstring("Have: 5000"))

			Expect(diff).To(ContainSubstring("LogLevel"))
			Expect(diff).To(ContainSubstring("Want: INFO"))
			Expect(diff).To(ContainSubstring("Have: DEBUG"))

			Expect(diff).To(ContainSubstring("Input config differences"))
			Expect(diff).To(ContainSubstring("Input.mqtt differs"))

			Expect(diff).To(ContainSubstring("Output config differences"))
			Expect(diff).To(ContainSubstring("Output.kafka differs"))
		})
	})

	// Test package-level functions
	Describe("Package-level functions", func() {
		It("ConfigsEqual should use default comparator", func() {
			config1 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "test/topic",
					},
				},
			}

			config2 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "test/topic",
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
			config1 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "test/topic",
					},
				},
			}

			config2 := BenthosServiceConfig{
				Input: map[string]interface{}{
					"mqtt": map[string]interface{}{
						"topic": "different/topic",
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
