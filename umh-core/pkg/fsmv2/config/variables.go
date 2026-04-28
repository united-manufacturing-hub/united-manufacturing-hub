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

import "time"

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
//   - Contains: Fleet-wide settings from management loop
//   - Template access: Nested ({{ .global.api_endpoint }}, {{ .global.cluster_id }})
//   - Serialization: YES (persisted in state/config files)
//   - Source: Central management system, shared configuration
//   - Purpose: Distinguish fleet-wide settings from worker-specific variables
//
// Internal Namespace:
//   - Contains: Framework-injected identity/structural desired state
//     (worker ID, parent ID, creation timestamp, optional bridged-by tag).
//   - Template access: Nested ({{ .internal.id }}, {{ .internal.created_at }})
//   - Serialization: YES on JSON (round-trips through CSE storage between
//     collector and reconciler goroutines per Design Intent §13/§14); NO on
//     YAML (users do not author this — supervisor injects it).
//   - Source: FSM runtime, system-generated metadata.
//   - Purpose: Identity carried alongside the rest of desired state.
//
// Why typed (not map[string]any): per §4-D LOCKED, Internal carries
// identity/structural desired state that the supervisor injects. Typing the
// struct codegens cleanly to TypeScript (vs the "any leak" Record<string,
// unknown> a bare map produces, see §17), and makes ChildSpec.Hash output
// deterministic across map-iteration orderings.
type VariableBundle struct {
	// User contains user-defined variables, parent state variables, and computed values.
	// Variables in this namespace are accessible at top-level in templates ({{ .varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	User map[string]any `json:"user,omitempty" yaml:"user,omitempty"`

	// Global contains fleet-wide settings provided by the management loop.
	// Variables in this namespace require explicit prefix ({{ .global.varname }}).
	// This namespace is serialized to YAML/JSON and persisted with state/config.
	Global map[string]any `json:"global,omitempty" yaml:"global,omitempty"`

	// Internal carries framework-injected identity (worker ID, parent ID,
	// creation timestamp, optional bridged-by tag). The field round-trips
	// through CSE storage as part of the observation document so JSON
	// serialization is required; YAML is suppressed because users do not
	// author this content.
	Internal VariablesInternal `json:"internal" yaml:"-"`
}

// VariablesInternal is the framework-injected identity / structural desired
// state for a worker. Unlike User and Global variables that the user authors,
// Internal is populated by the supervisor at injection time
// (reconciliation.go) and round-trips through CSE storage as part of the
// observation document.
//
// The JSON tags preserve the historical wire keys (id, parent_id, created_at,
// bridged_by) so existing CSE documents remain decodable through the
// transition; the Go field names follow Go conventions (PascalCase). Per §17,
// keeping the wire shape stable avoids a delta-storm against pre-P1.5c
// observation documents.
type VariablesInternal struct {
	// WorkerID is the unique worker identifier. Always populated by the
	// supervisor injector. Templates read it as {{ .internal.id }} via the
	// flatten map; the JSON wire tag is camelCase per §4-D LOCKED so the
	// codegened TypeScript surface stays idiomatic.
	WorkerID string `json:"workerID"            yaml:"-"`

	// ParentID is the parent supervisor's worker ID. Empty for root
	// supervisors. Templates read it as {{ .internal.parent_id }} via the
	// flatten map; JSON wire is camelCase per §4-D LOCKED.
	ParentID string `json:"parentID,omitempty"  yaml:"-"`

	// BridgedBy carries the optional bridge-source label used by tests and
	// some adapter pathways. Empty for the common case. Templates read it
	// as {{ .internal.bridged_by }} via the flatten map; JSON wire is
	// camelCase per §4-D LOCKED.
	BridgedBy string `json:"bridgedBy,omitempty" yaml:"-"`

	// CreatedAt is the supervisor-recorded creation time of the worker
	// document, mirroring CSE storage's createdAt field. Templates read it
	// as {{ .internal.created_at }} via the flatten map; JSON wire is
	// camelCase per §4-D LOCKED.
	CreatedAt time.Time `json:"createdAt"           yaml:"-"`
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

	result["internal"] = v.Internal.flatten()

	return result
}

