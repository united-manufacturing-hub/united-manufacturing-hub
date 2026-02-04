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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Memory Leak Characterization", func() {
	var (
		s               *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		triangularStore *storage.TriangularStore
		basicStore      persistence.Store
		ctx             context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error

		basicStore = memory.NewInMemoryStore()

		err = basicStore.CreateCollection(ctx, "test_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_observed", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, storage.DeltaCollectionName, nil)
		Expect(err).ToNot(HaveOccurred())

		triangularStore = storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
		})
	})

	AfterEach(func() {
		if basicStore != nil {
			_ = basicStore.Close(ctx)
		}
	})

	Describe("Delta Entry Accumulation", Label("memory-leak"), func() {
		It("should demonstrate unbounded delta growth after repeated worker create/remove cycles", func() {
			const numCycles = 50
			const ticksPerWorker = 5

			initialDeltaCount := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Initial delta count: %d\n", initialDeltaCount)

			for cycle := range numCycles {
				workerID := fmt.Sprintf("leak-test-worker-%d", cycle)
				identity := deps.Identity{ID: workerID, Name: fmt.Sprintf("Worker %d", cycle)}
				worker := &mockWorker{observed: createMockObservedStateWithID(workerID)}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				for range ticksPerWorker {
					err = s.TestTickAll(ctx)
					Expect(err).ToNot(HaveOccurred())
				}

				err = s.RemoveWorker(ctx, workerID)
				Expect(err).ToNot(HaveOccurred())
			}

			finalDeltaCount := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Final delta count after %d cycles: %d\n", numCycles, finalDeltaCount)
			GinkgoWriter.Printf("Delta growth: %d entries\n", finalDeltaCount-initialDeltaCount)

			Expect(finalDeltaCount).To(BeNumerically(">", initialDeltaCount),
				"Expected delta count to grow after worker cycles (demonstrating the leak)")

			expectedMinGrowth := numCycles * ticksPerWorker / 2
			Expect(finalDeltaCount-initialDeltaCount).To(BeNumerically(">=", expectedMinGrowth),
				"Expected significant delta growth indicating memory leak")
		})

		It("should demonstrate worker document accumulation after removal", func() {
			const numWorkers = 20

			for i := range numWorkers {
				workerID := fmt.Sprintf("doc-leak-worker-%d", i)
				identity := deps.Identity{ID: workerID, Name: fmt.Sprintf("Worker %d", i)}
				worker := &mockWorker{observed: createMockObservedStateWithID(workerID)}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				err = s.TestTickAll(ctx)
				Expect(err).ToNot(HaveOccurred())

				err = s.RemoveWorker(ctx, workerID)
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(s.ListWorkers()).To(BeEmpty(), "All workers should be removed from supervisor")

			identityDocs := countDocuments(ctx, basicStore, "test_identity")
			desiredDocs := countDocuments(ctx, basicStore, "test_desired")
			observedDocs := countDocuments(ctx, basicStore, "test_observed")

			GinkgoWriter.Printf("After removing %d workers:\n", numWorkers)
			GinkgoWriter.Printf("  Identity documents: %d\n", identityDocs)
			GinkgoWriter.Printf("  Desired documents: %d\n", desiredDocs)
			GinkgoWriter.Printf("  Observed documents: %d\n", observedDocs)

			totalDocs := identityDocs + desiredDocs + observedDocs
			GinkgoWriter.Printf("  Total worker documents: %d (expected 0 after cleanup)\n", totalDocs)

			Expect(totalDocs).To(BeNumerically(">", 0),
				"Expected worker documents to remain after removal (demonstrating the leak)")
		})
	})

	Describe("Memory Cleanup Verification", Label("memory-cleanup"), func() {
		It("should have bounded memory after cleanup is implemented", func() {
			Skip("This test will pass after ENG-4295 implements storage cleanup")

			const numCycles = 100
			const ticksPerWorker = 3
			const maxExpectedDeltas = 1000

			for cycle := range numCycles {
				workerID := fmt.Sprintf("cleanup-test-worker-%d", cycle)
				identity := deps.Identity{ID: workerID, Name: fmt.Sprintf("Worker %d", cycle)}
				worker := &mockWorker{observed: createMockObservedStateWithID(workerID)}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				for range ticksPerWorker {
					err = s.TestTickAll(ctx)
					Expect(err).ToNot(HaveOccurred())
				}

				err = s.RemoveWorker(ctx, workerID)
				Expect(err).ToNot(HaveOccurred())
			}

			finalDeltaCount := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Final delta count after %d cycles with cleanup: %d\n", numCycles, finalDeltaCount)

			Expect(finalDeltaCount).To(BeNumerically("<=", maxExpectedDeltas),
				"Expected bounded delta count after cleanup implementation")

			identityDocs := countDocuments(ctx, basicStore, "test_identity")
			desiredDocs := countDocuments(ctx, basicStore, "test_desired")
			observedDocs := countDocuments(ctx, basicStore, "test_observed")

			Expect(identityDocs).To(Equal(0), "Expected no orphaned identity documents")
			Expect(desiredDocs).To(Equal(0), "Expected no orphaned desired documents")
			Expect(observedDocs).To(Equal(0), "Expected no orphaned observed documents")
		})
	})
})

func countDeltas(ctx context.Context, store persistence.Store) int {
	docs, err := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
	if err != nil {
		return 0
	}

	return len(docs)
}

func countDocuments(ctx context.Context, store persistence.Store, collectionName string) int {
	docs, err := store.Find(ctx, collectionName, persistence.Query{})
	if err != nil {
		return 0
	}

	return len(docs)
}
