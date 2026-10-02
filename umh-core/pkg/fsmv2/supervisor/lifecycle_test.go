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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Supervisor Lifecycle", func() {
	Describe("Start and tickLoop integration", func() {
		Context("when supervisor is started", func() {
			It("should run tick loop until context is cancelled", func() {
				store := createTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "container",
					Store:                   store,
					Logger:                  deps.NewNopFSMLogger(),
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond, // Short timeout for tests
				})

				identity := mockIdentity()
				worker := &mockWorker{}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

				ctx, cancel := context.WithCancel(context.Background())

				done := s.Start(ctx)

				time.Sleep(200 * time.Millisecond)

				cancel()

				Eventually(done, 2*time.Second).Should(BeClosed())
			})
		})
	})

	Describe("Tick with shutdown request error", func() {
		Context("when RequestShutdown fails during timeout handling", func() {
			It("should still return error about unresponsive collector", func() {
				store := newMockTriangularStore()

				s := newSupervisorWithWorker(&mockWorker{
					observed: &mockObservedState{
						ID:          mockIdentity().ID,
						CollectedAt: time.Now().Add(-25 * time.Second),
						Desired:     &mockDesiredState{},
					},
				}, store, supervisor.CollectorHealthConfig{
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 1,
				})

				// Update the observed state in the store with stale data
				// This is necessary because newSupervisorWithWorker saves fresh data
				identity := mockIdentity()
				store.Observed["test"] = map[string]interface{}{
					identity.ID: persistence.Document{
						"id":          identity.ID,
						"collectedAt": time.Now().Add(-25 * time.Second),
					},
				}

				s.TestSetRestartCount(1)

				store.SaveDesiredErr = errors.New("save error")

				err := s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})
	})

	Describe("Tick state transition edge case", func() {
		Context("when state violates invariant by switching state AND emitting action", func() {
			It("should return error (panic recovered)", func() {
				store := newMockTriangularStore()

				nextState := &mockState{}
				action := &mockAction{}
				initialState := &mockState{
					nextState: nextState,
					action:    action,
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("tick panic"))
			})
		})
	})

	Describe("processSignal error handling", func() {
		Context("when SignalNeedsRemoval is received", func() {
			It("should remove worker from registry", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(BeEmpty())
			})
		})

		Context("when SignalNeedsRestart is received", func() {
			It("should mark worker for restart and request graceful shutdown", func() {
				// NOTE: SignalNeedsRestart now triggers a full worker restart (graceful shutdown + reset)
				// instead of just restarting the collector. The restart count is NOT incremented
				// because we're doing a full restart, not a collector-only restart.
				// See "SignalNeedsRestart full worker restart" tests for all FSM state combinations.
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRestart,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				// Worker should still exist (not removed yet - waiting for graceful shutdown)
				workers := s.ListWorkers()
				Expect(workers).To(HaveLen(1))
			})
		})

		Context("when unknown signal is received", func() {
			It("should return error for invalid signal", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.Signal(999),
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown signal"))
			})
		})
	})

	Describe("removal tombstones the worker's documents", func() {
		newRemovalSupervisor := func(store storage.TriangularStoreInterface, logger deps.FSMLogger) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
			removalState := &mockState{
				signal: fsmv2.SignalNeedsRemoval,
			}
			removalState.nextState = removalState

			return newSupervisorWithWorkerAndLogger(&mockWorker{initialState: removalState}, store, supervisor.CollectorHealthConfig{}, logger)
		}

		It("tombstones the documents on plain removal", func() {
			identity := mockIdentity()
			store := newMockTriangularStore()
			s := newRemovalSupervisor(store, deps.NewNopFSMLogger())

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(s.ListWorkers()).To(BeEmpty())
			Expect(store.TombstoneCalls).To(HaveLen(1))
			Expect(store.TombstoneCalls[0].WorkerType).To(Equal("test"))
			Expect(store.TombstoneCalls[0].ID).To(Equal(identity.ID))
			Expect(store.TombstoneCalls[0].By).To(Equal("supervisor"))
		})

		It("does not tombstone the documents on restart", func() {
			identity := mockIdentity()
			store := newMockTriangularStore()
			s := newRemovalSupervisor(store, deps.NewNopFSMLogger())
			s.TestSetPendingRestart(identity.ID)
			s.TestMarkAsStarted()

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(s.ListWorkers()).To(HaveLen(1))
			Expect(store.TombstoneCalls).To(BeEmpty())
		})

		It("does not tombstone the documents on RemoveWorker", func() {
			identity := mockIdentity()
			store := newMockTriangularStore()
			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{})

			Expect(s.RemoveWorker(context.Background(), identity.ID)).To(Succeed())
			Expect(s.ListWorkers()).To(BeEmpty())
			Expect(store.TombstoneCalls).To(BeEmpty())
		})

		It("still removes the worker and warns when Tombstone fails", func() {
			store := newMockTriangularStore()
			store.TombstoneErr = errors.New("tombstone failed")
			logger := &sentryWarnRecorder{}
			s := newRemovalSupervisor(store, logger)

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(s.ListWorkers()).To(BeEmpty())
			Expect(store.TombstoneCalls).To(HaveLen(1))

			warnings := logger.Warns()
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0].Msg).To(Equal("worker_removal_tombstone_failed"))
			Expect(warnings[0].Fields).To(ContainElement(deps.Field{Key: "target_worker_id", Value: mockIdentity().ID}))
		})

		It("tombstones the documents even when the tick context is cancelled", func() {
			identity := mockIdentity()
			roles := []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved}

			basicStore := memory.NewInMemoryStore()
			for _, role := range roles {
				Expect(basicStore.CreateCollection(context.Background(), "test_"+role, nil)).To(Succeed())
			}

			// The store rejects writes on a cancelled context, so only the
			// context tombstoneWorker derives itself can carry the
			// tombstone write. The hook store records whether that context
			// carries the deadline that bounds the write.
			realStore := storage.NewTriangularStore(&cancelledWriteStore{inner: basicStore}, deps.NewNopFSMLogger())
			store := &tombstoneHookStore{TriangularStoreInterface: realStore}
			s := newRemovalSupervisor(store, deps.NewNopFSMLogger())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			Expect(s.TestTick(ctx)).To(Succeed())
			Expect(s.ListWorkers()).To(BeEmpty())
			Expect(store.tombstoneCtxHasDeadline).To(BeTrue(),
				"the tombstone write runs under s.mu, so without a deadline it could hold the supervisor forever")

			for _, role := range roles {
				doc, getErr := basicStore.Get(context.Background(), "test_"+role, identity.ID)
				Expect(getErr).ToNot(HaveOccurred())
				Expect(doc[storage.FieldDeletedAt]).ToNot(BeNil(),
					"the %s document must carry a tombstone despite the cancelled tick context", role)
			}
		})

		It("writes a non-nil _deleted_at on the real store's documents", func() {
			identity := mockIdentity()
			roles := []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved}

			basicStore := memory.NewInMemoryStore()
			for _, role := range roles {
				Expect(basicStore.CreateCollection(context.Background(), "test_"+role, nil)).To(Succeed())
			}

			realStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())
			s := newRemovalSupervisor(realStore, deps.NewNopFSMLogger())

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(s.ListWorkers()).To(BeEmpty())

			for _, role := range roles {
				doc, getErr := basicStore.Get(context.Background(), "test_"+role, identity.ID)
				Expect(getErr).ToNot(HaveOccurred())
				Expect(doc).To(HaveKey(storage.FieldDeletedAt), "the %s document must carry a tombstone", role)
				Expect(doc[storage.FieldDeletedAt]).ToNot(BeNil(), "the %s document's _deleted_at must be non-nil, or it carries no tombstone", role)
				Expect(doc[storage.FieldDeletedBy]).To(Equal("supervisor"))
			}
		})

		It("does not tombstone a worker added again while the old one is being removed", func() {
			identity := mockIdentity()
			roles := []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved}

			basicStore := memory.NewInMemoryStore()
			for _, role := range roles {
				Expect(basicStore.CreateCollection(context.Background(), "test_"+role, nil)).To(Succeed())
			}

			store := &tombstoneHookStore{TriangularStoreInterface: storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())}
			s := newRemovalSupervisor(store, deps.NewNopFSMLogger())

			addDone := make(chan error, 1)

			// Add a worker with the same id just before the old one is
			// tombstoned. An AddWorker that does not wait for the removal
			// finishes within 300 ms.
			store.beforeTombstone = func() {
				go func() { addDone <- s.AddWorker(identity, &mockWorker{}) }()

				select {
				case err := <-addDone:
					addDone <- err
				case <-time.After(300 * time.Millisecond):
				}
			}

			Expect(s.TestTick(context.Background())).To(Succeed())
			Eventually(addDone).Should(Receive(BeNil()))

			Expect(s.ListWorkers()).To(ContainElement(identity.ID))

			for _, role := range roles {
				doc, getErr := basicStore.Get(context.Background(), "test_"+role, identity.ID)
				Expect(getErr).ToNot(HaveOccurred())
				Expect(doc).ToNot(HaveKey(storage.FieldDeletedAt),
					"the %s document of the worker added again must not carry a tombstone", role)
			}
		})
	})

	Describe("SignalNeedsRestart full worker restart", func() {
		Context("when SignalNeedsRestart is received", func() {
			It("should mark worker for restart and request graceful shutdown", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRestart,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(HaveLen(1))

				identity := mockIdentity()
				var desiredState mockDesiredState
				loadErr := store.LoadDesiredTyped(context.Background(), "test", identity.ID, &desiredState)
				Expect(loadErr).ToNot(HaveOccurred())
				Expect(desiredState.ShutdownRequested).To(BeTrue())
			})
		})

		Context("when SignalNeedsRemoval received for worker in pendingRestart", func() {
			It("should restart worker instead of removing", func() {
				store := newMockTriangularStore()

				initialState := &mockState{}
				initialState.nextState = initialState

				stoppedState := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				stoppedState.nextState = stoppedState

				worker := &mockWorker{initialState: stoppedState}

				s := newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{})

				identity := mockIdentity()
				s.TestSetPendingRestart(identity.ID)

				// processSignal only restarts when started=true; set it before the tick.
				s.TestMarkAsStarted()

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(HaveLen(1))

				var desiredState mockDesiredState
				loadErr := store.LoadDesiredTyped(context.Background(), "test", identity.ID, &desiredState)
				Expect(loadErr).ToNot(HaveOccurred())
				Expect(desiredState.ShutdownRequested).To(BeFalse())
			})
		})

		Context("when SignalNeedsRemoval received for worker NOT in pendingRestart", func() {
			It("should remove worker normally", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(BeEmpty())
			})
		})

		Context("when graceful restart times out", func() {
			It("should force reset worker after timeout", func() {
				store := newMockTriangularStore()

				state := &mockState{}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})
				identity := mockIdentity()

				s.TestSetPendingRestart(identity.ID)

				s.TestSetRestartRequestedAt(identity.ID, time.Now().Add(-35*time.Second))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workers := s.ListWorkers()
				Expect(workers).To(HaveLen(1))

				Expect(s.TestIsPendingRestart(identity.ID)).To(BeFalse())
			})
		})
	})

	Describe("Run(ctx) error API", func() {
		Context("when ctx is cancelled externally", func() {
			It("should block until cancelled, then shut down and return nil", func() {
				store := createTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "container",
					Store:                   store,
					Logger:                  deps.NewNopFSMLogger(),
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 200 * time.Millisecond,
				})

				ctx, cancel := context.WithCancel(context.Background())

				runResult := make(chan error, 1)

				go func() {
					runResult <- s.Run(ctx)
				}()

				time.Sleep(100 * time.Millisecond)

				// Run must be blocking (channel should have no result yet)
				Expect(runResult).ToNot(Receive(), "Run should block until ctx is cancelled")

				// Cancel the context to trigger shutdown
				cancel()

				// Run should return nil within a reasonable time
				Eventually(runResult, 2*time.Second).Should(Receive(BeNil()),
					"Run should return nil after ctx cancellation")
			})
		})
	})
})

