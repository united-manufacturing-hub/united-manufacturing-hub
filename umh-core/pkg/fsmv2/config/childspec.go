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

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
)

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
// Do not add fields like ShouldRun, IsRunning, Enabled, Active, or State to your DesiredState.
//
// Correct lifecycle control:
//   - ShutdownRequested: Inherited from this type. Set by supervisor for graceful shutdown.
//   - ParentMappedState: For child workers only. Injected by supervisor from parent's ChildStartStates.
//
// The desired lifecycle state ("running"/"stopped") is read from the user spec
// via BaseUserSpec.GetState() in DeriveDesiredState and stored directly on the
// outer DesiredState type (not on BaseDesiredState). Callers that used to set
// BaseDesiredState{State: x} should instead set the State field on the embedding
// struct (e.g., MyDesiredState{State: x, ...}).
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
	ShutdownRequested bool `json:"ShutdownRequested" yaml:"ShutdownRequested"` //nolint:tagliatelle // Match JSON field name for API compatibility
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
	Variables VariableBundle `json:"variables" yaml:"variables"` // Variable bundle (User, Global, Internal namespaces)
	Config    string         `json:"config"    yaml:"config"`    // Raw user-provided configuration (YAML, JSON, or other format)
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
// INVARIANT: If a parent state checks ChildrenHealthy/ChildrenUnhealthy to gate
// its own transitions, that state MUST be listed in ChildStartStates. Otherwise,
// children are mapped to "stopped" and can never satisfy the health check,
// creating a permanent deadlock.
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
	Dependencies     map[string]any `json:"dependencies,omitempty"     yaml:"dependencies,omitempty"`     // Additional deps to merge with parent's deps (child overrides parent)
	UserSpec         UserSpec       `json:"userSpec"                   yaml:"userSpec"`                   // Raw user config (input to DeriveDesiredState)
	Name             string         `json:"name"                       yaml:"name"`                       // Unique name for this child (within parent scope)
	WorkerType       string         `json:"workerType"                 yaml:"workerType"`                 // Type of worker to create (registered worker factory key)
	ChildStartStates []string       `json:"childStartStates,omitempty" yaml:"childStartStates,omitempty"` // Parent FSM states where child should run (empty = always run)
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

// Hash returns a deterministic hash of a ChildSpec for change detection.
// This is used by the supervisor to detect when a ChildSpec has changed,
// enabling incremental validation (only re-validate specs whose hash changed).
//
// The hash is computed from all relevant fields: Name, WorkerType, UserSpec,
// ChildStartStates, and Dependencies (excluding any unexported fields).
//
// Returns a hex-encoded FNV-1a 64-bit hash string (16 characters) and an error
// if Variables cannot be marshaled to JSON.
func (c ChildSpec) Hash() (string, error) {
	h := fnv.New64a()

	// Hash name and worker type with null byte separators to prevent collisions
	// e.g., Name="ab", WorkerType="cd" vs Name="abc", WorkerType="d" would otherwise
	// produce the same hash input ("abcd" vs "abcd")
	h.Write([]byte(c.Name))
	h.Write([]byte{0}) // separator
	h.Write([]byte(c.WorkerType))
	h.Write([]byte{0}) // separator

	// Hash UserSpec (config string + variables)
	h.Write([]byte(c.UserSpec.Config))
	h.Write([]byte{0}) // separator

	varsBytes, err := json.Marshal(c.UserSpec.Variables)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Variables for hashing: %w", err)
	}

	h.Write(varsBytes)
	h.Write([]byte{0}) // separator

	// Hash ChildStartStates
	for _, state := range c.ChildStartStates {
		h.Write([]byte(state))
		h.Write([]byte{0}) // separator
	}

	// Dependencies contain runtime objects (channels, etc.) that can't be meaningfully hashed,
	// so we skip them. Changes to dependencies don't require re-validation anyway.

	return fmt.Sprintf("%016x", h.Sum64()), nil
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
	if len(c.ChildStartStates) == 0 {
		return DesiredStateRunning
	}

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
	ErrorMsg      string // Error message if unhealthy (empty if healthy)
	HierarchyPath string // Full path in the worker hierarchy (e.g., "app.parent.child")
	IsHealthy     bool   // Whether the child is considered healthy (PhaseRunningHealthy only)
	IsOperational bool   // Whether the child is operational (PhaseRunningHealthy or PhaseRunningDegraded)
	IsStopped     bool   // Whether the child is in a stopped phase
	// Infrastructure status fields (framework-tracked)
	IsStale       bool // True if observation age > stale threshold (~10s)
	IsCircuitOpen bool // True if infrastructure failure detected (circuit breaker open)
}

