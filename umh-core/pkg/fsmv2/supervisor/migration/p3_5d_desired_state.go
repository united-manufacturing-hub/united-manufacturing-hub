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

// Package migration contains per-change lazy migrations applied during the P3
// cascade. Each file documents a single breaking change and the constraints
// operators must observe when deploying.
//
// # P3.5d: BaseDesiredState.State field removed
//
// This change removes the "state" string field from config.BaseDesiredState and
// deletes the DeriveLeafState helper. After P3.5d, DesiredState no longer
// carries a lifecycle-intent field; shutdown is signalled exclusively through
// IsShutdownRequested().
//
// # Why the migration is NOT a no-op
//
// Old CSE records may contain:
//
//	{"state":"stopped","ShutdownRequested":false,...}
//
// After P3.5d, encoding/json silently drops "state" (unknown field) and leaves
// ShutdownRequested at its zero value (false). A worker whose user intent was
// "stopped" would therefore re-enter its running path on the first reconciliation
// tick after upgrade — a silent state flip.
//
// The migration detects "state":"stopped" and promotes it to
// ShutdownRequested:true, preserving the operator's original intent.
// "state":"running" requires no action; ShutdownRequested:false is already the
// correct zero value.
//
// # Rolling-upgrade (HA) constraint
//
// Upgrade is safe. Old code writes JSON with a "state" key; new code reads that
// JSON, runs MigrateP3_5dDesiredState, and produces correct behavior. Workers
// that are already running are unaffected (ShutdownRequested stays false).
// Workers that are stopped have their intent preserved
// (ShutdownRequested set to true).
//
// # Rollback constraint
//
// Rollback after upgrade is NOT safe without additional steps. Once new code
// writes DesiredState JSON without "state", old code reads an empty GetState()
// and interprets it as "running" — stopped workers would start. To roll back
// safely: restore a pre-upgrade CSE snapshot before switching back to old code.
package migration

import (
	"encoding/json"
	"fmt"
)

// desiredStateWire is a minimal wire representation used for migration only.
// It reads the legacy "state" field alongside the canonical ShutdownRequested.
type desiredStateWire struct {
	State             string `json:"state"`
	ShutdownRequested bool   `json:"ShutdownRequested"`
}

// MigrateP3_5dDesiredState is applied lazily on every reconciliation tick when
// the supervisor loads a DesiredState from CSE storage.
//
// If the JSON contains "state":"stopped", the function rewrites the document
// to set ShutdownRequested:true and removes the "state" key, preserving the
// operator's original lifecycle intent. "state":"running" requires no action.
//
// Parameters:
//   - desiredStateJSON: the raw JSON bytes loaded from CSE for one worker.
//
// Returns:
//   - migrated: rewritten JSON bytes if changed is true; nil otherwise (caller
//     uses the original bytes).
//   - changed: true if the document was rewritten (i.e. "state":"stopped" was
//     found and promoted).
//   - err: non-nil only if the JSON is malformed and cannot be parsed.
func MigrateP3_5dDesiredState(desiredStateJSON []byte) (migrated []byte, changed bool, err error) {
	if len(desiredStateJSON) == 0 {
		return nil, false, nil
	}

	var wire desiredStateWire
	if err := json.Unmarshal(desiredStateJSON, &wire); err != nil {
		return nil, false, fmt.Errorf("p3_5d migration: failed to parse desired state JSON: %w", err)
	}

	// "state":"running" (or absent) is fine — ShutdownRequested:false is correct.
	if wire.State != "stopped" {
		return nil, false, nil
	}

	// "state":"stopped" must be promoted to ShutdownRequested:true.
	// Unmarshal the full document into a generic map so we can remove "state"
	// and set ShutdownRequested without losing any other fields.
	var doc map[string]interface{}
	if err := json.Unmarshal(desiredStateJSON, &doc); err != nil {
		return nil, false, fmt.Errorf("p3_5d migration: failed to parse desired state document: %w", err)
	}

	delete(doc, "state")
	doc["ShutdownRequested"] = true

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, false, fmt.Errorf("p3_5d migration: failed to marshal migrated desired state: %w", err)
	}

	return out, true, nil
}
