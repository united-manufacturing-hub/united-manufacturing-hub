# FSM v2 Architecture Improvements

**Analysis Date**: November 8, 2025
**Scope**: Comprehensive evaluation of FSM v2 architecture for non-intuitive patterns
**Methodology**: Architecture simplification + collision-zone thinking + Go idioms analysis

---

## Executive Summary

Five key findings emerged from analyzing FSM v2 worker architecture:

1. **Snapshot immutability is elegant but implicit** - Pass-by-value is clever Go usage but requires understanding Go semantics. Recommendation: Add compile-time markers.

2. **interface{} for Observed/Desired loses type safety** - Current approach trades type safety for flexibility. Recommendation: Provide typed wrapper helpers.

3. **State.Next() return signature is packed** - (State, Signal, Action) tuple is correct granularity but violates "state transition OR action, not both" rule. Recommendation: Enforce at runtime.

4. **VariableBundle 3-tier namespace is non-obvious** - User/Global/Internal distinction with different flattening rules confuses developers. Recommendation: Simplify or document intensively.

5. **"TryingTo" vs descriptive nouns naming works well** - Clear semantic distinction between active/passive states. Recommendation: Keep pattern, codify in linter.

**Impact**: None of these issues are architectural flaws. They represent trade-offs between simplicity, type safety, and flexibility. Most improvements are documentation, testing helpers, or linting rules rather than breaking changes.

---

## Analysis Methodology

This analysis applies multiple problem-solving approaches:

### 1. Architecture Simplification (from skills/architecture/architecture-simplification)
- **Conversion counting**: Counting transformation steps in DeriveDesiredState pipeline
- **Direct path analysis**: Checking if intermediate types are justified
- **YAGNI principle**: Verifying abstractions solve concrete problems, not speculation

### 2. Collision-Zone Thinking (from skills/problem-solving/collision-zone-thinking)
- **Metaphor-mixing**: Treating FSM like other domain patterns (circuit breakers, DNA, etc.)
- **Emergent properties**: What new capabilities appear from analogies?
- **Boundary testing**: Where do metaphors break down?

### 3. Go Idioms Comparison
- **Standard library patterns**: How do similar patterns work in Go stdlib?
- **Error handling**: Do we follow Go conventions?
- **Interface design**: Are interfaces minimal and focused?

### 4. Root Cause Analysis
- **Why does pattern exist?**: Historical context and original problem
- **Is problem still relevant?**: Has landscape changed?
- **What would happen without it?**: Cost-benefit of removal

---

## Category 1: API Design Issues

### Issue 1.1: Snapshot Immutability via Pass-by-Value is Implicit

**Current Pattern**:
```go
// worker.go lines 82-114
type Snapshot struct {
    Identity Identity
    Observed interface{}
    Desired  interface{}
}

// State.Next receives Snapshot by VALUE (copied)
Next(snapshot Snapshot) (State, Signal, Action)
```

**Why Non-Intuitive**:
- Immutability depends on understanding Go's pass-by-value semantics
- Interface fields contain pointers, so field mutation IS possible
- Documentation says "immutable" but nothing enforces it at compile time
- New developers may not realize modification is safe-but-useless

**Root Cause**:
Design choice to leverage Go's language-level pass-by-value rather than defensive copying. This is actually elegant and efficient, but the immutability guarantee is implicit rather than explicit.

**Alternatives**:

**Alt 1: Keep current + Add compile-time marker**
```go
// Use struct tag to mark immutable types
type Snapshot struct {
    Identity Identity    `immutable:"true"`
    Observed interface{} `immutable:"true"`
    Desired  interface{} `immutable:"true"`
}

// Custom linter enforces no assignments to tagged fields
```

**Alt 2: Make fields private with getters**
```go
type Snapshot struct {
    identity Identity
    observed interface{}
    desired  interface{}
}

func (s Snapshot) Identity() Identity    { return s.identity }
func (s Snapshot) Observed() interface{} { return s.observed }
func (s Snapshot) Desired() interface{}  { return s.desired }
```

**Alt 3: Use explicit ReadOnlySnapshot interface**
```go
type ReadOnlySnapshot interface {
    Identity() Identity
    Observed() interface{}
    Desired() interface{}
}

// States receive interface, not struct
Next(snapshot ReadOnlySnapshot) (State, Signal, Action)
```

**Trade-offs**:
| Approach | Type Safety | Performance | Developer UX | Migration Cost |
|----------|------------|-------------|--------------|----------------|
| Current | Medium | Best | Requires knowledge | N/A |
| Alt 1 (marker) | Medium | Best | Better docs | Low (additive) |
| Alt 2 (private) | High | Good | Clearest | High (breaking) |
| Alt 3 (interface) | High | Good | Clear | High (breaking) |

**Recommendation**: **Keep current + Add Alt 1 (compile-time marker)**

**Rationale**:
- Pass-by-value is idiomatic Go and performs well
- Problem is discoverability, not correctness
- Custom linter (`go vet` check) can enforce immutability at build time
- No breaking changes, additive only

**Priority**: Low
**Migration Complexity**: Easy (additive documentation + optional linter)

---

### Issue 1.2: interface{} for Snapshot.Observed/Desired Loses Type Safety

**Current Pattern**:
```go
type Snapshot struct {
    Identity Identity
    Observed interface{} // Could be ObservedState or anything
    Desired  interface{} // Could be DesiredState or anything
}

// States must type-assert every time
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
    observed := snapshot.Observed.(*MyObservedState)  // Can panic!
    desired := snapshot.Desired.(*MyDesiredState)
    // ...
}
```

**Why Non-Intuitive**:
- Type assertions can panic if supervisor passes wrong type
- No compile-time verification that state and snapshot types match
- Errors only discovered at runtime
- Boilerplate type assertion in every Next() method

**Root Cause**:
FSM v2 needs to support multiple worker types with different observed/desired state structures. Using interface{} provides maximum flexibility at the cost of type safety.

**Alternatives**:

**Alt 1: Generics (Go 1.18+)**
```go
type Snapshot[O ObservedState, D DesiredState] struct {
    Identity Identity
    Observed O
    Desired  D
}

type State[O ObservedState, D DesiredState] interface {
    Next(snapshot Snapshot[O, D]) (State[O, D], Signal, Action)
}

// Usage: type-safe, no assertions needed
type MyState struct{}

func (s *MyState) Next(snapshot Snapshot[MyObservedState, MyDesiredState]) (State[MyObservedState, MyDesiredState], Signal, Action) {
    // snapshot.Observed is typed as MyObservedState
    // snapshot.Desired is typed as MyDesiredState
    return s, SignalNone, nil
}
```

**Alt 2: Helper Methods with Type Checking**
```go
// Add to Snapshot type
func (s Snapshot) ObservedAs(target ObservedState) (ObservedState, error) {
    if s.Observed == nil {
        return nil, fmt.Errorf("observed state is nil")
    }
    // Reflection-based type checking with helpful error messages
    return s.Observed, nil
}

// Usage: safer than raw type assertion
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
    observed, err := snapshot.ObservedAs(&MyObservedState{})
    if err != nil {
        // Graceful error handling instead of panic
    }
}
```

**Alt 3: Type Registration System**
```go
// Worker registers its types at init
func init() {
    fsmv2.RegisterWorkerTypes("communicator",
        reflect.TypeOf(&CommunicatorObservedState{}),
        reflect.TypeOf(&CommunicatorDesiredState{}))
}

// Supervisor validates types before calling Next()
// Panics early with clear message if mismatch
```

**Trade-offs**:
| Approach | Type Safety | Complexity | Backward Compat | Code Clarity |
|----------|------------|-----------|----------------|--------------|
| Current | Low | Low | N/A | Medium |
| Alt 1 (generics) | High | High | Breaking | High |
| Alt 2 (helpers) | Medium | Low | Compatible | Medium |
| Alt 3 (registration) | Medium | Medium | Compatible | Medium |

**Recommendation**: **Alt 2 (helper methods) in short term, Alt 1 (generics) in FSM v3**

**Rationale**:
- Alt 2 is backward-compatible and improves error messages immediately
- Alt 1 (generics) is the correct long-term solution but requires FSM v3
- Generics add compile-time safety but increase cognitive load
- Type registration (Alt 3) catches errors earlier but adds boilerplate

**Priority**: Medium
**Migration Complexity**:
- Alt 2: Easy (additive helper methods)
- Alt 1: Hard (requires FSM v3, breaks all existing workers)

---

### Issue 1.3: State.Next() Return Signature Allows Invalid Combinations

**Current Pattern**:
```go
// worker.go lines 210-241
Next(snapshot Snapshot) (State, Signal, Action)

// Documentation says:
// "Should not switch the state and emit an action at the same time"
// "supervisor should check for this and panic if this happens"
```

