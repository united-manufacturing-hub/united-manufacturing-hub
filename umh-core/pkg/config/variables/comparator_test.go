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

package variables_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

var _ = Describe("VariableBundle Comparator", func() {
	var (
		comparator *variables.Comparator
	)

	BeforeEach(func() {
		comparator = variables.NewComparator()
	})

	Describe("ConfigsEqual", func() {
		It("should return true for identical configs", func() {
			config1 := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
				Global: map[string]any{
					"global1": "value1",
				},
				Internal: map[string]any{
					"internal1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
				Global: map[string]any{
					"global1": "value1",
				},
				Internal: map[string]any{
					"internal1": "value1",
				},
			}

			Expect(comparator.ConfigsEqual(config1, config2)).To(BeTrue())
		})

		It("should return false for different configs", func() {
			config1 := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				User: map[string]any{
					"key1": "different",
				},
			}

			Expect(comparator.ConfigsEqual(config1, config2)).To(BeFalse())
		})

		It("should handle nil maps", func() {
			config1 := variables.VariableBundle{}
			config2 := variables.VariableBundle{}

			Expect(comparator.ConfigsEqual(config1, config2)).To(BeTrue())
		})
	})

	Describe("ConfigDiff", func() {
		It("should return identical message for equal configs", func() {
			config := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
			}

			Expect(comparator.ConfigDiff(config, config)).To(Equal("Configs are identical"))
		})

		It("should detect differences in User variables", func() {
			config1 := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				User: map[string]any{
					"key1": "different",
				},
			}

			Expect(comparator.ConfigDiff(config1, config2)).To(ContainSubstring("User variables differ"))
		})

		It("should detect differences in Global variables", func() {
			config1 := variables.VariableBundle{
				Global: map[string]any{
					"global1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				Global: map[string]any{
					"global1": "different",
				},
			}

			Expect(comparator.ConfigDiff(config1, config2)).To(ContainSubstring("Global variables differ"))
		})

		It("should detect differences in Internal variables", func() {
			config1 := variables.VariableBundle{
				Internal: map[string]any{
					"internal1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				Internal: map[string]any{
					"internal1": "different",
				},
			}

			Expect(comparator.ConfigDiff(config1, config2)).To(ContainSubstring("Internal variables differ"))
		})

		It("should detect multiple differences", func() {
			config1 := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
				},
				Global: map[string]any{
					"global1": "value1",
				},
			}
			config2 := variables.VariableBundle{
				User: map[string]any{
					"key1": "different",
				},
				Global: map[string]any{
					"global1": "different",
				},
			}

			diff := comparator.ConfigDiff(config1, config2)
			Expect(diff).To(ContainSubstring("User variables differ"))
			Expect(diff).To(ContainSubstring("Global variables differ"))
		})
	})
})
