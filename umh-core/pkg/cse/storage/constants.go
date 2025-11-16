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

// CSE Metadata Field Constants
//
// DESIGN DECISION: Use compile-time constants for CSE field names.
// WHY: Field names are stable conventions (_sync_id, _version, etc.) that never change.
// TRADE-OFF: No flexibility in field naming, but consistency is critical for sync engine.
// INSPIRED BY: Linear's sync engine metadata field conventions (_sync_id, _version).
const (
	// FieldSyncID is the global sync version, incremented on every change across all collections.
	// Used for efficient "give me all changes since sync_id X" queries.
	// Index this field for sync queries: WHERE _sync_id > last_synced.
	FieldSyncID = "_sync_id"

	// FieldVersion is the document version used for optimistic locking.
	// Prevents lost updates: UPDATE ... WHERE _version = expected_version.
	FieldVersion = "_version"

	// FieldCreatedAt is the creation timestamp.
	// Immutable after initial insert.
	FieldCreatedAt = "_created_at"

	// FieldUpdatedAt is the last update timestamp.
	// Updated on every modification.
	FieldUpdatedAt = "_updated_at"

	// FieldDeletedAt is the soft delete timestamp (NULL if not deleted).
	// Enables soft deletes without losing data.
	FieldDeletedAt = "_deleted_at"

	// FieldDeletedBy is the user who deleted the record (for audit trail).
	// NULL if not deleted.
	FieldDeletedBy = "_deleted_by"
)

// Collection Role Constants
//
// DESIGN DECISION: Three-collection triangular model per worker type.
// WHY: Separates identity (what), desired (intent), observed (reality).
// TRADE-OFF: More collections to manage, but clearer separation of concerns.
// INSPIRED BY: FSM v2 triangular model architecture.
const (
	// RoleIdentity represents immutable worker identity (IP, hostname, bootstrap config).
	// Never changes after creation, establishes "what is this worker?".
	RoleIdentity = "identity"

	// RoleDesired represents user intent and configuration.
	// What the user WANTS the worker to do.
	RoleDesired = "desired"

	// RoleObserved represents system reality and current state.
	// What the worker IS ACTUALLY doing.
	RoleObserved = "observed"
)

// CSE Field Sets per Role
//
// DESIGN DECISION: Hardcode CSE fields per role (discovered all roles use identical fields)
// WHY: Eliminates need for registry lookup to determine which fields to inject.
//      All roles follow same CSE convention: _sync_id, _version, timestamps.
//
// INSPIRED BY: Investigation that revealed CSEFields are identical across all worker types.
//              Registry was providing runtime lookup for compile-time constants.
//
// TRADE-OFF: Less flexible than registry, but we don't need flexibility here.
//            CSE metadata conventions are stable and don't vary per worker type.

// Identity CSE fields (immutable, created once)
// WARNING: This slice is exported for read-only access. DO NOT MODIFY.
// Modifying this slice will break CSE metadata conventions across the system.
var IdentityCSEFields = []string{
	FieldSyncID,    // Global sync version for delta queries
	FieldVersion,   // Always 1 for identity (never changes)
	FieldCreatedAt, // Timestamp of identity creation
}

// Desired CSE fields (user intent, versioned for optimistic locking)
// WARNING: This slice is exported for read-only access. DO NOT MODIFY.
// Modifying this slice will break CSE metadata conventions across the system.
var DesiredCSEFields = []string{
	FieldSyncID,    // Global sync version
	FieldVersion,   // Increments on each update (optimistic locking)
	FieldCreatedAt, // Timestamp of first save
	FieldUpdatedAt, // Timestamp of last save
}

// Observed CSE fields (system reality, frequently updated)
// WARNING: This slice is exported for read-only access. DO NOT MODIFY.
// Modifying this slice will break CSE metadata conventions across the system.
var ObservedCSEFields = []string{
	FieldSyncID,    // Global sync version
	FieldVersion,   // Increments on each update
	FieldCreatedAt, // Timestamp of first observation
	FieldUpdatedAt, // Timestamp of last observation
}

// getCSEFields returns the appropriate CSE fields for a given role.
// This replaces registry lookup with compile-time constant selection.
func getCSEFields(role string) []string {
	switch role {
	case RoleIdentity:
		return IdentityCSEFields
	case RoleDesired:
		return DesiredCSEFields
	case RoleObserved:
		return ObservedCSEFields
	default:
		// Should never happen with typed enum
		panic("unknown role: " + role)
	}
}