**Why Non-Intuitive**:
- API allows invalid combinations: `(newState, signal, action)` where both state change AND action occur
- Runtime panic is the enforcement mechanism, not compile-time prevention
- Documentation mentions this as a "should not" but type system permits it
- Application logic bug, not caught until runtime

**Root Cause**:
Design choice to use tuple return instead of sum type (either state transition OR action). Go doesn't have sum types, so tuple is the pragmatic choice.

**Alternatives**:

**Alt 1: Keep Current + Runtime Validation in Tests**
```go
// Add helper for state tests
func VerifyStateTransitionRules(state State, snapshot Snapshot) error {
    nextState, signal, action := state.Next(snapshot)

    if nextState != state && action != nil {
        return fmt.Errorf(
            "invalid: state transition (%T → %T) and action (%s) both present",
            state, nextState, action.Name(),
        )
    }
    return nil
}

// Usage in tests
It("should follow state transition rules", func() {
    err := VerifyStateTransitionRules(state, snapshot)
    Expect(err).ToNot(HaveOccurred())
})
```

**Alt 2: Split into Two Methods**
```go
type State interface {
    // Returns (nextState, signal) - NO action
    EvaluateTransition(snapshot Snapshot) (State, Signal)

    // Returns action to execute in current state
    // Called ONLY if EvaluateTransition returned self
    GetAction(snapshot Snapshot) Action
}

// Supervisor enforces: if nextState != current, don't call GetAction
```

**Alt 3: Use Discriminated Union Pattern**
```go
type StateDecision struct {
    Signal Signal

    // Exactly one of these is non-nil
    Transition *StateTransition
    Action     Action
}

type StateTransition struct {
    NextState State
}

type State interface {
    Next(snapshot Snapshot) StateDecision
}

// Supervisor validates only one field is non-nil
```

**Alt 4: Separate Active vs Passive States at Type Level**
```go
type PassiveState interface {
    State
    // Only returns state transitions, never actions
    EvaluateConditions(snapshot Snapshot) (PassiveState, Signal)
}

type ActiveState interface {
    State
    // Returns action but stays in self
    GetAction(snapshot Snapshot) (Action, Signal)
}

// Compiler enforces passive states can't emit actions
```

**Trade-offs**:
| Approach | Compile Safety | Runtime Overhead | Code Clarity | Breaking Change |
|----------|---------------|------------------|--------------|-----------------|
| Current | None | Low | Medium | N/A |
| Alt 1 (test helper) | None | None | Medium | No |
| Alt 2 (split methods) | Partial | Low | Higher | Yes |
| Alt 3 (union) | Partial | Medium | Higher | Yes |
| Alt 4 (type split) | High | Low | Highest | Yes |

**Recommendation**: **Alt 1 (test helper) now, Alt 4 (type split) for FSM v3**

**Rationale**:
- Alt 1 catches bugs in tests without breaking changes
- Alt 4 encodes active/passive distinction in type system (best long-term)
- Alt 2 and Alt 3 are intermediate solutions with breaking changes but less type safety than Alt 4
- Active/passive split is already a naming convention, make it a type system guarantee

**Priority**: High (affects correctness)
**Migration Complexity**:
- Alt 1: Easy (additive test helper)
- Alt 4: Hard (FSM v3, requires rewriting all states)

---

### Issue 1.4: CollectObservedState vs DeriveDesiredState Separation

**Current Pattern**:
```go
type Worker interface {
    // Async goroutine, polls actual state
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // Sync call, derives from config
    DeriveDesiredState(spec interface{}) (types.DesiredState, error)
}
```

**Why Non-Intuitive**:
- Two separate methods that produce halves of the snapshot
- Names don't clearly convey "async vs sync" distinction
- "Derive" suggests computation, "Collect" suggests I/O, but both are transformations
- Not obvious why they can't be unified

**Root Cause**:
Different execution contexts and lifecycles:
- CollectObservedState: Runs in separate goroutine with timeout, may fail repeatedly
- DeriveDesiredState: Called on main tick, must be fast and deterministic

**Alternatives**:

**Alt 1: Rename for Clarity**
```go
type Worker interface {
    // Make async nature explicit
    MonitorActualState(ctx context.Context) (ObservedState, error)

    // Make deterministic nature explicit
    ComputeDesiredState(spec interface{}) (types.DesiredState, error)
}
```

**Alt 2: Unify with Execution Mode**
```go
type Worker interface {
    GetState(mode StateMode, input interface{}) (interface{}, error)
}

type StateMode int
const (
    StateMode_Observed StateMode = iota  // Async, can fail
    StateMode_Desired                     // Sync, pure function
)

// Supervisor calls:
// observed, _ := worker.GetState(StateMode_Observed, nil)
// desired, _ := worker.GetState(StateMode_Desired, spec)
```

**Alt 3: Keep Separate but Add Intent Comments**
```go
type Worker interface {
    // ASYNC: Runs in separate goroutine, can fail, returns actual system state
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // SYNC: Pure function, fast, computes desired state from config
    DeriveDesiredState(spec interface{}) (types.DesiredState, error)
}
```

**Trade-offs**:
| Approach | Clarity | API Surface | Backward Compat | Go Idioms |
|----------|---------|-------------|-----------------|-----------|
| Current | Medium | Minimal | N/A | Good |
| Alt 1 (rename) | Higher | Minimal | Breaking | Good |
| Alt 2 (unify) | Lower | Minimal | Breaking | Poor |
| Alt 3 (comments) | Higher | Minimal | Compatible | Good |

**Recommendation**: **Keep current separation, add Alt 3 (intent comments)**

**Rationale**:
- Separation is architecturally correct (different execution models)
- Alt 2 (unification) hides important distinction
- Alt 1 (rename) is better but breaks all workers for modest clarity gain
- Alt 3 (comments) achieves 80% of benefit with zero breaking changes
- "Collect" vs "Derive" actually conveys async vs sync reasonably well

**Priority**: Low
**Migration Complexity**: Easy (documentation only)

---

## Category 2: Naming and Conventions

### Issue 2.1: "TryingTo" vs "Ensuring" vs Descriptive Nouns

**Current Pattern**:
```go
// Active states: "TryingTo" prefix
type TryingToAuthenticateState struct{}  // Emits action every tick
type TryingToStartState struct{}

// Passive states: Descriptive nouns
type RunningState struct{}  // Observes only
type StoppedState struct{}
type SyncingState struct{}   // Wait, this loops too?
```

**Why Non-Intuitive**:
- "TryingTo" clearly indicates retrying behavior
- "Syncing" is passive noun but actually emits actions (line 912: `return s, SignalNone, syncAction`)
- Inconsistency: SyncingState is active (emits actions) but uses passive naming
- TODO comment in worker.go line 191: "Clarify naming to avoid confusion between trying vs confirming"

**Root Cause**:
Naming convention evolved to distinguish retry loops (TryingTo) from stable states, but SyncingState doesn't fit the pattern - it's a stable operational state that continuously emits actions.

**Alternatives**:

**Alt 1: "Ensuring" Pattern (from worker.go TODO line 193)**
```go
// Captures both action and verification
type EnsuringAuthenticatedState struct{}  // Tries AND confirms
type EnsuringStartedState struct{}
type EnsuringStoppedState struct{}

// Or for ongoing operations:
type EnsuringSyncState struct{}  // Continuously ensures sync
```

**Alt 2: Add "Operational" Prefix for Long-Running Actions**
```go
type OperationalSyncingState struct{}  // Active but stable
type OperationalMonitoringState struct{}

// Distinguishes from:
type TryingToStartState struct{}  // Active until condition met
```

**Alt 3: Use Verb Gerunds Consistently for Active States**
```go
// Active states all use -ing verbs
type AuthenticatingState struct{}  // Clear it's doing something
type StartingState struct{}
type SyncingState struct{}         // Already follows this

// Passive states use nouns
type RunningState struct{}
type StoppedState struct{}
```

**Alt 4: Split Active into "Initializing" vs "Operational"**
```go
// Initializing actions (transient)
type InitializingAuthState struct{}  // Runs until success
type InitializingStartState struct{}

// Operational actions (continuous)
type OperationalSyncState struct{}  // Runs indefinitely
type OperationalMonitorState struct{}

// Passive observation (no actions)
type RunningState struct{}
type StoppedState struct{}
```

**Trade-offs**:
| Approach | Clarity | Consistency | Backward Compat | Verbosity |
|----------|---------|-------------|-----------------|-----------|
| Current | Medium | Low | N/A | Low |
| Alt 1 (Ensuring) | High | High | Breaking | Medium |
| Alt 2 (Operational) | High | Medium | Breaking | High |
| Alt 3 (Gerunds) | Medium | High | Breaking | Low |
| Alt 4 (Init/Op split) | Highest | Highest | Breaking | High |

