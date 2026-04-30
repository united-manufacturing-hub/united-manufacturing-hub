# FSMv2

Type-safe state machine framework for managing worker lifecycles with compile-time safety.

> **Implementation details**: For Go idiom patterns, code examples, and API contracts, see `doc.go`.

## Quick Start

**Start here:** The [`workers/example/helloworld/`](workers/example/helloworld/) directory contains a minimal working example with extensive comments. See its [README](workers/example/helloworld/README.md) for step-by-step instructions.

```bash
# Run the existing examples
go run pkg/fsmv2/cmd/runner/main.go --list          # List all scenarios
go run pkg/fsmv2/cmd/runner/main.go --scenario=helloworld --duration=5s  # Minimal single worker
go run pkg/fsmv2/cmd/runner/main.go --scenario=simple --duration=5s      # Parent with 2 children
```

### Creating a New Worker

```bash
# 1. Copy the template structure
workers/myworker/
├── worker.go           # Worker interface (3 methods) + init() for registration
├── userspec.go         # User configuration schema
├── dependencies.go     # External dependencies
├── snapshot/
│   └── snapshot.go     # ObservedState (embeds DesiredState with json:",inline")
├── state/
│   ├── stopped.go      # Initial state
│   └── running.go      # Target state
└── action/
    └── start.go        # I/O operation (idempotent)

# 2. CRITICAL: Name types correctly (folder "myworker" → types "MyworkerXxx")
# 3. Implement the 3 Worker methods in worker.go
# 4. Add init() function to register with factory
# 5. Test with local runner
```

