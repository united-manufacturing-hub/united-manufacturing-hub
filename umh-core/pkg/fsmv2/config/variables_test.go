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

package config_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("VariableBundle", func() {
	Describe("YAML serialization", func() {
		It("should serialize User and Global, but exclude Internal", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id":         "internal-123",
					"bridged_by": "bridge-1",
				},
			}

			data, err := yaml.Marshal(bundle)
			Expect(err).ToNot(HaveOccurred())

			yamlStr := string(data)
			Expect(yamlStr).To(ContainSubstring("user:"))
			Expect(yamlStr).To(ContainSubstring("IP: 192.168.1.100"))
			Expect(yamlStr).To(ContainSubstring("PORT: 502"))
			Expect(yamlStr).To(ContainSubstring("global:"))
			Expect(yamlStr).To(ContainSubstring("api_endpoint: https://api.example.com"))
			Expect(yamlStr).ToNot(ContainSubstring("internal"))
			Expect(yamlStr).ToNot(ContainSubstring("id:"))
			Expect(yamlStr).ToNot(ContainSubstring("bridged_by:"))
		})

		It("should deserialize User and Global correctly", func() {
			yamlData := `
user:
  IP: 192.168.1.100
  PORT: 502
global:
  api_endpoint: https://api.example.com
`
			var bundle config.VariableBundle
			err := yaml.Unmarshal([]byte(yamlData), &bundle)

			Expect(err).ToNot(HaveOccurred())
			Expect(bundle.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(bundle.User).To(HaveKeyWithValue("PORT", 502))
			Expect(bundle.Global).To(HaveKeyWithValue("api_endpoint", "https://api.example.com"))
			Expect(bundle.Internal).To(BeNil())
		})

		It("should handle empty VariableBundle correctly", func() {
			bundle := config.VariableBundle{}

			data, err := yaml.Marshal(bundle)
			Expect(err).ToNot(HaveOccurred())

			// Use semantic check instead of exact string match (more robust against yaml library changes)
			var result map[string]any
			err = yaml.Unmarshal(data, &result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should round-trip through YAML correctly", func() {
			original := config.VariableBundle{
				User: map[string]any{
					"IP":         "10.0.0.1",
					"PORT":       8080,
					"enabled":    true,
					"rate_limit": 100.5,
				},
				Global: map[string]any{
					"cluster_id": "prod-cluster",
					"version":    "2.0",
				},
				Internal: map[string]any{
					"should_not_serialize": true,
				},
			}

			data, err := yaml.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			var roundtripped config.VariableBundle
			err = yaml.Unmarshal(data, &roundtripped)
			Expect(err).ToNot(HaveOccurred())

			Expect(roundtripped.User).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(roundtripped.User).To(HaveKeyWithValue("PORT", 8080))
			Expect(roundtripped.User).To(HaveKeyWithValue("enabled", true))
			Expect(roundtripped.User).To(HaveKeyWithValue("rate_limit", 100.5))
			Expect(roundtripped.Global).To(HaveKeyWithValue("cluster_id", "prod-cluster"))
			Expect(roundtripped.Global).To(HaveKeyWithValue("version", "2.0"))
			Expect(roundtripped.Internal).To(BeNil())
		})
	})

	Describe("JSON serialization", func() {
		It("should serialize User and Global, but exclude Internal", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id":         "internal-123",
					"bridged_by": "bridge-1",
				},
			}

			data, err := json.Marshal(bundle)
			Expect(err).ToNot(HaveOccurred())

			jsonStr := string(data)
			Expect(jsonStr).To(ContainSubstring(`"user"`))
			Expect(jsonStr).To(ContainSubstring(`"IP":"192.168.1.100"`))
			Expect(jsonStr).To(ContainSubstring(`"PORT":502`))
			Expect(jsonStr).To(ContainSubstring(`"global"`))
			Expect(jsonStr).To(ContainSubstring(`"api_endpoint":"https://api.example.com"`))
			Expect(jsonStr).ToNot(ContainSubstring(`"internal"`))
			Expect(jsonStr).ToNot(ContainSubstring(`"id":"internal-123"`))
			Expect(jsonStr).ToNot(ContainSubstring(`"bridged_by"`))
		})

		It("should round-trip through JSON correctly", func() {
			original := config.VariableBundle{
				User: map[string]any{
					"IP":      "10.0.0.1",
					"PORT":    float64(8080),
					"enabled": true,
				},
				Global: map[string]any{
					"cluster_id": "prod-cluster",
				},
				Internal: map[string]any{
					"should_not_serialize": true,
				},
			}

			data, err := json.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			var roundtripped config.VariableBundle
			err = json.Unmarshal(data, &roundtripped)
			Expect(err).ToNot(HaveOccurred())

			Expect(roundtripped.User).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(roundtripped.User).To(HaveKeyWithValue("PORT", float64(8080)))
			Expect(roundtripped.User).To(HaveKeyWithValue("enabled", true))
			Expect(roundtripped.Global).To(HaveKeyWithValue("cluster_id", "prod-cluster"))
			Expect(roundtripped.Internal).To(BeNil())
		})
	})

	Describe("Flatten", func() {
		It("should promote User variables to top-level", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).To(HaveKeyWithValue("PORT", 502))
		})

		It("should nest Global variables under 'global' key", func() {
			bundle := config.VariableBundle{
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
					"cluster_id":   "prod-123",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKey("global"))
			globalMap, ok := result["global"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(globalMap).To(HaveKeyWithValue("api_endpoint", "https://api.example.com"))
			Expect(globalMap).To(HaveKeyWithValue("cluster_id", "prod-123"))
		})

		It("should nest Internal variables under 'internal' key", func() {
			bundle := config.VariableBundle{
				Internal: map[string]any{
					"id":        "internal-123",
					"timestamp": 1234567890,
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKey("internal"))
			internalMap, ok := result["internal"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(internalMap).To(HaveKeyWithValue("id", "internal-123"))
			Expect(internalMap).To(HaveKeyWithValue("timestamp", 1234567890))
		})

		It("should handle multiple User variables all promoted to top-level", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":         "10.0.0.1",
					"PORT":       8080,
					"enabled":    true,
					"rate_limit": 100.5,
					"name":       "my-device",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(result).To(HaveKeyWithValue("PORT", 8080))
			Expect(result).To(HaveKeyWithValue("enabled", true))
			Expect(result).To(HaveKeyWithValue("rate_limit", 100.5))
			Expect(result).To(HaveKeyWithValue("name", "my-device"))
		})

		It("should handle all three namespaces together", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id": "internal-123",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).To(HaveKeyWithValue("PORT", 502))
			Expect(result).To(HaveKey("global"))
			Expect(result).To(HaveKey("internal"))

			globalMap := result["global"].(map[string]any)
			Expect(globalMap).To(HaveKeyWithValue("api_endpoint", "https://api.example.com"))

			internalMap := result["internal"].(map[string]any)
			Expect(internalMap).To(HaveKeyWithValue("id", "internal-123"))
		})

		It("should handle empty User map without breaking", func() {
			bundle := config.VariableBundle{
				User: map[string]any{},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKey("global"))
			Expect(result).ToNot(HaveKey("IP"))
			Expect(result).ToNot(HaveKey("PORT"))
		})

		It("should handle nil User map without breaking", func() {
			bundle := config.VariableBundle{
				User: nil,
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKey("global"))
			Expect(result).ToNot(HaveKey("IP"))
		})

		It("should NOT include global key when Global map is nil", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
				Global: nil,
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).ToNot(HaveKey("global"))
		})

		It("should NOT include internal key when Internal map is nil", func() {
			bundle := config.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
				Internal: nil,
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).ToNot(HaveKey("internal"))
		})

		It("should handle completely empty VariableBundle", func() {
			bundle := config.VariableBundle{}

			result := bundle.Flatten()

			Expect(result).ToNot(BeNil())
			Expect(result).ToNot(HaveKey("global"))
			Expect(result).ToNot(HaveKey("internal"))
			Expect(result).To(BeEmpty())
		})
	})

	Describe("Merge", func() {
		It("should copy parent User variables to merged result", func() {
			parent := config.VariableBundle{
				User: map[string]any{
					"IP":   "10.0.0.1",
					"PORT": 502,
				},
			}
			child := config.VariableBundle{}

			result := config.Merge(parent, child)

			Expect(result.User).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(result.User).To(HaveKeyWithValue("PORT", 502))
		})

		It("should copy child User variables to merged result", func() {
			parent := config.VariableBundle{}
			child := config.VariableBundle{
				User: map[string]any{
					"DEVICE_ID": "plc-01",
					"TIMEOUT":   5000,
				},
			}

			result := config.Merge(parent, child)

			Expect(result.User).To(HaveKeyWithValue("DEVICE_ID", "plc-01"))
			Expect(result.User).To(HaveKeyWithValue("TIMEOUT", 5000))
		})

		It("should override parent User variables when child has same keys", func() {
			parent := config.VariableBundle{
				User: map[string]any{
					"IP":   "10.0.0.1",
					"PORT": 502,
				},
			}
			child := config.VariableBundle{
				User: map[string]any{
					"PORT":      503, // Override parent PORT
					"DEVICE_ID": "plc-01",
				},
			}

			result := config.Merge(parent, child)

			Expect(result.User).To(HaveKeyWithValue("IP", "10.0.0.1"))      // From parent
			Expect(result.User).To(HaveKeyWithValue("PORT", 503))           // Child overrides parent
			Expect(result.User).To(HaveKeyWithValue("DEVICE_ID", "plc-01")) // From child
		})

		It("should handle nil parent User map gracefully", func() {
			parent := config.VariableBundle{
				User: nil,
			}
			child := config.VariableBundle{
				User: map[string]any{
					"DEVICE_ID": "plc-01",
				},
			}

			result := config.Merge(parent, child)

			Expect(result.User).ToNot(BeNil())
			Expect(result.User).To(HaveKeyWithValue("DEVICE_ID", "plc-01"))
		})

		It("should handle nil child User map gracefully", func() {
			parent := config.VariableBundle{
				User: map[string]any{
					"IP":   "10.0.0.1",
					"PORT": 502,
				},
			}
			child := config.VariableBundle{
				User: nil,
			}

			result := config.Merge(parent, child)

			Expect(result.User).ToNot(BeNil())
			Expect(result.User).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(result.User).To(HaveKeyWithValue("PORT", 502))
		})

		It("should handle both nil User maps gracefully", func() {
			parent := config.VariableBundle{
				User: nil,
			}
			child := config.VariableBundle{
				User: nil,
			}

			result := config.Merge(parent, child)

			Expect(result.User).ToNot(BeNil())
			Expect(result.User).To(BeEmpty())
		})

		It("should merge User and Global, but NOT Internal (Internal regenerated per-worker)", func() {
			parent := config.VariableBundle{
				User: map[string]any{
					"IP": "10.0.0.1",
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
				Internal: map[string]any{
					"id": "parent-123",
				},
			}
			child := config.VariableBundle{
				User: map[string]any{
					"PORT": 502,
				},
				Global: map[string]any{
					"cluster_id": "prod-cluster",
				},
				Internal: map[string]any{
					"id": "child-456",
				},
			}

			result := config.Merge(parent, child)

			// User variables should be merged (child overrides parent)
			Expect(result.User).To(HaveKeyWithValue("IP", "10.0.0.1"))
			Expect(result.User).To(HaveKeyWithValue("PORT", 502))

			// Global variables should be merged (child overrides parent)
			// Fleet-wide settings should propagate from parent to child
			Expect(result.Global).To(HaveKeyWithValue("api_endpoint", "https://api.example.com"))
			Expect(result.Global).To(HaveKeyWithValue("cluster_id", "prod-cluster"))

			// Internal should NOT be merged (runtime-only, regenerated per-worker by supervisor)
			Expect(result.Internal).To(BeNil())
		})
	})

	Describe("MergeWithOverrides", func() {
		It("should detect when child User variables override parent User variables", func() {
			parent := config.VariableBundle{
				User: map[string]any{
					"IP":   "10.0.0.1",
					"PORT": 502,
				},
			}
			child := config.VariableBundle{
				User: map[string]any{
					"PORT": 503, // Override
				},
			}

			result := config.MergeWithOverrides(parent, child)

			// Should have one override
			Expect(result.Overrides).To(HaveLen(1))
			Expect(result.Overrides[0].Namespace).To(Equal("User"))
			Expect(result.Overrides[0].Key).To(Equal("PORT"))
			Expect(result.Overrides[0].OldValue).To(Equal(502))
			Expect(result.Overrides[0].NewValue).To(Equal(503))
		})

		It("should detect when child Global variables override parent Global variables", func() {
			parent := config.VariableBundle{
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
					"cluster_id":   "prod-cluster",
				},
			}
			child := config.VariableBundle{
				Global: map[string]any{
					"api_endpoint": "https://api.other.com", // Override
				},
			}

			result := config.MergeWithOverrides(parent, child)

			// Should have one override
			Expect(result.Overrides).To(HaveLen(1))
			Expect(result.Overrides[0].Namespace).To(Equal("Global"))
			Expect(result.Overrides[0].Key).To(Equal("api_endpoint"))
			Expect(result.Overrides[0].OldValue).To(Equal("https://api.example.com"))
			Expect(result.Overrides[0].NewValue).To(Equal("https://api.other.com"))
		})

		It("should return empty overrides when no conflicts exist", func() {
			parent := config.VariableBundle{
				User: map[string]any{"IP": "10.0.0.1"},
			}
			child := config.VariableBundle{
				User: map[string]any{"PORT": 502},
			}

			result := config.MergeWithOverrides(parent, child)

			Expect(result.Overrides).To(BeEmpty())
		})
	})

	Describe("Clone", func() {
		It("should deeply clone nested maps so modifications don't affect original", func() {
			// Create original with nested map in User
			original := config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": 502,
					"nested": map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
				},
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
					"settings": map[string]any{
						"timeout": 30,
						"retries": 3,
					},
				},
			}

			// Clone the bundle
			cloned := original.Clone()

			// Modify nested map in cloned User
			nestedClone, ok := cloned.User["nested"].(map[string]any)
			Expect(ok).To(BeTrue())
			nestedClone["key1"] = "modified_value"
			nestedClone["key3"] = "new_value"

			// Modify nested map in cloned Global
			settingsClone, ok := cloned.Global["settings"].(map[string]any)
			Expect(ok).To(BeTrue())
			settingsClone["timeout"] = 60
			settingsClone["new_setting"] = "added"

			// Verify original nested maps are NOT affected
			nestedOriginal, ok := original.User["nested"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(nestedOriginal["key1"]).To(Equal("value1"), "Original nested map should not be modified")
			Expect(nestedOriginal).ToNot(HaveKey("key3"), "Original nested map should not have new keys")

			settingsOriginal, ok := original.Global["settings"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(settingsOriginal["timeout"]).To(Equal(30), "Original Global nested map should not be modified")
			Expect(settingsOriginal).ToNot(HaveKey("new_setting"), "Original Global nested map should not have new keys")
		})

		It("should handle nil maps gracefully", func() {
			original := config.VariableBundle{
				User:   nil,
				Global: nil,
			}

			cloned := original.Clone()

			Expect(cloned.User).To(BeNil())
			Expect(cloned.Global).To(BeNil())
		})

		It("should handle empty maps correctly", func() {
			original := config.VariableBundle{
				User:   map[string]any{},
				Global: map[string]any{},
			}

			cloned := original.Clone()

			Expect(cloned.User).ToNot(BeNil())
			Expect(cloned.User).To(BeEmpty())
			Expect(cloned.Global).ToNot(BeNil())
			Expect(cloned.Global).To(BeEmpty())
		})

		It("should NOT clone Internal map (intentional - regenerated per-worker)", func() {
			// Internal map is intentionally NOT cloned by Clone() because:
			// 1. Internal variables are regenerated per-worker (identity, parent_id, etc.)
			// 2. Cloning Internal would preserve stale identity from parent
			// 3. The supervisor sets fresh Internal values after cloning
			original := config.VariableBundle{
				Internal: map[string]any{
					"id":        "worker-123",
					"parent_id": "parent-456",
				},
			}

			cloned := original.Clone()

			// Verify Internal is nil after cloning (intentional behavior)
			Expect(cloned.Internal).To(BeNil(),
				"Internal should NOT be cloned - it's regenerated per-worker by the supervisor")

			// Verify original is unchanged
			Expect(original.Internal).ToNot(BeNil())
			Expect(original.Internal["id"]).To(Equal("worker-123"))
		})
	})
})
