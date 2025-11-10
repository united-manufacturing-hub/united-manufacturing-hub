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
// ## 1. Define Dependencies
//
//	type MyWorkerDeps struct {
//	    Logger *zap.Logger
//	    Config *MyConfig
//	}
//
// ## 2. Define States
//
//	type InitializingState struct{}
//	type RunningState struct{}
//	type StoppingState struct{}
//
//	func (s InitializingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
//	    if snapshot.Desired.ShutdownRequested() {
//	        return StoppingState{}, fsmv2.SignalNone, nil
//	    }
//	    // Check if initialization complete
//	    return s, fsmv2.SignalNone, &InitializeAction{}
//	}
//
//	func (s RunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
//	    if snapshot.Desired.ShutdownRequested() {
//	        return StoppingState{}, fsmv2.SignalNone, nil
//	    }
//	    // Monitor and maintain running state
//	    return s, fsmv2.SignalNone, nil
//	}
//
//	func (s StoppingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
//	    // Cleanup complete, signal removal
//	    return s, fsmv2.SignalNeedsRemoval, nil
//	}
//
// ## 3. Implement Worker
//
//	type MyWorker struct {
//	    fsmv2.BaseWorker[MyWorkerDeps]
//	}
//
//	func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
//	    deps := w.GetDependencies()
//	    // Collect actual system state (process status, metrics, etc.)
//	    return &MyObservedState{...}, nil
//	}
//
//	func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    // Transform user config into desired state
//	    return config.DesiredState{State: "running"}, nil
//	}
//
//	func (w *MyWorker) GetInitialState() fsmv2.State {
//	    return InitializingState{}
//	}
//
// ## 4. Create and Run Supervisor
//
//	deps := MyWorkerDeps{Logger: logger, Config: config}
//	worker := &MyWorker{BaseWorker: fsmv2.NewBaseWorker(deps)}
//
//	supervisor := supervisor.NewSupervisor(
//	    fsmv2.Identity{ID: "worker-1", Name: "My Worker", WorkerType: "my_type"},
//	    worker,
//	    deps,
//	    supervisor.WithTickInterval(1*time.Second),
//	)
//
//	ctx := context.Background()
//	if err := supervisor.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # Key Concepts
//
// ## State Machine Flow
//
// The supervisor orchestrates worker lifecycle through a tick loop:
//
//	┌─────────────────────────────────────────────────────┐
//	│                   Tick Loop                         │
//	│                                                     │
//	│  1. CollectObservedState() → Database               │
//	│  2. DeriveDesiredState() → Snapshot                 │
//	│  3. currentState.Next(snapshot)                     │
//	│       ↓                                             │
//	│     (nextState, signal, action)                     │
//	│  4. Execute action (with retry/backoff)             │
//	│  5. Transition to nextState                         │
//	│  6. Process signal (remove/restart)                 │
//	│  7. Reconcile children (if parent worker)           │
//	│                                                     │
//	└─────────────────────────────────────────────────────┘
//
// ## States: Structs, Not Strings
//
// States are concrete types implementing the State interface, providing:
//   - Compile-time type safety (no invalid states)
//   - Explicit transitions (visible in code)
//   - Encapsulated logic (each state is self-contained)
//
// Example:
//
//	// Type-safe transition ✅
//	return RunningState{}, fsmv2.SignalNone, nil
//
//	// Would not compile ❌
//	return "running", fsmv2.SignalNone, nil
//
// See PATTERNS.md for detailed explanation.
//
// ## Immutability: Pass-by-Value
//
// Snapshots are passed by value to State.Next(), making them inherently immutable.
// Go's pass-by-value semantics guarantee that states cannot modify the supervisor's data:
//
//	func (s MyState) Next(snapshot Snapshot) (State, Signal, Action) {
//	    // snapshot is a COPY - mutations only affect local copy
//	    snapshot.Identity.Name = "modified"  // Safe, doesn't affect supervisor
//	    return s, SignalNone, nil
//	}
//
// No getters or defensive copying needed - the language enforces immutability.
// See PATTERNS.md for detailed explanation.
//
// ## Actions: Idempotent Operations
//
// Actions represent side effects that transition the system between states.
// All actions MUST be idempotent - safe to retry after partial completion:
//
//	type StartProcessAction struct {
//	    ProcessPath string
//	}
//
//	func (a *StartProcessAction) Execute(ctx context.Context) error {
//	    // Check if already done (idempotency)
//	    if processIsRunning(a.ProcessPath) {
//	        return nil  // Already started, safe to call again
//	    }
//	    return startProcess(ctx, a.ProcessPath)
//	}
//
//	func (a *StartProcessAction) Name() string {
//	    return "StartProcess"
//	}
//
// The supervisor retries failed actions with exponential backoff, so idempotency is critical.
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
// See PATTERNS.md for detailed explanation.
//
// ## Variables: Three-Tier Namespace
//
// VariableBundle provides three namespaces for configuration:
//   - User: Top-level template access ({{ .IP }}, {{ .PORT }})
//   - Global: Fleet-wide settings ({{ .global.cluster_id }})
//   - Internal: Runtime metadata ({{ .internal.timestamp }}) - NOT serialized
//
// Example:
//
//	bundle := config.VariableBundle{
//	    User: map[string]any{"IP": "192.168.1.100", "PORT": 502},
//	    Global: map[string]any{"cluster_id": "prod"},
//	    Internal: map[string]any{"timestamp": time.Now()},
//	}
//
//	// Template rendering:
//	// {{ .IP }}                 → "192.168.1.100"
//	// {{ .global.cluster_id }}  → "prod"
//	// {{ .internal.timestamp }} → current time
//
// User and Global are serialized (persisted). Internal is runtime-only.
// See PATTERNS.md for detailed explanation.
//
// ## Hierarchical Composition: Parent-Child Workers
//
// Parents declare children via ChildSpec in DeriveDesiredState().
// Supervisor handles creation, updates, and cleanup automatically:
//
//	func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
//	    return config.DesiredState{
//	        State: "running",
//	        ChildrenSpecs: []config.ChildSpec{
//	            {
//	                Name:       "mqtt-connection",
//	                WorkerType: "mqtt_client",
//	                UserSpec:   config.UserSpec{Config: "url: tcp://localhost:1883"},
//	                StateMapping: map[string]string{
//	                    "active":  "connected",  // Parent state → child state
//	                    "closing": "stopped",
//	                },
//	            },
//	        },
//	    }, nil
//	}
//
// StateMapping coordinates FSM states (NOT data passing). Use VariableBundle for data.
//
// For practical examples of passing configuration data from parent to child workers,
// see PATTERNS.md "Pattern 10: Variable Passing (Parent → Child)" which includes
// complete cookbook examples showing:
//   - How to set variables in parent's ChildSpec
//   - How child accesses variables via Snapshot
//   - Template rendering with flattened variables
//   - Common patterns (static config, dynamic config, parent state propagation)
//
// See PATTERNS.md for detailed explanation.
//
// # Architecture Documentation
//
// For detailed architecture explanations, see:
//   - PATTERNS.md - Established patterns and design decisions
//   - api.go - Core interfaces (Worker, State, Action)
//   - supervisor/supervisor.go - Orchestration and lifecycle management
//   - types/childspec.go - Hierarchical composition
//   - types/variables.go - Variable namespaces
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
// States MUST check ShutdownRequested() first in Next():
//
//	func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
//	    // ALWAYS check shutdown first
//	    if snapshot.Desired.(config.DesiredState).ShutdownRequested() {
//	        return StoppingState{}, SignalNone, nil
//	    }
//	    // ... rest of logic
//	}
//
// ## Type-Safe Dependencies
//
// Use BaseWorker[D] for type-safe dependency access:
//
//	type MyWorker struct {
//	    fsmv2.BaseWorker[MyWorkerDeps]
//	}
//
//	func (w *MyWorker) someMethod() {
//	    deps := w.GetDependencies()
//	    deps.Logger.Info("...")  // Type-safe, no casting
//	}
//
// # Testing
//
// Test FSM workers by:
//   1. Creating test states and actions
//   2. Calling Next() with test snapshots
//   3. Verifying returned state, signal, and action
//   4. Testing action idempotency with VerifyActionIdempotency helper
//
// Example:
//
//	var _ = Describe("MyWorker", func() {
//	    It("transitions to running when started", func() {
//	        state := InitializingState{}
//	        snapshot := Snapshot{
//	            Identity: Identity{ID: "test"},
//	            Observed: &MyObservedState{Started: true},
//	            Desired:  config.DesiredState{State: "running"},
//	        }
//	        nextState, signal, action := state.Next(snapshot)
//	        Expect(nextState).To(BeAssignableToTypeOf(RunningState{}))
//	        Expect(signal).To(Equal(SignalNone))
//	        Expect(action).To(BeNil())
//	    })
//	})
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
//
// # Related Packages
//
//   - pkg/fsmv2/types - Common data structures (ChildSpec, DesiredState, VariableBundle)
//   - pkg/fsmv2/supervisor - Supervisor implementation and orchestration
//   - pkg/fsm/benthos - Example FSM worker implementation
//   - pkg/fsm/redpanda - Example FSM worker implementation
package fsmv2
