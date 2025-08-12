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

import (
	"gopkg.in/yaml.v3"
)

// Each namespace is optional so we can omit empty maps when (re)marshaling.
type VariableBundle struct {
	User     map[string]any `yaml:"user,omitempty"`
	Global   map[string]any `yaml:"global,omitempty"`
	Internal map[string]any `yaml:"internal,omitempty"`
}

// Equal checks if two VariableBundles are equal.
func (vb VariableBundle) Equal(other VariableBundle) bool {
	return NewComparator().ConfigsEqual(vb, other)
}

// Convenient helper – flat view the template engine will see.
func (vb VariableBundle) Flatten() map[string]any {
	out := map[string]any{}
	// move user variables to the top level
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

// MarshalYAML implements yaml.Marshaler to provide user-friendly YAML output
// User variables are flattened to the top level, while global and internal are omitted.
func (vb VariableBundle) MarshalYAML() (interface{}, error) {
	// If no user variables, return empty map
	if len(vb.User) == 0 {
		return map[string]any{}, nil
	}

	// Return user variables directly (flattened)
	return vb.User, nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read user-friendly YAML input
// All top-level variables are automatically placed in the User namespace.
func (vb *VariableBundle) UnmarshalYAML(value *yaml.Node) error {
	// Handle the case where variables is explicitly structured with namespaces
	type explicitBundle struct {
		User     map[string]any `yaml:"user,omitempty"`
		Global   map[string]any `yaml:"global,omitempty"`
		Internal map[string]any `yaml:"internal,omitempty"`
	}

	var explicit explicitBundle

	err := value.Decode(&explicit)
	if err == nil {
		// Check if this looks like an explicit namespace structure
		if explicit.User != nil || explicit.Global != nil || explicit.Internal != nil {
			vb.User = explicit.User
			vb.Global = explicit.Global
			vb.Internal = explicit.Internal

			return nil
		}
	}

	// Otherwise, treat all variables as user variables (flat structure)
	var userVars map[string]any

	err = value.Decode(&userVars)
	if err != nil {
		return err
	}

	vb.User = userVars
	// Keep existing Global and Internal if they were set programmatically
	// (don't overwrite them with nil)
	return nil
}
