# Migrating from FSMv1 to FSMv2

This guide helps developers migrate existing FSMv1 workers to FSMv2. If you are familiar with FSMv1's looplab FSM, callbacks, channels, and goroutines, this document will help you understand the new patterns and how to convert your code.

## Who This Guide Is For

- Developers who have implemented FSMv1 workers using the looplab FSM library
- Developers maintaining FSMv1 code that needs to be migrated to FSMv2
- Anyone familiar with FSMv1 patterns who wants to understand FSMv2 equivalents

## Overview of Key Changes

| FSMv1 | FSMv2 |
|-------|-------|
| looplab FSM library with callbacks | Struct-based states with `Next()` method |
| State as string constants | State as Go types (compile-time safety) |
| 8 files per FSM (machine.go, actions.go, reconcile.go, ...) | 3 Worker methods + states + actions |
| Channels and goroutines for coordination | Single-threaded tick loop, no async coordination |
| Business logic mixed with boilerplate | Worker has business logic, Supervisor has boilerplate |
| `fsm_callbacks.go` (fail-free) | States are pure functions (no I/O) |
| `manager.go` lifecycle handling | Supervisor handles automatically |
| Self-managed lifecycle | Supervisor-owned lifecycle |

## Conceptual Changes

### FSMv1: looplab FSM with callbacks

In FSMv1, state machines were built using the looplab FSM library:

- **States as strings**: `StateRunning`, `StateStopped`, `StateStarting`
- **Transitions via callbacks**: `before_running`, `after_starting`, `enter_stopped`
- **Complex coordination**: Channels for communication, goroutines for async operations
- **Self-managed lifecycle**: Workers managed their own startup, shutdown, and error handling
- **Mixed concerns**: Business logic, retry logic, and boilerplate were interleaved

A typical FSMv1 implementation had these files:
```text
machine.go       # FSM definition with state constants and transitions
fsm_callbacks.go # Callback implementations (fail-free, logging only)
actions.go       # Idempotent operations (can fail and retry)
reconcile.go     # Single-threaded control loop
manager.go       # Lifecycle management
models.go        # Data structures
```

### FSMv2: Struct-based states with single-threaded tick loop

FSMv2 fundamentally changes the approach:

- **States as Go types**: `StoppedState{}`, `RunningState{}`, `TryingToConnectState{}`
- **Decision via `Next()` method**: Each state returns (nextState, signal, action)
- **Single-threaded tick loop**: No channels, no goroutines for coordination
- **Supervisor-owned lifecycle**: The supervisor handles retries, timeouts, metrics, and shutdown
- **Separation of concerns**: States are pure (no I/O), Actions perform I/O, Worker collects observations

A typical FSMv2 implementation has these files:
```text
workers/myworker/
├── worker.go           # Worker interface (3 methods)
├── userspec.go         # User configuration schema
├── dependencies.go     # External dependencies (HTTP client, etc.)
├── snapshot/
│   ├── observed.go     # What system actually is
│   └── desired.go      # What user wants
├── state/
│   ├── stopped.go      # Initial state
│   ├── trying_to_start.go
│   └── running.go
└── action/
    ├── start.go        # I/O operation (idempotent)
    └── stop.go
```

### Key Mindset Shift

**FSMv1**: You write everything - lifecycle, retries, error handling, metrics.

**FSMv2**: You write business logic (states, actions). The supervisor handles everything else.

## Step-by-step Migration Checklist

### 1. Map FSMv1 files to FSMv2 structure

| FSMv1 File | FSMv2 Equivalent |
|------------|------------------|
| `machine.go` | `state/*.go` (one file per state) |
| `fsm_callbacks.go` | Logic moves to `State.Next()` methods |
| `actions.go` | `action/*.go` (one file per action) |
| `reconcile.go` | Handled by supervisor (delete) |
| `manager.go` | Handled by supervisor (delete) |
| `models.go` | `snapshot/observed.go`, `snapshot/desired.go` |

### 2. Convert state strings to state structs

**FSMv1:**
```go
const (
    StateRunning  = "running"
    StateStopped  = "stopped"
    StateStarting = "starting"
)

fsm := looplab.NewFSM(
    StateStopped,
    looplab.Events{
        {Name: "start", Src: []string{StateStopped}, Dst: StateStarting},
        {Name: "started", Src: []string{StateStarting}, Dst: StateRunning},
        {Name: "stop", Src: []string{StateRunning}, Dst: StateStopped},
    },
    looplab.Callbacks{...},
)
```

