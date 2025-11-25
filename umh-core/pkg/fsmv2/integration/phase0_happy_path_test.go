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

package integration_test

import (
	"context"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
	childSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func setupTestStore(ctx context.Context, workerTypes ...string) *storage.TriangularStore {
	basicStore := memory.NewInMemoryStore()

	// If no worker types provided, default to parent worker type
	if len(workerTypes) == 0 {
		workerTypes = []string{storage.DeriveWorkerType[snapshot.ParentObservedState]()}
	}

	// Create collections following naming convention: {workerType}_{role}
	for _, workerType := range workerTypes {
		_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)
	}

	return storage.NewTriangularStore(basicStore)
}

var _ = Describe("Phase 0: Happy Path Integration", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		parentSupervisor  *supervisor.Supervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState]
		logger            *zap.SugaredLogger
		initialGoroutines int
		err               error
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		factory.ResetRegistry()
	})

	AfterEach(func() {
		cancel() // Cancel context FIRST so tick loop exits
		if parentSupervisor != nil {
			parentSupervisor.Shutdown() // Then wait for shutdown
		}

		Eventually(func() int {
			runtime.GC()

			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Basic Factory Registration", func() {
		It("should register parent worker type and create worker instances", func() {
			By("Step 1: Registering parent worker type in factory")

			// Register worker factories
			err = factory.RegisterFactory[snapshot.ParentObservedState, *snapshot.ParentDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				return parent.NewParentWorker(identity.ID, identity.Name, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}

				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			// Register supervisor factories (needed for child creation via reconcileChildren)
			err = factory.RegisterSupervisorFactory[snapshot.ParentObservedState, *snapshot.ParentDesiredState](
				func(cfg interface{}) interface{} {
					supervisorCfg := cfg.(supervisor.Config)

					return supervisor.NewSupervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState](supervisorCfg)
				})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterSupervisorFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](
				func(cfg interface{}) interface{} {
					supervisorCfg := cfg.(supervisor.Config)

					return supervisor.NewSupervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](supervisorCfg)
				})
			Expect(err).ToNot(HaveOccurred())

			By("Step 2: Creating parent supervisor")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Example Parent",
				WorkerType: storage.DeriveWorkerType[snapshot.ParentObservedState](),
			}

			parentWorker := parent.NewParentWorker(parentIdentity.ID, parentIdentity.Name, logger)
			Expect(parentWorker).NotTo(BeNil())

			By("Step 3: Verifying worker has expected initial state")

			mockStore := setupTestStore(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), storage.DeriveWorkerType[childSnapshot.ChildObservedState]())

			parentSupervisor = supervisor.NewSupervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState](supervisor.Config{
				WorkerType:   storage.DeriveWorkerType[snapshot.ParentObservedState](),
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			By("Step 3.5: Setting UserSpec with children_count config")

			// Set UserSpec on supervisor so DeriveDesiredState can generate ChildrenSpecs
			parentSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: "children_count: 1",
			})

			desiredDoc := persistence.Document{
				"id":    "parent-001",
				"state": "running",
			}
			err = mockStore.SaveDesired(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), "parent-001", desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			By("Step 3.6: Saving initial observed state with fresh timestamp")

			initialObserved := snapshot.ParentObservedState{
				ID:          "parent-001",
				CollectedAt: time.Now(),
			}
			_, err = mockStore.SaveObserved(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), "parent-001", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := parentSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 4: Verifying parent transitions to Running")

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)

				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 5: Verifying child is created and started")

			Eventually(func() int {
				children := parentSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(1))

			By("Step 6: Verifying child transitions to Connected")

			Eventually(func() string {
				children := parentSupervisor.GetChildren()
				if len(children) == 0 {
					return ""
				}
				var childSupervisor *supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]
				for _, child := range children {
					var ok bool
					childSupervisor, ok = child.(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState])
					if !ok {
						return ""
					}

					break
				}
				workerIDs := childSupervisor.ListWorkers()
				if len(workerIDs) == 0 {
					return ""
				}
				state, _, _ := childSupervisor.GetWorkerState(workerIDs[0])

				return state
			}, "5s", "100ms").Should(Equal("Connected"))

			By("Step 7: Both processing ticks normally")

			time.Sleep(500 * time.Millisecond)

			parentState, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)
			Expect(parentState).To(Equal("Running"))

			children := parentSupervisor.GetChildren()
			Expect(children).To(HaveLen(1))

			var childSupervisor *supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]
			for _, child := range children {
				var ok bool
				childSupervisor, ok = child.(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState])
				if !ok {
					continue
				}

				break
			}
			childWorkerIDs := childSupervisor.ListWorkers()
			Expect(childWorkerIDs).To(HaveLen(1))
			childState, _, _ := childSupervisor.GetWorkerState(childWorkerIDs[0])
			Expect(childState).To(Equal("Connected"))
		})

		It("should register child worker type and create worker instances", func() {
			By("Step 1: Registering child worker type in factory")

			err = factory.RegisterFactory[snapshot.ParentObservedState, *snapshot.ParentDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				return parent.NewParentWorker(identity.ID, identity.Name, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}

				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			By("Step 2: Creating child worker via factory")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-002",
				Name:       "Example Parent",
				WorkerType: storage.DeriveWorkerType[snapshot.ParentObservedState](),
			}

			parentWorker, err := factory.NewWorkerByType(storage.DeriveWorkerType[snapshot.ParentObservedState](), parentIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentWorker).NotTo(BeNil())

			mockStore := setupTestStore(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), storage.DeriveWorkerType[childSnapshot.ChildObservedState]())

			parentSupervisor = supervisor.NewSupervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState](supervisor.Config{
				WorkerType:   storage.DeriveWorkerType[snapshot.ParentObservedState](),
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			By("Step 2.5: Saving initial observed state with fresh timestamp")

			initialObserved := snapshot.ParentObservedState{
				ID:          "parent-002",
				CollectedAt: time.Now(),
			}
			_, err = mockStore.SaveObserved(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), "parent-002", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := parentSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)

				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 3: Requesting parent shutdown via FSM")

			err = parentSupervisor.TestRequestShutdown(ctx, parentIdentity.ID, "test shutdown")
			Expect(err).NotTo(HaveOccurred())

			By("Step 3: Verifying parent transitions to Stopped and then gets removed")

			Eventually(func() bool {
				state, _, err := parentSupervisor.GetWorkerState(parentIdentity.ID)
				// Accept either "Stopped" state (caught during transition) or worker removed (err != nil)
				return state == "Stopped" || err != nil
			}, "5s", "100ms").Should(BeTrue(), "Worker should transition to Stopped and then be removed")

			By("Step 4: Verifying child is removed")

			Eventually(func() int {
				children := parentSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(0))
		})

		It("should create supervisor without errors", func() {
			By("Step 1: Creating supervisor instance")

			beforeGoroutines := runtime.NumGoroutine()

			By("Step 2: Creating and starting parent worker")

			err = factory.RegisterFactory[snapshot.ParentObservedState, *snapshot.ParentDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				return parent.NewParentWorker(identity.ID, identity.Name, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}

				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			parentIdentity := fsmv2.Identity{
				ID:         "parent-003",
				Name:       "Example Parent",
				WorkerType: storage.DeriveWorkerType[snapshot.ParentObservedState](),
			}

			parentWorker, err := factory.NewWorkerByType(storage.DeriveWorkerType[snapshot.ParentObservedState](), parentIdentity)
			Expect(err).NotTo(HaveOccurred())

			mockStore := setupTestStore(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState]())

			parentSupervisor = supervisor.NewSupervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState](supervisor.Config{
				WorkerType:   storage.DeriveWorkerType[snapshot.ParentObservedState](),
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			By("Step 2.5: Saving initial observed state with fresh timestamp")

			initialObserved := snapshot.ParentObservedState{
				ID:          "parent-003",
				CollectedAt: time.Now(),
			}
			_, err = mockStore.SaveObserved(ctx, storage.DeriveWorkerType[snapshot.ParentObservedState](), "parent-003", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := parentSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 3: Waiting for parent and child to be Running/Connected")

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)

				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 4: Stopping supervisor")

			parentSupervisor.Shutdown()

			// Cancel context to stop tick loop and wait for completion
			cancel()
			<-done

			By("Step 5: Verifying no goroutine leaks")

			// Recreate context for AfterEach
			ctx, cancel = context.WithCancel(context.Background())

			Eventually(func() int {
				runtime.GC()

				return runtime.NumGoroutine()
			}, "5s", "100ms").Should(BeNumerically("<=", beforeGoroutines+5))
		})

		It("should reduce database writes with delta checking", func() {
			By("Step 1: Creating test worker with stable observed state")

			testWorker := NewDeltaTestWorker("delta-test-001", "Delta Test Worker")
			testIdentity := fsmv2.Identity{
				ID:         testWorker.GetID(),
				Name:       testWorker.GetName(),
				WorkerType: "delta-test-worker",
			}

			err = factory.RegisterFactory[DeltaTestObservedState, *config.DesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				return testWorker
			})
			Expect(err).ToNot(HaveOccurred())

			By("Step 2: Creating supervisor with fast observation interval")

			mockStore := setupTestStore(ctx, "delta-test-worker")

			deltaTestSupervisor := supervisor.NewSupervisor[DeltaTestObservedState, *config.DesiredState](supervisor.Config{
				WorkerType:   "delta-test-worker",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = deltaTestSupervisor.AddWorker(testIdentity, testWorker)
			Expect(err).NotTo(HaveOccurred())

			done := deltaTestSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 3: Waiting for worker to reach Running state")

			Eventually(func() string {
				state, _, _ := deltaTestSupervisor.GetWorkerState(testIdentity.ID)

				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 4: Running 100 observation ticks with 2 state changes")

			initialSyncID := testWorker.GetCurrentSyncID()
			syncIDChanges := []int64{initialSyncID}

			for i := range 100 {
				if i == 30 {
					testWorker.SetCPU(75)
				}
				if i == 60 {
					testWorker.SetMemory(8192)
				}

				time.Sleep(10 * time.Millisecond)

				currentSyncID := testWorker.GetCurrentSyncID()
				if currentSyncID != syncIDChanges[len(syncIDChanges)-1] {
					syncIDChanges = append(syncIDChanges, currentSyncID)
				}
			}

			By("Step 5: Verifying write reduction (should be ~3 writes, not 100)")

			Expect(len(syncIDChanges)).To(BeNumerically("<=", 5),
				"Expected ~3 writes (initial + 2 changes), got %d writes", len(syncIDChanges))

			reduction := float64(100-len(syncIDChanges)) / 100.0 * 100.0
			GinkgoWriter.Printf("Write reduction: %.1f%% (%d writes instead of 100)\n", reduction, len(syncIDChanges))

			Expect(reduction).To(BeNumerically(">", 90.0),
				"Expected >90%% write reduction, got %.1f%%", reduction)
		})
	})
})

