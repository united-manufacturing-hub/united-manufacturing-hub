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

var _ = Describe("Redpanda YAML Normalizer", func() {
	Describe("NormalizeConfig", func() {
		It("should set default values for empty config", func() {
			config := RedpandaServiceConfig{}
			normalizer := NewNormalizer()

			normalizedConfig := normalizer.NormalizeConfig(config)
			Expect(normalizedConfig.Topic.DefaultTopicRetentionMs).To(Equal(int64(604800000)))
			Expect(normalizedConfig.Topic.DefaultTopicRetentionBytes).To(Equal(int64(0)))
			Expect(normalizedConfig.Topic.DefaultTopicCompressionAlgorithm).To(Equal("snappy"))
		})

		It("should not override existing compression algorithm", func() {
			config := RedpandaServiceConfig{}
			config.Topic.DefaultTopicCompressionAlgorithm = "lz4"
			normalizer := NewNormalizer()

			normalizedConfig := normalizer.NormalizeConfig(config)
			Expect(normalizedConfig.Topic.DefaultTopicCompressionAlgorithm).To(Equal("lz4"))
		})
	})
})
