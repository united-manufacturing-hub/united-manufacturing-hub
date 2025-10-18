# CSE (Control Sync Engine) Package

Layer 2 of the FSM v2 persistence architecture - Common Sync Engine conventions for synchronizing state between distributed nodes.

## Quick Start

```go
// Initialize CSE components
registry := cse.NewRegistry()
registry.Register(&cse.CollectionMetadata{
    Name:       "workers",
    WorkerType: "container",
    Role:       cse.RoleIdentity,
})

triangular := cse.NewTriangularStore(store, registry)
syncState := cse.NewSyncState(store, registry)

// Record and sync changes
triangular.SaveObserved(ctx, "container", "worker-123", observedDoc)
syncState.RecordChange(cse.TierEdge, 12345)
delta := syncState.GetDeltaSince(cse.TierFrontend, 12300)
```

## Architecture WARNING ⚠️

**DEPLOYMENT:** Frontend ↔ Relay ↔ Edge (3 physical tiers)
**DATA FLOW:** Frontend ↔ Edge (2 logical tiers)

### Relay is a TRANSPARENT PROXY

The relay is **E2E encrypted** and **cannot see message contents**.

Think of relay like:
- nginx reverse proxy
- Cloudflare edge node
- Your ISP's routers

**Relay provides:**
- NAT traversal (customer sites behind firewalls)
- HTTPS termination / TLS offloading
- Authentication / routing
- Connection management

**Relay does NOT:**
- Read data (E2E encrypted)
- Transform data (blind to contents)
- Validate data (cannot see it)
- Merge changes (no sync logic)
- Maintain sync state (not a tier)

### DO NOT Re-Implement Relay State Tracking!

**If you're tempted to add:**
- ❌ `TierRelay` constant
- ❌ `relaySyncID` field
- ❌ `pendingRelay` queue
- ❌ Relay cases in sync methods

**STOP and ask:**
1. ✅ Would this work if relay is just nginx?
2. ✅ Can E2E encrypted relay actually see this data?
3. ✅ Did I read ARCHITECTURE.md?

**The relay is blind. Keep it that way.**

See `ARCHITECTURE.md` for full explanation of why we had 3-tier implementation and why it was wrong.

## Package Components

### Registry (registry.go)
Schema metadata and capability tracking for CSE-aware collections.

**Collection Registration:**
```go
registry := cse.NewRegistry()
registry.Register(&cse.CollectionMetadata{
    Name:       "workers",
    WorkerType: "container",
    Role:       cse.RoleIdentity,
})
metadata, err := registry.Get("workers")
```

**Schema Versioning:**
```go
// Register schema version for workerType+role
registry.RegisterVersion("container", "identity", "v2")

// Check version
version := registry.GetVersion("workers")  // Returns "v2"

// Get all versions
versions := registry.GetAllVersions()
// Returns: {"workers": "v2", "datapoints": "v1", ...}
```

**Feature Registry:**
```go
// Register feature support
registry.RegisterFeature("delta_sync", true)
registry.RegisterFeature("e2e_encryption", true)
registry.RegisterFeature("triangular_model", true)

// Check feature support
if registry.HasFeature("delta_sync") {
    // Enable delta sync functionality
}

// Get all features
features := registry.GetFeatures()
// Returns: {"delta_sync": true, "e2e_encryption": true, ...}
```

**Capability Negotiation Use Case:**

Schema versioning and feature registry enable frontend-backend capability negotiation:

1. **Frontend** has all schema versions compiled in (v1, v2, v3)
2. **umh-core** advertises supported schemas via `GetAllVersions()`
3. **Frontend** selects appropriate schema version at runtime
4. **Frontend** enables/disables features based on `GetFeatures()`

This allows:
- One frontend version to work with multiple umh-core versions
- Graceful degradation when backend lacks features
- Per-feature capability detection (not just version number)

### Triangular Store (triangular.go)
High-level operations for FSM v2's triangular model (Identity/Desired/Observed).

```go
triangular := cse.NewTriangularStore(store, registry)
triangular.SaveIdentity(ctx, "container", "worker-123", identityDoc)
triangular.SaveDesired(ctx, "container", "worker-123", desiredDoc)
triangular.SaveObserved(ctx, "container", "worker-123", observedDoc)
```

Auto-injects CSE metadata:
- `_sync_id` (global monotonic counter for delta sync)
- `_version` (optimistic locking)
- `_created_at`, `_updated_at` (timestamps)

### Transaction Cache (tx_cache.go)
SAGA-aware transaction caching for crash resilience.

```go
cache := cse.NewTxCache(store)
tx := cache.BeginTx(ctx)
cache.RecordOp(txID, opType, collection, doc)
cache.Commit(txID)
pending := cache.GetPending()
cache.Replay(ctx, pending)
```

### Sync State (sync_state.go)
**2-tier** sync state tracking between Frontend and Edge.

```go
syncState := cse.NewSyncState(store, registry)
syncState.RecordChange(cse.TierEdge, 12345)      // Edge created change
syncState.MarkSynced(cse.TierEdge, 12345)        // Frontend received it
delta := syncState.GetDeltaSince(cse.TierFrontend, 12300)
```

**Tiers:**
- `TierEdge`: umh-core at customer site (creates changes)
- `TierFrontend`: Browser with SQLite (receives changes)

**No `TierRelay`** - relay is transparent proxy, not a sync participant.

### Object Pool (pool.go)
Singleton management with reference counting.

```go
pool := cse.NewObjectPool()
worker := pool.GetOrCreate("worker-123", factory)
pool.Acquire("worker-123")
pool.Release("worker-123")
```

## Design Principles

### Inspired by Linear's Sync Engine

CSE follows patterns from Linear's proven sync architecture:
- Local-first (offline capable)
- Delta sync (only changes since last sync)
- Subscription-based (client declares what to sync)
- Optimistic concurrency (`_version` for conflict detection)
- Global monotonic counter (`_sync_id` for ordering)

**Key difference from Linear:**
- Linear: 2-tier (Client ↔ Server)
- UMH: 2-tier logical (Frontend ↔ Edge), 3-tier physical (+ transparent relay for NAT)

See: https://github.com/wzhudev/reverse-linear-sync-engine

### Triangular Model (FSM v2)

FSM v2 separates state into three categories:
- **Identity**: Immutable properties (UUID, name)
- **Desired**: User intent (configuration, target state)
- **Observed**: System reality (actual state, metrics)

This enables:
- Compare desired vs observed to drive reconciliation
- Immutable identity prevents accidental mutations
- Clear separation of "what we want" vs "what is"

### CSE Metadata Conventions

All documents include:
- `_sync_id`: Global counter (for delta queries: `WHERE _sync_id > lastSyncID`)
- `_version`: Optimistic locking (detect concurrent modifications)
- `_created_at`: Creation timestamp
- `_updated_at`: Last modification timestamp
- `_synced_at`: Last sync timestamp (optional)
- `_sync_status`: Sync state (pending/synced/conflict)

## Testing

All components use Ginkgo v2 + Gomega:

```bash
ginkgo -r ./umh-core/pkg/persistence/cse
```

Current test coverage: 172 specs (includes schema versioning and feature registry)

## References

- **ENG-3622**: CSE RFC (Linear-quality UX for Kubernetes)
- **ENG-3647**: FSM v2 RFC
- **ARCHITECTURE.md**: Why relay is transparent (prevent future confusion)
- **Linear sync engine**: wzhudev/reverse-linear-sync-engine
