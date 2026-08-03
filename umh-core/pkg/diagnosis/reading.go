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

// Reading is a value or an absence.
type Reading struct {
	v  float64
	ok bool
}

// Known returns a Reading carrying a value.
func Known(f float64) Reading { return Reading{v: f, ok: true} }

// Unknown returns a Reading carrying an absence.
func Unknown() Reading { return Reading{} }

// Get returns the value and whether it is present.
func (r Reading) Get() (float64, bool) { return r.v, r.ok }
