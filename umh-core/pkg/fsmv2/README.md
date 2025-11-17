# RFC FSM Library

<Summary>

## The Problem

The current implementation of FSMs in umh-core using looplab/fsm requires a complex mental model. Go developers are used to having everything explicit, but the FSM layer adds an abstraction. It is therefore not possible to follow the code from top to bottom. Instead one needs to think about multiupkle executions and the states it moves between. this makes it difficult to follow.

Also it has a lot of boilerplate code and the error handling is very chaotic (some in the supervisor, some in the boilerplate part of the FSM, some in the actual business logic)

Furthermore, there are some limitations:
1. The current state is only kept in memory, which resets on a restart. This makes it impossible to do proper High Availability
2. Some features requires the concept of an “Archive Storage” - versioning, audit trails, “history of states”

## The Solution

A simplified interface for creating and managing an FSM integratabtle with the “Control Sync Engine” (see also separate RFC).

There is the concept of a worker, and of a supervisor (similar to what currently exist as FSM and manager).

A worker has an identity (unique ID, etc.), a desired state (that is derived from the user configuration), and an observed state (which is gathered from monitoring).

```
        IDENTITY
       (What it is)
           /\
          /  \
         /    \
        /      \
    DESIRED  OBSERVED
  (Want)      (Have)
       \      /
        \    /
      RECONCILIATION
      (Make have = want)
```

## Mental Models

To work effectively with FSMv2, developers need to understand these core concepts that differ from traditional OOP patterns or string-based FSMs:

### 1. States Are Structs, Not Strings

**FSMv2 uses typed structs for states, not string constants or enums.**

```go
// FSMv2 approach (type-safe) ✅
type RunningState struct{}
type StoppedState struct{}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    return StoppedState{}, SignalNone, nil  // Compiler enforces valid states
}

// String-based FSM (error-prone) ❌
state := "runing"  // Typo not caught by compiler!
```

**Why this matters:**
- **Compile-time safety**: Can't create invalid states like `"runing"` (typo)
- **Explicit transitions**: All state transitions visible in code (grep for `return.*State{}`)
- **State-specific logic**: Each state encapsulates its own behavior in `Next()` method

**Contrast with string FSMs**: Most FSM libraries use `fsm.SetState("running")`. FSMv2 returns `RunningState{}` - the compiler prevents typos and invalid states.

### 2. Snapshots Are Immutable by Value

**Snapshots are passed by value to `State.Next()`, making them immutable through Go's value semantics.**

```go
func (s MyState) Next(snapshot Snapshot) (State, Signal, Action) {
    // snapshot is a COPY - mutations only affect local copy
    snapshot.Identity.Name = "modified"  // Safe, doesn't affect supervisor

    // This is why FSMv2 doesn't need getters or defensive copying
    return s, SignalNone, nil
}
```

**Why this matters:**
- **No getters needed**: Direct field access is safe (`snapshot.Identity.ID`)
- **No defensive copying**: Language enforces immutability automatically
- **Simpler code**: No `GetIdentity()` or `snapshot.Clone()` methods

**Contrast with OOP**: Object-oriented patterns expect getters (`snapshot.GetIdentity().GetID()`) to prevent mutation. Go's pass-by-value eliminates this need.

### 3. One Control Loop, No Races

**The reconciliation loop is the ONLY place that modifies FSM state - single-threaded, deterministic, no locks needed.**

```
┌─────────────────────────────────────┐
│         Supervisor Tick             │
│     (Single-threaded loop)          │
│                                     │
│  1. CollectObservedState()          │
│  2. DeriveDesiredState()            │
│  3. state.Next(snapshot)            │
│       ↓                             │
│     (nextState, signal, action)     │
│  4. Execute action (retry/backoff)  │
│  5. Transition to nextState         │
│  6. Process signal                  │
│  7. Reconcile children              │
└─────────────────────────────────────┘
```

