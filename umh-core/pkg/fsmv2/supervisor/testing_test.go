// Copyright 2025 UMH Systems GmbH
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

			registry := store.Registry()
			Expect(registry.IsRegistered("s6_identity")).To(BeTrue())
			Expect(registry.IsRegistered("s6_desired")).To(BeTrue())
			Expect(registry.IsRegistered("s6_observed")).To(BeTrue())
		})

		It("should create triangular store for container workerType", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("container")
			Expect(store).ToNot(BeNil())

			registry := store.Registry()
			Expect(registry.IsRegistered("container_identity")).To(BeTrue())
			Expect(registry.IsRegistered("container_desired")).To(BeTrue())
			Expect(registry.IsRegistered("container_observed")).To(BeTrue())
		})

		It("should use correct CSEFields for identity role", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")
			registry := store.Registry()

			metadata, err := registry.Get("test_identity")
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.CSEFields).To(ConsistOf(
				storage.FieldSyncID,
				storage.FieldVersion,
				storage.FieldCreatedAt,
			))
		})

		It("should use correct CSEFields for desired role", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")
			registry := store.Registry()

			metadata, err := registry.Get("test_desired")
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.CSEFields).To(ConsistOf(
				storage.FieldSyncID,
				storage.FieldVersion,
				storage.FieldCreatedAt,
				storage.FieldUpdatedAt,
			))
		})

		It("should use correct CSEFields for observed role", func() {
			store := supervisor.CreateTestTriangularStoreForWorkerType("test")
			registry := store.Registry()

			metadata, err := registry.Get("test_observed")
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.CSEFields).To(ConsistOf(
				storage.FieldSyncID,
				storage.FieldVersion,
				storage.FieldCreatedAt,
				storage.FieldUpdatedAt,
			))
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
