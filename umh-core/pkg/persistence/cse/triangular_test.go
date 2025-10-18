package cse_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

// mockStore implements basic.Store interface for testing.
// Provides in-memory storage with collection-based isolation.
type mockStore struct {
	collections map[string]map[string]basic.Document
	mu          sync.RWMutex
}

func newMockStore() *mockStore {
	return &mockStore{
		collections: make(map[string]map[string]basic.Document),
	}
}

func (m *mockStore) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] != nil {
		return errors.New("collection already exists")
	}

	m.collections[name] = make(map[string]basic.Document)
	return nil
}

func (m *mockStore) DropCollection(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[name] == nil {
		return basic.ErrNotFound
	}

	delete(m.collections, name)
	return nil
}

func (m *mockStore) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		m.collections[collection] = make(map[string]basic.Document)
	}

	id := doc["id"].(string)

	// Copy document to prevent external mutations
	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	m.collections[collection][id] = docCopy
	return id, nil
}

func (m *mockStore) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.collections[collection] == nil {
		return nil, basic.ErrNotFound
	}

	doc, exists := m.collections[collection][id]
	if !exists {
		return nil, basic.ErrNotFound
	}

	// Copy document to prevent external mutations
	docCopy := make(basic.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	return docCopy, nil
}

func (m *mockStore) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collections[collection] == nil {
		return basic.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}

	// Copy document to prevent external mutations
	docCopy := make(basic.Document)
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
		return basic.ErrNotFound
	}

	if _, exists := m.collections[collection][id]; !exists {
		return basic.ErrNotFound
	}

	delete(m.collections[collection], id)
	return nil
}

func (m *mockStore) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
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

func (m *mockStore) BeginTx(ctx context.Context) (basic.Tx, error) {
	return &mockTx{mockStore: m, rollback: false}, nil
}

func (m *mockStore) Close() error {
	return nil
}

// mockTx implements basic.Tx interface for testing.
type mockTx struct {
	mockStore *mockStore
	rollback  bool
}

func (tx *mockTx) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	return tx.mockStore.CreateCollection(ctx, name, schema)
}

func (tx *mockTx) DropCollection(ctx context.Context, name string) error {
	return tx.mockStore.DropCollection(ctx, name)
}

func (tx *mockTx) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	return tx.mockStore.Insert(ctx, collection, doc)
}

func (tx *mockTx) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	return tx.mockStore.Get(ctx, collection, id)
}

func (tx *mockTx) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	return tx.mockStore.Update(ctx, collection, id, doc)
}

func (tx *mockTx) Delete(ctx context.Context, collection string, id string) error {
	return tx.mockStore.Delete(ctx, collection, id)
}

func (tx *mockTx) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	return tx.mockStore.Find(ctx, collection, query)
}

func (tx *mockTx) BeginTx(ctx context.Context) (basic.Tx, error) {
	return tx.mockStore.BeginTx(ctx)
}

func (tx *mockTx) Close() error {
	return tx.mockStore.Close()
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

// setupTestRegistry creates a registry with triangular collections for testing.
func setupTestRegistry() *cse.Registry {
	registry := cse.NewRegistry()

	registry.Register(&cse.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          cse.RoleIdentity,
		CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
		IndexedFields: []string{cse.FieldSyncID},
	})

	registry.Register(&cse.CollectionMetadata{
		Name:          "container_desired",
		WorkerType:    "container",
		Role:          cse.RoleDesired,
		CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
		IndexedFields: []string{cse.FieldSyncID},
	})

	registry.Register(&cse.CollectionMetadata{
		Name:          "container_observed",
		WorkerType:    "container",
		Role:          cse.RoleObserved,
		CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
		IndexedFields: []string{cse.FieldSyncID},
	})

	return registry
}

func TestTriangularStore_NewTriangularStore(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()

	ts := cse.NewTriangularStore(store, registry)

	if ts == nil {
		t.Fatal("NewTriangularStore() returned nil")
	}
}

