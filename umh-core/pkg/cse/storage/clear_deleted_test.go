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
	"errors"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func expectClearedDocument(before, after persistence.Document, clearingSyncID int64) {
	ExpectWithOffset(1, after).NotTo(HaveKey(storage.FieldDeletedAt))
	ExpectWithOffset(1, after).NotTo(HaveKey(storage.FieldDeletedBy))

	expected := make(persistence.Document, len(before))
	for key, value := range before {
		if key == storage.FieldDeletedAt || key == storage.FieldDeletedBy {
			continue
		}

		expected[key] = value
	}

	expected[storage.FieldSyncID] = clearingSyncID

	ExpectWithOffset(1, after).To(Equal(expected),
		"every field but the tombstone keys must survive ClearDeleted")
}

func expectDeltaDescribesClearing(delta storage.Delta) {
	ExpectWithOffset(1, delta.Changes).NotTo(BeNil())
	ExpectWithOffset(1, delta.Changes.Added).To(BeEmpty())
	ExpectWithOffset(1, delta.Changes.Modified).To(BeEmpty())
	ExpectWithOffset(1, delta.Changes.Removed).To(ConsistOf(storage.FieldDeletedAt, storage.FieldDeletedBy))
}

// deltaRecordingStore exists because a TriangularStore built over a failing
// backend has its own sync id counter, so GetDeltas cannot show what it wrote.
type deltaRecordingStore struct {
	persistence.Store

	mu     sync.Mutex
	deltas []persistence.Document
}

func (s *deltaRecordingStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	if collection == storage.DeltaCollectionName {
		s.mu.Lock()
		s.deltas = append(s.deltas, doc)
		s.mu.Unlock()
	}

	return s.Store.Insert(ctx, collection, doc)
}

func (s *deltaRecordingStore) deltaCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.deltas)
}

