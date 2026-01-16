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
	// Emitted by Stopped state when ShutdownRequested=true and cleanup is complete.
	SignalNeedsRemoval

	// SignalNeedsRestart tells supervisor the worker has detected an unrecoverable error
	// and needs a full restart. The supervisor will:
	//   1. Set ShutdownRequested=true (trigger graceful shutdown)
	//   2. Wait for worker to complete shutdown and emit SignalNeedsRemoval
	//   3. Reset worker to initial state instead of removing it
	//   4. Restart the observation collector
	//
	// If graceful shutdown takes too long (>30s), the supervisor force-resets the worker.
	//
	// Use this when:
	//   - Action failures indicate permanent misconfiguration
	//   - Worker state is corrupted and needs a fresh start
	//   - External resource needs reconnection from scratch
	//
	// Example in state:
	//
	//   func (s *TryingToConnectState) Next(snap MySnapshot) (State, Signal, Action) {
	//       if snap.Observed.ConsecutiveFailures > 100 {
	//           return s, fsmv2.SignalNeedsRestart, nil
	//       }
	//       return s, fsmv2.SignalNone, &ConnectAction{}
	//   }
	SignalNeedsRestart
)

// Identity uniquely identifies a worker instance.
// This is immutable for the lifetime of the worker.
type Identity struct {
	ID            string `json:"id"`            // Unique identifier (e.g., UUID)
	Name          string `json:"name"`          // Human-readable name
	WorkerType    string `json:"workerType"`    // Type of worker (e.g., "container", "pod")
	HierarchyPath string `json:"hierarchyPath"` // Full path from root: "scenario123(application)/parent-123(parent)/child001(child)"
}

// ObservedState represents the actual state gathered from monitoring the system.
// Implementations should include timestamps to detect staleness.
// The supervisor collects this via CollectObservedState() in a separate goroutine.
type ObservedState interface {
	// GetObservedDesiredState returns the desired state that is actually deployed.
	// Comparing what's deployed vs what we want to deploy.
	// It is required to enforce that everything we configure should also be read back to double-check it.
	GetObservedDesiredState() DesiredState

	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}

// DesiredState represents what we want the system to be.
// Derived from user configuration via DeriveDesiredState().
//
// INVARIANT: DesiredState must NEVER contain Dependencies (runtime interfaces).
// Pass dependencies to Action.Execute() instead.
type DesiredState interface {
	// IsShutdownRequested is set by supervisor to initiate graceful shutdown.
	// States MUST check this first in their Next() method.
	IsShutdownRequested() bool
}

// ShutdownRequestable allows setting the shutdown flag on any DesiredState.
// All DesiredState types should embed config.BaseDesiredState to satisfy this interface.
// Type-safe shutdown request propagation from supervisor to workers.
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
// Passed by value to State.Next(), making it inherently immutable.
// Use helpers.ConvertSnapshot[O, D](snapAny) for type-safe field access.
type Snapshot struct {
	Identity Identity    // Who am I?
	Observed interface{} // What is the actual state? (ObservedState or basic.Document)
	Desired  interface{} // What should the state be? (DesiredState or basic.Document)
}

// Action represents a side effect that transitions the system between states.
// Executed by the supervisor after State.Next() returns them.
// Retried with exponential backoff on failure.
//
// REQUIREMENT: Actions MUST be idempotent (safe to call multiple times).
// Check if work is already done before performing it.
type Action[TDeps any] interface {
	// Execute performs the action. Must handle context cancellation.
	Execute(ctx context.Context, deps TDeps) error
	// Name returns a descriptive name for logging/debugging.
	Name() string
}

// State represents a single state in the FSM lifecycle.
// States are stateless - they examine the snapshot and decide what happens next.
// State.Next() must be a pure function: no side effects, no external calls.
//
// Key rules:
//   - Check ShutdownRequested first in Next()
//   - Return action OR state change, not both (supervisor panics otherwise)
//   - "TryingTo" prefix = active state emitting actions; nouns = passive observation
type State[TSnapshot any, TDeps any] interface {
	// Next evaluates the snapshot and returns the next transition.
	// Pure function called on each tick. Snapshot is passed by value (immutable).
	// Returns: nextState, signal to supervisor, optional action to execute.
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
type Worker interface {
	// CollectObservedState monitors the actual system state.
	// Called in a separate goroutine with timeout protection.
	// Must respect context cancellation. Errors are logged but don't stop the FSM.
	CollectObservedState(ctx context.Context) (ObservedState, error)

	// DeriveDesiredState derives the target state from user configuration (spec).
	// This is a derivation, not a copy - it parses, validates, and computes derived fields.
	// Pure function - no side effects. Called on each tick.
	DeriveDesiredState(spec interface{}) (config.DesiredState, error)

	// GetInitialState returns the starting state for this worker.
	// Called once during worker creation.
	GetInitialState() State[any, any]

	// Shutdown is managed by the supervisor via ShutdownRequested in desired state.
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

