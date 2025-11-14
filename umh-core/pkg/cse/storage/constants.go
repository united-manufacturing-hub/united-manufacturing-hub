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
var IdentityCSEFields = []string{
	FieldSyncID,    // Global sync version for delta queries
	FieldVersion,   // Always 1 for identity (never changes)
	FieldCreatedAt, // Timestamp of identity creation
}

// Desired CSE fields (user intent, versioned for optimistic locking)
var DesiredCSEFields = []string{
	FieldSyncID,    // Global sync version
	FieldVersion,   // Increments on each update (optimistic locking)
	FieldCreatedAt, // Timestamp of first save
	FieldUpdatedAt, // Timestamp of last save
}

// Observed CSE fields (system reality, frequently updated)
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
