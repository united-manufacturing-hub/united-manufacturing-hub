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

package persistence_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Store Interface", func() {
	Describe("Interface Existence", func() {
		It("should have Store interface defined", func() {
			var _ persistence.Store = nil
			Expect(true).To(BeTrue())
		})

		It("should have Document type defined", func() {
			var _ persistence.Document = nil
			Expect(true).To(BeTrue())
		})
	})

	Describe("CreateCollection", func() {
		It("should have correct method signature", func() {
			ctx := context.Background()

			var store persistence.Store
			if store != nil {
				_ = store.CreateCollection(ctx, "test_collection", nil)
			}
			Expect(true).To(BeTrue())
		})
	})

	Describe("Transaction Interface", func() {
		It("should have Tx interface defined", func() {
			var _ persistence.Tx = nil
			Expect(true).To(BeTrue())
		})

		It("should have Tx extend Store interface", func() {
			var tx persistence.Tx
			var _ persistence.Store = tx
			Expect(true).To(BeTrue())
		})
	})
})
