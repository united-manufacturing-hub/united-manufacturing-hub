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

var _ = Describe("Redpanda YAML Comparator", func() {
	Describe("ConfigsEqual", func() {
		It("should consider identical configs equal", func() {
			config1 := RedpandaServiceConfig{}
			config1.Topic.DefaultTopicRetentionMs = 1000
			config1.Topic.DefaultTopicRetentionBytes = 1000

			config2 := RedpandaServiceConfig{}
			config2.Topic.DefaultTopicRetentionMs = 1000
			config2.Topic.DefaultTopicRetentionBytes = 1000

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeTrue())
		})
		It("should consider different configs unequal", func() {
			config1 := RedpandaServiceConfig{}
			config1.Topic.DefaultTopicRetentionMs = 1000
			config1.Topic.DefaultTopicRetentionBytes = 1000

			config2 := RedpandaServiceConfig{}
			config2.Topic.DefaultTopicRetentionMs = 1000
			config2.Topic.DefaultTopicRetentionBytes = 1001

			comparator := NewComparator()
			equal := comparator.ConfigsEqual(config1, config2)

			Expect(equal).To(BeFalse())
		})
	})

	Describe("ConfigDiff", func() {
		It("should generate readable diff for different configs", func() {
			config1 := RedpandaServiceConfig{}
			config1.Topic.DefaultTopicRetentionMs = 1000
			config1.Topic.DefaultTopicRetentionBytes = 1000

			config2 := RedpandaServiceConfig{}
			config2.Topic.DefaultTopicRetentionMs = 1001
			config2.Topic.DefaultTopicRetentionBytes = 1000

			comparator := NewComparator()
			diff := comparator.ConfigDiff(config1, config2)

			Expect(diff).To(ContainSubstring("Topic.DefaultTopicRetentionMs"))
			Expect(diff).To(ContainSubstring("1000"))
			Expect(diff).To(ContainSubstring("1001"))
		})
	})
})
