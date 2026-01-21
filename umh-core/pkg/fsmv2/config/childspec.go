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
//
// # Lifecycle Control Invariant
//
// The FSM controls worker lifecycle through state transitions, not through custom bool fields.
// Do not add fields like ShouldRun, IsRunning, Enabled, or Active to your DesiredState.
//
// Correct lifecycle control:
//   - ShutdownRequested: Inherited from this type. Set by supervisor for graceful shutdown.
//   - ParentMappedState: For child workers only. Injected by supervisor from parent's ChildStartStates.
//   - State ("running"/"stopped"): From BaseUserSpec.GetState(). Controls whether worker should be running.
//
// Correct ShouldBeRunning() implementations:
//
//	// Root/leaf workers (no parent):
//	func (s *MyDesiredState) ShouldBeRunning() bool {
//	    return !s.ShutdownRequested
//	}
//
//	// Child workers (have parent):
//	func (s *MyDesiredState) ShouldBeRunning() bool {
//	    return !s.ShutdownRequested && s.ParentMappedState == config.DesiredStateRunning
//	}
//
// See ValidateNoCustomLifecycleFields in pkg/fsmv2/internal/validator/snapshot.go for enforcement.
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

// GetState returns the desired lifecycle state.
// Implements fsmv2.DesiredState interface.
func (b *BaseDesiredState) GetState() string {
	if b.State == "" {
		return DesiredStateRunning
	}

	return b.State
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
func (b *BaseUserSpec) GetState() string {
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

// Clone creates a deep copy of the UserSpec.
// The Config string is copied by value, Variables maps are deep-copied.
func (u UserSpec) Clone() UserSpec {
	return UserSpec{
		Config:    u.Config,
		Variables: u.Variables.Clone(),
	}
}

// ChildSpec is a declarative specification for a child FSM worker.
// Parent workers return these in DeriveDesiredState().ChildrenSpecs to declare their children.
// The supervisor reconciles actual children to match these specs (Kubernetes-style).
//
// # Declarative Child Management
//
// Parents don't create/destroy children directly. Instead they declare what should exist,
// and the supervisor handles creation, updates, and cleanup automatically:
//
//  1. Parent returns []ChildSpec in DesiredState.ChildrenSpecs
//  2. Supervisor compares with actual children
//  3. Supervisor creates missing children
//  4. Supervisor updates changed children
//  5. Supervisor removes extra children
//
// Clean separation of concerns:
//   - Parents focus on "what should exist"
//   - Supervisor handles "how to make it exist"
//   - Children run independently in their own FSMs
//
// # Child Start States
//
// ChildStartStates coordinates parent state with child lifecycle.
//
// ChildStartStates specifies which parent FSM states cause children to run.
// When the parent is in a listed state, children run. Otherwise, they stop.
//
// Example use case: Children should only run when parent is in "Running" or "TryingToStart" states.
//
// Format:
//
//	ChildStartStates: []string{"Running", "TryingToStart"}
//
// When to use:
// - Parent lifecycle controls child lifecycle (children run only in certain parent states)
// - Simple "run when parent is active" patterns
//
// When not to use:
// - Passing data between states (use VariableBundle instead)
// - Triggering actions (use signals instead)
//
// # Dependency Inheritance
//
// Dependencies are additional deps to merge with parent's deps.
// Child values override parent values for the same keys.
// NOTE: This is a shallow merge - interface/channel values are shared, not copied.
// Set to nil to use parent's deps unchanged.
//
// Example - Protocol converter managing connections:
//
//	// Parent (protocol converter) declares a child (MQTT connection)
//	ChildSpec{
//	    Name:       "mqtt-connection",
//	    WorkerType: "mqtt_client",
//	    UserSpec:   UserSpec{Config: "url: tcp://localhost:1883"},
//	    ChildStartStates: []string{"Running", "TryingToStart"}, // Child runs when parent is active
//	}
//
// Example - Benthos managing connections and data flows:
//
//	// Benthos declares multiple children with different lifecycle rules
//	[]ChildSpec{
//	    {
//	        Name:       "modbus-connection",
//	        WorkerType: "modbus_client",
//	        UserSpec:   UserSpec{Config: "address: 192.168.1.100:502"},
//	        // Empty ChildStartStates = child always runs (follows parent's DesiredState.State)
//	    },
//	    {
//	        Name:       "source-flow",
//	        WorkerType: "benthos_flow",
//	        UserSpec:   UserSpec{Config: "input: {...}"},
//	        ChildStartStates: []string{"Running"}, // Child only runs when parent is Running
//	    },
//	}
type ChildSpec struct {
	Name             string         `json:"name"                       yaml:"name"`                       // Unique name for this child (within parent scope)
	WorkerType       string         `json:"workerType"                 yaml:"workerType"`                 // Type of worker to create (registered worker factory key)
	UserSpec         UserSpec       `json:"userSpec"                   yaml:"userSpec"`                   // Raw user config (input to DeriveDesiredState)
	ChildStartStates []string       `json:"childStartStates,omitempty" yaml:"childStartStates,omitempty"` // Parent FSM states where child should run (empty = always run)
	Dependencies     map[string]any `json:"dependencies,omitempty"     yaml:"dependencies,omitempty"`     // Additional deps to merge with parent's deps (child overrides parent)
}

// MarshalJSON implements json.Marshaler for ChildSpec.
// Consistent JSON serialization across the system.
func (c *ChildSpec) MarshalJSON() ([]byte, error) {
	type Alias ChildSpec

	return json.Marshal((*Alias)(c))
}

// Clone creates a deep copy of the ChildSpec.
// Note: Dependencies is shallow-copied (values are shared intentionally since they
// represent shared resources like channels and interfaces).
func (c ChildSpec) Clone() ChildSpec {
	clone := c

	clone.UserSpec = c.UserSpec.Clone()

	if c.ChildStartStates != nil {
		clone.ChildStartStates = make([]string, len(c.ChildStartStates))
		copy(clone.ChildStartStates, c.ChildStartStates)
	}

	if c.Dependencies != nil {
		clone.Dependencies = make(map[string]any, len(c.Dependencies))
		for k, v := range c.Dependencies {
			clone.Dependencies[k] = v
		}
	}

	return clone
}

// GetMappedChildState returns the desired state for this child based on the parent's current FSM state.
//
// Logic:
//   - If ChildStartStates is empty: child always runs (returns "running")
//   - If parentState is in ChildStartStates: child should run (returns "running")
//   - Otherwise: child should stop (returns "stopped")
//
// Example:
//
//	spec := ChildSpec{ChildStartStates: []string{"Running", "TryingToStart"}}
//	spec.GetMappedChildState("Running")        // returns "running"
//	spec.GetMappedChildState("TryingToStop")   // returns "stopped"
//	spec.GetMappedChildState("Stopped")        // returns "stopped"
//
// Note: This replaces the deprecated StateMapping approach with direct state checks.
func (c *ChildSpec) GetMappedChildState(parentState string) string {
	// Empty ChildStartStates = always run
	if len(c.ChildStartStates) == 0 {
		return DesiredStateRunning
	}

	// Check if parent state is in the list
	for _, state := range c.ChildStartStates {
		if state == parentState {
			return DesiredStateRunning
		}
	}

	return DesiredStateStopped
}

// ChildInfo provides a read-only snapshot of a child worker's current state.
// This is used by ChildrenView to give parent workers visibility into their children
// without allowing direct modification.
//
// All fields are copies, not references - modifying them has no effect on the actual child.
type ChildInfo struct {
	Name          string // Child name (unique within parent scope)
	WorkerType    string // Child worker type
	StateName     string // Current FSM state name (e.g., "Running", "TryingToStart")
	StateReason   string // Human-readable reason for current state
	IsHealthy     bool   // Whether the child is considered healthy
	ErrorMsg      string // Error message if unhealthy (empty if healthy)
	HierarchyPath string // Full path in the worker hierarchy (e.g., "app.parent.child")
}

// ChildrenView provides read-only access to a parent worker's children.
// This interface enables parent workers to observe their children's state
// without being able to modify them.
//
// The supervisor injects ChildrenView via setter methods on ObservedState,
// not through dependencies. To use it:
//
// 1. Add fields to your ObservedState to store children info:
//
//	type MyObservedState struct {
//	    ChildrenHealthy   int
//	    ChildrenUnhealthy int
//	}
//
// 2. Implement SetChildrenView on your ObservedState (called automatically by supervisor):
//
//	func (o MyObservedState) SetChildrenView(view any) fsmv2.ObservedState {
//	    if cv, ok := view.(config.ChildrenView); ok {
//	        o.ChildrenHealthy, o.ChildrenUnhealthy = cv.Counts()
//	        // Or use cv.List(), cv.Get(name), cv.AllHealthy(), cv.AllStopped()
//	    }
//	    return o
//	}
//
// 3. Access children info in State.Next() via the snapshot:
//
//	func (s *RunningState) Next(snap MySnapshot) (State, Signal, Action) {
//	    if snap.Observed.ChildrenUnhealthy > 0 {
//	        return &DegradedState{}, SignalNone, nil
//	    }
//	    // ...
//	}
//
// For simpler use cases, implement SetChildrenCounts instead:
//
//	func (o MyObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
//	    o.ChildrenHealthy = healthy
//	    o.ChildrenUnhealthy = unhealthy
//	    return o
//	}
//
// See workers/example/exampleparent/snapshot/snapshot.go for the simple counts pattern.
type ChildrenView interface {
	// List returns info about all children.
	List() []ChildInfo

	// Get returns info about a specific child by name, or nil if not found.
	Get(name string) *ChildInfo

	// Counts returns the number of healthy and unhealthy children.
	Counts() (healthy, unhealthy int)

	// AllHealthy returns true if all children are healthy (or there are no children).
	AllHealthy() bool

	// AllStopped returns true if all children are in the Stopped state (or there are no children).
	AllStopped() bool
}

// DesiredState represents what we want the system to be.
// This is returned by Worker.DeriveDesiredState() and used by State.Next() for decisions.
//
// The supervisor can inject shutdown requests by setting State to "shutdown".
// Workers must check ShutdownRequested() first in their State.Next() implementations.
//
// # Children Management
//
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
	BaseDesiredState                                                                                   // Provides ShutdownRequested field and methods (IsShutdownRequested, SetShutdownRequested)
	State            string      `json:"state"                      yaml:"state"`                      // Current desired state ("running", "stopped", "shutdown", etc.)
	ChildrenSpecs    []ChildSpec `json:"childrenSpecs,omitempty"    yaml:"childrenSpecs,omitempty"`    // Declarative specification of child workers
	OriginalUserSpec interface{} `json:"originalUserSpec,omitempty" yaml:"-"`                          // Captures the input that produced this DesiredState (for debugging/traceability)
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
//	    // Always check shutdown first
//	    if desired.IsShutdownRequested() {
//	        return StoppingState{}, fsmv2.SignalNone, nil
//	    }
//	    // ... rest of logic
//	}

// ChildSpecProvider is implemented by DesiredState types that can have children.
// Used by supervisor to extract children specs for reconciliation.
// Workers with children should return a DesiredState type implementing this interface.
type ChildSpecProvider interface {
	GetChildrenSpecs() []ChildSpec
}

// GetChildrenSpecs returns the children specifications.
// Implements ChildSpecProvider interface.
func (d *DesiredState) GetChildrenSpecs() []ChildSpec {
	return d.ChildrenSpecs
}

// GetState returns the desired lifecycle state.
// Implements fsmv2.DesiredState interface.
// This method uses DesiredState.State field (not the embedded BaseDesiredState.State).
func (d *DesiredState) GetState() string {
	if d.State == "" {
		return DesiredStateRunning
	}

	return d.State
}
