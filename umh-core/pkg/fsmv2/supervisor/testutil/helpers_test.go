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

package testutil_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
)

var _ = Describe("Testutil Helpers", func() {
	Describe("Worker", func() {
		It("should collect observed state successfully", func() {
			worker := &testutil.Worker{}
			ctx := context.Background()

			observed, err := worker.CollectObservedState(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			state, ok := observed.(*testutil.ObservedState)
			Expect(ok).To(BeTrue())
			Expect(state.ID).To(Equal("test-worker"))
		})
	})

	Describe("CreateTriangularStore", func() {
		It("should create a non-nil store", func() {
			store := testutil.CreateTriangularStore()
			Expect(store).NotTo(BeNil())
		})
	})

	Describe("CreateTriangularStoreForWorkerType", func() {
		It("should create a non-nil store for a worker type", func() {
			store := testutil.CreateTriangularStoreForWorkerType("s6")
			Expect(store).NotTo(BeNil())
		})
	})

	Describe("Identity", func() {
		It("should return a test identity with expected values", func() {
			identity := testutil.Identity()

			Expect(identity.ID).To(Equal("test-worker"))
			Expect(identity.WorkerType).To(Equal("container"))
		})
	})

	Describe("WorkerWithType", func() {
		It("should collect observed state with custom worker type ID", func() {
			worker := &testutil.WorkerWithType{
				WorkerType: "s6",
			}
			ctx := context.Background()

			observed, err := worker.CollectObservedState(ctx)

			Expect(err).NotTo(HaveOccurred())

			state, ok := observed.(*testutil.ObservedState)
			Expect(ok).To(BeTrue())
			Expect(state.ID).To(Equal("s6-worker"))
		})
	})
})