**New to FSMv2?** Read [How it works](#how-it-works) below, then see [File structure](#file-structure-for-a-worker) for the full template.

---

## Modeling your worker

FSMv2 runs the same control loop as a PLC or a room thermostat: observe the process, compare to the desired state, actuate, repeat. Your worker maps to three parts of that loop.

### The control loop

- **`CollectObservedState` is the sensor.** Read the world: check if an action completed, query a service, measure a connection. Read-only I/O. Example: helloworld reads `deps.HasSaidHello()` to see if the greeting was logged (`worker.go:115`).
- **`State.Next()` is the controller.** Pure logic, no I/O. It compares observed state to desired state and decides: change state, or emit an action. Example: helloworld's `RunningState` checks shutdown, then checks the observed mood (`state/running.go:35`).
- **Actions are the actuator.** Change the world: write files, send HTTP requests, authenticate. Always idempotent. Example: helloworld's `SayHelloAction` logs a greeting and sets a flag; the communicator's `AuthenticateAction` gets a JWT token.

Sensor reads from Dependencies; actuator changes the external world and writes results back through Dependencies. The controller touches neither.

### Lifecycle phases

Each state embeds a base type that tells the supervisor which phase the control loop is in:

- `helpers.StoppedBase` — loop is off, no observation or actuation happening
- `helpers.StartingBase` — first observations arriving, actuator not yet confirmed working
- `helpers.RunningHealthyBase` — loop running, health checks passing
- `helpers.RunningDegradedBase` — loop running, but health checks failing (sensor sees a problem the actuator cannot fully fix)
- `helpers.StoppingBase` — actuator shutting down gracefully, loop winding down

See `internal/helpers/base_states.go` for the source. Example: helloworld's `RunningState` embeds `helpers.RunningHealthyBase` (`state/running.go:26`).

Your states handle process deviations: is the port open? Is the sync working? Is the token expired? The supervisor handles instrumentation failures: stale observations, collector crashes, action panics, timeouts. You don't write states for those. In control loop terms, your controller handles the process. The supervisor is the watchdog that monitors the instruments.

---

## How it works

FSMv2 is a **state machine supervisor**:

1. **You define** States (decision logic) and Actions (I/O operations)
2. **Supervisor runs the control loop** [described above](#the-control-loop): observe, compare, actuate, repeat (~4µs per worker per tick)
3. **State transitions happen** when observations change, NOT when actions complete

```text
Config Change → DeriveDesiredState() → Supervisor compares → State.Next() decides → Action executes → CollectObservedState() → Loop
```

**What you implement**: 3 Worker methods + States + Actions. **What you don't**: retries, timeouts, metrics, lifecycle management.

## Motivation

FSMv2 solves problems from UMH Classic (Kubernetes-based) and FSMv1:

| Problem | FSMv2 Solution |
|---------|----------------|
| **2-3 days** to set up FSMv1 boilerplate | ~55% less code, focused on business logic |
| 30+ second pod creation in Kubernetes | Sub-100ms configuration changes |
| "Callback hell" - 6 channels, 4 goroutines in Communicator alone | Single-threaded tick loop, no async coordination |
| AI couldn't help (would "freestyle and break things") | Clean patterns AI can understand and generate |
| Pod states don't reflect actual health | Explicit states: `running`, `degraded` (with reason), `stopped` |

FSMv2 implements the same [observe → compare → actuate](#the-control-loop) pattern as Kubernetes controllers and PLCs.

### For FSMv1 developers

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

### Worker API v2 (reduced boilerplate)

Worker API v2 reduces a minimal worker from ~662 SLOC / 7 files to ~50 SLOC / 1 file using generics. Instead of hand-writing ObservedState, DesiredState, snapshot conversion, and factory registration, you embed `WorkerBase[TConfig, TStatus]` and call `register.Worker`.

```go
// Registration (replaces init() + factory wiring + supervisor factory + CSE type registry)
func init() {
    register.Worker[MyConfig, MyStatus, register.NoDeps]("myworker", NewMyWorker)
}

// Worker struct (replaces separate dependencies, observed state, desired state files)
type MyWorker struct {
    fsmv2.WorkerBase[MyConfig, MyStatus]
}

func NewMyWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
    w := &MyWorker{}
    w.InitBase(id, logger, sr)
    return w, nil
}

// CollectObservedState — the only required method
func (w *MyWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
    cfg := fsmv2.ExtractConfig[MyConfig](desired) // typed config access
    // ... observe the world using cfg.Host, cfg.Port, etc.
    return fsmv2.NewObservation(MyStatus{Reachable: true}), nil
}
```

The framework provides: `DeriveDesiredState`, `GetInitialState`, `Config()`, `NewObservation()`, `ConvertWorkerSnapshot`, flat JSON serialization, and CSE type registry wiring. The collector handles CollectedAt, framework metrics, action history, and metric accumulation automatically. Optional capabilities (`ActionProvider`, `ChildSpecProvider`, `MetricsProvider`, `GracefulShutdowner`) are detected via interface implementation on your worker struct.

See `MIGRATION.md` for migrating existing workers from old-API to new-API.

## Worker interface

### The 3 worker methods

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

### States: pure functions with Next()

States are concrete Go types (not strings). Each state implements `Next()`, which returns a `NextResult` via `fsmv2.Transition()`:

```go
// States embed a lifecycle base and implement Next()
type StoppedState struct{ helpers.StoppedBase }
type TryingToStartState struct{ helpers.StartingBase }
type RunningState struct{ helpers.RunningHealthyBase }

// Next() returns a NextResult containing: state, signal, action, reason
func (s *TryingToStartState) Next(snapAny any) fsmv2.NextResult[any, any] {
    snap := helpers.ConvertSnapshot[ObservedState, *DesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested")
    }
    // Check observation - did the process start?
    if snap.Observed.IsRunning {
        return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "Process is running")
    }
    // Not running yet - emit action to start it.
    // Pass typed actions directly — the framework auto-wraps them via reflection.
    return fsmv2.Transition(s, fsmv2.SignalNone, &StartAction{}, "Starting process")
}
```

`fsmv2.Transition` is the recommended return shape. It is a non-generic alias for `fsmv2.Result[any, any]` that also auto-wraps typed `Action[TDeps]` values into the internal `Action[any]` envelope, so state files no longer need a caller-visible `WrapAction` adapter.

> **Deprecated — removed in PR3**
>
> The older `fsmv2.Result[any, any](...)` return and the `fsmv2.WrapAction[TDeps](&MyAction{})` wrapper are still present for in-flight migrations but will be deleted in PR3 to shrink the API surface. Do not write new code against them.

### State transitions: observation-driven

State transitions happen when observed state changes, NOT when actions complete. Actions can fail, so the pattern is: emit action → observe result → then transition.

Return EITHER a state change OR an action from `Next()`, not both. See [State XOR action rule](#state-xor-action-rule).

### State names

Every state must implement `String()` for logging and metrics. Use `helpers.DeriveStateName()` to derive the name automatically — it strips the `State` suffix and preserves casing:

```go
func (s *TryingToStartState) String() string {
    return helpers.DeriveStateName(s)  // returns "TryingToStart"
}
```

**Naming convention**: `DeriveStateName` produces PascalCase names (`RunningState` → `"Running"`, `TryingToStartState` → `"TryingToStart"`, `StoppedState` → `"Stopped"`).

Reason strings are the 4th argument to `fsmv2.Transition()` in `Next()`, not a separate method. See the `Next()` example above.

### Signals

States return a signal alongside the next state and action:

| Signal | Meaning | When to Use |
|--------|---------|-------------|
| `SignalNone` | Normal operation | Default for most transitions |
| `SignalNeedsRemoval` | Ready to be removed | After cleanup complete, parent can delete this worker |
| `SignalNeedsRestart` | Needs full restart | Unrecoverable error, request supervisor restart with backoff |

### Actions: where I/O happens

All actions must be idempotent.

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

### I/O isolation rule

| Component | I/O | Purpose |
|-----------|-----|---------|
| States | NO (pure functions) | Decide next state + action |
| Actions | WRITE (idempotent) | Change the world: HTTP, file, network I/O |
| Worker | READ (query system state) | Collect observations from dependencies and state store |

### State XOR action rule

`State.Next()` should return EITHER a state change OR an action, not both:

```go
// Correct: Action without state change
return currentState, SignalNone, &SomeAction{}

// Correct: State change without action
return NewState{}, SignalNone, nil

// Avoid: Both at once (supervisor will reject)
return NewState{}, SignalNone, &SomeAction{}  // Don't do this
```

## File structure for a worker

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

## The triangle model

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

### Transformation pipeline

User configuration is **transformed**, not copied:

```text
UserSpec (user intent)      →  "I want it warm at 8am"
        ↓
    DeriveDesiredState()    →  parses, validates, computes
        ↓
DesiredState (technical)    →  { Temperature: 22°C, Humidity: 45%, Schedule: [...] }
```

Think of it like a smart thermostat: The user enters "I want it warm" (UserSpec). The thermostat derives the ideal humidity and temperature settings (DesiredState). The control loop then makes the actual room match those settings.

`DesiredState` may differ significantly from `UserSpec`. The `DeriveDesiredState()` function can parse YAML, validate input, compute derived fields, and apply defaults.

### Snapshots

The term comes from PLC control theory: PLCs read all input states and save them into a "processing image of inputs" - a frozen point-in-time view. The PLC runs its logic based on that snapshot for deterministic behavior. FSMv2 follows the same pattern.

## Example: Communicator worker

The old Communicator had "6 channels and 4 goroutines to send messages to REST API... impossible to debug."

FSMv2 Communicator is a clean state machine:

```text
Stopped → Syncing ↔ Recovering
   ↑         ↓          ↓
   └─────────┴──────────┘
```

**State machine flow**:
1. `StoppedState`: Initial state, transitions on startup
2. `SyncingState`: Orchestrates `TransportWorker` child (which runs `PushWorker`/`PullWorker`), monitors child health
3. `RecoveringState`: Error recovery with intelligent backoff by error type

Authentication, token expiry, and transport lifecycle are handled internally by `TransportWorker` and its children (ENG-4264).

**Key design decisions**:
- I/O operations (HTTP) isolated in Actions, never in States
- Actions update shared state via Dependencies (thread-safe setters)
- CollectObservedState reads from Dependencies (thread-safe getters)

### Enabling FSMv2 Features

FSMv2 features are controlled via environment variables:

```bash
docker run -d \
  -e AUTH_TOKEN=your-auth-token \
  -e API_URL=https://management.umh.app \
  -e USE_FSMV2_TRANSPORT=true \
  -e USE_FSMV2_MEMORY_CLEANUP=true \
  -e USE_FSMV2_PROTOCOL_CONVERTER=true \
  umh-core:latest
```

| Variable | Required | Description |
|----------|----------|-------------|
| `AUTH_TOKEN` | Yes | Authentication token from Management Console |
| `API_URL` | Yes | Backend relay server URL (e.g., `https://management.umh.app`) |
| `USE_FSMV2_TRANSPORT` | No | Set to `true` to enable FSMv2 communicator |
| `USE_FSMV2_MEMORY_CLEANUP` | No | Set to `true` to enable FSMv2 memory cleanup (persistence worker) |
| `USE_FSMV2_PROTOCOL_CONVERTER` | No | Set to `true` to enable FSMv2 protocol converter |

### Disabling FSMv2 Features

To revert to the legacy behavior, set the corresponding flag to `false` or omit it (defaults to `false`):

```bash
-e USE_FSMV2_TRANSPORT=false
```

### Verifying FSMv2 Communicator

To verify that FSMv2 communicator is enabled and working:

#### Check Metrics

```bash
curl localhost:8080/metrics | grep fsmv2
```

You should see metrics like `umh_fsmv2_*` indicating the FSMv2 supervisor is running.

#### Watch Communicator Logs

```bash
cat /data/logs/umh-core/current | grep fsmv2
```

Or to follow logs in real-time:

```bash
tail -f /data/logs/umh-core/current | grep fsmv2
```

You should see log entries like `supervisor_heartbeat` with `hierarchy_path` containing `communicator` and `worker_states` showing the current state (e.g., `Syncing`).

#### Enable Debug Logging

For more detailed FSMv2 logs, set `LOGGING_LEVEL=DEBUG`:

```bash
# In Docker
docker run -e LOGGING_LEVEL=DEBUG ...

# In docker-compose.yml
services:
  umh-core:
    environment:
      - LOGGING_LEVEL=DEBUG
```

Then filter for FSMv2-specific logs:

```bash
tail -f /data/logs/umh-core/current | grep fsmv2
```

Debug output includes action completions, state changes, and observation updates:

```text
[DEBUG] action_completed - hierarchy_path=.../communicator-001(communicator), action_name=sync, duration_ms=122
[DEBUG] observed_changed - worker=.../communicator-001(communicator), changes=[{... CollectedAt modified}]
[INFO]  supervisor_heartbeat - hierarchy_path=.../communicator-001(communicator), tick=400, worker_states=map[communicator-001:Syncing]
```

## Supervisor responsibilities

Developers implement business logic; the supervisor handles:

| Feature | What You Don't Write | Default Behavior |
|---------|---------------------|------------------|
| **Observation** | Backoff on errors, stale detection | 10s stale → pause, 20s → restart collector |
| **Actions** | Worker pool, panic recovery, timeout | 10 workers, 30s timeout, auto-retry |
| **Metrics** | FrameworkMetrics auto-injected | TimeInCurrentStateMs, StateTransitionsTotal |
| **Persistence** | Delta checking (skips unchanged writes) | Automatic I/O reduction |
| **Shutdown** | `IsShutdownRequested()` propagated to all states | Graceful cleanup |
| **Templates** | Variable expansion (User/Global/Internal) | Strict mode (fails on missing) |
| **Children** | Spawn, manage, reconcile child workers | Automatic lifecycle |

### Template variables

Three namespaces, flattened for templates:
- **User**: `{{ .IP }}`, `{{ .PORT }}` (from connection config)
- **Global**: `{{ .global.api_endpoint }}` (fleet-wide settings)
- **Internal**: `{{ .internal.id }}` (runtime-only, not persisted)
- **Location**: `{{ .location_path }}` (computed from ISA-95 hierarchy)

Strict mode: missing variables fail immediately (no silent omissions).

### Child worker management

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
- `ChildrenView.AllHealthy` / `ChildrenView.AllStopped` / `ChildrenView.AllOperational` - pre-computed aggregate predicates (struct fields, not methods)
- `ChildrenView.HealthyCount` / `ChildrenView.UnhealthyCount` - pre-computed aggregate counts
- `StateReader.LoadObservedTyped()` - query child state from parent

### FrameworkMetrics (auto-injected)

Available via `snapshot.Observed.Metrics.Framework`:
- `TimeInCurrentStateMs` - how long in current state
- `StateTransitionsTotal` - total transition count
- `CollectorRestarts` - observation collector restart count
- `StartupCount` - persistent counter across restarts

### Data freshness protection

The supervisor handles stale observations:
1. Data older than 10s → FSM paused with clear logging
2. Data older than 20s → collector restarted with exponential backoff
3. Max 3 restart attempts → graceful shutdown requested
4. Circuit breaker state → observable via Prometheus

## Testing

### Local runner

Run FSM scenarios locally without deploying:

```bash
# List available scenarios
go run pkg/fsmv2/cmd/runner/main.go --list

# Run a scenario
go run pkg/fsmv2/cmd/runner/main.go --scenario=simple --duration=10s

# Debug mode with tracing
go run pkg/fsmv2/cmd/runner/main.go --scenario=communicator --log-level=debug
```

Built-in scenarios: `helloworld`, `simple`, `failing`, `panic`, `slow`, `cascade`, `timeout`, `configerror`, `inheritance`, `communicator`, `concurrent`, `persistence`

### Unit tests

```bash
# Run all FSMv2 tests
ginkgo -r ./pkg/fsmv2/

# Run with race detection
ginkgo -r -race ./pkg/fsmv2/

# Focus on specific patterns
ginkgo run --focus="State transitions" -v ./pkg/fsmv2/
```

### Architecture tests

Validate enforced patterns (lock ordering, immutability, state naming):

```bash
ginkgo run --focus="Architecture" -v ./pkg/fsmv2/
```

### Testing your worker

1. **State transitions**: Call `Next()` with test snapshots
2. **Action idempotency**: Verify actions are safe to call multiple times
3. **Shutdown handling**: Test `IsShutdownRequested()` path in all states

**Example test pattern:**

```go
It("should transition to Running when process is observed", func() {
    snap := helpers.BuildTestSnapshot(
        MyObservedState{IsRunning: true},
        &MyDesiredState{},
    )
    state := &TryingToStartState{}
    result := state.Next(snap)

    Expect(result.State).To(BeAssignableToTypeOf(&RunningState{}))
    Expect(result.Signal).To(Equal(fsmv2.SignalNone))
    Expect(result.Action).To(BeNil())
})
```

## Troubleshooting

| Symptom | Check | Resolution |
|---------|-------|------------|
| Stuck in `TryingTo*` state | Action logs for errors | Fix action failure or external dependency |
| Running but not working | Observation timestamps | Verify `CollectObservedState()` queries actual system |
| Won't shut down | `IsShutdownRequested()` check | Ensure all states check shutdown first |
| Action runs multiple times | Action idempotency | Add "already done" check before work |
| Data considered stale | Circuit breaker metrics | Check observation collection errors |

**Performance baseline**: ~4µs per worker per tick, linear O(n) scaling (see benchmark tests).

## Further reading

- [Kubernetes Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) - The control loop pattern
- [IT/OT Control Loops](https://learn.umh.app/lesson/introduction-into-it-ot-control-loop/) - Manufacturing context
- [UMH Core vs Classic](https://docs.umh.app/umh-core-vs-classic-faq) - Why we moved from Kubernetes
- [Why S6 exists](https://skarnet.org/software/s6/why.html) - Process supervision philosophy
- [The Docker Way](https://github.com/just-containers/s6-overlay?tab=readme-ov-file#the-docker-way) - Multi-process container rationale