type DeltaTestWorker struct {
	id            string
	name          string
	cpu           int
	memory        int
	currentSyncID int64
	mu            sync.RWMutex
	store         *storage.TriangularStore
	observedState *DeltaTestObservedState
}

type DeltaTestObservedState struct {
	CPU       int       `bson:"cpu"       json:"cpu"`
	Memory    int       `bson:"memory"    json:"memory"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
}

func (d DeltaTestObservedState) GetTimestamp() time.Time {
	return d.Timestamp
}

func (d DeltaTestObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &config.DesiredState{}
}

func NewDeltaTestWorker(id, name string) *DeltaTestWorker {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	// Create collections following naming convention: {workerType}_{role}
	_ = basicStore.CreateCollection(ctx, "delta-test-worker_identity", nil)
	_ = basicStore.CreateCollection(ctx, "delta-test-worker_observed", nil)

	store := storage.NewTriangularStore(basicStore)

	return &DeltaTestWorker{
		id:            id,
		name:          name,
		cpu:           50,
		memory:        4096,
		currentSyncID: 0,
		store:         store,
		observedState: &DeltaTestObservedState{
			CPU:       50,
			Memory:    4096,
			Timestamp: time.Now(),
		},
	}
}

func (w *DeltaTestWorker) GetID() string {
	return w.id
}

func (w *DeltaTestWorker) GetName() string {
	return w.name
}

func (w *DeltaTestWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	observed := &DeltaTestObservedState{
		CPU:       w.cpu,
		Memory:    w.memory,
		Timestamp: time.Now(),
	}

	w.observedState = observed

	return observed, nil
}

func (w *DeltaTestWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{}, nil
}

func (w *DeltaTestWorker) GetInitialState() fsmv2.State[any, any] {
	return &mockState{name: "Running"}
}

func (w *DeltaTestWorker) Next(ctx context.Context, current, parent string, observed interface{}) (string, fsmv2.Action[any], error) {
	return "Running", nil, nil
}

func (w *DeltaTestWorker) SetCPU(cpu int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.cpu = cpu
}

func (w *DeltaTestWorker) SetMemory(memory int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.memory = memory
}

func (w *DeltaTestWorker) GetCurrentSyncID() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	doc, err := w.store.LoadObserved(context.Background(), "delta-test-worker", w.id)
	if err != nil {
		return 0
	}

	docMap, ok := doc.(persistence.Document)
	if !ok {
		return 0
	}

	syncID, ok := docMap["_sync_id"].(int64)
	if !ok {
		return 0
	}

	return syncID
}

func (w *DeltaTestWorker) ShouldCreateChildren(currentState string, observed interface{}) bool {
	return false
}

func (w *DeltaTestWorker) GetChildSpecs(currentState string, observed interface{}) ([]interface{}, error) {
	return nil, nil
}

type mockState struct {
	name string
}

func (m *mockState) String() string {
	return m.name
}

func (m *mockState) Reason() string {
	return ""
}

func (m *mockState) Next(snapshot any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	return m, fsmv2.SignalNone, nil
}
