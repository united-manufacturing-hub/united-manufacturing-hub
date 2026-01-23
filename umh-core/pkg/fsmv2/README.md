# FSMv2

Type-safe state machine framework for managing worker lifecycles with compile-time safety.

> **Implementation Details**: For Go idiom patterns, code examples, and API contracts, see `doc.go`.

## How It Works (60 seconds)

FSMv2 is a **state machine supervisor**:

1. **You define** States (decision logic) and Actions (I/O operations)
2. **Supervisor runs a loop**: Observe → Compare → Decide → Act → Repeat (~4µs per worker per tick)
3. **State transitions happen** when observations change, NOT when actions complete

```text
Config Change → DeriveDesiredState() → Supervisor compares → State.Next() decides → Action executes → CollectObservedState() → Loop
```

**What you implement**: 3 Worker methods + States + Actions. **What you don't**: retries, timeouts, metrics, lifecycle management.

> **Quick Start**: Jump to [File Structure](#file-structure-for-new-worker) and follow the 5-step guide to create your first worker.

## Why FSMv2?

FSMv2 solves problems that emerged from UMH Classic (Kubernetes-based) and FSMv1:

| Problem | FSMv2 Solution |
|---------|----------------|
| **2-3 days** to set up FSMv1 boilerplate | ~55% less code, focused on business logic |
| 30+ second pod creation in Kubernetes | Sub-100ms configuration changes |
| "Callback hell" - 6 channels, 4 goroutines in Communicator alone | Single-threaded tick loop, no async coordination |
| AI couldn't help (would "freestyle and break things") | Clean patterns AI can understand and generate |
| Pod states don't reflect actual health | Explicit states: `running`, `degraded` (with reason), `stopped` |

**FSMv2 implements the same control loop pattern as Kubernetes controllers and PLCs:**
Desired state → Compare → Actuate → Observe → Repeat.

### For FSMv1 Developers

| FSMv1 Pattern | FSMv2 Equivalent |
|---------------|------------------|
| 8 files per FSM (machine.go, actions.go, reconcile.go, ...) | 3 Worker methods + states + actions |
| looplab FSM library with callbacks | Struct-based states with `Next()` method |
| State as string constants | State as Go types (compile-time safety) |
| Business logic mixed with boilerplate | Worker has business logic, Supervisor has boilerplate |
| `fsm_callbacks.go` (fail-free) | States are pure functions (no I/O) |
| `manager.go` lifecycle handling | Supervisor handles automatically |
| Observation snapshot via complex manager | `CollectObservedState()` - you implement |
| Desired via config parsing | `DeriveDesiredState()` - you implement |

**Key mindset shift**: You write business logic (states, actions). The supervisor handles everything else.

## Worker Interface (What YOU Write)

### The 3 Worker Methods

```go
type Worker interface {
    // Query actual system state (runs in background goroutine, ~1s interval)
    // Must include timestamp for staleness detection
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // Transform user config to desired state (called on config change)
    DeriveDesiredState(spec interface{}) (DesiredState, error)

    // Return the initial state for new workers (called once at startup)
    GetInitialState() State
}
```

### States: Pure Functions with Next()

States are concrete Go types (not strings). Each state implements `Next()`:

```go
// States are empty structs - behavior only
type StoppedState struct{}
type TryingToStartState struct{}
type RunningState struct{}

// Next() returns: (nextState, signal, action)
func (s TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // Type assertions are safe here - each worker defines its own concrete types
    // and the supervisor guarantees type consistency within a worker
    desired := snapshot.Desired.(DesiredState)
    observed := snapshot.Observed.(ObservedState)

    // ALWAYS check shutdown first
    if desired.IsShutdownRequested() {
        return StoppedState{}, fsmv2.SignalNone, nil
    }
    // Check observation - did the process start?
    if observed.IsRunning {
        return RunningState{}, fsmv2.SignalNone, nil  // Transition based on observation
    }
    // Not running yet - emit action to start it
    return s, fsmv2.SignalNone, &StartAction{}  // Stay in same state, emit action
}
```

### State Transitions: Observation-Driven

**Critical principle**: State transitions happen when observed state changes, NOT when actions complete.

**Why?** Actions can fail. If we transitioned on action completion, we'd need complex error handling. Instead: emit action → observe result → then transition.

**Important**: Return EITHER a state change OR an action from `Next()`, not both. See [State XOR Action Rule](#state-xor-action-rule).

### State Names and Reasons (Required Methods)

Every state must implement two additional methods for observability:

```go
// String() returns the state name for logging and metrics
func (s TryingToStartState) String() string {
    return "trying_to_start"
}

// Reason() provides human-readable context about the state
func (s DegradedState) Reason() string {
    return "degraded: 5 consecutive sync errors, retrying with backoff"
}
```

**Naming convention**: Use lowercase, underscore-separated names (`stopped`, `trying_to_connect`, `running`, `degraded`).

### Signals

States return a signal alongside the next state and action:

| Signal | Meaning | When to Use |
|--------|---------|-------------|
| `SignalNone` | Normal operation | Default for most transitions |
| `SignalNeedsRemoval` | Ready to be removed | After cleanup complete, parent can delete this worker |
| `SignalNeedsRestart` | Needs full restart | Unrecoverable error, request supervisor restart with backoff |

### Actions: Where I/O Happens

All actions MUST be idempotent - safe to call multiple times with same result.

```go
type StartProcessAction struct{}  // Empty struct - no fields

func (a *StartProcessAction) Execute(ctx context.Context, deps any) error {
    // Type assertion is safe - supervisor guarantees correct dependency type per worker
    d := deps.(MyDependencies)

    // Check if already done (idempotency)
    if d.ProcessManager().IsRunning() {
        return nil  // Already started, safe to call again
    }
    return d.ProcessManager().Start(ctx)
}

func (a *StartProcessAction) Name() string { return "StartProcess" }
```

**Thread-safety note**: The tick loop is single-threaded (State.Next() is safe), but Actions execute in a parallel worker pool. Dependencies passed to `Execute()` must use mutexes/atomics for any shared state modified by actions.

### I/O Isolation Rule

| Component | Can Do I/O | Purpose |
|-----------|------------|---------|
| States | NO (pure functions) | Decide next state + action |
| Actions | YES (idempotent) | Perform HTTP, file, network I/O |
| Worker | NO (read-only) | Collect observations |

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

## File Structure for New Worker

```text
workers/myworker/
├── worker.go           # Worker interface (3 methods)
├── userspec.go         # User configuration schema
├── dependencies.go     # External dependencies (HTTP client, etc.)
├── snapshot/
│   ├── observed.go     # What system actually is (the "core")
│   └── desired.go      # What user wants
├── state/
│   ├── stopped.go      # Initial state
│   ├── trying_to_start.go  # Transitional state
│   └── running.go      # Target state
└── action/
    ├── start.go        # I/O operation (idempotent)
    └── stop.go         # Cleanup action
```

### Quick Start: 5 Steps

1. **Copy** the file structure template above
2. **Define** `ObservedState` and `DesiredState` in `snapshot/`
3. **Implement** the 3 Worker methods in `worker.go`
4. **Create** states with `Next()` functions
5. **Test** with local runner: `go run pkg/fsmv2/cmd/runner/main.go --scenario=simple`

## The Triangle Model

Every worker in FSMv2 is represented by three components:

```text
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
- **Observed**: Actual system state (from CollectObservedState)

The supervisor runs a single-threaded reconciliation loop that makes observed state match desired state.

### The Transformation Pipeline (Thermostat Analogy)

User configuration is **transformed**, not copied:

```text
UserSpec (user intent)      →  "I want it warm at 8am"
        ↓
    DeriveDesiredState()    →  parses, validates, computes
        ↓
DesiredState (technical)    →  { Temperature: 22°C, Humidity: 45%, Schedule: [...] }
```

**Think of it like a smart thermostat**: The user enters "I want it warm" (UserSpec). The thermostat derives the ideal humidity and temperature settings (DesiredState). The control loop then makes the actual room match those settings.

**Key insight**: `DesiredState` may be COMPLETELY DIFFERENT from `UserSpec`. The `DeriveDesiredState()` function can parse YAML, validate input, compute derived fields, and apply defaults.

### Why "Snapshot"?

The term comes from PLC control theory: PLCs read all input states and save them into a "processing image of inputs" - a frozen point-in-time view. The PLC runs its logic based on that snapshot, ensuring deterministic behavior. FSMv2 follows the same pattern.

## Real-World Example: Communicator Worker

The old Communicator had "6 channels and 4 goroutines to send messages to REST API... impossible to debug."

FSMv2 Communicator is a clean state machine:

```text
Stopped → TryingToAuthenticate → Syncing ↔ Degraded
   ↑              ↓                 ↓         ↓
   └──────────────┴─────────────────┴─────────┘
              (Token expiry or auth loss)
```

**State Machine Flow**:
1. `StoppedState`: Initial state, transitions on startup
2. `TryingToAuthenticateState`: Emits `AuthenticateAction` (HTTP POST) with exponential backoff
3. `SyncingState`: Emits `SyncAction` continuously (HTTP pull/push loop), checks token expiry
4. `DegradedState`: Error recovery with intelligent backoff by error type

**Token Expiry Handling**: `SyncingState` checks `IsTokenExpired()` and transitions back to `TryingToAuthenticateState` to refresh JWT before continuing.

**Consecutive Error Handling**: Tracks consecutive errors per operation type. When threshold exceeded, triggers automatic transport reset for recovery from transient network failures.

**Key Design Decisions**:
- I/O operations (HTTP) isolated in Actions, never in States
- Actions update shared state via Dependencies (thread-safe setters)
- CollectObservedState reads from Dependencies (thread-safe getters)

## What the Supervisor Handles Automatically

Developers implement business logic; the supervisor handles everything else:

| Feature | What You Don't Write | Default Behavior |
|---------|---------------------|------------------|
| **Observation** | Backoff on errors, stale detection | 10s stale → pause, 20s → restart collector |
| **Actions** | Worker pool, panic recovery, timeout | 10 workers, 30s timeout, auto-retry |
| **Metrics** | FrameworkMetrics auto-injected | TimeInCurrentStateMs, StateTransitionsTotal |
| **Persistence** | Delta checking (skips unchanged writes) | Automatic I/O reduction |
| **Shutdown** | `IsShutdownRequested()` propagated to all states | Graceful cleanup |
| **Templates** | Variable expansion (User/Global/Internal) | Strict mode (fails on missing) |
| **Children** | Spawn, manage, reconcile child workers | Automatic lifecycle |

### Template Variables

Three namespaces, flattened for templates:
- **User**: `{{ .IP }}`, `{{ .PORT }}` (from connection config)
- **Global**: `{{ .global.api_endpoint }}` (fleet-wide settings)
- **Internal**: `{{ .internal.id }}` (runtime-only, not persisted)
- **Location**: `{{ .location_path }}` (computed from ISA-95 hierarchy)

Strict mode: missing variables fail immediately (no silent omissions).

### Child Worker Management

Define a child in `DeriveDesiredState()` → supervisor spawns it automatically:

```go
func (w *MyWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &MyDesiredState{
        Children: []ChildSpec{
            {Name: "child-1", WorkerType: "mychildtype", UserSpec: childConfig},
        },
    }, nil
}
```

**Accessing children**:
- `ChildrenView.List()` - all children with state info
- `ChildrenView.Get(name)` - single child by name
- `ChildrenView.AllHealthy()` / `AllStopped()` - aggregate checks
- `StateReader.LoadObservedTyped()` - query child state from parent

### FrameworkMetrics (Auto-Injected)

Available via `snapshot.Observed.Metrics.Framework`:
- `TimeInCurrentStateMs` - how long in current state
- `StateTransitionsTotal` - total transition count
- `CollectorRestarts` - observation collector restart count
- `StartupCount` - persistent counter across restarts

### Data Freshness Protection

The supervisor protects against stale observations:
1. Data older than 10s → FSM paused with clear logging
2. Data older than 20s → collector restarted with exponential backoff
3. Max 3 restart attempts → graceful shutdown requested
4. Circuit breaker state → observable via Prometheus

## Testing

### Local Runner

Run FSM scenarios locally without deploying (solves the "1 minute iteration time" problem):

```bash
# List available scenarios
go run pkg/fsmv2/cmd/runner/main.go --list

# Run a scenario
go run pkg/fsmv2/cmd/runner/main.go --scenario=simple --duration=10s

# Debug mode with tracing
go run pkg/fsmv2/cmd/runner/main.go --scenario=communicator --log-level=debug
```

Built-in scenarios: `simple`, `failing`, `panic`, `cascade`, `timeout`, `communicator`

### Unit Tests

```bash
# Run all FSMv2 tests
ginkgo -r ./pkg/fsmv2/

# Run with race detection
ginkgo -r -race ./pkg/fsmv2/

# Focus on specific patterns
ginkgo run --focus="State transitions" -v ./pkg/fsmv2/
```

### Architecture Tests

Validate enforced patterns (lock ordering, immutability, state naming):

```bash
ginkgo run --focus="Architecture" -v ./pkg/fsmv2/
```

### Testing Your Worker

1. **State transitions**: Call `Next()` with test snapshots
2. **Action idempotency**: Verify actions are safe to call multiple times
3. **Shutdown handling**: Test `IsShutdownRequested()` path in all states

**Example test pattern:**

```go
It("should transition to Running when process is observed", func() {
    snapshot := fsmv2.Snapshot{
        Observed: &MyObservedState{IsRunning: true},
        Desired:  &MyDesiredState{},
    }
    state := TryingToStartState{}
    nextState, signal, action := state.Next(snapshot)

    Expect(nextState).To(BeAssignableToTypeOf(RunningState{}))
    Expect(signal).To(Equal(fsmv2.SignalNone))
    Expect(action).To(BeNil())
})
```

## Troubleshooting Quick Reference

| Symptom | Check | Resolution |
|---------|-------|------------|
| Stuck in `TryingTo*` state | Action logs for errors | Fix action failure or external dependency |
| Running but not working | Observation timestamps | Verify `CollectObservedState()` queries actual system |
| Won't shut down | `IsShutdownRequested()` check | Ensure all states check shutdown first |
| Action runs multiple times | Action idempotency | Add "already done" check before work |
| Data considered stale | Circuit breaker metrics | Check observation collection errors |

**Performance baseline**: ~4µs per worker per tick, linear O(n) scaling (see benchmark tests).

## Further Reading

- [Kubernetes Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) - The control loop pattern
- [IT/OT Control Loops](https://learn.umh.app/lesson/introduction-into-it-ot-control-loop/) - Manufacturing context
- [UMH Core vs Classic](https://docs.umh.app/umh-core-vs-classic-faq) - Why we moved from Kubernetes
- [Why S6 exists](https://skarnet.org/software/s6/why.html) - Process supervision philosophy
- [The Docker Way](https://github.com/just-containers/s6-overlay?tab=readme-ov-file#the-docker-way) - Multi-process container rationale
