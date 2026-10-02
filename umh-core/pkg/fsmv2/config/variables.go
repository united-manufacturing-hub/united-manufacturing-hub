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

// VariableBundle provides three-tier namespace structure for FSMv2 variables.
//
// The three namespaces serve distinct purposes with different serialization
// and template access patterns:
//
// WHY map[string]any INSTEAD OF TYPED STRUCTS:
//
// VariableBundle uses map[string]any because:
//  1. Users define arbitrary config fields in YAML (cannot pre-type)
//  2. Template variables are user-defined ({{ .CustomField }})
//  3. Type safety enforced at template rendering (Golang templates validate)
//
// Example user YAML:
//
//	variables:
//	  CustomIP: "192.168.1.100"      # User-defined field
//	  CustomPort: 502                # User-defined field
//	  MySpecialFlag: true            # User-defined field
//
// We CANNOT use structs because field names are user-controlled.
// Type safety happens when template renders (undefined vars = error).
//
// User Namespace:
//   - Contains: User-defined variables + parent state variables + computed values
//   - Template access: Top-level ({{ .IP }}, {{ .PORT }})
//   - Serialization: YES (persisted in state/config files)
//   - Source: Configuration files, parent workers, runtime computations
//   - Purpose: Variables that should be accessible without prefix in templates
//
// Global Namespace:
//   - Contains: Settings a spec passes to its own worker and to every worker below it
//   - Template access: Nested ({{ .global.api_endpoint }}, {{ .global.cluster_id }})
//   - Serialization: YES (persisted in state/config files)
//   - Source: A spec's variables.global, or the dynamic-children registry (Writer.SetVariables)
//   - Purpose: Distinguish shared settings from worker-specific variables
//
// Internal Namespace:
//   - Contains: Framework-injected identity/structural desired state
//     (worker ID, parent ID, creation timestamp, optional bridged-by tag).
//   - Template access: Nested ({{ .internal.id }}, {{ .internal.created_at }})
//   - Serialization: NO (runtime-only; supervisor regenerates per-worker)
//   - Source: FSM runtime, system-generated metadata.
//   - Purpose: Identity carried alongside the rest of desired state.
type VariableBundle struct {
	// User contains user-defined variables, parent state variables, and computed values.
	// Variables in this namespace are accessible at top-level in templates ({{ .varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	User map[string]any `json:"user,omitempty" yaml:"user,omitempty"`

	// Global holds settings a spec passes to its own worker and to every worker below it.
	// Variables in this namespace require explicit prefix ({{ .global.varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	Global map[string]any `json:"global,omitempty" yaml:"global,omitempty"`

	// Internal carries framework-injected identity (worker ID, parent ID,
	// creation timestamp, optional bridged-by tag). Runtime-only: not
	// serialized to JSON or YAML. The supervisor regenerates per-worker
	// identity at reconciliation time.
	Internal map[string]any `json:"-" yaml:"-"`
}

// Flatten returns a map with User variables promoted to top-level and
// Global/Internal nested. User variables flatten to top-level
// ({{ .varname }}); Global and Internal require explicit prefixes
// ({{ .global.varname }}, {{ .internal.varname }}).
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
		// Skip reserved keys to avoid collision with namespace prefixes.
		// Users should not define variables named "global" or "internal" as these
		// are reserved for the Global and Internal namespace maps.
		if k == "global" || k == "internal" {
			continue
		}

		result[k] = val
	}

	if v.Global != nil {
		result["global"] = v.Global
	}

	if v.Internal != nil {
		result["internal"] = v.Internal
	}

	return result
}

// VariableConflict names a key that both bundles set.
type VariableConflict struct {
	Namespace string // "User" or "Global"
	Key       string
}

// ChildVariableConflict is a VariableConflict in one named child's spec.
type ChildVariableConflict struct {
	Child string
	VariableConflict
}

// MergeResult holds the merged bundle and each key both bundles set.
type MergeResult struct {
	Bundle    VariableBundle
	Conflicts []VariableConflict
}

// Merge creates a new VariableBundle combining parent and child.
// On a key both set, the parent's value wins, in User and in Global.
// Internal is NOT merged (regenerated by supervisor for each worker).
//
// Example:
//
//	parent.User = {IP: "10.0.0.1", PORT: 502}
//	parent.Global = {api_endpoint: "https://api.example.com"}
//	child.User  = {DEVICE_ID: "plc-01", PORT: 503}
//	child.Global = {cluster_id: "cluster-01"}
//	result.User = {IP: "10.0.0.1", PORT: 502, DEVICE_ID: "plc-01"}
//	result.Global = {api_endpoint: "https://api.example.com", cluster_id: "cluster-01"}
func Merge(parent, child VariableBundle) VariableBundle {
	result := MergeWithConflicts(parent, child)

	return result.Bundle
}

// MergeWithConflicts is like Merge but also returns each key both bundles
// set, whatever the values.
func MergeWithConflicts(parent, child VariableBundle) MergeResult {
	user, userConflicts := mergeNamespace("User", parent.User, child.User)
	global, globalConflicts := mergeNamespace("Global", parent.Global, child.Global)

	// Set Global to nil if empty to maintain JSON omitempty behavior
	if len(global) == 0 {
		global = nil
	}

	return MergeResult{
		Bundle: VariableBundle{
			User:   user,
			Global: global,
		},
		Conflicts: append(userConflicts, globalConflicts...),
	}
}

// mergeNamespace merges one namespace with the precedence Merge documents.
func mergeNamespace(namespace string, parent, child map[string]any) (map[string]any, []VariableConflict) {
	merged := make(map[string]any)

	var conflicts []VariableConflict

	for k, v := range parent {
		merged[k] = deepCloneValue(v)
	}

	for k, v := range child {
		if _, exists := merged[k]; exists {
			conflicts = append(conflicts, VariableConflict{Namespace: namespace, Key: k})

			continue
		}

		merged[k] = deepCloneValue(v)
	}

	return merged, conflicts
}

// deepCloneMap creates a deep copy of a map preserving original types.
// Unlike JSON round-trip, this preserves numeric types (int, int64, float32, etc.)
// instead of coercing all numbers to float64.
func deepCloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))

	for k, v := range m {
		result[k] = deepCloneValue(v)
	}

	return result
}

// deepCloneValue recursively clones a value, preserving its original type.
func deepCloneValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		return deepCloneMap(val)

	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = deepCloneValue(item)
		}

		return result

	// Primitive types are immutable, return as-is (preserving original type)
	// This preserves int, int64, float32, float64, bool, string, etc.
	default:
		return v
	}
}

// Clone creates a deep copy of the VariableBundle.
// All maps are deeply copied (including nested structures) to prevent shared references.
// Internal is NOT copied (regenerated per-worker by supervisor).
func (v VariableBundle) Clone() VariableBundle {
	clone := VariableBundle{}

	clone.User = deepCloneMap(v.User)
	clone.Global = deepCloneMap(v.Global)

	return clone
}
