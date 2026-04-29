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
// # Rolling-upgrade (HA) constraint
//
// Upgrade is safe. Old instances write CSE JSON that contains a "state" key;
// new instances read that JSON and silently drop the field because
// encoding/json ignores unknown keys. No data loss occurs.
//
// # Rollback constraint
//
// Rollback is NOT safe once new code has written a DesiredState without the
// "state" key. Old code reading such JSON sees GetState()==""  and the old
// ValidateDesiredState block rejects it, stalling the reconciliation loop.
// To roll back safely: clear all CSE DesiredState entries before switching back
// to old code, or restore a pre-upgrade CSE snapshot.
package migration

// MigrateP3_5dDesiredState is applied lazily on every reconciliation tick when
// the supervisor loads a DesiredState from CSE storage.
//
// The migration is a no-op: Go's encoding/json silently drops the old "state"
// key when unmarshalling into config.BaseDesiredState (which no longer has that
// field), so no explicit JSON surgery is required.
//
// The function exists to (a) document the migration boundary, (b) serve as a
// test hook that can be instrumented in integration tests, and (c) reserve the
// call-site in reconciliation.go for future compensating logic if the rollback
// constraint above must be relaxed.
//
// Parameters:
//   - desiredStateJSON: the raw JSON bytes loaded from CSE for one worker.
//
// Returns:
//   - migrated: always nil (the caller continues with the original bytes).
//   - changed: always false (no rewrite needed).
//   - err: always nil.
func MigrateP3_5dDesiredState(_ []byte) (migrated []byte, changed bool, err error) {
	// No-op: encoding/json already handles the "state" key removal implicitly.
	return nil, false, nil
}