// ChildrenView is a per-tick snapshot of a parent worker's children. The
// supervisor builds one via NewChildrenView on every collector cycle and
// injects it onto ObservedState through SetChildrenView, then round-trips it
// through CSE storage to the reconciler goroutine.
//
// Aggregate predicates (HealthyCount, UnhealthyCount, AllHealthy,
// AllOperational, AllStopped) are pre-computed at construction time from each
// ChildInfo.Phase so readers access them as direct field reads instead of
// method calls.
//
// To consume a ChildrenView in a worker:
//
// 1. Add fields to your ObservedState to store children info:
//
//	type MyObservedState struct {
//	    ChildrenHealthy   int
//	    ChildrenUnhealthy int
//	}
//
// 2. Implement SetChildrenView on your ObservedState (the supervisor calls it
//    automatically through the ChildrenViewConsumer capability interface):
//
//	func (o MyObservedState) SetChildrenView(view config.ChildrenView) fsmv2.ObservedState {
//	    o.ChildrenHealthy = view.HealthyCount
//	    o.ChildrenUnhealthy = view.UnhealthyCount
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
// See workers/example/examplechild/snapshot/snapshot.go for the simple counts pattern.
type ChildrenView struct {
	Children       []ChildInfo `json:"children"`
	HealthyCount   int         `json:"healthyCount"`
	UnhealthyCount int         `json:"unhealthyCount"`
	AllHealthy     bool        `json:"allHealthy"`
	AllOperational bool        `json:"allOperational"`
	AllStopped     bool        `json:"allStopped"`
}

// NewChildrenView builds a ChildrenView from a slice of ChildInfo entries.
// Aggregate predicates are derived from each ChildInfo's IsHealthy,
// IsOperational, and IsStopped booleans:
//
//   - HealthyCount counts entries with IsHealthy true.
//   - UnhealthyCount counts entries that are neither healthy nor stopped.
//   - AllHealthy is true when every entry is healthy, or when there are no
//     entries.
//   - AllOperational is true when every entry is operational, or when there
//     are no entries.
//   - AllStopped is true when every entry is stopped, or when there are no
//     entries.
//
// A nil input slice is normalised to an empty slice so CSE delta-sync produces
// a stable JSON shape across ticks. Tests that need a fake ChildrenView build
// one by passing a hand-authored []ChildInfo to this constructor, with no mock
// library required.
func NewChildrenView(children []ChildInfo) ChildrenView {
	if children == nil {
		children = []ChildInfo{}
	}

	view := ChildrenView{
		Children:       children,
		AllHealthy:     true,
		AllOperational: true,
		AllStopped:     true,
	}

	for _, c := range children {
		if c.IsHealthy {
			view.HealthyCount++
		} else if !c.IsStopped {
			view.UnhealthyCount++
		}

		if !c.IsHealthy {
			view.AllHealthy = false
		}

		if !c.IsOperational {
			view.AllOperational = false
		}

		if !c.IsStopped {
			view.AllStopped = false
		}
	}

	return view
}

// Get returns the ChildInfo for name as a copy along with a presence bool.
// The bool is false when no child with that name exists in the view.
func (v ChildrenView) Get(name string) (ChildInfo, bool) {
	for i := range v.Children {
		if v.Children[i].Name == name {
			return v.Children[i], true
		}
	}

	return ChildInfo{}, false
}

// List returns the slice of ChildInfo entries.
//
// Deprecated: read v.Children directly. Retained as a migration shim for
// callers written against the prior ChildrenView interface.
func (v ChildrenView) List() []ChildInfo {
	return v.Children
}

// Counts returns the pre-computed (healthy, unhealthy) child counts.
//
// Deprecated: read v.HealthyCount and v.UnhealthyCount directly. Retained as
// a migration shim for callers written against the prior ChildrenView
// interface.
func (v ChildrenView) Counts() (healthy, unhealthy int) {
	return v.HealthyCount, v.UnhealthyCount
}

// DesiredState represents what we want the system to be.
// This is returned by Worker.DeriveDesiredState() and used by State.Next() for decisions.
//
// Deprecated: New workers should return *fsmv2.WrappedDesiredState[TConfig] from
// DeriveDesiredState instead. WrappedDesiredState promotes BaseDesiredState fields
// alongside TConfig fields and is the canonical Pattern A desired state type.
// DesiredState remains supported for legacy test helpers (supervisor/testutil) but
// should not be used in new production workers.
//
// The supervisor injects shutdown requests by setting ShutdownRequested = true.
// Workers must check IsShutdownRequested() first in their State.Next() implementations.
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
//	    BaseDesiredState: BaseDesiredState{ShutdownRequested: true},  // Triggers shutdown sequence
//	    ChildrenSpecs:    nil,                                        // Children removed during shutdown
//	}
type DesiredState struct {
	OriginalUserSpec interface{}      `json:"originalUserSpec,omitempty" yaml:"-"`                       // Captures the input that produced this DesiredState (for debugging/traceability)
	State            string           `json:"state"                      yaml:"state"`                   // "stopped" or "running" - desired lifecycle state
	ChildrenSpecs    []ChildSpec      `json:"childrenSpecs,omitempty"    yaml:"childrenSpecs,omitempty"` // Declarative specification of child workers
	BaseDesiredState `yaml:",inline"` // Provides ShutdownRequested field and IsShutdownRequested/SetShutdownRequested methods
}

// GetState returns the desired lifecycle state, defaulting to "running" if empty.
func (d *DesiredState) GetState() string {
	if d.State == "" {
		return DesiredStateRunning
	}

	return d.State
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
//
// Deprecated: This method exists to support the deprecated DesiredState type.
// WrappedDesiredState[TConfig].GetChildrenSpecs() is the canonical implementation.
func (d *DesiredState) GetChildrenSpecs() []ChildSpec {
	return d.ChildrenSpecs
}

// NOTE: GetState() is defined directly on DesiredState (above), not on the embedded BaseDesiredState.
// The State field on DesiredState is the canonical source of truth for lifecycle state.
