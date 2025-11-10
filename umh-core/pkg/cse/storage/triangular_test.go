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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type mockStore struct {
	collections map[string]map[string]persistence.Document
	mu          sync.RWMutex
}

func newMockStore() *mockStore {
	return &mockStore{
		collections: make(map[string]map[string]persistence.Document),
	}
}

func (m *mockStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] != nil {
		return errors.New("collection already exists")
	}

	m.collections[name] = make(map[string]persistence.Document)

	return nil
}

func (m *mockStore) DropCollection(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] == nil {
		return persistence.ErrNotFound
	}

	delete(m.collections, name)

	return nil
}

func (m *mockStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		m.collections[collection] = make(map[string]persistence.Document)
	}

	id := doc["id"].(string)

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	m.collections[collection][id] = docCopy

	return id, nil
}

func (m *mockStore) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.collections[collection] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, exists := m.collections[collection][id]
	if !exists {
		return nil, persistence.ErrNotFound
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	return docCopy, nil
}

func (m *mockStore) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		return persistence.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return persistence.ErrNotFound
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	m.collections[collection][id] = docCopy

	return nil
}

func (m *mockStore) Delete(ctx context.Context, collection string, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		return persistence.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return persistence.ErrNotFound
	}

	delete(m.collections[collection], id)

	return nil
}

func (m *mockStore) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.collections[collection] == nil {
		return []persistence.Document{}, nil
	}

	results := make([]persistence.Document, 0)
	for _, doc := range m.collections[collection] {
		docCopy := make(persistence.Document)
		for k, v := range doc {
			docCopy[k] = v
		}

		results = append(results, docCopy)
	}

	return results, nil
}

func (m *mockStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return &mockTx{mockStore: m, rollback: false}, nil
}

func (m *mockStore) Close(ctx context.Context) error {
	return nil
}

func (m *mockStore) Maintenance(ctx context.Context) error {
	return nil
}

type mockTx struct {
	mockStore *mockStore
	rollback  bool
}

func (tx *mockTx) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return tx.mockStore.CreateCollection(ctx, name, schema)
}

func (tx *mockTx) DropCollection(ctx context.Context, name string) error {
	return tx.mockStore.DropCollection(ctx, name)
}

func (tx *mockTx) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	return tx.mockStore.Insert(ctx, collection, doc)
}

func (tx *mockTx) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	return tx.mockStore.Get(ctx, collection, id)
}

func (tx *mockTx) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	return tx.mockStore.Update(ctx, collection, id, doc)
}

func (tx *mockTx) Delete(ctx context.Context, collection string, id string) error {
	return tx.mockStore.Delete(ctx, collection, id)
}

func (tx *mockTx) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return tx.mockStore.Find(ctx, collection, query)
}

func (tx *mockTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return tx.mockStore.BeginTx(ctx)
}

func (tx *mockTx) Close(ctx context.Context) error {
	return tx.mockStore.Close(ctx)
}

func (tx *mockTx) Commit() error {
	if tx.rollback {
		return errors.New("transaction already rolled back")
	}

	return nil
}

func (tx *mockTx) Rollback() error {
	tx.rollback = true

	return nil
}

func (tx *mockTx) Maintenance(ctx context.Context) error {
	return tx.mockStore.Maintenance(ctx)
}

