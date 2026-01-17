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

// Package storage provides TriangularStore - the Go implementation of CSE's three-state model.
//
// # Overview
//
// TriangularStore implements the Control Sync Engine (CSE) storage pattern for
// managing closed-loop systems. See Linear ENG-3622 for the full CSE concept.
//
// The three-state model separates each worker into:
//   - Identity: Immutable worker identification (ID, name, type)
//   - Desired: User intent/configuration (the intended state)
//   - Observed: System reality (the actual state)
//
// The FSMv2 supervisor uses this package for all state persistence.
// See pkg/fsmv2/supervisor/supervisor.go for usage patterns.
//
// # Go Structs with Reflection
//
// CSE uses Go structs with reflection-based type derivation. Collection names
// are derived from struct type names using a naming convention:
//
//	ContainerObservedState → "container_observed"
//	RelayDesiredState      → "relay_desired"
//	ParentIdentity         → "parent_identity"
//
// This convention-over-configuration approach eliminates the need for explicit
// type registration. The type name suffix (DesiredState, ObservedState) determines
// the role, and the prefix determines the worker type.
//
// # Why Three Separate Collections?
//
// Each worker's state is stored in three separate collections because identity,
// desired, and observed have fundamentally different lifecycles:
//
// Identity: Created once at worker creation, never updated. Immutable fields
// like worker ID, name, and worker type that identify "what is this worker?"
//
// Desired: Updated by users/configuration changes. Represents intent - what
// the system should be doing. Participates in optimistic locking with versions.
//
// Observed: Updated frequently by polling (every 500ms by default). Represents
// reality - what the system is doing. Ephemeral and reconstructed from external
// system queries.
//
// Separating them prevents coupling their update patterns. A configuration change
// (desired) shouldn't require knowing current system state (observed), and polling
// shouldn't affect identity or configuration.
//
// # Why Delta Checking?
//
// Both SaveDesired and SaveObserved include built-in delta checking that skips
// writes when data hasn't changed:
//
//	changed, err := storage.SaveDesiredTyped[ContainerDesiredState](ts, ctx, id, desired)
//	changed, err := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, observed)
//	// changed=false means write was skipped (data unchanged)
//
// Why? Both desired and observed state can be written frequently. For observed,
// polling happens every 500ms in FSMv2. For desired, supervisor ticks may re-derive
// the same desired state on every tick. Without delta checking, we'd write
// identical data repeatedly, incrementing sync IDs unnecessarily and generating
// noise in change streams.
//
// Delta checking compares business data (excludes CSE metadata fields like
// _sync_id, _version, timestamps). If only metadata would change, the write
// is skipped entirely.
//
// This enables efficient delta streaming to clients: only actual changes
// increment sync_id, so clients requesting "changes since sync_id X" receive
// only meaningful updates.
//
// # Why Version Management Differs?
//
// Desired and Observed handle versions differently:
//
// Desired: _version increments on every update.
// Why? Desired represents user intent and participates in optimistic locking.
// When two clients modify configuration concurrently, version conflicts reveal
// the race condition. The pattern is: read version, modify, write with expected
// version, fail if version changed.
//
// Observed: _version is preserved (doesn't increment).
// Why? Observed state is ephemeral. It's reconstructed by polling external
// systems, not by user modifications. There's no "concurrent modification" to
// detect - the latest poll result is always authoritative. Version conflicts
// don't make sense for ephemeral data.
//
// # Why Two API Patterns?
//
// The storage API provides two patterns for different use cases:
//
// 1. Runtime Polymorphic API (interface methods):
//
//	ts.SaveObserved(ctx, "container", id, observed)
//
// Use when worker type is determined at runtime. Supervisors managing multiple
// worker types use this because they iterate over heterogeneous workers and
// can't know types at compile time.
//
// 2. Compile-Time Typed API (generic functions):
//
//	storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, observed)
//
// Use when type is known at compile time. Collectors and workers use this for
// type safety, IDE autocomplete, and compile-time error checking.
//
// Both patterns use the same underlying convention-based naming ({workerType}_{role})
// and the same storage backend. The interface methods are not deprecated - they
// serve the runtime polymorphic use case where generics can't help.
//
// # Why Convention-Based Naming?
//
// Collection names follow a strict convention: {workerType}_{role}
//
//	container_identity    container_desired    container_observed
//	relay_identity        relay_desired        relay_observed
//
// Why? Convention-based naming eliminates the need for explicit type registration.
// The generic functions derive workerType from the struct name using reflection:
//
//	DeriveWorkerType[ContainerObservedState]() → "container"
//
// This "convention over configuration" approach reduces boilerplate. You define
// a struct with the right naming convention, and storage operations just work.
// No registry, no initialization, no mapping files.
//
// # Why Atomic LoadSnapshot?
//
// LoadSnapshot uses a database transaction to atomically load all three parts:
//
//	snapshot, err := ts.LoadSnapshot(ctx, "container", "worker-123")
//	// snapshot.Identity, snapshot.Desired, snapshot.Observed are consistent
//
// Why? FSM state machines need a consistent view of worker state to make
// correct decisions. Without atomic loading, you might see desired="stop"
// with observed="running" (from before the desired change), leading to
// incorrect state transitions.
//
// Transactions ensure snapshot isolation: all three parts reflect the same
// point in time, even if another goroutine is updating observed state.
//
// # CSE Metadata Fields
//
// TriangularStore auto-injects CSE metadata fields transparently:
//
//	_sync_id:    Global sync version (for delta sync queries)
//	_version:    Document version (for optimistic locking)
//	_created_at: Creation timestamp
//	_updated_at: Last update timestamp
//
// Callers never manage these fields manually. The storage layer handles
// incrementing sync IDs after successful writes, managing versions per role,
// and setting timestamps appropriately.
//
// # Future: Delta Streaming via sync_id
//
// The _sync_id field enables efficient client synchronization (planned feature):
//
//	// Client requests: "give me all changes since my last sync"
//	SELECT * FROM all_collections WHERE _sync_id > 12345 ORDER BY _sync_id
//
// This allows clients to:
//   - Maintain local caches synchronized with server state
//   - Request only incremental changes instead of full snapshots
//   - Track exactly which updates they've processed
//
// The infrastructure (sync_id auto-increment, delta checking) is in place.
// The query API for streaming changes to clients is planned for future phases.
//
// # Future: UserSpec and SAGA Patterns
//
// The triangular model separates user intent from system state:
//
//	UserSpec       → Raw user configuration (YAML/UI input)
//	    ↓
//	DeriveDesiredState()
//	    ↓
//	DesiredState   → Computed runtime target
//	    ↓ (reconciliation)
//	ObservedState  → Actual system state
//
// Planned enhancements:
//   - UserSpec versioning for audit trails (who changed what, when)
//   - SAGA pattern: users modify UserSpec only, DesiredState flows automatically
//   - Conflict resolution via resource IDs (not last-write-wins)
//   - Audit logs tracking user requests and their effects
//
// See PR #2235 (FSM v2 Phase 1) and Linear ENG-3622 (CSE RFC) for context.
//
// # Usage Example
//
//	// Create store
//	ts := storage.NewTriangularStore(persistenceStore, logger)
//
//	// Save identity once (immutable)
//	ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "name": "Container A",
//	})
//
//	// Save desired state (versioned, optimistic locking)
//	storage.SaveDesiredTyped[ContainerDesiredState](ts, ctx, "worker-123", desired)
//
//	// Save observed state (delta-checked, skips if unchanged)
//	changed, _ := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, "worker-123", observed)
//
//	// Load complete snapshot for FSM decision
//	snapshot, _ := ts.LoadSnapshot(ctx, "container", "worker-123")
package storage
