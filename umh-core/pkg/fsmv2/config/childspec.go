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
// IsBeingRemoved() method and the RemovalRequestable interface's SetBeingRemoved() method.
//
// # Lifecycle Control Invariant
//
// The FSM controls worker lifecycle through state transitions, not through custom bool fields.
// Do not add fields like ShouldRun, IsRunning, Enabled, or Active to your DesiredState.
//
// Correct lifecycle control:
//   - BeingRemoved: Inherited from this type. Set by supervisor for graceful shutdown via
//     SetBeingRemoved; read via the IsBeingRemoved() method.
//     Parent-driven stop flows through the same flag: parent sets ChildSpec.Enabled=false,
//     which the CHANGE-19 reducer translates into BeingRemoved=true on the child.
//   - User-facing state ("running"/"stopped"): Read from TConfig via BaseUserSpec.GetState() in
//     state files. The wrapper-level BaseDesiredState does not carry a State field of its own.
//
// Correct ShouldBeRunning() implementations:
//
//	func (s *MyDesiredState) ShouldBeRunning() bool {
//	    return !s.IsBeingRemoved()
//	}
//
// Custom lifecycle fields are forbidden; the framework controls lifecycle
// exclusively via ShouldStop() and reconciliation.
type BaseDesiredState struct {
	BeingRemoved bool `json:"isBeingRemoved" yaml:"isBeingRemoved"` //nolint:tagliatelle // Match JSON field name for API compatibility
}

// SetBeingRemoved sets the removal-requested flag.
// This satisfies the RemovalRequestable interface.
func (b *BaseDesiredState) SetBeingRemoved(v bool) {
	b.BeingRemoved = v
}

