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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ErrNoDesiredState signals that no desired state is available yet.
// This is an expected condition on first boot before the supervisor has written
// the initial desired state to CSE storage, distinguishing it from genuine
// load failures (e.g., deserialization errors, store connectivity issues).
// The DesiredStateProvider in supervisor/api.go returns (nil, ErrNoDesiredState)
// when persistence.ErrNotFound is encountered, signaling the collector to skip
// the collection cycle without Sentry noise.
var ErrNoDesiredState = errors.New("fsmv2: no desired state available")

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
// Intentional pair with TimestampProvider: ObservedState is the compile-time
// return-type contract for CollectObservedState, while TimestampProvider is the
// runtime capability the supervisor asserts against Snapshot.Observed (typed
// `any` — see line above). Identical method sets today, different roles.
//
// Invariant: TimestampProvider must remain a strict subset of ObservedState's
// methods so the runtime assertion never misses a value the compile-time
// contract accepted. The compile-time check below enforces this.
//
//nolint:iface // see godoc above — paired contract/capability interfaces.
type ObservedState interface {
	// GetTimestamp returns the time when this observed state was collected,
	// used for staleness checks.
	GetTimestamp() time.Time
}

// TimestampProvider allows access to observation timestamps for staleness checks.
//
//nolint:iface // see ObservedState above — paired contract/capability interfaces.
type TimestampProvider interface {
	GetTimestamp() time.Time
}

// Compile-time invariant: every ObservedState satisfies TimestampProvider.
// If ObservedState ever grows methods, TimestampProvider stays a strict subset
// so the runtime assertions in supervisor stay correct.
var _ TimestampProvider = (ObservedState)(nil)

// DesiredState represents the target state derived from user configuration.
type DesiredState interface {
	// IsShutdownRequested is set by supervisor to initiate graceful shutdown.
	// States should check this first in their Next() method.
	IsShutdownRequested() bool
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
	// The desired parameter provides the current desired state so observation-based
	// workers can access configuration (target IP, port, etc.) without workarounds.
	// The supervisor guarantees desired is always non-nil; collection is skipped
	// until a desired state exists in the store.
	CollectObservedState(ctx context.Context, desired DesiredState) (ObservedState, error)

	// DeriveDesiredState derives the target state from user configuration (spec).
	DeriveDesiredState(spec interface{}) (DesiredState, error)

	// GetInitialState returns the starting state for this worker.
	// Called once during worker creation.
	GetInitialState() State[any, any]
}

// DependencyProvider exposes worker dependencies for action execution.
// Workers that embed fsmv2.WorkerBase automatically satisfy this interface.
type DependencyProvider interface {
	// GetDependenciesAny returns the worker's dependencies as any.
	GetDependenciesAny() any
}

// BaseUserSpec is satisfied by config types that embed config.BaseUserSpec.
// WorkerBase.DeriveDesiredState uses this interface to read and validate the
// desired lifecycle state ("running" or "stopped") from TConfig, then propagate
// it into WrappedDesiredState.State.
type BaseUserSpec interface {
	GetState() string
}

// --- Capability interfaces (optional, discovered via type assertion) ---

// ActionProvider enables side effects via actions.
// Workers that implement this interface opt into the action execution pipeline.
// The supervisor calls Actions() once at registration to discover available actions.
type ActionProvider interface {
	Actions() map[string]Action[any]
}

// MetricsProvider enables custom Prometheus metrics.
// Workers that implement this interface register custom collectors
// with Prometheus at registration time.
type MetricsProvider interface {
	Metrics() []prometheus.Collector
}

// GracefulShutdowner enables custom cleanup on shutdown.
// Workers that implement this interface get a chance to flush buffers,
// close connections, etc. before the supervisor removes the worker.
type GracefulShutdowner interface {
	Shutdown(ctx context.Context) error
}

// ChildrenViewConsumer enables access to the full child state tree.
// Workers that implement this interface receive the complete children
// supervisor view each tick, enabling extraction of circuit breaker state,
// stale counts, and other detailed child information beyond aggregate counts.
//
// SetChildrenView returns the updated ObservedState because it is invoked on
// value receivers (the collector treats ObservedState as immutable and
// re-assigns the returned value), matching every other Set* method on
// Observation.
type ChildrenViewConsumer interface {
	SetChildrenView(view config.ChildrenView) ObservedState
}

// --- WrappedDesiredState ---

