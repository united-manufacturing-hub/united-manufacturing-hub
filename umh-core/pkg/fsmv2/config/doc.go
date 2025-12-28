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
// # Why Three Variable Tiers?
//
// Variables are organized into three namespaces for different purposes:
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
//   - Contains: Runtime metadata (timestamps, derived values, system state)
//   - Template access: Prefixed {{ .internal.timestamp }}
//   - Serialization: NO (runtime-only, not persisted)
//   - Use case: Values that shouldn't be saved to config
//
// # Why User Variables Get Top-Level Access?
//
// User variables are "flattened" to top-level in templates because:
//
// 1. Most Common Use Case: 80%+ of template variables are user-defined
// (IP addresses, ports, connection strings). Making them top-level
// reduces boilerplate.
//
// 2. Ergonomic Templates: {{ .IP }} is cleaner than {{ .user.IP }}
//
// 3. Backward Compatibility: Existing templates expect top-level access.
//
// Flattening example:
//
//	bundle := VariableBundle{
//	    User:     map[string]any{"IP": "192.168.1.100"},
//	    Global:   map[string]any{"cluster": "prod"},
//	    Internal: map[string]any{"timestamp": time.Now()},
//	}
//
//	flat := bundle.Flatten()
//	// flat["IP"] = "192.168.1.100"           (top-level)
//	// flat["global"]["cluster"] = "prod"     (nested)
//	// flat["internal"]["timestamp"] = ...    (nested)
//
// Template usage:
//
//	{{ .IP }}                  // User variable (flattened)
//	{{ .global.cluster }}      // Global variable (nested)
//	{{ .internal.timestamp }}  // Internal variable (nested)
//
// # Why Internal Variables Are Not Serialized?
//
// Internal variables use json:"-" yaml:"-" tags because:
//
// 1. Runtime-Only: Values like timestamps and system state shouldn't be
// saved to config files. They're computed fresh each run.
//
// 2. Avoid Stale Data: Saved timestamps would be wrong on next load.
//
// 3. Security: Some internal values might be sensitive (tokens, PIDs).
//
// # Why map[string]any Instead of Structs?
//
// VariableBundle uses map[string]any because:
//
// 1. User-Defined Fields: Users define arbitrary config fields in YAML.
// We cannot pre-type {{ .MyCustomField }} or {{ .IP_PRIMARY }}.
//
// 2. Dynamic Templates: Template variables are user-controlled. A typed
// struct would require code changes for each new variable.
//
// 3. Type Safety at Render: Go templates validate variable existence and
// type at render time. Missing or wrong-typed variables produce clear errors.
//
// # ChildSpec and Hierarchical Composition
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
// Why ChildSpec instead of direct child creation?
//
// 1. Declarative: Parent declares what children SHOULD exist, not HOW
// to create them. Supervisor handles creation/deletion/updates.
//
// 2. Kubernetes-Style: Like Deployments managing Pods. Desired state vs
// actual state reconciliation.
//
// 3. Dynamic: Children can be added/removed by changing ChildrenSpecs
// in DeriveDesiredState(). No manual worker creation needed.
//
// # ChildStartStates: Coordinating Parent and Child Lifecycle
//
// ChildStartStates specifies which parent FSM states cause the child to run:
//
//	ChildStartStates: []string{"Running", "TryingToStart"}
//	// Child runs when parent is in "Running" or "TryingToStart"
//	// Child stops when parent is in any other state
//
// Why ChildStartStates?
//
// 1. Coordinated Lifecycle: Parent controls when children start/stop.
// A parent in "Stopping" state automatically stops all children.
//
// 2. Simple Logic: Child runs if parent state is in the list, stops otherwise.
// Empty list means child always runs.
//
// 3. Declarative Control: Parent doesn't imperatively start/stop children.
// It declares the desired child state, and the child FSM handles transitions.
//
// Note: ChildStartStates is for lifecycle coordination, not data passing.
// Use VariableBundle to pass data from parent to child.
//
// # DesiredState and Shutdown Control
//
// DesiredState includes IsShutdownRequested() for graceful shutdown:
//
//	type DesiredState struct {
//	    State            string
//	    ShutdownRequested bool
//	    ChildrenSpecs    []ChildSpec
//	}
//
// Why ShutdownRequested in DesiredState?
//
// 1. Supervisor Control: The supervisor sets ShutdownRequested when shutdown
// is initiated. Workers check this in their state transitions.
//
// 2. Graceful Shutdown: Workers transition through proper cleanup states
// before signaling removal. No abrupt termination.
//
// 3. Hierarchy-Aware: Parent shutdown sets ShutdownRequested for all children.
// Children clean up before parent completes shutdown.
//
// # Template Expansion
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
// Template expansion happens during DeriveDesiredState(), transforming
// user configuration into concrete desired state.
//
// # Validation
//
// ChildSpec validation ensures:
//   - Name is non-empty
//   - WorkerType is registered in factory
//   - Variables are valid (no nil maps)
//
// See childspec_validation.go for validation details.
package config
