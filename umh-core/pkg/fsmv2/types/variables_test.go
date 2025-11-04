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
})