func setupTestRegistry() *storage.Registry {
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

var _ = Describe("TriangularStore", func() {
	var (
		store    *mockStore
		registry *storage.Registry
		ts       *storage.TriangularStore
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		registry = setupTestRegistry()
		ts = storage.NewTriangularStore(store, registry)
	})

	Describe("NewTriangularStore", func() {
		It("should create non-nil store", func() {
			Expect(ts).NotTo(BeNil())
		})
	})

	Describe("SaveIdentity", func() {
		var identity persistence.Document

		BeforeEach(func() {
			identity = persistence.Document{
				"id":   "worker-123",
				"name": "Container A",
				"ip":   "192.168.1.100",
			}
		})

		It("should save identity successfully", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", identity)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should inject CSE metadata", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", identity)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_identity", "worker-123")
			Expect(err).NotTo(HaveOccurred())

			Expect(saved[storage.FieldSyncID]).NotTo(BeNil())
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
			Expect(saved[storage.FieldCreatedAt]).NotTo(BeNil())
		})

		It("should preserve original fields", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", identity)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_identity", "worker-123")
			Expect(err).NotTo(HaveOccurred())

			Expect(saved["name"]).To(Equal("Container A"))
			Expect(saved["ip"]).To(Equal("192.168.1.100"))
		})
	})

	Describe("LoadIdentity", func() {
		BeforeEach(func() {
			identity := persistence.Document{
				"id":   "worker-123",
				"name": "Container A",
			}
			ts.SaveIdentity(ctx, "container", "worker-123", identity)
		})

		It("should load identity successfully", func() {
			loaded, err := ts.LoadIdentity(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded["name"]).To(Equal("Container A"))
		})

		Context("when worker does not exist", func() {
			It("should return ErrNotFound", func() {
				_, err := ts.LoadIdentity(ctx, "container", "nonexistent")
				Expect(err).To(MatchError(persistence.ErrNotFound))
			})
		})
	})

	Describe("SaveDesired", func() {
		var desired persistence.Document

		BeforeEach(func() {
			desired = persistence.Document{
				"id":     "worker-123",
				"config": "value1",
			}
		})

		It("should save desired successfully", func() {
			err := ts.SaveDesired(ctx, "container", "worker-123", desired)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set version to 1 on first save", func() {
			err := ts.SaveDesired(ctx, "container", "worker-123", desired)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_desired", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
		})

		Context("when updating desired state", func() {
			BeforeEach(func() {
				ts.SaveDesired(ctx, "container", "worker-123", desired)
			})

			It("should increment sync ID", func() {
				saved, _ := store.Get(ctx, "container_desired", "worker-123")
				firstSyncID := saved[storage.FieldSyncID].(int64)

				desired["config"] = "value2"
				ts.SaveDesired(ctx, "container", "worker-123", desired)

				saved, _ = store.Get(ctx, "container_desired", "worker-123")
				secondSyncID := saved[storage.FieldSyncID].(int64)

				Expect(secondSyncID).To(BeNumerically(">", firstSyncID))
			})

			It("should increment version", func() {
				desired["config"] = "value2"
				err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				saved, err := store.Get(ctx, "container_desired", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(saved[storage.FieldVersion]).To(Equal(int64(2)))
			})

			It("should set updated_at timestamp", func() {
				desired["config"] = "value2"
				err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				saved, err := store.Get(ctx, "container_desired", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(saved[storage.FieldUpdatedAt]).NotTo(BeNil())
			})
		})
	})

	Describe("LoadDesired", func() {
		BeforeEach(func() {
			desired := persistence.Document{
				"id":     "worker-123",
				"config": "value",
			}
			ts.SaveDesired(ctx, "container", "worker-123", desired)
		})

		It("should load desired successfully", func() {
			loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded["config"]).To(Equal("value"))
		})
	})

	Describe("SaveObserved", func() {
		var observed persistence.Document

		BeforeEach(func() {
			observed = persistence.Document{
				"id":     "worker-123",
				"status": "running",
				"cpu":    50,
			}
		})

		It("should save observed successfully", func() {
			err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set version to 1 on first save", func() {
			err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
		})

		Context("when updating observed state", func() {
			BeforeEach(func() {
				ts.SaveObserved(ctx, "container", "worker-123", observed)
			})

			It("should increment sync ID", func() {
				saved, _ := store.Get(ctx, "container_observed", "worker-123")
				firstSyncID := saved[storage.FieldSyncID].(int64)

				observed["cpu"] = 60
				ts.SaveObserved(ctx, "container", "worker-123", observed)

				saved, _ = store.Get(ctx, "container_observed", "worker-123")
				secondSyncID := saved[storage.FieldSyncID].(int64)

				Expect(secondSyncID).To(BeNumerically(">", firstSyncID))
			})

			It("should NOT increment version", func() {
				observed["cpu"] = 60
				err := ts.SaveObserved(ctx, "container", "worker-123", observed)
				Expect(err).NotTo(HaveOccurred())

				saved, err := store.Get(ctx, "container_observed", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
			})
		})
	})

	Describe("LoadObserved", func() {
		BeforeEach(func() {
			observed := persistence.Document{
				"id":     "worker-123",
				"status": "running",
			}
			ts.SaveObserved(ctx, "container", "worker-123", observed)
		})

		It("should load observed successfully", func() {
			loaded, err := ts.LoadObserved(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded["status"]).To(Equal("running"))
		})
	})

	Describe("LoadSnapshot", func() {
		BeforeEach(func() {
			ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   "worker-123",
				"name": "Container A",
			})
			ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id":     "worker-123",
				"config": "value",
			})
			ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
				"id":     "worker-123",
				"status": "running",
			})
		})

		It("should load complete snapshot", func() {
			snapshot, err := ts.LoadSnapshot(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())

			Expect(snapshot.Identity["name"]).To(Equal("Container A"))
			Expect(snapshot.Desired["config"]).To(Equal("value"))
			observedDoc, ok := snapshot.Observed.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(observedDoc["status"]).To(Equal("running"))
		})

		Context("when parts are missing", func() {
			It("should fail when desired and observed are missing", func() {
				ts := storage.NewTriangularStore(newMockStore(), registry)
				ts.SaveIdentity(ctx, "container", "worker-456", persistence.Document{
					"id":   "worker-456",
					"name": "Container B",
				})

				_, err := ts.LoadSnapshot(ctx, "container", "worker-456")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("DeleteWorker", func() {
		BeforeEach(func() {
			ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   "worker-123",
				"name": "Container A",
			})
			ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id":     "worker-123",
				"config": "value",
			})
			ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
				"id":     "worker-123",
				"status": "running",
			})
		})

		It("should delete all three parts", func() {
			err := ts.DeleteWorker(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(ctx, "container_identity", "worker-123")
			Expect(err).To(MatchError(persistence.ErrNotFound))

			_, err = store.Get(ctx, "container_desired", "worker-123")
			Expect(err).To(MatchError(persistence.ErrNotFound))

			_, err = store.Get(ctx, "container_observed", "worker-123")
			Expect(err).To(MatchError(persistence.ErrNotFound))
		})
	})

	Describe("GlobalSyncID", func() {
		It("should increment across all operations", func() {
			ts.SaveIdentity(ctx, "container", "worker-1", persistence.Document{
				"id": "worker-1",
			})
			identity1, _ := store.Get(ctx, "container_identity", "worker-1")
			syncID1 := identity1[storage.FieldSyncID].(int64)

			ts.SaveDesired(ctx, "container", "worker-2", persistence.Document{
				"id": "worker-2",
			})
			desired2, _ := store.Get(ctx, "container_desired", "worker-2")
			syncID2 := desired2[storage.FieldSyncID].(int64)

			ts.SaveObserved(ctx, "container", "worker-3", persistence.Document{
				"id": "worker-3",
			})
			observed3, _ := store.Get(ctx, "container_observed", "worker-3")
			syncID3 := observed3[storage.FieldSyncID].(int64)

			Expect(syncID2).To(BeNumerically(">", syncID1))
			Expect(syncID3).To(BeNumerically(">", syncID2))
		})
	})

	Describe("UnregisteredWorkerType", func() {
		It("should fail for unregistered worker type", func() {
			err := ts.SaveIdentity(ctx, "nonexistent", "worker-123", persistence.Document{
				"id": "worker-123",
			})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("TimestampProgression", func() {
		It("should have updated_at after created_at", func() {
			ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id": "worker-123",
			})
			first, _ := store.Get(ctx, "container_desired", "worker-123")
			createdAt := first[storage.FieldCreatedAt].(time.Time)

			time.Sleep(10 * time.Millisecond)

			ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id": "worker-123",
			})
			second, _ := store.Get(ctx, "container_desired", "worker-123")
			updatedAt := second[storage.FieldUpdatedAt].(time.Time)

			Expect(updatedAt).To(BeTemporally(">", createdAt))
		})
	})

	Describe("DocumentValidation", func() {
		It("should fail for nil document", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", nil)
			Expect(err).To(HaveOccurred())
		})

		It("should fail for document without id field", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"name": "Container A",
			})
			Expect(err).To(HaveOccurred())
		})

		Context("for desired state", func() {
			It("should fail for document without id field", func() {
				err := ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
					"config": "value",
				})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("for observed state", func() {
			It("should fail for document without id field", func() {
				err := ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
					"status": "running",
				})
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("LoadObservedTyped", func() {
		type TestObservedState struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			CPU    int64  `json:"cpu"`
		}

		It("should deserialize Document to typed struct", func() {
			observed := persistence.Document{
				"id":     "worker-123",
				"status": "running",
				"cpu":    int64(50),
			}
			err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			var result TestObservedState
			err = ts.LoadObservedTyped(ctx, "container", "worker-123", &result)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.ID).To(Equal("worker-123"))
			Expect(result.Status).To(Equal("running"))
			Expect(result.CPU).To(Equal(int64(50)))
		})

		It("should return error for type mismatch", func() {
			observed := persistence.Document{
				"id":  "worker-456",
				"cpu": "not-a-number",
			}
			err := ts.SaveObserved(ctx, "container", "worker-456", observed)
			Expect(err).NotTo(HaveOccurred())

			var result TestObservedState
			err = ts.LoadObservedTyped(ctx, "container", "worker-456", &result)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot assign"))
		})
	})
})