// IsBeingRemoved returns the removal-requested flag.
//
// This is the canonical read accessor used by the DesiredState interface and
// by state files. The underlying field is BeingRemoved (no Is-prefix) to mirror
// the IsDisabled/Disabled pairing on WrappedDesiredState.
func (b *BaseDesiredState) IsBeingRemoved() bool {
	return b.BeingRemoved
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
// # Child Lifecycle
//
// Children are always-enabled when their parent is running. Deliberate disable
// goes through the Enabled=false reducer (CHANGE-19) — set Enabled: false on a
// child's spec to drive it to Stopped while keeping it resident.
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
//	    Enabled:    true,
//	}
//
// Example - Benthos managing connections and data flows:
//
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
	Dependencies map[string]any `json:"-"          yaml:"-"`          // Runtime channels and interfaces; not serializable. Inherited from parent at supervisor merge time (see ChildSpec.Hash godoc).
	UserSpec     UserSpec       `json:"userSpec"   yaml:"userSpec"`   // Raw user config (input to DeriveDesiredState)
	Name         string         `json:"name"       yaml:"name"`       // Unique name for this child (within parent scope)
	WorkerType   string         `json:"workerType" yaml:"workerType"` // Type of worker to create (registered worker factory key)

	// Enabled is the parent's per-tick enable signal for this child. The CHANGE-19
	// reducer translates it to IsBeingRemoved on the child's desired state every
	// tick. Three-state semantics:
	//
	//   - Name present, Enabled: true  → child runs (reducer writes IsBeingRemoved=false)
	//   - Name present, Enabled: false → stopped-but-resident: child reaches Stopped and stays
	//     there; supervisor remains resident in s.children, NOT placed in s.pendingRemoval
	//   - Name absent from spec list   → graceful despawn (full removal via pendingRemoval)
	//
	// The zero value (§4-C LOCKED) is false: ChildSpec literals that omit Enabled
	// produce a stopped-but-resident child. Parents that want a running child must
	// explicitly set Enabled: true in their renderChildren body.
	//
	// Pause/resume flow: setting Enabled=false drives the child to Stopped; setting
	// Enabled=true again writes IsBeingRemoved=false, and the child's state machine
	// transitions from Stopped back to TryingToStart on the next tick.
	//
	// One-way stop guarantee: once a child enters TryingToStop, it completes the stop
	// before accepting a new IsBeingRemoved=false signal. The supervisor only
	// manages the flag; the child's state machine enforces the trajectory.
	//
	// Children read snap.Desired.IsBeingRemoved() and never read Enabled directly
	// (Design Intent §4 no-Parent-references-in-children). Implemented by the CHANGE-19
	// reducer in supervisor/reconciliation.go (the spec→IsBeingRemoved translation
	// runs at the start of reconcileChildren, before Phase 1's pendingRemoval writes).
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// NewChildSpec creates a ChildSpec with Enabled:true set by default.
// Use this constructor in all RenderChildren implementations — it makes
// the F4⊕G1 trap (forgotten Enabled:false silently stopping children)
// impossible to trigger by accident.
func NewChildSpec(name, workerType string, userSpec UserSpec) ChildSpec {
	return ChildSpec{
		Name:       name,
		WorkerType: workerType,
		UserSpec:   userSpec,
		Enabled:    true,
	}
}

// DisableAll marks every ChildSpec in the slice as Enabled=false. Used by
// parent state machines that want children resident-but-shutdown via the
// CHANGE-19 reducer (e.g. parent in Stopped/Stopping/transient states).
// Returns the slice for chaining. Mutates in place.
//
// Pattern:
//
//	return fsmv2.Transition(..., config.DisableAll(pkg.RenderChildren(snap)))
//
// CHANGE-19 reducer (supervisor/reconciliation.go ~line 1660) translates
// Enabled=false into RequestShutdown synchronously before the child's tick,
// so children stay resident in supervisor.children with IsBeingRemoved
// set. Flipping back to Enabled=true issues ClearShutdownRequest for clean
// resume.
func DisableAll(specs []ChildSpec) []ChildSpec {
	for i := range specs {
		specs[i].Enabled = false
	}
	return specs
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
// The hash is computed from the comparable spec fields: Name, WorkerType,
// Enabled, and UserSpec. Dependencies are intentionally excluded — they hold
// runtime objects (channels, mutex-protected values) that cannot be
// meaningfully hashed and whose churn does not require re-validation.
// Unexported fields are also excluded.
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

	// Hash Enabled (single value byte: 1 for true, 0 for false) so flipping
	// the per-tick enable signal changes the hash and triggers re-validation
	// downstream. The trailing zero byte is the field separator (matching the
	// pattern above) — not the value, which is the byte already written. The
	// value byte and separator byte are distinct writes intentionally so that
	// Enabled=false (value 0) is still distinguishable from omission via the
	// position of the separator that always follows.
	if c.Enabled {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
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

	// Dependencies contain runtime objects (channels, etc.) that can't be meaningfully hashed,
	// so we skip them. Changes to dependencies don't require re-validation anyway.

	return fmt.Sprintf("%016x", h.Sum64()), nil
}

// ChildInfo provides a read-only snapshot of a child worker's current state.
// This is used by ChildrenView to give parent workers visibility into their children
// without allowing direct modification.
//
// All fields are copies, not references - modifying them has no effect on the actual child.
//
// JSON encoding uses camelCase. UnmarshalJSON also accepts the legacy
// PascalCase form for backward compatibility during the migration window.
type ChildInfo struct {
	Name          string         `json:"name"`          // Child name (unique within parent scope)
	WorkerType    string         `json:"workerType"`    // Child worker type
	StateName     string         `json:"stateName"`     // Current FSM state name (raw, e.g., "Running", "TryingToConnect" — display only)
	StateReason   string         `json:"stateReason"`   // Human-readable reason for current state
	ErrorMsg      string         `json:"errorMsg"`      // Error message if unhealthy (empty if healthy)
	HierarchyPath string         `json:"hierarchyPath"` // Full path in the worker hierarchy (e.g., "app.parent.child")
	Phase         LifecyclePhase `json:"phase"`         // Cached lifecycle phase populated by the supervisor; ChildrenView predicates read this rather than parsing StateName.
	IsHealthy     bool           `json:"isHealthy"`     // Whether the child is considered healthy
	// Infrastructure status fields (framework-tracked)
	IsStale       bool `json:"isStale"`       // True if observation age > stale threshold (~10s)
	IsCircuitOpen bool `json:"isCircuitOpen"` // True if infrastructure failure detected (circuit breaker open)
}

// UnmarshalJSON reads ChildInfo from both the canonical camelCase form and the
// legacy PascalCase form for migration compatibility.
//
// Strategy: decode into both auxiliary forms and merge — for each field, the
// camelCase value wins when it is non-zero; otherwise the PascalCase value
// fills in. This preserves legacy bool true-states (e.g., IsHealthy: true,
// IsStale: true) even when newer writers omit the field. Both forms eventually
// re-marshal to the canonical camelCase form.
//
// Phase has no PascalCase legacy companion: it was introduced together with
// the camelCase migration, so older payloads simply omit it (decodes to
// PhaseUnknown, which is the correct fall-through for unclassifiable rows).
// During a CSE migration window, snapshots written by a pre-Phase supervisor
// decode with Phase=PhaseUnknown for at most one reconcile interval — the next
// tick rebuilds ChildInfo from live child.GetLifecyclePhase() calls (see
// supervisor/children_view.go buildChildInfo) and the field is repopulated.
// Aggregate predicates therefore self-recover within a single tick (~10ms);
// no operator action is required.
func (c *ChildInfo) UnmarshalJSON(data []byte) error {
	var legacy struct {
		Name          string `json:"Name"`
		WorkerType    string `json:"WorkerType"`
		StateName     string `json:"StateName"`
		StateReason   string `json:"StateReason"`
		ErrorMsg      string `json:"ErrorMsg"`
		HierarchyPath string `json:"HierarchyPath"`
		IsHealthy     bool   `json:"IsHealthy"`
		IsStale       bool   `json:"IsStale"`
		IsCircuitOpen bool   `json:"IsCircuitOpen"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("ChildInfo: unmarshal legacy form: %w", err)
	}

	type childInfoCamel ChildInfo
	var camel childInfoCamel
	if err := json.Unmarshal(data, &camel); err != nil {
		return fmt.Errorf("ChildInfo: unmarshal camel form: %w", err)
	}

	if camel.Name == "" {
		camel.Name = legacy.Name
	}
	if camel.WorkerType == "" {
		camel.WorkerType = legacy.WorkerType
	}
	if camel.StateName == "" {
		camel.StateName = legacy.StateName
	}
	if camel.StateReason == "" {
		camel.StateReason = legacy.StateReason
	}
	if camel.ErrorMsg == "" {
		camel.ErrorMsg = legacy.ErrorMsg
	}
	if camel.HierarchyPath == "" {
		camel.HierarchyPath = legacy.HierarchyPath
	}
	if !camel.IsHealthy {
		camel.IsHealthy = legacy.IsHealthy
	}
	if !camel.IsStale {
		camel.IsStale = legacy.IsStale
	}
	if !camel.IsCircuitOpen {
		camel.IsCircuitOpen = legacy.IsCircuitOpen
	}

	*c = ChildInfo(camel)

	return nil
}

// ChildrenView is a serializable read-only snapshot of a parent worker's
// children at a single tick. It carries pre-computed aggregate counts and
// predicates so consumers do not need access to the live supervisor tree.
//
// The struct is pure data: JSON-serializable end-to-end (Design Intent §13)
// and round-trips through CSE storage between the collector goroutine and the
// reconciler goroutine without losing information.
//
// The supervisor builds ChildrenView via NewChildrenView and injects it via
// setter methods on ObservedState. To use it:
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
//	func (o MyObservedState) SetChildrenView(view config.ChildrenView) fsmv2.ObservedState {
//	    o.ChildrenHealthy = view.HealthyCount
//	    o.ChildrenUnhealthy = view.UnhealthyCount
//	    // Or use view.List(), view.Get(name), view.AllHealthy, view.AllStopped
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
// See fsmv2/observation.go (Observation.SetChildrenCounts and the embedded
// ChildrenHealthy/ChildrenUnhealthy fields) for the framework-supplied counts pattern.
type ChildrenView struct {
	// Children carries the per-child snapshots in deterministic order.
	Children []ChildInfo `json:"children"`
	// HealthyCount is the number of children in PhaseRunningHealthy.
	HealthyCount int `json:"healthyCount"`
	// UnhealthyCount is the number of children that are neither healthy nor stopped.
	// Includes PhaseUnknown, PhaseStarting, PhaseRunningDegraded, PhaseStopping.
	UnhealthyCount int `json:"unhealthyCount"`
	// AllHealthy is true when every child is PhaseRunningHealthy, or there are
	// no children.
	AllHealthy bool `json:"allHealthy"`
	// AllOperational is true when every child is PhaseRunningHealthy or
	// PhaseRunningDegraded, or there are no children.
	AllOperational bool `json:"allOperational"`
	// AllStopped is true when every child is PhaseStopped, or there are no
	// children.
	AllStopped bool `json:"allStopped"`
}

// NewChildrenView builds a ChildrenView from a slice of ChildInfo entries.
//
// Aggregate predicate rules:
//   - AllHealthy: empty slice yields true; otherwise true iff every child has
//     Phase == PhaseRunningHealthy.
//   - AllOperational: empty slice yields true; otherwise true iff every child
//     has Phase ∈ {PhaseRunningHealthy, PhaseRunningDegraded}.
//   - AllStopped: empty slice yields true; otherwise true iff every child has
//     Phase == PhaseStopped.
//   - HealthyCount: number of children with Phase == PhaseRunningHealthy.
//   - UnhealthyCount: number of children with Phase ∈ {PhaseUnknown,
//     PhaseStarting, PhaseRunningDegraded, PhaseStopping}. PhaseStopped
//     children count as neither healthy nor unhealthy.
//
// Predicates read the cached Phase field on each ChildInfo rather than parsing
// StateName, because StateName carries raw worker state names like "Connected"
// or "TryingToConnect" that the prefix-based ParseLifecyclePhase cannot
// classify. The supervisor populates Phase from child.GetLifecyclePhase() in
// buildChildInfo. NewChildrenView normalises a nil children slice to an empty
// slice so JSON encoding emits "children":[] instead of "children":null,
// keeping CSE delta-sync stable across ticks.
func NewChildrenView(children []ChildInfo) ChildrenView {
	// Normalise nil to an empty slice into a separate local so the input
	// parameter is not shadowed; callers that passed nil keep their value
	// unchanged in the caller's scope.
	normalised := children
	if normalised == nil {
		normalised = []ChildInfo{}
	}

	v := ChildrenView{
		Children: normalised,
	}

	allHealthy := true
	allOperational := true
	allStopped := true

	for i := range normalised {
		phase := normalised[i].Phase

		if phase.IsHealthy() {
			v.HealthyCount++
		} else if !phase.IsStopped() {
			// Everything except healthy and stopped is unhealthy.
			// Includes PhaseUnknown, PhaseStarting, PhaseRunningDegraded, PhaseStopping.
			v.UnhealthyCount++
		}

		if !phase.IsHealthy() {
			allHealthy = false
		}
		if !phase.IsOperational() {
			allOperational = false
		}
		if !phase.IsStopped() {
			allStopped = false
		}
	}

	v.AllHealthy = allHealthy
	v.AllOperational = allOperational
	v.AllStopped = allStopped

	return v
}

// List returns the child snapshots. The returned slice should be treated as
// read-only; mutations are not observed by the supervisor.
//
// Deprecated: prefer reading the Children field directly. This wrapper is
// retained for migration compatibility with the prior interface API.
func (v ChildrenView) List() []ChildInfo {
	return v.Children
}

// Get returns a copy of the ChildInfo for the given child name. The boolean is
// false when no child with that name exists in this view.
func (v ChildrenView) Get(name string) (ChildInfo, bool) {
	for i := range v.Children {
		if v.Children[i].Name == name {
			return v.Children[i], true
		}
	}

	return ChildInfo{}, false
}

// Counts returns the pre-computed healthy / unhealthy counts.
//
// Deprecated: read HealthyCount / UnhealthyCount fields directly. Retained for
// migration compatibility with code that previously called Counts() on the
// ChildrenView interface.
func (v ChildrenView) Counts() (healthy, unhealthy int) {
	return v.HealthyCount, v.UnhealthyCount
}

// DesiredState represents what we want the system to be.
// This is returned by Worker.DeriveDesiredState() and used by State.Next() for decisions.
//
// The supervisor injects shutdown requests by setting IsBeingRemoved = true.
// Workers must check IsBeingRemoved first in their State.Next() implementations.
//
// # Children Management
//
// The ChildrenSpecs field enables declarative child management. Parent workers populate
// this to declare what children should exist. The supervisor handles all lifecycle:
//
//	// In parent's DeriveDesiredState():
//	func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    return config.DesiredState{
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
//	    BaseDesiredState: BaseDesiredState{BeingRemoved: true},  // Triggers shutdown sequence
//	    ChildrenSpecs:    nil,                                        // Children removed during shutdown
//	}
type DesiredState struct {
	OriginalUserSpec interface{}      `json:"-"                          yaml:"-"` // Captures the input that produced this DesiredState (for debugging/traceability); not serialized (would emit untyped any over the wire per §17).
	BaseDesiredState `yaml:",inline"` // Provides BeingRemoved field and methods (IsBeingRemoved, SetBeingRemoved)
	ChildrenSpecs    []ChildSpec      `json:"childrenSpecs,omitempty"    yaml:"childrenSpecs,omitempty"` // Declarative specification of child workers
}

// NOTE: IsBeingRemoved() and SetBeingRemoved() are provided by embedded BaseDesiredState.
// The BeingRemoved field is the canonical source of truth for shutdown state.
//
// Shutdown flow:
//  1. Supervisor sets BeingRemoved = true via SetBeingRemoved()
//  2. State.Next() calls IsBeingRemoved() → returns true
//  3. State transitions to shutdown/cleanup states
//  4. Eventually returns SignalNeedsRemoval
//  5. Supervisor removes worker from system
//
// Example usage in State.Next():
//
//	func (s RunningState) Next(snapshot fsmv2.Snapshot) (State, Signal, Action) {
//	    desired := snapshot.Desired.(types.DesiredState)
//	    // Always check shutdown first
//	    if desired.IsBeingRemoved() {
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

// NOTE: User-facing lifecycle state ("running"/"stopped") lives on TConfig via embedded
// BaseUserSpec.State, not on BaseDesiredState. State files read it via Config.GetState().
