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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// Signal communicates special conditions from states to the supervisor.
type Signal int

const (
	// SignalNone indicates normal operation, no special action needed.
	SignalNone Signal = iota

	// SignalNeedsRemoval tells supervisor this worker has completed cleanup and can be removed.
	// Emitted by Stopped state when IsShutdownRequested() returns true and cleanup is complete.
	SignalNeedsRemoval

	// SignalNeedsRestart tells supervisor the worker has detected an unrecoverable error
	// and needs a full restart. The supervisor will:
	//   1. Call SetShutdownRequested(true) (trigger graceful shutdown)
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
	// Pseudo-code example:
	//
	//   func (s *TryingToConnectState) Next(snap MySnapshot) (State, Signal, Action) {
	//       if snap.Observed.ConsecutiveFailures > 100 {
	//           return s, fsmv2.SignalNeedsRestart, nil
	//       }
	//       return s, fsmv2.SignalNone, &ConnectAction{}
	//   }
	//
	// Note: Due to Go's lack of covariance, actual implementations use State[any, any].
	// See doc.go "Immutability" section for the actual implementation pattern.
	SignalNeedsRestart
)

// ObservedState represents the actual state gathered from monitoring the system.
type ObservedState interface {
	// GetObservedDesiredState returns the desired state that is actually deployed.
	GetObservedDesiredState() DesiredState

	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}

// TimestampProvider allows access to observation timestamps for staleness checks.
type TimestampProvider interface {
	GetTimestamp() time.Time
}

// DesiredState represents the target state derived from user configuration.
type DesiredState interface {
	// IsShutdownRequested is set by supervisor to initiate graceful shutdown.
	// States should check this first in their Next() method.
	IsShutdownRequested() bool

	// GetState returns the desired lifecycle state ("running", "stopped", etc.).
	// Used by supervisor to validate state values after DeriveDesiredState.
	GetState() string
}

// ShutdownRequestable allows setting the shutdown flag on any DesiredState.
// Embed config.BaseDesiredState (from pkg/fsmv2/config) to satisfy this interface.
type ShutdownRequestable interface {
	SetShutdownRequested(bool)
}

// Snapshot is the complete view of the worker at a point in time (immutable).
type Snapshot struct {
	Observed interface{}   // What is the actual state? (ObservedState or basic.Document).
	Desired  interface{}   // What should the state be? (DesiredState or basic.Document).
	Identity deps.Identity // Who am I?
}

// Action represents an idempotent side effect that modifies external system state.
// Actions must be idempotent; failed actions leave state unchanged for retry.
type Action[TDeps any] interface {
	// Execute performs the action. Must handle context cancellation.
	Execute(ctx context.Context, deps TDeps) error
	// Name returns a descriptive name for logging/debugging.
	Name() string
}

// NextResult contains the result of a State.Next() evaluation.
// All fields except State are optional - use helpers.Result() to construct.
type NextResult[TSnapshot any, TDeps any] struct {
	// State is the next state (can be same state if no transition).
	State State[TSnapshot, TDeps]

	// Action is an optional action to execute (nil if none).
	Action Action[TDeps]

	// Reason is a human-readable explanation of the current state.
	// REQUIRED - describes WHY we're in this state.
	// Can include dynamic data from the snapshot.
	// Example: "sync degraded: 5 consecutive errors (authentication_failure)"
	Reason string

	// Signal indicates framework-level events (shutdown, restart, etc.).
	Signal Signal
}

// State represents a single state in the FSM lifecycle (stateless).
type State[TSnapshot any, TDeps any] interface {
	// Next evaluates the snapshot and returns the next transition.
	// Pure function called on each tick. The supervisor passes the snapshot by value (immutable).
	// Returns a NextResult containing the next state, signal, optional action, and reason.
	// The reason is REQUIRED and should explain WHY we're in/transitioning to this state.
	Next(snapshot TSnapshot) NextResult[TSnapshot, TDeps]

	// String returns the state name for logging/debugging.
	String() string

	// LifecyclePhase returns the lifecycle phase of this state.
	// Used by parent supervisors to classify child health without knowing
	// implementation details of the child's state machine.
	//
	// The supervisor uses this to:
	//   - Construct the observed state name: phase.Prefix() + lowercase(String())
	//   - Classify child health: phase.IsHealthy(), phase.IsOperational()
	//   - Enable ChildrenManager methods: AllHealthy(), Counts()
	//
	// Lifecycle phases:
	//   - PhaseStopped:         stopped                → neutral health
	//   - PhaseStarting:        starting_*             → unhealthy
	//   - PhaseRunningHealthy:  running_healthy_*      → HEALTHY
	//   - PhaseRunningDegraded: running_degraded_*     → unhealthy (but operational)
	//   - PhaseStopping:        stopping_*             → unhealthy
	//   - PhaseUnknown:         unknown_*              → unhealthy
	LifecyclePhase() config.LifecyclePhase
}

// Result creates a NextResult with the given components.
// This is a convenience function to reduce boilerplate when returning from Next().
//
// Usage:
//
//	return fsmv2.Result(s, fsmv2.SignalNone, nil, "Worker is stopped")
//
//	reason := fmt.Sprintf("degraded: %d errors (%s)", errors, errorType)
//	return fsmv2.Result(&DegradedState{}, fsmv2.SignalNone, nil, reason)
func Result[TSnapshot any, TDeps any](
	state State[TSnapshot, TDeps],
	signal Signal,
	action Action[TDeps],
	reason string,
) NextResult[TSnapshot, TDeps] {
	return NextResult[TSnapshot, TDeps]{
		State:  state,
		Signal: signal,
		Action: action,
		Reason: reason,
	}
}

// Worker is the business logic interface that developers implement.
// Note: Shutdown is managed by the supervisor via ShutdownRequested in desired state,
// not by a method on this interface.
type Worker interface {
	// CollectObservedState monitors the actual system state.
	CollectObservedState(ctx context.Context) (ObservedState, error)

	// DeriveDesiredState derives the target state from user configuration (spec).
	DeriveDesiredState(spec interface{}) (DesiredState, error)

	// GetInitialState returns the starting state for this worker.
	// Called once during worker creation.
	GetInitialState() State[any, any]
}

// DependencyProvider exposes worker dependencies for action execution.
// Workers that embed helpers.BaseWorker automatically satisfy this interface.
type DependencyProvider interface {
	// GetDependenciesAny returns the worker's dependencies as any.
	GetDependenciesAny() any
}
