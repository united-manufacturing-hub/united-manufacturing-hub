# FSM v2 Package Usage Guide

## Overview

The FSM v2 package provides a type-safe, hierarchical finite state machine framework for managing worker lifecycle in the United Manufacturing Hub. It separates concerns into three clear layers:

- **Worker**: Business logic implementation (what the worker does)
- **State**: Decision logic (when to transition, what actions to take)
- **Supervisor**: Orchestration (tick loop, action execution, child management)

This architecture enables pure functional state transitions, explicit action execution with retry/backoff, and declarative child management following Kubernetes-style patterns.

## Package Structure

```
pkg/fsmv2/
├── api.go                      # Core interfaces (Worker, State, Action)
├── doc.go                      # Complete API reference and quick start
├── README.md                   # Mental models and usage guidance
├── PATTERNS.md                 # Design patterns and rationale
│
├── config/                     # Configuration types
│   ├── childspec.go           # Child worker specification
│   ├── variables.go           # Variable namespaces (User/Global/Internal)
│   └── template.go            # Template expansion
│
├── supervisor/                 # Orchestration layer
│   ├── supervisor.go          # Main supervisor implementation
│   ├── execution/             # Action execution with retry/backoff
│   ├── collection/            # Observed state collection
│   ├── health/                # Freshness checking
│   └── metrics/               # Prometheus metrics
│
├── factory/                    # Worker type registration
│   └── worker_factory.go      # Type-safe factory pattern
│
├── root/                       # Root supervisor implementation
│   ├── worker.go              # Passthrough root worker
│   └── types.go               # Root-specific types
│
└── workers/                    # Example implementations
    ├── example/
    │   ├── example-child/     # Child worker example
    │   ├── example-parent/    # Parent worker example
    │   └── root_supervisor/   # Root supervisor example
    └── communicator/          # Production communicator worker
```

## Key Concepts

### 1. The Triangle Model (Identity + Desired + Observed)

FSM v2 uses a three-part model for worker state:

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

- **Identity**: Immutable worker identification (ID, Name, WorkerType)
- **Desired**: What state the system SHOULD be in (from user config)
- **Observed**: What state the system IS in (from monitoring)

### 2. States Are Structs, Not Strings

FSM v2 uses typed structs for states, providing compile-time type safety:

```go
// Type-safe state (FSM v2)
type RunningState struct{}
type StoppedState struct{}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    return StoppedState{}, SignalNone, nil  // Compiler enforces valid states
}

// String-based FSM (error-prone)
state := "runing"  // Typo not caught by compiler!
```

### 3. The Tick Loop

The supervisor orchestrates worker lifecycle through a continuous tick loop:

```
┌─────────────────────────────────────────────────────┐
│                   Tick Loop                         │
│                                                     │
│  1. CollectObservedState() → Database               │
│  2. DeriveDesiredState() → Snapshot                 │
│  3. currentState.Next(snapshot)                     │
│       ↓                                             │
│     (nextState, signal, action)                     │
│  4. Execute action (with retry/backoff)             │
│  5. Transition to nextState                         │
│  6. Process signal (remove/restart)                 │
│  7. Reconcile children (if parent worker)           │
│                                                     │
└─────────────────────────────────────────────────────┘
```

## How to Use: Step-by-Step Guide

### Step 1: Define Your Dependencies

Create a dependencies struct containing everything your worker needs:

```go
type MyWorkerDeps struct {
    Logger      *zap.Logger
    Config      *MyConfig
    HTTPClient  *http.Client
}
```

### Step 2: Define Your States

Each state is a struct implementing the `State` interface:

```go
// Active states use "TryingTo" prefix and emit actions
type TryingToConnectState struct{}

func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Check if action succeeded (transition to next state)
    if snap.Observed.ConnectionStatus == "connected" {
        return &ConnectedState{}, fsmv2.SignalNone, nil
    }

    // Keep trying (emit action again)
    return s, fsmv2.SignalNone, &ConnectAction{}
}

func (s *TryingToConnectState) String() string {
    return fsmv2.DeriveStateName(s)
}

func (s *TryingToConnectState) Reason() string {
    return "Attempting to establish connection"
}
```

```go
// Passive states use descriptive nouns and only observe
type ConnectedState struct{}

func (s *ConnectedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Check if connection lost
    if snap.Observed.ConnectionStatus != "connected" {
        return &DisconnectedState{}, fsmv2.SignalNone, nil
    }

    // Stay in current state
    return s, fsmv2.SignalNone, nil
}

func (s *ConnectedState) String() string {
    return fsmv2.DeriveStateName(s)
}

func (s *ConnectedState) Reason() string {
    return "Connection established and healthy"
}
```

**State Naming Convention:**
- **Active states**: Use "TryingTo" prefix (e.g., `TryingToConnectState`, `TryingToStartState`)
  - Emit actions on every tick until success
  - Represent ongoing operations that need retrying
- **Passive states**: Use descriptive nouns (e.g., `ConnectedState`, `StoppedState`, `DegradedState`)
  - Only observe and transition based on conditions
  - Never emit actions

