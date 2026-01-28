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
//   - Contains: Runtime metadata (timestamps, derived values, system state)
//   - Template access: Prefixed {{ .internal.timestamp }}
//   - Serialization: NO (runtime-only, not persisted)
//   - Use case: Values that shouldn't be saved to config
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
// # Internal variable serialization
//
// Internal variables use json:"-" yaml:"-" tags because they contain
// runtime-only values (timestamps, system state) that are computed fresh
// each run. Saving them would produce stale data on next load.
//
// # map[string]any design
//
// VariableBundle uses map[string]any because users define arbitrary config
// fields in YAML. A typed struct would require code changes for each new
// variable. Go templates validate variable existence and type at render time.
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