func TestTriangularStore_SaveIdentity(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	identity := basic.Document{
		"id":   "worker-123",
		"name": "Container A",
		"ip":   "192.168.1.100",
	}

	err := ts.SaveIdentity(context.Background(), "container", "worker-123", identity)
	if err != nil {
		t.Fatalf("SaveIdentity() failed: %v", err)
	}

	// Verify CSE metadata was injected
	saved, err := store.Get(context.Background(), "container_identity", "worker-123")
	if err != nil {
		t.Fatalf("Failed to retrieve saved identity: %v", err)
	}

	if saved[cse.FieldSyncID] == nil {
		t.Error("_sync_id not injected")
	}

	if saved[cse.FieldVersion] != int64(1) {
		t.Errorf("_version = %v, want 1", saved[cse.FieldVersion])
	}

	if saved[cse.FieldCreatedAt] == nil {
		t.Error("_created_at not injected")
	}

	// Verify original fields preserved
	if saved["name"] != "Container A" {
		t.Errorf("name = %v, want 'Container A'", saved["name"])
	}

	if saved["ip"] != "192.168.1.100" {
		t.Errorf("ip = %v, want '192.168.1.100'", saved["ip"])
	}
}

func TestTriangularStore_LoadIdentity(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save identity first
	identity := basic.Document{
		"id":   "worker-123",
		"name": "Container A",
	}
	ts.SaveIdentity(context.Background(), "container", "worker-123", identity)

	// Load identity
	loaded, err := ts.LoadIdentity(context.Background(), "container", "worker-123")
	if err != nil {
		t.Fatalf("LoadIdentity() failed: %v", err)
	}

	if loaded["name"] != "Container A" {
		t.Errorf("name = %v, want 'Container A'", loaded["name"])
	}

	// Test loading non-existent worker
	_, err = ts.LoadIdentity(context.Background(), "container", "nonexistent")
	if !errors.Is(err, basic.ErrNotFound) {
		t.Errorf("LoadIdentity() for nonexistent worker should return ErrNotFound, got %v", err)
	}
}

func TestTriangularStore_SaveDesired(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// First save
	desired := basic.Document{
		"id":     "worker-123",
		"config": "value1",
	}
	err := ts.SaveDesired(context.Background(), "container", "worker-123", desired)
	if err != nil {
		t.Fatalf("SaveDesired() failed: %v", err)
	}

	saved, _ := store.Get(context.Background(), "container_desired", "worker-123")
	firstSyncID := saved[cse.FieldSyncID].(int64)
	firstVersion := saved[cse.FieldVersion].(int64)

	if firstVersion != 1 {
		t.Errorf("First save _version = %v, want 1", firstVersion)
	}

	// Second save (update)
	desired["config"] = "value2"
	err = ts.SaveDesired(context.Background(), "container", "worker-123", desired)
	if err != nil {
		t.Fatalf("SaveDesired() second save failed: %v", err)
	}

	saved, _ = store.Get(context.Background(), "container_desired", "worker-123")
	secondSyncID := saved[cse.FieldSyncID].(int64)
	secondVersion := saved[cse.FieldVersion].(int64)

	if secondSyncID <= firstSyncID {
		t.Errorf("SaveDesired() should increment _sync_id: first=%v, second=%v", firstSyncID, secondSyncID)
	}

	if secondVersion != 2 {
		t.Errorf("SaveDesired() should increment _version to 2, got %v", secondVersion)
	}

	if saved[cse.FieldUpdatedAt] == nil {
		t.Error("_updated_at not set on update")
	}
}

func TestTriangularStore_LoadDesired(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save desired first
	desired := basic.Document{
		"id":     "worker-123",
		"config": "value",
	}
	ts.SaveDesired(context.Background(), "container", "worker-123", desired)

	// Load desired
	loaded, err := ts.LoadDesired(context.Background(), "container", "worker-123")
	if err != nil {
		t.Fatalf("LoadDesired() failed: %v", err)
	}

	if loaded["config"] != "value" {
		t.Errorf("config = %v, want 'value'", loaded["config"])
	}
}