**Recommendation**: **Alt 3 (gerund verbs) for FSM v3, document current pattern now**

**Rationale**:
- Alt 3 simplifies to two categories: "-ing verbs" (active) vs "nouns" (passive)
- SyncingState already follows this pattern
- "Ensuring" (Alt 1) is clever but unfamiliar to most developers
- "Operational" (Alt 2, Alt 4) adds extra word without enough benefit
- Current pattern works but needs clearer documentation of "SyncingState exception"

**Priority**: Medium (affects code readability)
**Migration Complexity**: Medium (requires renaming all states in FSM v3)

---

### Issue 2.2: ObservedState vs DesiredState Naming

**Current Pattern**:
```go
type ObservedState interface {
    GetObservedDesiredState() DesiredState  // Contains desired state copy
    GetTimestamp() time.Time
}

type DesiredState interface {
    ShutdownRequested() bool
}
```

**Why Non-Intuitive**:
- ObservedState contains a DesiredState (method: `GetObservedDesiredState()`)
- Mental model confusion: "What I observe" vs "What I want" vs "What I observe wanting"
- Name doesn't convey "what's actually deployed" vs "what we want to deploy"

**Root Cause**:
worker.go line 64-66: "GetObservedDesiredState returns the desired state that is actually deployed. This allows comparing what's deployed vs what we want to deploy."

The abstraction is correct (need both "actual config" and "target config"), but names don't convey this relationship clearly.

**Alternatives**:

**Alt 1: Rename ObservedDesiredState to DeployedState**
```go
type ObservedState interface {
    GetDeployedState() DesiredState  // Clear: what's currently deployed
    GetTimestamp() time.Time
}

// Usage:
deployed := observed.GetDeployedState()
target := desired
if deployed != target {
    // Reconfigure
}
```

**Alt 2: Use "Actual" vs "Target" Terminology**
```go
type ActualState interface {
    GetCurrentConfig() TargetState
    GetTimestamp() time.Time
}

type TargetState interface {
    ShutdownRequested() bool
}
```

**Alt 3: Keep Names, Improve Documentation**
```go
type ObservedState interface {
    // Returns the configuration that is CURRENTLY deployed/active.
    // Compare with DesiredState to detect drift.
    GetObservedDesiredState() DesiredState

    GetTimestamp() time.Time
}
```

**Trade-offs**:
| Approach | Clarity | Familiarity | Breaking | Verbosity |
|----------|---------|-------------|----------|-----------|
| Current | Low | Low | N/A | Medium |
| Alt 1 (Deployed) | High | High | Yes | Low |
| Alt 2 (Actual/Target) | High | Medium | Yes | Low |
| Alt 3 (docs) | Medium | Low | No | Low |

**Recommendation**: **Alt 1 (DeployedState) for FSM v3**

**Rationale**:
- "Deployed" clearly means "what's currently running"
- Aligns with DevOps terminology (deployed vs desired)
- Simpler than "ObservedDesiredState" tongue-twister
- Alt 2 (Actual/Target) is clearer but less familiar to Kubernetes/DevOps users

**Priority**: Low (works correctly, just confusing name)
**Migration Complexity**: Medium (FSM v3, rename interface method)

---

### Issue 2.3: Worker vs Supervisor Naming

**Current Pattern**:
```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(spec interface{}) (types.DesiredState, error)
    GetInitialState() State
}

// Supervisor manages Worker lifecycle
```

**Why Non-Intuitive**:
- "Worker" suggests it does work, but it mostly defines interfaces
- "Supervisor" suggests management, but it also drives the FSM loop
- Names don't convey that Worker is "business logic definition" and Supervisor is "FSM engine"

**Root Cause**:
Naming borrowed from Erlang/OTP supervision trees, but Go developers may not have that context.

**Alternatives**:

**Alt 1: Use "Controller" Pattern from Kubernetes**
```go
type Controller interface {
    ObserveState(ctx context.Context) (ObservedState, error)
    ComputeDesiredState(spec interface{}) (types.DesiredState, error)
    GetInitialState() State
}

// Reconciler drives the control loop
type Reconciler struct {
    controller Controller
    // ...
}
```

**Alt 2: Use "Agent" Pattern from Distributed Systems**
```go
type Agent interface {
    // Agent observes and decides
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(spec interface{}) (types.DesiredState, error)
    GetInitialState() State
}

// Engine executes agent decisions
type Engine struct {
    agent Agent
    // ...
}
```

**Alt 3: Keep Current, Add Clarifying Comments**
```go
// Worker defines the business logic and state machine behavior.
// Supervisor executes the FSM loop and manages Worker lifecycle.
type Worker interface { /* ... */ }
```

**Trade-offs**:
| Approach | Clarity | Familiarity | Breaking | Ecosystem Fit |
|----------|---------|-------------|----------|---------------|
| Current | Medium | High (Erlang) | N/A | FSM/Erlang |
| Alt 1 (Controller) | High | High (K8s) | Yes | Kubernetes |
| Alt 2 (Agent) | Medium | Medium | Yes | Distributed Sys |
| Alt 3 (docs) | Medium | High (Erlang) | No | FSM/Erlang |

**Recommendation**: **Keep Worker/Supervisor with Alt 3 (clarifying docs)**

**Rationale**:
- Worker/Supervisor is established pattern from Erlang/OTP
- UMH already uses Kubernetes patterns, but Controller is overloaded term
- Breaking change for modest clarity gain not justified
- Documentation can bridge the gap for non-Erlang developers

**Priority**: Low
**Migration Complexity**: Easy (documentation only) vs Hard (rename everything)

---

## Category 3: Patterns and Abstractions

### Issue 3.1: Dependencies Embedding vs Explicit Parameters

**Current Pattern**:
```go
// In communicator worker:
type Dependencies interface {
    GetTransport() transport.Transport
    GetLogger() logger.Logger
}

type CommunicatorWorker struct {
    dependencies Dependencies
    // Embedded dependency bag
}

// Actions also take Dependencies
func NewAuthenticateAction(
    dependencies Dependencies,
    relayURL string,
    // ...
) *AuthenticateAction
```

**Why Non-Intuitive**:
- Dependencies bag hides what's actually needed
- Each action declares its own dependency subset
- Not clear which dependencies are actually used vs available
- Testing requires mocking entire Dependencies interface

**Root Cause**:
Dependency injection pattern to avoid global state and enable testing. Dependencies interface provides uniform API but hides actual requirements.

**Alternatives**:

**Alt 1: Explicit Parameters (No Dependency Bag)**
```go
type CommunicatorWorker struct {
    transport transport.Transport
    logger    logger.Logger
    // Explicit fields
}

// Actions take only what they need
func NewAuthenticateAction(
    transport transport.Transport,  // Explicit
    logger logger.Logger,
    relayURL string,
) *AuthenticateAction
```

**Alt 2: Separate Dependencies Per Component**
```go
type WorkerDependencies struct {
    Transport transport.Transport
    Logger    logger.Logger
}

type ActionDependencies struct {
    Transport transport.Transport
    Logger    logger.Logger
}

// Clear separation of concerns
```

**Alt 3: Keep Interface but Add Type-Specific Subset Interfaces**
```go
type AuthActionDeps interface {
    GetTransport() transport.Transport
    GetLogger() logger.Logger
    // Only what AuthAction needs
}

func NewAuthenticateAction(deps AuthActionDeps, ...) *AuthenticateAction

// Testing mocks only required subset
```

**Alt 4: Use Go 1.23 Iterator Pattern for Dependency Discovery**
```go
type Dependencies struct {
    components map[reflect.Type]interface{}
}

func (d *Dependencies) Get(target interface{}) bool {
    // Type-safe retrieval with generic-like interface
    val := d.components[reflect.TypeOf(target)]
    if val == nil {
        return false
    }
    reflect.ValueOf(target).Elem().Set(reflect.ValueOf(val))
    return true
}

// Usage:
var transport transport.Transport
if deps.Get(&transport) {
    // Use transport
}
```

**Trade-offs**:
| Approach | Explicitness | Test Complexity | Verbosity | Type Safety |
|----------|-------------|----------------|-----------|-------------|
| Current | Low | Medium | Low | Medium |
| Alt 1 (explicit) | High | Low | High | High |
| Alt 2 (split bags) | Medium | Medium | Medium | Medium |
| Alt 3 (subset interfaces) | Medium | Low | Medium | High |
| Alt 4 (discovery) | Low | High | Low | Low |

**Recommendation**: **Alt 3 (subset interfaces) for better testing**

