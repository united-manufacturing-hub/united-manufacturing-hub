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

package storage_test

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func expectDeltaReportsNoTombstone(changes *storage.Diff) {
	ExpectWithOffset(1, changes).NotTo(BeNil())

	ExpectWithOffset(1, changes.Added).NotTo(HaveKey(storage.FieldDeletedAt))
	ExpectWithOffset(1, changes.Added).NotTo(HaveKey(storage.FieldDeletedBy))
	ExpectWithOffset(1, changes.Modified).NotTo(HaveKey(storage.FieldDeletedAt))
	ExpectWithOffset(1, changes.Modified).NotTo(HaveKey(storage.FieldDeletedBy))
	ExpectWithOffset(1, changes.Removed).NotTo(ContainElement(storage.FieldDeletedAt))
	ExpectWithOffset(1, changes.Removed).NotTo(ContainElement(storage.FieldDeletedBy))
}

func onlyDeltaAfter(ctx context.Context, ts *storage.TriangularStore, syncID int64) storage.Delta {
	resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: syncID})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.RequiresBootstrap).To(BeFalse())
	ExpectWithOffset(1, resp.Deltas).To(HaveLen(1))

	return resp.Deltas[0]
}

func expectNoDeltaAfter(ctx context.Context, ts *storage.TriangularStore, syncID int64) {
	resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: syncID})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.RequiresBootstrap).To(BeFalse())
	ExpectWithOffset(1, resp.Deltas).To(BeEmpty())
}

func saveInitialDocuments(ctx context.Context, ts *storage.TriangularStore, workerType, workerID string) {
	ExpectWithOffset(1, ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
		"id":   workerID,
		"name": "Container A",
	})).To(Succeed())

	_, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
		"id":     workerID,
		"config": "production",
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	_, err = ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
		"id":     workerID,
		"status": "running",
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

