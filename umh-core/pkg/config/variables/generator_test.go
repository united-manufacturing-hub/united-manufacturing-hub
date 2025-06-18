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

var _ = Describe("VariableBundle Generator", func() {
	var (
		generator *variables.Generator
	)

	BeforeEach(func() {
		generator = variables.NewGenerator()
	})

	Describe("RenderConfig", func() {
		It("should generate valid YAML for a complete config", func() {
			config := variables.VariableBundle{
				User: map[string]any{
					"key1": "value1",
					"key2": 42,
				},
				Global: map[string]any{
					"global1": "value1",
					"global2": true,
				},
				Internal: map[string]any{
					"internal1": "value1",
					"internal2": 3.14,
				},
			}

			yaml, err := generator.RenderConfig(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml).To(ContainSubstring("key1: value1"))
			Expect(yaml).To(ContainSubstring("key2: 42"))
			Expect(yaml).To(ContainSubstring("global1: value1"))
			Expect(yaml).To(ContainSubstring("global2: true"))
			Expect(yaml).To(ContainSubstring("internal1: value1"))
			Expect(yaml).To(ContainSubstring("internal2: 3.14"))
		})

		It("should handle empty maps", func() {
			config := variables.VariableBundle{}

			yaml, err := generator.RenderConfig(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml).To(Equal("{}\n"))
		})

		It("should handle nil maps", func() {
			config := variables.VariableBundle{
				User:     nil,
				Global:   nil,
				Internal: nil,
			}

			yaml, err := generator.RenderConfig(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml).To(Equal("{}\n"))
		})

		It("should handle complex nested structures", func() {
			config := variables.VariableBundle{
				User: map[string]any{
					"nested": map[string]any{
						"key1": []any{1, 2, 3},
						"key2": map[string]any{
							"subkey": "value",
						},
					},
				},
			}

			yaml, err := generator.RenderConfig(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml).To(ContainSubstring("nested:"))
			Expect(yaml).To(ContainSubstring("- 1"))
			Expect(yaml).To(ContainSubstring("- 2"))
			Expect(yaml).To(ContainSubstring("- 3"))
			Expect(yaml).To(ContainSubstring("subkey: value"))
		})
	})
})
