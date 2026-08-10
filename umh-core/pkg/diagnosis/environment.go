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

// Capability is a startup fact about the environment: whether a source exists on
// this box at all. An instrument names the capabilities it needs in Requires,
// and Signal.Capable keeps the instruments the environment satisfies. It is the
// first of the three gates the package doc lists; the other two are per-tick.
type Capability string

// Environment is the set of capabilities present.
type Environment struct {
	caps map[Capability]bool
}

// NewEnvironment builds an Environment holding the given capabilities. The set
// is fixed once built: every tick sees the set declared at start.
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
