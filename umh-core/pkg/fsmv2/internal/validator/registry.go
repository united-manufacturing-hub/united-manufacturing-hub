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

package validator

// PatternRegistry maps pattern types to their WHY explanations.
// This registry provides educational context when violations are found.
var PatternRegistry = map[string]PatternInfo{
	"EMPTY_STATE": {
		Name: "Empty State Structs",
		Why: `States must be pure behavior with no fields (except embedded base types).
WHY: States represent WHERE in the lifecycle, not WHAT data exists. If a state
needs data, it comes from the Snapshot. This ensures states are stateless and
predictable - the same Snapshot always produces the same transition.`,
		CorrectCode: `type ConnectedState struct {
    BaseChildState  // OK: embedded base type
}
// NOT: type ConnectedState struct { retryCount int }`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"STATELESS_ACTION": {
		Name: "Stateless Actions",
		Why: `Actions must have no mutable state fields (no counters, no retry tracking).
WHY: Actions are idempotent commands - they can be retried without side effects.
If an action needs retry counts, those belong in ObservedState where the
supervisor can track them. This prevents subtle bugs where action state
persists across reconciliation cycles.`,
		CorrectCode: `type ConnectAction struct{}  // Stateless - OK
// NOT: type ConnectAction struct { failureCount int }`,
		ReferenceFile: "example-child/action/connect.go",
	},
	"MISSING_ENTRY_ASSERTION": {
		Name: "Single Entry-Point Type Assertion",
		Why: `Next() must have exactly ONE type assertion/conversion at the first statement.
WHY: Go doesn't support covariance, so State[any, any] is required for the Worker
interface. The single entry-point pattern ensures type safety: convert once at
entry, then use strongly-typed snapshot throughout. Multiple assertions or
late assertions indicate logic that should be refactored.`,
		CorrectCode: `func (s *MyState) Next(snapAny any) (...) {
    snap := helpers.ConvertSnapshot[ObsState, *DesState](snapAny)  // First line
    // Now use snap.Observed, snap.Desired with full type safety
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"MULTIPLE_ASSERTIONS": {
		Name: "Single Entry-Point Type Assertion",
		Why: `Next() must have exactly ONE type assertion/conversion at the first statement.
WHY: Multiple type assertions indicate scattered type handling. Each assertion
is a potential runtime panic. The single entry-point pattern centralizes type
conversion, making it easy to verify correctness and impossible to forget.`,
		CorrectCode: `func (s *MyState) Next(snapAny any) (...) {
    snap := helpers.ConvertSnapshot[ObsState, *DesState](snapAny)  // ONCE at entry
    // NOT: multiple conversions scattered through the method
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"ASSERTION_NOT_AT_ENTRY": {
		Name: "Single Entry-Point Type Assertion",
		Why: `The type assertion must be the FIRST statement in Next().
WHY: If code runs before the type assertion, it's operating on untyped data.
The entry-point pattern ensures all logic has access to typed snapshot data.
Early-exit optimizations should still go through the typed snapshot.`,
		CorrectCode: `func (s *MyState) Next(snapAny any) (...) {
    snap := helpers.ConvertSnapshot[...](snapAny)  // MUST be first
    if snap.Desired.IsShutdownRequested() { ... }
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"PURE_DERIVE": {
		Name: "Pure DeriveDesiredState",
		Why: `DeriveDesiredState() must not access dependencies directly.
WHY: This method converts UserSpec → DesiredState. It should be a pure
transformation based only on configuration, not runtime state. Accessing
dependencies here creates hidden coupling and makes the derivation non-
deterministic. Dependencies are for actions and observation, not derivation.`,
		CorrectCode: `func (w *Worker) DeriveDesiredState(spec MySpec) (*DesiredState, error) {
    return &DesiredState{ChildCount: spec.ChildCount}, nil
    // NOT: deps := w.GetDependencies(); deps.SomeService.Check()
}`,
		ReferenceFile: "example-child/worker.go",
	},
	"SHUTDOWN_CHECK_NOT_FIRST": {
		Name: "Shutdown Check First",
		Why: `Every state's Next() must check IsShutdownRequested() as the FIRST conditional.
WHY: Lifecycle states (stopping, shutdown) must ALWAYS override operational logic.
If a worker is being stopped, it should not try to reconnect, process data, or
perform any actions. Checking shutdown first prevents race conditions where a
worker starts an operation right before shutdown is requested.`,
		CorrectCode: `func (s *MyState) Next(snapAny any) (...) {
    snap := helpers.ConvertSnapshot[...](snapAny)
    if snap.Desired.IsShutdownRequested() {  // MUST be first conditional
        return &TryingToStopState{}, SignalNone, nil
    }
    // Then other logic...
}`,
		ReferenceFile: "example-child/state/state_connected.go:30",
	},
	"CHILD_MUST_USE_IS_STOP_REQUIRED": {
		Name: "Child Workers Must Use IsStopRequired()",
		Why: `Child workers must check IsStopRequired() instead of just IsShutdownRequested().
WHY: Child workers have TWO shutdown signals:
1. IsShutdownRequested() - explicit shutdown request
2. !ShouldBeRunning() - parent stopped via StateMapping

Using only IsShutdownRequested() misses parent lifecycle changes, causing
children to stay running when parent goes to TryingToStop.

IsStopRequired() = IsShutdownRequested() || !ShouldBeRunning()`,
		CorrectCode: `func (s *ChildState) Next(snapAny any) (...) {
    snap := helpers.ConvertSnapshot[...](snapAny)
    if snap.Observed.IsStopRequired() {  // NOT snap.Desired.IsShutdownRequested()
        return &TryingToStopState{}, SignalNone, nil
    }
    // Then other logic...
}`,
		ReferenceFile: "example-child/state/state_connected.go:33",
	},
	"STATE_AND_ACTION": {
		Name: "State Change XOR Action",
		Why: `Next() must return EITHER a new state OR an action, never both simultaneously.
WHY: Actions execute asynchronously. If the state changes at the same time,
the supervisor doesn't know which state to transition to after the action
completes. This rule ensures deterministic behavior: stay in current state →
execute action → observe result → then decide on state transition.`,
		CorrectCode: `// Correct: state change, no action
return &ConnectedState{}, SignalNone, nil

// Correct: same state, with action
return s, SignalNone, &ConnectAction{}

// WRONG: both state change AND action
return &ConnectedState{}, SignalNone, &SomeAction{}`,
		ReferenceFile: "example-child/state/state_trying_to_connect.go:38-41",
	},
	"MISSING_COLLECTED_AT": {
		Name: "ObservedState CollectedAt Timestamp",
		Why: `Every ObservedState struct must have a CollectedAt time.Time field.
WHY: The supervisor uses this timestamp to detect stale data. If a collector
crashes or hangs, stale timestamps trigger recovery mechanisms. Without
timestamps, the supervisor cannot distinguish between "recently collected"
and "collector is dead, data is hours old". This is critical for self-healing.`,
		CorrectCode:   "type MyObservedState struct {\n    ID          string    `json:\"id\"`\n    CollectedAt time.Time `json:\"collected_at\"`  // REQUIRED\n    // ... other fields\n}",
		ReferenceFile: "example-child/snapshot/snapshot.go:61",
	},
	"MISSING_IS_SHUTDOWN_REQUESTED": {
		Name: "DesiredState IsShutdownRequested Method",
		Why: `Every DesiredState type must implement IsShutdownRequested() bool.
WHY: This is the coordination mechanism for graceful shutdown. When the
supervisor needs to stop a worker, it sets shutdown=true in DesiredState.
States check this method to know when to transition to stopping state.
Without it, workers cannot respond to shutdown requests.`,
		CorrectCode: `type MyDesiredState struct {
    shutdownRequested bool
}

func (s *MyDesiredState) IsShutdownRequested() bool {
    return s.shutdownRequested
}`,
		ReferenceFile: "example-child/snapshot/snapshot.go:47-49",
	},
	"STATE_MISSING_STRING_METHOD": {
		Name: "State String() and Reason() Methods",
		Why: `Every State implementation MUST have String() and Reason() methods.
WHY: Without these methods, logs show memory addresses like "*example.ConnectedState"
instead of readable state names. Debugging FSM states becomes impossible, and support
teams can't diagnose issues from logs without reading source code. The String()
method enables clear logging and the Reason() method provides context for transitions.`,
		CorrectCode: `func (s *ConnectedState) String() string {
    return helpers.DeriveStateName(s)  // Returns "Connected"
}

func (s *ConnectedState) Reason() string {
    return "Active connection established"
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"NIL_STATE_RETURN": {
		Name: "No Nil State Returns",
		Why: `Next() must NEVER return nil as the first return value (state).
WHY: The supervisor assumes states are always non-nil. A nil state causes a
panic when calling state.String() or state.Next(). States should always
return a valid state - either the current state (s) or a new state type.
If "no change" is needed, return s (current state), not nil.`,
		CorrectCode: `// Correct: stay in same state
return s, SignalNone, nil

// Correct: transition to different state
return &StoppedState{}, SignalNone, nil

// WRONG: nil state causes panic
return nil, SignalNone, nil`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"SIGNAL_STATE_MISMATCH": {
		Name: "Signal-State Mutual Exclusion",
		Why: `When returning SignalNeeds* (removal/restart), the state must be the current state (s).
WHY: Signals indicate the worker needs external intervention (removal, restart).
If the state changes simultaneously, there's ambiguity: does the new state apply
before or after the signal is processed? By requiring signals only with same-state
returns, we ensure clear semantics: signal processing happens in the current state.`,
		CorrectCode: `// Correct: signal with same state
return s, SignalNeedsRemoval, nil

// Correct: state change without signal
return &StoppedState{}, SignalNone, nil

// WRONG: state change with signal
return &StoppedState{}, SignalNeedsRemoval, nil`,
		ReferenceFile: "example-child/state/state_stopped.go",
	},
	"MISSING_CONTEXT_CANCELLATION_COLLECT": {
		Name: "Context Cancellation in CollectObservedState",
		Why: `CollectObservedState() must handle context cancellation via select{case <-ctx.Done()}.
WHY: Collection runs with a timeout. Without context cancellation handling, the
method blocks until I/O completes (network timeout, disk read) even after the
context is cancelled. This delays shutdown and can cause timeouts to cascade.
Early exit on ctx.Done() enables responsive shutdown.`,
		CorrectCode: `func (w *Worker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()  // Early exit
    default:
    }
    // ... actual collection logic
}`,
		ReferenceFile: "example-child/worker.go",
	},
	"MISSING_NIL_SPEC_CHECK": {
		Name: "Nil Spec Handling in DeriveDesiredState",
		Why: `DeriveDesiredState() must check if spec == nil before type casting.
WHY: During shutdown or when no config exists, spec may be nil. Without a nil
check, the type assertion panics. This is the first defensive layer that
prevents runtime panics from configuration issues. Always check nil first,
then type-assert safely.`,
		CorrectCode: `func (w *Worker) DeriveDesiredState(spec any) (*DesiredState, error) {
    if spec == nil {
        return &DesiredState{shutdownRequested: true}, nil  // Graceful nil handling
    }
    userSpec, ok := spec.(MyUserSpec)
    if !ok {
        return nil, fmt.Errorf("invalid spec type: %T", spec)
    }
    // ... derive from userSpec
}`,
		ReferenceFile: "example-child/worker.go",
	},
	"MISSING_CONTEXT_CANCELLATION_ACTION": {
		Name: "Context Cancellation in Actions",
		Why: `Execute() methods must check ctx.Done() for cancellation.
WHY: Actions may involve I/O operations (network calls, file writes) that block.
Without context cancellation, the supervisor cannot abort long-running actions
during shutdown. This causes shutdown delays and resource leaks. Always provide
an escape hatch via ctx.Done() checks before and during blocking operations.`,
		CorrectCode: `func (a *ConnectAction) Execute(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return ctx.Err()  // Early exit
    default:
    }

    // For long operations, check periodically:
    for i := 0; i < retries; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := tryConnect(); err == nil {
                return nil
            }
        }
    }
    return errors.New("connection failed")
}`,
		ReferenceFile: "example-child/action/connect.go",
	},
	"INTERNAL_RETRY_LOOP": {
		Name: "No Internal Retry Loops in Actions",
		Why: `Actions must NOT implement internal retry loops with error handling.
WHY: The supervisor manages retries with exponential backoff. If an action
has its own retry loop, it fights the supervisor's backoff strategy, causing:
1. Double retry delays (action retries × supervisor retries)
2. No visibility into failure counts (hidden inside action)
3. Difficulty tuning backoff parameters
Let the action fail fast; the supervisor handles retries.`,
		CorrectCode: `// Correct: single attempt, let supervisor retry
func (a *ConnectAction) Execute(ctx context.Context) error {
    return doConnect(ctx)  // Fails or succeeds once
}

// WRONG: internal retry loop
func (a *ConnectAction) Execute(ctx context.Context) error {
    for i := 0; i < 3; i++ {
        if err := doConnect(ctx); err == nil {
            return nil
        }
        time.Sleep(time.Second)  // Fighting supervisor backoff!
    }
    return errors.New("failed after retries")
}`,
		ReferenceFile: "example-child/action/connect.go",
	},
	"VALUE_RECEIVER_ON_WORKER": {
		Name: "Pointer Receivers on Workers",
		Why: `All Worker interface methods must use pointer receivers (*T).
WHY: Workers contain state (dependencies, configuration) that must persist
across method calls. Value receivers create copies, losing mutations.
More critically, Go interfaces work differently with pointer vs value
receivers - inconsistent receiver types cause subtle interface satisfaction
bugs where methods "disappear" depending on how the type is used.`,
		CorrectCode: `// Correct: pointer receiver
func (w *MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    return w.doCollection(ctx)
}

// WRONG: value receiver loses state
func (w MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    return w.doCollection(ctx)  // Changes to w are lost!
}`,
		ReferenceFile: "example-child/worker.go",
	},
	"TRYINGTO_NO_ACTION": {
		Name: "TryingTo States Return Actions",
		Why: `States named "TryingTo*" MUST return non-nil actions in at least one code path.
WHY: The "TryingTo" prefix indicates an active state that performs work to achieve
a goal. If it never returns an action, it's passive (should be renamed) or broken.
This naming convention helps developers understand state behavior at a glance.`,
		CorrectCode: `// TryingTo prefix = MUST have action
func (s *TryingToConnectState) Next(snap any) (...) {
    // At least one path returns an action
    return s, SignalNone, &ConnectAction{}
}`,
		ReferenceFile: "example-child/state/state_trying_to_connect.go",
	},
	"MISSING_CATCHALL_RETURN": {
		Name: "Exhaustive Transition Coverage",
		Why: `Next() methods should end with a catch-all return: "return s, SignalNone, nil".
WHY: This ensures the FSM always has a valid transition. Without a catch-all,
edge cases may cause undefined behavior. The catch-all is a safety net that
maintains the current state when no explicit transition matches.`,
		CorrectCode: `func (s *MyState) Next(snap any) (...) {
    if condition1 { return &State1{}, ... }
    if condition2 { return &State2{}, ... }
    // Catch-all: stay in current state
    return s, SignalNone, nil
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"MISSING_BASE_STATE": {
		Name: "Base State Type Embedding",
		Why: `State structs should embed exactly one Base*State type.
WHY: Base states provide common functionality (logging, metrics) and enforce
the type hierarchy. Without embedding, states lose these capabilities and
may not properly satisfy interfaces.`,
		CorrectCode: `type ConnectedState struct {
    BaseChildState  // Embeds base functionality
}`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"MISSING_DEPENDENCY_VALIDATION": {
		Name: "Dependency Validation in Constructors",
		Why: `NewXxxWorker constructors should validate required dependencies are non-nil.
WHY: Nil dependencies cause panics at runtime, often deep in call stacks where
the root cause is hard to identify. Early validation in constructors provides
clear error messages and fail-fast behavior.`,
		CorrectCode: `func NewMyWorker(deps Dependencies) (*MyWorker, error) {
    if deps.Logger == nil {
        return nil, errors.New("logger is required")
    }
    return &MyWorker{deps: deps}, nil
}`,
		ReferenceFile: "example-child/worker.go",
	},
	"MISSING_CHILDSPEC_VALIDATION": {
		Name: "Child Spec Validation",
		Why: `DeriveDesiredState should validate ChildrenSpecs before returning them.
WHY: Invalid child specs cause supervisor errors during reconciliation. Early
validation in DeriveDesiredState provides clear error messages and prevents
partial deployments where some children fail.`,
		CorrectCode: `func (w *Worker) DeriveDesiredState(spec MySpec) (*DesiredState, error) {
    for _, child := range spec.Children {
        if child.Name == "" {
            return nil, errors.New("child name is required")
        }
    }
    return &DesiredState{Children: spec.Children}, nil
}`,
		ReferenceFile: "exampleparent/worker.go",
	},
	"CHANNEL_OPERATION_IN_ACTION": {
		Name: "No Channel Operations in Actions",
		Why: `Execute() methods should not use goroutines, channels, or channel operations.
WHY: Actions are synchronous operations managed by the supervisor. Spawning
goroutines or using channels creates concurrent state that the supervisor
can't track. This leads to resource leaks on shutdown and race conditions.
If async work is needed, model it as a state transition instead.`,
		CorrectCode: `// Correct: synchronous operation
func (a *Action) Execute(ctx context.Context) error {
    return doSyncOperation(ctx)
}

// WRONG: spawns goroutine
func (a *Action) Execute(ctx context.Context) error {
    go doAsyncWork()  // Supervisor can't track this!
    return nil
}`,
		ReferenceFile: "example-child/action/connect.go",
	},
	"OBSERVED_STATE_NOT_EMBEDDING_DESIRED": {
		Name: "ObservedState Must Embed DesiredState",
		Why: `ObservedState structs must embed their DesiredState type anonymously with json:",inline" tag.
WHY: This ensures that all desired state fields are automatically included in the observed
state, maintaining consistency between what is desired and what is observed. The inline
tag prevents nested JSON structure and keeps the serialized format flat. Using named fields
instead of embedding leads to duplication and potential inconsistency.`,
		CorrectCode: `type MyObservedState struct {
    CollectedAt time.Time ` + "`json:\"collected_at\"`" + `

    MyDesiredState ` + "`json:\",inline\"`" + `  // REQUIRED: anonymous embedding with inline tag

    // Observed-only fields
    ActualValue int ` + "`json:\"actual_value\"`" + `
}

// WRONG: named field instead of embedding
type MyObservedState struct {
    Desired MyDesiredState  // This is wrong - should be anonymous
}`,
		ReferenceFile: "example-child/snapshot/snapshot.go",
	},
	"MISSING_STATE_FIELD": {
		Name: "State Field Required in DesiredState and ObservedState",
		Why: `Both DesiredState and ObservedState structs must have a State string field.
WHY: The State field represents the current lifecycle state of the worker (e.g., "running",
"stopped"). This field is essential for FSM coordination and state tracking. Without it,
the supervisor cannot determine what state the worker should be in (desired) or what state
it is currently in (observed). This field enables the FSM to make correct state transition
decisions.`,
		CorrectCode: `type MyDesiredState struct {
    config.BaseDesiredState
    State string ` + "`json:\"state\"`" + `  // REQUIRED: lifecycle state
    // Other desired fields...
}

type MyObservedState struct {
    CollectedAt time.Time ` + "`json:\"collected_at\"`" + `
    MyDesiredState ` + "`json:\",inline\"`" + `
    State string ` + "`json:\"state\"`" + `  // REQUIRED: current state
    // Other observed fields...
}`,
		ReferenceFile: "example-child/snapshot/snapshot.go",
	},
	"INVALID_DESIRED_STATE_VALUE": {
		Name: "DesiredState.State Must Be 'stopped' or 'running'",
		Why: `DesiredState.State MUST only be "stopped" or "running".

WHY: These represent user INTENT (lifecycle commands), not current state.
- "running": User wants the component active
- "stopped": User wants the component inactive

Any other value (like "connected", "starting") confuses DESIRED state
(what we want) with OBSERVED state (what we have). This causes FSM errors
and unpredictable behavior.

NOTE: This is validated BOTH at test-time (AST checks in worker.go DeriveDesiredState)
AND at runtime (user YAML config validation). Runtime errors are user-facing
and follow UX_STANDARDS.md Error Excellence - provide actionable guidance.`,
		CorrectCode: `// Correct: use constants in DeriveDesiredState
return config.DesiredState{
    State: config.DesiredStateRunning,  // Use constants!
}

// Also valid: literal strings
return config.DesiredState{
    State: "running",  // OK but prefer constants
}

// WRONG: invalid state values
return config.DesiredState{
    State: "connected",  // FSM operational state, not lifecycle intent
    State: "starting",   // Transitional state, not user intent
    State: "active",     // Ambiguous, use "running" instead
}`,
		ReferenceFile: "workers/example/examplechild/worker.go",
	},
	"MISSING_SET_STATE_METHOD": {
		Name: "ObservedState Must Have SetState Method",
		Why: `ObservedState types must implement SetState(string) method.
WHY: The Supervisor injects a StateProvider callback into the Collector. When collecting
observed state, the Collector calls StateProvider to get the current FSM state name, then
calls SetState on the observed state to inject it. Without this method, the State field
in ObservedState remains empty - the "observed state is only in collector" boundary is
preserved while still allowing FSM state to be observed.`,
		CorrectCode: `type MyObservedState struct {
    CollectedAt time.Time ` + "`json:\"collected_at\"`" + `
    State       string    ` + "`json:\"state\"`" + `
    // ... other fields
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o *MyObservedState) SetState(s string) {
    o.State = s
}`,
		ReferenceFile: "workers/application/snapshot/snapshot.go",
	},
	"FOLDER_WORKER_TYPE_MISMATCH": {
		Name: "Folder Name Must Match Worker Type",
		Why: `Worker folder names must exactly match the derived worker type.
WHY: The factory registration system uses type names to derive worker types:
"ExamplechildObservedState" → "examplechild". If the folder is named differently (e.g., "example-child"),
it creates confusion and enables registration mismatches where supervisor factory
registers as "examplechild" but worker factory registers manually as "example-child".
Go type names cannot contain hyphens, so hyphenated folder names can NEVER match.`,
		CorrectCode: `// Folder "examplechild" with type ExamplechildObservedState → worker type "examplechild" ✓
workers/example/examplechild/snapshot/snapshot.go:
    type ExamplechildObservedState struct { ... }

// Folder "exampleparent" with type ExampleparentObservedState → "exampleparent" ✓
workers/example/exampleparent/snapshot/snapshot.go:
    type ExampleparentObservedState struct { ... }

// WRONG: Folder "example-child" can never match (hyphens invalid in Go types)`,
		ReferenceFile: "workers/example/examplechild/snapshot/snapshot.go",
	},
	"DEPENDENCIES_IN_DESIRED_STATE": {
		Name: "Dependencies Not Allowed in DesiredState",
		Why: `DesiredState structs must NOT have a Dependencies field.
WHY: DesiredState is pure configuration that can be serialized to JSON/YAML.
Dependencies are runtime interfaces (loggers, clients, services) that belong
in Worker, not in DesiredState. Including dependencies in DesiredState breaks
serialization and creates tight coupling between configuration and runtime.`,
		CorrectCode: `// Correct: Dependencies in Worker, not DesiredState
type MyWorker struct {
    deps Dependencies  // Runtime dependencies here
}

type MyDesiredState struct {
    config.BaseDesiredState
    State string  // Configuration only
}

// WRONG: Dependencies in DesiredState
type MyDesiredState struct {
    Dependencies *Dependencies  // This is forbidden
}`,
		ReferenceFile: "example-child/snapshot/snapshot.go",
	},
	"REGISTRY_MISMATCH": {
		Name: "Worker/Supervisor Registry Mismatch",
		Why: `Worker and supervisor registries must contain the same type names.
WHY: Every worker type needs both a worker factory (creates Worker instances) and
a supervisor factory (creates Supervisor instances). If only one is registered,
the FSM system will panic at runtime when trying to create the missing component.
Use RegisterWorkerType[TObserved, TDesired]() to register both atomically.`,
		CorrectCode: `func init() {
    err := factory.RegisterWorkerType[snapshot.MyObservedState, *snapshot.MyDesiredState](
        func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
            worker, _ := NewMyWorker(id, logger)
            return worker
        },
        func(cfg interface{}) interface{} {
            return supervisor.NewSupervisor[snapshot.MyObservedState, *snapshot.MyDesiredState](
                cfg.(supervisor.Config))
        },
    )
    if err != nil {
        panic(err)
    }
}`,
		ReferenceFile: "factory/worker_factory.go:451-461",
	},
}

// GetPattern returns pattern info for a violation type.
func GetPattern(patternType string) (PatternInfo, bool) {
	pattern, ok := PatternRegistry[patternType]
	return pattern, ok
}