**Why this matters:**
- **Deterministic behavior**: Same inputs always produce same state transitions
- **No race conditions**: Only one goroutine modifies state
- **Easy debugging**: Single execution path, no concurrent mutations
- **No locks required**: Workers don't need to implement synchronization

**Contrast with concurrent designs**: Traditional FSMs often have multiple threads calling `SetState()` concurrently, requiring locks and complex synchronization.

### 4. Actions Are Idempotent Operations

**Actions MUST be idempotent - calling them multiple times has the same effect as calling once.**

```go
type StartProcessAction struct {
    ProcessPath string
}

func (a *StartProcessAction) Execute(ctx context.Context) error {
    // ✅ IDEMPOTENT: Check if already done
    if processIsRunning(a.ProcessPath) {
        return nil  // Already started, safe to call again
    }
    return startProcess(ctx, a.ProcessPath)
}

// ❌ NON-IDEMPOTENT: Calling twice starts process twice
func (a *StartProcessAction) Execute(ctx context.Context) error {
    return startProcess(ctx, a.ProcessPath)  // No check!
}
```

**Why this matters:**
- **Supervisor retries**: Failed actions are retried with exponential backoff
- **Network issues**: Partial execution due to timeouts or connection loss
- **Tick repeats**: State machine may tick multiple times in same state

**Implementation pattern**:
1. Check if work already done
2. If yes, return `nil` (not an error)
3. If no, perform work
4. Return error only if work fails

### 5. Declarative Child Management

**Parents declare desired children via `ChildSpec` in `DeriveDesiredState()`. Supervisor handles creation, updates, and cleanup automatically (Kubernetes-style reconciliation).**

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (DesiredState, error) {
    return DesiredState{
        State: "running",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "mqtt-connection",
                WorkerType: "mqtt_client",
                UserSpec:   UserSpec{Config: "url: tcp://localhost:1883"},
                StateMapping: map[string]string{
                    "active":  "connected",  // Parent state → child state
                    "closing": "stopped",
                },
            },
        },
    }, nil
}
```

**Why this matters:**
- **No imperative creation**: Don't call `CreateChild()` or `DeleteChild()`
- **Automatic reconciliation**: Supervisor diffs desired vs actual children
- **Lifecycle management**: Supervisor handles creation, updates, graceful shutdown
- **StateMapping coordinates FSMs**: Parent state triggers child state transitions (NOT data passing)

**Contrast with imperative patterns**: Traditional code explicitly creates/deletes children. FSMv2 declares what children SHOULD exist, supervisor makes it so.

**Note**: For passing configuration data to children, use `VariableBundle` (see Mental Model #6).

### 6. Three Variable Tiers with Template Flattening

**FSMv2 provides three variable namespaces for configuration, with special flattening for User variables in templates.**

```go
bundle := VariableBundle{
    User: map[string]any{
        "IP":   "192.168.1.100",
        "PORT": 502,
    },
    Global: map[string]any{
        "cluster_id": "prod",
        "region":     "us-west",
    },
    Internal: map[string]any{
        "timestamp": time.Now(),  // Runtime-only, not serialized
    },
}
```

**Template access (IMPORTANT - User variables are flattened):**

```yaml
# User variables: Top-level access (flattened)
{{ .IP }}    # ✅ CORRECT (192.168.1.100)
{{ .PORT }}  # ✅ CORRECT (502)

# Global variables: Nested access
{{ .global.cluster_id }}  # ✅ CORRECT (prod)
{{ .global.region }}      # ✅ CORRECT (us-west)

# Internal variables: Nested access
{{ .internal.timestamp }}  # ✅ CORRECT (current time)

