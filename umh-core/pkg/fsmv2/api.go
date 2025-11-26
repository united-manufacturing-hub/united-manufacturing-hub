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

package fsmv2

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// Signal is used by states to communicate special conditions to the supervisor.
// These signals trigger supervisor-level actions beyond normal state transitions.
type Signal int

const (
	// SignalNone indicates normal operation, no special action needed.
	SignalNone Signal = iota
	// SignalNeedsRemoval tells supervisor this worker has completed cleanup and can be removed.
	SignalNeedsRemoval
	// SignalNeedsRestart tells supervisor to initiate shutdown for a restart cycle.
	SignalNeedsRestart
)

// Identity uniquely identifies a worker instance.
// This is immutable for the lifetime of the worker.
type Identity struct {
	ID         string `json:"id"`         // Unique identifier (e.g., UUID)
	Name       string `json:"name"`       // Human-readable name
	WorkerType string `json:"workerType"` // Type of worker (e.g., "container", "pod")
}

// ObservedState represents the actual state gathered from monitoring the system.
// Implementations should include timestamps to detect staleness.
// The supervisor collects this via CollectObservedState() in a separate goroutine.
type ObservedState interface {
	// GetObservedDesiredState returns the desired state that is actually deployed.
	// This allows comparing what's deployed vs what we want to deploy.
	// It is required to enforce that everything we configure should also be read back to double-check it.
	GetObservedDesiredState() DesiredState

	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}

// DesiredState represents what we want the system to be.
// Derived from user configuration via DeriveDesiredState().
// The supervisor can inject shutdown requests here.
type DesiredState interface {
	// IsShutdownRequested is set by supervisor to initiate graceful shutdown.
	// States MUST check this first in their Next() method.
	IsShutdownRequested() bool
}

// ShutdownRequestable allows setting the shutdown flag on any DesiredState.
// All DesiredState types should embed helpers.BaseDesiredState to satisfy this interface.
// This enables type-safe shutdown request propagation from supervisor to workers.
//
// Example usage:
//
//	if sr, ok := any(desired).(fsmv2.ShutdownRequestable); ok {
//	    sr.SetShutdownRequested(true)
//	}
type ShutdownRequestable interface {
	SetShutdownRequested(bool)
}

// Snapshot is the complete view of the worker at a point in time.
// The supervisor assembles this from the database and passes it to State.Next().
// This enables pure functional state transitions based on complete information.
//
// IMMUTABILITY (Invariant I9):
// Snapshot is passed by value to State.Next(), making it inherently immutable.
// States receive a COPY of the snapshot, so mutations don't affect the original.
// This guarantees that state transitions are pure functions without side effects.
//
// We do NOT use getters because:
//  1. Pass-by-value makes mutation impossible (copies on assignment)
//  2. Getters add boilerplate without adding safety
//  3. Go convention favors simple field access over accessors (see time.Time, net.IP)
//
// This is the idiomatic Go approach: Use value semantics for immutability,
// not OOP patterns like getters/setters.
//
// Go's pass-by-value semantics enforce this at the language level:
//   - When State.Next(snapshot Snapshot) is called, Go copies the struct
//   - Fields (Identity, Observed, Desired) are copied as interface pointers
//   - States can mutate their local copy without affecting supervisor's snapshot
//   - No runtime validation needed - the compiler enforces this
//
// Example showing immutability in practice:
//
//	func (s MyState) Next(snapshot Snapshot) (State, Signal, Action) {
//	    // snapshot is a copy - mutations here don't affect supervisor's snapshot
//	    snapshot.Observed = nil  // This only affects the local copy
//	    snapshot.Identity.Name = "modified"  // Local copy only
//	    return s, SignalNone, nil
//	}
//
// Defense-in-depth layers:
//   - Layer 1: Pass-by-value (Go language design)
//   - Layer 2: Documentation (this godoc)
//   - Layer 3: Tests demonstrating immutability (supervisor/immutability_test.go)
type Snapshot struct {
	Identity Identity    // Who am I?
	Observed interface{} // What is the actual state? (ObservedState or basic.Document)
	Desired  interface{} // What should the state be? (DesiredState or basic.Document)
}