// tombstoneHookStore records facts about the Tombstone call and can run a
// hook from inside it.
type tombstoneHookStore struct {
	storage.TriangularStoreInterface

	beforeTombstone func()

	// tombstoneCtxHasDeadline records whether the context of the Tombstone
	// call carries a deadline.
	tombstoneCtxHasDeadline bool
}

func (h *tombstoneHookStore) Tombstone(ctx context.Context, workerType string, id string, deletedBy string) error {
	if hook := h.beforeTombstone; hook != nil {
		h.beforeTombstone = nil

		hook()
	}

	_, h.tombstoneCtxHasDeadline = ctx.Deadline()

	return h.TriangularStoreInterface.Tombstone(ctx, workerType, id, deletedBy)
}

// cancelledWriteStore rejects every write whose context is cancelled, then
// delegates to inner. It models the store contract that a write may honour
// cancellation, so only the context tombstoneWorker derives can write the
// tombstone.
type cancelledWriteStore struct {
	inner persistence.Store
}

func (s *cancelledWriteStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return s.inner.CreateCollection(ctx, name, schema)
}

func (s *cancelledWriteStore) DropCollection(ctx context.Context, name string) error {
	return s.inner.DropCollection(ctx, name)
}

func (s *cancelledWriteStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return s.inner.Insert(ctx, collection, doc)
}

