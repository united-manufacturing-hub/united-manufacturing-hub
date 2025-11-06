package types_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

var _ = Describe("VariableBundle", func() {
	Describe("YAML serialization", func() {
		It("should serialize User and Global, but exclude Internal", func() {
			bundle := types.VariableBundle{
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
			var bundle types.VariableBundle
			err := yaml.Unmarshal([]byte(yamlData), &bundle)

			Expect(err).ToNot(HaveOccurred())
			Expect(bundle.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(bundle.User).To(HaveKeyWithValue("PORT", 502))
			Expect(bundle.Global).To(HaveKeyWithValue("api_endpoint", "https://api.example.com"))
			Expect(bundle.Internal).To(BeNil())
		})

		It("should handle empty VariableBundle correctly", func() {
			bundle := types.VariableBundle{}

			data, err := yaml.Marshal(bundle)
			Expect(err).ToNot(HaveOccurred())

			yamlStr := string(data)
			Expect(yamlStr).To(Equal("{}\n"))
		})

		It("should round-trip through YAML correctly", func() {
			original := types.VariableBundle{
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

			var roundtripped types.VariableBundle
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
			bundle := types.VariableBundle{
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
			original := types.VariableBundle{
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

			var roundtripped types.VariableBundle
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
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
			bundle := types.VariableBundle{
				User: nil,
				Global: map[string]any{
					"api_endpoint": "https://api.example.com",
				},
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKey("global"))
			Expect(result).ToNot(HaveKey("IP"))
		})

		It("should set global key to nil when Global map is nil", func() {
			bundle := types.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
				Global: nil,
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).To(HaveKey("global"))
			Expect(result["global"]).To(BeNil())
		})

		It("should set internal key to nil when Internal map is nil", func() {
			bundle := types.VariableBundle{
				User: map[string]any{
					"IP": "192.168.1.100",
				},
				Internal: nil,
			}

			result := bundle.Flatten()

			Expect(result).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(result).To(HaveKey("internal"))
			Expect(result["internal"]).To(BeNil())
		})

		It("should handle completely empty VariableBundle", func() {
			bundle := types.VariableBundle{}

			result := bundle.Flatten()

			Expect(result).ToNot(BeNil())
			Expect(result).To(HaveKey("global"))
			Expect(result).To(HaveKey("internal"))
			Expect(result["global"]).To(BeNil())
			Expect(result["internal"]).To(BeNil())
		})
	})
})
