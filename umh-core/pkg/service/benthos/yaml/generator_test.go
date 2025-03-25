package yaml

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
)

var _ = Describe("Benthos YAML Generator", func() {
	type testCase struct {
		config      *config.BenthosServiceConfig
		expected    []string
		notExpected []string
	}

	DescribeTable("generator rendering",
		func(tc testCase) {
			generator := NewGenerator()
			yamlStr, err := generator.RenderConfig(*tc.config)
			Expect(err).NotTo(HaveOccurred())

			// Check for expected strings
			for _, exp := range tc.expected {
				Expect(yamlStr).To(ContainSubstring(exp))
			}

			// Check for strings that should not be present
			for _, notExp := range tc.notExpected {
				Expect(yamlStr).NotTo(ContainSubstring(notExp))
			}
		},
		Entry("should render empty stdout output correctly",
			testCase{
				config: &config.BenthosServiceConfig{
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					MetricsPort: 4195,
					LogLevel:    "INFO",
				},
				expected: []string{
					"output:",
					"  stdout: {}",
					"http:",
					"  address: 0.0.0.0:4195",
					"logger:",
					"  level: INFO",
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render configured output correctly",
			testCase{
				config: &config.BenthosServiceConfig{
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{
							"codec": "lines",
						},
					},
					MetricsPort: 4195,
					LogLevel:    "INFO",
				},
				expected: []string{
					"output:",
					"  stdout:",
					"    codec: lines",
				},
				notExpected: []string{
					"stdin",
				},
			}),
		Entry("should render empty input correctly",
			testCase{
				config: &config.BenthosServiceConfig{
					Input: map[string]interface{}{},
					Output: map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					MetricsPort: 4195,
					LogLevel:    "INFO",
				},
				expected: []string{
					"output:",
					"  stdout: {}",
				},
				notExpected: []string{
					"input: {}", // Empty input should not render as an empty object
				},
			}),
		Entry("should render Kafka input with processor and AWS S3 output",
			testCase{
				config: &config.BenthosServiceConfig{
					Input: map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"kafka:9092"},
							"topics":    []string{"benthos_topic"},
							"consumer_group": map[string]interface{}{
								"session_timeout": "6s",
							},
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
					Output: map[string]interface{}{
						"aws_s3": map[string]interface{}{
							"bucket": "example-bucket",
							"path":   "${!count:files}-${!timestamp_unix_nano}.txt",
						},
					},
					MetricsPort: 4195,
					LogLevel:    "INFO",
				},
				expected: []string{
					"input:",
					"  kafka:",
					"    addresses:",
					"    - kafka:9092",
					"    topics:",
					"    - benthos_topic",
					"    consumer_group:",
					"      session_timeout: 6s",
					"pipeline:",
					"  processors:",
					"  - text:",
					"      operator: to_upper",
					"output:",
					"  aws_s3:",
					"    bucket: example-bucket",
					"    path: ${!count:files}-${!timestamp_unix_nano}.txt",
				},
				notExpected: []string{
					"stdin",
				},
			}),
	)

	// Add package-level function test
	Describe("RenderBenthosYAML package function", func() {
		It("should produce the same output as the Generator", func() {
			// Setup test config
			inputConfig := map[string]interface{}{
				"kafka": map[string]interface{}{
					"addresses": []string{"kafka:9092"},
					"topics":    []string{"test_topic"},
				},
			}

			outputConfig := map[string]interface{}{
				"kafka": map[string]interface{}{
					"addresses": []string{"kafka:9092"},
					"topic":     "output_topic",
				},
			}

			// Use package-level function
			yamlStr1, err := RenderBenthosYAML(
				inputConfig,
				outputConfig,
				nil,
				nil,
				nil,
				nil,
				4195,
				"INFO",
			)
			Expect(err).NotTo(HaveOccurred())

			// Use Generator directly
			cfg := config.BenthosServiceConfig{
				Input:       inputConfig,
				Output:      outputConfig,
				MetricsPort: 4195,
				LogLevel:    "INFO",
			}
			generator := NewGenerator()
			yamlStr2, err := generator.RenderConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Both should produce the same result
			Expect(yamlStr1).To(Equal(yamlStr2))
		})
	})
})
