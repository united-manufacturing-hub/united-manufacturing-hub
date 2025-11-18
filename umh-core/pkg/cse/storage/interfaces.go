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
// MIGRATION STATUS: The API migration is complete. Both patterns are fully supported and
// coexist intentionally. The interface methods are NOT deprecated - they serve the runtime
// polymorphic use case. Use the typed functions for compile-time type safety when the
// worker type is known.
//
// See MIGRATION.md for detailed examples and migration patterns.
type TriangularStoreInterface interface {
	// SaveIdentity stores immutable worker identity (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveIdentityTyped[T]() instead.
	//
	// Identity is created once and never updated.
	SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error

	// LoadIdentity retrieves worker identity (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadIdentityTyped[T]() instead.
	//
	// Returns persistence.ErrNotFound if identity doesn't exist.
	LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error)

	// SaveDesired stores user intent/configuration (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveDesiredTyped[T]() instead.
	//
	// Auto-increments _version on each save for optimistic concurrency control.
	SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error

	// LoadDesired retrieves user intent (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadDesiredTyped[T]() instead.
	//
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if desired state doesn't exist.
	LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadDesiredTyped loads desired state and deserializes into provided pointer.
	// This method supports reflection-based code that doesn't know types at compile time.
	// For compile-time type safety, use LoadDesiredTyped[T]() package-level function instead.
	LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// SaveObserved stores system reality with delta checking (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use SaveObservedTyped[T]() instead.
	//
	// Auto-increments _sync_id for delta synchronization.
	// Accepts interface{} to support both persistence.Document and typed FSM states.
	// Returns (changed bool, error) where changed indicates if data was written.
	SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error)

	// LoadObserved retrieves system state (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadObservedTyped[T]() instead.
	//
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if observed state doesn't exist.
	LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadObservedTyped loads observed state and deserializes into provided pointer.
	// This method supports reflection-based code that doesn't know types at compile time.
	// For compile-time type safety, use LoadObservedTyped[T]() package-level function instead.
	LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// LoadSnapshot atomically loads all three parts of the triangular model (runtime polymorphic API).
	// Use when worker type is determined at runtime (supervisors, factories).
	// For compile-time type-safe code, use LoadSnapshotTyped[T]() instead (if available).
	//
	// Ensures consistent view of worker state at a single point in time.
	// Returns nil for missing parts (e.g., Observed may be nil before first observation).
	LoadSnapshot(ctx context.Context, workerType string, id string) (*Snapshot, error)
}

// Compile-time check that TriangularStore implements TriangularStoreInterface.
var _ TriangularStoreInterface = (*TriangularStore)(nil)