func TestTriangularStore_SaveObserved(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// First save
	observed := basic.Document{
		"id":     "worker-123",
		"status": "running",
		"cpu":    50,
	}
	err := ts.SaveObserved(context.Background(), "container", "worker-123", observed)
	if err != nil {
		t.Fatalf("SaveObserved() failed: %v", err)
	}

	saved, _ := store.Get(context.Background(), "container_observed", "worker-123")
	firstSyncID := saved[cse.FieldSyncID].(int64)
	firstVersion := saved[cse.FieldVersion].(int64)

	if firstVersion != 1 {
		t.Errorf("First save _version = %v, want 1", firstVersion)
	}

	// Second save (update)
	observed["cpu"] = 60
	err = ts.SaveObserved(context.Background(), "container", "worker-123", observed)
	if err != nil {
		t.Fatalf("SaveObserved() second save failed: %v", err)
	}

	saved, _ = store.Get(context.Background(), "container_observed", "worker-123")
	secondSyncID := saved[cse.FieldSyncID].(int64)
	secondVersion := saved[cse.FieldVersion].(int64)

	// Critical test: Observed should increment sync ID but NOT version
	if secondSyncID <= firstSyncID {
		t.Errorf("SaveObserved() should increment _sync_id: first=%v, second=%v", firstSyncID, secondSyncID)
	}

	if secondVersion != 1 {
		t.Errorf("SaveObserved() should NOT increment _version (ephemeral data), expected 1, got %v", secondVersion)
	}
}

func TestTriangularStore_LoadObserved(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save observed first
	observed := basic.Document{
		"id":     "worker-123",
		"status": "running",
	}
	ts.SaveObserved(context.Background(), "container", "worker-123", observed)

	// Load observed
	loaded, err := ts.LoadObserved(context.Background(), "container", "worker-123")
	if err != nil {
		t.Fatalf("LoadObserved() failed: %v", err)
	}

	if loaded["status"] != "running" {
		t.Errorf("status = %v, want 'running'", loaded["status"])
	}
}

func TestTriangularStore_LoadSnapshot(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save all three parts
	ts.SaveIdentity(context.Background(), "container", "worker-123", basic.Document{
		"id":   "worker-123",
		"name": "Container A",
	})
	ts.SaveDesired(context.Background(), "container", "worker-123", basic.Document{
		"id":     "worker-123",
		"config": "value",
	})
	ts.SaveObserved(context.Background(), "container", "worker-123", basic.Document{
		"id":     "worker-123",
		"status": "running",
	})

	// Load snapshot
	snapshot, err := ts.LoadSnapshot(context.Background(), "container", "worker-123")
	if err != nil {
		t.Fatalf("LoadSnapshot() failed: %v", err)
	}

	if snapshot.Identity["name"] != "Container A" {
		t.Error("Identity not loaded correctly")
	}
	if snapshot.Desired["config"] != "value" {
		t.Error("Desired not loaded correctly")
	}
	if snapshot.Observed["status"] != "running" {
		t.Error("Observed not loaded correctly")
	}
}

func TestTriangularStore_LoadSnapshot_MissingParts(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save only identity
	ts.SaveIdentity(context.Background(), "container", "worker-123", basic.Document{
		"id":   "worker-123",
		"name": "Container A",
	})

	// Attempt to load snapshot (should fail - missing desired and observed)
	_, err := ts.LoadSnapshot(context.Background(), "container", "worker-123")
	if err == nil {
		t.Error("LoadSnapshot() should fail when parts are missing")
	}
}

func TestTriangularStore_DeleteWorker(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save all three parts
	ts.SaveIdentity(context.Background(), "container", "worker-123", basic.Document{
		"id":   "worker-123",
		"name": "Container A",
	})
	ts.SaveDesired(context.Background(), "container", "worker-123", basic.Document{
		"id":     "worker-123",
		"config": "value",
	})
	ts.SaveObserved(context.Background(), "container", "worker-123", basic.Document{
		"id":     "worker-123",
		"status": "running",
	})

	// Delete worker
	err := ts.DeleteWorker(context.Background(), "container", "worker-123")
	if err != nil {
		t.Fatalf("DeleteWorker() failed: %v", err)
	}

	// Verify all parts deleted
	_, err = store.Get(context.Background(), "container_identity", "worker-123")
	if !errors.Is(err, basic.ErrNotFound) {
		t.Error("Identity should be deleted")
	}

	_, err = store.Get(context.Background(), "container_desired", "worker-123")
	if !errors.Is(err, basic.ErrNotFound) {
		t.Error("Desired should be deleted")
	}

	_, err = store.Get(context.Background(), "container_observed", "worker-123")
	if !errors.Is(err, basic.ErrNotFound) {
		t.Error("Observed should be deleted")
	}
}