### Step 3: Define Your Actions

Actions represent side effects that are idempotent and retriable:

```go
type ConnectAction struct {
    // Empty struct - dependencies injected via Execute()
}

func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
    deps := depsAny.(MyWorkerDeps)

    // Check if already done (IDEMPOTENCY REQUIREMENT)
    if alreadyConnected() {
        return nil  // Safe to call again
    }

    // Perform the action
    return establishConnection(ctx, deps.HTTPClient)
}

func (a *ConnectAction) Name() string {
    return "connect"
}
```

**Idempotency Requirement:** Every action MUST check if work is already done before performing it. The supervisor retries failed actions with exponential backoff.

### Step 4: Implement the Worker Interface

The worker ties everything together:

```go
package myworker

import (
    "context"
    "time"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type MyWorker struct {
    *fsmv2.BaseWorker[*MyWorkerDeps]
    identity fsmv2.Identity
    logger   *zap.SugaredLogger
}

func NewMyWorker(id, name string, logger *zap.SugaredLogger) *MyWorker {
    deps := &MyWorkerDeps{Logger: logger}

    return &MyWorker{
        BaseWorker: fsmv2.NewBaseWorker(deps),
        identity: fsmv2.Identity{
            ID:         id,
            Name:       name,
            WorkerType: storage.DeriveWorkerType[MyObservedState](),
        },
        logger: logger,
    }
}

// CollectObservedState gathers current system state
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Poll external systems, check process status, etc.
    observed := MyObservedState{
        ID:               w.identity.ID,
        CollectedAt:      time.Now(),
        ConnectionStatus: w.checkConnectionStatus(),
    }

    return observed, nil
}

// DeriveDesiredState transforms user config into desired state
func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    // Handle nil spec (used during initialization)
    if spec == nil {
        return config.DesiredState{
            State:         "connected",
            ChildrenSpecs: nil,
        }, nil
    }

    userSpec, ok := spec.(config.UserSpec)
    if !ok {
        return config.DesiredState{}, fmt.Errorf("invalid spec type")
    }

    // Parse user config and generate desired state
    return config.DesiredState{
        State:         "connected",
        ChildrenSpecs: nil,  // Or populate with child specs for parent workers
    }, nil
}

// GetInitialState returns the starting state
func (w *MyWorker) GetInitialState() fsmv2.State[any, any] {
    return &StoppedState{}
}
```

### Step 5: Register Worker in Factory

Enable dynamic worker creation via the factory pattern:

```go
func init() {
    // Register worker factory
    _ = factory.RegisterFactory[MyObservedState, *MyDesiredState](
        func(identity fsmv2.Identity) fsmv2.Worker {
            return NewMyWorker(identity.ID, identity.Name, logger)
        })

    // Register supervisor factory (required for parent-child relationships)
    _ = factory.RegisterSupervisorFactory[MyObservedState, *MyDesiredState](
        func(cfg interface{}) interface{} {
            supervisorCfg := cfg.(supervisor.Config)
            return supervisor.NewSupervisor[MyObservedState, *MyDesiredState](supervisorCfg)
        })
}
```

### Step 6: Create and Run the Supervisor

The supervisor manages the worker lifecycle:

```go
import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

func main() {
    ctx := context.Background()
    logger := zap.NewDevelopment().Sugar()

    // Create storage (triangular store for Identity/Desired/Observed)
    basicStore := memory.NewInMemoryStore()
    triangularStore := storage.NewTriangularStore(basicStore)

    // Create supervisor
    sup := supervisor.NewSupervisor[MyObservedState, *MyDesiredState](supervisor.Config{
        WorkerType:   storage.DeriveWorkerType[MyObservedState](),
        Store:        triangularStore,
        Logger:       logger,
        TickInterval: 1 * time.Second,
    })

    // Create worker identity
    identity := fsmv2.Identity{
        ID:         "worker-1",
        Name:       "My Worker",
        WorkerType: storage.DeriveWorkerType[MyObservedState](),
    }

    // Create and add worker
    worker := NewMyWorker(identity.ID, identity.Name, logger)
    err := sup.AddWorker(identity, worker)
    if err != nil {
        log.Fatal(err)
    }

    // Start supervisor (begins tick loop)
    if err := sup.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Wait for termination signal
    <-ctx.Done()
    sup.Shutdown()
}
```

## Parent-Child Hierarchies

FSM v2 supports hierarchical composition where parent workers manage child workers:

### Parent Worker: Declaring Children

