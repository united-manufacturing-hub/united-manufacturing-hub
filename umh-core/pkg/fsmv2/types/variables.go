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


package types

// VariableBundle provides three-tier namespace structure for FSMv2 variables.
//
// The three namespaces serve distinct purposes with different serialization
// and template access patterns:
//
// User Namespace:
//   - Contains: User-defined variables + parent state variables + computed values
//   - Template access: Top-level ({{ .IP }}, {{ .PORT }})
//   - Serialization: YES (persisted in state/config files)
//   - Source: Configuration files, parent workers, runtime computations
//   - Purpose: Variables that should be accessible without prefix in templates
//
// Global Namespace:
//   - Contains: Fleet-wide settings from management loop
//   - Template access: Nested ({{ .global.api_endpoint }}, {{ .global.cluster_id }})
//   - Serialization: YES (persisted in state/config files)
//   - Source: Central management system, shared configuration
//   - Purpose: Distinguish fleet-wide settings from worker-specific variables
//
// Internal Namespace:
//   - Contains: Runtime metadata (id, timestamps, bridged_by)
//   - Template access: Nested ({{ .internal.id }}, {{ .internal.timestamp }})
//   - Serialization: NO (runtime-only, not persisted)
//   - Source: FSM runtime, system-generated metadata
//   - Purpose: Metadata that exists during execution but shouldn't be saved
type VariableBundle struct {
	// User contains user-defined variables, parent state variables, and computed values.
	// Variables in this namespace are accessible at top-level in templates ({{ .varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	User map[string]any `json:"user,omitempty" yaml:"user,omitempty"`

	// Global contains fleet-wide settings provided by the management loop.
	// Variables in this namespace require explicit prefix ({{ .global.varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	Global map[string]any `json:"global,omitempty" yaml:"global,omitempty"`

	// Internal contains runtime metadata like worker IDs, timestamps, and bridging info.
	// Variables in this namespace require explicit prefix ({{ .internal.varname }}).
	// This namespace is NOT serialized (yaml:"-" json:"-") and exists only at runtime.
	Internal map[string]any `json:"-" yaml:"-"`
}

// Flatten returns a map with User variables promoted to top-level and Global/Internal nested.
// This enables intuitive template syntax where User variables are accessible as {{ .varname }}
// while Global and Internal require explicit prefixes ({{ .global.varname }}, {{ .internal.varname }}).
//
// Example:
//
//	bundle := VariableBundle{
//	    User: map[string]any{"IP": "192.168.1.100"},
//	    Global: map[string]any{"api_endpoint": "https://api.example.com"},
//	}
//	flattened := bundle.Flatten()
//	// flattened["IP"] = "192.168.1.100"
//	// flattened["global"] = map[string]any{"api_endpoint": "https://api.example.com"}
func (v VariableBundle) Flatten() map[string]any {
	result := make(map[string]any)

	for k, val := range v.User {
		result[k] = val
	}

	result["global"] = v.Global
	result["internal"] = v.Internal

	return result
}