**Rationale**:
- Alt 1 (explicit params) is most clear but creates very long parameter lists
- Alt 3 provides best balance: clear requirements, easy mocking, not too verbose
- Alt 2 doesn't add enough value over current pattern
- Alt 4 is overly complex for this use case

**Priority**: Low (current pattern works, just not optimal for testing)
**Migration Complexity**: Medium (requires creating subset interfaces, non-breaking)

---

### Issue 3.2: BaseWorker vs Manual Implementation

**Current Pattern**:
```go
// No BaseWorker exists in current codebase
// Each worker implements full Worker interface manually
```

**Why Non-Intuitive**:
- Every worker must implement 3 methods (CollectObservedState, DeriveDesiredState, GetInitialState)
- Simple workers (like Communicator MVP) duplicate boilerplate
- No default implementations for common patterns

**Root Cause**:
Go doesn't have inheritance or default interface methods, so base functionality must be composed or embedded.

**Alternatives**:

**Alt 1: Create BaseWorker with Embeddable Defaults**
```go
type BaseWorker struct {
    initialState State
}

func (b *BaseWorker) GetInitialState() State {
    return b.initialState
}

// Workers can embed and override
type MyWorker struct {
    BaseWorker  // Embedded base
}

// Override only what you need
func (w *MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Custom implementation
}
```

**Alt 2: Provide Helper Constructors**
```go
// For simple pass-through workers
func NewPassThroughWorker(initialState State) Worker {
    return &passThroughWorker{initialState: initialState}
}

type passThroughWorker struct {
    initialState State
}

func (w *passThroughWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    return &EmptyObservedState{CollectedAt: time.Now()}, nil
}

func (w *passThroughWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
    return types.DesiredState{State: "running"}, nil
}

func (w *passThroughWorker) GetInitialState() State {
    return w.initialState
}
```

**Alt 3: Keep Manual Implementation (Current)**
```go
// Explicit is better than implicit
// Each worker shows exactly what it does
// No hidden magic in base classes
```

**Trade-offs**:
| Approach | Boilerplate | Explicitness | Magic | Flexibility |
|----------|------------|-------------|-------|-------------|
| Current | High | High | None | High |
| Alt 1 (base) | Low | Low | Some | Medium |
| Alt 2 (helpers) | Medium | Medium | Little | Medium |

**Recommendation**: **Alt 2 (helper constructors) for common patterns**

**Rationale**:
- Go idiom: "A little copying is better than a little dependency"
- Alt 1 (BaseWorker) hides behavior in embedded struct, not idiomatic
- Alt 2 provides convenience without hiding behavior
- Keep manual implementation for complex workers
- Helpers are opt-in, not mandatory

**Priority**: Low (convenience feature)
**Migration Complexity**: Easy (additive helpers, backward compatible)

---

### Issue 3.3: Action Idempotency Testing Burden

**Current Pattern**:
```go
// worker.go lines 146-152
// Testing idempotency:
// Use the idempotency test helper in supervisor/action_helpers_test.go:
//
//	VerifyActionIdempotency(action, 3, func() {
//	    Expect(fileExists("test.txt")).To(BeTrue())
//	})
//
// REQUIREMENT (FSM v2): Every Action implementation MUST have an idempotency test.
```

**Why Non-Intuitive**:
- Idempotency is CRITICAL (Invariant I10) but relies on developer discipline
- No compile-time enforcement
- No runtime checks in production
- Test helper exists but nothing forces its use

**Root Cause**:
Idempotency is a runtime property that can't be enforced by Go's type system. Testing is the only verification mechanism.

**Alternatives**:

**Alt 1: Add Runtime Idempotency Checker in Development**
```go
type IdempotentAction struct {
    action Action
    checkMode bool  // Only in dev/test builds
}

func (a *IdempotentAction) Execute(ctx context.Context) error {
    if !a.checkMode {
        return a.action.Execute(ctx)
    }

    // Execute twice, verify same result
    result1 := a.action.Execute(ctx)
    result2 := a.action.Execute(ctx)

    if !equalResults(result1, result2) {
        panic("Action is not idempotent!")
    }
    return result1
}
```

**Alt 2: Generate Idempotency Tests Automatically**
```go
//go:generate idempotency-test-gen -type=MyAction

type MyAction struct {
    // Implementation
}

// Generator creates:
// func TestMyAction_Idempotency(t *testing.T) {
//     VerifyActionIdempotency(...)
// }
```

**Alt 3: Add Linter Check for Missing Tests**
```go
// Custom go vet check:
// For each type implementing Action interface,
// verify corresponding _test.go contains VerifyActionIdempotency call
```

**Alt 4: Provide Framework for Declarative Idempotency**
```go
type CheckThenActAction struct {
    CheckDone func(ctx context.Context) (bool, error)
    PerformWork func(ctx context.Context) error
}

func (a *CheckThenActAction) Execute(ctx context.Context) error {
    done, err := a.CheckDone(ctx)
    if err != nil {
        return err
    }
    if done {
        return nil  // Idempotent return
    }
    return a.PerformWork(ctx)
}

// Framework guarantees idempotency if CheckDone is correct
```

**Trade-offs**:
| Approach | Safety | Overhead | Dev UX | Coverage |
|----------|--------|----------|--------|----------|
| Current | Low | None | Medium | Partial |
| Alt 1 (runtime) | High | High | Poor | Full |
| Alt 2 (codegen) | Medium | Low | Good | Full |
| Alt 3 (linter) | Medium | None | Good | Full |
| Alt 4 (framework) | High | Low | Best | Partial |

**Recommendation**: **Alt 3 (linter) + Alt 4 (framework) for common patterns**

**Rationale**:
- Alt 3 enforces testing discipline without runtime overhead
- Alt 4 makes simple idempotent actions trivial to implement correctly
- Alt 1 (runtime check) too expensive for production
- Alt 2 (codegen) adds build complexity
- Combination provides both enforcement and ease of use

**Priority**: High (idempotency is critical invariant)
**Migration Complexity**:
- Alt 3: Easy (new linter rule, non-breaking)
- Alt 4: Easy (additive helper, opt-in)

---

### Issue 3.4: State Machine Validation Opportunities

**Current Pattern**:
```go
// No compile-time or runtime validation of state machine structure
// Possible issues only found via:
// - Manual code review
// - Runtime testing
// - Production bugs
```

**Why Non-Intuitive**:
- State machines can have unreachable states (no transitions point to them)
- State machines can have states with no exit (infinite loops)
- State machines can have missing shutdown transitions
- No visualization or validation tools

**Root Cause**:
State transitions defined in code (Next() methods) rather than declaratively, so structure is implicit.

**Alternatives**:

**Alt 1: Declarative State Machine Definition**
```go
type StateMachine struct {
    States []StateDefinition
    Transitions []TransitionDefinition
}

type StateDefinition struct {
    Name string
    Type StateType  // Active, Passive, etc.
    EntryConditions []Condition
    ExitConditions []Condition
}

type TransitionDefinition struct {
    From State
    To State
    Guard Condition
}

// Validation can check:
// - All states reachable
// - All states have shutdown path
// - No infinite loops
// - Active states have actions
```

**Alt 2: Add Validation Helper in Tests**
```go
func ValidateStateMachine(states []State, initialState State) error {
    graph := buildStateGraph(states)

    // Check reachability
    if unreachable := graph.FindUnreachable(initialState); len(unreachable) > 0 {
        return fmt.Errorf("unreachable states: %v", unreachable)
    }

    // Check shutdown paths
    for _, state := range states {
        if !graph.HasPathTo(state, shutdownState) {
            return fmt.Errorf("state %s has no path to shutdown", state)
        }
    }

    return nil
}
```

**Alt 3: Generate State Machine Diagrams**
```go
//go:generate statemachine-diagram -pkg=myworker -output=diagram.svg

// Generates visual diagram from Next() implementations
// Developer can visually verify transitions
```

**Alt 4: Runtime State Machine Recorder**
```go
type StateRecorder struct {
    transitions []Transition
}

// Records all transitions during test runs
// Validates coverage: did we exercise all states?
// Validates invariants: did we reach invalid states?
```

**Trade-offs**:
| Approach | Validation Strength | Implementation Cost | Runtime Cost | Usability |
|----------|-------------------|---------------------|-------------|-----------|
| Current | None | N/A | None | N/A |
| Alt 1 (declarative) | High | Very High | Low | Complex |
| Alt 2 (test helper) | Medium | Low | None | Simple |
| Alt 3 (diagrams) | Low | Medium | None | Good |
| Alt 4 (recorder) | Medium | Medium | Medium | Medium |

**Recommendation**: **Alt 2 (test helper) + Alt 3 (diagrams)**

**Rationale**:
- Alt 1 (declarative FSM) is FSM v3 material, too invasive for v2
- Alt 2 provides good enough validation with minimal cost
- Alt 3 helps human review of state machine structure
- Alt 4 useful but Alt 2 already catches most issues

