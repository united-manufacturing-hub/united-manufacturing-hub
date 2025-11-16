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

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// TxCacheCollection is the collection name for storing transaction cache records.
//
// DESIGN DECISION: Prefix with underscore to signal system collection
// WHY: Prevents collision with user data, follows CSE metadata field convention
// TRADE-OFF: Must create collection explicitly at startup
// INSPIRED BY: MongoDB system collections (_id, _sync_id).
const TxCacheCollection = "_tx_cache"

// Operation type constants for transaction cache entries.
//
// DESIGN DECISION: String constants instead of iota enum
// WHY: Serializes directly to JSON/storage without conversion, easier to debug
// TRADE-OFF: No compiler enforcement of valid values, but simplifies storage
// INSPIRED BY: HTTP method strings (GET/POST/PUT/DELETE), SQL operation names.
const (
	OpTypeInsert = "insert"
	OpTypeUpdate = "update"
	OpTypeDelete = "delete"
)

// DefaultTxCacheTTL is the default retention period for completed transactions.
//
// DESIGN DECISION: 24 hours default TTL
// WHY: Balance between recovery window (can replay yesterday's transactions)
// and storage overhead (don't accumulate stale data indefinitely)
// TRADE-OFF: Must call Cleanup() periodically, or storage grows unbounded
// INSPIRED BY: Database transaction logs (1-7 day retention), undo/redo logs.
const DefaultTxCacheTTL = 24 * time.Hour

// TxStatus represents the lifecycle state of a cached transaction.
//
// DESIGN DECISION: String type for status instead of int
// WHY: Self-documenting in logs and storage, easier to debug than numeric codes
// TRADE-OFF: Slightly more storage space than int, but negligible
// INSPIRED BY: HTTP status codes (but as strings), database transaction states.
type TxStatus string

// Transaction status constants.
//
// DESIGN DECISION: Three states (pending/committed/failed), no intermediate states
// WHY: Simple state machine - transaction either in progress, succeeded, or failed
// TRADE-OFF: Can't distinguish "aborting" vs "aborted", but not needed for cache
// INSPIRED BY: Two-phase commit (prepare/commit/abort), database transaction lifecycle.
const (
	// TxStatusPending indicates transaction is in progress, not yet committed or rolled back.
	// Operations are being recorded but not yet finalized.
	TxStatusPending TxStatus = "pending"

	// TxStatusCommitted indicates transaction completed successfully.
	// All operations were applied to storage.
	TxStatusCommitted TxStatus = "committed"

	// TxStatusFailed indicates transaction was rolled back.
	// Operations were NOT applied to storage.
	TxStatusFailed TxStatus = "failed"
)

// CachedOp represents a single operation within a cached transaction.
//
// DESIGN DECISION: Store full document data, not just deltas
// WHY: Enables full replay of transaction without accessing current database state
// TRADE-OFF: Higher storage overhead, but necessary for crash recovery
// INSPIRED BY: Database write-ahead log (WAL), redo log in MySQL/Postgres
//
// Example:
//
//	op := CachedOp{
//	    OpType:     OpTypeInsert,
//	    Collection: "container_desired",
//	    ID:         "worker-123",
//	    Data:       persistence.Document{"id": "worker-123", "config": "production"},
//	    Timestamp:  time.Now(),
//	}
type CachedOp struct {
	// OpType is the operation type: "insert", "update", or "delete"
	OpType string

	// Collection is the target collection name
	Collection string

	// ID is the document identifier
	ID string

	// Data is the complete document data (nil for delete operations)
	Data persistence.Document

	// Timestamp is when the operation was recorded (for ordering within transaction)
	Timestamp time.Time
}

// CachedTx represents a cached transaction with all its operations and metadata.
//
// DESIGN DECISION: Store complete transaction history, not just current state
// WHY: Enables crash recovery, replay, and audit trail
// TRADE-OFF: Memory/storage overhead grows with transaction complexity
// INSPIRED BY: Database transaction log, Git commit objects (full history)
//
// Lifecycle:
//  1. BeginTx() creates CachedTx with status=pending
//  2. RecordOp() appends operations to Ops slice
//  3. Commit() or Rollback() sets status and FinishedAt
//  4. Flush() persists to storage
//  5. Cleanup() removes old transactions after TTL expires
//
// Example:
//
//	tx := &CachedTx{
//	    TxID:      "tx-20250110-001",
//	    Status:    TxStatusPending,
//	    Ops:       []CachedOp{...},
//	    StartedAt: time.Now(),
//	    Metadata:  map[string]interface{}{"source": "fsm_worker", "worker_id": "container-123"},
//	}
type CachedTx struct {
	// TxID is the unique transaction identifier
	TxID string

	// Status is the transaction lifecycle state (pending/committed/failed)
	Status TxStatus

	// Ops is the ordered list of operations in this transaction
	Ops []CachedOp

	// StartedAt is when the transaction began
	StartedAt time.Time

	// FinishedAt is when the transaction completed (nil if still pending)
	FinishedAt *time.Time

	// Metadata contains arbitrary transaction metadata (source, worker ID, etc.)
	Metadata map[string]interface{}
}