// WrappedDesiredState wraps a developer's TConfig into the full DesiredState
// required by the supervisor. BaseDesiredState promotion provides
// IsShutdownRequested and SetShutdownRequested for free. The State field
// carries the desired lifecycle state ("running"/"stopped") set by
// DeriveDesiredState from the user spec's BaseUserSpec.GetState().
//
// The framework constructs this during DeriveDesiredState. Developers define
// their TConfig type and call the typed DeriveDesiredState helpers to produce it.
type WrappedDesiredState[TConfig any] struct {
	Config        TConfig            `json:"config"`
	State         string             `json:"state"                   yaml:"state"` // "stopped" or "running" - desired lifecycle state
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`
	config.BaseDesiredState
}

// GetState returns the desired lifecycle state, defaulting to "running" if empty.
func (d *WrappedDesiredState[TConfig]) GetState() string {
	if d.State == "" {
		return config.DesiredStateRunning
	}

	return d.State
}

// GetChildrenSpecs returns the children specifications.
// Implements config.ChildSpecProvider interface.
func (d *WrappedDesiredState[TConfig]) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// --- WorkerSnapshot and ConvertWorkerSnapshot ---

// WorkerSnapshot is the typed snapshot passed to State.Next(). It eliminates
// unsafe type assertions previously required in every state file.
type WorkerSnapshot[TConfig any, TStatus any] struct {
	CollectedAt         time.Time
	Config              TConfig
	Status              TStatus
	ChildrenView        any
	Identity            deps.Identity
	ParentMappedState   string
	LastActionResults   []deps.ActionResult
	Metrics             deps.MetricsEmbedder
	ChildrenHealthy     int
	ChildrenUnhealthy   int
	IsShutdownRequested bool
}

// ShouldStop returns true when the worker should transition to stopped,
// whether from an explicit shutdown request or a parent-driven stop signal.
func (s WorkerSnapshot[TConfig, TStatus]) ShouldStop() bool {
	return s.IsShutdownRequested || s.ParentMappedState == config.DesiredStateStopped
}

// ConvertWorkerSnapshot type-asserts the raw snapshot from State.Next() into a
// fully typed WorkerSnapshot. Panics with a descriptive message if the snapshot
// contains unexpected types.
func ConvertWorkerSnapshot[TConfig any, TStatus any](snapAny any) WorkerSnapshot[TConfig, TStatus] {
	snap, ok := snapAny.(Snapshot)
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected fsmv2.Snapshot, got %T", snapAny))
	}

	obs, ok := snap.Observed.(Observation[TStatus])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected Observation[TStatus], got %T", snap.Observed))
	}

	des, ok := snap.Desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected *WrappedDesiredState[TConfig], got %T", snap.Desired))
	}

	return WorkerSnapshot[TConfig, TStatus]{
		Config:              des.Config,
		Status:              obs.Status,
		Identity:            snap.Identity,
		IsShutdownRequested: des.IsShutdownRequested(),
		ParentMappedState:   obs.ParentMappedState,
		CollectedAt:         obs.CollectedAt,
		LastActionResults:   obs.LastActionResults,
		Metrics:             obs.MetricsEmbedder,
		ChildrenHealthy:     obs.ChildrenHealthy,
		ChildrenUnhealthy:   obs.ChildrenUnhealthy,
		ChildrenView:        obs.ChildrenView,
	}
}

// ExtractConfig type-asserts a DesiredState to *WrappedDesiredState[TConfig]
// and returns the developer's typed config.
// Panics with a descriptive message if the type does not match.
func ExtractConfig[TConfig any](desired DesiredState) TConfig {
	wds, ok := desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ExtractConfig: expected *WrappedDesiredState[TConfig], got %T", desired))
	}

	return wds.Config
}

// --- SimpleAction ---

// SimpleAction creates an Action[any] from a typed function. It checks
// ctx.Done() before invoking fn, eliminating the need for explicit action
// structs in simple cases.
//
// The returned action type-asserts depsAny to TDeps at call time.
// A wrong type returns a descriptive error.
func SimpleAction[TDeps any](name string, fn func(ctx context.Context, deps TDeps) error) Action[any] {
	return &simpleAction[TDeps]{name: name, fn: fn}
}

type simpleAction[TDeps any] struct {
	fn   func(ctx context.Context, deps TDeps) error
	name string
}

func (a *simpleAction[TDeps]) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	typedDeps, ok := depsAny.(TDeps)
	if !ok {
		return fmt.Errorf("SimpleAction %q: expected deps type %T, got %T", a.name, *new(TDeps), depsAny)
	}

	return a.fn(ctx, typedDeps)
}

func (a *simpleAction[TDeps]) Name() string   { return a.name }
func (a *simpleAction[TDeps]) String() string { return a.name }

// --- InitialStateRegistry ---

var (
	initialStateRegistry   = make(map[string]State[any, any])
	initialStateRegistryMu sync.RWMutex
)

// RegisterInitialState registers the initial state for a worker type.
// Called from state package init() functions. Panics on duplicate registration.
func RegisterInitialState(workerType string, state State[any, any]) {
	initialStateRegistryMu.Lock()
	defer initialStateRegistryMu.Unlock()

	if _, exists := initialStateRegistry[workerType]; exists {
		panic(fmt.Sprintf("RegisterInitialState: duplicate registration for %q", workerType))
	}

	initialStateRegistry[workerType] = state
}

// LookupInitialState returns the registered initial state for a worker type.
// Returns nil if no state is registered.
func LookupInitialState(workerType string) State[any, any] {
	initialStateRegistryMu.RLock()
	defer initialStateRegistryMu.RUnlock()

	return initialStateRegistry[workerType]
}

// ResetInitialStateRegistry clears all registrations. For testing only.
func ResetInitialStateRegistry() {
	initialStateRegistryMu.Lock()
	defer initialStateRegistryMu.Unlock()

	initialStateRegistry = make(map[string]State[any, any])
}
