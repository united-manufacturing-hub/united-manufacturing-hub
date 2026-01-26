# FSMv2

Type-safe state machine framework for managing worker lifecycles with compile-time safety.

> **Implementation details**: For Go idiom patterns, code examples, and API contracts, see `doc.go`.

## Quick Start

**Start here:** The [`workers/example/helloworld/`](workers/example/helloworld/) directory contains a minimal working example with extensive comments. See its [README](workers/example/helloworld/README.md) for step-by-step instructions.

```bash
# Run the existing examples
go run pkg/fsmv2/cmd/runner/main.go --list          # List all scenarios
go run pkg/fsmv2/cmd/runner/main.go --scenario=simple --duration=5s
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

## How it works

FSMv2 is a **state machine supervisor**:

1. **You define** States (decision logic) and Actions (I/O operations)
2. **Supervisor runs a loop**: Observe → Compare → Decide → Act → Repeat (~4µs per worker per tick)
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

FSMv2 implements the same control loop pattern as Kubernetes controllers and PLCs:
Desired state → Compare → Actuate → Observe → Repeat.

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

### State transitions: observation-driven

State transitions happen when observed state changes, NOT when actions complete. Actions can fail, so the pattern is: emit action → observe result → then transition.

Return EITHER a state change OR an action from `Next()`, not both. See [State XOR action rule](#state-xor-action-rule).

### State names and reasons (required methods)

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

| Component | Can Do I/O | Purpose |
|-----------|------------|---------|
| States | NO (pure functions) | Decide next state + action |
| Actions | YES (idempotent) | Perform HTTP, file, network I/O |
| Worker | NO (read-only) | Collect observations |

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
Stopped → TryingToAuthenticate → Syncing ↔ Degraded
   ↑              ↓                 ↓         ↓
   └──────────────┴─────────────────┴─────────┘
              (Token expiry or auth loss)
```

**State machine flow**:
1. `StoppedState`: Initial state, transitions on startup
2. `TryingToAuthenticateState`: Emits `AuthenticateAction` (HTTP POST) with exponential backoff
3. `SyncingState`: Emits `SyncAction` continuously (HTTP pull/push loop), checks token expiry
4. `DegradedState`: Error recovery with intelligent backoff by error type

**Token expiry handling**: `SyncingState` checks `IsTokenExpired()` and transitions back to `TryingToAuthenticateState` to refresh JWT before continuing.

**Consecutive error handling**: Tracks consecutive errors per operation type. When threshold is exceeded, triggers automatic transport reset for recovery from transient network failures.

**Key design decisions**:
- I/O operations (HTTP) isolated in Actions, never in States
- Actions update shared state via Dependencies (thread-safe setters)
- CollectObservedState reads from Dependencies (thread-safe getters)

### Enabling FSMv2 Communicator

#### Option 1: Environment Variables

```bash
docker run -d \
  -e AUTH_TOKEN=your-auth-token \
  -e API_URL=https://management.umh.app \
  -e USE_FSMV2_TRANSPORT=true \
  umh-core:latest
```

| Variable | Required | Description |
|----------|----------|-------------|
| `AUTH_TOKEN` | Yes | Authentication token from Management Console |
| `API_URL` | Yes | Backend relay server URL (e.g., `https://management.umh.app`) |
| `USE_FSMV2_TRANSPORT` | Yes | Set to `true` to enable FSMv2 communicator |

#### Option 2: config.yaml

```yaml
agent:
  communicator:
    apiUrl: "https://management.umh.app"
    authToken: "your-auth-token"
    useFSMv2Transport: true
```

### Disabling FSMv2 Communicator

To revert to the legacy communicator, explicitly set:

```bash
-e USE_FSMV2_TRANSPORT=false
```

Or in config.yaml:

```yaml
agent:
  communicator:
    useFSMv2Transport: false
```

**Note:** If `useFSMv2Transport` was previously enabled and saved to config.yaml, you must explicitly set it to `false` - simply omitting the environment variable will not override the persisted config value.

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
- `ChildrenView.AllHealthy()` / `AllStopped()` - aggregate checks
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

Built-in scenarios: `simple`, `failing`, `panic`, `cascade`, `timeout`, `communicator`

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
