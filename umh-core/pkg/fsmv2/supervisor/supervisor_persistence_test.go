// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

type mockBasicStore struct {
	collections map[string]map[string]basic.Document
	mu          sync.RWMutex
}

func newMockBasicStore() *mockBasicStore {
	return &mockBasicStore{
		collections: make(map[string]map[string]basic.Document),
	}
}

func (m *mockBasicStore) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collections[name] != nil {
		return errors.New("collection already exists")
	}
	m.collections[name] = make(map[string]basic.Document)
	return nil
}

func (m *mockBasicStore) DropCollection(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collections[name] == nil {
		return basic.ErrNotFound
	}
	delete(m.collections, name)
	return nil
}

func (m *mockBasicStore) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collections[collection] == nil {
		m.collections[collection] = make(map[string]basic.Document)
	}
	id := doc["id"].(string)
	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}
	m.collections[collection][id] = docCopy
	return id, nil
}

func (m *mockBasicStore) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.collections[collection] == nil {
		return nil, basic.ErrNotFound
	}
	doc, exists := m.collections[collection][id]
	if !exists {
		return nil, basic.ErrNotFound
	}
	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}
	return docCopy, nil
}

func (m *mockBasicStore) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collections[collection] == nil {
		return basic.ErrNotFound
	}
	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}
	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}
	m.collections[collection][id] = docCopy
	return nil
}

func (m *mockBasicStore) Delete(ctx context.Context, collection string, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.collections[collection] == nil {
		return basic.ErrNotFound
	}
	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}
	delete(m.collections[collection], id)
	return nil
}

func (m *mockBasicStore) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.collections[collection] == nil {
		return []basic.Document{}, nil
	}
	results := make([]basic.Document, 0)
	for _, doc := range m.collections[collection] {
		docCopy := make(basic.Document)
		for k, v := range doc {
			docCopy[k] = v
		}
		results = append(results, docCopy)
	}
	return results, nil
}

func (m *mockBasicStore) BeginTx(ctx context.Context) (basic.Tx, error) {
	return &mockBasicTx{mockStore: m, rollback: false}, nil
}

func (m *mockBasicStore) Close(ctx context.Context) error {
	return nil
}

func (m *mockBasicStore) Maintenance(ctx context.Context) error {
	return nil
}

type mockBasicTx struct {
	mockStore *mockBasicStore
	rollback  bool
}

func (tx *mockBasicTx) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	return tx.mockStore.CreateCollection(ctx, name, schema)
}

func (tx *mockBasicTx) DropCollection(ctx context.Context, name string) error {
	return tx.mockStore.DropCollection(ctx, name)
}

func (tx *mockBasicTx) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	return tx.mockStore.Insert(ctx, collection, doc)
}

func (tx *mockBasicTx) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	return tx.mockStore.Get(ctx, collection, id)
}

func (tx *mockBasicTx) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	return tx.mockStore.Update(ctx, collection, id, doc)
}

func (tx *mockBasicTx) Delete(ctx context.Context, collection string, id string) error {
	return tx.mockStore.Delete(ctx, collection, id)
}

func (tx *mockBasicTx) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	return tx.mockStore.Find(ctx, collection, query)
}

func (tx *mockBasicTx) BeginTx(ctx context.Context) (basic.Tx, error) {
	return tx.mockStore.BeginTx(ctx)
}

func (tx *mockBasicTx) Close(ctx context.Context) error {
	return tx.mockStore.Close(ctx)
}

func (tx *mockBasicTx) Commit() error {
	if tx.rollback {
		return errors.New("transaction already rolled back")
	}
	return nil
}

func (tx *mockBasicTx) Rollback() error {
	tx.rollback = true
	return nil
}

func (tx *mockBasicTx) Maintenance(ctx context.Context) error {
	return tx.mockStore.Maintenance(ctx)
}