**Priority**: Medium (improves correctness)
**Migration Complexity**:
- Alt 2: Easy (additive test utility)
- Alt 3: Medium (requires code generation tool)

---

## Category 4: Type System

### Issue 4.1: VariableBundle Three-Tier Namespace Complexity

**Current Pattern**:
```go
type VariableBundle struct {
    User     map[string]any  // Top-level in templates: {{ .IP }}
    Global   map[string]any  // Nested in templates: {{ .global.api_endpoint }}
    Internal map[string]any  // Nested in templates: {{ .internal.worker_id }}
}

// Flatten() makes User variables top-level
flattened := bundle.Flatten()
// Result: { "IP": "...", "global": {...}, "internal": {...} }
```

**Why Non-Intuitive**:
- Three different namespaces with different access patterns
- User variables become top-level (not `{{ .user.IP }}`)
- Global/Internal stay nested (`{{ .global.X }}`, `{{ .internal.Y }}`)
- Flattening rules not obvious from type definition
- No documentation of which namespace to use when

**Root Cause**:
Design accommodates three use cases:
1. User variables: Configuration-specific (per-worker)
2. Global variables: System-wide settings (shared across workers)
3. Internal variables: Runtime metadata (framework-provided)

Flattening User to top-level reduces verbosity in templates.

**Alternatives**:

**Alt 1: Single Flat Namespace with Prefixes**
```go
type Variables map[string]any

// Usage:
vars := Variables{
    "IP": "...",                    // User variable
    "__global_api_endpoint": "...", // Global (prefix convention)
    "__internal_worker_id": "...",  // Internal (prefix convention)
}

// Templates:
// {{ .IP }}
// {{ .__global_api_endpoint }}
// {{ .__internal_worker_id }}
```

**Alt 2: Explicit Namespaces with Documented Rules**
```go
type VariableBundle struct {
    // User variables: Accessible as {{ .variableName }} (top-level)
    // Use for: IP addresses, ports, endpoints, protocol-specific configs
    User map[string]any

    // Global variables: Accessible as {{ .global.variableName }}
    // Use for: Shared settings, API endpoints, common timeouts
    Global map[string]any

    // Internal variables: Accessible as {{ .internal.variableName }}
    // Use for: Worker IDs, runtime metadata (set by framework)
    Internal map[string]any
}

// Flatten() documentation improved
func (v VariableBundle) Flatten() map[string]any {
    // Returns map where:
    // - User variables are TOP-LEVEL: .IP, .PORT
    // - Global variables nested: .global.api_endpoint
    // - Internal variables nested: .internal.worker_id
}
```

**Alt 3: Remove Internal Namespace (Simplify to Two Tiers)**
```go
type VariableBundle struct {
    User map[string]any    // Top-level: {{ .IP }}
    Global map[string]any  // Nested: {{ .global.api_endpoint }}
}

// Framework-provided vars go in Global
```

**Alt 4: Make All Namespaces Top-Level**
```go
type VariableBundle struct {
    User     map[string]any
    Global   map[string]any
    Internal map[string]any
}

func (v VariableBundle) Flatten() map[string]any {
    // All variables top-level, collision errors if duplicates
    flattened := make(map[string]any)

    for k, v := range v.User {
        flattened[k] = v
    }
    for k, v := range v.Global {
        if _, exists := flattened[k]; exists {
            panic("Variable collision: " + k)
        }
        flattened[k] = v
    }
    // Same for Internal

    return flattened
}

// Templates:
// {{ .IP }}
// {{ .api_endpoint }}
// {{ .worker_id }}
```

**Trade-offs**:
| Approach | Simplicity | Namespace Isolation | Collision Risk | Template Verbosity |
|----------|-----------|-------------------|----------------|-------------------|
| Current | Medium | High | None | Low (User), Medium (Global/Internal) |
| Alt 1 (prefixes) | High | Low | Medium | Medium (prefixes) |
| Alt 2 (docs) | Medium | High | None | Low (User), Medium (Global/Internal) |
| Alt 3 (two-tier) | High | High | Low | Low (User), Medium (Global) |
| Alt 4 (all flat) | High | None | High | Low |

**Recommendation**: **Alt 2 (explicit documentation) short-term, Alt 3 (two-tier) long-term**

**Rationale**:
- Alt 2 improves current pattern with clear docs (no breaking changes)
- Alt 3 simplifies to two tiers: User (worker-specific) and Global (system-wide)
- Framework metadata can live in Global namespace
- Alt 1 (prefixes) loses type safety and namespace isolation
- Alt 4 (all flat) creates collision nightmares

**Priority**: Medium (affects template usability)
**Migration Complexity**:
- Alt 2: Easy (documentation only)
- Alt 3: Medium (requires migrating Internal vars to Global)

---

### Issue 4.2: types.UserSpec as Configuration Abstraction

**Current Pattern**:
```go
type UserSpec struct {
    Config    string         // YAML/JSON with {{ .VARIABLE }} syntax
    Variables VariableBundle // Three namespaces for substitution
}

// DeriveDesiredState receives interface{}, usually UserSpec
DeriveDesiredState(spec interface{}) (types.DesiredState, error)
```

**Why Non-Intuitive**:
- Config is opaque string (could be YAML, JSON, TOML, anything)
- No validation before passing to Worker
- Type assertion required (`spec.(types.UserSpec)`)
- Template variables in Config string, not pre-parsed

**Root Cause**:
Design allows Workers to define their own config formats. UserSpec is just a container, actual parsing happens in Worker.

**Alternatives**:

**Alt 1: Strongly-Typed Config per Worker**
```go
type BenthosWorkerConfig struct {
    Input  BenthosInput
    Output BenthosOutput
    Pipeline BenthosPipeline
}

type Worker[C any] interface {
    DeriveDesiredState(config C) (types.DesiredState, error)
}

// Usage:
type BenthosWorker struct{}

func (w *BenthosWorker) DeriveDesiredState(config BenthosWorkerConfig) (types.DesiredState, error) {
    // config is typed, no parsing/validation needed
}
```

**Alt 2: Config Validation Interface**
```go
type ValidatableConfig interface {
    Validate() error
}

type UserSpec struct {
    Config    ValidatableConfig  // Must implement Validate()
    Variables VariableBundle
}

// Workers define their config type
type MyWorkerConfig struct {
    IP string
    Port int
}

func (c *MyWorkerConfig) Validate() error {
    if c.IP == "" {
        return fmt.Errorf("IP required")
    }
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("invalid port")
    }
    return nil
}
```

**Alt 3: Keep UserSpec, Add Schema Validation**
```go
type UserSpec struct {
    Config    string
    Variables VariableBundle
    Schema    *Schema  // JSON Schema, YAML schema, etc.
}

// Validation happens before passing to Worker
func ValidateUserSpec(spec UserSpec) error {
    return spec.Schema.Validate(spec.Config)
}
```

**Alt 4: Two-Phase Config (Raw + Parsed)**
```go
type UserSpec struct {
    RawConfig    string
    ParsedConfig interface{}  // Result of parsing RawConfig
    Variables    VariableBundle
}

// Supervisor parses config before calling DeriveDesiredState
// Worker receives pre-parsed, validated config
```

**Trade-offs**:
| Approach | Type Safety | Flexibility | Validation | Complexity |
|----------|-----------|------------|-----------|-----------|
| Current | Low | High | None | Low |
| Alt 1 (generics) | High | Low | Compile-time | High |
| Alt 2 (interface) | Medium | High | Runtime | Medium |
| Alt 3 (schema) | Low | High | Runtime | Medium |
| Alt 4 (two-phase) | Medium | High | Runtime | Medium |

**Recommendation**: **Alt 2 (ValidatableConfig) for gradual improvement**

**Rationale**:
- Alt 1 (generics) is ideal long-term but breaks everything (FSM v3)
- Alt 2 provides validation without breaking existing code
- Alt 3 (schema) adds dependency and complexity
- Alt 4 (two-phase) splits responsibilities awkwardly

**Priority**: Low (current pattern works, just not type-safe)
**Migration Complexity**:
- Alt 2: Medium (add interface, update workers gradually)
- Alt 1: Hard (FSM v3, complete rewrite)

---

## Category 5: Hierarchies and Composition

### Issue 5.1: ChildrenSpecs Reconciliation Complexity

**Current Pattern**:
```go
type DesiredState struct {
    State         string
    ChildrenSpecs []ChildSpec  // Declarative child workers
}

// Supervisor reconciles:
// 1. Compare desired children with actual children
// 2. Create missing children
// 3. Remove extra children
// 4. Update mismatched children
```

