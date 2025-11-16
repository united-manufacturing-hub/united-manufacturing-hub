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
type TriangularStoreInterface interface {
	// SaveIdentity stores immutable worker identity.
	// Identity is created once and never updated.
	SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error

	// LoadIdentity retrieves worker identity.
	// Returns persistence.ErrNotFound if identity doesn't exist.
	LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error)

	// SaveDesired stores user intent/configuration.
	// Auto-increments _version on each save for optimistic concurrency control.
	SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error

	// LoadDesired retrieves user intent.
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if desired state doesn't exist.
	LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadDesiredTyped loads desired state and deserializes into provided pointer.
	// Used by reflection-based code. For compile-time type safety, use LoadDesiredTyped[T]() function.
	LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// SaveObserved stores system reality with delta checking.
	// Auto-increments _sync_id for delta synchronization.
	// Accepts interface{} to support both persistence.Document and typed FSM states.
	// Returns (changed bool, error) where changed indicates if data was written.
	SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error)

	// LoadObserved retrieves system state.
	// Returns persistence.Document or typed struct based on TypeRegistry.
	// Returns persistence.ErrNotFound if observed state doesn't exist.
	LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error)

	// LoadObservedTyped loads observed state and deserializes into provided pointer.
	// Used by reflection-based code. For compile-time type safety, use LoadObservedTyped[T]() function.
	LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error

	// LoadSnapshot atomically loads all three parts of the triangular model.
	// Ensures consistent view of worker state at a single point in time.
	// Returns nil for missing parts (e.g., Observed may be nil before first observation).
	LoadSnapshot(ctx context.Context, workerType string, id string) (*Snapshot, error)
}

// Compile-time check that TriangularStore implements TriangularStoreInterface.
var _ TriangularStoreInterface = (*TriangularStore)(nil)
