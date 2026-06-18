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

	"gopkg.in/yaml.v3"
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
// Do not add fields like ShouldRun, IsRunning, Active, or State to your DesiredState.
//
// Correct lifecycle control:
//   - ShutdownRequested: Inherited from this type. Set by supervisor for graceful shutdown.
//   - Disabled: Inherited from this type. Set by the supervisor's disable-mapping pass
//     from the parent's ChildSpec.Enabled; child workers stay resident in Stopped.
//
// The desired lifecycle state ("running"/"stopped") lives on the user spec, not
// on the DesiredState. Embed BaseUserSpec in your UserSpec config type to inherit
// GetState(); DeriveDesiredState reads and validates it. As stated above, do not
// add a State field to your DesiredState — the supervisor drives lifecycle through
// ShutdownRequested and Disabled and never populates a custom State field.
//
// Correct ShouldBeRunning() implementation:
//
//	func (s *MyDesiredState) ShouldBeRunning() bool {
//	    return !s.ShutdownRequested
//	}
//
// Child workers do not gate on a parent-supplied field: the supervisor's
// disable-mapping pass sets Disabled from the parent's ChildSpec.Enabled, and
// state files route lifecycle through snap.ShouldStop().
//
// See ValidateNoCustomLifecycleFields in pkg/fsmv2/internal/validator/snapshot.go for enforcement.
type BaseDesiredState struct {
	ShutdownRequested bool `json:"ShutdownRequested" yaml:"ShutdownRequested"` //nolint:tagliatelle // Match JSON field name for API compatibility
	Disabled          bool `json:"Disabled"          yaml:"Disabled"`          //nolint:tagliatelle // Match JSON field name for API compatibility
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

// IsDisabled is written exclusively by the disable-mapping pass (see supervisor/reconciliation.go applyDisableMapping).
// The restart subsystem never sets it.
func (b *BaseDesiredState) IsDisabled() bool {
	return b.Disabled
}

func (b *BaseDesiredState) SetDisabled(v bool) {
	b.Disabled = v
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
// # Child Lifecycle Gating
//
// A child runs while the parent sets ChildSpec.Enabled=true for it. The
// supervisor's disable-mapping pass translates Enabled into the child's
// Disabled bit, and state files route lifecycle through snap.ShouldStop().
// Setting Enabled=false leaves the child resident in Stopped (not despawned);
// flipping it back to true resumes the child on the next tick.
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
//	    Enabled:    true, // Parent wants this child running
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
//	        Enabled:    true,
//	    },
//	    {
//	        Name:       "source-flow",
//	        WorkerType: "benthos_flow",
//	        UserSpec:   UserSpec{Config: "input: {...}"},
//	        Enabled:    true,
//	    },
//	}
type ChildSpec struct {
	Dependencies map[string]any `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // Additional deps to merge with parent's deps (child overrides parent)
	UserSpec     UserSpec       `json:"userSpec"               yaml:"userSpec"`               // Raw user config (input to DeriveDesiredState)
	Name         string         `json:"name"                   yaml:"name"`                   // Unique name for this child (within parent scope)
	WorkerType   string         `json:"workerType"             yaml:"workerType"`             // Type of worker to create (registered worker factory key)

	// Enabled is the parent's per-tick request for this child to run. Zero
	// value (false) means "stopped but resident" — children stay in Stopped
	// without being despawned. Setting Enabled=false then Enabled=true resumes
	// the child on the next tick. Children read snap.IsDisabled, not Enabled
	// directly; the disable-mapping pass translates Enabled to IsDisabled.
	//
	// Caution: children are disabled by default. A ChildSpec constructed
	// without setting Enabled spawns a child that is held in Stopped, with
	// no validation error. Every child that should run must set Enabled
	// explicitly (NewChildSpec takes it as a required parameter).
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// Clone creates a deep copy of the ChildSpec.
// Note: Dependencies is shallow-copied (values are shared intentionally since they
// represent shared resources like channels and interfaces).
func (c ChildSpec) Clone() ChildSpec {
	clone := c

	clone.UserSpec = c.UserSpec.Clone()

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
// Enabled, and Dependencies (excluding any unexported fields).
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

	if c.Enabled {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}

	h.Write([]byte{0}) // separator

	// Dependencies contain runtime objects (channels, etc.) that can't be meaningfully hashed,
	// so we skip them. Changes to dependencies don't require re-validation anyway.

	return fmt.Sprintf("%016x", h.Sum64()), nil
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

// ChildrenView is the per-tick snapshot of a parent's children. Aggregate
// predicates (HealthyCount, UnhealthyCount, AllHealthy, AllOperational,
// AllStopped) are computed at construction so callers can read them as fields.
//
// To consume a ChildrenView in a worker:
//
// 1. Add fields to your ObservedState to store children info:
//
//			type MyObservedState struct {
//			    ChildrenHealthy   int
//			    ChildrenUnhealthy int
//			}
//
//	 2. Implement SetChildrenView on your ObservedState (the supervisor calls it
//	    automatically through the ChildrenViewConsumer capability interface):
//
//	    func (o MyObservedState) SetChildrenView(view config.ChildrenView) fsmv2.ObservedState {
//	    o.ChildrenHealthy = view.HealthyCount
//	    o.ChildrenUnhealthy = view.UnhealthyCount
//	    return o
//	    }
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

// NewChildrenView builds a ChildrenView from ChildInfo entries. A nil input
// slice is normalised to empty so CSE delta-sync produces a stable JSON shape
// across ticks.
func NewChildrenView(children []ChildInfo) ChildrenView {
	childrenCopy := make([]ChildInfo, len(children))
	copy(childrenCopy, children)

	view := ChildrenView{
		Children:       childrenCopy,
		AllHealthy:     true,
		AllOperational: true,
		AllStopped:     true,
	}

	for _, c := range childrenCopy {
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

// NewChildSpec creates a typed ChildSpec by marshalling cfg into UserSpec.Config (YAML).
// Returns an error if YAML marshalling fails (unexpected for static struct types).
// The Dependencies field is left at its zero value; callers that need it should
// set it on the returned spec.
//
// Use NewChildSpec to build typed child specs from a parent's RenderChildren body:
//
//	spec, err := config.NewChildSpec("push", "push", push.PushUserSpec{}, enabled)
//	if err != nil {
//	    return nil, err
//	}
func NewChildSpec[T any](name, workerType string, cfg T, enabled bool) (ChildSpec, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return ChildSpec{}, fmt.Errorf("NewChildSpec: marshal %q/%q: %w", name, workerType, err)
	}

	return ChildSpec{
		Name:       name,
		WorkerType: workerType,
		UserSpec:   UserSpec{Config: string(data)},
		Enabled:    enabled,
	}, nil
}