func (s *cancelledWriteStore) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	return s.inner.Get(ctx, collection, id)
}

func (s *cancelledWriteStore) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.inner.Update(ctx, collection, id, doc)
}

func (s *cancelledWriteStore) Delete(ctx context.Context, collection string, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.inner.Delete(ctx, collection, id)
}

func (s *cancelledWriteStore) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return s.inner.Find(ctx, collection, query)
}

func (s *cancelledWriteStore) Maintenance(ctx context.Context) error {
	return s.inner.Maintenance(ctx)
}

func (s *cancelledWriteStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	tx, err := s.inner.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	return &cancelledWriteTx{inner: tx}, nil
}

func (s *cancelledWriteStore) Close(ctx context.Context) error {
	return s.inner.Close(ctx)
}

// cancelledWriteTx applies the same cancellation check to every write inside
// the transaction.
type cancelledWriteTx struct {
	inner persistence.Tx
}

func (t *cancelledWriteTx) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return t.inner.CreateCollection(ctx, name, schema)
}

func (t *cancelledWriteTx) DropCollection(ctx context.Context, name string) error {
	return t.inner.DropCollection(ctx, name)
}

func (t *cancelledWriteTx) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return t.inner.Insert(ctx, collection, doc)
}

func (t *cancelledWriteTx) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	return t.inner.Get(ctx, collection, id)
}

