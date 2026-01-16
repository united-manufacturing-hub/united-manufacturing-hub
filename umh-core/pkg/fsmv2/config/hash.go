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
//   - Variables.User and Variables.Global (serialized as sorted JSON for determinism)
//
// NOTE: Variables.Internal is NOT included in the hash because it has the
// json:"-" tag (see variables.go). This is intentional: Internal variables
// are regenerated per-tick by the supervisor (workerID, createdAt, parentID)
// and remain stable within a supervisor's lifetime. They do not need to
// trigger cache invalidation.
//
// Map key ordering does not affect the hash because JSON marshaling
// in Go produces sorted keys for maps.
//
// Returns a hex-encoded FNV-1a 64-bit hash string (16 characters).
func ComputeUserSpecHash(spec UserSpec) string {
	h := fnv.New64a()

	// Hash the Config string directly
	h.Write([]byte(spec.Config))

	// Hash the Variables as JSON for determinism
	// json.Marshal sorts map keys alphabetically, ensuring consistent output
	if varsBytes, err := json.Marshal(spec.Variables); err == nil {
		h.Write(varsBytes)
	}
	// On marshal error, we just skip Variables - the Config hash alone provides some differentiation

	return fmt.Sprintf("%016x", h.Sum64())
}
