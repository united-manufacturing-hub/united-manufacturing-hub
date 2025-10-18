package cse_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

// Test constructor
func TestNewTxCache(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()

	cache := cse.NewTxCache(store, registry)

	if cache == nil {
		t.Fatal("NewTxCache() returned nil")
	}
}

// Test starting a new transaction
func TestTxCache_BeginTx(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	metadata := map[string]interface{}{
		"saga_type": "deploy_bridge",
		"worker_id": "worker-123",
	}

	err := cache.BeginTx("tx-123", metadata)
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	// Verify transaction is pending
	pending, err := cache.GetPending()
	if err != nil {
		t.Fatalf("GetPending() failed: %v", err)
	}

	if len(pending) != 1 {
		t.Errorf("Expected 1 pending transaction, got %d", len(pending))
	}

	if pending[0].TxID != "tx-123" {
		t.Errorf("Wrong transaction ID: %v", pending[0].TxID)
	}

	if pending[0].Status != cse.TxStatusPending {
		t.Errorf("Wrong status: %v", pending[0].Status)
	}

	if pending[0].Metadata["saga_type"] != "deploy_bridge" {
		t.Errorf("Metadata not preserved: %v", pending[0].Metadata)
	}
}

// Test recording operations within a transaction
func TestTxCache_RecordOp(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)

	op := cse.CachedOp{
		OpType:     "insert",
		Collection: "container_desired",
		ID:         "worker-123",
		Data:       basic.Document{"config": "value"},
		Timestamp:  time.Now(),
	}

	err := cache.RecordOp("tx-123", op)
	if err != nil {
		t.Fatalf("RecordOp() failed: %v", err)
	}

	// Verify operation was recorded
	pending, err := cache.GetPending()
	if err != nil {
		t.Fatalf("GetPending() failed: %v", err)
	}

	if len(pending[0].Ops) != 1 {
		t.Errorf("Expected 1 operation, got %d", len(pending[0].Ops))
	}

	if pending[0].Ops[0].OpType != "insert" {
		t.Errorf("Wrong operation type: %v", pending[0].Ops[0].OpType)
	}

	if pending[0].Ops[0].Collection != "container_desired" {
		t.Errorf("Wrong collection: %v", pending[0].Ops[0].Collection)
	}
}

// Test recording multiple operations
func TestTxCache_RecordMultipleOps(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)

	// Record 3 operations
	for i := 0; i < 3; i++ {
		op := cse.CachedOp{
			OpType:     "insert",
			Collection: "test",
			ID:         "doc-" + strconv.Itoa(i),
			Data:       basic.Document{"value": i},
			Timestamp:  time.Now(),
		}
		cache.RecordOp("tx-123", op)
	}

	pending, _ := cache.GetPending()
	if len(pending[0].Ops) != 3 {
		t.Errorf("Expected 3 operations, got %d", len(pending[0].Ops))
	}
}

// Test committing a transaction
func TestTxCache_Commit(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})

	err := cache.Commit("tx-123")
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Verify transaction is no longer pending
	pending, _ := cache.GetPending()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending transactions after commit, got %d", len(pending))
	}
}

// Test rolling back a transaction
func TestTxCache_Rollback(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})

	err := cache.Rollback("tx-123")
	if err != nil {
		t.Fatalf("Rollback() failed: %v", err)
	}

	// Verify transaction is no longer pending
	pending, _ := cache.GetPending()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending transactions after rollback, got %d", len(pending))
	}
}

// Test flushing cache to storage
func TestTxCache_Flush(t *testing.T) {
	store := newMockStore()
	if err := store.CreateCollection(context.Background(), "_tx_cache", nil); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", map[string]interface{}{"saga_type": "deploy"})
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})

	err := cache.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify data was written to storage
	docs, err := store.Find(context.Background(), "_tx_cache", basic.Query{})
	if err != nil {
		t.Fatalf("Find() failed: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("Expected 1 document in storage, got %d", len(docs))
	}

	// Verify document structure
	doc := docs[0]
	if doc["id"] != "tx-123" {
		t.Errorf("Wrong document ID: %v", doc["id"])
	}
	if doc["status"] != string(cse.TxStatusPending) {
		t.Errorf("Wrong status: %v", doc["status"])
	}
}

// Test flushing committed transaction
func TestTxCache_FlushCommitted(t *testing.T) {
	store := newMockStore()
	if err := store.CreateCollection(context.Background(), "_tx_cache", nil); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)
	cache.Commit("tx-123")

	err := cache.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify committed transaction was persisted
	docs, _ := store.Find(context.Background(), "_tx_cache", basic.Query{})
	if len(docs) != 1 {
		t.Errorf("Expected 1 document in storage, got %d", len(docs))
	}

	if docs[0]["status"] != string(cse.TxStatusCommitted) {
		t.Errorf("Wrong status after flush: %v", docs[0]["status"])
	}

	if docs[0]["finished_at"] == nil {
		t.Error("Expected finished_at to be set after commit")
	}
}

