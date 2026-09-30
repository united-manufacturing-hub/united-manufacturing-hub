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
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// pausingStore holds the result of the first Get of one document until the
// test releases it, so a test can run another call while a save sits
// between its read and its write.
type pausingStore struct {
	*mockStore

	mu         sync.Mutex
	collection string
	id         string
	paused     chan struct{}
	release    chan struct{}
}

func (p *pausingStore) pauseNextGet(collection, id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.collection = collection
	p.id = id
	p.paused = make(chan struct{})
	p.release = make(chan struct{})
}

func (p *pausingStore) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	p.mu.Lock()
	armed := p.paused != nil && collection == p.collection && id == p.id
	paused, release := p.paused, p.release

	if armed {
		p.paused = nil
	}
	p.mu.Unlock()

	doc, err := p.mockStore.Get(ctx, collection, id)

	if armed {
		close(paused)
		<-release
	}

	return doc, err
}

// giveTimeToFinish waits up to 200 ms for done, then puts back what it
// received. If the call under test can finish while the save is paused, this
// gives it the time to do so. If it waits for the save, this times out, and
// the test's outcome is the same either way.
func giveTimeToFinish(done chan error) {
	select {
	case err := <-done:
		done <- err
	case <-time.After(200 * time.Millisecond):
	}
}

var _ = Describe("A save running while the tombstone changes", func() {
	It("does not lose the tombstone MarkDeleted writes", func() {
		const (
			workerType = "container"
			workerID   = "worker-1"
		)

		ctx := context.Background()
		mockClock := clock.NewMock()
		t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		mockClock.Set(t0)

		backend := &pausingStore{mockStore: newMockStore()}
		ts := storage.NewTriangularStoreWithClock(backend, deps.NewNopFSMLogger(), mockClock)

		saveInitialDocuments(ctx, ts, workerType, workerID)

		observedCollection := workerType + "_" + storage.RoleObserved
		backend.pauseNextGet(observedCollection, workerID)
		paused, release := backend.paused, backend.release

		saveDone := make(chan error, 1)

		go func() {
			_, err := ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
				"id":           workerID,
				"status":       "running",
				"collected_at": t0.Add(time.Hour),
			})
			saveDone <- err
		}()

		Eventually(paused).Should(BeClosed())

		markDone := make(chan error, 1)

		go func() {
			markDone <- ts.MarkDeleted(ctx, workerType, workerID, "removed")
		}()

		giveTimeToFinish(markDone)

		close(release)

		Eventually(saveDone).Should(Receive(BeNil()))
		Eventually(markDone).Should(Receive(BeNil()))

		stored, err := backend.mockStore.Get(ctx, observedCollection, workerID)
		Expect(err).NotTo(HaveOccurred())
		expectTombstone(stored, t0, "removed")
		Expect(stored["collected_at"]).To(Equal(t0.Add(time.Hour)))
	})

	It("does not bring back a tombstone ClearDeleted removes", func() {
		const (
			workerType = "container"
			workerID   = "worker-1"
		)

		ctx := context.Background()
		mockClock := clock.NewMock()
		t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		mockClock.Set(t0)

		backend := &pausingStore{mockStore: newMockStore()}
		ts := storage.NewTriangularStoreWithClock(backend, deps.NewNopFSMLogger(), mockClock)

		saveInitialDocuments(ctx, ts, workerType, workerID)
		Expect(ts.MarkDeleted(ctx, workerType, workerID, "removed")).To(Succeed())

		observedCollection := workerType + "_" + storage.RoleObserved
		backend.pauseNextGet(observedCollection, workerID)
		paused, release := backend.paused, backend.release

		saveDone := make(chan error, 1)

		go func() {
			_, err := ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
				"id":           workerID,
				"status":       "running",
				"collected_at": t0.Add(time.Hour),
			})
			saveDone <- err
		}()

		Eventually(paused).Should(BeClosed())

		clearDone := make(chan error, 1)

		go func() {
			clearDone <- ts.ClearDeleted(ctx, workerType, workerID)
		}()

		giveTimeToFinish(clearDone)

		close(release)

		Eventually(saveDone).Should(Receive(BeNil()))
		Eventually(clearDone).Should(Receive(BeNil()))

		stored, err := backend.mockStore.Get(ctx, observedCollection, workerID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).NotTo(HaveKey(storage.FieldDeletedAt))
		Expect(stored).NotTo(HaveKey(storage.FieldDeletedBy))
		Expect(stored["collected_at"]).To(Equal(t0.Add(time.Hour)))
	})
})
