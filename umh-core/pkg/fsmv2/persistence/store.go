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

package persistence

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// Store is the interface for persisting FSM state.
// Implementations can use SQLite (edge), Postgres (HA), or other backends.
//
// The workerType parameter identifies the FSM type (e.g., "container", "s6", "benthos")
// and maps to corresponding tables:
//   - Identity: {workerType}_identity
//   - Desired:  {workerType}_desired
//   - Observed: {workerType}_observed
//
// All operations automatically increment _sync_id for CSE compatibility.
//
// Note: Transaction support (BeginTx) was removed as it is not currently used
// by the supervisor. Implementations can use their own internal transactions
// without exposing them through this interface.
type Store interface {
	// SaveIdentity persists the immutable identity of a worker.
	// The data parameter contains worker-specific identity fields.
	SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error

	// LoadIdentity retrieves the identity of a worker.
	// Returns nil if not found.
	LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error)

	// SaveDesired persists the desired state (user intent).
	// Automatically increments _sync_id and _version.
	SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error

	// LoadDesired retrieves the desired state.
	// Returns nil if not found.
	LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error)

	// SaveObserved persists the observed state (system reality).
	// Automatically increments _sync_id.
	// The observed state must include a mirror of the desired state fields.
	SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error

	// LoadObserved retrieves the observed state.
	// Returns nil if not found.
	LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error)

	// LoadSnapshot retrieves a complete snapshot by joining identity, desired, and observed.
	// This is the primary method used by the supervisor on each tick.
	// Returns error if worker not found.
	LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error)

	// GetLastSyncID returns the current global sync version.
	// Used by CSE for delta sync.
	GetLastSyncID(ctx context.Context) (int64, error)

	// IncrementSyncID atomically increments and returns the new global sync version.
	// Called automatically by Save operations.
	IncrementSyncID(ctx context.Context) (int64, error)

	// Close closes the store and releases resources.
	Close() error
}

// Tx represents a database transaction.
// Note: Transactions are not currently supported in this interface.
// This type exists for future compatibility.
type Tx interface {
	Store

	// Commit persists all changes made within the transaction.
	Commit() error

	// Rollback discards all changes made within the transaction.
	Rollback() error
}
