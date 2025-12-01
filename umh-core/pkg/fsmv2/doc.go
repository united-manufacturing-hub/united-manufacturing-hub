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
// For the conceptual overview and Triangle Model diagram, see README.md.
//
// FSMv2 separates concerns into three layers:
//   - Worker: Business logic implementation (what the worker does)
//   - State: Decision logic (when to transition, what actions to take)
//   - Supervisor: Orchestration (tick loop, action execution, child management)
//
// This separation enables:
//   - Pure functional state transitions (no side effects in Next())
//   - Explicit action execution with retry/backoff
//   - Declarative child management (Kubernetes-style)
//   - Type-safe dependencies via generics
//
// # Quick Start
//
// For complete working examples, see:
//   - workers/example/example-child/worker.go - Child worker implementation
//   - workers/example/example-child/state/ - State definitions and transitions
//   - workers/example/example-child/action/ - Idempotent actions
//   - workers/example/exampleparent/worker.go - Parent with child management
//   - examples/simple/main.go - Runnable example with YAML config
//
// # Key Concepts
//
// For the Triangle Model and Tick Loop diagram, see README.md.
//
// ## States: Structs, Not Strings
//
// States are concrete Go types implementing the State interface, providing:
//   - Compile-time type safety (returning wrong type won't compile)
//   - Explicit transitions (visible in code)
//   - Encapsulated logic (each state is self-contained)
//
// See workers/example/example-child/state/ for state implementations.
//
// ## Immutability: Pass-by-Value
//
// Snapshots are passed by value to State.Next(), making them inherently immutable.
// Go's pass-by-value semantics guarantee states cannot modify the supervisor's data.
// No getters or defensive copying needed - the language enforces immutability.
// See internal/helpers/state_adapter.go for ConvertSnapshot helper that provides type-safe access.
//
// ## Actions: Idempotent Operations
//
// Actions represent side effects that transition the system between states.
// All actions MUST be idempotent - safe to retry after partial completion.
//
// Key requirements:
//   - Actions are EMPTY STRUCTS - dependencies injected via Execute(ctx, depsAny)
//   - ALWAYS check ctx.Done() first for cancellation
//   - Check if work already done before performing it (idempotency)
//
// See workers/example/example-child/action/connect.go for a complete example.
//
// The supervisor retries failed actions with exponential backoff, so idempotency is critical.
//
// ## Retry and Backoff Configuration
//
// FSMv2 automatically retries failed actions with exponential backoff to handle transient failures.
//
// Action Execution Retry (per-action):
//   - Base delay: 1 second
//   - Max delay: 60 seconds
//   - Formula: delay = min(2^attempts × baseDelay, maxDelay)
//   - Default timeout: 30 seconds per action attempt
//   - No max attempts limit (retries until action succeeds or supervisor shuts down)
//
// Infrastructure Health Circuit Breaker (infrastructure failures):
//   - Max attempts: 5 (DefaultMaxInfraRecoveryAttempts)
//   - Attempt window: 5 minutes (DefaultRecoveryAttemptWindow)
//   - Backoff range: 1s → 60s (exponential)
//   - Escalation after 5 failed attempts (logs manual intervention required)
//
// Example action retry sequence:
//   Attempt 1: Execute immediately
//   Attempt 2: Wait 1s, execute
//   Attempt 3: Wait 2s, execute
//   Attempt 4: Wait 4s, execute
//   Attempt 5: Wait 8s, execute
//   Attempt 6+: Wait 60s, execute (capped at maxDelay)
//
// Circuit Breaker Escalation Flow:
//   Attempts 1-3: Log warnings with retry countdown
//   Attempt 4: "WARNING: One retry attempt remaining before escalation"
//   Attempt 5: "ESCALATION REQUIRED: Manual intervention needed"
//   Runbook: https://docs.umh.app/runbooks/supervisor-escalation
//
// Implementation details in:
//   - supervisor/internal/execution/backoff.go (ExponentialBackoff implementation)
//   - supervisor/infrastructure_health.go (circuit breaker constants)
//   - supervisor/reconciliation.go (retry logic and escalation)
//
// ## Validation: Defense-in-Depth
//
// FSMv2 validates data at multiple layers (not centralized) for robustness:
//   - Layer 1: API entry (supervisor.AddWorker) - Fast fail on invalid input
//   - Layer 2: Reconciliation (reconcileChildren) - Runtime consistency checks
//   - Layer 3: Factory (WorkerFactory) - Registry validation
//   - Layer 4: Worker constructor (NewMyWorker) - Business logic validation
//
// This is intentional, not redundant. Each layer defends against different failure modes.
//
// ## Variables: Three-Tier Namespace
//
// VariableBundle provides three namespaces for configuration:
//   - User: Top-level template access (flattened, e.g., {{ .IP }})
//   - Global: Fleet-wide settings (nested, e.g., {{ .global.cluster_id }})
//   - Internal: Runtime metadata (nested) - NOT serialized
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
//   - StateMapping coordinates FSM states (NOT data passing)
//   - Use VariableBundle for passing data to children
//
// See workers/example/exampleparent/worker.go for complete parent-child example.
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
// Active states (emit actions until condition met):
//   - Prefix: "TryingTo" or "Ensuring"
//   - Examples: TryingToStartState, EnsuringConnectedState
//
// Passive states (observe and react):
//   - Descriptive nouns
//   - Examples: RunningState, StoppedState, DegradedState
//
// ## Shutdown Handling
//
// States MUST check IsShutdownRequested() as their first conditional in Next().
// See workers/example/example-child/state/ for examples of proper shutdown handling.
//
// ## Type-Safe Dependencies
//
// Use BaseWorker[D] for type-safe dependency access without casting.
// See workers/example/example-child/dependencies.go for dependency pattern.
//
// # Testing
//
// Test FSM workers by:
//  1. Creating test states and actions
//  2. Calling Next() with test snapshots
//  3. Verifying returned state, signal, and action
//  4. Testing action idempotency with VerifyActionIdempotency helper
//
// See workers/example/example-child/state/*_test.go for state transition tests.
// See workers/example/example-child/action/*_test.go for action idempotency tests.
//
// # Thread Safety
//
// The supervisor manages concurrency:
//   - CollectObservedState() runs in separate goroutine with timeout
//   - State.Next() called in supervisor's main goroutine (single-threaded)
//   - Action.Execute() runs in supervisor's main goroutine with retry/backoff
//
// Workers don't need to implement locking - the supervisor handles it.
//
// # Best Practices
//
//   - Keep Next() pure (no side effects, no external calls)
//   - Make actions idempotent (check if work already done)
//   - Check ShutdownRequested() first in all states
//   - Use type-safe state structs, not strings
//   - Return action OR transition, not both simultaneously
//   - Handle context cancellation in all async operations
//   - Test action idempotency with VerifyActionIdempotency helper
package fsmv2