# ❌ WRONG: Don't use nested access for User variables
{{ .user.IP }}  # WRONG - User variables are flattened!
```

**Why this matters:**
- **User ergonomics**: Top-level access (`{{ .IP }}`) is more intuitive than `{{ .user.IP }}`
- **Namespace separation**: Global and Internal use nested access to avoid conflicts
- **Serialization control**: Internal variables are runtime-only, never persisted

**Variable lifecycle:**
- **User**: Configuration from external sources (persisted in database)
- **Global**: Fleet-wide settings (persisted in database)
- **Internal**: Runtime metadata (NOT persisted, reconstructed on startup)

### 7. Triangular Store Architecture

**FSMv2 uses a three-collection persistence model that separates worker state into Identity, Desired, and Observed.**

```
        IDENTITY
       (What it is)
           /\
          /  \
    DESIRED  OBSERVED
   (Want)     (Have)
       \      /
      RECONCILIATION
```

**Why three parts, not two:**

1. **Identity** (immutable): Worker identification (ID, IP, hostname)
   - Created once, never updated (`_version=1` forever)
   - Enables worker tracking across restarts

2. **Desired** (user intent): Configuration that SHOULD be deployed
   - Derived from user config via `DeriveDesiredState(spec)`
   - Versioned for optimistic locking (prevents lost updates from concurrent edits)

3. **Observed** (system reality): Configuration that IS deployed
   - Collected via `CollectObservedState()` polling external systems
   - NOT versioned (ephemeral, constantly changing)
   - Uses `_sync_id` (monotonic counter) instead

**Atomic snapshots prevent race conditions:**

```go
snapshot, err := store.LoadSnapshot(ctx, "container", "worker-123")
// All three parts loaded atomically via database transaction
// FSM never sees mismatched state (e.g., old desired + new observed)
```

**Collection-based storage enables:**
- **Delta sync**: "Give me all desired changes since sync_id=500"
- **Role queries**: "Show all observed states for health dashboard"
- **Independent versioning**: Desired participates in optimistic locking, observed doesn't

**What breaks without this pattern:**
- Can't detect config drift (desired ≠ observed)
- Can't distinguish "user config" from "deployed config"
- Race conditions in FSM (reading inconsistent snapshots)
- Version conflicts on every observed state poll

**Code reference**: See `pkg/cse/storage/triangular.go` for implementation details.

## Type Safety with Generics

FSMv2 uses Go generics for compile-time type safety across all components:

- **Supervisor**: `Supervisor[TObserved, TDesired]` ensures all workers have consistent state types
- **Collector**: `Collector[TObserved]` guarantees type-safe observation collection
- **Factory**: `RegisterFactory[TObserved, TDesired]()` provides typed worker registration

### Type Derivation

Worker types are automatically derived from observed state type names:
- `ParentObservedState` → `"parent"`
- `ContainerObservedState` → `"container"`

No manual WorkerType constants needed!

### Example Usage

```go
// Before (reflection-based, error-prone)
supervisor := NewSupervisor(Config{
    WorkerType: "parent",  // String constant, no compile-time checking
    // ...
})
collector := collection.NewCollector(collection.CollectorConfig{
    WorkerType: "parent",  // Must match supervisor string exactly
})
factory.RegisterFactoryByType("parent", func(id Identity) Worker {
    return NewParentWorker(id)
})

// After (generics-based, type-safe)
supervisor := NewSupervisor[ParentObservedState, *ParentDesiredState](Config{
    // WorkerType derived automatically from ParentObservedState
    // ...
})
collector := collection.NewCollector[ParentObservedState](collection.CollectorConfig{
    // Type parameter ensures matching with supervisor
})
factory.RegisterFactory[ParentObservedState, *ParentDesiredState](func(id Identity) Worker {
    return NewParentWorker(id)
})
```

**Benefits:**
1. **Compile-time type safety** - No runtime type errors
2. **IDE autocomplete** - Full type information in editors
3. **Reduced boilerplate** - No manual WorkerType constants
4. **Better refactoring** - Type changes caught at compile time
5. **Zero reflection** - Improved performance and debuggability

**Note:** DesiredState types often use pointer receivers for methods like `IsShutdownRequested()`, so use `*ParentDesiredState` as the type parameter.

### Responsibilities of the worker

The worker implements the `Worker` and `State` interfaces to provide business logic. See `doc.go` for complete interface definitions and API documentation.

**Key responsibilities:**
- `CollectObservedState()`: Gather current system state (potentially long-running, runs in separate goroutine)
- `DeriveDesiredState(spec)`: Transform user config into desired state (pure function, no side effects)
- `GetInitialState()`: Provide the starting state for the FSM
- `State.Next(snapshot)`: Decision logic for state transitions and actions

Each state MUST handle ShutdownRequested explicitly at the beginning of its Next() method. This makes shutdown paths visible and ensures proper cleanup sequencing.

```go
type RunningState struct{}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    // EXPLICIT: First check shutdown
    if snapshot.Desired.ShutdownRequested {
        // I'm running, so I need to stop first
        return StoppingState{}, SignalNone, nil
    }

    // ...
}
```

```go
type StoppedState struct{}