// TxCache provides transaction caching for crash recovery and replay.
//
// DESIGN DECISION: Two-tier storage (memory cache + persistent store)
// WHY: Fast in-memory access for active transactions, persistence for recovery
// TRADE-OFF: Must flush to disk periodically or lose recent transactions
// INSPIRED BY: Write-ahead logging (WAL), Redis AOF (append-only file)
//
// Use cases:
//  1. Crash recovery: Replay pending transactions after restart
//  2. Debugging: Audit trail of all database operations
//  3. Distributed sync: Reconstruct state changes for delta sync
//
// Thread-safety: All methods use RWMutex for concurrent access.
// Persistence: Call Flush() to persist in-memory cache to storage.
//
// Example:
//
//	cache := cse.NewTxCache(store)
//
//	// Start transaction
//	cache.BeginTx("tx-001", map[string]interface{}{"source": "fsm"})
//
//	// Record operations
//	cache.RecordOp("tx-001", CachedOp{OpType: OpTypeInsert, Collection: "container_desired", ID: "worker-123", Data: doc})
//
//	// Commit and persist
//	cache.Commit("tx-001")
//	cache.Flush(ctx)
//
//	// Cleanup old transactions (run periodically)
//	cache.Cleanup(ctx, DefaultTxCacheTTL)
type TxCache struct {
	// store is the persistent storage backend
	store persistence.Store

	// cache is the in-memory map of transaction ID to cached transaction
	cache map[string]*CachedTx

	// mu protects concurrent access to cache map
	mu sync.RWMutex
}

// NewTxCache creates a new transaction cache with empty in-memory storage.
//
// DESIGN DECISION: Start with empty cache, not restored from storage
// WHY: Caller controls when to restore state (via explicit Load call in future)
// TRADE-OFF: Must explicitly load persisted transactions if needed
// INSPIRED BY: Database connection pools (create empty, populate on demand)
//
// Parameters:
//   - store: Backend storage for persistence
//
// Returns:
//   - *TxCache: Ready-to-use transaction cache instance
func NewTxCache(store persistence.Store) *TxCache {
	return &TxCache{
		store: store,
		cache: make(map[string]*CachedTx),
	}
}

// BeginTx starts a new cached transaction with optional metadata.
//
// DESIGN DECISION: Allow nil metadata (auto-initialize to empty map)
// WHY: Metadata is optional - not all transactions need it
// TRADE-OFF: Caller can't distinguish "no metadata" from "empty metadata", but doesn't matter
// INSPIRED BY: Database BEGIN TRANSACTION, HTTP request headers (optional)
//
// Parameters:
//   - txID: Unique transaction identifier (caller's responsibility to ensure uniqueness)
//   - metadata: Optional metadata (source, worker ID, etc.) - nil is allowed
//
// Returns:
//   - error: Currently always returns nil (reserved for future validation)
//
// Example:
//
//	cache.BeginTx("tx-001", map[string]interface{}{
//	    "source":    "fsm_worker",
//	    "worker_id": "container-123",
//	})
func (tc *TxCache) BeginTx(txID string, metadata map[string]interface{}) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	tc.cache[txID] = &CachedTx{
		TxID:      txID,
		Status:    TxStatusPending,
		Ops:       make([]CachedOp, 0),
		StartedAt: time.Now(),
		Metadata:  metadata,
	}

	return nil
}

// RecordOp appends an operation to a transaction's operation list.
//
// DESIGN DECISION: Append operations in order, preserve sequence
// WHY: Transaction replay requires exact operation order
// TRADE-OFF: Growing slice for long transactions, but most transactions are small
// INSPIRED BY: Database redo log (append-only, preserves order)
//
// Parameters:
//   - txID: Transaction identifier
//   - op: Operation to record (insert/update/delete with full document data)
//
// Returns:
//   - error: If transaction doesn't exist (must call BeginTx first)
//
// Example:
//
//	op := CachedOp{
//	    OpType:     OpTypeUpdate,
//	    Collection: "container_desired",
//	    ID:         "worker-123",
//	    Data:       persistence.Document{"id": "worker-123", "config": "production"},
//	    Timestamp:  time.Now(),
//	}
//	cache.RecordOp("tx-001", op)
func (tc *TxCache) RecordOp(txID string, op CachedOp) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tx, exists := tc.cache[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	tx.Ops = append(tx.Ops, op)

	return nil
}