// flatten projects VariablesInternal into the map-shaped form the templating
// layer expects ({{ .internal.id }}, {{ .internal.parent_id }}, etc.). Note
// the deliberate snake_case keys here (`id`, `parent_id`, `bridged_by`,
// `created_at`) — they preserve the existing template vocabulary while the
// JSON wire tags use camelCase per §4-D LOCKED. Wire tags and flatten keys
// are independent.
//
// Required fields (`id`, `created_at`) are always emitted; the supervisor
// injector guarantees non-zero values before template rendering, so a
// missing `id` here would indicate an invariant violation upstream rather
// than a template error.
//
// Optional fields (`parent_id`, `bridged_by`) are omitted when empty so
// that referencing them in templates surfaces an explicit missing-key
// error rather than a silent empty-string substitution.
func (i VariablesInternal) flatten() map[string]any {
	m := map[string]any{
		"id":         i.WorkerID,
		"created_at": i.CreatedAt,
	}

	if i.ParentID != "" {
		m["parent_id"] = i.ParentID
	}

	if i.BridgedBy != "" {
		m["bridged_by"] = i.BridgedBy
	}

	return m
}

// VariableOverride tracks when a child variable overrides a parent variable.
type VariableOverride struct {
	OldValue  any
	NewValue  any
	Namespace string // "User" or "Global"
	Key       string
}

// MergeResult contains the merged VariableBundle and any override warnings.
type MergeResult struct {
	Bundle    VariableBundle
	Overrides []VariableOverride
}

// Merge creates a new VariableBundle combining parent and child.
// Child User/Global variables override parent User/Global variables.
// Internal is NOT merged (regenerated by supervisor for each worker).
//
// Example:
//
//	parent.User = {IP: "10.0.0.1", PORT: 502}
//	parent.Global = {api_endpoint: "https://api.example.com"}
//	child.User  = {DEVICE_ID: "plc-01", PORT: 503}
//	child.Global = {cluster_id: "cluster-01"}
//	result.User = {IP: "10.0.0.1", PORT: 503, DEVICE_ID: "plc-01"}
//	result.Global = {api_endpoint: "https://api.example.com", cluster_id: "cluster-01"}
func Merge(parent, child VariableBundle) VariableBundle {
	result := MergeWithOverrides(parent, child)

	return result.Bundle
}

// MergeWithOverrides is like Merge but also returns a list of overrides.
// This allows callers to log warnings when child variables override parent variables.
func MergeWithOverrides(parent, child VariableBundle) MergeResult {
	merged := VariableBundle{
		User:   make(map[string]any),
		Global: make(map[string]any),
	}

	var overrides []VariableOverride

	// Merge User variables (child overrides parent)
	// Use deepCloneValue to prevent shared references between parent/child and merged bundle.
	for k, v := range parent.User {
		merged.User[k] = deepCloneValue(v)
	}

	for k, v := range child.User {
		if oldVal, exists := merged.User[k]; exists {
			overrides = append(overrides, VariableOverride{
				Namespace: "User",
				Key:       k,
				OldValue:  oldVal,
				NewValue:  v,
			})
		}

		merged.User[k] = deepCloneValue(v)
	}

	// Merge Global variables (child overrides parent)
	// Use deepCloneValue to prevent shared references between parent/child and merged bundle.
	for k, v := range parent.Global {
		merged.Global[k] = deepCloneValue(v)
	}

	for k, v := range child.Global {
		if oldVal, exists := merged.Global[k]; exists {
			overrides = append(overrides, VariableOverride{
				Namespace: "Global",
				Key:       k,
				OldValue:  oldVal,
				NewValue:  v,
			})
		}

		merged.Global[k] = deepCloneValue(v)
	}

	// Set Global to nil if empty to maintain JSON omitempty behavior
	if len(merged.Global) == 0 {
		merged.Global = nil
	}

	return MergeResult{
		Bundle:    merged,
		Overrides: overrides,
	}
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
// Internal is reset to its zero-value VariablesInternal{} on the clone — the
// supervisor injects per-worker identity at reconciliation time, so a clone
// inherits no parent identity. This is implicit in the body (the function
// only assigns User and Global; the un-assigned Internal field gets the
// zero value of the new VariableBundle).
func (v VariableBundle) Clone() VariableBundle {
	clone := VariableBundle{}

	clone.User = deepCloneMap(v.User)
	clone.Global = deepCloneMap(v.Global)

	return clone
}
