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

package graphql

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

var _ = Describe("GraphQL Resolver", func() {
	var (
		resolver    *Resolver
		cache       *topicbrowser.Cache
		snapshotMgr *fsm.SnapshotManager
	)

	BeforeEach(func() {
		cache = topicbrowser.NewCache()
		snapshotMgr = &fsm.SnapshotManager{}
		resolver = &Resolver{
			SnapshotManager:   snapshotMgr,
			TopicBrowserCache: cache,
		}
	})

	Context("Topics Query", func() {
		It("should handle empty cache", func() {
			ctx := context.Background()
			queryRes := resolver.Query()
			topics, err := queryRes.Topics(ctx, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(topics).To(BeEmpty())
		})

		It("should handle limit parameter", func() {
			ctx := context.Background()
			queryRes := resolver.Query()
			limit := 5
			topics, err := queryRes.Topics(ctx, nil, &limit)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(topics)).To(BeNumerically("<=", 5))
		})

		It("should handle filter parameter", func() {
			ctx := context.Background()
			queryRes := resolver.Query()
			filter := &TopicFilter{
				Text: stringPtr("test"),
			}
			topics, err := queryRes.Topics(ctx, filter, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(topics).To(BeEmpty())
		})
	})

	Context("Topic Query", func() {
		It("should handle non-existent topic", func() {
			ctx := context.Background()
			queryRes := resolver.Query()
			topic, err := queryRes.Topic(ctx, "non.existent.topic")

			Expect(err).NotTo(HaveOccurred())
			Expect(topic).To(BeNil())
		})
	})
})

// Helper function
func stringPtr(s string) *string {
	return &s
}