**Why Non-Intuitive**:
- Kubernetes-style declarative management is powerful but not obvious
- Reconciliation logic hidden in Supervisor
- No documentation of reconciliation algorithm
- Unclear what happens when ChildSpec changes (recreate? update? merge?)

**Root Cause**:
Design follows Kubernetes controller pattern: parent declares desired children, supervisor reconciles actual state. This is correct pattern for hierarchical systems, but unfamiliar to developers without K8s experience.

**Alternatives**:

**Alt 1: Explicit Lifecycle Methods on Parent**
```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(spec interface{}) (types.DesiredState, error)
    GetInitialState() State

    // Explicit child management
    CreateChild(spec ChildSpec) (Worker, error)
    UpdateChild(child Worker, spec ChildSpec) error
    DeleteChild(child Worker) error
}
```

**Alt 2: Keep Declarative, Document Reconciliation Algorithm**
```go
// In DesiredState documentation:

// ChildrenSpecs defines desired child workers using declarative reconciliation:
//
// Reconciliation Algorithm:
// 1. Match children by Name (must be unique within parent)
// 2. For each desired child:
//    - If no actual child exists: Create new worker
//    - If actual child exists with different WorkerType: Delete old, create new
//    - If actual child exists with same type but different UserSpec: Call child's DeriveDesiredState
// 3. For each actual child not in desired list: Delete worker
//
// Reconciliation is idempotent and eventual consistent.
```

**Alt 3: Provide Explicit Reconciliation Hooks**
```go
type Worker interface {
    // ... existing methods ...

    // Optional: Called before child reconciliation
    OnBeforeReconcileChildren(desired []ChildSpec, actual []Worker) error

    // Optional: Called after child reconciliation
    OnAfterReconcileChildren(created []Worker, updated []Worker, deleted []Worker) error
}
```

**Alt 4: Simplify to Add/Remove Only (No Update)**
```go
// ChildrenSpecs reconciliation:
// - Create children not in actual list
// - Delete children not in desired list
// - NO in-place updates (delete + recreate instead)
//
// Simpler algorithm, easier to understand
// Trade-off: More churn during reconfigurations
```

**Trade-offs**:
| Approach | Explicitness | K8s Similarity | Complexity | Flexibility |
|----------|-------------|---------------|-----------|-------------|
| Current | Low | High | Medium | High |
| Alt 1 (explicit) | High | Low | High | Medium |
| Alt 2 (docs) | Medium | High | Medium | High |
| Alt 3 (hooks) | High | High | High | Highest |
| Alt 4 (simplified) | Medium | Medium | Low | Medium |

**Recommendation**: **Alt 2 (document algorithm) + Alt 3 (optional hooks)**

**Rationale**:
- Alt 2 makes implicit behavior explicit without breaking changes
- Alt 3 provides escape hatch for complex scenarios
- Alt 1 (explicit lifecycle) loses declarative benefits
- Alt 4 (simplified) too restrictive for real-world use cases

**Priority**: Medium (affects understandability)
**Migration Complexity**:
- Alt 2: Easy (documentation only)
- Alt 3: Medium (add optional interface methods)

---

### Issue 5.2: StateMapping for Parent-Child Communication

**Current Pattern**:
```go
// No StateMapping exists in current codebase
// Parent-child communication not documented
```

**Why Non-Intuitive**:
- How do parents pass state to children?
- How do children report state back to parents?
- Is there a standard mechanism or each worker defines its own?

**Root Cause**:
Parent-child state communication not yet implemented in FSM v2. Design question for future.

**Alternatives**:

**Alt 1: StateMapping in ChildSpec**
```go
type ChildSpec struct {
    Name       string
    WorkerType string
    UserSpec   UserSpec

    // NEW: Map parent state to child variables
    StateMapping map[string]string  // "parent.field" → "child.variable"
}

// Example:
StateMapping: {
    "parent.authToken": "child.token",
    "parent.endpoint": "child.server_url",
}

// Supervisor injects parent observed state into child variables
```

**Alt 2: Shared State via Context**
```go
type ChildContext struct {
    ParentObservedState ObservedState
    ParentDesiredState DesiredState
    ParentIdentity Identity
}

// Children can access parent state
func (w *ChildWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    childCtx := GetChildContext(ctx)
    parentAuth := childCtx.ParentObservedState.(*ParentObserved).AuthToken
    // Use parent's auth token
}
```

**Alt 3: Explicit Parent Reference**
```go
type Worker interface {
    // ... existing methods ...

    // Optional: Set by supervisor when worker is child
    SetParent(parent Worker)
    GetParent() Worker
}

// Children query parent directly
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
    if parent := w.GetParent(); parent != nil {
        parentState := parent.GetCurrentObservedState()
        // Use parent state
    }
}
```

**Alt 4: Event-Based Communication**
```go
type ParentWorker interface {
    Worker

    // Parent publishes events
    PublishEvent(event StateEvent)
}

type ChildWorker interface {
    Worker

    // Child subscribes to parent events
    OnParentEvent(event StateEvent)
}

// Supervisor routes events from parent to children
```

**Trade-offs**:
| Approach | Explicitness | Coupling | Complexity | Flexibility |
|----------|-------------|----------|-----------|-------------|
| None (current) | N/A | None | N/A | N/A |
| Alt 1 (mapping) | High | Low | Low | Medium |
| Alt 2 (context) | Medium | Medium | Medium | Medium |
| Alt 3 (reference) | High | High | Medium | High |
| Alt 4 (events) | Medium | Low | High | Highest |

**Recommendation**: **Alt 1 (StateMapping) for simple cases + Alt 3 (parent reference) for complex**

**Rationale**:
- Alt 1 covers 80% of use cases with minimal complexity
- Alt 3 provides escape hatch for complex communication needs
- Alt 2 (context) hides communication, makes testing harder
- Alt 4 (events) is over-engineered for current needs

**Priority**: Low (feature not yet needed)
**Migration Complexity**: Easy (new feature, backward compatible)

---

### Issue 5.3: Multi-Level Hierarchy Limits

**Current Pattern**:
```go
// Can parent workers have children that have children?
// Are there depth limits?
// How does DeriveDesiredState work at each level?
```

**Why Non-Intuitive**:
- Documentation doesn't specify maximum depth
- No discussion of resource limits for deep hierarchies
- Unclear if all levels participate in reconciliation
- No guidance on when to use multi-level vs flat

**Root Cause**:
Design supports arbitrary depth but doesn't document it or provide guidance.

**Alternatives**:

**Alt 1: Document Current Behavior**
```markdown
## Hierarchical Workers

FSM v2 supports arbitrary-depth worker hierarchies:

- **Depth Limit**: None (supervisor recursively manages all levels)
- **Reconciliation**: Each level reconciles its immediate children only
- **Resource Impact**: O(n) workers, O(log n) depth typical
- **Use Cases**:
  - Level 1: Region workers
  - Level 2: Factory workers
  - Level 3: Line workers
  - Level 4: Device workers

**Guidelines**:
- Prefer flat hierarchies (2-3 levels) for simplicity
- Use deep hierarchies (4+ levels) when isolation boundaries align
- Consider resource constraints (100s of workers? OK. 1000s? Review design)
```

**Alt 2: Add Depth Limit Configuration**
```go
type SupervisorConfig struct {
    MaxWorkerDepth int  // Default: 10
}

// Supervisor rejects ChildSpecs beyond configured depth
```

**Alt 3: Provide Hierarchy Visualization Tools**
```bash
# CLI tool to visualize worker hierarchy
$ umh-fsm-tree

Root (Agent)
├── Communicator
├── ProtocolConverter-1
│   ├── S7-Source
│   └── MQTT-Sink
└── ProtocolConverter-2
    ├── OPC-UA-Source
    └── Kafka-Sink

Total: 7 workers, Max Depth: 3
```

**Alt 4: Progressive Disclosure Pattern**
```markdown
## Worker Hierarchies

**Quick Start**: Most workers don't have children. Skip to CollectObservedState.

**Intermediate**: Create 1-2 child workers using ChildrenSpecs. See template_worker.go.

**Advanced**: Multi-level hierarchies for complex orchestration. See hierarchy_guide.md.
```

**Trade-offs**:
| Approach | Clarity | Safety | Tooling | Effort |
|----------|---------|--------|---------|--------|
| Current | Low | Medium | None | N/A |
| Alt 1 (docs) | High | Medium | None | Low |
| Alt 2 (limits) | Medium | High | None | Low |
| Alt 3 (viz) | High | Medium | Required | High |
| Alt 4 (progressive) | High | Medium | None | Low |

**Recommendation**: **Alt 1 (docs) + Alt 4 (progressive disclosure) + Alt 3 (viz tool) long-term**

