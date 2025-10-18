package cse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// Tier represents a CSE synchronization tier in the three-tier architecture.
//
// DESIGN DECISION: Named tiers instead of numeric levels (0/1/2)
// WHY: Self-documenting code. "TierEdge" is clearer than "0" or "TIER_LEVEL_0".
// TRADE-OFF: More verbose but eliminates confusion about tier ordering.
// INSPIRED BY: Network OSI layers use names (Physical/Data Link/Network) not just numbers.
//
// CSE Architecture:
//
//	Frontend (Web UI)
//	    ↕ (delta sync with relaySyncID)
//	Relay (Cloud)
//	    ↕ (delta sync with edgeSyncID)
//	Edge (Customer Site)
//
// Each tier tracks:
//   - Local sync ID (last change applied locally)
//   - Remote sync ID (last change synced to next tier)
//   - Pending changes (not yet synced to next tier)
type Tier string

const (
	TierEdge     Tier = "edge"
	TierRelay    Tier = "relay"
	TierFrontend Tier = "frontend"
)

// SyncState tracks synchronization state across CSE's three-tier architecture.
//
// DESIGN DECISION: Three separate sync IDs (edge/relay/frontend) instead of single global ID
// WHY: Each tier may be at different sync states due to network latency, offline periods,
// or sync failures. Edge may be at sync ID 12345, relay at 12340, frontend at 12335.
// TRADE-OFF: More complex tracking, but accurately reflects CSE's distributed reality.
// INSPIRED BY: CRDTs with vector clocks, Linear's client/server sync IDs
//
// CSE EXTENSION: Linear has 2 tiers (client ↔ server), CSE has 3 (edge ↔ relay ↔ frontend).
// Linear uses lastSyncId in both client and server. CSE extends this to three separate IDs.
//
// Example sync progression:
//
//	Edge creates change → syncID=12345, edgeSyncID=12345, relaySyncID=12340
//	Edge syncs to relay → relay receives 12345, relaySyncID=12345
//	Relay syncs to frontend → frontend receives 12345, frontendSyncID=12345
//
// Thread-safety: All methods use RWMutex for concurrent access.
// Persistence: Call Flush() to persist state, Load() to restore after restart.
type SyncState struct {
	store    basic.Store
	registry *Registry

	edgeSyncID     int64
	relaySyncID    int64
	frontendSyncID int64

	pendingEdge  []int64
	pendingRelay []int64

	mu sync.RWMutex
}

func NewSyncState(store basic.Store, registry *Registry) *SyncState {
	return &SyncState{
		store:        store,
		registry:     registry,
		pendingEdge:  make([]int64, 0),
		pendingRelay: make([]int64, 0),
	}
}

func (ss *SyncState) GetEdgeSyncID() int64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.edgeSyncID
}

func (ss *SyncState) SetEdgeSyncID(syncID int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.edgeSyncID = syncID
	return nil
}

func (ss *SyncState) GetRelaySyncID() int64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.relaySyncID
}

func (ss *SyncState) SetRelaySyncID(syncID int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.relaySyncID = syncID
	return nil
}

func (ss *SyncState) GetFrontendSyncID() int64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.frontendSyncID
}

func (ss *SyncState) SetFrontendSyncID(syncID int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.frontendSyncID = syncID
	return nil
}

func (ss *SyncState) RecordChange(syncID int64, tier Tier) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	switch tier {
	case TierEdge:
		ss.pendingEdge = append(ss.pendingEdge, syncID)
	case TierRelay:
		ss.pendingRelay = append(ss.pendingRelay, syncID)
	default:
		return fmt.Errorf("unsupported tier: %s", tier)
	}

	return nil
}

func (ss *SyncState) MarkSynced(tier Tier, syncID int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	switch tier {
	case TierEdge:
		ss.pendingEdge = filterSyncIDs(ss.pendingEdge, syncID)
		ss.relaySyncID = syncID
	case TierRelay:
		ss.pendingRelay = filterSyncIDs(ss.pendingRelay, syncID)
		ss.frontendSyncID = syncID
	default:
		return fmt.Errorf("unsupported tier: %s", tier)
	}

	return nil
}