// Test replaying operations from a transaction
func TestTxCache_Replay(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "update",
		Collection: "test",
		ID:         "doc-2",
		Data:       basic.Document{"value": 456},
		Timestamp:  time.Now(),
	})

	executedOps := []cse.CachedOp{}
	executor := func(op cse.CachedOp) error {
		executedOps = append(executedOps, op)
		return nil
	}

	err := cache.Replay(context.Background(), "tx-123", executor)
	if err != nil {
		t.Fatalf("Replay() failed: %v", err)
	}

	if len(executedOps) != 2 {
		t.Errorf("Expected 2 executed operations, got %d", len(executedOps))
	}

	if executedOps[0].ID != "doc-1" {
		t.Errorf("Wrong first operation replayed: %v", executedOps[0])
	}

	if executedOps[1].ID != "doc-2" {
		t.Errorf("Wrong second operation replayed: %v", executedOps[1])
	}
}

// Test replaying with executor error
func TestTxCache_ReplayWithError(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	cache.BeginTx("tx-123", nil)
	cache.RecordOp("tx-123", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})

	expectedErr := fmt.Errorf("executor failed")
	executor := func(op cse.CachedOp) error {
		return expectedErr
	}

	err := cache.Replay(context.Background(), "tx-123", executor)
	if err == nil {
		t.Error("Expected Replay() to fail when executor returns error")
	}
}

// Test cleanup of old transactions
func TestTxCache_Cleanup(t *testing.T) {
	store := newMockStore()
	if err := store.CreateCollection(context.Background(), "_tx_cache", nil); err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	// Create old committed transaction
	cache.BeginTx("tx-old", nil)
	cache.Commit("tx-old")

	// Create recent committed transaction
	cache.BeginTx("tx-new", nil)
	cache.Commit("tx-new")

	// Create pending transaction (should not be cleaned up)
	cache.BeginTx("tx-pending", nil)

	// Flush to storage
	cache.Flush(context.Background())

	// Manually update the old transaction's finished_at to 25 hours ago
	oldTime := time.Now().Add(-25 * time.Hour)
	oldDoc, _ := store.Get(context.Background(), "_tx_cache", "tx-old")
	oldDoc["finished_at"] = oldTime.Format(time.RFC3339)
	if err := store.Update(context.Background(), "_tx_cache", "tx-old", oldDoc); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Run cleanup with 24 hour TTL
	err := cache.Cleanup(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() failed: %v", err)
	}

	// Verify old transaction was removed
	docs, _ := store.Find(context.Background(), "_tx_cache", basic.Query{})
	if len(docs) != 2 {
		t.Errorf("Expected 2 transactions after cleanup (tx-new and tx-pending), got %d", len(docs))
		for i, doc := range docs {
			t.Logf("Doc %d: id=%v, status=%v, finished_at=%v", i, doc["id"], doc["status"], doc["finished_at"])
		}
	}

	// Verify tx-old was removed
	_, err = store.Get(context.Background(), "_tx_cache", "tx-old")
	if err != basic.ErrNotFound {
		t.Error("Expected tx-old to be removed after cleanup")
	}
}

// Test getting pending transactions only
func TestTxCache_GetPending_OnlyPending(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	// Create pending transaction
	cache.BeginTx("tx-pending", nil)

	// Create committed transaction
	cache.BeginTx("tx-committed", nil)
	cache.Commit("tx-committed")

	// Create failed transaction
	cache.BeginTx("tx-failed", nil)
	cache.Rollback("tx-failed")

	pending, err := cache.GetPending()
	if err != nil {
		t.Fatalf("GetPending() failed: %v", err)
	}

	if len(pending) != 1 {
		t.Errorf("Expected 1 pending transaction, got %d", len(pending))
	}

	if pending[0].TxID != "tx-pending" {
		t.Errorf("Wrong pending transaction: %v", pending[0].TxID)
	}
}

// Test error handling for non-existent transaction
func TestTxCache_RecordOp_NonExistentTx(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	err := cache.RecordOp("tx-nonexistent", cse.CachedOp{
		OpType:     "insert",
		Collection: "test",
		ID:         "doc-1",
		Data:       basic.Document{"value": 123},
		Timestamp:  time.Now(),
	})

	if err == nil {
		t.Error("Expected RecordOp() to fail for non-existent transaction")
	}
}

// Test error handling for committing non-existent transaction
func TestTxCache_Commit_NonExistentTx(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	err := cache.Commit("tx-nonexistent")
	if err == nil {
		t.Error("Expected Commit() to fail for non-existent transaction")
	}
}

// Test concurrent access to cache
func TestTxCache_ConcurrentAccess(t *testing.T) {
	store := newMockStore()
	registry := cse.NewRegistry()
	cache := cse.NewTxCache(store, registry)

	// Start multiple transactions concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			txID := "tx-" + string(rune('0'+id))
			cache.BeginTx(txID, nil)
			cache.RecordOp(txID, cse.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-" + string(rune('0'+id)),
				Data:       basic.Document{"value": id},
				Timestamp:  time.Now(),
			})
			cache.Commit(txID)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all transactions completed
	pending, _ := cache.GetPending()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending transactions, got %d", len(pending))
	}
}
