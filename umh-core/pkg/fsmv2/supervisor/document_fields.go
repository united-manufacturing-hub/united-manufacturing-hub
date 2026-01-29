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

package supervisor

// Document Field Constants for FSMv2
//
// DESIGN DECISION: Only define constants for fields unique to FSMv2.
// Timestamp fields (created_at, updated_at) should use CSE storage constants
// (FieldCreatedAt, FieldUpdatedAt) from pkg/cse/storage/constants.go to avoid duplication.
const (
	// FieldID is the worker unique identifier.
	FieldID = "id"

	// FieldName is the worker display name.
	FieldName = "name"

	// FieldWorkerType is the worker type designation.
	FieldWorkerType = "worker_type"

	// FieldShutdownRequested is the graceful shutdown signal.
	// Set in desired documents to request FSM shutdown.
	// PascalCase matches struct tag: `json:"ShutdownRequested"`.
	FieldShutdownRequested = "ShutdownRequested"

	// FieldParentID is the parent supervisor ID (renamed from "bridged_by").
	FieldParentID = "parent_id"
)