func filterSyncIDs(syncIDs []int64, threshold int64) []int64 {
	result := make([]int64, 0)
	for _, id := range syncIDs {
		if id > threshold {
			result = append(result, id)
		}
	}
	return result
}

func (ss *SyncState) GetPendingChanges(tier Tier) ([]int64, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	switch tier {
	case TierEdge:
		result := make([]int64, len(ss.pendingEdge))
		copy(result, ss.pendingEdge)
		return result, nil
	case TierRelay:
		result := make([]int64, len(ss.pendingRelay))
		copy(result, ss.pendingRelay)
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported tier: %s", tier)
	}
}

func (ss *SyncState) GetDeltaSince(tier Tier) (*basic.Query, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var lastSyncID int64

	switch tier {
	case TierEdge:
		lastSyncID = ss.relaySyncID
	case TierRelay:
		lastSyncID = ss.edgeSyncID
	case TierFrontend:
		lastSyncID = ss.relaySyncID
	default:
		return nil, fmt.Errorf("unsupported tier: %s", tier)
	}

	query := &basic.Query{
		Filters: []basic.FilterCondition{
			{
				Field: FieldSyncID,
				Op:    basic.Gt,
				Value: lastSyncID,
			},
		},
	}

	return query, nil
}

func (ss *SyncState) Flush(ctx context.Context) error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	doc := basic.Document{
		"id":                "sync_state",
		"edge_sync_id":      ss.edgeSyncID,
		"relay_sync_id":     ss.relaySyncID,
		"frontend_sync_id":  ss.frontendSyncID,
		"pending_edge":      ss.pendingEdge,
		"pending_relay":     ss.pendingRelay,
		"updated_at":        time.Now().Format(time.RFC3339Nano),
	}

	existing, err := ss.store.Get(ctx, "_sync_state", "sync_state")
	if err == basic.ErrNotFound {
		_, err = ss.store.Insert(ctx, "_sync_state", doc)
		return err
	}

	if err != nil {
		return fmt.Errorf("failed to check existing sync state: %w", err)
	}

	if existing != nil {
		return ss.store.Update(ctx, "_sync_state", "sync_state", doc)
	}

	return nil
}

func (ss *SyncState) Load(ctx context.Context) error {
	doc, err := ss.store.Get(ctx, "_sync_state", "sync_state")
	if err == basic.ErrNotFound {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if edgeSyncID, ok := doc["edge_sync_id"].(int64); ok {
		ss.edgeSyncID = edgeSyncID
	} else if edgeSyncID, ok := doc["edge_sync_id"].(float64); ok {
		ss.edgeSyncID = int64(edgeSyncID)
	}

	if relaySyncID, ok := doc["relay_sync_id"].(int64); ok {
		ss.relaySyncID = relaySyncID
	} else if relaySyncID, ok := doc["relay_sync_id"].(float64); ok {
		ss.relaySyncID = int64(relaySyncID)
	}

	if frontendSyncID, ok := doc["frontend_sync_id"].(int64); ok {
		ss.frontendSyncID = frontendSyncID
	} else if frontendSyncID, ok := doc["frontend_sync_id"].(float64); ok {
		ss.frontendSyncID = int64(frontendSyncID)
	}

	if pendingEdge, ok := doc["pending_edge"].([]interface{}); ok {
		ss.pendingEdge = convertToInt64Slice(pendingEdge)
	} else if pendingEdge, ok := doc["pending_edge"].([]int64); ok {
		ss.pendingEdge = pendingEdge
	}

	if pendingRelay, ok := doc["pending_relay"].([]interface{}); ok {
		ss.pendingRelay = convertToInt64Slice(pendingRelay)
	} else if pendingRelay, ok := doc["pending_relay"].([]int64); ok {
		ss.pendingRelay = pendingRelay
	}

	return nil
}

func convertToInt64Slice(input []interface{}) []int64 {
	result := make([]int64, 0, len(input))
	for _, v := range input {
		switch val := v.(type) {
		case int64:
			result = append(result, val)
		case float64:
			result = append(result, int64(val))
		case json.Number:
			if num, err := val.Int64(); err == nil {
				result = append(result, num)
			}
		}
	}
	return result
}
