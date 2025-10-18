package cse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// Tier represents a CSE synchronization tier in the two-tier architecture.
//
// DESIGN DECISION: Named tiers instead of numeric levels (0/1)
// WHY: Self-documenting code. "TierEdge" is clearer than "0" or "TIER_LEVEL_0".
// TRADE-OFF: More verbose but eliminates confusion about tier ordering.
// INSPIRED BY: Network OSI layers use names (Physical/Data Link/Network) not just numbers.
//
// CSE Architecture:
//
//	Frontend (Web UI)
//	    ↕ (delta sync, relay is transparent E2E encrypted proxy)
//	Edge (Customer Site)
//
// Each tier tracks:
//   - Local sync ID (last change created/received)
//   - Pending changes (not yet synced to other tier)
type Tier string

const (
	TierEdge     Tier = "edge"
	TierFrontend Tier = "frontend"
)

// SyncState tracks synchronization state across CSE's two-tier architecture.
//
// DESIGN DECISION: Two separate sync IDs (edge/frontend) instead of single global ID
// WHY: Each tier may be at different sync states due to network latency, offline periods,
// or sync failures. Edge may be at sync ID 12345, frontend at 12335.
// TRADE-OFF: More complex tracking, but accurately reflects CSE's distributed reality.
// INSPIRED BY: CRDTs with vector clocks, Linear's client/server sync IDs
//
// CSE MATCHES Linear: Linear has 2 tiers (client ↔ server), CSE has 2 (edge ↔ frontend).
// Linear uses lastSyncId in both client and server. CSE uses edgeSyncID and frontendSyncID.
//
// Example sync progression:
//
//	Edge creates change → syncID=12345, edgeSyncID=12345
//	Edge syncs to frontend → frontend receives 12345, frontendSyncID=12345
//
// Thread-safety: All methods use RWMutex for concurrent access.
// Persistence: Call Flush() to persist state, Load() to restore after restart.
type SyncState struct {
	store    basic.Store
	registry *Registry

	edgeSyncID     int64
	frontendSyncID int64

	pendingEdge []int64

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
		store:       store,
		registry:    registry,
		pendingEdge: make([]int64, 0),
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

// RecordChange tracks a new change for synchronization to the other tier.
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
//	// Sync to frontend succeeds for 100-101, fails for 102
//	syncState.MarkSynced(cse.TierEdge, 101) // Removes 100, 101 from pending
//	// 102 remains in pendingEdge for retry
//
// Note: Only TierEdge tracks pending changes. TierFrontend is the receiving
// tier, so it doesn't need pending tracking.
func (ss *SyncState) RecordChange(syncID int64, tier Tier) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	switch tier {
	case TierEdge:
		ss.pendingEdge = append(ss.pendingEdge, syncID)
	default:
		return fmt.Errorf("unsupported tier: %s", tier)
	}

	return nil
}

// MarkSynced marks changes up to syncID as successfully synced to the other tier.
//
// DESIGN DECISION: MarkSynced updates the OTHER tier's sync ID, not the current tier's
// WHY: Matches Linear's pattern. When edge syncs to frontend, frontend's sync ID updates.
// "frontendSyncID" means "what frontend has received from edge".
// TRADE-OFF: Slightly confusing naming, but matches Linear's architecture.
// INSPIRED BY: Linear's lastSyncId in server (tracks what was received from client).
//
// Tier progression:
//   - MarkSynced(TierEdge, 100) → updates frontendSyncID=100 (frontend received up to 100)
//
// All pending changes ≤ syncID are removed from the pending list.
//
// Example:
//
//	syncState.RecordChange(100, cse.TierEdge)
//	syncState.RecordChange(101, cse.TierEdge)
//	syncState.RecordChange(102, cse.TierEdge)
//
//	// Sync successfully sent 100-101 to frontend
//	syncState.MarkSynced(cse.TierEdge, 101)
//	// Result: frontendSyncID=101, pendingEdge=[102]
func (ss *SyncState) MarkSynced(tier Tier, syncID int64) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	switch tier {
	case TierEdge:
		ss.pendingEdge = filterSyncIDs(ss.pendingEdge, syncID)
		ss.frontendSyncID = syncID
	default:
		return fmt.Errorf("unsupported tier: %s", tier)
	}

	return nil
}

// filterSyncIDs removes sync IDs that are less than or equal to the threshold.
//
// DESIGN DECISION: Filter in-place with new slice, not modify original
// WHY: Preserve original slice for caller, avoid side effects
// TRADE-OFF: Allocates new slice, but pendingEdge is typically small
// INSPIRED BY: Go slice filtering patterns, functional programming filter()
//
// Parameters:
//   - syncIDs: List of pending sync IDs
//   - threshold: Maximum sync ID that has been successfully synced
//
// Returns:
//   - []int64: New slice containing only sync IDs > threshold
//
// Example:
//
//	pending := []int64{100, 101, 102, 103}
//	remaining := filterSyncIDs(pending, 101)
//	// Result: [102, 103] (100 and 101 were successfully synced)
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
//   - TierEdge: Query changes > frontendSyncID (what frontend hasn't received yet)
//   - TierFrontend: Query changes > edgeSyncID (what frontend needs to request from edge)
//
// Example:
//
//	// Edge has changes up to 12345, frontend has received up to 12340
//	query, _ := syncState.GetDeltaSince(cse.TierEdge)
//	// Returns: Query{Filters: [{Field: "_sync_id", Op: "$gt", Value: 12340}]}
//	// When executed: Returns changes 12341-12345 (5 new changes)
//
// Usage with store:
//
//	query, _ := syncState.GetDeltaSince(cse.TierEdge)
//	changes, _ := store.Find(ctx, "container_desired", *query)
//	// Send changes to frontend via HTTP
func (ss *SyncState) GetDeltaSince(tier Tier) (*basic.Query, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var lastSyncID int64

	switch tier {
	case TierEdge:
		lastSyncID = ss.frontendSyncID
	case TierFrontend:
		lastSyncID = ss.edgeSyncID
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
//	  "frontend_sync_id": 12335,
//	  "pending_edge": [12341, 12342, 12343, 12344, 12345],
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
		"id":               "sync_state",
		"edge_sync_id":     ss.edgeSyncID,
		"frontend_sync_id": ss.frontendSyncID,
		"pending_edge":     ss.pendingEdge,
		"updated_at":       time.Now().Format(time.RFC3339Nano),
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

	return nil
}

// convertToInt64Slice converts a slice of interface{} values to []int64.
//
// DESIGN DECISION: Handle multiple numeric types (int64, float64, json.Number)
// WHY: JSON unmarshaling produces different types depending on parser configuration
// TRADE-OFF: Complex type switching, but necessary for robust JSON deserialization
// INSPIRED BY: Type coercion in dynamic languages, JSON number handling in Go
//
// Parameters:
//   - input: Slice of interface{} values (typically from JSON unmarshaling)
//
// Returns:
//   - []int64: Converted slice (skips values that can't be converted)
//
// Note: Silently skips non-numeric values instead of erroring.
// This is defensive - better to have partial data than fail completely.
//
// Example:
//
//	// JSON: {"pending_edge": [100.0, 101.0, 102.0]}
//	// Unmarshal produces: []interface{}{float64(100), float64(101), float64(102)}
//	pending := convertToInt64Slice(jsonSlice)
//	// Result: []int64{100, 101, 102}
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
