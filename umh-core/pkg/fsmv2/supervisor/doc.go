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

// Package supervisor provides the orchestration layer for FSMv2 workers.
//
// # Overview
//
// The Supervisor manages the complete lifecycle of FSMv2 workers through a
// deterministic tick loop. It coordinates observation collection, desired state
// derivation, state machine evaluation, and asynchronous action execution.
//
// For the core FSMv2 interfaces (Worker, State, Action), see ../api.go.
// For the Triangle Model and architecture overview, see ../doc.go.
//
// # Supervisor responsibilities
//
// The Supervisor handles:
//   - Worker lifecycle management (add, remove, restart)
//   - Observation collection via Collector (separate goroutine)
//   - Desired state derivation and template expansion
//   - State machine evaluation and transition logic
//   - Asynchronous action execution via ActionExecutor
//   - Hierarchical composition (parent-child worker trees)
//   - Infrastructure health monitoring with circuit breaker
//   - Data freshness validation with collector restart
//   - Graceful shutdown coordination
//
// The Supervisor does not:
//   - Execute actions synchronously (delegated to ActionExecutor)
//   - Implement business logic (delegated to Worker and State)
//   - Manage state transitions (delegated to State.Next())
//
// # Tick loop
//
// The tick() method executes four phases in priority order. Each tick completes
// in under 10ms, supporting high frequency calls (100Hz+).
//
// ## Phase 1: Infrastructure supervision
//
// Verifies child consistency via InfrastructureHealthChecker.
//
// Circuit breaker pattern:
//   - Opens on failure, skips rest of tick
//   - Child restart handled with exponential backoff
//   - Max 5 attempts in 5-minute window (DefaultMaxInfraRecoveryAttempts)
//   - Escalates to manual intervention after exhausting attempts
//
// Completes in under 1ms without blocking.
//
// See infrastructure_health.go for implementation details.
//
// ## Phase 0.5: Variable injection
//
// Injects variables into UserSpec before template expansion:
//
// Global variables:
//   - From management system (fleet-wide configuration)
//   - Available as {{ .global.key }} in templates
//
// Internal variables:
//   - supervisorID: Supervisor's worker ID
//   - createdAt: Worker creation timestamp
//   - bridgedBy: Parent worker ID (if applicable)
//   - Available as {{ .internal.key }} in templates
//
// User variables:
//   - From UserSpec.Variables.User (preserved, not overwritten)
//   - Available as {{ .key }} in templates (flattened namespace)
//
// Variables are available for template expansion in DeriveDesiredState().
//
// See config/variables.go for the VariableBundle struct definition.
//
// ## Phase 0: Hierarchical composition
//
// Derives desired state and reconciles the worker hierarchy.
//
// DeriveDesiredState:
//   - Worker implementation returns DesiredState with ChildrenSpecs
//   - Template expansion using injected variables
//   - Result cached based on UserSpec hash
//   - Cache invalidated when UserSpec changes
//
// Reconcile children:
//   - Creates child supervisors for new ChildrenSpecs
//   - Updates existing children with new UserSpec
//   - Requests graceful shutdown for removed children
//   - Removes children after workers complete shutdown
//   - Propagates errors (halts reconciliation on failure)
//
// Apply state mapping:
//   - Parent state influences child desired state
//   - ChildStartStates determines when children should run
//   - Empty ChildStartStates means child always runs
//
// Recursively tick children:
//   - Ticks all children in hierarchy order
//   - Logs errors but does not propagate them (isolation)
//
// See config/childspec.go for ChildSpec definition and reconciliation logic.
//
// ## Phase 2: Async action execution
//
// Evaluates state machine and executes actions asynchronously.
//
// State machine evaluation:
//   - Loads fresh snapshot from database
//   - Calls state.Next() with snapshot
//   - Returns next state, signal, and optional action
//
// Action execution:
//   - Actions execute in global worker pool (non-blocking)
//   - Default timeout: 30 seconds per action attempt
//   - Retry handled by tick-based re-evaluation (no exponential backoff)
//   - Action-observation gating prevents duplicate actions
//
// See internal/execution/doc.go for ActionExecutor details.
//
// # Per-worker tick steps
//
// The tickWorker() method processes a single worker through these steps:
//
// 1. Load snapshot from database
//
// Loads the freshest Identity, ObservedState, and DesiredState from database.
// The snapshot provides an immutable view of the worker's state.
//
// Invariant I16: ObservedState must never be nil.
//
// 2. Check data freshness (Invariant I3)
//
// Verifies observation timestamp is recent:
//   - Stale threshold: Data older than expected but not yet timeout
//   - Timeout threshold: Data old enough to indicate collector failure
//
// Freshness check skipped during shutdown (collectors already stopped).
//
// On timeout:
//   - Restarts collector with exponential backoff
//   - Max restart attempts: 3 (DefaultMaxCollectorRestartAttempts)
//   - Escalates to shutdown if max attempts exhausted
//
// States assume data is fresh except during shutdown.
//
// 3. Action-observation gating
//
// Prevents duplicate action execution when ticker fires faster than collector:
//   - After enqueueing action, sets actionPending flag
//   - Blocks FSM tick until observation is newer than action timestamp
//   - Clears actionPending flag when fresh observation arrives
//
// The action's effect is observed before re-evaluation.
//
// 4. Call state.Next() with snapshot
//
// State machine evaluates transitions:
//   - Returns next state (may be same as current)
//   - Returns signal (None, NeedsRemoval, NeedsRestart)
//   - Returns optional action (mutually exclusive with state change)
//
// Validation: Cannot switch state and emit action simultaneously (supervisor panics).
//
// 5. Execute action if returned
//
// If action is returned:
//   - Checks if action already in progress (skip duplicate)
//   - Gets dependencies from worker via DependencyProvider
//   - Enqueues action via executor.EnqueueAction()
//   - Sets action gating (blocks FSM tick until fresh observation)
//
// Actions execute asynchronously in worker pool. Errors handled via retry.
//
// 6. Transition state if changed
//
// If next state differs from current state:
//   - Logs state transition with reason
//   - Updates workerCtx.currentState under lock
//
// State transitions are logged at INFO level.
//
// 7. Process signal
//
// Signals coordinate worker lifecycle:
//
// SignalNone:
//   - Normal operation, continue
//
// SignalNeedsRemoval:
//   - Checks if worker should restart instead (pendingRestart flag)
//   - If restart: resets state to initial and restarts collector
//   - If removal: collects final observation, stops collector, cleans up children
//
// SignalNeedsRestart:
//   - Marks for restart (pendingRestart flag)
//   - Requests graceful shutdown via requestShutdown()
//   - Worker goes through shutdown states, emits SignalNeedsRemoval
//   - On SignalNeedsRemoval: restarts instead of removes
//
// Restart timeout: 30 seconds (DefaultGracefulRestartTimeout). If exceeded,
// force-resets state and restarts collector.
//
// See processSignal() in reconciliation.go for implementation.
//
// # Thread safety
//
// The Supervisor manages concurrency through lock coordination.
//
// ## Supervisor-level locking
//
// s.mu (RWMutex) protects:
//   - workers map (workerID -> WorkerContext)
//   - children map (childName -> SupervisorInterface)
//   - userSpec, globalVars, cachedDesiredState
//   - pendingRestart, restartRequestedAt maps
//
// Read lock: Accessing workers/children without modification
// Write lock: Adding/removing workers/children, updating configuration
//
// ## Worker-level locking
//
// workerCtx.mu (RWMutex) protects:
//   - currentState (FSM state)
//   - actionPending flag (action-observation gating)
//   - lastActionObsTime (gating timestamp)
//
// Read lock: Reading state for logging/queries
// Write lock: State transitions, setting action gating
//
// ## Lock-free operations
//
// tickInProgress (atomic.Bool):
//   - Prevents concurrent tick operations on same worker
//   - CompareAndSwap ensures only one tick in flight
//
// circuitOpen (atomic.Bool):
//   - Circuit breaker state (infrastructure health)
//   - No mutex needed for single boolean flag
//
// tickCount (atomic.Uint64):
//   - Tick counter for heartbeat logging
//   - Incremented atomically to handle parent tick() calling child tick()
//
// ## Concurrent operations
//
// CollectObservedState():
//   - Runs in separate goroutine per worker
//   - Timeout: DefaultObservationInterval (500ms)
//   - Saves observation to database (database handles concurrency)
//   - Does not modify supervisor state directly
//
// State.Next():
//   - Runs in main goroutine (tick loop)
//   - Receives snapshot by value (immutable)
//   - Does not modify supervisor state directly
//   - Returns state (supervisor updates workerCtx.currentState under lock)
//
// Action.Execute():
//   - Runs asynchronously in ActionExecutor worker pool
//   - Modifies external system state (not supervisor state)
//   - Completion logged, no supervisor state update
//
// ## Deadlock prevention
//
// Hierarchy lock order:
//   - Acquire parent lock before child lock
//   - Do not call child.GetHierarchyPath() while holding parent lock
//     (child would try to acquire parent lock, causing deadlock)
//
// Lock release before blocking:
//   - Release s.mu before calling child.Shutdown() (blocks until child stops)
//   - Release s.mu before waiting on child done channels
//
// Minimal lock duration:
//   - Hold locks only for data structure access
//   - Release before I/O operations (database, network)
//   - Release before calling child methods
//
// ## Race condition patterns
//
// Worker removal race:
//   - Parent tick() obtains workerIDs list under lock
//   - Child tick() tries to access worker
//   - Worker may have been removed between list capture and access
//   - Solution: Check if worker exists, skip if missing (benign race)
//
// Child reconciliation race:
//   - Parent calls reconcileChildren() under lock
//   - Parent calls child.tick() outside lock
//   - Child may be removed between reconciliation and tick
//   - Solution: Copy children map before releasing lock, handle missing children
//
// See lockmanager/doc.go for lock debugging and deadlock detection.
//
// # Action execution
//
// Actions represent idempotent side effects that modify external system state.
// The Supervisor coordinates action execution without blocking the tick loop.
//
// ## Execution flow
//
// Enqueue:
//   - State.Next() returns action
//   - Supervisor checks if action already in progress (skip duplicate)
//   - Supervisor enqueues action via executor.EnqueueAction()
//   - Non-blocking: if queue full, enqueue fails
//
// Execute:
//   - ActionExecutor worker picks action from queue
//   - Creates context with timeout (default: 30 seconds)
//   - Calls action.Execute(ctx, deps)
//   - Logs completion or error
//   - Clears in-progress flag
//
// Retry:
//   - On failure, in-progress flag cleared
//   - Next tick: state.Next() re-evaluated with fresh observation
//   - If conditions unchanged: state.Next() returns same action
//   - Action enqueued again (tick-based retry)
//
// ## Async execution
//
// Actions execute asynchronously for the following reasons:
//
// Non-blocking tick loop: Actions may take seconds (network calls, process
// starts). Synchronous execution would stall the tick loop.
//
// Timeout protection: Actions have per-operation timeouts. If an action
// hangs (network partition), context is cancelled and action fails.
//
// Isolation: Action failures don't bring down the supervisor or affect
// other workers. Each action runs in its own goroutine.
//
// Predictable performance: Tick loop completes in under 10ms regardless of
// action complexity, enabling high-frequency ticking (100Hz+).
//
// ## Idempotency requirement
//
// All actions must be idempotent (safe to call multiple times):
//
// Retry after timeout: Action may time out but partially complete. The
// supervisor retries, so action must check if work already done.
//
// Network partitions: Action may execute but completion ack lost. The
// supervisor retries, so action must handle "already done" gracefully.
//
// Tick-based retry: Supervisor retries by re-evaluating state.Next(). Each
// retry must be safe.
//
// Example idempotent action:
//
//	func (a *StartProcessAction) Execute(ctx context.Context, deps any) error {
//	    // Check if already done (idempotency)
//	    if processIsRunning(a.ProcessPath) {
//	        return nil  // Already started
//	    }
//	    return startProcess(ctx, a.ProcessPath)
//	}
//
// ## Action-observation gating
//
// The actionPending flag prevents duplicate actions when ticker fires faster
// than collector:
//
// Without gating (incorrect):
//   - Tick 1: Enqueue ConnectAction, observation timestamp 10:00:00
//   - Tick 2: state.Next() re-evaluated, returns ConnectAction again (duplicate)
//   - Result: Two concurrent connect attempts
//
// With gating (correct):
//   - Tick 1: Enqueue ConnectAction, set actionPending, lastActionObsTime 10:00:00
//   - Tick 2: actionPending blocks, skip state.Next()
//   - Tick 3: Observation arrives (10:00:01 > 10:00:00), clear actionPending
//   - Tick 4: state.Next() re-evaluated with fresh observation
//
// Action effects are observed before re-evaluation.
//
// ## Worker pool architecture
//
// ActionExecutor uses a fixed worker pool (default: 10 workers):
//
// Bounded concurrency: Prevents resource exhaustion. In production, thousands
// of actions could be enqueued simultaneously.
//
// Queue backpressure: Buffered channel (2x worker count) provides
// backpressure. If queue full, enqueue returns error.
//
// Predictable resources: Fixed pool means predictable memory/CPU usage
// regardless of action volume.
//
// See internal/execution/doc.go for ActionExecutor details.
//
// # Testing
//
// Test supervisors by:
//  1. Creating a supervisor with test worker
//  2. Calling tick() to progress FSM
//  3. Verifying state transitions via GetCurrentState()
//  4. Checking children via GetChildren()
//  5. Verifying observations in database
//
// See supervisor_test.go for complete examples.
//
// # Best practices
//
//   - Check IsShutdownRequested() as first condition in state.Next()
//   - Make all actions idempotent (check if work already done)
//   - Use ChildStartStates to coordinate child lifecycle (not data passing)
//   - Pass data to children via VariableBundle, not direct method calls
//   - Release locks before calling child methods (prevent deadlock)
//   - Handle context cancellation in all async operations
//   - Log state transitions at INFO level
//   - Test action idempotency with VerifyActionIdempotency helper
package supervisor