func setupPersistenceTestRegistry() *storage.Registry {
	registry := storage.NewRegistry()
	registry.Register(&storage.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          "container_desired",
		WorkerType:    "container",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	registry.Register(&storage.CollectionMetadata{
		Name:          "container_observed",
		WorkerType:    "container",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	return registry
}

var _ = Describe("Supervisor Persistence", func() {
	var (
		ctx           context.Context
		basicStore    basic.Store
		registry      *storage.Registry
		triangleStore *storage.TriangularStore
		s             *supervisor.Supervisor
		workerID      string
	)

	BeforeEach(func() {
		ctx = context.Background()
		workerID = "test-worker-123"

		basicStore = newMockBasicStore()
		registry = setupPersistenceTestRegistry()
		triangleStore = storage.NewTriangularStore(basicStore, registry)

		s = supervisor.NewSupervisor(supervisor.Config{
			WorkerType: "container",
			Store:      triangleStore,
			Logger:     zap.NewNop().Sugar(),
		})
	})

	AfterEach(func() {
		if basicStore != nil {
			_ = basicStore.Close(ctx)
		}
	})

	Describe("AddWorker persistence", func() {
		It("should persist identity when worker is added", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			worker := &mockContainerWorker{
				identity: &container.ContainerIdentity{
					ID:   workerID,
					Name: "Test Container",
				},
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			doc, err := triangleStore.LoadIdentity(ctx, "container", workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(doc).ToNot(BeNil())
			Expect(doc["id"]).To(Equal(workerID))
			Expect(doc["Name"]).To(Equal("Test Container"))
		})

		It("should persist initial desired state when worker is added", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			worker := &mockContainerWorker{
				identity: &container.ContainerIdentity{
					ID:   workerID,
					Name: "Test Container",
				},
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			doc, err := triangleStore.LoadDesired(ctx, "container", workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(doc).ToNot(BeNil())
			Expect(doc["shutdownRequested"]).To(BeFalse())
		})

		It("should persist initial observed state when worker is added", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			observedState := &container.ContainerObservedState{
				CPUUsageMCores:  500.0,
				CPUCoreCount:    4,
				MemoryUsedBytes: 1024 * 1024 * 1024,
				CollectedAt:     time.Now(),
			}

			worker := &mockContainerWorker{
				identity: &container.ContainerIdentity{
					ID:   workerID,
					Name: "Test Container",
				},
				observedState: observedState,
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			doc, err := triangleStore.LoadObserved(ctx, "container", workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(doc).ToNot(BeNil())
			Expect(doc["CPUUsageMCores"]).To(BeNumerically("==", 500.0))
		})

		It("should validate worker returns consistent types on AddWorker", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			callCount := 0
			worker := &mockWorker{
				collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
					callCount++
					if callCount == 1 {
						return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
					}
					return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
				},
			}

			Expect(func() {
				_ = s.AddWorker(identity, worker)
			}).To(Panic())
		})
	})

	Describe("Observation loop persistence", func() {
		It("should persist observed state updates from collector", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			initialState := &container.ContainerObservedState{
				CPUUsageMCores: 500.0,
				CollectedAt:    time.Now(),
			}

			updatedState := &container.ContainerObservedState{
				CPUUsageMCores: 800.0,
				CollectedAt:    time.Now(),
			}

			worker := &mockContainerWorker{
				identity: &container.ContainerIdentity{
					ID:   workerID,
					Name: "Test Container",
				},
				observedState: initialState,
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			doc, err := triangleStore.LoadObserved(ctx, "container", workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(doc["CPUUsageMCores"]).To(BeNumerically("==", 500.0))

			worker.observedState = updatedState

			workerCtx, err := s.GetWorker(workerID)
			Expect(err).ToNot(HaveOccurred())

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).ToNot(HaveOccurred())

			observedDoc, err := supervisor.ToDocument(observed)
			Expect(err).ToNot(HaveOccurred())

			err = triangleStore.SaveObserved(ctx, "container", workerID, observedDoc)
			Expect(err).ToNot(HaveOccurred())

			doc, err = triangleStore.LoadObserved(ctx, "container", workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(doc["CPUUsageMCores"]).To(BeNumerically("==", 800.0))

			_ = workerCtx
		})
	})

	Describe("Type validation", func() {
		It("should cache observed type on AddWorker", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Container",
				WorkerType: "container",
			}

			worker := &mockContainerWorker{
				identity: &container.ContainerIdentity{
					ID:   workerID,
					Name: "Test Container",
				},
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			expectedType := reflect.TypeOf(&container.ContainerObservedState{})
			types := s.GetObservedTypes()
			Expect(types[workerID]).To(Equal(expectedType))
		})
	})
})

type mockContainerWorker struct {
	identity      *container.ContainerIdentity
	observedState *container.ContainerObservedState
}

func (m *mockContainerWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.observedState != nil {
		return m.observedState, nil
	}
	return &container.ContainerObservedState{
		CPUUsageMCores: 500.0,
		CPUCoreCount:   4,
		CollectedAt:    time.Now(),
	}, nil
}

func (m *mockContainerWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &container.ContainerDesiredState{}, nil
}

func (m *mockContainerWorker) GetInitialState() fsmv2.State {
	return &container.ActiveState{}
}
