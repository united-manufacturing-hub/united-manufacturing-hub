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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

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

var _ = Describe("TriangularStore", func() {
	var (
		store *mockStore
		ts    *storage.TriangularStore
		ctx   context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		ts = storage.NewTriangularStore(store, zap.NewNop().Sugar())
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
			err := ts.SaveIdentity(ctx, "container", "worker-123", identity)
			Expect(err).ToNot(HaveOccurred())
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
			_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set version to 1 on first save", func() {
			_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "container_desired", "worker-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)))
		})

		Context("when updating desired state", func() {
			BeforeEach(func() {
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should increment sync ID", func() {
				saved, _ := store.Get(ctx, "container_desired", "worker-123")
				firstSyncID := saved[storage.FieldSyncID].(int64)

				desired["config"] = "value2"
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).ToNot(HaveOccurred())

				saved, _ = store.Get(ctx, "container_desired", "worker-123")
				secondSyncID := saved[storage.FieldSyncID].(int64)

				Expect(secondSyncID).To(BeNumerically(">", firstSyncID))
			})

			It("should increment version", func() {
				desired["config"] = "value2"
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				saved, err := store.Get(ctx, "container_desired", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(saved[storage.FieldVersion]).To(Equal(int64(2)))
			})

			It("should set updated_at timestamp", func() {
				desired["config"] = "value2"
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
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
			_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
			Expect(err).ToNot(HaveOccurred())
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

		Context("after registry elimination (Task 2.4)", func() {
			It("should always return Document regardless of TypeRegistry", func() {
				desired := persistence.Document{
					"id":      "worker-123",
					"name":    "parent-worker",
					"command": "start",
				}
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				doc, ok := loaded.(persistence.Document)
				Expect(ok).To(BeTrue(), "LoadDesired always returns Document after registry elimination")
				Expect(doc["name"]).To(Equal("parent-worker"))
				Expect(doc["command"]).To(Equal("start"))
			})

			It("should return Document even with channels in data", func() {
				desired := persistence.Document{
					"id":   "worker-123",
					"data": make(chan int),
				}
				_, err := ts.SaveDesired(ctx, "container", "worker-123", desired)
				Expect(err).NotTo(HaveOccurred())

				loaded, err := ts.LoadDesired(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				_, ok := loaded.(persistence.Document)
				Expect(ok).To(BeTrue(), "LoadDesired returns Document regardless of data types")
			})
		})
	})

	Describe("SaveDesired without registry (Task 2.4 TDD)", func() {
		It("should work with convention-based collection names", func() {
			doc := persistence.Document{
				"id":     "worker-456",
				"config": "test-value",
			}

			_, err := ts.SaveDesired(ctx, "testworker", "worker-456", doc)
			Expect(err).NotTo(HaveOccurred())

			saved, err := store.Get(ctx, "testworker_desired", "worker-456")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved["config"]).To(Equal("test-value"))
		})

		It("should inject CSE metadata using constants", func() {
			doc := persistence.Document{
				"id":    "worker-789",
				"field": "value",
			}

			_, err := ts.SaveDesired(ctx, "testworker", "worker-789", doc)
			Expect(err).NotTo(HaveOccurred())

			loaded, err := store.Get(ctx, "testworker_desired", "worker-789")
			Expect(err).NotTo(HaveOccurred())

			Expect(loaded).To(HaveKey(storage.FieldSyncID))
			Expect(loaded).To(HaveKey(storage.FieldVersion))
			Expect(loaded).To(HaveKey(storage.FieldCreatedAt))
			Expect(loaded[storage.FieldVersion]).To(Equal(int64(1)))

			// Update to verify _updated_at is set on updates
			doc["field"] = "updated-value"
			_, err = ts.SaveDesired(ctx, "testworker", "worker-789", doc)
			Expect(err).NotTo(HaveOccurred())

			updated, err := store.Get(ctx, "testworker_desired", "worker-789")
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(HaveKey(storage.FieldUpdatedAt))
			Expect(updated[storage.FieldVersion]).To(Equal(int64(2)))
		})
	})

	Describe("LoadDesired without registry (Task 2.4 TDD)", func() {
		BeforeEach(func() {
			doc := persistence.Document{
				"id":   "worker-999",
				"data": "test-data",
			}
			_, err := ts.SaveDesired(ctx, "testworker", "worker-999", doc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should always return Document using convention-based collection name", func() {
			result, err := ts.LoadDesired(ctx, "testworker", "worker-999")
			Expect(err).NotTo(HaveOccurred())

			doc, ok := result.(persistence.Document)
			Expect(ok).To(BeTrue(), "LoadDesired should always return Document")
			Expect(doc["data"]).To(Equal("test-data"))
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
			err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   "worker-123",
				"name": "Container A",
			})
			Expect(err).ToNot(HaveOccurred())
			_, err = ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id":     "worker-123",
				"config": "value",
			})
			Expect(err).ToNot(HaveOccurred())
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
			It("should succeed with nil desired and observed when only identity exists", func() {
				ts := storage.NewTriangularStore(newMockStore(), zap.NewNop().Sugar())
				err := ts.SaveIdentity(ctx, "container", "worker-456", persistence.Document{
					"id":   "worker-456",
					"name": "Container B",
				})
				Expect(err).ToNot(HaveOccurred())

				snapshot, err := ts.LoadSnapshot(ctx, "container", "worker-456")
				Expect(err).ToNot(HaveOccurred())
				Expect(snapshot.Identity).ToNot(BeNil())
				Expect(snapshot.Identity["name"]).To(Equal("Container B"))
				Expect(snapshot.Desired).To(BeNil())
				Expect(snapshot.Observed).To(BeNil())
			})
		})

		Context("cache isolation", func() {
			It("should return deep copies so callers cannot corrupt the cache", func() {
				// First load - gets from store and caches
				snapshot1, err := ts.LoadSnapshot(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot1.Identity["name"]).To(Equal("Container A"))

				// Mutate the returned snapshot - this should NOT affect the cache
				snapshot1.Identity["name"] = "CORRUPTED"
				snapshot1.Desired["config"] = "CORRUPTED"
				observedDoc1, ok := snapshot1.Observed.(persistence.Document)
				Expect(ok).To(BeTrue())
				observedDoc1["status"] = "CORRUPTED"

				// Second load - should get from cache but return fresh copy
				snapshot2, err := ts.LoadSnapshot(ctx, "container", "worker-123")
				Expect(err).NotTo(HaveOccurred())

				// Verify cache was not corrupted
				Expect(snapshot2.Identity["name"]).To(Equal("Container A"), "cache was corrupted by caller mutation")
				Expect(snapshot2.Desired["config"]).To(Equal("value"), "cache was corrupted by caller mutation")
				observedDoc2, ok := snapshot2.Observed.(persistence.Document)
				Expect(ok).To(BeTrue())
				Expect(observedDoc2["status"]).To(Equal("running"), "cache was corrupted by caller mutation")
			})
		})
	})

	Describe("GlobalSyncID", func() {
		It("should increment across all operations", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-1", persistence.Document{
				"id": "worker-1",
			})
			Expect(err).NotTo(HaveOccurred())
			identity1, _ := store.Get(ctx, "container_identity", "worker-1")
			syncID1 := identity1[storage.FieldSyncID].(int64)

			_, err = ts.SaveDesired(ctx, "container", "worker-2", persistence.Document{
				"id": "worker-2",
			})
			Expect(err).NotTo(HaveOccurred())
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

	Describe("TimestampProgression", func() {
		It("should have updated_at after created_at", func() {
			_, err := ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id": "worker-123",
			})
			Expect(err).NotTo(HaveOccurred())
			first, _ := store.Get(ctx, "container_desired", "worker-123")
			createdAt := first[storage.FieldCreatedAt].(time.Time)

			time.Sleep(10 * time.Millisecond)

			// Add a field change so delta checking allows the update
			_, err = ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
				"id":      "worker-123",
				"updated": true,
			})
			Expect(err).NotTo(HaveOccurred())
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

		It("should fail for document with non-string id field", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   123, // int instead of string
				"name": "Container A",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty string"))
		})

		It("should fail for document with empty string id field", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   "",
				"name": "Container A",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty string"))
		})

		It("should fail for document with mismatched id field", func() {
			err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
				"id":   "worker-456", // different from parameter
				"name": "Container A",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not match"))
		})

		Context("for desired state", func() {
			It("should fail for document without id field", func() {
				_, err := ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
					"config": "value",
				})
				Expect(err).To(HaveOccurred())
			})

			It("should fail for document with mismatched id field", func() {
				_, err := ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
					"id":     "worker-456", // different from parameter
					"config": "value",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not match"))
			})
		})

		Context("for observed state", func() {
			It("should succeed for document without id field (id is auto-injected from parameter)", func() {
				// SaveObserved auto-injects id from the function parameter
				_, err := ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
					"status": "running",
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("deriveWorkerType", func() {
		It("should derive parent from ParentDesiredState", func() {
			workerType, err := storage.DeriveWorkerType[ParentDesiredState]()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerType).To(Equal("parent"))
		})

		It("should derive parent from ParentObservedState", func() {
			workerType, err := storage.DeriveWorkerType[ParentObservedState]()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerType).To(Equal("parent"))
		})

		It("should derive communicator from CommunicatorObservedState", func() {
			workerType, err := storage.DeriveWorkerType[CommunicatorObservedState]()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerType).To(Equal("communicator"))
		})

		It("should derive child from ChildDesiredState", func() {
			workerType, err := storage.DeriveWorkerType[ChildDesiredState]()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerType).To(Equal("child"))
		})

		It("should return error for type without DesiredState or ObservedState suffix", func() {
			_, err := storage.DeriveWorkerType[InvalidType]()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not end with DesiredState or ObservedState"))
		})

		It("should return error for type without state suffix (EmptyNameType)", func() {
			// EmptyNameType has a name but doesn't end with DesiredState or ObservedState
			// This tests the same path as InvalidType - types that don't follow the naming convention
			_, err := storage.DeriveWorkerType[EmptyNameType]()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not end with DesiredState or ObservedState"))
		})
	})

	Describe("DeriveCollectionName", func() {
		It("should derive collection name for identity role", func() {
			name, err := storage.DeriveCollectionName[TestObservedState](storage.RoleIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("test_identity"))
		})

		It("should derive collection name for desired role", func() {
			name, err := storage.DeriveCollectionName[TestObservedState](storage.RoleDesired)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("test_desired"))
		})

		It("should derive collection name for observed role", func() {
			name, err := storage.DeriveCollectionName[TestObservedState](storage.RoleObserved)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("test_observed"))
		})
	})

	Describe("LoadDesired[T]", func() {
		It("should derive table name from type and load typed desired state", func() {
			desired := persistence.Document{
				"id":      "parent-001",
				"name":    "ParentWorker",
				"command": "start",
			}
			_, err := ts.SaveDesired(ctx, "parent", "parent-001", desired)
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

	Describe("SaveDesiredTyped[T]", func() {
		It("should derive table name from type and save typed desired state", func() {
			desired := ParentDesiredState{
				Name:    "ParentWorker",
				Command: "start",
			}

			_, err := storage.SaveDesiredTyped[ParentDesiredState](ts, ctx, "parent-001", desired)
			Expect(err).NotTo(HaveOccurred())

			result, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, "parent-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("ParentWorker"))
			Expect(result.Command).To(Equal("start"))
		})

		It("should serialize struct to Document before saving", func() {
			desired := ParentDesiredState{
				Name:    "TestWorker",
				Command: "stop",
			}

			_, err := storage.SaveDesiredTyped[ParentDesiredState](ts, ctx, "parent-002", desired)
			Expect(err).NotTo(HaveOccurred())

			doc, err := ts.LoadDesired(ctx, "parent", "parent-002")
			Expect(err).NotTo(HaveOccurred())
			docMap, ok := doc.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(docMap["name"]).To(Equal("TestWorker"))
			Expect(docMap["command"]).To(Equal("stop"))
		})
	})

	Describe("LoadObserved without TypeRegistry (TDD for Task 2.3)", func() {
		It("should always return Document, never use TypeRegistry", func() {
			observed := TestObservedState{
				ID:     "worker-tdd-test",
				Status: "running",
				CPU:    int64(75),
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-tdd-test", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := ts.LoadObserved(ctx, "container", "worker-tdd-test")
			Expect(err).NotTo(HaveOccurred())

			doc, ok := result.(persistence.Document)
			Expect(ok).To(BeTrue(), "LoadObserved should return Document, not typed struct")
			Expect(doc["id"]).To(Equal("worker-tdd-test"))
			Expect(doc["status"]).To(Equal("running"))
			Expect(doc["cpu"]).To(BeNumerically("==", 75))
		})
	})

	Describe("LoadObserved with TypeRegistry (deprecated tests)", func() {
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

		It("should always return Document (TypeRegistry no longer used)", func() {
			observed := TestObservedState{
				ID:     "worker-typed",
				Status: "running",
				CPU:    int64(75),
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-typed", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := ts.LoadObserved(ctx, "container", "worker-typed")
			Expect(err).NotTo(HaveOccurred())

			doc, ok := result.(persistence.Document)
			Expect(ok).To(BeTrue(), "LoadObserved always returns Document now")
			Expect(doc["id"]).To(Equal("worker-typed"))
			Expect(doc["status"]).To(Equal("running"))
			Expect(doc["cpu"]).To(BeNumerically("==", 75))
		})

		It("should return Document with any data (no deserialization errors)", func() {
			observed := persistence.Document{
				"id":     "worker-bad-data",
				"status": "running",
				"cpu":    "not-a-number",
			}
			_, err := ts.SaveObserved(ctx, "container", "worker-bad-data", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := ts.LoadObserved(ctx, "container", "worker-bad-data")
			Expect(err).NotTo(HaveOccurred())

			doc, ok := result.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(doc["cpu"]).To(Equal("not-a-number"))
		})

		It("should return ErrNotFound for non-existent document", func() {
			_, err := ts.LoadObserved(ctx, "container", "nonexistent-worker")
			Expect(err).To(MatchError(persistence.ErrNotFound))
		})
	})

	Describe("LoadObservedTyped[T]", func() {
		It("should derive table name from type and load typed observed state", func() {
			observed := persistence.Document{
				"id":     "parent-001",
				"name":   "ParentWorker",
				"status": "running",
			}
			_, err := ts.SaveObserved(ctx, "parent", "parent-001", observed)
			Expect(err).NotTo(HaveOccurred())

			result, err := storage.LoadObservedTyped[ParentObservedState](ts, ctx, "parent-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeAssignableToTypeOf(ParentObservedState{}))
			Expect(result.Name).To(Equal("ParentWorker"))
			Expect(result.Status).To(Equal("running"))
		})

		It("should return ErrNotFound for non-existent document", func() {
			_, err := storage.LoadObservedTyped[ParentObservedState](ts, ctx, "non-existent")
			Expect(err).To(Equal(persistence.ErrNotFound))
		})
	})

	Describe("SaveObservedTyped[T]", func() {
		It("should derive table name from type and save typed observed state", func() {
			observed := ParentObservedState{
				Name:   "ParentWorker",
				Status: "running",
			}

			changed, err := storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-001", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())

			result, err := storage.LoadObservedTyped[ParentObservedState](ts, ctx, "parent-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("ParentWorker"))
			Expect(result.Status).To(Equal("running"))
		})

		It("should serialize struct to Document before saving", func() {
			observed := ParentObservedState{
				Name:   "TestWorker",
				Status: "stopped",
			}

			changed, err := storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-002", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())

			doc, err := ts.LoadObserved(ctx, "parent", "parent-002")
			Expect(err).NotTo(HaveOccurred())
			docMap, ok := doc.(persistence.Document)
			Expect(ok).To(BeTrue())
			Expect(docMap["name"]).To(Equal("TestWorker"))
			Expect(docMap["status"]).To(Equal("stopped"))
		})

		It("should detect no change when saving identical observed state", func() {
			observed := ParentObservedState{
				Name:   "SameWorker",
				Status: "idle",
			}

			// First save
			changed, err := storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-003", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())

			// Second save with same data
			changed, err = storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-003", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
		})
	})

	Describe("SaveObserved without registry (Task 2.2 TDD)", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "testworker_observed", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should work with convention-based collection names and constant CSE fields", func() {
			observed := persistence.Document{
				"id":     "worker-456",
				"status": "active",
				"memory": 1024,
			}

			changed, err := ts.SaveObserved(ctx, "testworker", "worker-456", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())

			saved, err := store.Get(ctx, "testworker_observed", "worker-456")
			Expect(err).NotTo(HaveOccurred())
			Expect(saved["status"]).To(Equal("active"))
			Expect(saved["memory"]).To(Equal(1024))

			Expect(saved[storage.FieldSyncID]).NotTo(BeNil(), "sync_id should be set")
			Expect(saved[storage.FieldVersion]).To(Equal(int64(1)), "version should be 1")
			Expect(saved[storage.FieldCreatedAt]).NotTo(BeNil(), "created_at should be set")
			_, hasUpdatedAt := saved[storage.FieldUpdatedAt]
			Expect(hasUpdatedAt).To(BeFalse(), "updated_at should NOT be set on first save")
		})

		It("should perform delta checking using constant CSE fields", func() {
			observed := persistence.Document{
				"id":     "worker-789",
				"status": "running",
				"load":   50,
			}

			changed, err := ts.SaveObserved(ctx, "testworker", "worker-789", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())

			changed, err = ts.SaveObserved(ctx, "testworker", "worker-789", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse(), "Second save with same data should return changed=false")

			observed["load"] = 75
			changed, err = ts.SaveObserved(ctx, "testworker", "worker-789", observed)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue(), "Save with different data should return changed=true")
		})
	})

	Describe("Creation Deltas", func() {
		Context("SaveDesired on new document", func() {
			It("should emit a creation delta to the delta store", func() {
				desired := persistence.Document{
					"id":     "new-worker-001",
					"config": "value1",
					"port":   8080,
					"name":   "TestWorker",
				}

				changed, err := ts.SaveDesired(ctx, "container", "new-worker-001", desired)
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue(), "First save should return changed=true")

				// Check delta was stored in _deltas collection
				deltas, err := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				Expect(err).NotTo(HaveOccurred())
				Expect(deltas).ToNot(BeEmpty(), "Delta should be stored")

				// Find the delta for our worker
				var foundDelta persistence.Document
				for _, d := range deltas {
					if d["worker_id"] == "new-worker-001" && d["role"] == "desired" {
						foundDelta = d

						break
					}
				}
				Expect(foundDelta).NotTo(BeNil(), "Delta for new-worker-001 should exist")
				Expect(foundDelta["worker_type"]).To(Equal("container"))
				Expect(foundDelta["changes"]).NotTo(BeEmpty(), "Changes should be recorded")
			})
		})

		Context("SaveObserved on new document", func() {
			It("should emit a creation delta to the delta store", func() {
				observed := persistence.Document{
					"id":     "new-worker-002",
					"status": "running",
					"cpu":    75,
				}

				changed, err := ts.SaveObserved(ctx, "container", "new-worker-002", observed)
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue(), "First save should return changed=true")

				// Check delta was stored
				deltas, err := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				Expect(err).NotTo(HaveOccurred())

				var foundDelta persistence.Document
				for _, d := range deltas {
					if d["worker_id"] == "new-worker-002" && d["role"] == "observed" {
						foundDelta = d

						break
					}
				}
				Expect(foundDelta).NotTo(BeNil(), "Delta for new-worker-002 should exist")
				Expect(foundDelta["changes"]).NotTo(BeEmpty(), "Changes should be recorded")
			})
		})

		Context("SaveIdentity on new document", func() {
			It("should emit a creation delta to the delta store", func() {
				identity := persistence.Document{
					"id":   "new-worker-003",
					"name": "Container A",
					"ip":   "192.168.1.100",
				}

				err := ts.SaveIdentity(ctx, "container", "new-worker-003", identity)
				Expect(err).NotTo(HaveOccurred())

				// Check delta was stored (identity now emits deltas)
				deltas, err := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				Expect(err).NotTo(HaveOccurred())

				var foundDelta persistence.Document
				for _, d := range deltas {
					if d["worker_id"] == "new-worker-003" && d["role"] == "identity" {
						foundDelta = d

						break
					}
				}
				Expect(foundDelta).NotTo(BeNil(), "Identity delta for new-worker-003 should exist")
				Expect(foundDelta["changes"]).NotTo(BeEmpty(), "Changes should be recorded")
			})
		})

		Context("subsequent saves after creation", func() {
			It("should emit update deltas, not creation deltas", func() {
				desired := persistence.Document{
					"id":     "worker-update-test",
					"config": "initial",
				}

				// First save - creation delta
				changed1, err := ts.SaveDesired(ctx, "container", "worker-update-test", desired)
				Expect(err).NotTo(HaveOccurred())
				Expect(changed1).To(BeTrue())

				// Get delta count after first save
				deltas1, _ := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				count1 := len(deltas1)

				// Second save with changed data - update delta
				desired["config"] = "updated"
				changed2, err := ts.SaveDesired(ctx, "container", "worker-update-test", desired)
				Expect(err).NotTo(HaveOccurred())
				Expect(changed2).To(BeTrue())

				// Verify new delta was added
				deltas2, _ := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				Expect(len(deltas2)).To(BeNumerically(">", count1), "Update should create new delta")
			})
		})

		Context("document with no business fields", func() {
			It("should not emit a delta", func() {
				// Get initial delta count
				deltasBefore, _ := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				countBefore := len(deltasBefore)

				observed := persistence.Document{
					"id": "empty-worker",
				}

				changed, err := ts.SaveObserved(ctx, "container", "empty-worker", observed)
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue(), "First save should still return changed=true")

				// Verify no new delta was added (no business fields to record)
				deltasAfter, _ := store.Find(ctx, storage.DeltaCollectionName, persistence.Query{})
				Expect(deltasAfter).To(HaveLen(countBefore), "No delta should be added for document with only id")
			})
		})
	})
})

// Benchmarks for SaveObserved delta checking

func BenchmarkSaveObservedNoChange(b *testing.B) {
	store := newMockStore()
	ts := storage.NewTriangularStore(store, zap.NewNop().Sugar())
	ctx := context.Background()

	// Create initial document
	doc := persistence.Document{
		"id":     "worker-123",
		"status": "running",
		"cpu":    45.2,
	}

	_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)

	b.ResetTimer()

	for range b.N {
		// Same data - should detect no change
		_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)
	}
}

func BenchmarkSaveObservedWithChange(b *testing.B) {
	store := newMockStore()
	ts := storage.NewTriangularStore(store, zap.NewNop().Sugar())
	ctx := context.Background()

	// Create initial document
	doc := persistence.Document{
		"id":     "worker-123",
		"status": "running",
		"cpu":    45.2,
	}

	_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)

	b.ResetTimer()

	for i := range b.N {
		// Changing data each iteration
		doc["cpu"] = float64(i) * 0.1
		_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)
	}
}
