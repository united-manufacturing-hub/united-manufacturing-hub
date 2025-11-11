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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

func setupTestStore(ctx context.Context, workerType string) *storage.TriangularStore {
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_identity",
		WorkerType:    workerType,
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_desired",
		WorkerType:    workerType,
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_observed",
		WorkerType:    workerType,
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)

	return storage.NewTriangularStore(basicStore, registry)
}

var _ = Describe("Phase 0: Happy Path Integration", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		parentSupervisor  *supervisor.Supervisor
		logger            *zap.SugaredLogger
		initialGoroutines int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		factory.ResetRegistry()
	})

	AfterEach(func() {
		if parentSupervisor != nil {
			parentSupervisor.Shutdown()
		}
		cancel()

		Eventually(func() int {
			runtime.GC()
			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Basic Factory Registration", func() {
		It("should register parent worker type and create worker instances", func() {
			By("Step 1: Registering parent worker type in factory")

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger)
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})

			By("Step 2: Creating parent supervisor")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentWorker).NotTo(BeNil())

			By("Step 3: Verifying worker has expected initial state")

			mockStore := setupTestStore(ctx, parent.WorkerType)

			parentSupervisor = supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   parent.WorkerType,
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			<-parentSupervisor.Start(ctx)

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
				var childSupervisor *supervisor.Supervisor
				for _, child := range children {
					childSupervisor = child
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

			var childSupervisor *supervisor.Supervisor
			for _, child := range children {
				childSupervisor = child
				break
			}
			childWorkerIDs := childSupervisor.ListWorkers()
			Expect(childWorkerIDs).To(HaveLen(1))
			childState, _, _ := childSupervisor.GetWorkerState(childWorkerIDs[0])
			Expect(childState).To(Equal("Connected"))
		})

		It("should register child worker type and create worker instances", func() {
			By("Step 1: Registering child worker type in factory")

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger)
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})

			By("Step 2: Creating child worker via factory")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-002",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentWorker).NotTo(BeNil())

			mockStore := setupTestStore(ctx, parent.WorkerType)

			parentSupervisor = supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   parent.WorkerType,
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			<-parentSupervisor.Start(ctx)

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)
				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 2: Requesting parent shutdown")

			parentSupervisor.Shutdown()

			By("Step 3: Verifying parent transitions to Stopped")

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)
				return state
			}, "5s", "100ms").Should(Equal("Stopped"))

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

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger)
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})

			parentIdentity := fsmv2.Identity{
				ID:         "parent-003",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())

			mockStore := setupTestStore(ctx, parent.WorkerType)

			parentSupervisor = supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   parent.WorkerType,
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err = parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			<-parentSupervisor.Start(ctx)

			By("Step 3: Waiting for parent and child to be Running/Connected")

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(parentIdentity.ID)
				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 4: Stopping supervisor")

			parentSupervisor.Shutdown()

			By("Step 5: Verifying no goroutine leaks")

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

			factory.RegisterWorkerType("delta-test-worker", func(identity fsmv2.Identity) fsmv2.Worker {
				return testWorker
			})

			By("Step 2: Creating supervisor with fast observation interval")

			mockStore := setupTestStore(ctx, "delta-test-worker")

			parentSupervisor = supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   "delta-test-worker",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err := parentSupervisor.AddWorker(testIdentity, testWorker)
			Expect(err).NotTo(HaveOccurred())

			<-parentSupervisor.Start(ctx)

			By("Step 3: Waiting for worker to reach Running state")

			Eventually(func() string {
				state, _, _ := parentSupervisor.GetWorkerState(testIdentity.ID)
				return state
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 4: Running 100 observation ticks with 2 state changes")

			initialSyncID := testWorker.GetCurrentSyncID()
			syncIDChanges := []int64{initialSyncID}

			for i := 0; i < 100; i++ {
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
	id             string
	name           string
	cpu            int
	memory         int
	currentSyncID  int64
	mu             sync.RWMutex
	store          *storage.TriangularStore
	observedState  *DeltaTestObservedState
}

type DeltaTestObservedState struct {
	CPU       int       `bson:"cpu" json:"cpu"`
	Memory    int       `bson:"memory" json:"memory"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
}

func (d *DeltaTestObservedState) GetTimestamp() time.Time {
	return d.Timestamp
}

func (d *DeltaTestObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &config.DesiredState{}
}

func NewDeltaTestWorker(id, name string) *DeltaTestWorker {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()
	registry := storage.NewRegistry()

	registry.Register(&storage.CollectionMetadata{
		Name:          "delta-test-worker_identity",
		WorkerType:    "delta-test-worker",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          "delta-test-worker_observed",
		WorkerType:    "delta-test-worker",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	_ = basicStore.CreateCollection(ctx, "delta-test-worker_identity", nil)
	_ = basicStore.CreateCollection(ctx, "delta-test-worker_observed", nil)

	store := storage.NewTriangularStore(basicStore, registry)

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

func (w *DeltaTestWorker) GetInitialState() fsmv2.State {
	return &mockState{name: "Running"}
}

func (w *DeltaTestWorker) Next(ctx context.Context, current, parent string, observed interface{}) (string, fsmv2.Action, error) {
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

	syncID, ok := doc["_sync_id"].(int64)
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

func (m *mockState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}