**FSMv2:**
```go
// state/stopped.go
type StoppedState struct{}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }

    // Transition to TryingToStart when desired state is running
    if snap.Observed.ShouldBeRunning() {
        return &TryingToStartState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string { return "stopped" }
func (s *StoppedState) Reason() string { return "Worker is stopped" }
```

### 3. Convert callbacks to State.Next() methods

**FSMv1 callback:**
```go
func (m *MyFSM) beforeStarting(e *fsm.Event) {
    // This runs before entering "starting" state
    if !m.isConfigValid() {
        e.Cancel()
        return
    }
    m.startProcess()
}

func (m *MyFSM) enterRunning(e *fsm.Event) {
    // This runs when entering "running" state
    m.logger.Info("Worker is now running")
}
```

**FSMv2 Next():**
```go
func (s *TryingToStartState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Check shutdown first
    if snap.Observed.IsStopRequired() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Transition based on observation, not action completion
    if snap.Observed.IsProcessRunning {
        return &RunningState{}, fsmv2.SignalNone, nil
    }

    // Emit action - stay in same state until observation changes
    return s, fsmv2.SignalNone, &StartProcessAction{}
}

func (s *TryingToStartState) String() string { return "trying_to_start" }
func (s *TryingToStartState) Reason() string { return "Starting process" }
```

### 4. Convert actions to Action interface implementations

**FSMv1 action (in actions.go):**
```go
func (m *MyFSM) startProcess(ctx context.Context) error {
    if m.process.IsRunning() {
        return nil // Idempotent
    }

    cmd := exec.CommandContext(ctx, "myprocess")
    return cmd.Start()
}
```

**FSMv2 action:**
```go
// action/start_process.go
type StartProcessAction struct{}

func (a *StartProcessAction) Execute(ctx context.Context, depsAny any) error {
    // Check cancellation first
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    deps := depsAny.(MyDependencies)

    // Check if already done (idempotent)
    if deps.ProcessManager().IsRunning() {
        return nil
    }

    return deps.ProcessManager().Start(ctx)
}

func (a *StartProcessAction) Name() string { return "start_process" }
```

### 5. Remove channel-based coordination

**FSMv1 with channels:**
```go
type MyFSM struct {
    eventCh    chan Event
    resultCh   chan Result
    shutdownCh chan struct{}
    wg         sync.WaitGroup
}

func (m *MyFSM) Run() {
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        for {
            select {
            case event := <-m.eventCh:
                m.handleEvent(event)
            case <-m.shutdownCh:
                return
            }
        }
    }()
}
```

**FSMv2 (no channels needed):**
```go
// The supervisor runs the tick loop automatically.
// State.Next() is called on each tick with a fresh snapshot.
// No channels, no goroutines for coordination.

type MyWorker struct {
    *helpers.BaseWorker[*MyDependencies]
    logger *zap.SugaredLogger
}

func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Runs in background goroutine managed by supervisor
    return &MyObservedState{
        CollectedAt:      time.Now(),
        IsProcessRunning: w.deps.ProcessManager().IsRunning(),
    }, nil
}

func (w *MyWorker) GetInitialState() fsmv2.State[any, any] {
    return &StoppedState{}
}
```

### 6. Implement the three Worker methods

Every FSMv2 worker must implement:

```go
type Worker interface {
    // Query actual system state (runs periodically in a background goroutine)
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // Transform user config to desired state (called on config change)
    DeriveDesiredState(spec interface{}) (DesiredState, error)

    // Return the initial state for new workers (called once at startup)
    GetInitialState() State
}
```

## Code Examples

### FSMv1 state definition to FSMv2 state struct

**FSMv1:**
```go
// machine.go
const (
    StateStopped  = "stopped"
    StateStarting = "starting"
    StateRunning  = "running"
)

// Transitions defined in FSM constructor
events := fsm.Events{
    {Name: "start", Src: []string{StateStopped}, Dst: StateStarting},
    {Name: "started", Src: []string{StateStarting}, Dst: StateRunning},
    {Name: "stop", Src: []string{StateRunning, StateStarting}, Dst: StateStopped},
}
```

**FSMv2:**
```go
// state/stopped.go
type StoppedState struct{}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    if snap.Desired.IsShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }

    if snap.Observed.ShouldBeRunning() {
        return &TryingToStartState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string { return "stopped" }
func (s *StoppedState) Reason() string { return "Worker is stopped" }
```

### FSMv1 callback to FSMv2 Next() method

