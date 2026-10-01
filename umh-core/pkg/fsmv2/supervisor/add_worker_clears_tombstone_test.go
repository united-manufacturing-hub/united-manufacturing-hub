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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func newSupervisorOverStore(store storage.TriangularStoreInterface) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType:              "test",
		Store:                   store,
		Logger:                  deps.NewNopFSMLogger(),
		GracefulShutdownTimeout: 100 * time.Millisecond,
	})
}

var _ = Describe("AddWorker clears the worker's tombstone", func() {
	It("clears the tombstone after its three saves, and only adds the worker when the clear succeeds", func() {
		identity := mockIdentity()
		ctx := context.Background()

		store := newMockTriangularStore()
		s := newSupervisorOverStore(store)

		Expect(s.AddWorker(identity, &mockWorker{})).To(Succeed())
		Expect(s.ListWorkers()).To(ContainElement(identity.ID))

		Expect(store.ClearTombstoneCalls).To(HaveLen(1),
			"AddWorker must clear the worker's tombstone exactly once")
		Expect(store.ClearTombstoneCalls[0].WorkerType).To(Equal("test"))
		Expect(store.ClearTombstoneCalls[0].ID).To(Equal(identity.ID))

		Expect(store.SaveAndClearCalls).To(Equal([]string{
			"save_identity", "save_observed", "save_desired", "clear_tombstone",
		}), "the tombstone must be cleared after the identity, observed and desired saves")

		clearErr := errors.New("clear deleted failed")
		failingStore := newMockTriangularStore()
		failingStore.ClearTombstoneErr = clearErr
		failingS := newSupervisorOverStore(failingStore)

		addErr := failingS.AddWorker(identity, &mockWorker{})
		Expect(addErr).To(HaveOccurred())
		Expect(errors.Is(addErr, clearErr)).To(BeTrue(),
			"AddWorker must report the failing clear, not swallow it")
		Expect(failingS.ListWorkers()).To(BeEmpty(),
			"a worker whose tombstone could not be cleared must not be added")

		failingSaveStore := newMockTriangularStore()
		failingSaveStore.SaveDesiredErr = errors.New("save desired failed")
		failingSaveS := newSupervisorOverStore(failingSaveStore)

		Expect(failingSaveS.AddWorker(identity, &mockWorker{})).To(HaveOccurred())
		Expect(failingSaveStore.ClearTombstoneCalls).To(BeEmpty(),
			"the tombstone must not be cleared when a save failed")

		roles := []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved}

		basicStore := memory.NewInMemoryStore()
		for _, role := range roles {
			Expect(basicStore.CreateCollection(ctx, "test_"+role, nil)).To(Succeed())
		}

		realStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		Expect(realStore.SaveIdentity(ctx, "test", identity.ID, persistence.Document{"id": identity.ID})).To(Succeed())
		_, err := realStore.SaveDesired(ctx, "test", identity.ID, persistence.Document{"id": identity.ID})
		Expect(err).ToNot(HaveOccurred())
		_, err = realStore.SaveObserved(ctx, "test", identity.ID, persistence.Document{"id": identity.ID, "collectedAt": time.Now()})
		Expect(err).ToNot(HaveOccurred())

		Expect(realStore.Tombstone(ctx, "test", identity.ID, "removed")).To(Succeed())

		for _, role := range roles {
			doc, getErr := basicStore.Get(ctx, "test_"+role, identity.ID)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(doc).To(HaveKey(storage.FieldDeletedAt),
				"the %s document must carry the tombstone before the re-add", role)
		}

		realS := newSupervisorOverStore(realStore)

		Expect(realS.AddWorker(identity, &mockWorker{})).To(Succeed())
		Expect(realS.ListWorkers()).To(ContainElement(identity.ID))

		for _, role := range roles {
			doc, getErr := basicStore.Get(ctx, "test_"+role, identity.ID)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(doc).ToNot(HaveKey(storage.FieldDeletedAt),
				"the %s document must lose its tombstone when the worker is added again", role)
			Expect(doc).ToNot(HaveKey(storage.FieldDeletedBy))
		}
	})
})

var _ = Describe("AddWorker error paths", func() {
	It("returns the derive error and writes no documents", func() {
		identity := mockIdentity()

		store := newMockTriangularStore()
		s := newSupervisorOverStore(store)

		deriveErr := errors.New("derive desired failed")
		addErr := s.AddWorker(identity, &mockWorker{deriveErr: deriveErr})

		Expect(errors.Is(addErr, deriveErr)).To(BeTrue(),
			"AddWorker must report the failing derive, not swallow it")
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(store.SaveAndClearCalls).To(BeEmpty())
	})

	It("returns the collect error and writes no documents", func() {
		identity := mockIdentity()

		store := newMockTriangularStore()
		s := newSupervisorOverStore(store)

		collectErr := errors.New("collect observed failed")
		addErr := s.AddWorker(identity, &mockWorker{collectErr: collectErr})

		Expect(errors.Is(addErr, collectErr)).To(BeTrue(),
			"AddWorker must report the failing collection, not swallow it")
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(store.SaveAndClearCalls).To(BeEmpty())
	})

	It("returns the identity-save error and writes nothing further", func() {
		identity := mockIdentity()

		store := newMockTriangularStore()
		store.SaveIdentityErr = errors.New("save identity failed")
		s := newSupervisorOverStore(store)

		addErr := s.AddWorker(identity, &mockWorker{})

		Expect(errors.Is(addErr, store.SaveIdentityErr)).To(BeTrue(),
			"AddWorker must report the failing identity save, not swallow it")
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(store.SaveAndClearCalls).To(BeEmpty(),
			"a failed identity save must not be followed by further writes")
	})
})
