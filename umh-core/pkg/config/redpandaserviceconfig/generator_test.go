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

package redpandaserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Redpanda YAML Generator", func() {
	type testCase struct {
		config      *RedpandaServiceConfig
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
		Entry("should render empty paths correctly",
			testCase{
				config: func() *RedpandaServiceConfig {
					cfg := &RedpandaServiceConfig{}
					cfg.Topic.DefaultTopicRetentionMs = 0
					cfg.Topic.DefaultTopicRetentionBytes = 0
					return cfg
				}(),
				expected: []string{
					"retention_ms: 604800000",
					"retention_bytes: null",
				},
			}),
		Entry("should render configured paths correctly",
			testCase{
				config: func() *RedpandaServiceConfig {
					cfg := &RedpandaServiceConfig{}
					cfg.Topic.DefaultTopicRetentionMs = 1000
					cfg.Topic.DefaultTopicRetentionBytes = 1000
					cfg.Topic.DefaultTopicCompressionAlgorithm = "lz4"
					return cfg
				}(),
				expected: []string{
					"retention_ms: 1000",
					"retention_bytes: 1000",
					"log_compression_type: \"lz4\"",
				},
			}),
		Entry("should render default compression algorithm",
			testCase{
				config: func() *RedpandaServiceConfig {
					cfg := &RedpandaServiceConfig{}
					cfg.Topic.DefaultTopicRetentionMs = 1000
					cfg.Topic.DefaultTopicRetentionBytes = 1000
					return cfg
				}(),
				expected: []string{
					"retention_ms: 1000",
					"retention_bytes: 1000",
					"log_compression_type: \"snappy\"",
				},
			}),
	)

	It("should generate valid YAML with custom compression", func() {
		cfg := RedpandaServiceConfig{}
		cfg.Topic.DefaultTopicRetentionMs = 1000
		cfg.Topic.DefaultTopicRetentionBytes = 1000
		cfg.Topic.DefaultTopicCompressionAlgorithm = "lz4"
		cfg.Topic.DefaultTopicCleanupPolicy = "delete"
		cfg.Topic.DefaultTopicSegmentMs = 604800000

		generator := NewGenerator()
		yaml, err := generator.RenderConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml).To(ContainSubstring("log_compression_type: \"lz4\""))
		Expect(yaml).To(ContainSubstring("log_cleanup_policy: \"delete\""))
		Expect(yaml).To(ContainSubstring("log_segment_ms: 604800000"))
	})

	It("should use default cleanup policy when empty", func() {
		cfg := RedpandaServiceConfig{}
		cfg.Topic.DefaultTopicRetentionMs = 1000
		cfg.Topic.DefaultTopicRetentionBytes = 1000

		generator := NewGenerator()
		yaml, err := generator.RenderConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml).To(ContainSubstring("log_cleanup_policy: \"compact\""))
	})

	It("should use default segment ms when empty", func() {
		cfg := RedpandaServiceConfig{}
		cfg.Topic.DefaultTopicRetentionMs = 1000
		cfg.Topic.DefaultTopicRetentionBytes = 1000

		generator := NewGenerator()
		yaml, err := generator.RenderConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml).To(ContainSubstring("log_segment_ms: 3600000"))
	})
})
