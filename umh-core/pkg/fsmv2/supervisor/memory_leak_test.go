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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Delta Compaction", Label("memory-cleanup"), func() {
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

	Describe("CompactDeltas", func() {
		It("should delete all deltas when retention window is zero", func() {
			Skip("ENG-4295: Requires CompactDeltas implementation on TriangularStore")

			generateDeltas(ctx, s, 5, 3)

			deltasBeforeCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas before compaction: %d\n", deltasBeforeCompact)
			Expect(deltasBeforeCompact).To(BeNumerically(">", 0),
				"Should have deltas before compaction")

			deleted, err := triangularStore.CompactDeltas(ctx, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(deltasBeforeCompact),
				"All deltas should be deleted with zero retention")

			deltasAfterCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas after compaction with 0 retention: %d\n", deltasAfterCompact)
			Expect(deltasAfterCompact).To(Equal(0),
				"No deltas should remain after compaction with zero retention")
		})

		It("should preserve recent deltas within retention window", func() {
			Skip("ENG-4295: Requires CompactDeltas implementation on TriangularStore")

			generateDeltas(ctx, s, 5, 3)

			deltasBeforeCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas before compaction: %d\n", deltasBeforeCompact)
			Expect(deltasBeforeCompact).To(BeNumerically(">", 0),
				"Should have deltas before compaction")

			deleted, err := triangularStore.CompactDeltas(ctx, 1*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(0),
				"No deltas should be deleted with 1-hour retention (all are recent)")

			deltasAfterCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas after compaction with 1hr retention: %d\n", deltasAfterCompact)
			Expect(deltasAfterCompact).To(Equal(deltasBeforeCompact),
				"All recent deltas should be preserved within retention window")
		})

		It("should maintain bounded delta count with periodic compaction", func() {
			Skip("ENG-4295: Requires CompactDeltas implementation on TriangularStore")

			const numCycles = 100
			const ticksPerWorker = 3
			const maxExpectedDeltas = 50

			for cycle := range numCycles {
				workerID := fmt.Sprintf("compaction-test-worker-%d", cycle)
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

				_, err = triangularStore.CompactDeltas(ctx, 0)
				Expect(err).ToNot(HaveOccurred())
			}

			finalDeltaCount := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Final delta count after %d cycles with compaction: %d\n", numCycles, finalDeltaCount)

			Expect(finalDeltaCount).To(BeNumerically("<=", maxExpectedDeltas),
				"Delta count should be bounded when compaction runs after each cycle")
		})
	})

	Describe("Maintenance", func() {
		It("should clear internal caches without error", func() {
			Skip("ENG-4295: Requires Maintenance implementation on TriangularStore")

			generateDeltas(ctx, s, 3, 2)

			err := triangularStore.Maintenance(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be idempotent (safe to call multiple times)", func() {
			Skip("ENG-4295: Requires Maintenance implementation on TriangularStore")

			for range 3 {
				err := triangularStore.Maintenance(ctx)
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})
})

func generateDeltas(ctx context.Context, s *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState], numWorkers, ticksPerWorker int) {
	for i := range numWorkers {
		workerID := fmt.Sprintf("delta-gen-worker-%d", i)
		identity := deps.Identity{ID: workerID, Name: fmt.Sprintf("Worker %d", i)}
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
}

func countDeltas(ctx context.Context, store persistence.Store) int {
	docs, err := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
	if err != nil {
		return 0
	}

	return len(docs)
}