**FSMv1:**
```go
// fsm_callbacks.go
func (m *MyFSM) beforeRunning(e *fsm.Event) {
    // Pre-condition check
    if !m.isHealthy() {
        e.Cancel()
        m.logger.Warn("Cannot enter running: unhealthy")
        return
    }
}

func (m *MyFSM) enterRunning(e *fsm.Event) {
    m.logger.Info("Entered running state")
    m.metrics.RecordStateChange("running")
}

func (m *MyFSM) leaveRunning(e *fsm.Event) {
    m.logger.Info("Leaving running state")
}
```

**FSMv2:**
```go
// state/running.go
type RunningState struct{}

func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Check shutdown first (equivalent to leave callback)
    if snap.Observed.IsStopRequired() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Health check (equivalent to before callback condition)
    if !snap.Observed.IsHealthy {
        return &DegradedState{Reason: "health check failed"}, fsmv2.SignalNone, nil
    }

    // Stay running
    return s, fsmv2.SignalNone, nil
}

func (s *RunningState) String() string { return "running" }
func (s *RunningState) Reason() string { return "Worker is healthy and operational" }
```

### FSMv1 action to FSMv2 Action implementation

**FSMv1:**
```go
// actions.go
func (m *MyFSM) connectToServer(ctx context.Context) error {
    // Check if already connected
    if m.client.IsConnected() {
        return nil
    }

    // Attempt connection
    client, err := NewClient(m.config.ServerURL)
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }

    if err := client.Connect(ctx); err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }

    m.client = client
    return nil
}
```

**FSMv2:**
```go
// action/connect.go
type ConnectAction struct{}

func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
    // Check cancellation first
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    deps := depsAny.(MyDependencies)
    logger := deps.ActionLogger("connect")

    // Idempotency check - already connected?
    if deps.Client().IsConnected() {
        logger.Debug("Already connected, skipping")
        return nil
    }

    logger.Info("Attempting to connect")

    // Perform the connection
    if err := deps.Client().Connect(ctx); err != nil {
        return fmt.Errorf("connection failed: %w", err)
    }

    // Update shared state via dependencies (thread-safe)
    deps.SetConnected(true)
    logger.Info("Connection established")

    return nil
}

func (a *ConnectAction) Name() string { return "connect" }
```

## Common Patterns

### Error handling: from callbacks to signal returns

**FSMv1:**
```go
func (m *MyFSM) handleError(err error) {
    m.errorCount++
    if m.errorCount > maxRetries {
        m.fsm.Event("fail_permanently")
    } else {
        m.fsm.Event("retry")
    }
}
```

**FSMv2:**
```go
func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Check consecutive failures from observation
    if snap.Observed.ConsecutiveFailures > 100 {
        // Signal unrecoverable error - supervisor will restart
        return s, fsmv2.SignalNeedsRestart, nil
    }

    // Normal retry - emit action again
    return s, fsmv2.SignalNone, &ConnectAction{}
}
```

### Retry logic: from manual to supervisor-managed

**FSMv1:**
```go
func (m *MyFSM) retryWithBackoff(ctx context.Context) {
    backoff := time.Second
    for {
        if err := m.doOperation(); err == nil {
            return
        }

        select {
        case <-time.After(backoff):
            backoff = min(backoff*2, time.Minute)
        case <-ctx.Done():
            return
        }
    }
}
```

**FSMv2:**
```go
// Retries happen automatically via tick loop.
// Action fails -> state unchanged -> next tick -> state.Next() called again
// The supervisor manages the tick interval.

func (s *TryingToSyncState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Check if sync completed
    if snap.Observed.IsSynced {
        return &SyncedState{}, fsmv2.SignalNone, nil
    }

    // Emit action - supervisor handles retry on failure
    return s, fsmv2.SignalNone, &SyncAction{}
}
```

### Health checks: from external to ObservedState

**FSMv1:**
```go
func (m *MyFSM) healthCheck() bool {
    return m.client.Ping() == nil && m.lastActivity.After(time.Now().Add(-timeout))
}

// Called externally or in reconcile loop
if !m.healthCheck() {
    m.fsm.Event("degrade")
}
```

**FSMv2:**
```go
// Health is part of ObservedState, collected automatically
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    deps := w.GetDependencies()

    return &MyObservedState{
        CollectedAt:      time.Now(),
        IsHealthy:        deps.Client().Ping() == nil,
        LastActivityTime: deps.GetLastActivityTime(),
    }, nil
}

// State checks observation for health
func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Health check via observation
    if !snap.Observed.IsHealthy {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

### Child management: from manual to ChildSpec

**FSMv1:**
```go
func (m *ParentFSM) createChildren() {
    for _, childConfig := range m.config.Children {
        child := NewChildFSM(childConfig)
        m.children[childConfig.Name] = child
        go child.Run()
    }
}