Parents declare children via `ChildrenSpecs` in `DeriveDesiredState()`:

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    userSpec := spec.(config.UserSpec)

    var parentSpec ParentUserSpec
    yaml.Unmarshal([]byte(userSpec.Config), &parentSpec)

    // Declare desired children
    childrenSpecs := make([]config.ChildSpec, parentSpec.ChildrenCount)
    for i := range parentSpec.ChildrenCount {
        childrenSpecs[i] = config.ChildSpec{
            Name:       fmt.Sprintf("child-%d", i),
            WorkerType: "child",  // Must match registered worker type
            UserSpec:   config.UserSpec{
                Config: "value: 10",  // Child-specific config
            },
            StateMapping: map[string]string{
                "running": "connected",  // Parent "running" → child "connected"
                "stopped": "stopped",    // Parent "stopped" → child "stopped"
            },
        }
    }

    return config.DesiredState{
        State:         "running",
        ChildrenSpecs: childrenSpecs,
    }, nil
}
```

**Key Points:**
- **Declarative**: Don't call `CreateChild()` or `DeleteChild()` imperatively
- **Automatic reconciliation**: Supervisor diffs desired vs actual children
- **StateMapping**: Coordinates parent-child FSM states (NOT data passing)
- **VariableBundle**: Use for passing configuration data to children

### Example: Complete Hierarchy

See `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/` for complete working examples:

- **example-child/**: Child worker with connection management
- **example-parent/**: Parent worker managing multiple children
- **root_supervisor/**: Root supervisor using passthrough pattern

## Code Examples from Codebase

### Example Child Worker

File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-child/worker.go`

Key implementation:
- Lines 34-66: Worker struct and constructor using `BaseWorker[D]` pattern
- Lines 68-78: `CollectObservedState()` implementation
- Lines 80-106: `DeriveDesiredState()` with YAML parsing
- Lines 108-111: `GetInitialState()` returns initial state
- Lines 127-133: Factory registration in `init()`

### Example State Implementation

File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-child/state/state_trying_to_connect.go`

Key implementation:
- Lines 28-42: `Next()` method with snapshot conversion, shutdown check, transition logic
- Lines 44-50: `String()` and `Reason()` methods for debugging

### Example Action Implementation

File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-child/action/connect.go`

Key implementation:
- Lines 28-30: Empty action struct (dependencies injected at execution)
- Lines 40-45: `Execute()` with dependency injection and idempotency
- Lines 47-53: `String()` and `Name()` methods

### Integration Test Example

File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/integration/phase0_happy_path_test.go`

Shows complete workflow:
- Lines 88-117: Factory registration for parent and child workers
- Lines 119-142: Creating supervisor with worker
- Lines 144-150: Setting UserSpec to configure children

## Common Patterns

### 1. Shutdown Handling

States MUST check shutdown first in `Next()`:

```go
func (s RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &StoppingState{}, fsmv2.SignalNone, nil
    }

    // ... rest of logic
}
```

### 2. Final State with Removal Signal

Terminal states signal removal to supervisor:

```go
type DeletedState struct{}

func (s *DeletedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    // Already deleted, signal for removal from supervisor
    return s, fsmv2.SignalNeedsRemoval, nil
}
```

### 3. Type-Safe Dependencies

Use `BaseWorker[D]` for typed dependency access:

```go
type MyWorker struct {
    *fsmv2.BaseWorker[*MyWorkerDeps]
}

func (w *MyWorker) someMethod() {
    deps := w.GetDependencies()
    deps.Logger.Info("...")  // Type-safe, no casting
}
```

### 4. Testing State Transitions

Test states by calling `Next()` with test snapshots:

```go
var _ = Describe("StoppedState", func() {
    It("transitions to connecting when desired state wants running", func() {
        state := &StoppedState{}
        snapshot := fsmv2.Snapshot{
            Identity: fsmv2.Identity{ID: "test"},
            Observed: MyObservedState{},
            Desired:  &MyDesiredState{State: "connected"},
        }

        nextState, signal, action := state.Next(snapshot)

        Expect(nextState).To(BeAssignableToTypeOf(&TryingToConnectState{}))
        Expect(signal).To(Equal(fsmv2.SignalNone))
        Expect(action).To(BeNil())
    })
})
```

## Best Practices

1. **Keep `Next()` pure**: No side effects, no external calls
2. **Make actions idempotent**: Check if work already done before performing
3. **Check `ShutdownRequested()` first**: In all state `Next()` methods
4. **Use type-safe state structs**: Not strings
5. **Return action OR transition**: Not both simultaneously
6. **Handle context cancellation**: In all async operations
7. **Test action idempotency**: Use `VerifyActionIdempotency` helper

## Further Reading

- **`/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/README.md`**: Mental models and key concepts
- **`/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/doc.go`**: Complete API reference
- **`/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/PATTERNS.md`**: Design patterns and rationale
- **`/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/`**: Complete working examples

## Summary

The FSM v2 package provides:

- **Type-safe state machines** using Go generics and struct-based states
- **Pure functional state transitions** with immutable snapshots
- **Declarative child management** following Kubernetes patterns
- **Automatic retry/backoff** for failed actions
- **Hierarchical composition** for complex multi-worker systems
- **Compile-time guarantees** instead of runtime errors

The key to using FSM v2 effectively is understanding the separation of concerns:
- **Workers** implement business logic (what to do)
- **States** implement decision logic (when to do it)
- **Supervisor** handles orchestration (how to do it reliably)

This architecture makes complex distributed systems easier to reason about, test, and maintain.
