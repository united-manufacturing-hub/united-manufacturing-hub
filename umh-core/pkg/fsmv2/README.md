# FSM v2

Type-safe state machine framework for managing worker lifecycles with compile-time safety.

> **Implementation Details**: For Go idiom patterns, code examples, and API contracts, see `doc.go`.

## The Triangle Model

Every worker in FSMv2 is represented by three components:

```
        IDENTITY
       (What it is)
           /\
          /  \
         /    \
        /      \
    DESIRED  OBSERVED
   (Want)     (Have)
       \      /
        \    /
      RECONCILIATION
      (Make have = want)
```

- **Identity**: Immutable worker identification (ID, name, type)
- **Desired**: Configuration that SHOULD be deployed (from user config)
- **Observed**: Configuration that IS deployed (from monitoring)

The supervisor runs a single-threaded reconciliation loop that makes observed state match desired state.

## Quick Start

```bash
cd pkg/fsmv2/examples/simple
go run main.go
```

This example demonstrates:
- Application supervisor as entry point
- YAML configuration
- Parent-child worker hierarchy
- Graceful shutdown

### Minimal Worker Example

```go
// 1. Define your states (empty structs - behavior only)
type StoppedState struct{}
type TryingToStartState struct{}
type RunningState struct{}

// 2. Implement state transitions
func (s StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // ALWAYS check shutdown first
    if snapshot.Desired.(DesiredState).IsShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }
    // Transition to starting
    return TryingToStartState{}, fsmv2.SignalNone, nil
}

func (s TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    if snapshot.Desired.(DesiredState).IsShutdownRequested() {
        return StoppedState{}, fsmv2.SignalNone, nil
    }
    // Check if already running
    if snapshot.Observed.(ObservedState).IsRunning {
        return RunningState{}, fsmv2.SignalNone, nil
    }
    // Emit start action
    return s, fsmv2.SignalNone, &StartAction{}
}

// 3. Implement worker
type MyWorker struct{}

func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Query actual system state
    return &MyObservedState{IsRunning: checkIfRunning()}, nil
}

func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    // Transform user config to desired state
    return config.DesiredState{State: "running"}, nil
}

func (w *MyWorker) GetInitialState() fsmv2.State {
    return StoppedState{}
}
```

**Next steps:**
- Read `doc.go` for interface contracts and detailed patterns
- See `workers/example/` for complete reference implementations

## Core Concepts

### Single Tick Loop

The supervisor orchestrates worker lifecycle through a deterministic tick loop:

```
┌─────────────────────────────────────────────────────┐
│                   Tick Loop                         │
│                                                     │
│  1. CollectObservedState() → stored async          │
│  2. DeriveDesiredState(spec) → create snapshot     │
│  3. currentState.Next(snapshot)                    │
│       ↓                                            │
│     (nextState, signal, action)                    │
│  4. Execute action (with retry/backoff)            │
│  5. Transition to nextState                        │
│  6. Process signal (remove/restart)                │
│  7. Reconcile children (if parent worker)          │
│                                                    │
└─────────────────────────────────────────────────────┘
```

### States: Structs, Not Strings

States are concrete Go types, not string constants. This feels more aligned with Go idioms than using the FSM library (like in FSM_V1)

```go
// Type-safe transition (compiler validates)
return RunningState{}, fsmv2.SignalNone, nil

// Would not compile (type mismatch)
return "running", fsmv2.SignalNone, nil
```

### Idempotency Requirement

All actions MUST be idempotent - safe to call multiple times with same result.

**Actions are empty structs** - dependencies are injected via `Execute()`:

```go
// Action struct is EMPTY - no fields, no state
type StartProcessAction struct{}

func (a *StartProcessAction) Execute(ctx context.Context, depsAny any) error {
    // ALWAYS check context cancellation first
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    deps := depsAny.(MyDependencies)
    processPath := deps.GetProcessPath() // Get data from dependencies

    // Check if already done (idempotency)
    if deps.ProcessManager().IsRunning(processPath) {
        return nil  // Already started, safe to call again
    }
    return deps.ProcessManager().Start(ctx, processPath)
}
```

**Testing idempotency:**
```go
execution.VerifyActionIdempotency(action, 3, func() {
    Expect(fileExists("test.txt")).To(BeTrue())
})
```

### Implementation Patterns

For detailed implementation patterns including:
- State naming conventions (TryingTo/Ensuring prefixes)
- Shutdown handling requirements
- Immutability guarantees

**See `doc.go` for Go idiom patterns and code examples.**

### State XOR Action Rule

`State.Next()` should return EITHER a state change OR an action, not both:

```go
// Correct: Action without state change
return currentState, SignalNone, &SomeAction{}

// Correct: State change without action
return NewState{}, SignalNone, nil

// Avoid: Both at once (supervisor will reject)
return NewState{}, SignalNone, &SomeAction{}  // Don't do this
```

**Rationale**: Clear separation of concerns - transition logic vs side effects.

## Key Interfaces

| Interface | Purpose | Key Method |
|-----------|---------|------------|
| `Worker` | Worker implementation | `CollectObservedState()`, `DeriveDesiredState()`, `GetInitialState()` |
| `State[O, D]` | FSM state behavior | `Next(snapshot) → (State, Signal, Action)` |
| `Action[D]` | Idempotent operation | `Execute(ctx, deps) → error` |
| `ObservedState` | Current system state | `GetTimestamp() → time.Time` |
| `DesiredState` | Target configuration | `IsShutdownRequested() → bool` |

**For detailed contracts and usage patterns, see `doc.go`.**

## Type Safety with Generics

FSMv2 uses Go generics for compile-time type safety:

```go
// Supervisor with typed observed/desired states
supervisor := NewSupervisor[ParentObservedState, *ParentDesiredState](config)

// Worker types derived automatically from state type names:
// - ParentObservedState → "parent"
// - ContainerObservedState → "container"
```

No manual WorkerType constants needed - the compiler catches type mismatches.

## Variable Namespaces

Three tiers with special flattening for User variables in templates:

```yaml
# User variables: Top-level access (flattened)
{{ .IP }}     # From variables.User["IP"]
{{ .PORT }}   # From variables.User["PORT"]

# Global variables: Nested access
{{ .global.cluster_id }}  # From variables.Global

# Internal variables: Nested access (NOT serialized)
{{ .internal.timestamp }}  # Runtime-only
```

**Why three tiers:**
- **User**: Per-worker configuration (IP addresses, ports) - most common
- **Global**: Fleet-wide settings (cluster ID, environment) - shared
- **Internal**: Runtime metadata (timestamps, derived values) - not persisted

## Hierarchical Composition

Parents declare children via `ChildSpec` in `DeriveDesiredState()`:

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    return config.DesiredState{
        State: "running",
        ChildrenSpecs: []config.ChildSpec{
            {
                Name:       "mqtt-connection",
                WorkerType: "mqtt_client",
                UserSpec:   config.UserSpec{Config: "url: tcp://localhost:1883"},
                StateMapping: map[string]string{
                    "active":  "connected",  // Parent state → child desired state
                    "closing": "stopped",
                },
            },
        },
    }, nil
}
```

Supervisor handles child creation, updates, and cleanup automatically.

**Note**: `ChildSpec` is intentionally different from supervisor `YAMLConfig`. `StateMapping` is child-specific coordination that belongs in ChildSpec, not top-level config.

**StateMapping** coordinates FSM states (not data passing). Use `VariableBundle` for data.

## Testing

### Run Architecture Tests

```bash
# See all enforced patterns with explanations
ginkgo run --focus="Architecture" -v ./pkg/fsmv2/

# Run all FSMv2 tests
ginkgo -r ./pkg/fsmv2/
```

## Implementation Reference

| File | Purpose |
|------|---------|
| `doc.go` | Package overview, quick start, key concepts |
| `api.go` | Core interfaces (Worker, State, Action, Snapshot) |
| `dependencies.go` | Core dependency injection contract |
| `internal/helpers/` | Convenience helpers (BaseState, BaseWorker, ConvertSnapshot) |
| `supervisor/supervisor.go` | Orchestration and lifecycle management |
| `supervisor/execution/` | Action execution with retry/backoff |
| `supervisor/collection/` | Observed state collection |
| `supervisor/health/` | Staleness detection and freshness |
| `supervisor/lockmanager/` | Thread safety and lock ordering |
| `config/variables.go` | Variable namespaces (User, Global, Internal) |
| `config/childspec.go` | Hierarchical composition |
| `workers/example/` | Reference worker implementations |
| `architecture_test.go` | Enforced patterns with WHY explanations |
| `internal/validator/` | AST-based architectural validation helpers |

## Storage Layer

FSMv2 uses `TriangularStoreInterface` (from `pkg/cse/storage`) for state persistence:

- **Identity**: Saved once during worker creation, immutable
- **Desired**: Stored with version management for optimistic locking
- **Observed**: Stored with delta checking (skips writes if unchanged)

The supervisor handles all storage operations automatically:
- `SaveIdentity()` during worker creation
- `SaveDesired()` when configuration changes
- `SaveObserved()` during async collection (with delta checking)
- `LoadSnapshot()` for atomic loading of all three parts

**See `pkg/cse/storage/doc.go` for storage WHYs.**