**Rationale**:
- Alt 1 and Alt 4 provide immediate clarity with no code changes
- Alt 2 (limits) not needed unless problems emerge
- Alt 3 (viz tool) extremely valuable for debugging but requires development

**Priority**: Low (works correctly, just not documented)
**Migration Complexity**:
- Alt 1, Alt 4: Easy (documentation)
- Alt 3: Medium (new tool development)

---

## Category 6: Testing and Validation

### Issue 6.1: Idempotency Testing Helpers

**Current Pattern**:
```go
// supervisor/execution/action_idempotency_test.go provides helper
func VerifyActionIdempotency(action Action, count int, verify func())

// But usage is manual and optional
```

**Why Non-Intuitive**:
- Helper exists but no enforcement
- New developers might not know it exists
- No template for creating idempotency tests
- No CI check for missing tests

**Root Cause**:
Testing helpers are discoverable via documentation but not enforced by tooling.

**Alternatives**:

**Alt 1: Make Test Helper More Discoverable**
```go
// Add to Worker package README:

## Testing Your Worker

### Required Tests

1. **Idempotency Tests** (mandatory for all actions)
   ```go
   import "github.com/.../fsmv2/testing"

   func TestMyAction_Idempotency(t *testing.T) {
       action := &MyAction{...}
       testing.VerifyActionIdempotency(t, action, 3)
   }
   ```

2. **State Transition Tests**
3. **Context Cancellation Tests**
```

**Alt 2: Generate Test Scaffolding**
```bash
# Code generator for new actions
$ go generate ./...

// Creates:
// action/my_action.go
// action/my_action_test.go (with idempotency test scaffold)
```

**Alt 3: Add CI Check for Test Coverage**
```bash
# In CI pipeline:
$ go test -cover -coverprofile=coverage.out
$ go tool cover -func=coverage.out | grep "Execute.*0.0%" && exit 1

# Fails if any Action.Execute() method has 0% test coverage
```

**Alt 4: Provide Test Examples in Documentation**
```markdown
## Example: Idempotent File Creation

```go
type CreateFileAction struct {
    Path string
    Content string
}

func (a *CreateFileAction) Execute(ctx context.Context) error {
    // Check-then-act pattern
    if _, err := os.Stat(a.Path); err == nil {
        return nil  // Already exists
    }
    return os.WriteFile(a.Path, []byte(a.Content), 0644)
}

// Test:
func TestCreateFileAction_Idempotency(t *testing.T) {
    action := &CreateFileAction{
        Path: "/tmp/test.txt",
        Content: "hello",
    }

    testing.VerifyActionIdempotency(t, action, 5)

    // Verify file exists exactly once
    content, _ := os.ReadFile(action.Path)
    if string(content) != "hello" {
        t.Fatal("File content incorrect")
    }
}
```
```

**Trade-offs**:
| Approach | Discoverability | Enforcement | Effort |
|----------|----------------|------------|--------|
| Current | Low | None | N/A |
| Alt 1 (docs) | Medium | None | Low |
| Alt 2 (codegen) | High | Partial | High |
| Alt 3 (CI) | Low | High | Low |
| Alt 4 (examples) | High | None | Low |

**Recommendation**: **Alt 1 (docs) + Alt 3 (CI check) + Alt 4 (examples)**

**Rationale**:
- Alt 1 and Alt 4 improve discoverability with low effort
- Alt 3 provides hard enforcement without being invasive
- Alt 2 (codegen) is overkill for current scale

**Priority**: Medium (improves test quality)
**Migration Complexity**: Easy (documentation + CI config changes)

---

### Issue 6.2: State Machine Validation Gaps

**(Covered in Issue 3.4 - State Machine Validation Opportunities)**

---

### Issue 6.3: Integration Test Patterns

**Current Pattern**:
```go
// No standard pattern for testing full Worker lifecycle
// Each worker writes its own integration tests
```

**Why Non-Intuitive**:
- No template for integration testing
- Unclear how to test Worker + Supervisor interaction
- Missing examples of async collection testing
- No guidance on mocking dependencies

**Root Cause**:
Integration testing patterns not yet established for FSM v2.

**Alternatives**:

**Alt 1: Provide Integration Test Framework**
```go
package testing

type WorkerTestHarness struct {
    Worker Worker
    Supervisor *Supervisor
    TickInterval time.Duration
}

func NewWorkerTestHarness(worker Worker) *WorkerTestHarness {
    // Sets up worker + supervisor in test mode
}

func (h *WorkerTestHarness) Start() {
    // Starts supervisor loop
}

func (h *WorkerTestHarness) Stop() {
    // Gracefully stops supervisor
}

func (h *WorkerTestHarness) WaitForState(expected State, timeout time.Duration) error {
    // Blocks until state reached or timeout
}

func (h *WorkerTestHarness) InjectDesiredState(desired DesiredState) {
    // Simulates config update
}

// Usage:
func TestWorker_FullLifecycle(t *testing.T) {
    harness := testing.NewWorkerTestHarness(&MyWorker{})
    harness.Start()
    defer harness.Stop()

    // Wait for initialization
    if err := harness.WaitForState(&RunningState{}, 5*time.Second); err != nil {
        t.Fatal(err)
    }

    // Trigger reconfiguration
    harness.InjectDesiredState(types.DesiredState{...})

    // Verify transition
    if err := harness.WaitForState(&ReconfiguredState{}, 5*time.Second); err != nil {
        t.Fatal(err)
    }
}
```

**Alt 2: Document Testing Patterns**
```markdown
## Integration Testing Guide

### Pattern 1: Full Lifecycle Test

Test worker from initialization through shutdown:

1. Create worker with test dependencies
2. Create supervisor
3. Start supervisor with worker
4. Wait for expected state transitions
5. Inject configuration changes
6. Verify reconfiguration
7. Trigger shutdown
8. Verify graceful termination
```

**Alt 3: Provide Mock Dependencies**
```go
package testing

type MockTransport struct {
    PullResponses [][]Message
    PushCalls [][]Message
}

func (m *MockTransport) Pull(ctx context.Context, token string) ([]Message, error) {
    // Returns pre-programmed responses
}

func (m *MockTransport) Push(ctx context.Context, token string, messages []Message) error {
    m.PushCalls = append(m.PushCalls, messages)
    return nil
}

// Usage:
func TestCommunicator_Sync(t *testing.T) {
    transport := &testing.MockTransport{
        PullResponses: [][]Message{
            {Message{Content: "msg1"}},
            {Message{Content: "msg2"}},
        },
    }

    worker := &CommunicatorWorker{
        dependencies: &testDeps{transport: transport},
    }

    // Test sync behavior
}
```

**Trade-offs**:
| Approach | Usability | Completeness | Maintenance |
|----------|-----------|-------------|-------------|
| Current | Low | N/A | N/A |
| Alt 1 (framework) | High | High | High |
| Alt 2 (docs) | Medium | Medium | Low |
| Alt 3 (mocks) | High | Medium | Medium |

**Recommendation**: **Alt 2 (docs) + Alt 3 (mocks) now, Alt 1 (framework) when patterns stabilize**

**Rationale**:
- Alt 2 and Alt 3 provide immediate value with low investment
- Alt 1 (framework) is valuable but requires patterns to stabilize first
- Mocks (Alt 3) are essential for testing without external dependencies

**Priority**: Medium (improves test coverage)
**Migration Complexity**: Easy (additive helpers and documentation)

---

## Prioritized Roadmap

### Phase 1: Quick Wins (No Breaking Changes)

**Timeframe**: 1-2 weeks

| Issue | Solution | Benefit | Effort |
|-------|---------|---------|--------|
| 1.3 (Next() validation) | Test helper for invalid state+action | Catches correctness bugs | 1 day |
| 1.4 (Method naming) | Add intent comments to Worker interface | Clarifies async vs sync | 1 hour |
| 3.3 (Idempotency) | Add linter check + framework helper | Enforces critical invariant | 3 days |
| 4.1 (VariableBundle) | Document three-tier namespace rules | Reduces template errors | 1 day |
| 5.1 (ChildrenSpecs) | Document reconciliation algorithm | Makes implicit behavior explicit | 2 days |
| 6.1 (Test helpers) | Improve documentation + CI check | Better test coverage | 2 days |

**Total Effort**: ~10 days
**Impact**: High (catches bugs, improves documentation)

---

### Phase 2: Naming and Documentation (Minor Breaking Changes)

**Timeframe**: 1-2 months (alongside regular development)

