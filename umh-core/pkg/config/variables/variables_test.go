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
	"gopkg.in/yaml.v3"
)

var _ = Describe("VariableBundle YAML Marshaling", func() {
	Describe("MarshalYAML", func() {
		It("should marshal user variables as flat structure", func() {
			vb := variables.VariableBundle{
				User: map[string]any{
					"HOST": "localhost",
					"PORT": "8080",
					"nested": map[string]any{
						"key": "value",
					},
				},
				Global: map[string]any{
					"global_key": "global_value",
				},
				Internal: map[string]any{
					"internal_key": "internal_value",
				},
			}

			yamlBytes, err := yaml.Marshal(vb)
			Expect(err).NotTo(HaveOccurred())

			yamlStr := string(yamlBytes)
			// Should contain user variables at top level
			Expect(yamlStr).To(ContainSubstring("HOST: localhost"))
			Expect(yamlStr).To(ContainSubstring("PORT: \"8080\""))
			Expect(yamlStr).To(ContainSubstring("nested:"))
			Expect(yamlStr).To(ContainSubstring("key: value"))

			// Should NOT contain global or internal namespaces
			Expect(yamlStr).NotTo(ContainSubstring("global:"))
			Expect(yamlStr).NotTo(ContainSubstring("internal:"))
			Expect(yamlStr).NotTo(ContainSubstring("user:"))
		})

		It("should marshal empty bundle as empty map", func() {
			vb := variables.VariableBundle{}

			yamlBytes, err := yaml.Marshal(vb)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(yamlBytes)).To(Equal("{}\n"))
		})
	})

	Describe("UnmarshalYAML", func() {
		It("should unmarshal flat structure into User namespace", func() {
			yamlStr := `
HOST: localhost
PORT: "8080"
nested:
  key: value
  number: 42
`
			var vb variables.VariableBundle
			err := yaml.Unmarshal([]byte(yamlStr), &vb)
			Expect(err).NotTo(HaveOccurred())

			Expect(vb.User).To(HaveKeyWithValue("HOST", "localhost"))
			Expect(vb.User).To(HaveKeyWithValue("PORT", "8080"))
			Expect(vb.User).To(HaveKey("nested"))

			nested, ok := vb.User["nested"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(nested).To(HaveKeyWithValue("key", "value"))
			Expect(nested).To(HaveKeyWithValue("number", 42))

			Expect(vb.Global).To(BeNil())
			Expect(vb.Internal).To(BeNil())
		})

		It("should unmarshal explicit namespace structure", func() {
			yamlStr := `
user:
  HOST: localhost
  PORT: "8080"
global:
  global_key: global_value
internal:
  internal_key: internal_value
`
			var vb variables.VariableBundle
			err := yaml.Unmarshal([]byte(yamlStr), &vb)
			Expect(err).NotTo(HaveOccurred())

			Expect(vb.User).To(HaveKeyWithValue("HOST", "localhost"))
			Expect(vb.User).To(HaveKeyWithValue("PORT", "8080"))
			Expect(vb.Global).To(HaveKeyWithValue("global_key", "global_value"))
			Expect(vb.Internal).To(HaveKeyWithValue("internal_key", "internal_value"))
		})

		It("should preserve existing Global and Internal when unmarshaling flat structure", func() {
			// Start with a bundle that has global and internal set
			vb := variables.VariableBundle{
				Global: map[string]any{
					"existing_global": "value",
				},
				Internal: map[string]any{
					"existing_internal": "value",
				},
			}

			yamlStr := `
HOST: localhost
PORT: "8080"
`
			err := yaml.Unmarshal([]byte(yamlStr), &vb)
			Expect(err).NotTo(HaveOccurred())

			// User variables should be set
			Expect(vb.User).To(HaveKeyWithValue("HOST", "localhost"))
			Expect(vb.User).To(HaveKeyWithValue("PORT", "8080"))

			// Existing Global and Internal should be preserved
			Expect(vb.Global).To(HaveKeyWithValue("existing_global", "value"))
			Expect(vb.Internal).To(HaveKeyWithValue("existing_internal", "value"))
		})

		It("should handle empty YAML", func() {
			yamlStr := `{}`
			var vb variables.VariableBundle
			err := yaml.Unmarshal([]byte(yamlStr), &vb)
			Expect(err).NotTo(HaveOccurred())

			Expect(vb.User).To(BeEmpty())
			Expect(vb.Global).To(BeNil())
			Expect(vb.Internal).To(BeNil())
		})
	})

	Describe("Round-trip compatibility", func() {
		It("should maintain user variables through marshal/unmarshal cycle", func() {
			original := variables.VariableBundle{
				User: map[string]any{
					"HOST": "localhost",
					"PORT": "8080",
					"complex": map[string]any{
						"nested": []any{1, 2, 3},
						"bool":   true,
					},
				},
			}

			// Marshal to YAML
			yamlBytes, err := yaml.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			// Unmarshal back
			var restored variables.VariableBundle
			err = yaml.Unmarshal(yamlBytes, &restored)
			Expect(err).NotTo(HaveOccurred())

			// Should be identical
			Expect(restored.User).To(Equal(original.User))
		})
	})
})
