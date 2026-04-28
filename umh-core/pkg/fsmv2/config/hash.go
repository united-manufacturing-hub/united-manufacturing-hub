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

package config

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
)

// ComputeUserSpecHash computes a deterministic hash of a UserSpec.
// This is used by the supervisor to detect when a UserSpec has changed,
// enabling it to skip DeriveDesiredState calls when inputs are unchanged.
//
// The hash is computed from:
//   - Config string (hashed directly)
//   - The full Variables bundle, serialized as sorted JSON (deterministic
//     because Go's JSON marshaler sorts map keys; the typed
//     VariablesInternal struct serializes in field order)
//
// Variables.Internal IS included in the hash (the JSON tag is "internal"
// post-P1.5c, not "-"), but per-worker hash determinism is preserved
// because Internal contains supervisor-assigned identity (WorkerID,
// ParentID, CreatedAt, BridgedBy) that is fixed at supervisor creation
// and never changes for that worker. The cache is per-worker rather than
// shared across workers — each worker's UserSpec → DesiredState mapping
// gets its own cache entry, which is correct because Internal carries
// per-worker identity that DeriveDesiredState may legitimately consume
// (e.g. via templates that reference {{ .internal.id }}).
//
// Cache behavior: DeriveDesiredState is called when Config or Variables change.
// If neither changes, the cached result is reused.
//
// Map key ordering does not affect the hash because JSON marshaling
// in Go produces sorted keys for maps.
//
// Returns a hex-encoded FNV-1a 64-bit hash string (16 characters).
func ComputeUserSpecHash(spec UserSpec) string {
	h := fnv.New64a()
	h.Write([]byte(spec.Config))

	// Marshal failure falls back to Config-only hash; Variables from YAML are always serializable.
	if varsBytes, err := json.Marshal(spec.Variables); err == nil {
		h.Write(varsBytes)
	}

	return fmt.Sprintf("%016x", h.Sum64())
}