func (s StoppedState) Next(snapshot Snapshot) (State, Signal) {
    // EXPLICIT: When stopped and shutdown requested
    if snapshot.Desired.ShutdownRequested {
        // Now I can start cleanup
        return DeletingState{}, SignalNone, nil
    }

    // ...
}
```

```
type DeletedState struct{}

func (s DeletedState) Next(snapshot Snapshot) (State, Signal) {
    // Final state - signal removal
    return DeletedState{}, SignalNeedsRemoval, nil
}
```



### Responsibilities of the supervisor

The supervisor is then responsible for:
1. Calling the methods of the worker interface `DeriveDesiredState` and `NextState` in sequence (tick-based), and storing the result in a database
2. Ensures that during `Create()`, that will start a new goroutine that will in regular intervals call `CollectObservedState()`.
3. `Tick(snapshot)`, that will do this:
	1. Evaluate the state of the collector goroutine: how old is the latest value in the database from the observed state? Is the data stale (e.g., after 10 seconds) or maybe even “broken” (e.g., if it has been stale for another 10 seconds) and requires a restart? Are there errors appearing in CollectObservedState()?
	2. Calls `DeriveDesiredState(spec)`, where spec is the user configuration derived from the snapshot. Whatever desired state is returned here, is stored into the database\
	3. Calls `worker.CurrentState.Next(snapshot)` where snapshot is the latest available identity, observed state, desired state (see also RFC “CSE”). Upon
4. If an error occurs, it will check if the error is of the type “Retryable” or “Permanent”. Retryable errors are timeouts of all sorts and should be retried in the next tick with ExponentialBackoff and will be at one point of time if they keep repeating become a Permanent error. Permanent errors should cause a removal of the worker.
5. If a worker needs to be removed, the supervisor will set `ShutdownRequested` to true in the desired state. It is required as there is external state of the worker that needs to be cleaned up (e.g., the S6 files). This then causes the worker to reconcile towards a stopped state, and then towards removing itself
6. A worker can also request itself to be restarted, e.g., if it detected config changes that cannot be done online and that require a re-creation.
7. Shutdowns happen over multiple ticks
8. If the supervisor receives the `SignalNeedsRemoval` signal, it means the worker has successfully removed itself and we can remove it from our supervisor
9. If the supervisor receives the `SignalNeedsRestart` signal, it will set the `ShutdownRequested` to true.


Because the supervisor is taking care of the "tick" and not the worker itself, the supervisor can track "ticks in current state" for timeout detection, and also detect workers that are "stuck" (they are not progressing through states)

#### Design Decision: Data Freshness and Collector Health

The FSM separates two orthogonal concerns that could otherwise become entangled:

1. **Infrastructure concern** (supervisor): Is the observation collector working?
2. **Application concern** (states): Is the application/service healthy?

**Collector health is a supervisor concern, not a state concern.** States never see stale data - they assume observations are always fresh and focus purely on business logic (e.g., "is container CPU healthy?").

The supervisor checks data freshness **before** calling `state.Next()`. If observations are stale, the supervisor handles it:
- Pauses the FSM (doesn't call state transitions)
- Attempts to restart the collector with exponential backoff
- Escalates to graceful shutdown if collector cannot be recovered

**Why this separation matters:**

- **Simplicity**: States are pure business logic without infrastructure concerns - easier to write and test
- **Safety**: No risk of making decisions on stale data (e.g., thinking CPU is 50% when it's actually 95% and about to OOM)
- **Reliability**: Automatic collector recovery with clear escalation paths
- **Observability**: Clear separation makes it obvious where failures occur (collector infrastructure vs application health)

#### State Naming Convention

FSM v2 uses a strict naming convention to make state behavior immediately obvious:

**PASSIVE states** (observe and transition based on conditions):
- Use descriptive nouns/adjectives: `ActiveState`, `DegradedState`, `StoppedState`
- **Never emit actions** - only evaluate conditions and return next state
- Examples: Health monitoring, status observation

**ACTIVE states** (perform work with retry logic):
- Use "TryingTo" prefix: `TryingToStartState`, `TryingToAuthenticateState`, `TryingToStopState`
- **Always emit actions on every tick** until success
- Examples: Authentication, initialization, cleanup operations

**Rule of thumb:** If the state name uses a gerund ("-ing") but doesn't emit actions, it's named wrong. Either add the action or rename to a descriptive noun.

See `PATTERNS.md` for detailed state naming patterns, edge cases, and complete FSM examples.

## Testing Actions

All actions MUST include idempotency tests using the `VerifyActionIdempotency` helper (Invariant I10).

### Idempotency Requirement

Actions in FSM v2 are designed to be called multiple times safely. The supervisor may retry actions on failure, network issues, or state machine ticks. Therefore, **every action implementation must be idempotent**.

### Writing Idempotency Tests

Use the test helper from `pkg/fsmv2/supervisor/execution`:

```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
)