var _ = Describe("Save keeps a tombstone", func() {
	const workerType = "container"

	var (
		ctx       context.Context
		mockClock *clock.Mock
		t0        time.Time
		backend   *mockStore
		ts        *storage.TriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()

		mockClock = clock.NewMock()
		t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		mockClock.Set(t0)

		backend = newMockStore()
		ts = storage.NewTriangularStoreWithClock(backend, deps.NewNopFSMLogger(), mockClock)
	})

	It("carries a stored tombstone through every save and never writes one of its own", func() {
		By("keeping the tombstone when a deleted worker's document is saved with changes")

		const deletedID = "worker-1"
		saveInitialDocuments(ctx, ts, workerType, deletedID)

		Expect(ts.Tombstone(ctx, workerType, deletedID, "removed")).To(Succeed())

		mockClock.Add(time.Hour)
		laterMarkTime := mockClock.Now()

		changedSaves := []struct {
			role            string
			save            func() error
			collection      string
			field           string
			value           interface{}
			reportedAsAdded bool
		}{
			{
				role: storage.RoleDesired,
				save: func() error {
					_, err := ts.SaveDesired(ctx, workerType, deletedID, persistence.Document{
						"id":     deletedID,
						"config": "staging",
					})

					return err
				},
				collection: workerType + "_" + storage.RoleDesired,
				field:      "config",
				value:      "staging",
			},
			{
				role: storage.RoleObserved,
				save: func() error {
					_, err := ts.SaveObserved(ctx, workerType, deletedID, persistence.Document{
						"id":           deletedID,
						"status":       "running",
						"collected_at": t0.Add(2 * time.Hour),
					})

					return err
				},
				collection:      workerType + "_" + storage.RoleObserved,
				field:           "collected_at",
				value:           t0.Add(2 * time.Hour),
				reportedAsAdded: true,
			},
		}

		for _, s := range changedSaves {
			syncBefore, err := ts.GetLatestSyncID(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.save()).To(Succeed())

			stored, err := backend.Get(ctx, s.collection, deletedID)
			Expect(err).NotTo(HaveOccurred())
			expectTombstone(stored, t0, "removed")
			Expect(stored[s.field]).To(Equal(s.value))

			delta := onlyDeltaAfter(ctx, ts, syncBefore)
			Expect(delta.Role).To(Equal(s.role))
			expectDeltaReportsNoTombstone(delta.Changes)

			if s.reportedAsAdded {
				Expect(delta.Changes.Added).To(HaveKey(s.field))
			} else {
				Expect(delta.Changes.Modified).To(HaveKey(s.field))
			}
		}

		By("keeping the tombstone and appending no delta for an unchanged save")

		const unchangedID = "worker-2"
		saveInitialDocuments(ctx, ts, workerType, unchangedID)

		Expect(ts.Tombstone(ctx, workerType, unchangedID, "removed")).To(Succeed())

		syncAfterDelete, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.SaveIdentity(ctx, workerType, unchangedID, persistence.Document{
			"id":   unchangedID,
			"name": "Container A",
		})).To(Succeed())

		changed, err := ts.SaveDesired(ctx, workerType, unchangedID, persistence.Document{
			"id":     unchangedID,
			"config": "production",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		changed, err = ts.SaveObserved(ctx, workerType, unchangedID, persistence.Document{
			"id":     unchangedID,
			"status": "running",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		By("keeping the stored tombstone when an unchanged save carries a tombstone of its own")

		changed, err = ts.SaveDesired(ctx, workerType, unchangedID, persistence.Document{
			"id":                   unchangedID,
			"config":               "production",
			storage.FieldDeletedAt: t0.Add(9 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		changed, err = ts.SaveObserved(ctx, workerType, unchangedID, persistence.Document{
			"id":                   unchangedID,
			"status":               "running",
			storage.FieldDeletedAt: t0.Add(9 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		expectNoDeltaAfter(ctx, ts, syncAfterDelete)

		for _, role := range []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved} {
			stored, err := backend.Get(ctx, workerType+"_"+role, unchangedID)
			Expect(err).NotTo(HaveOccurred())
			expectTombstone(stored, laterMarkTime, "removed")
		}

		By("dropping an incoming tombstone when the stored document has none")

		const liveID = "worker-3"
		saveInitialDocuments(ctx, ts, workerType, liveID)

		syncBeforeLive, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = ts.SaveObserved(ctx, workerType, liveID, persistence.Document{
			"id":                   liveID,
			"status":               "running",
			"collected_at":         t0.Add(4 * time.Hour),
			storage.FieldDeletedAt: t0.Add(5 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())

		liveStored, err := backend.Get(ctx, workerType+"_"+storage.RoleObserved, liveID)
		Expect(err).NotTo(HaveOccurred())
		Expect(liveStored).NotTo(HaveKey(storage.FieldDeletedAt))
		Expect(liveStored).NotTo(HaveKey(storage.FieldDeletedBy))
		Expect(liveStored["collected_at"]).To(Equal(t0.Add(4 * time.Hour)))

		liveDelta := onlyDeltaAfter(ctx, ts, syncBeforeLive)
		Expect(liveDelta.Role).To(Equal(storage.RoleObserved))
		expectDeltaReportsNoTombstone(liveDelta.Changes)
		Expect(liveDelta.Changes.Added).To(HaveKey("collected_at"))

		syncAfterLive, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		changed, err = ts.SaveDesired(ctx, workerType, liveID, persistence.Document{
			"id":                   liveID,
			"config":               "production",
			storage.FieldDeletedAt: t0.Add(5 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		changed, err = ts.SaveObserved(ctx, workerType, liveID, persistence.Document{
			"id":                   liveID,
			"status":               "running",
			"collected_at":         t0.Add(4 * time.Hour),
			storage.FieldDeletedAt: t0.Add(5 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		expectNoDeltaAfter(ctx, ts, syncAfterLive)

		for _, role := range []string{storage.RoleDesired, storage.RoleObserved} {
			stored, err := backend.Get(ctx, workerType+"_"+role, liveID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored).NotTo(HaveKey(storage.FieldDeletedAt))
			Expect(stored).NotTo(HaveKey(storage.FieldDeletedBy))
		}

		By("keeping the stored tombstone over one a save tries to bring in")

		const lateID = "worker-4"
		saveInitialDocuments(ctx, ts, workerType, lateID)

		Expect(ts.Tombstone(ctx, workerType, lateID, "removed")).To(Succeed())

		syncBeforeLate, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		mockClock.Add(2 * time.Hour)

		_, err = ts.SaveObserved(ctx, workerType, lateID, persistence.Document{
			"id":                   lateID,
			"status":               "running",
			"collected_at":         t0.Add(6 * time.Hour),
			storage.FieldDeletedAt: t0.Add(3 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		lateStored, err := backend.Get(ctx, workerType+"_"+storage.RoleObserved, lateID)
		Expect(err).NotTo(HaveOccurred())
		expectTombstone(lateStored, laterMarkTime, "removed")
		Expect(lateStored["collected_at"]).To(Equal(t0.Add(6 * time.Hour)))

		lateDelta := onlyDeltaAfter(ctx, ts, syncBeforeLate)
		Expect(lateDelta.Role).To(Equal(storage.RoleObserved))
		expectDeltaReportsNoTombstone(lateDelta.Changes)
		Expect(lateDelta.Changes.Added).To(HaveKey("collected_at"))
	})

	It("drops a tombstone a first save brings in, out of the document and its creation delta", func() {
		const freshID = "worker-5"

		syncBefore, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = ts.SaveObserved(ctx, workerType, freshID, persistence.Document{
			"id":                   freshID,
			"status":               "running",
			"collected_at":         t0.Add(7 * time.Hour),
			storage.FieldDeletedAt: t0.Add(8 * time.Hour),
			storage.FieldDeletedBy: "collector",
		})
		Expect(err).NotTo(HaveOccurred())

		freshStored, err := backend.Get(ctx, workerType+"_"+storage.RoleObserved, freshID)
		Expect(err).NotTo(HaveOccurred())
		Expect(freshStored).NotTo(HaveKey(storage.FieldDeletedAt))
		Expect(freshStored).NotTo(HaveKey(storage.FieldDeletedBy))
		Expect(freshStored["collected_at"]).To(Equal(t0.Add(7 * time.Hour)))

		freshDelta := onlyDeltaAfter(ctx, ts, syncBefore)
		Expect(freshDelta.Role).To(Equal(storage.RoleObserved))
		expectDeltaReportsNoTombstone(freshDelta.Changes)
		Expect(freshDelta.Changes.Added).To(HaveKey("status"))
		Expect(freshDelta.Changes.Added).To(HaveKey("collected_at"))
	})
})
