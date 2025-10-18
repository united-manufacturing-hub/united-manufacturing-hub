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

// NewSyncState creates a new SyncState tracker with all sync IDs initialized to 0.
//
// DESIGN DECISION: All sync IDs start at 0, not -1 or 1
// WHY: 0 means "no changes synced yet", which is semantically clearer than -1.
// First sync will query "WHERE _sync_id > 0" to get all changes.
// TRADE-OFF: Can't distinguish "never synced" from "synced up to ID 0", but ID 0 is never used.
// INSPIRED BY: Git commit hashes (initial commit has no parent), database AUTO_INCREMENT starts at 1.
//
// Usage:
//
//	syncState := cse.NewSyncState(store, registry)
//	syncState.Load(ctx) // Restore state after restart
//	defer syncState.Flush(ctx) // Persist state on shutdown
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

// RecordChange tracks a new change for synchronization to the next tier.
//
// DESIGN DECISION: Track pending changes, not just "last synced" ID
// WHY: Need to retry failed syncs and track partial sync progress. If sync fails,
// we know which specific changes need to be retried.
// TRADE-OFF: Memory/storage overhead for pending list vs simplicity of single ID.
// INSPIRED BY: Git unpushed commits (git knows exactly what to push), email outbox.
//
// Example:
//
//	// Edge created changes 100, 101, 102
//	syncState.RecordChange(100, cse.TierEdge)
//	syncState.RecordChange(101, cse.TierEdge)
//	syncState.RecordChange(102, cse.TierEdge)
//
//	// Sync to relay succeeds for 100-101, fails for 102
//	syncState.MarkSynced(cse.TierEdge, 101) // Removes 100, 101 from pending
//	// 102 remains in pendingEdge for retry
//
// Note: Only TierEdge and TierRelay track pending changes. TierFrontend is the final
// destination (no further sync), so it doesn't need pending tracking.
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

// MarkSynced marks changes up to syncID as successfully synced to the next tier.
//
// DESIGN DECISION: MarkSynced updates the NEXT tier's sync ID, not the current tier's
// WHY: Matches Linear's pattern. When edge syncs to relay, relay's sync ID updates.
// "relaySyncID" means "what relay has received from edge", not "what relay has sent".
// TRADE-OFF: Slightly confusing naming, but matches Linear's architecture.
// INSPIRED BY: Linear's lastSyncId in server (tracks what was received from client).
//
// Tier progression:
//   - MarkSynced(TierEdge, 100) → updates relaySyncID=100 (relay received up to 100)
//   - MarkSynced(TierRelay, 100) → updates frontendSyncID=100 (frontend received up to 100)
//
// All pending changes ≤ syncID are removed from the pending list.
//
// Example:
//
//	syncState.RecordChange(100, cse.TierEdge)
//	syncState.RecordChange(101, cse.TierEdge)
//	syncState.RecordChange(102, cse.TierEdge)
//
//	// Sync successfully sent 100-101 to relay
//	syncState.MarkSynced(cse.TierEdge, 101)
//	// Result: relaySyncID=101, pendingEdge=[102]
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

// GetDeltaSince constructs a query to fetch changes since the last sync for a tier.
//
// DESIGN DECISION: Delta sync using "WHERE _sync_id > lastSyncID"
// WHY: Efficient sync (only changes since last sync), not full data transfer every time.
// Requires monotonic sync IDs (enforced by TriangularStore).
// TRADE-OFF: Can't sync if sync ID sequence has gaps, but TriangularStore guarantees monotonic IDs.
// INSPIRED BY: Linear's delta sync, rsync (only transfer diffs), Git fetch (only new commits).
//
// Tier logic:
//   - TierEdge: Query changes > relaySyncID (what relay hasn't received yet)
//   - TierRelay: Query changes > edgeSyncID (what relay needs to send to frontend)
//   - TierFrontend: Query changes > relaySyncID (what frontend hasn't received yet)
//
// Example:
//
//	// Edge has changes up to 12345, relay has received up to 12340
//	query, _ := syncState.GetDeltaSince(cse.TierEdge)
//	// Returns: Query{Filters: [{Field: "_sync_id", Op: "$gt", Value: 12340}]}
//	// When executed: Returns changes 12341-12345 (5 new changes)
//
// Usage with store:
//
//	query, _ := syncState.GetDeltaSince(cse.TierEdge)
//	changes, _ := store.Find(ctx, "container_desired", *query)
//	// Send changes to relay via HTTP
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

// Flush persists sync state to storage for recovery after restart.
//
// DESIGN DECISION: Singleton document with ID "sync_state" in special collection "_sync_state"
// WHY: Only one sync state per edge instance. Singleton pattern prevents duplicates.
// TRADE-OFF: Can't track multiple edge instances from one storage, but each edge runs independently.
// INSPIRED BY: SQLite PRAGMA settings (singleton config), Git HEAD file (single pointer).
//
// Storage schema:
//
//	{
//	  "id": "sync_state",
//	  "edge_sync_id": 12345,
//	  "relay_sync_id": 12340,
//	  "frontend_sync_id": 12335,
//	  "pending_edge": [12341, 12342, 12343, 12344, 12345],
//	  "pending_relay": [12336, 12337, 12338, 12339, 12340],
//	  "updated_at": "2025-01-15T10:30:00.123456789Z"
//	}
//
// Flush timing: Call after batch of changes (not every change) to reduce I/O overhead.
// Critical errors: Flush/Load errors indicate sync state corruption - handle with care.
//
// Usage:
//
//	// After processing batch of changes
//	syncState.Flush(ctx)
//
//	// On graceful shutdown
//	defer syncState.Flush(ctx)
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

// Load restores sync state from storage after restart.
//
// DESIGN DECISION: Silent success if no state exists (returns nil, not error)
// WHY: First startup has no persisted state yet. This is normal, not an error condition.
// TRADE-OFF: Can't distinguish "fresh start" from "storage corruption", but fresh start is common.
// INSPIRED BY: Git clone (no .git/config initially), browser first run (no saved state).
//
// Type handling: Handles both int64 and float64 from JSON unmarshaling.
// JSON stores numbers as float64, but we need int64 for sync IDs.
//
// Usage:
//
//	syncState := cse.NewSyncState(store, registry)
//	if err := syncState.Load(ctx); err != nil {
//	    // Critical error: storage corruption
//	    log.Fatalf("failed to load sync state: %v", err)
//	}
//	// syncState now has restored state or defaults to 0
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