func TestTriangularStore_GlobalSyncID_Increments(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Save identity
	ts.SaveIdentity(context.Background(), "container", "worker-1", basic.Document{
		"id": "worker-1",
	})
	identity1, _ := store.Get(context.Background(), "container_identity", "worker-1")
	syncID1 := identity1[cse.FieldSyncID].(int64)

	// Save desired (different worker)
	ts.SaveDesired(context.Background(), "container", "worker-2", basic.Document{
		"id": "worker-2",
	})
	desired2, _ := store.Get(context.Background(), "container_desired", "worker-2")
	syncID2 := desired2[cse.FieldSyncID].(int64)

	// Save observed (yet another worker)
	ts.SaveObserved(context.Background(), "container", "worker-3", basic.Document{
		"id": "worker-3",
	})
	observed3, _ := store.Get(context.Background(), "container_observed", "worker-3")
	syncID3 := observed3[cse.FieldSyncID].(int64)

	// Verify global sync ID increments across all operations
	if syncID2 <= syncID1 {
		t.Errorf("Global sync ID should increment: identity=%v, desired=%v", syncID1, syncID2)
	}
	if syncID3 <= syncID2 {
		t.Errorf("Global sync ID should increment: desired=%v, observed=%v", syncID2, syncID3)
	}
}

func TestTriangularStore_UnregisteredWorkerType(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Attempt to save identity for unregistered worker type
	err := ts.SaveIdentity(context.Background(), "nonexistent", "worker-123", basic.Document{
		"id": "worker-123",
	})
	if err == nil {
		t.Error("SaveIdentity() should fail for unregistered worker type")
	}
}

func TestTriangularStore_TimestampProgression(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// First save
	ts.SaveDesired(context.Background(), "container", "worker-123", basic.Document{
		"id": "worker-123",
	})
	first, _ := store.Get(context.Background(), "container_desired", "worker-123")
	createdAt := first[cse.FieldCreatedAt].(time.Time)

	// Small delay to ensure time progresses
	time.Sleep(10 * time.Millisecond)

	// Second save
	ts.SaveDesired(context.Background(), "container", "worker-123", basic.Document{
		"id": "worker-123",
	})
	second, _ := store.Get(context.Background(), "container_desired", "worker-123")
	updatedAt := second[cse.FieldUpdatedAt].(time.Time)

	if !updatedAt.After(createdAt) {
		t.Errorf("_updated_at should be after _created_at: created=%v, updated=%v", createdAt, updatedAt)
	}
}

func TestTriangularStore_DocumentValidation(t *testing.T) {
	store := newMockStore()
	registry := setupTestRegistry()
	ts := cse.NewTriangularStore(store, registry)

	// Test nil document
	err := ts.SaveIdentity(context.Background(), "container", "worker-123", nil)
	if err == nil {
		t.Error("SaveIdentity() should fail for nil document")
	}

	// Test document without id field
	err = ts.SaveIdentity(context.Background(), "container", "worker-123", basic.Document{
		"name": "Container A",
	})
	if err == nil {
		t.Error("SaveIdentity() should fail for document without 'id' field")
	}

	// Test same for desired
	err = ts.SaveDesired(context.Background(), "container", "worker-123", basic.Document{
		"config": "value",
	})
	if err == nil {
		t.Error("SaveDesired() should fail for document without 'id' field")
	}

	// Test same for observed
	err = ts.SaveObserved(context.Background(), "container", "worker-123", basic.Document{
		"status": "running",
	})
	if err == nil {
		t.Error("SaveObserved() should fail for document without 'id' field")
	}
}
