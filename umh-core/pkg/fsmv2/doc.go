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

// Package fsmv2 provides a type-safe finite state machine framework for managing
// worker lifecycle in the United Manufacturing Hub.
//
// # Overview
//
// For background on why FSMv2 exists and how it relates to Kubernetes/PLC control loop patterns,
// see README.md "Why FSMv2?" section. For the conceptual overview and Triangle Model diagram,
// see README.md.
//
// FSMv2 separates concerns into three layers:
//   - Worker: Business logic implementation (what the worker does).
//   - State: Decision logic (when to transition, what actions to take).
//   - Supervisor: Orchestration (tick loop, action execution, child management).
//
// This separation enables:
//   - Pure functional state transitions (no side effects in Next()).
//   - Explicit action execution with retry/backoff.
//   - Declarative child management (Kubernetes-style).
//   - Type-safe dependencies via generics.
//
// # Quick Start
//
// For complete working examples, see:
//   - workers/example/examplechild/worker.go - Child worker implementation
//   - workers/example/examplechild/state/ - State definitions and transitions
//   - workers/example/examplechild/action/ - Idempotent actions
//   - workers/example/exampleparent/worker.go - Parent with child management
//   - examples/cascade.go - Runnable example with YAML config
//
// # Key Concepts
//
// For the Triangle Model and Tick Loop diagram, see README.md.
//
// ## States: Structs, Not Strings
//
// States are concrete Go types implementing the State interface, providing:
//   - Compile-time type safety (returning wrong type will not compile).
//   - Explicit transitions (visible in code).
//   - Encapsulated logic (each state is self-contained).
//
// See workers/example/examplechild/state/ for state implementations.
//
// ## Immutability: Pass-by-Value
//
// The supervisor passes snapshots by value to State.Next(), making them inherently immutable.
// Go's pass-by-value semantics guarantee states cannot modify the supervisor's data.
// No getters or defensive copying needed - the language enforces immutability.
// See internal/helpers/state_adapter.go for ConvertSnapshot helper that provides type-safe access.
//
// Note: The State interface uses generics (State[TSnapshot, TDeps]), but implementations
// use State[any, any] because Go lacks covariance support. The Snapshot struct provides
// type-safe access via helpers.ConvertSnapshot[O, D](snapAny).
//
// ## Actions: Idempotent Operations
//
// Actions represent idempotent side effects that modify external system state.
// They do not cause state transitions directly. All actions must be idempotent,
// meaning they are safe to retry after partial completion.
//
// Key requirements:
//   - Actions are empty structs. Dependencies are injected via Execute(ctx, depsAny).
//   - Always check ctx.Done() first for cancellation.
//   - Check if work is already done before performing it (idempotency).
//
// See workers/example/examplechild/action/connect.go for a complete example.
//
// When an action fails, the state remains unchanged. On the next tick,
// the supervisor calls state.Next() again, which may return the same action.
//
// ## Error Handling
//
// Return an error for transient failures that the framework should retry:
//   - Network timeouts or connection refusals.
//   - Retriable conditions like pool exhaustion or rate limiting.
//
// Avoid returning errors for these situations:
//   - Validation failures: Validate in state.Next() before emitting the action.
//   - Permanent failures: Return SignalNeedsRestart from state.Next() instead.
//   - Expected conditions: For example, "already connected" is success.
//
// ## DesiredState: No Runtime Dependencies
//
// DesiredState never contains Dependencies. This architectural constraint ensures
// serializability, since Dependencies are runtime interfaces (connections, pools).
//
// If you need state to check a runtime condition (like IsConnected()):
//  1. Collector reads it from dependencies
//  2. Collector writes to ObservedState field (e.g., ConnectionHealth)
//  3. State.Next() checks ObservedState, not dependencies
//
// See workers/example/examplechild/worker.go for the correct pattern.
//
// ## Retry Mechanism
//
// FSMv2 retries failed actions through tick-based state re-evaluation, not automatic backoff.
//
// Primary Retry Mechanism (tick-based re-evaluation):
//   - Action fails → state remains unchanged → inProgress flag cleared
//   - Next tick → state.Next() called again with fresh observation
//   - If conditions unchanged → state.Next() returns same action → action enqueued again
//   - Retry rate governed by tick interval (not exponential backoff)
//   - The retry mechanism has no max attempts limit and retries until the action succeeds or the supervisor shuts down.
//
// Action-Observation Gating:
//   - After action enqueued, actionPending flag is set
//   - FSM tick is blocked until fresh observation arrives (prevents duplicate actions)
//   - Observation timestamp must be newer than action enqueue time to proceed
//   - This ensures the action's effect is observed before re-evaluation
//
// Action Execution (per-action):
//   - Default timeout: 30 seconds per action attempt
//   - Executed asynchronously in worker pool (non-blocking tick loop)
//   - There is no automatic retry within a single execution. A failure clears the inProgress flag.
//   - Retries happen naturally via tick-based re-evaluation
//
// Infrastructure Health Circuit Breaker (infrastructure failures):
//   - Max attempts: 5 (DefaultMaxInfraRecoveryAttempts)
//   - Attempt window: 5 minutes (DefaultRecoveryAttemptWindow)
//   - Backoff range: 1s → 60s (exponential)
//   - Escalation after 5 failed attempts (logs manual intervention required)
//
// Example retry flow:
//
//	Tick 1: state.Next() returns ConnectAction → action enqueued
//	        Action fails → inProgress cleared
//	Tick 2: actionPending blocks until fresh observation (gating)
//	Tick 3: state.Next() re-evaluates → conditions unchanged → ConnectAction returned
//	        Action enqueued again → retries naturally via tick loop
//
// Circuit Breaker Escalation Flow (infrastructure only):
//
//	Attempts 1-3: Log warnings with retry countdown
//	Attempt 4: "Warning: One retry attempt remaining before escalation"
//	Attempt 5: "Escalation required: Manual intervention needed"
//	Runbook: https://docs.umh.app/runbooks/supervisor-escalation
//
// Implementation details in:
//   - supervisor/internal/execution/action_executor.go (async execution, timeout)
//   - supervisor/internal/execution/backoff.go (ExponentialBackoff for circuit breaker)
//   - supervisor/infrastructure_health.go (circuit breaker constants)
//   - supervisor/reconciliation.go (tick loop, action gating, re-evaluation)
//
// ## Validation: Layered Approach
//
// FSMv2 validates data at multiple layers to catch errors early:
//   - Layer 1: API entry (supervisor.AddWorker) - Fast fail on invalid input
//   - Layer 2: Reconciliation (reconcileChildren) - Runtime consistency checks
//   - Layer 3: Factory (WorkerFactory) - Registry validation
//   - Layer 4: Worker constructor (NewMyWorker) - Business logic validation
//
// Each layer catches different types of errors.
//
// ## Variables: Three-Tier Namespace
//
// VariableBundle provides three namespaces for configuration:
//   - User: Top-level template access (flattened, e.g., {{ .IP }})
//   - Global: Fleet-wide settings (nested, e.g., {{ .global.cluster_id }})
//   - Internal: Runtime metadata (nested). Not serialized.
//
// User and Global are persisted. Internal is runtime-only.
// See config/variables.go for the VariableBundle struct definition.
//
// ## Hierarchical Composition: Parent-Child Workers
//
// Parents declare children via ChildSpec in DeriveDesiredState().
// Supervisor handles creation, updates, and cleanup automatically.
//
// Key concepts:
//   - Parent returns ChildrenSpecs in DeriveDesiredState()
//   - ChildStartStates coordinates child lifecycle (not data passing)
//   - Use VariableBundle for passing data to children
//
// See workers/example/exampleparent/worker.go for complete parent-child example.
//
// ## Helper Functions
//
// The config package provides helpers to reduce boilerplate in DeriveDesiredState():
//
//	// ParseUserSpec[T] - type-safe parsing of UserSpec.Config
//	parsed, err := config.ParseUserSpec[MyConfig](spec)
//	if err != nil { return config.DesiredState{}, err }
//
//	// DeriveLeafState[T] - one-liner for leaf workers (no children)
//	// Requires MyConfig to implement GetState() string
//	return config.DeriveLeafState[MyConfig](spec)
//
// These helpers eliminate 15-25 lines of boilerplate per worker.
// See config/helpers.go for full documentation.
//
// ## Parent-Child Visibility (ChildrenView)
//
// Parent workers can observe their children's state via ChildrenView interface:
//
//	type ChildrenView interface {
//	    List() []ChildInfo           // All children with state info
//	    Get(name string) *ChildInfo  // Single child by name
//	    Counts() (healthy, unhealthy int)
//	    AllHealthy() bool
//	    AllStopped() bool
//	}
//
// ChildInfo provides read-only info about each child:
//   - Name, WorkerType, StateName, IsHealthy
//   - HierarchyPath for logging context
//
// To use in parent workers, implement SetChildrenView() on your ObservedState:
//
//	func (o MyObservedState) SetChildrenView(view config.ChildrenView) fsmv2.ObservedState {
//	    healthy, unhealthy := view.Counts()
//	    o.ChildrenHealthy = healthy
//	    o.ChildrenUnhealthy = unhealthy
//	    return o
//	}
//
// The supervisor automatically calls SetChildrenView() during observation collection.
// See config/childspec.go for ChildrenView and ChildInfo definitions.
//
// # Architecture Documentation
//
// For detailed architecture explanations, see:
//   - architecture_test.go - Patterns enforced by tests (run with -v for WHY explanations)
//   - api.go - Core interfaces (Worker, State, Action)
//   - internal/helpers/ - Convenience helpers (BaseState, BaseWorker, ConvertSnapshot)
//   - supervisor/supervisor.go - Orchestration and lifecycle management
//   - config/childspec.go - Hierarchical composition
//   - config/variables.go - Variable namespaces
//
// # Architecture Validation
//
// Run architecture tests to validate all patterns:
//
//	ginkgo run --focus="Architecture" -v ./pkg/fsmv2/
//
// Tests enforce: immutability, shutdown handling, state naming, idempotency.
//
// # Common Patterns
//
// ## State Naming Conventions
//
// Active states emit actions until a condition is met:
//   - Use the prefix "TryingTo" or "Ensuring".
//   - Examples include TryingToStartState and EnsuringConnectedState.
//
// Passive states observe and react:
//   - Use descriptive nouns.
//   - Examples include RunningState, StoppedState, and DegradedState.
//
// ## Shutdown Handling
//
// Check IsShutdownRequested() as the first conditional in Next().
// See workers/example/examplechild/state/ for examples of proper shutdown handling.
//
// ## Type-Safe Dependencies
//
// Use BaseWorker[D] for type-safe dependency access without casting.
// See workers/example/examplechild/dependencies.go for dependency pattern.
//
// # Testing
//
// Test FSM workers by:
//  1. Creating test states and actions
//  2. Calling Next() with test snapshots
//  3. Verifying returned state, signal, and action
//  4. Testing action idempotency with VerifyActionIdempotency helper
//
// See workers/example/examplechild/state/*_test.go for state transition tests.
// See workers/example/examplechild/action/*_test.go for action idempotency tests.
//
// # Thread Safety
//
// The supervisor manages concurrency:
//   - CollectObservedState() runs in separate goroutine with timeout
//   - The supervisor calls State.Next() in its main goroutine (single-threaded).
//   - The supervisor runs Action.Execute() in its main goroutine with retry/backoff.
//
// Workers do not need to implement locking because the supervisor handles it.
//
// # Best Practices
//
//   - Keep Next() pure (see "Immutability" section above)
//   - Make actions idempotent (check if work already done)
//   - Check IsShutdownRequested() first in all states (see "Shutdown Handling" section above)
//   - Use type-safe state structs, not strings
//   - Return action OR transition, not both simultaneously
//   - Handle context cancellation in all async operations
//   - Test action idempotency with VerifyActionIdempotency helper
package fsmv2
