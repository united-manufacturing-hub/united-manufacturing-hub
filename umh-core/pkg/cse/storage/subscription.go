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

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"

// Subscription represents a client's sync subscription with position and optional filters.
type Subscription struct {
	Queries    []Query // What data to receive (empty = all)
	LastSyncID int64   // Client's last sync position
}

// Query is a stub for now - filtering will be added later. Empty = all changes.
type Query struct{}

// Delta represents a single change event for sync clients (diff only, not full document).
type Delta struct {
	Changes     *Diff  // Field-level changes (Added, Modified, Removed)
	WorkerType  string // Type of worker (e.g., "container", "relay")
	WorkerID    string // Unique identifier for the worker
	Role        string // "identity", "desired", or "observed"
	SyncID      int64  // Global monotonic sequence number
	TimestampMs int64  // When the change occurred (Unix ms)
}

// DeltasResponse contains incremental deltas or full bootstrap data if client is too far behind.
type DeltasResponse struct {
	Bootstrap         *BootstrapData // Full state if bootstrap needed
	Deltas            []Delta        // Incremental changes since LastSyncID
	LatestSyncID      int64          // Current sync position
	RequiresBootstrap bool           // True if client too far behind
	HasMore           bool           // More deltas available (pagination)
}

// BootstrapData contains full state for clients too far behind for incremental deltas.
type BootstrapData struct {
	Workers     []WorkerSnapshot // Full state of all workers
	AtSyncID    int64            // Sync position of this bootstrap
	TimestampMs int64            // When bootstrap was generated
}

// WorkerSnapshot contains the complete triangular state for a single worker.
type WorkerSnapshot struct {
	Identity   persistence.Document // Immutable identity (may be nil)
	Desired    persistence.Document // User intent (may be nil)
	Observed   persistence.Document // System reality (may be nil)
	WorkerType string               // Type of worker
	WorkerID   string               // Unique identifier
}
