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

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		mockClock       *clock.Mock
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

		mockClock = clock.NewMock()
		// Initialize mock clock to a realistic time (not Unix epoch)
		// This allows retention=0 tests to work correctly since cutoff time > delta timestamps
		mockClock.Set(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		triangularStore = storage.NewTriangularStoreWithClock(basicStore, deps.NewNopFSMLogger(), mockClock)

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      triangularStore,
			Logger:     deps.NewNopFSMLogger(),
		})
	})

	AfterEach(func() {
		if basicStore != nil {
			_ = basicStore.Close(ctx)
		}
	})

	Describe("CompactDeltas", func() {
		It("should delete all deltas when retention window is zero", func() {
			generateDeltas(ctx, s, 5, 3)

			deltasBeforeCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas before compaction: %d\n", deltasBeforeCompact)
			Expect(deltasBeforeCompact).To(BeNumerically(">", 0),
				"Should have deltas before compaction")

			// Advance clock so deltas are "in the past" relative to current time
			// Must advance at least 1ms since timestamps are stored in milliseconds
			mockClock.Add(1 * time.Millisecond)

			deleted, err := triangularStore.CompactDeltas(ctx, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(deltasBeforeCompact),
				"All deltas should be deleted with zero retention")

			deltasAfterCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas after compaction with 0 retention: %d\n", deltasAfterCompact)
			Expect(deltasAfterCompact).To(Equal(0),
				"No deltas should remain after compaction with zero retention")
		})

		It("should delete deltas older than retention window", func() {
			generateDeltas(ctx, s, 5, 3)

			deltasBeforeCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas before compaction: %d\n", deltasBeforeCompact)
			Expect(deltasBeforeCompact).To(BeNumerically(">", 0),
				"Should have deltas before compaction")

			// Advance mock clock by 15 minutes
			mockClock.Add(15 * time.Minute)

			// Compact with 10-minute retention - deltas are 15 min old, should be deleted
			deleted, err := triangularStore.CompactDeltas(ctx, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(deltasBeforeCompact),
				"All deltas should be deleted as they are older than retention window")

			deltasAfterCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas after compaction with 10min retention (15min old): %d\n", deltasAfterCompact)
			Expect(deltasAfterCompact).To(Equal(0),
				"No deltas should remain after compaction")
		})

		It("should preserve deltas within retention window", func() {
			generateDeltas(ctx, s, 5, 3)

			deltasBeforeCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas before compaction: %d\n", deltasBeforeCompact)
			Expect(deltasBeforeCompact).To(BeNumerically(">", 0),
				"Should have deltas before compaction")

			// Advance mock clock by only 5 minutes
			mockClock.Add(5 * time.Minute)

			// Compact with 10-minute retention - deltas are only 5 min old, should be preserved
			deleted, err := triangularStore.CompactDeltas(ctx, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleted).To(Equal(0),
				"No deltas should be deleted as they are within retention window")

			deltasAfterCompact := countDeltas(ctx, basicStore)
			GinkgoWriter.Printf("Deltas after compaction with 10min retention (5min old): %d\n", deltasAfterCompact)
			Expect(deltasAfterCompact).To(Equal(deltasBeforeCompact),
				"All recent deltas should be preserved within retention window")
		})

		It("should maintain bounded delta count with periodic compaction", func() {
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

				// Advance clock so deltas are "in the past"
				// Must advance at least 1ms since timestamps are stored in milliseconds
				mockClock.Add(1 * time.Millisecond)

				// Compact with zero retention to delete all deltas after each cycle
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
			generateDeltas(ctx, s, 3, 2)

			err := triangularStore.Maintenance(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be idempotent (safe to call multiple times)", func() {
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
	Expect(err).ToNot(HaveOccurred(), "countDeltas failed to query delta collection")

	return len(docs)
}