| Issue | Solution | Benefit | Migration |
|-------|---------|---------|-----------|
| 1.1 (Snapshot immutability) | Add `immutable` struct tags + linter | Compile-time enforcement | Add tags to Snapshot |
| 1.2 (Type safety) | Add helper methods for type assertions | Better error messages | Add to Snapshot type |
| 2.1 (State naming) | Document current pattern, propose gerund convention for v3 | Consistency | None now, plan for v3 |
| 2.2 (ObservedState) | Propose DeployedState rename for v3 | Clarity | None now, plan for v3 |
| 3.1 (Dependencies) | Create subset interfaces for testing | Easier mocking | Add interfaces, non-breaking |
| 3.2 (BaseWorker) | Provide helper constructors | Reduce boilerplate | Opt-in helpers |
| 5.3 (Hierarchy limits) | Document behavior + progressive disclosure | Prevents confusion | Documentation only |

**Total Effort**: ~15-20 days spread over 2 months
**Impact**: Medium (improves developer experience)

---

### Phase 3: Structural Improvements (Major Refactoring)

**Timeframe**: FSM v3 (6-12 months out)

| Issue | Solution | Benefit | Migration |
|-------|---------|---------|-----------|
| 1.2 (Type safety) | Generics for Snapshot[O, D] | Compile-time type checking | Rewrite all workers |
| 1.3 (Next() signature) | Split Active/Passive at type level | Encode convention in types | Rewrite all states |
| 2.1 (State naming) | Gerund verbs for active states | Consistency across codebase | Rename all states |
| 2.2 (ObservedState) | Rename to DeployedState | Clear terminology | Rename interface + methods |
| 4.1 (VariableBundle) | Simplify to two-tier (User/Global) | Reduce complexity | Migrate Internal vars |
| 4.2 (UserSpec) | Generics for Worker[C any] | Type-safe configs | Rewrite Worker interface |

**Total Effort**: ~40-60 days (major version bump)
**Impact**: High (architectural improvements)

---

### Phase 4: Future Considerations

**Ideas for FSM v4 or Beyond**:

1. **Declarative State Machines**: Define transitions declaratively rather than in code
2. **State Machine Visualization**: Auto-generate diagrams from state definitions
3. **Hot Reloading**: Update worker behavior without restart
4. **Distributed FSM**: Workers communicate across process boundaries
5. **Time-Travel Debugging**: Record and replay state transitions

---

## Comparison with Go Idioms

### How FSM v2 Aligns with Go Best Practices

**Strengths**:

1. **Pass-by-value for immutability** (Issue 1.1)
   - ✅ Idiomatic Go: Leverage language design
   - Comparison: `time.Time` passed by value for immutability

2. **Explicit error returns** (all methods)
   - ✅ Go convention: Errors as values
   - Comparison: Standard library `func Read() (n int, err error)`

3. **Context for cancellation** (CollectObservedState, Action.Execute)
   - ✅ Go idiom: ctx first parameter
   - Comparison: `http.Request.WithContext(ctx)`

4. **Interface-based design** (Worker, State, Action, ObservedState)
   - ✅ Small, focused interfaces
   - Comparison: `io.Reader`, `io.Writer` (minimal contracts)

**Divergences**:

1. **interface{} instead of generics** (Issue 1.2)
   - ⚠️ Pre-generics pattern (Go 1.17 era)
   - Modern Go: Would use `Snapshot[O, D any]`
   - Reason: FSM v2 pre-dates Go 1.18 generics, backward compatibility

2. **Tuple returns vs sum types** (Issue 1.3)
   - ⚠️ Encodes multiple outcomes in tuple
   - Go doesn't have sum types, so tuple is pragmatic
   - Comparison: `func Parse() (value int, err error)` similar pattern

3. **interface{} for spec parameter** (Issue 4.2)
   - ⚠️ Loses type safety
   - Common Go pattern for plugin/extension systems
   - Comparison: `encoding/json` uses `interface{}` for flexibility

4. **Embedding vs composition** (Issue 3.1, 3.2)
   - ✅ Dependencies interface is composition (good)
   - ❌ BaseWorker embedding would be non-idiomatic (avoided correctly)
   - Go proverb: "A little copying is better than a little dependency"

### Standard Library Patterns Comparison

**State Machine Patterns in Go Stdlib**:

1. **http.Handler**: Single method interface
   ```go
   type Handler interface {
       ServeHTTP(ResponseWriter, *Request)
   }
   ```
   - FSM v2 equivalent: `State.Next(Snapshot)` single method
   - ✅ Similar minimal interface approach

2. **context.Context**: Value passing for cancellation
   ```go
   func (c *Context) Done() <-chan struct{}
   ```
   - FSM v2 uses context in CollectObservedState and Action.Execute
   - ✅ Follows stdlib pattern exactly

3. **io.Reader/Writer**: Small, composable interfaces
   ```go
   type Reader interface {
       Read(p []byte) (n int, err error)
   }
   ```
   - FSM v2 `Worker` interface has 3 methods (reasonable)
   - ✅ Small surface area like stdlib

4. **database/sql.Driver**: Plugin architecture with interface{}
   ```go
   type Driver interface {
       Open(name string) (Conn, error)
   }
   ```
   - FSM v2 spec parameter uses interface{} for flexibility
   - ✅ Common pattern for extensible systems

### Error Handling Patterns

**FSM v2 Error Handling**:

```go
// Typical FSM v2 pattern
func (w *Worker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    status, err := checkStatus(ctx)
    if err != nil {
        return nil, fmt.Errorf("collect observed state: %w", err)  // ✅ Wraps error
    }
    return status, nil
}
```

**Comparison to Go Stdlib**:

```go
// database/sql pattern (similar)
func (db *DB) Query(query string, args ...any) (*Rows, error) {
    // ...
    if err != nil {
        return nil, fmt.Errorf("query: %w", err)  // Same pattern
    }
}
```

- ✅ Error wrapping with `fmt.Errorf` and `%w`
- ✅ Contextual information preserved
- ✅ Errors are values (no exceptions)

### Testing Patterns

**FSM v2 Testing**:

```go
func TestState_Transition(t *testing.T) {
    state := &MyState{}
    snapshot := Snapshot{...}

    nextState, signal, action := state.Next(snapshot)

    if nextState == nil {
        t.Fatal("expected non-nil state")
    }
}
```

**Go Stdlib Comparison**:

```go
// testing package pattern
func TestReader(t *testing.T) {
    r := strings.NewReader("hello")
    buf := make([]byte, 5)

    n, err := r.Read(buf)
    if err != nil {
        t.Fatal(err)
    }
    if n != 5 {
        t.Errorf("want 5, got %d", n)
    }
}
```

- ✅ Table-driven tests (FSM v2 uses Ginkgo, but same principle)
- ✅ Explicit assertions
- ⚠️ FSM v2 uses Ginkgo/Gomega (not stdlib testing package)

**Recommendation**: FSM v2 aligns well with Go idioms. Main divergence is pre-generics patterns (interface{} instead of type parameters). FSM v3 could adopt generics for better type safety while maintaining Go idiomatic design.

---

## References

### Code Line Numbers (worker.go)

- Lines 38-49: Signal type definitions
- Lines 51-57: Identity struct
- Lines 59-71: ObservedState interface
- Lines 73-80: DesiredState interface
- Lines 82-114: Snapshot struct with immutability documentation
- Lines 116-167: Action interface with idempotency requirements
- Lines 169-250: State interface with naming conventions
- Lines 252-318: Worker interface with three core methods

### Related Analysis Documents

- `FSM_V2_ANALYSIS_SUMMARY.md`: Executive summary of 13 capabilities
- `FSM_V2_WORKER_CAPABILITIES.md`: Comprehensive capability documentation
- `DERIVE_DESIRED_STATE_ANALYSIS.md`: Templating and composition patterns
- `README_FSM_V2_ANALYSIS.md`: Navigation guide

### Example Implementations

- `workers/communicator/worker.go`: Simple pass-through pattern
- `examples/template_worker.go`: Complex templating with children
- `supervisor/execution/action_idempotency_test.go`: Test helpers

---

## Conclusion

FSM v2 architecture demonstrates solid design choices with thoughtful trade-offs between type safety, flexibility, and simplicity. Most "non-intuitive" patterns stem from:

1. **Go language constraints** (no sum types, pre-generics era)
2. **Deliberate design choices** (pass-by-value, declarative children)
3. **Implicit knowledge requirements** (Kubernetes patterns, Erlang supervision)

**Key Recommendations**:

1. **Short-term** (1-3 months): Improve documentation, add linting rules, create test helpers
2. **Medium-term** (3-6 months): Minor API additions (helper methods, subset interfaces)
3. **Long-term** (FSM v3): Adopt generics, encode conventions in type system, simplify variable namespaces

**No architectural redesign needed.** Current patterns are correct, just need better documentation, tooling, and progressive adoption of modern Go features (generics).

**Impact**: These improvements will make FSM v2 more accessible to new developers without sacrificing the solid architectural foundation that exists today.
