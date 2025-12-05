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

// Package config provides core configuration types for FSMv2, including child specifications, variables, templates, and location hierarchies.
package config

import "encoding/json"

// BaseDesiredState provides common shutdown functionality for all DesiredState types.
// Workers embed this struct to get consistent shutdown handling without boilerplate.
//
// Example:
//
//	type MyDesiredState struct {
//	    config.BaseDesiredState
//	    // ... other fields
//	}
//
// Workers embedding BaseDesiredState automatically satisfy the DesiredState interface's
// IsShutdownRequested() method and the ShutdownRequestable interface's SetShutdownRequested() method.
type BaseDesiredState struct {
	ShutdownRequested bool   `json:"ShutdownRequested"`
	State             string `json:"state"` // "stopped" or "running" - desired lifecycle state
}

// IsShutdownRequested returns whether shutdown has been requested for this worker.
func (b *BaseDesiredState) IsShutdownRequested() bool {
	return b.ShutdownRequested
}

// SetShutdownRequested sets the shutdown requested flag.
// This satisfies the ShutdownRequestable interface.
func (b *BaseDesiredState) SetShutdownRequested(v bool) {
	b.ShutdownRequested = v
}

// BaseUserSpec provides common fields for all user configuration types.
// Workers embed this struct to get consistent state handling.
//
// Example:
//
//	type MyWorkerUserSpec struct {
//	    config.BaseUserSpec
//	    // ... worker-specific fields
//	}
//
// Workers embedding BaseUserSpec can use GetState() to get the desired lifecycle state
// with a default of "running" if not specified by the user.
type BaseUserSpec struct {
	// State specifies the desired lifecycle state: "running" or "stopped".
	// Defaults to "running" if empty. Validated in the supervisor after DeriveDesiredState.
	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

// GetState returns the desired state, defaulting to "running" if empty.
func (b BaseUserSpec) GetState() string {
	if b.State == "" {
		return DesiredStateRunning
	}
	return b.State
}

// UserSpec contains user-provided configuration for a worker.
// This is the "raw" configuration that users write, before templating or transformation.
//
// The supervisor passes this to Worker.DeriveDesiredState() where it's transformed
// into technical configuration (DesiredState). This separation allows workers to:
//   - Parse and validate user input
//   - Apply templates and variable substitution
//   - Add computed/derived settings
//   - Normalize configuration formats
//
// Example flow:
//
//	UserSpec{Config: "host: {{ .IP }}\nport: {{ .PORT }}"}
//	        ↓ DeriveDesiredState()
//	DesiredState{Host: "192.168.1.100", Port: 502}
type UserSpec struct {
	Config    string         `json:"config"    yaml:"config"`    // Raw user-provided configuration (YAML, JSON, or other format)
	Variables VariableBundle `json:"variables" yaml:"variables"` // Variable bundle (User, Global, Internal namespaces)
}

// ChildSpec is a declarative specification for a child FSM worker.
// Parent workers return these in DeriveDesiredState().ChildrenSpecs to declare their children.
// The supervisor reconciles actual children to match these specs (Kubernetes-style).
//
// DECLARATIVE CHILD MANAGEMENT:
// Parents don't create/destroy children directly. Instead they declare what should exist,
// and the supervisor handles creation, updates, and cleanup automatically:
//
//  1. Parent returns []ChildSpec in DesiredState.ChildrenSpecs
//  2. Supervisor compares with actual children
//  3. Supervisor creates missing children
//  4. Supervisor updates changed children
//  5. Supervisor removes extra children
//
// This enables clean separation of concerns:
//   - Parents focus on "what should exist"
//   - Supervisor handles "how to make it exist"
//   - Children run independently in their own FSMs
//
// STATE MAPPING: Parent State → Child State Coordination
//
// StateMapping allows parent FSM states to trigger child FSM state transitions.
// This is NOT data passing - it's state synchronization.
//
// Example use case: When parent enters "Starting" state, force all children
// to enter their "Initializing" state.
//
// Format:
//
//	StateMapping: map[string]string{
//	    "ParentStateName": "ChildStateName",
//	}
//
// When to use:
// - Parent lifecycle controls child lifecycle (e.g., Stopping → Cleanup)
// - Parent operational state affects child behavior (e.g., Paused → Idle)
//
// When NOT to use:
// - Passing data between states (use VariableBundle instead)
// - Triggering actions (use signals instead)
//
// Example:
//
//	StateMapping: map[string]string{
//	    "running":  "active",   // When parent is running, children should be active
//	    "stopping": "stopped",  // When parent is stopping, children should stop
//	}
//
// Example - Protocol converter managing connections:
//
//	// Parent (protocol converter) declares a child (MQTT connection)
//	ChildSpec{
//	    Name:       "mqtt-connection",
//	    WorkerType: "mqtt_client",
//	    UserSpec:   UserSpec{Config: "url: tcp://localhost:1883"},
//	    StateMapping: map[string]string{
//	        "idle":    "stopped",    // When converter idle, disconnect
//	        "active":  "connected",  // When converter active, connect
//	        "closing": "stopped",    // When converter closing, disconnect
//	    },
//	}
//
// Example - Benthos managing connections and data flows:
//
//	// Benthos declares multiple children with different mappings
//	[]ChildSpec{
//	    {
//	        Name:       "modbus-connection",
//	        WorkerType: "modbus_client",
//	        UserSpec:   UserSpec{Config: "address: 192.168.1.100:502"},
//	    },
//	    {
//	        Name:       "source-flow",
//	        WorkerType: "benthos_flow",
//	        UserSpec:   UserSpec{Config: "input: {...}"},
//	        StateMapping: map[string]string{
//	            "running": "active",
//	            "stopped": "stopped",
//	        },
//	    },
//	}
type ChildSpec struct {
	Name         string            `json:"name"                   yaml:"name"`                   // Unique name for this child (within parent scope)
	WorkerType   string            `json:"workerType"             yaml:"workerType"`             // Type of worker to create (registered worker factory key)
	UserSpec     UserSpec          `json:"userSpec"               yaml:"userSpec"`               // Raw user config (input to DeriveDesiredState)
	StateMapping map[string]string `json:"stateMapping,omitempty" yaml:"stateMapping,omitempty"` // Optional parent→child state mapping
}

// MarshalJSON implements json.Marshaler for ChildSpec.
// This ensures consistent JSON serialization across the system.
func (c *ChildSpec) MarshalJSON() ([]byte, error) {
	type Alias ChildSpec

	return json.Marshal((*Alias)(c))
}

// DesiredState represents what we want the system to be.
// This is returned by Worker.DeriveDesiredState() and used by State.Next() for decisions.
//
// The supervisor can inject shutdown requests by setting State to "shutdown".
// Workers MUST check ShutdownRequested() first in their State.Next() implementations.
//
// CHILDREN MANAGEMENT:
// The ChildrenSpecs field enables declarative child management. Parent workers populate
// this to declare what children should exist. The supervisor handles all lifecycle:
//
//	// In parent's DeriveDesiredState():
//	func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    return config.DesiredState{
//	        State: "running",
//	        ChildrenSpecs: []config.ChildSpec{
//	            {Name: "child-1", WorkerType: "mqtt_client", ...},
//	            {Name: "child-2", WorkerType: "modbus_client", ...},
//	        },
//	    }, nil
//	}
//
// Example with shutdown:
//
//	DesiredState{
//	    State:         "shutdown",  // Triggers shutdown sequence
//	    ChildrenSpecs: nil,         // Children removed during shutdown
//	}
type DesiredState struct {
	BaseDesiredState                                                                   // Provides ShutdownRequested field and methods (IsShutdownRequested, SetShutdownRequested)
	State            string      `json:"state"                   yaml:"state"`          // Current desired state ("running", "stopped", "shutdown", etc.)
	ChildrenSpecs    []ChildSpec `json:"childrenSpecs,omitempty" yaml:"childrenSpecs,omitempty"` // Declarative specification of child workers
}

// NOTE: IsShutdownRequested() and SetShutdownRequested() are provided by embedded BaseDesiredState.
// The ShutdownRequested field is the canonical source of truth for shutdown state.
//
// Shutdown flow:
//  1. Supervisor sets ShutdownRequested = true via SetShutdownRequested()
//  2. State.Next() calls IsShutdownRequested() → returns true
//  3. State transitions to shutdown/cleanup states
//  4. Eventually returns SignalNeedsRemoval
//  5. Supervisor removes worker from system
//
// Example usage in State.Next():
//
//	func (s RunningState) Next(snapshot fsmv2.Snapshot) (State, Signal, Action) {
//	    desired := snapshot.Desired.(types.DesiredState)
//	    // ALWAYS check shutdown first
//	    if desired.IsShutdownRequested() {
//	        return StoppingState{}, fsmv2.SignalNone, nil
//	    }
//	    // ... rest of logic
//	}