// Action represents a side effect that transitions the system between states.
// Actions are executed by the supervisor after State.Next() returns them.
// They can be long-running and will be retried with backoff on failure.
// Actions MUST be idempotent - safe to retry after partial completion.
//
// IDEMPOTENCY REQUIREMENT (Invariant I10):
// Actions MUST be safe to call multiple times. Each action implementation should:
//  1. Check if work is already done before performing it
//  2. Produce the same final state whether called once or multiple times
//  3. Handle partial completion gracefully (retry from checkpoint)
//
// Example idempotent action (empty struct, deps injected):
//
//	func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
//	    // ALWAYS check context cancellation first
//	    select {
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    default:
//	    }
//	    deps := depsAny.(MyDependencies)
//	    // Check if already done (idempotency)
//	    if deps.IsConnected() {
//	        return nil  // Already connected, idempotent
//	    }
//	    return deps.Connect(ctx)
//	}
//
// Example NON-idempotent action (DO NOT DO THIS):
//
//	func (a *IncrementCounterAction) Execute(ctx context.Context, depsAny any) error {
//	    deps := depsAny.(MyDependencies)
//	    deps.IncrementCounter()  // WRONG! Multiple calls increment multiple times
//	    return nil
//	}
//
// Testing idempotency:
// Use the idempotency test helper in supervisor/action_helpers_test.go:
//
//	VerifyActionIdempotency(action, 3, func() {
//	    Expect(fileExists("test.txt")).To(BeTrue())
//	})
//
// REQUIREMENT (FSM v2): Every Action implementation MUST have an idempotency test.
// Code reviewers: Check that action_*_test.go files use VerifyActionIdempotency.
//
// Defense-in-depth layers:
//   - Layer 1: Document requirement in Action interface
//   - Layer 2: Provide test helpers for verification
//   - Layer 3: Examples showing idempotent patterns
//   - Layer 4: ActionExecutor with exponential backoff validates this
//
// Example: StartProcess, StopProcess, CreateConfigFiles, CallAPI.
type Action[TDeps any] interface {
	// Execute performs the action. Can be blocking and long-running.
	// Must handle context cancellation. Must be idempotent.
	// Dependencies are injected via deps parameter at execution time.
	Execute(ctx context.Context, deps TDeps) error
	// Name returns a descriptive name for logging/debugging
	Name() string
}

// State represents a single state in the FSM lifecycle.
// Each state encapsulates the decision logic for transitions.
// States are stateless - they examine the snapshot and decide what happens next.
//
// Key principles:
//   - States MUST handle ShutdownRequested first
//   - State transitions are explicit and visible in code
//   - States return actions for side effects, not perform them
//
// State Naming and Behavior Convention:
//
// ACTIVE STATES (prefix: "TryingTo")
//   - Emit actions on every tick until success condition met
//   - Examples: TryingToStartState, TryingToStopState
//   - Represent ongoing operations that need retrying
//
// PASSIVE STATES (descriptive nouns)
//   - Only observe and transition based on conditions
//   - Examples: RunningState, StoppedState, DegradedState
//   - Represent stable conditions where no action needed
//
// TODO: Consider previous state tracking in Snapshot for debugging
// TODO: Clarify naming to avoid confusion between "trying" vs "confirming"
//
//	(e.g., ConfirmingStartState vs TryingToStartState)
//	Option: Use "Ensuring" pattern - EnsuringStartedState, EnsuringStoppedState
//	This captures both action and verification in one word (doing + confirming)
//
// Example implementation:
//
//	func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
//	    // Always check shutdown first
//	    if snapshot.Desired.ShutdownRequested() {
//	        return StoppingState{}, SignalNone, nil
//	    }
//	    // Check if reconfiguration needed
//	    if snapshot.Observed != snapshot.Desired {
//	        return ReconfiguringState{}, SignalNone, nil
//	    }
//	    // Stay in current state
//	    return s, SignalNone, nil
//	}
type State[TSnapshot any, TDeps any] interface {
	// Next evaluates the snapshot and returns the next transition.
	// This is a pure function - no side effects, no external calls.
	// The supervisor calls this on each tick (e.g., every second).
	//
	// IMMUTABILITY (Invariant I9):
	// The snapshot parameter is passed by value (copied), so any modifications
	// to it within Next() do not affect the supervisor's original snapshot.
	// This enforces immutability and enables pure functional transitions.
	//
	// Go's pass-by-value semantics guarantee:
	//   - snapshot is a COPY of the supervisor's snapshot
	//   - Mutations to snapshot only affect this local copy
	//   - The supervisor's snapshot remains unchanged
	//   - No defensive copying or validation needed
	//
	// TYPE SAFETY (Invariant I11):
	// The snapshot parameter uses the concrete TSnapshot type, eliminating
	// runtime type assertions. The compiler enforces type safety at compile time.
	// States receive correctly typed snapshots without manual casting.
	//
	// Returns:
	//   - nextState: State to transition to (can return self to stay)
	//   - signal: Optional signal to supervisor (usually SignalNone)
	//   - action: Optional action to execute before next tick (can be nil)
	//
	// Only returns new state when all conditions are met.
	// Should not switch the state and emit an action at the same time (supervisor should check for this and panic if this happens as this is an application logic issue).
	//
	// Supervisor will only call Next() if there is no ongoing action (to prevent multiple actions).
	//
	// Supervisor flow after calling Next():
	//   1. If action != nil: execute it (with retries/backoff on error)
	//   2. Transition to nextState
	//   3. Process signal (e.g., remove worker if SignalNeedsRemoval)
	//   4. Wait for next tick
	Next(snapshot TSnapshot) (State[TSnapshot, TDeps], Signal, Action[TDeps])

	// String returns the state name for logging/debugging
	String() string

	// Reason is the reason for the current state and gives more background information
	// For degraded state it could give exact information on what is degraded
	// For starting states, it could report the "sub-states",
	// so benthos could report the reason that it is starting is that S6 is not yet started
	Reason() string
}

