package yaml

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("Benthos YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := config.BenthosServiceConfig{}
			normalizer := NewNormalizer()

			normalizedConfig := normalizer.NormalizeConfig(config)

			Expect(normalizedConfig.Input).NotTo(BeNil())
			Expect(normalizedConfig.Output).NotTo(BeNil())
			Expect(normalizedConfig.Pipeline).NotTo(BeNil())
			Expect(normalizedConfig.MetricsPort).To(Equal(4195))
			Expect(normalizedConfig.LogLevel).To(Equal("INFO"))
		})

		It("should preserve existing values", func() {
			config := config.BenthosServiceConfig{
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
				Pipeline: map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"text": map[string]interface{}{
								"operator": "to_upper",
							},
						},
					},
				},
				MetricsPort: 8000,
				LogLevel:    "DEBUG",
			}

			normalizer := NewNormalizer()
			normalizedConfig := normalizer.NormalizeConfig(config)

			Expect(normalizedConfig.MetricsPort).To(Equal(8000))
			Expect(normalizedConfig.LogLevel).To(Equal("DEBUG"))

			// Check input preserved
			inputMqtt := normalizedConfig.Input["mqtt"].(map[string]interface{})
			Expect(inputMqtt["topic"]).To(Equal("test/topic"))

			// Check output preserved
			outputKafka := normalizedConfig.Output["kafka"].(map[string]interface{})
			Expect(outputKafka["topic"]).To(Equal("test-output"))

			// Check pipeline processors preserved
			processors := normalizedConfig.Pipeline["processors"].([]interface{})
			Expect(processors).To(HaveLen(1))
			processor := processors[0].(map[string]interface{})
			processorText := processor["text"].(map[string]interface{})
			Expect(processorText["operator"]).To(Equal("to_upper"))
		})

		It("should normalize maps by ensuring they're not nil", func() {
			config := config.BenthosServiceConfig{
				// Input is nil
				// Output is nil
				Pipeline: map[string]interface{}{}, // Empty but not nil
				// Buffer is nil
				// CacheResources is nil
				// RateLimitResources is nil
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}

			normalizer := NewNormalizer()
			normalizedConfig := normalizer.NormalizeConfig(config)

			Expect(normalizedConfig.Input).NotTo(BeNil())
			Expect(normalizedConfig.Output).NotTo(BeNil())
			Expect(normalizedConfig.Pipeline).NotTo(BeNil())

			// Buffer should have the none buffer set
			Expect(normalizedConfig.Buffer).To(HaveKey("none"))

			// These should be empty
			Expect(normalizedConfig.CacheResources).To(BeEmpty())
			Expect(normalizedConfig.RateLimitResources).To(BeEmpty())
		})
	})

	// Test the package-level function
	Describe("NormalizeBenthosConfig package function", func() {
		It("should use the default normalizer", func() {
			config := config.BenthosServiceConfig{}

			// Use package-level function
			normalizedConfig1 := NormalizeBenthosConfig(config)

			// Use normalizer directly
			normalizer := NewNormalizer()
			normalizedConfig2 := normalizer.NormalizeConfig(config)

			// Results should be the same
			Expect(normalizedConfig1.MetricsPort).To(Equal(normalizedConfig2.MetricsPort))
			Expect(normalizedConfig1.LogLevel).To(Equal(normalizedConfig2.LogLevel))
			Expect(normalizedConfig1.Input).NotTo(BeNil())
			Expect(normalizedConfig1.Output).NotTo(BeNil())
		})
	})
})