// Commit marks a transaction as successfully committed.
//
// DESIGN DECISION: Update status and timestamp in-place, don't remove from cache
// WHY: Keep committed transactions in cache for audit/debugging until Cleanup
// TRADE-OFF: Memory overhead grows until Cleanup runs, but enables post-mortem analysis
// INSPIRED BY: Git commit history (keeps all commits), database transaction log
//
// Parameters:
//   - txID: Transaction identifier
//
// Returns:
//   - error: If transaction doesn't exist
//
// Note: This only updates in-memory state. Call Flush() to persist to storage.
func (tc *TxCache) Commit(txID string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tx, exists := tc.cache[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	now := time.Now()
	tx.Status = TxStatusCommitted
	tx.FinishedAt = &now

	return nil
}

// Rollback marks a transaction as failed/aborted.
//
// DESIGN DECISION: Keep failed transactions in cache, same as committed
// WHY: Failed transactions are useful for debugging - what went wrong?
// TRADE-OFF: Memory overhead, but necessary for troubleshooting
// INSPIRED BY: Database undo log (keeps aborted transactions for analysis)
//
// Parameters:
//   - txID: Transaction identifier
//
// Returns:
//   - error: If transaction doesn't exist
//
// Note: This only updates in-memory state. Call Flush() to persist to storage.
func (tc *TxCache) Rollback(txID string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tx, exists := tc.cache[txID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	now := time.Now()
	tx.Status = TxStatusFailed
	tx.FinishedAt = &now

	return nil
}

// GetPending returns all transactions that are still pending (not committed or failed).
//
// DESIGN DECISION: Return snapshot of pending transactions, not live references
// WHY: Caller can safely iterate without holding lock, prevents deadlock
// TRADE-OFF: Snapshot may become stale if new transactions start, but acceptable
// INSPIRED BY: Database snapshot isolation, Go's copy-on-read pattern
//
// Returns:
//   - []*CachedTx: Slice of pending transactions (empty if none)
//   - error: Currently always returns nil (reserved for future use)
//
// Example:
//
//	// After crash, replay pending transactions
//	pending, _ := cache.GetPending()
//	for _, tx := range pending {
//	    cache.Replay(ctx, tx.TxID, executor)
//	}
func (tc *TxCache) GetPending() ([]*CachedTx, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	pending := make([]*CachedTx, 0)

	for _, tx := range tc.cache {
		if tx.Status == TxStatusPending {
			pending = append(pending, tx)
		}
	}

	return pending, nil
}

// Flush persists all in-memory cached transactions to storage.
//
// DESIGN DECISION: Upsert pattern (insert if new, update if exists)
// WHY: Idempotent - safe to call multiple times, handles both new and updated transactions
// TRADE-OFF: Extra Get() call overhead, but necessary for correctness
// INSPIRED BY: Database UPSERT (INSERT ... ON CONFLICT UPDATE), cache write-through
//
// Timing: Call after batch of transaction commits, or periodically (e.g., every 10 seconds).
// Don't call after every operation - batching reduces I/O overhead.
//
// Parameters:
//   - ctx: Cancellation context
//
// Returns:
//   - error: If any persistence operation fails
//
// Example:
//
//	// Batch processing
//	for _, op := range operations {
//	    cache.RecordOp(txID, op)
//	}
//	cache.Commit(txID)
//	cache.Flush(ctx) // Persist after batch
func (tc *TxCache) Flush(ctx context.Context) error {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	for _, tx := range tc.cache {
		doc := tc.txToDocument(tx)

		existing, err := tc.store.Get(ctx, TxCacheCollection, tx.TxID)
		if errors.Is(err, persistence.ErrNotFound) {
			_, err = tc.store.Insert(ctx, TxCacheCollection, doc)
			if err != nil {
				return fmt.Errorf("failed to insert transaction %s: %w", tx.TxID, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check existing transaction %s: %w", tx.TxID, err)
		} else if existing != nil {
			err = tc.store.Update(ctx, TxCacheCollection, tx.TxID, doc)
			if err != nil {
				return fmt.Errorf("failed to update transaction %s: %w", tx.TxID, err)
			}
		}
	}

	return nil
}

// Replay executes all operations in a transaction using the provided executor function.
//
// DESIGN DECISION: Callback-based replay instead of returning operations
// WHY: Executor can handle context cancellation, errors, and logging within callback
// TRADE-OFF: More complex API than just returning []CachedOp, but more flexible
// INSPIRED BY: Database redo log replay, event sourcing replay pattern
//
// Parameters:
//   - ctx: Cancellation context (available in executor, not passed directly)
//   - txID: Transaction identifier to replay
//   - executor: Callback function to execute each operation (must be idempotent)
//
// Returns:
//   - error: If transaction not found or any executor call fails
//
// Example:
//
//	// Crash recovery: replay pending transactions
//	executor := func(op CachedOp) error {
//	    switch op.OpType {
//	    case OpTypeInsert:
//	        _, err := store.Insert(ctx, op.Collection, op.Data)
//	        return err
//	    case OpTypeUpdate:
//	        return store.Update(ctx, op.Collection, op.ID, op.Data)
//	    case OpTypeDelete:
//	        return store.Delete(ctx, op.Collection, op.ID)
//	    }
//	    return nil
//	}
//	cache.Replay(ctx, "tx-001", executor)
func (tc *TxCache) Replay(ctx context.Context, txID string, executor func(op CachedOp) error) error {
	tc.mu.RLock()
	tx, exists := tc.cache[txID]
	tc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("transaction %s not found", txID)
	}

	for _, op := range tx.Ops {
		if err := executor(op); err != nil {
			return fmt.Errorf("failed to execute operation %s on %s: %w", op.OpType, op.ID, err)
		}
	}

	return nil
}

// Cleanup removes completed transactions older than the specified TTL.
//
// DESIGN DECISION: Delete from both memory cache and persistent storage
// WHY: Free memory and disk space for old transactions no longer needed for recovery
// TRADE-OFF: Lose audit trail after TTL expires, but necessary to prevent unbounded growth
// INSPIRED BY: Database log truncation, log rotation in syslog/rsyslog
//
// Cleanup criteria:
//   - Transaction must be finished (committed or failed, not pending)
//   - FinishedAt timestamp must be older than (now - ttl)
//   - Pending transactions are NEVER cleaned up (still needed for recovery)
//
// Parameters:
//   - ctx: Cancellation context
//   - ttl: Time-to-live for completed transactions (e.g., 24 * time.Hour)
//
// Returns:
//   - error: If storage query or deletion fails
//
// Example:
//
//	// Run cleanup daily
//	ticker := time.NewTicker(24 * time.Hour)
//	for range ticker.C {
//	    cache.Cleanup(ctx, cse.DefaultTxCacheTTL)
//	}
func (tc *TxCache) Cleanup(ctx context.Context, ttl time.Duration) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	toDelete := make(map[string]bool)

	docs, err := tc.store.Find(ctx, TxCacheCollection, persistence.Query{})
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return fmt.Errorf("failed to find transactions in storage: %w", err)
	}

	for _, doc := range docs {
		txID, _ := doc["id"].(string)
		finishedAtStr, _ := doc["finished_at"].(string)

		if finishedAtStr != "" {
			finishedAt, err := time.Parse(time.RFC3339, finishedAtStr)
			if err == nil && finishedAt.Before(cutoff) {
				toDelete[txID] = true
			}
		}
	}

	for txID := range toDelete {
		delete(tc.cache, txID)

		if err := tc.store.Delete(ctx, TxCacheCollection, txID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return fmt.Errorf("failed to delete transaction %s: %w", txID, err)
		}
	}

	return nil
}

// txToDocument converts a CachedTx to a persistence.Document for storage.
//
// DESIGN DECISION: Serialize operation data as JSON string, not nested objects
// WHY: Some backends (SQLite with JSON column) handle nested JSON better as strings
// TRADE-OFF: Extra JSON marshal/unmarshal overhead, but ensures portability
// INSPIRED BY: PostgreSQL JSONB storage (text-based), MongoDB nested documents
//
// Field mapping:
//   - id: Transaction ID (primary key)
//   - status: "pending"/"committed"/"failed"
//   - started_at: ISO-8601 timestamp string
//   - finished_at: ISO-8601 timestamp string (omitted if nil)
//   - metadata: Arbitrary key-value pairs
//   - ops: Array of operation objects (each with op_type, collection, id, timestamp, data)
//
// Note: This is an internal helper. Callers should use Flush() to persist transactions.
func (tc *TxCache) txToDocument(tx *CachedTx) persistence.Document {
	doc := persistence.Document{
		"id":         tx.TxID,
		"status":     string(tx.Status),
		"started_at": tx.StartedAt.Format(time.RFC3339),
		"metadata":   tx.Metadata,
	}

	if tx.FinishedAt != nil {
		doc["finished_at"] = tx.FinishedAt.Format(time.RFC3339)
	}

	opsData := make([]map[string]interface{}, len(tx.Ops))
	for i, op := range tx.Ops {
		opMap := map[string]interface{}{
			"op_type":    op.OpType,
			"collection": op.Collection,
			"id":         op.ID,
			"timestamp":  op.Timestamp.Format(time.RFC3339),
		}

		if op.Data != nil {
			dataBytes, _ := json.Marshal(op.Data)
			opMap["data"] = string(dataBytes)
		}

		opsData[i] = opMap
	}

	doc["ops"] = opsData

	return doc
}
