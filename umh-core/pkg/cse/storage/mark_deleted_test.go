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
	"time"

	"github.com/benbjohnson/clock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var fieldsMarkDeletedWrites = map[string]bool{
	storage.FieldDeletedAt: true,
	storage.FieldDeletedBy: true,
	storage.FieldSyncID:    true,
}

func expectOnlyMarkDeletedFieldsChanged(before, after persistence.Document) {
	for key := range after {
		_, existed := before[key]
		ExpectWithOffset(1, existed || fieldsMarkDeletedWrites[key]).To(BeTrue(), "MarkDeleted must not add field %s", key)
	}

	for key, beforeVal := range before {
		if fieldsMarkDeletedWrites[key] {
			continue
		}

		ExpectWithOffset(1, after).To(HaveKey(key), "field %s must survive MarkDeleted", key)
		ExpectWithOffset(1, after[key]).To(Equal(beforeVal), "field %s must keep its value across MarkDeleted", key)
	}
}

func expectTombstone(doc persistence.Document, at time.Time, by string) {
	deletedAt, ok := doc[storage.FieldDeletedAt].(time.Time)
	ExpectWithOffset(1, ok).To(BeTrue(), "%s must be a time.Time", storage.FieldDeletedAt)
	ExpectWithOffset(1, deletedAt.Equal(at)).To(BeTrue(),
		"%s must be the store clock time %v, got %v", storage.FieldDeletedAt, at, deletedAt)
	ExpectWithOffset(1, doc[storage.FieldDeletedBy]).To(Equal(by))
}

func expectDeltaDescribesTombstone(delta storage.Delta, at time.Time, by string) {
	ExpectWithOffset(1, delta.Changes).NotTo(BeNil())

	// The delta store persists Changes as JSON, so _deleted_at arrives as an
	// RFC 3339 string.
	deletedAtJSON, ok := delta.Changes.Added[storage.FieldDeletedAt].(string)
	ExpectWithOffset(1, ok).To(BeTrue(),
		"delta for role %s must report %s as its JSON encoding", delta.Role, storage.FieldDeletedAt)

	deletedAt, err := time.Parse(time.RFC3339, deletedAtJSON)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"delta for role %s must report %s as RFC 3339, got %q", delta.Role, storage.FieldDeletedAt, deletedAtJSON)
	ExpectWithOffset(1, deletedAt.Equal(at)).To(BeTrue(),
		"delta for role %s must report the tombstone time %v, got %v", delta.Role, at, deletedAt)

	ExpectWithOffset(1, delta.Changes.Added[storage.FieldDeletedBy]).To(Equal(by),
		"delta for role %s must report %s", delta.Role, storage.FieldDeletedBy)
}

var errCommitFailedByTest = errors.New("commit failed (injected)")

type commitFailingStore struct {
	*mockStore
}

func (s *commitFailingStore) BeginTx(_ context.Context) (persistence.Tx, error) {
	return &commitFailingTx{parent: s.mockStore}, nil
}

// commitFailingTx reads through to the backend, discards every write, and
// fails Commit.
type commitFailingTx struct {
	parent *mockStore
}

func (tx *commitFailingTx) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return tx.parent.CreateCollection(ctx, name, schema)
}

func (tx *commitFailingTx) DropCollection(ctx context.Context, name string) error {
	return tx.parent.DropCollection(ctx, name)
}

func (tx *commitFailingTx) Insert(_ context.Context, _ string, doc persistence.Document) (string, error) {
	return doc["id"].(string), nil
}

func (tx *commitFailingTx) Get(ctx context.Context, collection, id string) (persistence.Document, error) {
	return tx.parent.Get(ctx, collection, id)
}

func (tx *commitFailingTx) Update(_ context.Context, _ string, _ string, _ persistence.Document) error {
	return nil
}

func (tx *commitFailingTx) Delete(_ context.Context, _ string, _ string) error {
	return nil
}

func (tx *commitFailingTx) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return tx.parent.Find(ctx, collection, query)
}

func (tx *commitFailingTx) BeginTx(_ context.Context) (persistence.Tx, error) {
	return &commitFailingTx{parent: tx.parent}, nil
}

func (tx *commitFailingTx) Close(_ context.Context) error {
	return nil
}

func (tx *commitFailingTx) Maintenance(_ context.Context) error {
	return nil
}

func (tx *commitFailingTx) Commit() error {
	return errCommitFailedByTest
}

func (tx *commitFailingTx) Rollback() error {
	return nil
}

var errUpdateFailedByTest = errors.New("update failed (injected)")

type updateFailingStore struct {
	*mockStore
	failingCollection string
}

func (s *updateFailingStore) BeginTx(_ context.Context) (persistence.Tx, error) {
	return &updateFailingTx{commitFailingTx: &commitFailingTx{parent: s.mockStore}, failingCollection: s.failingCollection}, nil
}

type updateFailingTx struct {
	*commitFailingTx
	failingCollection string
}

func (tx *updateFailingTx) Update(ctx context.Context, collection, id string, doc persistence.Document) error {
	if collection == tx.failingCollection {
		return errUpdateFailedByTest
	}

	return tx.commitFailingTx.Update(ctx, collection, id, doc)
}

