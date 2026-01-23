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
// Variables.Internal is excluded (via json:"-" tag) because Internal contains
// only supervisor-assigned constants (workerID, parentID) that are set once
// at supervisor creation and never change.
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

	// Hash the Config string directly
	h.Write([]byte(spec.Config))

	// Hash the Variables as JSON for determinism.
	// json.Marshal sorts map keys alphabetically, ensuring consistent output.
	//
	// Note: If marshal fails, we skip Variables and use Config-only hash. This is safe because:
	// 1. Variables come from YAML config, which only produces JSON-serializable types
	// 2. ValidateChildSpec rejects unmarshalable specs upstream (childspec_validation.go)
	// 3. The Config-only hash still provides differentiation for the common case
	// This is defensive programming for an impossible-in-production state.
	if varsBytes, err := json.Marshal(spec.Variables); err == nil {
		h.Write(varsBytes)
	}

	return fmt.Sprintf("%016x", h.Sum64())
}
