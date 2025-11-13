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
	"reflect"
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

type TestObservedState struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	CPU    int64  `json:"cpu"`
}

type CommunicatorObservedState struct {
	Name string
}

type ChildDesiredState struct {
	Name string
}

type InvalidType struct {
	Name string
}

type EmptyNameType struct {
	Name string
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

	observedType := reflect.TypeOf(TestObservedState{})
	registry.Register(&storage.CollectionMetadata{
		Name:          "container_observed",
		WorkerType:    "container",
		Role:          storage.RoleObserved,
		ObservedType:  observedType,
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

		It("should create typeRegistry", func() {
			typeRegistry := ts.TypeRegistry()
			Expect(typeRegistry).NotTo(BeNil())
		})
	})

	Describe("TypeRegistry", func() {
		It("should return non-nil type registry", func() {
			typeRegistry := ts.TypeRegistry()
			Expect(typeRegistry).NotTo(BeNil())
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

		Context("when no type is registered", func() {
			It("should return Document", func() {
				loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				doc, ok := loaded.(persistence.Document)
				Expect(ok).To(BeTrue(), "should return Document when no type registered")
				Expect(doc["config"]).To(Equal("value"))
			})
		})

		Context("when type is registered", func() {
			var desiredType reflect.Type

			BeforeEach(func() {
				desiredType = reflect.TypeOf(ParentDesiredState{})
				observedType := reflect.TypeOf(ParentObservedState{})

				err := ts.TypeRegistry().RegisterWorkerType("worker-123", observedType, desiredType)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return typed struct", func() {
				desired := persistence.Document{
					"id":      "worker-123",
					"name":    "parent-worker",
					"command": "start",
				}
				err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				typedDesired, ok := loaded.(ParentDesiredState)
				Expect(ok).To(BeTrue(), "should return typed struct when type registered")
				Expect(typedDesired.Name).To(Equal("parent-worker"))
				Expect(typedDesired.Command).To(Equal("start"))
			})

			It("should return error on invalid JSON", func() {
				desired := persistence.Document{
					"id":   "worker-123",
					"data": make(chan int),
				}
				err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				_, err = ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when Document type explicitly registered", func() {
			BeforeEach(func() {
				docType := reflect.TypeOf(persistence.Document{})
				err := ts.TypeRegistry().RegisterWorkerType("worker-123", docType, docType)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return Document", func() {
				loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				doc, ok := loaded.(persistence.Document)
				Expect(ok).To(BeTrue(), "should return Document when Document type explicitly registered")
				Expect(doc["config"]).To(Equal("value"))
			})
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
			_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set version to 1 on first save", func() {
			_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
		})

		Context("when updating observed state", func() {
			BeforeEach(func() {
				_, _ = ts.SaveObserved(ctx, "container", "worker-123", observed)
			})

			It("should increment sync ID", func() {
				saved, _ := store.Get(ctx, "container_observed", "worker-123")
				firstSyncID := saved[storage.FieldSyncID].(int64)

				observed["cpu"] = 60
				_, _ = ts.SaveObserved(ctx, "container", "worker-123", observed)

				saved, _ = store.Get(ctx, "container_observed", "worker-123")
				secondSyncID := saved[storage.FieldSyncID].(int64)

				Expect(secondSyncID).To(BeNumerically(">", firstSyncID))
			})

			It("should NOT increment version", func() {
				observed["cpu"] = 60
				_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
				Expect(err).NotTo(HaveOccurred())

				saved, err := store.Get(ctx, "container_observed", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
			})
		})

		It("should skip unchanged writes by default", func() {
			// Save initial observed state
			_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get initial database state to verify write occurred
			saved, err := store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			firstSyncID := saved[storage.FieldSyncID].(int64)

			// Save SAME observed state again (no changes)
			changed, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse(), "SaveObserved should return changed=false when data is unchanged")

			// Verify no database write occurred (sync_id should be unchanged)
			saved, err = store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			secondSyncID := saved[storage.FieldSyncID].(int64)
			Expect(secondSyncID).To(Equal(firstSyncID), "sync_id should not change when no data changed")
		})

		It("should skip write when only CSE metadata changes", func() {
			// Save initial observed state
			_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get initial database state
			saved, err := store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			firstSyncID := saved[storage.FieldSyncID].(int64)

			// Manually modify ONLY CSE metadata fields in database
			saved[storage.FieldUpdatedAt] = time.Now().Add(10 * time.Hour)
			saved[storage.FieldVersion] = 999
			err = store.Update(ctx, "container_observed", "worker-123", saved)
			Expect(err).NotTo(HaveOccurred())

			// Save same observed state again (user data unchanged, only CSE metadata differs)
			changed, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse(), "SaveObserved should skip write when only CSE metadata changes")

			// Verify no database write occurred (sync_id unchanged)
			saved, err = store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			secondSyncID := saved[storage.FieldSyncID].(int64)
			Expect(secondSyncID).To(Equal(firstSyncID), "sync_id should not change when only CSE metadata differs")
		})

		It("should write when user data changes", func() {
			observed := persistence.Document{
				"id":     "worker-123",
				"cpu":    50,
				"memory": 4096,
			}

			// Save initial observed state
			_, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get initial database state
			saved, err := store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			firstSyncID := saved[storage.FieldSyncID].(int64)

			// Modify user data (not CSE metadata)
			observed["cpu"] = 75
			observed["memory"] = 8192

			// Save DIFFERENT observed state
			changed, err := ts.SaveObserved(ctx, "container", "worker-123", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue(), "SaveObserved should return changed=true when user data changes")

			// Verify database write occurred (sync_id incremented)
			saved, err = store.Get(ctx, "container_observed", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			secondSyncID := saved[storage.FieldSyncID].(int64)
			Expect(secondSyncID).To(BeNumerically(">", firstSyncID), "sync_id should increment when user data changes")

			// Verify new data was actually saved
			Expect(saved["cpu"]).To(Equal(75))
			Expect(saved["memory"]).To(Equal(8192))
		})
	})

	Describe("LoadObserved", func() {
		BeforeEach(func() {
			observed := persistence.Document{
				"id":     "worker-123",
				"status": "running",
			}
			_, _ = ts.SaveObserved(ctx, "container", "worker-123", observed)
		})

		It("should load observed successfully", func() {
			loaded, err := ts.LoadObserved(ctx, "container", "worker-123")
			Expect(err).NotTo(HaveOccurred())

			// LoadObserved returns interface{}, need to assert to Document
			doc, ok := loaded.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(doc["status"]).To(Equal("running"))
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
			_, _ = ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
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

			_, _ = ts.SaveObserved(ctx, "container", "worker-3", persistence.Document{
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
				_, err := ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
					"status": "running",
				})
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("deriveWorkerType", func() {
		It("should derive parent from ParentDesiredState", func() {
			workerType := storage.DeriveWorkerType[ParentDesiredState]()
			Expect(workerType).To(Equal("parent"))
		})

		It("should derive parent from ParentObservedState", func() {
			workerType := storage.DeriveWorkerType[ParentObservedState]()
			Expect(workerType).To(Equal("parent"))
		})

		It("should derive communicator from CommunicatorObservedState", func() {
			workerType := storage.DeriveWorkerType[CommunicatorObservedState]()
			Expect(workerType).To(Equal("communicator"))
		})

		It("should derive child from ChildDesiredState", func() {
			workerType := storage.DeriveWorkerType[ChildDesiredState]()
			Expect(workerType).To(Equal("child"))
		})

		It("should panic for type without DesiredState or ObservedState suffix", func() {
			Expect(func() {
				storage.DeriveWorkerType[InvalidType]()
			}).To(Panic())
		})

		It("should panic for type with empty name", func() {
			Expect(func() {
				storage.DeriveWorkerType[EmptyNameType]()
			}).To(Panic())
		})
	})

	Describe("LoadDesired[T]", func() {
		BeforeEach(func() {
			registry.Register(&storage.CollectionMetadata{
				Name:          "parent_identity",
				WorkerType:    "parent",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			registry.Register(&storage.CollectionMetadata{
				Name:          "parent_desired",
				WorkerType:    "parent",
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			registry.Register(&storage.CollectionMetadata{
				Name:          "parent_observed",
				WorkerType:    "parent",
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})

			desiredType := reflect.TypeOf(ParentDesiredState{})
			ts.TypeRegistry().RegisterWorkerType("parent", nil, desiredType)
		})

		It("should derive table name from type and load typed desired state", func() {
			desired := persistence.Document{
				"id":      "parent-001",
				"name":    "ParentWorker",
				"command": "start",
			}
			err := ts.SaveDesired(ctx, "parent", "parent-001", desired)
			Expect(err).NotTo(HaveOccurred())

			result, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, "parent-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeAssignableToTypeOf(ParentDesiredState{}))
			Expect(result.Name).To(Equal("ParentWorker"))
			Expect(result.Command).To(Equal("start"))
		})

		It("should return ErrNotFound for non-existent document", func() {
			_, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, "non-existent")
			Expect(err).To(Equal(persistence.ErrNotFound))
		})
	})

	Describe("LoadObserved with TypeRegistry", func() {
		It("should return Document when no type registered", func() {
			observed := persistence.Document{
				"id":     "worker-no-type",
				"status": "running",
				"cpu":    int64(50),
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-no-type", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := ts.LoadObserved(ctx, "container", "worker-no-type")
			Expect(err).NotTo(HaveOccurred())

			doc, ok := result.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(doc["id"]).To(Equal("worker-no-type"))
			Expect(doc["status"]).To(Equal("running"))
		})

		It("should return typed struct when type registered", func() {
			// Register type for this worker
			observedType := reflect.TypeOf(TestObservedState{})
			ts.TypeRegistry().RegisterWorkerType("worker-typed", nil, observedType)

			observed := TestObservedState{
				ID:     "worker-typed",
				Status: "running",
				CPU:    int64(75),
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-typed", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := ts.LoadObserved(ctx, "container", "worker-typed")
			Expect(err).NotTo(HaveOccurred())

			typed, ok := result.(TestObservedState)
			Expect(ok).To(BeTrue())
			Expect(typed.ID).To(Equal("worker-typed"))
			Expect(typed.Status).To(Equal("running"))
			Expect(typed.CPU).To(Equal(int64(75)))
		})

		It("should return error on deserialization failure", func() {
			// Register type
			observedType := reflect.TypeOf(TestObservedState{})
			ts.TypeRegistry().RegisterWorkerType("worker-bad-data", nil, observedType)

			// Save invalid data (string instead of int64)
			observed := persistence.Document{
				"id":     "worker-bad-data",
				"status": "running",
				"cpu":    "not-a-number",
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-bad-data", observed)
			Expect(err).NotTo(HaveOccurred())

			_, err = ts.LoadObserved(ctx, "container", "worker-bad-data")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to deserialize"))
		})

		It("should return ErrNotFound for non-existent document", func() {
			_, err := ts.LoadObserved(ctx, "container", "nonexistent-worker")
			Expect(err).To(MatchError(persistence.ErrNotFound))
		})
	})
})
