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

package variables

// Each namespace is optional so we can omit empty maps when (re)marshaling.
type VariableBundle struct {
	User     map[string]any `yaml:"user,omitempty"`
	Global   map[string]any `yaml:"global,omitempty"`
	Internal map[string]any `yaml:"internal,omitempty"`
}

// Equal checks if two VariableBundles are equal
func (vb VariableBundle) Equal(other VariableBundle) bool {
	return NewComparator().ConfigsEqual(vb, other)
}

// Convenient helper – flat view the template engine will see
func (vb VariableBundle) Flatten() map[string]any {
	out := map[string]any{}
	// Agent & user vars are both stored in User after precedence resolution.
	for k, v := range vb.User {
		out[k] = v
	}
	if len(vb.Global) > 0 {
		out["global"] = vb.Global
	}
	if len(vb.Internal) > 0 {
		out["internal"] = vb.Internal
	}
	return out
}
