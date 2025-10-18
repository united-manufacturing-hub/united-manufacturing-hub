package cse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

const (
	TxCacheCollection = "_tx_cache"

	OpTypeInsert = "insert"
	OpTypeUpdate = "update"
	OpTypeDelete = "delete"

	DefaultTxCacheTTL = 24 * time.Hour
)

type TxStatus string

const (
	TxStatusPending   TxStatus = "pending"
	TxStatusCommitted TxStatus = "committed"
	TxStatusFailed    TxStatus = "failed"
)

type CachedOp struct {
	OpType     string
	Collection string
	ID         string
	Data       basic.Document
	Timestamp  time.Time
}

type CachedTx struct {
	TxID       string
	Status     TxStatus
	Ops        []CachedOp
	StartedAt  time.Time
	FinishedAt *time.Time
	Metadata   map[string]interface{}
}

type TxCache struct {
	store    basic.Store
	registry *Registry
	cache    map[string]*CachedTx
	mu       sync.RWMutex
}

func NewTxCache(store basic.Store, registry *Registry) *TxCache {
	return &TxCache{
		store:    store,
		registry: registry,
		cache:    make(map[string]*CachedTx),
	}
}

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

func (tc *TxCache) Flush(ctx context.Context) error {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	for _, tx := range tc.cache {
		doc := tc.txToDocument(tx)

		existing, err := tc.store.Get(ctx, TxCacheCollection, tx.TxID)
		if err == basic.ErrNotFound {
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

func (tc *TxCache) Cleanup(ctx context.Context, ttl time.Duration) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	cutoff := time.Now().Add(-ttl)
	toDelete := make(map[string]bool)

	docs, err := tc.store.Find(ctx, TxCacheCollection, basic.Query{})
	if err != nil && err != basic.ErrNotFound {
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

		if err := tc.store.Delete(ctx, TxCacheCollection, txID); err != nil && err != basic.ErrNotFound {
			return fmt.Errorf("failed to delete transaction %s: %w", txID, err)
		}
	}

	return nil
}

func (tc *TxCache) txToDocument(tx *CachedTx) basic.Document {
	doc := basic.Document{
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