var _ = Describe("MyAction Idempotency", func() {
    It("should be idempotent", func() {
        action := &MyAction{
            // ... initialize action
        }

        execution.VerifyActionIdempotency(action, 3, func() {
            // Verify expected state after 3 executions
            // Example: check that calling action 3 times doesn't cause 3x the effect
        })
    })
})
```

### CI Enforcement

The CI pipeline runs `make check-idempotency-tests` which verifies that every action has at least one idempotency test. The build will fail if any action lacks this test.

To check locally:
```bash
make check-idempotency-tests
```

### Example

See `pkg/fsmv2/supervisor/execution/action_idempotency_test.go` for complete examples of idempotent and non-idempotent actions.

## Further Reading

This README provides mental models and usage guidance for FSMv2. For deeper understanding, consult these resources:

### Core Documentation

- **`doc.go`** - Complete API reference and quick start guide
  - Worker, State, and Action interfaces with detailed documentation
  - Example code for common patterns
  - Package-level overview and architectural concepts

- **`PATTERNS.md`** - Design rationale and established patterns
  - Why FSMv2 uses specific approaches (states as structs, pass-by-value immutability, etc.)
  - Pattern catalog with implementation guidance
  - Edge cases and advanced scenarios not covered in this README
  - Variable passing between parent and child workers (Pattern 10)

### Design Documents

- **`docs/design/fsmv2-child-observed-state-usage.md`** - Child observed state architecture
- **`docs/plans/fsmv2-package-restructuring.md`** - Complete restructuring plan and migration guide
- **`docs/plans/fsmv2-unused-code-integration.md`** - Integration of previously unused code

### Implementation Reference

- **`pkg/fsmv2/supervisor/supervisor.go`** - Orchestration and lifecycle management
- **`pkg/fsmv2/config/variables.go`** - Variable namespaces (User, Global, Internal)
- **`pkg/fsmv2/config/childspec.go`** - Hierarchical composition and child management
- **`pkg/cse/storage/triangular.go`** - Triangular store implementation (735 LOC)
