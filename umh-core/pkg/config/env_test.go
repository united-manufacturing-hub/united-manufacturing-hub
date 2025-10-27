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

package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
)

var _ = Describe("Redpanda config hardcoded values", func() {
	Context("when checking hardcoded Redpanda configurations in env.go", func() {
		It("should rely on normalizer for default Redpanda resource values", func() {
			emptyConfig := redpandaserviceconfig.RedpandaServiceConfig{
				Resources: redpandaserviceconfig.ResourcesConfig{},
			}

			normalized := redpandaserviceconfig.NormalizeRedpandaConfig(emptyConfig)

			Expect(normalized.Resources.MaxCores).To(Equal(1))
			Expect(normalized.Resources.MemoryPerCoreInBytes).To(Equal(2147483648))
		})

		It("should rely on normalizer for default Redpanda topic values", func() {
			emptyConfig := redpandaserviceconfig.RedpandaServiceConfig{
				Topic: redpandaserviceconfig.TopicConfig{},
			}

			normalized := redpandaserviceconfig.NormalizeRedpandaConfig(emptyConfig)

			Expect(normalized.Topic.DefaultTopicRetentionMs).To(BeNumerically(">", 0))
			Expect(normalized.Topic.DefaultTopicCompressionAlgorithm).NotTo(BeEmpty())
			Expect(normalized.Topic.DefaultTopicCleanupPolicy).NotTo(BeEmpty())
			Expect(normalized.Topic.DefaultTopicSegmentMs).To(BeNumerically(">", 0))
		})
	})

	Context("when checking that env.go doesn't override Redpanda config", func() {
		It("should preserve user-provided Redpanda resource values through config override", func() {
			configOverride := FullConfig{
				Internal: InternalConfig{
					Redpanda: RedpandaConfig{
						FSMInstanceConfig: FSMInstanceConfig{
							DesiredFSMState: "active",
						},
						RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{
							Topic:     redpandaserviceconfig.TopicConfig{},
							Resources: redpandaserviceconfig.ResourcesConfig{},
						},
					},
				},
			}

			Expect(configOverride.Internal.Redpanda.RedpandaServiceConfig.Resources.MaxCores).To(Equal(0))
			Expect(configOverride.Internal.Redpanda.RedpandaServiceConfig.Resources.MemoryPerCoreInBytes).To(Equal(0))
		})

		It("should preserve user-provided Redpanda topic values through config override", func() {
			configOverride := FullConfig{
				Internal: InternalConfig{
					Redpanda: RedpandaConfig{
						FSMInstanceConfig: FSMInstanceConfig{
							DesiredFSMState: "active",
						},
						RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{
							Topic:     redpandaserviceconfig.TopicConfig{},
							Resources: redpandaserviceconfig.ResourcesConfig{},
						},
					},
				},
			}

			Expect(configOverride.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicRetentionMs).To(Equal(int64(0)))
			Expect(configOverride.Internal.Redpanda.RedpandaServiceConfig.Topic.DefaultTopicCompressionAlgorithm).To(Equal(""))
		})
	})
})
