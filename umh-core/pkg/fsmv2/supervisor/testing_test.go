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

package supervisor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Testing Helpers", func() {
	Describe("CreateTestTriangularStoreForWorkerType", func() {
		It("should create triangular store for s6 workerType", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("s6")
			Expect(store).ToNot(BeNil())

			// Verify collections exist by attempting to save/load data
			ctx := context.Background()
			identity := map[string]interface{}{
				"id":   "test-s6",
				"name": "Test S6",
			}
			err := store.SaveIdentity(ctx, "s6", "test-s6", identity)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create triangular store for container workerType", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("container")
			Expect(store).ToNot(BeNil())

			// Verify collections exist by attempting to save/load data
			ctx := context.Background()
			identity := map[string]interface{}{
				"id":   "test-container",
				"name": "Test Container",
			}
			err := store.SaveIdentity(ctx, "container", "test-container", identity)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should inject correct CSE fields for identity role", func() {
			ctx := context.Background()
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")

			identity := map[string]interface{}{
				"id":   "test-worker",
				"name": "Test Worker",
			}
			err := store.SaveIdentity(ctx, "test", "test-worker", identity)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := store.LoadIdentity(ctx, "test", "test-worker")
			Expect(err).ToNot(HaveOccurred())
			// CSE fields are injected automatically
			Expect(retrieved).To(HaveKey(storage.FieldSyncID))
			Expect(retrieved).To(HaveKey(storage.FieldVersion))
			Expect(retrieved).To(HaveKey(storage.FieldCreatedAt))
		})

		It("should inject correct CSE fields for desired role", func() {
			ctx := context.Background()
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")

			desired := map[string]interface{}{
				"id":    "test-worker",
				"state": "running",
			}
			err := store.SaveDesired(ctx, "test", "test-worker", desired)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := store.LoadDesired(ctx, "test", "test-worker")
			Expect(err).ToNot(HaveOccurred())
			// CSE fields are injected automatically
			// Note: _updated_at is only set on updates, not on first save
			Expect(retrieved).To(HaveKey(storage.FieldSyncID))
			Expect(retrieved).To(HaveKey(storage.FieldVersion))
			Expect(retrieved).To(HaveKey(storage.FieldCreatedAt))
		})

		It("should inject correct CSE fields for observed role", func() {
			ctx := context.Background()
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")

			observed := map[string]interface{}{
				"id":    "test-worker",
				"state": "running",
			}
			_, err := store.SaveObserved(ctx, "test", "test-worker", observed)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := store.LoadObserved(ctx, "test", "test-worker")
			Expect(err).ToNot(HaveOccurred())
			// CSE fields are injected automatically
			// Note: _updated_at is only set on updates, not on first save
			Expect(retrieved).To(HaveKey(storage.FieldSyncID))
			Expect(retrieved).To(HaveKey(storage.FieldVersion))
			Expect(retrieved).To(HaveKey(storage.FieldCreatedAt))
		})

		It("should allow storing and retrieving data", func() {
			ctx := context.Background()
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")

			identity := map[string]interface{}{
				"id":   "worker-1",
				"name": "Test Worker",
			}
			err := store.SaveIdentity(ctx, "test", "worker-1", identity)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := store.LoadIdentity(ctx, "test", "worker-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved["id"]).To(Equal("worker-1"))
			Expect(retrieved["name"]).To(Equal("Test Worker"))
		})
	})

	Describe("TestWorkerWithType", func() {
		It("should return workerType-specific observed state", func() {
			ctx := context.Background()
			worker := &supervisor.TestWorkerWithType{
				WorkerType: "s6",
			}

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(observed).ToNot(BeNil())
		})

		It("should preserve TestWorker behavior when CollectFunc is set", func() {
			ctx := context.Background()
			customObserved := &supervisor.TestObservedState{
				ID: "custom-worker",
			}

			worker := &supervisor.TestWorkerWithType{
				TestWorker: supervisor.TestWorker{
					CollectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
						return customObserved, nil
					},
				},
				WorkerType: "s6",
			}

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(observed).To(Equal(customObserved))
		})

		It("should be backward compatible with existing TestWorker", func() {
			ctx := context.Background()

			oldWorker := &supervisor.TestWorker{
				Observed: &supervisor.TestObservedState{
					ID: "old-worker",
				},
			}

			observed, err := oldWorker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(observed).ToNot(BeNil())
		})
	})
})