// Worker is the business logic interface that developers implement.
// The supervisor manages the worker lifecycle using these methods.
//
// Interaction flow:
//
//  1. Supervisor creates worker and calls GetInitialState()
//
//  2. Supervisor starts goroutine calling CollectObservedState() in a loop
//
//  3. On each tick:
//     - Supervisor calls DeriveDesiredState() with latest config
//     - Supervisor reads latest ObservedState from DB (collected in step 2)
//     - Supervisor calls currentState.Next() with the snapshot
//     - Supervisor sets currentState to whatever currentState.Next() returns
//     - Supervisor executes any returned action
//
//  4. On shutdown: Supervisor sets ShutdownRequested in desired state
//
//  5. Worker states handle shutdown, eventually returning SignalNeedsRemoval
//
//  6. Supervisor removes worker from system
type Worker interface {
	// CollectObservedState monitors the actual system state.
	// Called in a separate goroutine with timeout protection.
	//
	// Context Handling (Invariant I6):
	//   - MUST respect context cancellation within grace period (5 seconds)
	//   - Failure to exit after context cancellation will cause panic
	//   - This enforces proper async operation lifecycle management
	//
	// Timeout Protection:
	//   - Wrapped with per-operation timeout (observation interval + cgroup buffer + margin)
	//   - Default: 1s interval + 200ms cgroup throttle + 1s margin = 2.2s timeout
	//   - Accounts for Docker/Kubernetes CPU throttling (100ms cgroup period)
	//   - Operations exceeding timeout are cancelled automatically
	//
	// Error Handling:
	//   - Errors are logged but don't stop the FSM
	//   - Supervisor handles staleness via FreshnessChecker
	//   - Repeated timeouts trigger collector restart with backoff
	//
	// Upgrade Notice from fsm: this function replaces the whole `_monitor` logic
	//
	// Example: Poll process status, check file existence, query APIs
	CollectObservedState(ctx context.Context) (ObservedState, error)

	// DeriveDesiredState transforms user configuration into desired state.
	// Pure function - no side effects. Called on each tick.
	// The spec parameter comes from user configuration.
	//
	// This is used for templating, for example to convert user configuration to the actual "technical" template.
	//
	// Returns concrete config.DesiredState to enable hierarchical composition via ChildrenSpecs field.
	// Parent workers can declare child FSM workers by populating ChildrenSpecs, allowing supervisor
	// to reconcile actual children to match desired specs (Kubernetes-style declarative management).
	//
	// Example: Parse YAML config, apply templates, validate settings
	DeriveDesiredState(spec interface{}) (config.DesiredState, error)

	// GetInitialState returns the starting state for this worker.
	// Called once during worker creation.
	//
	// Returns State[any, any] to allow supervisor to work with heterogeneous worker types.
	// Concrete workers return their specific typed states which satisfy this interface.
	//
	// Example: return &InitializingState{} or &StoppedState{}
	GetInitialState() State[any, any]

	// RequestShutdown sets the ShutdownRequested flag on the worker's desired state.
	// Called by supervisor to initiate graceful shutdown.
	// The worker is responsible for propagating this to its internal desired state.
	RequestShutdown()
}

// DependencyProvider is an optional interface that workers can implement
// to expose their dependencies for action execution.
// Workers that embed helpers.BaseWorker automatically satisfy this interface.
//
// Example usage:
//
//	if provider, ok := worker.(DependencyProvider); ok {
//	    deps := provider.GetDependenciesAny()
//	    action.Execute(ctx, deps)
//	}
type DependencyProvider interface {
	// GetDependenciesAny returns the worker's dependencies as any.
	// This is used by the ActionExecutor to pass dependencies to actions.
	GetDependenciesAny() any
}
