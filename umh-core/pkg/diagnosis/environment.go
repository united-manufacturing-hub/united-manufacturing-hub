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

package diagnosis

// Capability is a startup fact about the environment: whether a source exists
// here at all. It is NOT whether this tick's read succeeded; that is
// readability, a per-tick fact that must never be folded into a capability or a
// verdict, and a window left unselected that freezes is not a capability either.
// It is also not whether a window can supply a value right now; that is
// readiness, and it lives on the engine.
type Capability string

// Environment is the set of capabilities present.
type Environment struct {
	caps map[Capability]bool
}

// NewEnvironment builds an Environment from a set of capabilities. Capabilities
// are startup facts the caller owns, whether the box is virtualized or a quota
// is set, so without this the caller cannot build one and therefore cannot call
// Observe at all.
func NewEnvironment(caps ...Capability) Environment {
	set := make(map[Capability]bool, len(caps))
	for _, c := range caps {
		set[c] = true
	}
	return Environment{caps: set}
}

// Has reports whether a capability is present.
func (e Environment) Has(c Capability) bool {
	return e.caps[c]
}