var _ = Describe("ClearDeleted", func() {
	const workerType = "container"

	var (
		ctx         context.Context
		mockClock   *clock.Mock
		t0          time.Time
		backend     *mockStore
		ts          *storage.TriangularStore
		collections map[string]string
	)

	readRaw := func(id string) map[string]persistence.Document {
		raw := make(map[string]persistence.Document)

		for role, collection := range collections {
			doc, err := backend.Get(ctx, collection, id)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			raw[role] = doc
		}

		return raw
	}

	clearingDeltasByRole := func(afterSyncID int64, workerID string) map[string]storage.Delta {
		resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: afterSyncID})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, resp.RequiresBootstrap).To(BeFalse())

		byRole := make(map[string]storage.Delta)

		for _, delta := range resp.Deltas {
			ExpectWithOffset(1, delta.WorkerType).To(Equal(workerType))
			ExpectWithOffset(1, delta.WorkerID).To(Equal(workerID))
			ExpectWithOffset(1, byRole).NotTo(HaveKey(delta.Role), "role %s reported twice", delta.Role)
			expectDeltaDescribesClearing(delta)
			byRole[delta.Role] = delta
		}

		return byRole
	}

	BeforeEach(func() {
		ctx = context.Background()

		mockClock = clock.NewMock()
		t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		mockClock.Set(t0)

		backend = newMockStore()
		ts = storage.NewTriangularStoreWithClock(backend, deps.NewNopFSMLogger(), mockClock)

		collections = map[string]string{
			storage.RoleIdentity: workerType + "_identity",
			storage.RoleDesired:  workerType + "_desired",
			storage.RoleObserved: workerType + "_observed",
		}
	})

	It("removes the tombstone from every role document and keeps every other field", func() {
		const workerID = "worker-1"
		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		tombstoned := readRaw(workerID)

		syncBefore, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())

		deltas := clearingDeltasByRole(syncBefore, workerID)
		Expect(deltas).To(HaveLen(3))

		cleared := readRaw(workerID)

		for role := range collections {
			Expect(deltas).To(HaveKey(role))
			expectClearedDocument(tombstoned[role], cleared[role], deltas[role].SyncID)
		}

		syncAfter, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(syncAfter).To(Equal(syncBefore + 3))
	})

	It("serves the cleared documents through the snapshot read", func() {
		const workerID = "worker-1"
		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())

		snap, err := ts.LoadSnapshot(ctx, workerType, workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(snap.Identity).NotTo(HaveKey(storage.FieldDeletedAt))
		Expect(snap.Desired).NotTo(HaveKey(storage.FieldDeletedAt))

		observed, ok := snap.Observed.(persistence.Document)
		Expect(ok).To(BeTrue())
		Expect(observed).NotTo(HaveKey(storage.FieldDeletedAt))
	})

	It("writes nothing for a worker without a tombstone", func() {
		const workerID = "worker-2"
		saveInitialDocuments(ctx, ts, workerType, workerID)

		before := readRaw(workerID)

		syncBefore, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())

		after := readRaw(workerID)
		for role := range collections {
			Expect(after[role]).To(Equal(before[role]), "the %s document must not change", role)
		}

		expectNoDeltaAfter(ctx, ts, syncBefore)

		syncAfter, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(syncAfter).To(Equal(syncBefore))
	})

	It("clears only the documents a partly stored worker has", func() {
		const workerID = "worker-3"
		Expect(ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
			"id":   workerID,
			"name": "Container C",
		})).To(Succeed())
		_, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"config": "production",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		identityBefore, err := backend.Get(ctx, collections[storage.RoleIdentity], workerID)
		Expect(err).NotTo(HaveOccurred())
		desiredBefore, err := backend.Get(ctx, collections[storage.RoleDesired], workerID)
		Expect(err).NotTo(HaveOccurred())

		syncBefore, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())

		deltas := clearingDeltasByRole(syncBefore, workerID)
		Expect(deltas).To(HaveLen(2))
		Expect(deltas).To(HaveKey(storage.RoleIdentity))
		Expect(deltas).To(HaveKey(storage.RoleDesired))

		identityAfter, err := backend.Get(ctx, collections[storage.RoleIdentity], workerID)
		Expect(err).NotTo(HaveOccurred())
		expectClearedDocument(identityBefore, identityAfter, deltas[storage.RoleIdentity].SyncID)

		desiredAfter, err := backend.Get(ctx, collections[storage.RoleDesired], workerID)
		Expect(err).NotTo(HaveOccurred())
		expectClearedDocument(desiredBefore, desiredAfter, deltas[storage.RoleDesired].SyncID)

		_, err = backend.Get(ctx, collections[storage.RoleObserved], workerID)
		Expect(errors.Is(err, persistence.ErrNotFound)).To(BeTrue(),
			"ClearDeleted must not create a document the worker never had")
	})

	It("writes nothing and appends no delta when a write in the transaction fails", func() {
		const workerID = "worker-4"
		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		recorder := &deltaRecordingStore{
			Store: &updateFailingStore{mockStore: backend, failingCollection: workerType + "_" + storage.RoleObserved},
		}
		failingTs := storage.NewTriangularStoreWithClock(recorder, deps.NewNopFSMLogger(), mockClock)

		Expect(failingTs.ClearDeleted(ctx, workerType, workerID)).To(MatchError(errUpdateFailedByTest))

		for role, collection := range collections {
			doc, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).To(HaveKey(storage.FieldDeletedAt), "role %s must stay tombstoned", role)
		}

		Expect(recorder.deltaCount()).To(BeZero())
	})

	It("returns the commit error and appends no delta when the commit fails", func() {
		const workerID = "worker-5"
		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		recorder := &deltaRecordingStore{Store: &commitFailingStore{mockStore: backend}}
		failingTs := storage.NewTriangularStoreWithClock(recorder, deps.NewNopFSMLogger(), mockClock)

		Expect(failingTs.ClearDeleted(ctx, workerType, workerID)).To(MatchError(errCommitFailedByTest))

		for role, collection := range collections {
			doc, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).To(HaveKey(storage.FieldDeletedAt), "role %s must stay tombstoned", role)
		}

		Expect(recorder.deltaCount()).To(BeZero())
	})

	It("lets MarkDeleted stamp a new tombstone after ClearDeleted", func() {
		const workerID = "worker-1"
		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())
		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())

		mockClock.Add(2 * time.Hour)
		reStamp := mockClock.Now()

		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		for _, collection := range collections {
			doc, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())
			expectTombstone(doc, reStamp, "removed")
		}
	})
})
