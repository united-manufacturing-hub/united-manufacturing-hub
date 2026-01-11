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

package storage

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// TriangularStoreInterface provides high-level operations for FSM v2's triangular model.
// It manages the three-part separation of worker state: Identity (immutable),
// Desired (user intent), and Observed (system reality).
//
// Implementations should automatically inject CSE metadata (_sync_id, _version,
// timestamps) transparently to reduce boilerplate and prevent mistakes.
//
// The interface enables testing with mock implementations and allows for
// different storage strategies (batched writes, caching, etc.) without
// changing caller code.
//
// TWO API PATTERNS FOR STORAGE OPERATIONS:
//
// The CSE storage API provides two patterns for accessing worker state, optimized for
// different use cases:
//
// 1. RUNTIME POLYMORPHIC API (for supervisors, factories):
//    Interface methods with workerType string parameter. Use when worker type is
//    determined at runtime and you need polymorphic behavior across multiple worker types.
//
//    Example (supervisor managing multiple worker types):
//      func (s *Supervisor) saveState(ctx context.Context, workerType, id string, observed interface{}) {
//          s.store.SaveObserved(ctx, workerType, id, observed)
//      }
//
// 2. COMPILE-TIME TYPED API (for workers, tests):
//    Package-level generic functions with type parameters. Use when type is known at
//    compile time for type safety, better performance, and IDE autocomplete.
//
//    Example (worker with known type):
//      func (w *ContainerWorker) saveState(ctx context.Context) {
//          SaveObservedTyped[ContainerObservedState](w.store, ctx, w.id, w.observed)
//      }
//
// Both patterns use the same underlying convention-based naming: {workerType}_{role}
// (e.g., "container_observed", "relay_desired").
//
// WORKER TYPE NAMING:
//
// The workerType string follows convention-based naming for storage collections:
//   - "container" → container_identity, container_desired, container_observed
//   - "relay" → relay_identity, relay_desired, relay_observed
//   - "benthos" → benthos_identity, benthos_desired, benthos_observed
//
// Use lowercase, singular nouns. Avoid special characters.
//
// MIGRATION STATUS: The API migration is complete. Both patterns are fully supported and
// coexist intentionally. The interface methods are NOT deprecated - they serve the runtime
// polymorphic use case. Use the typed functions for compile-time type safety when the
// worker type is known.
type TriangularStoreInterface interface {
	// SaveIdentity stores immutable worker identity (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveIdentityTyped[T]() instead.
	//
	// Identity is created once and never updated.
	//
	// Unlike SaveDesired/SaveObserved, this returns only `error` (not `changed`) because:
	//   - Identity is write-once: Creating the same identity twice is idempotent
	//   - No delta tracking: Identity changes don't generate sync events
	//   - Idempotency: If identity already exists with same content, succeeds silently
	//
	// Returns error if identity exists with DIFFERENT content (immutability violation).
	SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error

	// LoadIdentity retrieves worker identity (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadIdentityTyped[T]() instead.
	//
	// Returns persistence.ErrNotFound if identity doesn't exist.
	LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error)

	// SaveDesired stores user intent/configuration with delta checking (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveDesiredTyped[T]() instead.
	//
	// Includes built-in delta checking that skips writes when data hasn't changed.
	// Auto-increments _version and _sync_id only when data actually changes.
	//
	// Returns (changed bool, error) where:
	//   - changed=true: Data was different from existing, delta was recorded, sync_id incremented
	//   - changed=false: Data identical to existing, no write occurred, no delta created
	//
	// Use cases for checking `changed`:
	//   - Avoid redundant downstream notifications when nothing changed
	//   - Track actual modification frequency for monitoring
	//   - Optimize sync by skipping unchanged workers
	SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (changed bool, err error)

	// LoadDesired retrieves user intent (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadDesiredTyped[T]() instead.
	//
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if desired state doesn't exist.
	LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadDesiredTyped (interface method) loads desired state into the provided pointer.
	//
	// NOTE: This is the INTERFACE METHOD (uses reflection). There is also a GENERIC FUNCTION
	// with the same name: LoadDesiredTyped[T](). Choose based on your use case:
	//
	//   - Interface method (this): Use for runtime polymorphic code where types aren't
	//     known at compile time (e.g., generic supervisors iterating over worker types)
	//
	//   - Generic function LoadDesiredTyped[T](): Use when type is known at compile time
	//     for type safety, better performance, and IDE autocomplete
	//
	// Example (interface method):
	//
	//	var desired MyDesiredState
	//	err := store.LoadDesiredTyped(ctx, workerType, id, &desired)
	LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// SaveObserved stores system reality with delta checking (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveObservedTyped[T]() instead.
	//
	// Auto-increments _sync_id for delta synchronization.
	// Accepts interface{} to support both persistence.Document and typed FSM states.
	//
	// Returns (changed bool, error) where:
	//   - changed=true: Data was different from existing, delta was recorded, sync_id incremented
	//   - changed=false: Data identical to existing, no write occurred, no delta created
	//
	// Use cases for checking `changed`:
	//   - Avoid redundant downstream notifications when nothing changed
	//   - Track actual modification frequency for monitoring
	//   - Optimize sync by skipping unchanged workers
	SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error)

	// LoadObserved retrieves system state (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadObservedTyped[T]() instead.
	//
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if observed state doesn't exist.
	LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadObservedTyped (interface method) loads observed state into the provided pointer.
	//
	// NOTE: This is the INTERFACE METHOD (uses reflection). There is also a GENERIC FUNCTION
	// with the same name: LoadObservedTyped[T](). Choose based on your use case:
	//
	//   - Interface method (this): Use for runtime polymorphic code where types aren't
	//     known at compile time (e.g., generic supervisors iterating over worker types)
	//
	//   - Generic function LoadObservedTyped[T](): Use when type is known at compile time
	//     for type safety, better performance, and IDE autocomplete
	//
	// Example (interface method):
	//
	//	var observed MyObservedState
	//	err := store.LoadObservedTyped(ctx, workerType, id, &observed)
	LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// LoadSnapshot atomically loads all three parts of the triangular model (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadSnapshotTyped[T]() instead (if available).
	//
	// Ensures consistent view of worker state at a single point in time.
	//
	// IMPORTANT: Check for nil before accessing snapshot fields:
	//   - Identity: Always present if worker exists
	//   - Desired: May be nil before first SaveDesired call
	//   - Observed: May be nil before first SaveObserved call
	//
	// Example (safe access):
	//
	//	snapshot, err := store.LoadSnapshot(ctx, "container", id)
	//	if snapshot.Observed != nil {
	//	    observedDoc := snapshot.Observed.(persistence.Document)
	//	    cpu := observedDoc["cpu"]
	//	}
	//
	// Returns persistence.ErrNotFound if the worker doesn't exist at all.
	LoadSnapshot(ctx context.Context, workerType string, id string) (*Snapshot, error)

	// GetLatestSyncID returns the current sync_id (latest event sequence number).
	// This can be used by clients to establish their initial sync position.
	GetLatestSyncID(ctx context.Context) (int64, error)

	// GetDeltas returns delta changes for sync clients.
	// Returns DeltasResponse containing either:
	//   - Incremental deltas if client is recent enough
	//   - Full bootstrap data if client is too far behind
	GetDeltas(ctx context.Context, sub Subscription) (DeltasResponse, error)
}

// Compile-time check that TriangularStore implements TriangularStoreInterface.
var _ TriangularStoreInterface = (*TriangularStore)(nil)
