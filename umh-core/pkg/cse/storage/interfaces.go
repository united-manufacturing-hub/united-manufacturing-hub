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
// It manages Identity (immutable), Desired (user intent), and Observed (system reality).
//
// Implementations automatically inject CSE metadata (_sync_id, _version, timestamps).
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
	// Identity is write-once: idempotent if same content, error if different content.
	// For compile-time type safety, use SaveIdentityTyped[T]() instead.
	SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error

	// LoadIdentity retrieves worker identity. Returns persistence.ErrNotFound if missing.
	// For compile-time type safety, use LoadIdentityTyped[T]() instead.
	LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error)

	// SaveDesired stores user intent with delta checking (runtime polymorphic API).
	// Skips writes when data unchanged. Returns changed=true if data differed and was written.
	// For compile-time type safety, use SaveDesiredTyped[T]() instead.
	SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (changed bool, err error)

	// LoadDesired retrieves user intent. Returns persistence.ErrNotFound if missing.
	// For compile-time type safety, use LoadDesiredTyped[T]() instead.
	LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadDesiredTyped loads desired state into dest pointer (uses reflection).
	// For compile-time type safety, use the generic function LoadDesiredTyped[T]() instead.
	LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// SaveObserved stores system reality with delta checking (runtime polymorphic API).
	// Skips writes when data unchanged. Returns changed=true if data differed and was written.
	// For compile-time type safety, use SaveObservedTyped[T]() instead.
	SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error)

	// LoadObserved retrieves system state. Returns persistence.ErrNotFound if missing.
	// For compile-time type safety, use LoadObservedTyped[T]() instead.
	LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadObservedTyped loads observed state into dest pointer (uses reflection).
	// For compile-time type safety, use the generic function LoadObservedTyped[T]() instead.
	LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// LoadSnapshot atomically loads Identity, Desired, and Observed for consistent view.
	// Desired/Observed may be nil if not yet saved. Returns persistence.ErrNotFound if worker missing.
	LoadSnapshot(ctx context.Context, workerType string, id string) (*Snapshot, error)

	// GetLatestSyncID returns the current sync_id for clients to establish initial sync position.
	GetLatestSyncID(ctx context.Context) (int64, error)

	// GetDeltas returns incremental deltas or full bootstrap data if client is too far behind.
	GetDeltas(ctx context.Context, sub Subscription) (DeltasResponse, error)
}

// Compile-time check that TriangularStore implements TriangularStoreInterface.
var _ TriangularStoreInterface = (*TriangularStore)(nil)
