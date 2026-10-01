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

// deltaFailingStore fails every insert into the delta collection, so a
// committed write's delta append fails.
type deltaFailingStore struct {
	*mockStore
}

func (d *deltaFailingStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	if collection == storage.DeltaCollectionName {
		return "", errors.New("delta insert failed")
	}

	return d.mockStore.Insert(ctx, collection, doc)
}

// warnRecorder collects the messages SentryWarn received.
type warnRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *warnRecorder) Debug(_ string, _ ...deps.Field) {}

func (r *warnRecorder) Info(_ string, _ ...deps.Field) {}

func (r *warnRecorder) SentryWarn(_ deps.Feature, _ string, msg string, _ ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.msgs = append(r.msgs, msg)
}

func (r *warnRecorder) SentryError(_ deps.Feature, _ string, _ error, _ string, _ ...deps.Field) {
}

func (r *warnRecorder) With(_ ...deps.Field) deps.FSMLogger { return r }

func (r *warnRecorder) sentryWarns() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string{}, r.msgs...)
}

var _ = Describe("A committed write whose delta append fails", func() {
	const (
		workerType = "container"
		workerID   = "worker-1"
	)

	It("still succeeds and warns, for saves, MarkDeleted and ClearDeleted", func() {
		ctx := context.Background()

		backend := &deltaFailingStore{mockStore: newMockStore()}
		rec := &warnRecorder{}
		ts := storage.NewTriangularStore(backend, rec)

		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(rec.sentryWarns()).To(HaveLen(3), "saves warn")

		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())
		Expect(rec.sentryWarns()).To(HaveLen(6), "MarkDeleted warns")

		tombstoned, err := backend.Get(ctx, workerType+"_observed", workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(tombstoned).To(HaveKey(storage.FieldDeletedAt))

		Expect(ts.ClearDeleted(ctx, workerType, workerID)).To(Succeed())
		Expect(rec.sentryWarns()).To(HaveLen(9), "ClearDeleted warns")

		cleared, err := backend.Get(ctx, workerType+"_observed", workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(cleared).NotTo(HaveKey(storage.FieldDeletedAt))
	})
})

var _ = Describe("An unchanged observed save", func() {
	const (
		workerType = "container"
		workerID   = "worker-1"
	)

	It("makes its fresh _updated_at visible through LoadSnapshot", func() {
		ctx := context.Background()

		mockClock := clock.NewMock()
		t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		mockClock.Set(t0)

		ts := storage.NewTriangularStoreWithClock(newMockStore(), deps.NewNopFSMLogger(), mockClock)

		saveInitialDocuments(ctx, ts, workerType, workerID)

		_, err := ts.LoadSnapshot(ctx, workerType, workerID)
		Expect(err).NotTo(HaveOccurred())

		mockClock.Add(time.Hour)

		changed, err := ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
			"id":     workerID,
			"status": "running",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		snap, err := ts.LoadSnapshot(ctx, workerType, workerID)
		Expect(err).NotTo(HaveOccurred())

		observed, ok := snap.Observed.(persistence.Document)
		Expect(ok).To(BeTrue())

		// LoadSnapshot returns a JSON round-trip, so _updated_at arrives as a
		// string, not a time.Time.
		updatedAt, ok := observed[storage.FieldUpdatedAt].(string)
		Expect(ok).To(BeTrue())

		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal(t0.Add(time.Hour)))
	})
})