func (m *ParentFSM) shutdownChildren() {
    for _, child := range m.children {
        child.Shutdown()
    }
    m.wg.Wait()
}
```

**FSMv2:**
```go
// Parent declares children in DeriveDesiredState - supervisor manages lifecycle
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    userSpec := spec.(config.UserSpec)

    var children []config.ChildSpec
    for _, childConfig := range userSpec.Children {
        children = append(children, config.ChildSpec{
            Name:       childConfig.Name,
            WorkerType: "mychild",
            UserSpec:   childConfig,
        })
    }

    return &config.DesiredState{
        BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
        ChildrenSpecs:    children,
    }, nil
}

// Parent state can check children via ChildrenView
func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[ParentObservedState, *ParentDesiredState](snapAny)

    // Check if all children are healthy
    if snap.Observed.ChildrenUnhealthy > 0 {
        return &DegradedState{Reason: "unhealthy children"}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

## Gotchas / Breaking Changes

### No more channels (single-threaded model)

**FSMv1 pattern (no longer valid):**
```go
// Don't do this in FSMv2
type MyWorker struct {
    eventCh chan Event  // NO - no channels needed
    doneCh  chan struct{} // NO - supervisor handles shutdown
}
```

**FSMv2 approach:**
The supervisor runs a single-threaded tick loop. State.Next() is called on each tick. There is no need for channels to coordinate state changes.

### Actions must be idempotent

Every action must be safe to call multiple times. The supervisor may retry actions that fail.

**Wrong:**
```go
func (a *CreateResourceAction) Execute(ctx context.Context, deps any) error {
    // This will fail on retry if resource already exists
    return deps.CreateResource()
}
```

**Correct:**
```go
func (a *CreateResourceAction) Execute(ctx context.Context, deps any) error {
    d := deps.(MyDeps)

    // Check if already created
    if d.ResourceExists() {
        return nil  // Idempotent - already done
    }

    return d.CreateResource()
}
```

### States are pure (no I/O)

State.Next() must not perform I/O operations. It should only make decisions based on the snapshot.

**Wrong:**
```go
func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    // Don't do I/O in Next()
    conn, err := net.Dial("tcp", "server:9000")  // NO!
    if err == nil {
        return &ConnectedState{}, fsmv2.SignalNone, nil
    }
    return s, fsmv2.SignalNone, nil
}
```

**Correct:**
```go
func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // Check observation (collected by CollectObservedState)
    if snap.Observed.IsConnected {
        return &ConnectedState{}, fsmv2.SignalNone, nil
    }

    // Emit action - I/O happens there
    return s, fsmv2.SignalNone, &ConnectAction{}
}
```

### Observation must include timestamps

ObservedState must include a timestamp for staleness detection. The supervisor uses this to detect when observations are too old.

**Required:**
```go
type MyObservedState struct {
    CollectedAt time.Time  // REQUIRED for staleness detection
    // ... other fields
}

func (o MyObservedState) GetTimestamp() time.Time {
    return o.CollectedAt
}
```

### State XOR Action rule

Return EITHER a state change OR an action from Next(), not both. The supervisor will reject attempts to do both.

**Wrong:**
```go
// Don't return both state change AND action
return &NewState{}, fsmv2.SignalNone, &SomeAction{}
```

**Correct:**
```go
// State change without action
return &NewState{}, fsmv2.SignalNone, nil

// OR action without state change
return s, fsmv2.SignalNone, &SomeAction{}
```

### Check shutdown first in every state

Every state's Next() method should check IsShutdownRequested() as the first condition.

**Required pattern:**
```go
func (s *AnyState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // ALWAYS check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &StoppedState{}, fsmv2.SignalNeedsRemoval, nil
        // Or transition to TryingToStop if cleanup needed
    }

    // ... rest of state logic
}
```

### Dependencies must be thread-safe

Actions execute asynchronously in a worker pool. Any shared state modified by actions must use thread-safe access (mutexes, atomics).

```go
type MyDependencies struct {
    mu        sync.RWMutex
    connected bool
}

func (d *MyDependencies) IsConnected() bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    return d.connected
}

func (d *MyDependencies) SetConnected(v bool) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.connected = v
}
```

## Further Reading

- [README.md](README.md) - Overview of FSMv2 and the triangle model
- [doc.go](doc.go) - API contracts, patterns, and best practices
- [workers/example/examplechild/](workers/example/examplechild/) - Complete example implementation
- [DEPENDENCIES.md](DEPENDENCIES.md) - Dependency injection patterns