var _ = Describe("MarkDeleted", func() {
	const workerType = "container"

	const workerID = "worker-1"

	var (
		ctx         context.Context
		mockClock   *clock.Mock
		t0          time.Time
		backend     *mockStore
		ts          *storage.TriangularStore
		collections map[string]string
		before      map[string]persistence.Document
		syncBefore  int64
	)

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

		Expect(ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
			"id":   workerID,
			"name": "Container A",
		})).To(Succeed())
		_, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"config": "production",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"status": "running",
		})
		Expect(err).NotTo(HaveOccurred())

		before = make(map[string]persistence.Document)

		for role, collection := range collections {
			doc, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())

			before[role] = doc
		}

		syncBefore, err = ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("tombstones every stored role document and appends one delta entry per role", func() {
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		for role, collection := range collections {
			after, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())

			expectTombstone(after, t0, "removed")
			expectOnlyMarkDeletedFieldsChanged(before[role], after)
		}

		resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: syncBefore})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.RequiresBootstrap).To(BeFalse())
		Expect(resp.Deltas).To(HaveLen(3))

		rolesSeen := make(map[string]bool)

		for _, delta := range resp.Deltas {
			Expect(delta.WorkerType).To(Equal(workerType))
			Expect(delta.WorkerID).To(Equal(workerID))
			Expect(delta.SyncID).To(BeNumerically(">", syncBefore))
			expectDeltaDescribesTombstone(delta, t0, "removed")

			Expect(rolesSeen[delta.Role]).To(BeFalse(), "role %s reported twice", delta.Role)
			rolesSeen[delta.Role] = true
		}

		Expect(rolesSeen).To(HaveLen(3))
		Expect(rolesSeen[storage.RoleIdentity]).To(BeTrue())
		Expect(rolesSeen[storage.RoleDesired]).To(BeTrue())
		Expect(rolesSeen[storage.RoleObserved]).To(BeTrue())
	})

	It("invalidates the snapshot cache", func() {
		cachedSnap, err := ts.LoadSnapshot(ctx, workerType, workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(cachedSnap.Identity).NotTo(HaveKey(storage.FieldDeletedAt))

		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		snapAfter, err := ts.LoadSnapshot(ctx, workerType, workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(snapAfter.Identity).To(HaveKey(storage.FieldDeletedAt))
		Expect(snapAfter.Desired).To(HaveKey(storage.FieldDeletedAt))

		observedAfter, ok := snapAfter.Observed.(persistence.Document)
		Expect(ok).To(BeTrue())
		Expect(observedAfter).To(HaveKey(storage.FieldDeletedAt))
	})

	It("a second MarkDeleted keeps the first tombstone and writes nothing", func() {
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		syncAfterFirst, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())

		mockClock.Add(2 * time.Hour)

		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		for role, collection := range collections {
			after, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())

			expectTombstone(after, t0, "removed")
			expectOnlyMarkDeletedFieldsChanged(before[role], after)
		}

		syncAfterSecond, err := ts.GetLatestSyncID(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(syncAfterSecond).To(Equal(syncAfterFirst))

		resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: syncAfterFirst})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Deltas).To(BeEmpty())
		Expect(resp.RequiresBootstrap).To(BeFalse())
	})

	It("tombstones the identity and observed documents of a worker without a desired document", func() {
		const missingDesiredID = "worker-2"

		Expect(ts.SaveIdentity(ctx, workerType, missingDesiredID, persistence.Document{
			"id":   missingDesiredID,
			"name": "Container B",
		})).To(Succeed())
		_, err := ts.SaveObserved(ctx, workerType, missingDesiredID, persistence.Document{
			"id":     missingDesiredID,
			"status": "idle",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(ts.MarkDeleted(ctx, workerType, missingDesiredID, "removed")).To(Succeed())

		otherIdentity, err := backend.Get(ctx, workerType+"_identity", missingDesiredID)
		Expect(err).NotTo(HaveOccurred())
		expectTombstone(otherIdentity, t0, "removed")

		otherObserved, err := backend.Get(ctx, workerType+"_observed", missingDesiredID)
		Expect(err).NotTo(HaveOccurred())
		expectTombstone(otherObserved, t0, "removed")
	})

	It("a commit failure tombstones no document", func() {
		failingBackend := newMockStore()
		failingTs := storage.NewTriangularStoreWithClock(
			&commitFailingStore{mockStore: failingBackend},
			deps.NewNopFSMLogger(),
			mockClock,
		)

		Expect(failingTs.SaveIdentity(ctx, workerType, workerID, persistence.Document{
			"id":   workerID,
			"name": "Container A",
		})).To(Succeed())
		_, err := failingTs.SaveDesired(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"config": "production",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = failingTs.SaveObserved(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"status": "running",
		})
		Expect(err).NotTo(HaveOccurred())

		err = failingTs.MarkDeleted(ctx, workerType, workerID, "removed")
		Expect(err).To(MatchError(errCommitFailedByTest))

		for _, collection := range collections {
			doc, err := failingBackend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).NotTo(HaveKey(storage.FieldDeletedAt))
		}
	})

	It("a write failure mid-transaction tombstones no document", func() {
		failingTs := storage.NewTriangularStoreWithClock(
			&updateFailingStore{mockStore: backend, failingCollection: workerType + "_" + storage.RoleDesired},
			deps.NewNopFSMLogger(),
			mockClock,
		)

		err := failingTs.MarkDeleted(ctx, workerType, workerID, "removed")
		Expect(err).To(MatchError(errUpdateFailedByTest))

		for _, collection := range collections {
			doc, err := backend.Get(ctx, collection, workerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).NotTo(HaveKey(storage.FieldDeletedAt))
		}

		resp, err := ts.GetDeltas(ctx, storage.Subscription{LastSyncID: syncBefore})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Deltas).To(BeEmpty())
	})
})
