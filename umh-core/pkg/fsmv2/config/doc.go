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

// Package config provides configuration types for FSMv2 workers.
//
// # Overview
//
// The config package defines:
//   - VariableBundle: Three-tier variable namespace (User, Global, Internal)
//   - ChildSpec: Declaration of child workers for hierarchical composition
//   - DesiredState: Target state with shutdown control
//   - Template utilities for variable expansion
//
// # Variable tiers
//
// Variables are organized into three namespaces:
//
// User Namespace:
//   - Contains: Worker-specific configuration (IP addresses, ports, URLs)
//   - Template access: Top-level {{ .IP }}, {{ .PORT }}
//   - Serialization: YES (persisted in config files)
//   - Use case: Most common variables, no prefix needed
//
// Global Namespace:
//   - Contains: Fleet-wide settings (cluster ID, environment, API endpoints)
//   - Template access: Prefixed {{ .global.cluster_id }}
//   - Serialization: YES (persisted in config files)
//   - Use case: Shared settings across all workers
//
// Internal Namespace:
//   - Contains: Supervisor-injected identity (worker ID, parent ID,
//     creation timestamp, bridge-source label) — typed VariablesInternal
//     struct, not a free-form map
//   - Template access: Prefixed {{ .internal.id }}, {{ .internal.parent_id }},
//     {{ .internal.created_at }}, {{ .internal.bridged_by }}
//   - Serialization: JSON YES (json:"internal", camelCase wire tags per
//     §4-D LOCKED for codegen cleanliness; round-trips through CSE storage
//     between supervisor goroutines per Design Intent §13/§14). YAML NO
//     (yaml:"-" — users do not author Internal; supervisor injects it at
//     reconciliation time)
//   - Use case: identity / structural desired state injected by the system,
//     not user-authored. Templates read it via the Flatten map's snake_case
//     keys; the JSON wire vocabulary uses camelCase. Wire tags and flatten
//     keys are independent.
//
// # User variable flattening
//
// User variables are flattened to top-level in templates because they
// represent 80%+ of template variables. {{ .IP }} is cleaner than {{ .user.IP }}.
//
// Flattening example:
//
//	bundle := VariableBundle{
//	    User:     map[string]any{"IP": "192.168.1.100"},
//	    Global:   map[string]any{"cluster": "prod"},
//	    Internal: VariablesInternal{WorkerID: "worker-42", CreatedAt: time.Now()},
//	}
//
//	flat := bundle.Flatten()
//	// flat["IP"] = "192.168.1.100"               (top-level)
//	// flat["global"]["cluster"] = "prod"         (nested)
//	// flat["internal"]["id"] = "worker-42"       (nested, typed struct)
//	// flat["internal"]["created_at"] = time.Time (nested, typed struct)
//
// Template usage:
//
//	{{ .IP }}                   // User variable (flattened)
//	{{ .global.cluster }}       // Global variable (nested)
//	{{ .internal.id }}          // Internal identity field (typed struct)
//	{{ .internal.created_at }}  // Internal creation time (typed struct)
//
// # Internal variable serialization
//
// Internal carries the typed VariablesInternal struct (yaml:"-",
// json:"internal"). Per §4-D LOCKED, the JSON form round-trips through CSE
// storage between supervisor goroutines (Design Intent §13/§14). YAML
// serialization is suppressed because users do not author Internal — the
// supervisor injects identity at reconciliation time.
//
// # map[string]any design (User and Global only)
//
// VariableBundle.User and VariableBundle.Global use map[string]any because
// users define arbitrary config fields in YAML. A typed struct would
// require code changes for each new variable. Go templates validate
// variable existence and type at render time.
//
// VariableBundle.Internal is the typed exception: it is a VariablesInternal
// struct, not a map, because Internal is supervisor-injected identity
// (fixed shape, fixed field names) rather than user-authored arbitrary
// data. Per Design Intent §13/§4-D LOCKED, the typed struct codegens
// cleanly to TypeScript and avoids the `Record<string, unknown>` surface
// that bare map[string]any produces.
//
// # ChildSpec and hierarchical composition
//
// ChildSpec declares child workers that a parent worker manages:
//
//	children := []config.ChildSpec{
//	    {
//	        Name:       "mqtt-connection",
//	        WorkerType: "mqtt_client",
//	        UserSpec:   config.UserSpec{Config: "url: tcp://..."},
//	        Variables: config.VariableBundle{
//	            User: map[string]any{"URL": "tcp://localhost:1883"},
//	        },
//	        ChildStartStates: []string{"Running", "TryingToStart"},
//	    },
//	}
//
// ChildSpec is declarative: parents declare what children should exist, not
// how to create them. The supervisor handles creation, deletion, and updates.
// Children can be added or removed by changing ChildrenSpecs in DeriveDesiredState().
//
// # ChildStartStates
//
// ChildStartStates specifies which parent FSM states cause the child to run:
//
//	ChildStartStates: []string{"Running", "TryingToStart"}
//	// Child runs when parent is in "Running" or "TryingToStart"
//	// Child stops when parent is in any other state
//
// The child runs if the parent state is in the list, stops otherwise.
// An empty list means the child always runs.
//
// ChildStartStates handles lifecycle coordination, not data passing.
// Use VariableBundle to pass data from parent to child.
//
// # DesiredState and shutdown control
//
// DesiredState includes IsShutdownRequested() for graceful shutdown:
//
//	type DesiredState struct {
//	    State            string
//	    ShutdownRequested bool
//	    ChildrenSpecs    []ChildSpec
//	}
//
// The supervisor sets ShutdownRequested when shutdown is initiated. Workers
// check this flag in their state transitions and complete cleanup states
// before signaling removal. Parent shutdown propagates ShutdownRequested
// to all children, so children clean up before parent completes shutdown.
//
// # Template expansion
//
// The config package provides template utilities for variable expansion:
//
//	config := `
//	    address: "{{ .IP }}:{{ .PORT }}"
//	    cluster: "{{ .global.cluster_id }}"
//	`
//
//	result, err := config.ExpandTemplate(config, bundle.Flatten())
//	// result: "address: \"192.168.1.100:502\"\ncluster: \"prod\""
//
// Template expansion happens during DeriveDesiredState(), transforming user
// configuration into concrete desired state.
//
// # Validation
//
// ChildSpec validation checks:
//   - Name is non-empty
//   - WorkerType is registered in factory
//   - Variables are valid (no nil maps)
//
// See childspec_validation.go for validation details.
package config