func (t *cancelledWriteTx) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return t.inner.Update(ctx, collection, id, doc)
}

func (t *cancelledWriteTx) Delete(ctx context.Context, collection string, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return t.inner.Delete(ctx, collection, id)
}

func (t *cancelledWriteTx) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return t.inner.Find(ctx, collection, query)
}

func (t *cancelledWriteTx) Maintenance(ctx context.Context) error {
	return t.inner.Maintenance(ctx)
}

func (t *cancelledWriteTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return t.inner.BeginTx(ctx)
}

func (t *cancelledWriteTx) Close(ctx context.Context) error {
	return t.inner.Close(ctx)
}

func (t *cancelledWriteTx) Commit() error   { return t.inner.Commit() }
func (t *cancelledWriteTx) Rollback() error { return t.inner.Rollback() }

var _ persistence.Store = (*cancelledWriteStore)(nil)
var _ persistence.Tx = (*cancelledWriteTx)(nil)

type sentryWarnRecorder struct {
	mu       sync.Mutex
	warnings []sentryWarn
}

type sentryWarn struct {
	Msg    string
	Fields []deps.Field
}

func (r *sentryWarnRecorder) Debug(_ string, _ ...deps.Field) {}
func (r *sentryWarnRecorder) Info(_ string, _ ...deps.Field)  {}

func (r *sentryWarnRecorder) SentryWarn(_ deps.Feature, _ string, msg string, fields ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.warnings = append(r.warnings, sentryWarn{Msg: msg, Fields: fields})
}

func (r *sentryWarnRecorder) SentryError(_ deps.Feature, _ string, _ error, _ string, _ ...deps.Field) {
}

func (r *sentryWarnRecorder) With(_ ...deps.Field) deps.FSMLogger { return r }

func (r *sentryWarnRecorder) Warns() []sentryWarn {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]sentryWarn{}, r.warnings...)
}
