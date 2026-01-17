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
// # Supervisor Responsibilities
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
// # Tick Loop
//
// The tick() method executes four phases in priority order. Each tick completes
// in under 10ms, making it safe to call at high frequency (100Hz+) without
// impacting system performance.
//
// ## Phase 1: Infrastructure Supervision
//
// Verifies child consistency via InfrastructureHealthChecker.
//
// Circuit breaker pattern:
//   - Opens on failure, skips rest of tick
//   - Child restart handled with exponential backoff
//   - Max 5 attempts in 5-minute window (DefaultMaxInfraRecoveryAttempts)
//   - Escalates to manual intervention after exhausting attempts
//
// Non-blocking operation completes in under 1ms.
//
// See infrastructure_health.go for implementation details.
//
// ## Phase 0.5: Variable Injection
//
// Injects variables into UserSpec before template expansion:
//
// Global Variables:
//   - From management system (fleet-wide configuration)
//   - Available as {{ .global.key }} in templates
//
// Internal Variables:
//   - supervisorID: Current supervisor's worker ID
//   - createdAt: Worker creation timestamp
//   - bridgedBy: Parent worker ID (if applicable)
//   - Available as {{ .internal.key }} in templates
//
// User Variables:
//   - From UserSpec.Variables.User (preserved, not overwritten)
//   - Available as {{ .key }} in templates (flattened namespace)
//
// Variables are available for template expansion in DeriveDesiredState().
//
// See config/variables.go for the VariableBundle struct definition.
//
// ## Phase 0: Hierarchical Composition
//
// Derives desired state and reconciles the worker hierarchy.
//
// DeriveDesiredState:
//   - Worker implementation returns DesiredState with ChildrenSpecs
//   - Template expansion using injected variables
//   - Result cached based on UserSpec hash (performance optimization)
//   - Cache invalidated when UserSpec changes
//
// Reconcile Children:
//   - Create new child supervisors for new ChildrenSpecs
//   - Update existing children with new UserSpec
//   - Request graceful shutdown for removed children
//   - Remove children after workers complete shutdown
//   - Errors propagated (halts reconciliation on failure)
//
// Apply State Mapping:
//   - Parent state influences child desired state
//   - ChildStartStates determines when children should run
//   - Empty ChildStartStates means child always runs
//
// Recursively Tick Children:
//   - All children ticked in hierarchy order
//   - Errors logged, not propagated (isolation)
//
// See config/childspec.go for ChildSpec definition and reconciliation logic.
//
// ## Phase 2: Async Action Execution
//
// Evaluates state machine and executes actions asynchronously.
//
// State Machine Evaluation:
//   - Loads fresh snapshot from database
//   - Calls state.Next() with snapshot
//   - Returns next state, signal, and optional action
//
// Action Execution:
//   - Actions execute in global worker pool (non-blocking)
//   - Default timeout: 30 seconds per action attempt
//   - Retry handled by tick-based re-evaluation (no exponential backoff)
//   - Action-observation gating prevents duplicate actions
//
// See internal/execution/doc.go for ActionExecutor details.
//
// # Per-Worker Tick Steps
//
// The tickWorker() method processes a single worker through these steps:
//
// 1. Load Snapshot from Database
//
// Loads the freshest Identity, ObservedState, and DesiredState from database.
// The snapshot provides an immutable view of the worker's current state.
//
// Invariant I16: ObservedState must never be nil.
//
// 2. Check Data Freshness (Invariant I3)
//
// Verifies observation timestamp is recent:
//   - Stale threshold: Data older than expected but not yet timeout
//   - Timeout threshold: Data so old it indicates collector failure
//
// Freshness check skipped during shutdown (collectors already stopped).
//
// On timeout:
//   - Restart collector with exponential backoff
//   - Max restart attempts: 3 (DefaultMaxCollectorRestartAttempts)
//   - Escalate to shutdown if max attempts exhausted
//
// This is the trust boundary. States assume data is always fresh except during shutdown.
//
// 3. Action-Observation Gating
//
// Prevents duplicate action execution when ticker fires faster than collector:
//   - After enqueueing action, set actionPending flag
//   - Block FSM tick until observation is newer than action timestamp
//   - Clear actionPending flag when fresh observation arrives
//
// This ensures the action's effect is observed before re-evaluation.
//
// 4. Call state.Next() with Snapshot
//
// State machine evaluates transitions:
//   - Returns next state (may be same as current)
//   - Returns signal (None, NeedsRemoval, NeedsRestart)
//   - Returns optional action (mutually exclusive with state change)
//
// Validation: Cannot switch state and emit action simultaneously (supervisor panics).
//
// 5. Execute Action if Returned
//
// If action is returned:
//   - Check if action already in progress (skip duplicate)
//   - Get dependencies from worker via DependencyProvider
//   - Enqueue action via executor.EnqueueAction()
//   - Set action gating (block FSM tick until fresh observation)
//
// Action executes asynchronously in worker pool. Errors handled via retry.
//
// 6. Transition State if Changed
//
// If next state differs from current state:
//   - Log state transition with reason
//   - Update workerCtx.currentState under lock
//
// State transitions are always logged at INFO level for visibility.
//
// 7. Process Signal
//
// Signals coordinate worker lifecycle:
//
// SignalNone:
//   - Normal operation, continue
//
// SignalNeedsRemoval:
//   - Check if worker should restart instead (pendingRestart flag)
//   - If restart: reset state to initial and restart collector
//   - If removal: collect final observation, stop collector, clean up children
//
// SignalNeedsRestart:
//   - Mark for restart (pendingRestart flag)
//   - Request graceful shutdown via requestShutdown()
//   - Worker goes through shutdown states, emits SignalNeedsRemoval
//   - On SignalNeedsRemoval: restart instead of remove
//
// Restart timeout: 30 seconds (DefaultGracefulRestartTimeout). If exceeded,
// force-reset state and restart collector.
//
// See processSignal() in reconciliation.go for implementation.
//
// # Thread Safety
//
// The Supervisor manages concurrency through careful lock coordination:
//
// ## Supervisor-Level Locking
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
// ## Worker-Level Locking
//
// workerCtx.mu (RWMutex) protects:
//   - currentState (FSM state)
//   - actionPending flag (action-observation gating)
//   - lastActionObsTime (gating timestamp)
//
// Read lock: Reading current state for logging/queries
// Write lock: State transitions, setting action gating
//
// ## Lock-Free Operations
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
// ## Concurrent Operations
//
// CollectObservedState():
//   - Runs in separate goroutine per worker
//   - Timeout: DefaultObservationInterval (500ms)
//   - Saves observation to database (database handles concurrency)
//   - Never modifies supervisor state directly
//
// State.Next():
//   - Runs in main goroutine (tick loop)
//   - Receives snapshot by value (immutable)
//   - Never modifies supervisor state directly
//   - Returns new state (supervisor updates workerCtx.currentState under lock)
//
// Action.Execute():
//   - Runs asynchronously in ActionExecutor worker pool
//   - Modifies external system state (not supervisor state)
//   - Completion logged, no supervisor state update
//
// ## Deadlock Prevention
//
// Hierarchy Lock Order:
//   - Always acquire parent lock before child lock
//   - Never call child.GetHierarchyPath() while holding parent lock
//     (child would try to acquire parent lock, causing deadlock)
//
// Lock Release Before Blocking:
//   - Release s.mu before calling child.Shutdown() (blocks until child stops)
//   - Release s.mu before waiting on child done channels
//
// Minimal Lock Duration:
//   - Hold locks only for data structure access
//   - Release before I/O operations (database, network)
//   - Release before calling child methods
//
// ## Race Condition Patterns
//
// Worker Removal Race:
//   - Parent tick() obtains workerIDs list under lock
//   - Child tick() tries to access worker
//   - Worker may have been removed between list capture and access
//   - Solution: Check if worker exists, skip if missing (benign race)
//
// Child Reconciliation Race:
//   - Parent calls reconcileChildren() under lock
//   - Parent calls child.tick() outside lock
//   - Child may be removed between reconciliation and tick
//   - Solution: Copy children map before releasing lock, handle missing children
//
// See lockmanager/doc.go for lock debugging and deadlock detection.
//
// # Action Execution
//
// Actions represent idempotent side effects that modify external system state.
// The Supervisor coordinates action execution without blocking the tick loop.
//
// ## Execution Flow
//
// Enqueue:
//   - State.Next() returns action
//   - Supervisor checks if action already in progress (skip duplicate)
//   - Supervisor enqueues action via executor.EnqueueAction()
//   - Non-blocking: if queue full, enqueue fails (not common)
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
// ## Why Async Execution?
//
// Actions are executed asynchronously because:
//
// 1. Non-blocking Tick Loop: Actions may take seconds (network calls, process
// starts). Executing synchronously would stall the entire tick loop.
//
// 2. Timeout Protection: Actions have per-operation timeouts. If an action
// hangs (network partition), context is cancelled and action fails cleanly.
//
// 3. Isolation: Action failures don't bring down the supervisor or affect
// other workers. Each action runs in its own goroutine.
//
// 4. Predictable Performance: Tick loop completes in under 10ms regardless of
// action complexity. This enables high-frequency ticking (100Hz+).
//
// ## Idempotency Requirement
//
// All actions must be idempotent (safe to call multiple times) because:
//
// 1. Retry After Timeout: Action may time out but partially complete. Supervisor
// will retry, so action must check if work already done.
//
// 2. Network Partitions: Action may execute but completion ack lost. Supervisor
// will retry, so action must handle "already done" gracefully.
//
// 3. Tick-Based Retry: Supervisor retries by re-evaluating state.Next(). Each
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
// ## Action-Observation Gating
//
// The actionPending flag prevents duplicate actions when ticker fires faster
// than collector:
//
// Without Gating (incorrect):
//   - Tick 1: Enqueue ConnectAction, observation timestamp 10:00:00
//   - Tick 2: state.Next() re-evaluated, returns ConnectAction again (duplicate)
//   - Result: Two concurrent connect attempts
//
// With Gating (correct):
//   - Tick 1: Enqueue ConnectAction, set actionPending, lastActionObsTime 10:00:00
//   - Tick 2: actionPending blocks, skip state.Next()
//   - Tick 3: New observation arrives (10:00:01 > 10:00:00), clear actionPending
//   - Tick 4: state.Next() re-evaluated with fresh observation
//
// This ensures action effects are observed before re-evaluation.
//
// ## Worker Pool Architecture
//
// ActionExecutor uses a fixed worker pool (default: 10 workers) because:
//
// 1. Bounded Concurrency: Prevents resource exhaustion. In production, thousands
// of actions could be enqueued simultaneously.
//
// 2. Queue Backpressure: Buffered channel (2x worker count) provides natural
// backpressure. If queue full, enqueue returns error.
//
// 3. Predictable Resources: Fixed pool means predictable memory/CPU usage
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
// # Best Practices
//
//   - Always check IsShutdownRequested() as first condition in state.Next()
//   - Make all actions idempotent (check if work already done)
//   - Use ChildStartStates to coordinate child lifecycle (not data passing)
//   - Pass data to children via VariableBundle, not direct method calls
//   - Release locks before calling child methods (prevent deadlock)
//   - Handle context cancellation in all async operations
//   - Log state transitions at INFO level (visibility)
//   - Test action idempotency with VerifyActionIdempotency helper
package supervisor
