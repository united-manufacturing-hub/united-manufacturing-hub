# FSM v2

Type-safe state machine framework for managing worker lifecycles with compile-time safety.

> **Implementation Details**: For Go idiom patterns, code examples, and API contracts, see `doc.go`.

## Why FSMv2?

FSMv2 solves problems that emerged from UMH Classic (Kubernetes-based) and FSMv1:

| Problem | FSMv2 Solution |
|---------|----------------|
| Kubernetes is complex for edge devices | Single Docker container with S6 process supervision |
| Pod states don't reflect actual health | Explicit states: `running`, `degraded` (with reason), `stopped` |
| 30+ second response time | Sub-100ms configuration changes |
| Goroutine async bugs in Classic (still unexplained failures in Sentry) | Single-threaded tick loop, no async coordination needed |

**FSMv2 implements the same control loop pattern as Kubernetes controllers and PLCs:**
Desired state → Compare → Actuate → Observe → Repeat.

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
- **Desired**: Target state to achieve (derived from user intent via DeriveDesiredState)
- **Observed**: Configuration that IS deployed (from monitoring)

The supervisor runs a single-threaded reconciliation loop that makes observed state match desired state.

### Why This Model?

The Triangle Model maps directly to control loop theory: **Identity** (what am I controlling?),
**Desired** (set point), **Observed** (process variable), **Reconciliation** (controller logic).
This is the same pattern used in Kubernetes controllers, PLCs, and thermostats.

The Triangle Model aligns with the Control Sync Engine (CSE) storage pattern used throughout UMH.
CSE separates each worker into three parts with different lifecycles:
- **Identity**: Write-once, immutable (ID, name, type)
- **Desired**: Versioned with optimistic locking (user intent)
- **Observed**: Ephemeral, reconstructed by polling (system reality)

This enables delta streaming to clients - only actual changes increment sync IDs, so clients
requesting "changes since sync_id X" receive only meaningful updates.

See `pkg/cse/storage/doc.go` for full CSE documentation.

### The Transformation Pipeline

The Triangle Model shows reconciliation. But where does Desired state come from?
User configuration is **transformed**, not copied:

```
UserSpec.Config (raw YAML)     →  "children_count: 5"
        ↓
    DeriveDesiredState()       →  parses, validates, computes
        ↓
DesiredState (technical)       →  { State: "running", ChildrenSpecs: [5 objects] }
```

TODO:
- change statemapping to be not generic to any states, and i think we only need a if parent is stopped, stop all child;ren as well"


**Key insight**: `DesiredState` structure may be COMPLETELY DIFFERENT from `UserSpec`.
The `DeriveDesiredState()` function is a transformation, not a copy - it can:
- Parse YAML/JSON into typed structs
- Validate user input
- Compute derived fields (e.g., generate ChildSpec array from a count)
- Apply defaults

See `workers/example/exampleparent/worker.go` for a concrete example.

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

### Tick Loop

The supervisor orchestrates worker lifecycle through a reconciliation loop:

1. **Collect** observed state (runs async in background)
2. **Derive** desired state by transforming UserSpec
3. **Decide** via `State.Next(snapshot)` → returns (nextState, signal, action)
4. **Execute** action async (with retry/backoff) while loop continues

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

**Rationale**: There should be one way to progress: emit action → observe result → then transition.
Allowing both creates confusion about the intended flow. The supervisor enforces this by panicking
if both are returned - this catches logic errors during development.

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

## Testing

### Run Architecture Tests

```bash
# See all enforced patterns with explanations
ginkgo run --focus="Architecture" -v ./pkg/fsmv2/

# Run all FSMv2 tests
ginkgo -r ./pkg/fsmv2/
```

## Further Reading

- [Migration from FSMv1](docs/migration-from-v1.md) - Quick reference for v1 developers
- [Kubernetes Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) - The control loop pattern
- [IT/OT Control Loops](https://learn.umh.app/lesson/introduction-into-it-ot-control-loop/) - Manufacturing context
- [UMH Core vs Classic](https://docs.umh.app/umh-core-vs-classic-faq) - Why we moved from Kubernetes
- [Why S6 exists](https://skarnet.org/software/s6/why.html) - Process supervision philosophy
- [The Docker Way](https://github.com/just-containers/s6-overlay?tab=readme-ov-file#the-docker-way) - Multi-process container rationale
