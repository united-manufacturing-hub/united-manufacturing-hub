# FSM v2 Worker Implementation Patterns

## Overview

This guide provides comprehensive patterns for creating FSM v2 workers in umh-core. It distills best practices from `example-parent` (parent-child relationship management) and `example-child` (resource connection management) into actionable patterns you can follow.

**Use this guide when:**
- Creating a new FSM v2 worker from scratch
- Understanding worker architecture and patterns
- Debugging existing worker implementations
- Reviewing worker code for consistency

**Reference implementations:**
- `/pkg/fsmv2/workers/example/example-parent/` - Parent worker managing children
- `/pkg/fsmv2/workers/example/example-child/` - Child worker managing connections

## Common Mistakes and How to Avoid Them

**CRITICAL: Read this section first before implementing a worker!**

These mistakes were discovered during implementation of example-parent and example-child workers. Each mistake prevented the worker from functioning at all. Future workers MUST avoid these patterns.

### 1. GetInitialState() Returns Wrong Type (Critical - Worker Won't Work)

**What went wrong:**
```go
// ❌ WRONG - Returns custom type instead of fsmv2.State
func (w *ParentWorker) GetInitialState() state.BaseParentState {
    return &state.StoppedState{}
}
```

**Correct implementation:**
```go
// ✅ CORRECT - Must return fsmv2.State interface
func (w *ParentWorker) GetInitialState() fsmv2.State {
    return state.NewStoppedState(w.deps)
}
```

**Why this matters:**
- Worker factory registration fails (can't cast to fsmv2.Worker)
- Supervisor can't integrate worker into system
- Compilation error when verifying interface compliance: `var _ fsmv2.Worker = (*YourWorker)(nil)`

**Severity:** P0 - Worker won't compile or register. Fix immediately.

### 2. States Missing Dependencies Field (Critical - States Can't Create Actions)

**What went wrong:**
```go
// ❌ WRONG - No way to create actions or access resources
type StoppedState struct{}

func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // Can't create actions - no dependencies!
    return &TryingToStartState{}, fsmv2.SignalNone, nil
}
```

**Correct implementation:**
```go
// ✅ CORRECT - Dependencies field allows action creation
type StoppedState struct {
    deps snapshot.ParentDependencies  // Interface type
}

func NewStoppedState(deps snapshot.ParentDependencies) *StoppedState {
    return &StoppedState{deps: deps}
}

func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, action.NewStartAction(s.deps)
}
```

**Why this matters:**
- States can't create actions without dependencies
- States can't access logger, config, or resources
- Transitions break when actions are needed

**Severity:** P0 - States can't function. Fix immediately.

### 3. States Don't Implement All fsmv2.State Methods (Critical - Won't Compile)

**What went wrong:**
```go
// ❌ WRONG - Only implemented Next(), missing String() and Reason()
type StoppedState struct {
    deps snapshot.ParentDependencies
}

func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, nil
}
// Missing String() and Reason() - won't compile!
```

**Correct implementation:**
```go
// ✅ CORRECT - Implements all three required methods
type StoppedState struct {
    deps snapshot.ParentDependencies
}

func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
    return "Stopped"
}

func (s *StoppedState) Reason() string {
    return "Worker is stopped"
}
```

**Why this matters:**
- Compilation fails: `state.StoppedState does not implement fsmv2.State`
- Supervisor can't log state transitions
- Tests can't verify state transitions

**Severity:** P0 - Won't compile. Fix immediately.

**Required methods:**
- `Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action)`
- `String() string` - Returns state name like "Stopped"
- `Reason() string` - Returns description like "Worker is stopped"

### 4. State Transitions Don't Use Constructors (High - Breaks at Runtime)

**What went wrong:**
```go
// ❌ WRONG - Direct struct creation loses dependencies
func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return &TryingToStartState{}, fsmv2.SignalNone, nil  // deps is nil!
}
```

**Correct implementation:**
```go
// ✅ CORRECT - Use constructor to pass dependencies
func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return NewTryingToStartState(s.deps), fsmv2.SignalNone, nil
}
```

**Why this matters:**
- Dependency chain breaks
- Next state can't create actions
- Runtime panics when accessing nil dependencies

**Severity:** P1 - Breaks at runtime. Fix before testing.

### 5. Import Cycles with Concrete Dependency Types (Critical - Won't Compile)

**What went wrong:**
```go
// ❌ WRONG - Direct import causes circular dependency
// In state/state_stopped.go:
import "pkg/fsmv2/workers/example/example-parent"

type StoppedState struct {
    deps *parent.ParentDependencies  // Concrete type
}
```

**Correct implementation:**
```go
// ✅ CORRECT - Use interface from snapshot package
// In state/state_stopped.go:
import "pkg/fsmv2/workers/example/example-parent/snapshot"

type StoppedState struct {
    deps snapshot.ParentDependencies  // Interface type
}
```

**Why this matters:**
- Compilation fails: `import cycle not allowed`
- State package imports worker package imports state package → cycle
- Must use interfaces from snapshot package to break cycle

**Severity:** P0 - Won't compile. Fix immediately.

**Dependency package pattern:**
```
worker/
├── snapshot/
│   └── snapshot.go          # Interfaces for dependencies
├── state/
│   ├── state_stopped.go     # Uses snapshot.YourDependencies interface
│   └── state_running.go
└── worker.go                # Provides concrete dependency implementation
```

## 1. Folder Structure Pattern

Every FSM v2 worker follows this standard layout:

```
pkg/fsmv2/workers/
└── your-worker/
    ├── worker.go              # Worker implementation (fsmv2.Worker interface)
    ├── worker_test.go         # Worker-level tests
    ├── dependencies.go        # Dependencies struct and interfaces
    ├── dependencies_test.go   # Dependency tests
    ├── state/                 # State machine states
    │   ├── base.go           # State interface definition
    │   ├── state_stopped.go  # Initial/stopped state
    │   ├── state_stopped_test.go
    │   ├── state_trying_to_start.go
    │   ├── state_trying_to_start_test.go
    │   ├── state_running.go  # Operational state
    │   ├── state_running_test.go
    │   ├── state_degraded.go # (optional) Degraded operational state
    │   ├── state_degraded_test.go
    │   ├── state_trying_to_stop.go
    │   └── state_trying_to_stop_test.go
    ├── action/                # State machine actions
    │   ├── action_start.go   # Actions that change the system
    │   ├── action_start_test.go
    │   ├── action_stop.go
    │   └── action_stop_test.go
    └── snapshot/              # State snapshots
        ├── snapshot.go        # ObservedState and DesiredState
        └── snapshot_test.go
```

**Purpose of each component:**

- `worker.go`: Implements fsmv2.Worker interface, orchestrates FSM
- `dependencies.go`: Injects external services (logger, config, connections)
- `state/`: State machine logic - each state determines next state + action
- `action/`: Idempotent operations (connect, start, stop, etc.)
- `snapshot/`: Immutable state snapshots for FSM decision-making

**Naming conventions:**
- Worker types: `YourWorker` (PascalCase)
- State files: `state_name.go` (snake_case)
- Action files: `action_name.go` (snake_case)
- Test files: `*_test.go` (matching the file they test)

## 2. Worker Implementation

### Worker Interface Requirements (EXACT SIGNATURES)

**Your worker MUST implement these EXACT signatures:**

```go
// REQUIRED: Return fsmv2.State, NOT your custom type
func (w *YourWorker) GetInitialState() fsmv2.State {
    return state.NewStoppedState(w.deps)  // Use constructor
}

// REQUIRED: Return ObservedState interface
func (w *YourWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return snapshot.YourObservedState{...}, nil
}

// REQUIRED: Accept interface{}, return config.DesiredState
func (w *YourWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    return config.DesiredState{...}, nil
}
```

**Common mistake:** Returning `state.BaseYourState` instead of `fsmv2.State` in GetInitialState().

**Verification:** This MUST compile:
```go
var _ fsmv2.Worker = (*YourWorker)(nil)
```

### The Worker Interface Definition

Every worker must implement `fsmv2.Worker`:

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(spec interface{}) (config.DesiredState, error)
    GetInitialState() State
}
```

### Worker Struct Pattern

```go
package yourworker

import (
    "context"
    "time"

    "go.uber.org/zap"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/yourworker/snapshot"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/yourworker/state"
)

// YourWorker implements the FSM v2 Worker interface
type YourWorker struct {
    *fsmv2.BaseWorker[*YourDependencies]
    identity fsmv2.Identity
    logger   *zap.SugaredLogger
    // Add domain-specific fields here (connections, resources, etc.)
}

// NewYourWorker creates a new worker instance
func NewYourWorker(
    id string,
    name string,
    logger *zap.SugaredLogger,
    // Add domain-specific parameters
) *YourWorker {
    dependencies := NewYourDependencies(logger /*, other deps */)

    return &YourWorker{
        BaseWorker: fsmv2.NewBaseWorker(dependencies),
        identity: fsmv2.Identity{
            ID:         id,
            Name:       name,
            WorkerType: storage.DeriveWorkerType[snapshot.YourObservedState](),
        },
        logger: logger,
    }
}
```

### Implementing CollectObservedState

Gathers current system state (non-blocking, fast):

```go
// CollectObservedState returns the current observed state
func (w *YourWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    observed := snapshot.YourObservedState{
        CollectedAt: time.Now(), // REQUIRED for freshness

        // Add domain-specific observed fields
        ConnectionStatus: w.getConnectionStatus(),
        HealthStatus:     w.getHealthStatus(),
        ErrorCount:       w.errorCount,
    }

    return observed, nil
}
```

**Key requirements:**
- MUST set `CollectedAt` to current time (freshness tracking)
- Should be fast (no blocking operations)
- Should capture actual system state (not desired state)
- Returns error only for unrecoverable issues

### Implementing DeriveDesiredState

Determines what state the worker should be in based on spec:

```go
// DeriveDesiredState determines what state the worker should be in
func (w *YourWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    // Parse spec (usually from config)
    yourSpec, ok := spec.(*YourWorkerSpec)
    if !ok {
        return fsmv2types.DesiredState{}, fmt.Errorf("invalid spec type")
    }

    desired := fsmv2types.DesiredState{
        State:         "running",
        ChildrenSpecs: nil, // or populate for parent workers
    }

    return desired, nil
}
```

### Implementing GetInitialState

Returns the state the FSM starts in:

```go
// GetInitialState returns the initial FSM state
// IMPORTANT: Must return fsmv2.State, NOT custom type
func (w *YourWorker) GetInitialState() fsmv2.State {
    return state.NewStoppedState(w.deps)  // Use constructor
}
```

**Always starts in `StoppedState`** - the FSM will transition from there.

**Critical:** Return type MUST be `fsmv2.State` (not `state.BaseYourState`). Use state constructor to pass dependencies.

### Factory Registration

Register your worker factory (usually in init function or registration file):

```go
func init() {
    fsmv2.RegisterWorkerFactory(WorkerType, func(spec interface{}) (fsmv2.Worker, error) {
        // Create and return worker instance
        return NewYourWorker(id, name, logger /* ... */), nil
    })
}
```

## 3. Dependencies Pattern

Dependencies provide actions with access to external services.

### Extending BaseDependencies

```go
package yourworker

import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "go.uber.org/zap"
)

// YourDependencies provides access to tools needed by worker actions
type YourDependencies struct {
    *fsmv2.BaseDependencies
    // Add domain-specific dependencies
    configLoader ConfigLoader
    connectionPool ConnectionPool
}

// NewYourDependencies creates new dependencies
func NewYourDependencies(
    logger *zap.SugaredLogger,
    configLoader ConfigLoader,
    connectionPool ConnectionPool,
) *YourDependencies {
    return &YourDependencies{
        BaseDependencies: fsmv2.NewBaseDependencies(logger),
        configLoader:     configLoader,
        connectionPool:   connectionPool,
    }
}

// Getter methods for each dependency
func (d *YourDependencies) GetConfigLoader() ConfigLoader {
    return d.configLoader
}

func (d *YourDependencies) GetConnectionPool() ConnectionPool {
    return d.connectionPool
}
```

### Defining Dependency Interfaces

Define interfaces for each dependency (enables mocking):

```go
// ConfigLoader loads configuration from external source
type ConfigLoader interface {
    LoadConfig() (map[string]interface{}, error)
    ValidateConfig(config map[string]interface{}) error
}

// ConnectionPool manages connections to external resources
type ConnectionPool interface {
    Acquire() (Connection, error)
    Release(Connection) error
    HealthCheck(Connection) error
}
```

### Mock Implementations for Tests

```go
// MockConfigLoader for testing
type MockConfigLoader struct {
    config map[string]interface{}
    err    error
}

func (m *MockConfigLoader) LoadConfig() (map[string]interface{}, error) {
    return m.config, m.err
}

func (m *MockConfigLoader) ValidateConfig(config map[string]interface{}) error {
    return m.err
}
```

**Examples from reference implementations:**

**example-parent** (ConfigLoader):
```go
type ConfigLoader interface {
    LoadConfig() (map[string]interface{}, error)
}
```

**example-child** (ConnectionPool):
```go
type ConnectionPool interface {
    Acquire() (Connection, error)
    Release(Connection) error
    HealthCheck(Connection) error
}
```

## 4. State Machine Design

### State Implementation Checklist

**Before implementing a state, verify:**

- [ ] State struct has `deps` field (type: interface from snapshot package, NOT concrete type)
- [ ] State has constructor: `NewXXXState(deps YourDependencies) *XXXState`
- [ ] State implements `Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action)`
- [ ] State implements `String() string` (returns state name like "Stopped")
- [ ] State implements `Reason() string` (returns description like "Worker is stopped")
- [ ] All state transitions use constructors: `return NewSomeState(s.deps), ...` (NEVER `&SomeState{}`)
- [ ] State performs type assertions for Observed and Desired state from snapshot
- [ ] Shutdown check comes FIRST in Next() method: `if snap.Desired.ShutdownRequested() { ... }`

**Common mistakes to avoid:**
- ❌ Forgetting String() or Reason() methods
- ❌ Using concrete dependency types (causes import cycles)
- ❌ Direct struct creation in transitions (`&State{}`)
- ❌ Missing shutdown check or putting it after operational logic

### State Interface Pattern

Define a base interface for all states in your worker:

```go
// state/base.go
package state

// BaseYourState defines the interface that all state implementations must satisfy
type BaseYourState interface {
    // No methods required - this is a type marker
}
```

**Why empty interface?** The `Next()` method is defined on each state struct individually, as it has state-specific signatures (different snapshot types).

### State Struct Pattern

Each state is a struct with dependencies and three required methods:

```go
// state/state_stopped.go
package state

import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/yourworker/snapshot"
)

// StoppedState represents the initial state before starting
type StoppedState struct {
    deps snapshot.YourDependencies  // REQUIRED: Interface type from snapshot package
}

// Constructor to pass dependencies
func NewStoppedState(deps snapshot.YourDependencies) *StoppedState {
    return &StoppedState{deps: deps}
}

// REQUIRED: Next() determines state transitions
func (s *StoppedState) Next(snap snapshot.YourSnapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // ALWAYS check shutdown first
    if snap.Desired.ShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }

    // Transition to next state using constructor
    return NewTryingToStartState(s.deps), fsmv2.SignalNone, nil
}

// REQUIRED: String() returns state name
func (s *StoppedState) String() string {
    return "Stopped"
}

// REQUIRED: Reason() returns description
func (s *StoppedState) Reason() string {
    return "Worker is stopped"
}
```

### Next() Method Implementation

**CRITICAL: The Next() method return signature is strictly enforced:**

```go
func (s *SomeState) Next(snap YourSnapshot) (NextState, Signal, Action)
```

**Return value mutual exclusivity rules:**

1. **State change OR action, never both**:
   ```go
   // ✅ CORRECT: State change, no action
   return &RunningState{}, fsmv2.SignalNone, nil

   // ✅ CORRECT: Action, stay in same state
   return s, fsmv2.SignalNone, action.NewStartAction(deps)

   // ❌ WRONG: State change AND action
   return &RunningState{}, fsmv2.SignalNone, action.NewStartAction(deps)
   ```

2. **Signal types**:
   - `fsmv2.SignalNone` - Normal operation
   - `fsmv2.SignalNeedsRemoval` - Worker should be removed
   - Use signals for side effects (like requesting worker removal)

3. **Shutdown check ALWAYS comes first**:
   ```go
   func (s *AnyState) Next(snap YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
       // THIS CHECK MUST BE FIRST IN EVERY STATE
       if snap.Desired.ShutdownRequested() {
           return &TryingToStopState{}, fsmv2.SignalNone, nil
       }

       // Rest of state logic...
   }
   ```

### State Transition Patterns

**Common state patterns:**

```
Stopped → TryingToStart → Running → TryingToStop → Stopped
                            ↓
                        Degraded
```

**Lifecycle states (always checked first):**
- `Stopped` - Initial state, nothing active
- `TryingToStart` - Executing startup action
- `TryingToStop` - Executing shutdown action

**Operational states:**
- `Running` - Normal operation, fully functional
- `Degraded` - Partially functional (some children unhealthy, etc.)
- `Disconnected` - Connection lost (for connection-managing workers)

**State transition logic flow:**

```go
func (s *RunningState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    // 1. ALWAYS check shutdown first (lifecycle override)
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // 2. Check for degraded conditions
    if snap.Observed.ErrorCount > threshold {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    // 3. Check for need to reconnect/restart
    if snap.Observed.ConnectionLost {
        return &TryingToStartState{}, fsmv2.SignalNone, nil
    }

    // 4. Stay in current state if all is well
    return s, fsmv2.SignalNone, nil
}
```

### State Transition Diagram

Example from example-child (connection management):

```
┌──────────┐
│ Stopped  │ (initial state)
└────┬─────┘
     │ desired: connected
     ↓
┌────────────────────┐
│ TryingToConnect    │ (action: connect)
└────┬───────────────┘
     │ action succeeds
     ↓
┌────────────────┐
│ Connected      │ (operational)
└────┬───────────┘
     │ shutdown requested
     ↓
┌────────────────────┐
│ TryingToStop       │ (action: disconnect)
└────┬───────────────┘
     │ action succeeds
     ↓
┌──────────┐
│ Stopped  │ (ready for removal)
└──────────┘
```

## 5. Action Implementation

Actions are idempotent operations that change the system state.

### Action Interface Requirements

Every action must implement:

```go
type Action interface {
    Execute(ctx context.Context) error
    Name() string
    String() string
}
```

### Action Implementation Pattern

```go
// action/start.go
package action

import (
    "context"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/yourworker/snapshot"
)

const StartActionName = "start"

// StartAction starts the worker's operations
type StartAction struct {
    dependencies snapshot.YourDependencies
}

// NewStartAction creates a new start action
func NewStartAction(deps snapshot.YourDependencies) *StartAction {
    return &StartAction{
        dependencies: deps,
    }
}

// Execute performs the start operation (MUST be idempotent)
func (a *StartAction) Execute(ctx context.Context) error {
    logger := a.dependencies.GetLogger()
    logger.Info("Starting worker")

    // Idempotent check: is it already started?
    if a.isAlreadyStarted() {
        logger.Debug("Already started, nothing to do")
        return nil
    }

    // Perform start operation
    if err := a.performStart(ctx); err != nil {
        return fmt.Errorf("failed to start: %w", err)
    }

    logger.Info("Worker started successfully")
    return nil
}

func (a *StartAction) String() string {
    return StartActionName
}

func (a *StartAction) Name() string {
    return StartActionName
}
```

### Idempotency Requirement (CRITICAL)

**Actions MUST be idempotent** - calling them multiple times should have the same effect as calling once.

**Why?** Actions can retry on failure. Non-idempotent actions cause:
- Resource leaks (creating connections without checking if they exist)
- State corruption (incrementing counters multiple times)
- Cascading failures (attempting to start already-running processes)

**How to write idempotent actions:**

```go
func (a *ConnectAction) Execute(ctx context.Context) error {
    // 1. Check current state
    if a.connection != nil && a.connection.IsAlive() {
        return nil // Already connected, idempotent
    }

    // 2. Clean up stale resources if needed
    if a.connection != nil && !a.connection.IsAlive() {
        a.connection.Close() // Clean up dead connection
        a.connection = nil
    }

    // 3. Perform operation
    conn, err := a.connectionPool.Acquire()
    if err != nil {
        return fmt.Errorf("failed to acquire connection: %w", err)
    }

    a.connection = conn
    return nil
}
```

**Idempotency checklist:**
- [ ] Check if operation already completed before starting
- [ ] Clean up partial state from previous failed attempts
- [ ] Operation can safely run multiple times
- [ ] No side effects accumulate across retries

### Context Handling

Always respect context cancellation:

```go
func (a *YourAction) Execute(ctx context.Context) error {
    // Check context before expensive operations
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Perform operation...

    // Check context again if operation is long
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    return nil
}
```

### Error Handling and Retries

Actions can return errors - the FSM will retry with exponential backoff:

```go
func (a *ConnectAction) Execute(ctx context.Context) error {
    logger := a.dependencies.GetLogger()

    // Attempt operation
    conn, err := a.connectionPool.Acquire()
    if err != nil {
        // Return error - FSM will retry
        logger.Warnf("Connection attempt failed: %v", err)
        return fmt.Errorf("failed to acquire connection: %w", err)
    }

    logger.Info("Connection established")
    return nil
}
```

**Error handling best practices:**
- Return errors for transient failures (connection timeouts, temporary resource unavailability)
- Log errors before returning them
- Wrap errors with context using `fmt.Errorf("operation failed: %w", err)`
- Don't return errors for "already done" cases (that's idempotency)

### Action Examples from Reference Implementations

**example-parent StartAction:**
```go
func (a *StartAction) Execute(ctx context.Context) error {
    logger := a.dependencies.GetLogger()
    logger.Info("Starting parent worker")
    // Load config, spawn children specs
    return nil
}
```

**example-child ConnectAction:**
```go
func (a *ConnectAction) Execute(ctx context.Context) error {
    logger := a.dependencies.GetLogger()
    logger.Info("Attempting to connect")

    // Simulate transient failures for testing
    if a.failureCount > 0 {
        a.failureCount--
        return fmt.Errorf("transient connection error")
    }

    logger.Info("Connection established successfully")
    return nil
}
```

## 6. ObservedState Pattern

ObservedState captures current system state at a point in time.

### Required Fields

```go
// snapshot/snapshot.go
package snapshot

import (
    "time"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// YourObservedState represents the current state of the worker
type YourObservedState struct {
    CollectedAt time.Time // REQUIRED: When this state was collected

    YourDesiredState // Embed desired state for comparison

    // Domain-specific observed fields
    ConnectionStatus  string
    LastError         error
    ErrorCount        int
    HealthStatus      string
}
```

### GetTimestamp() Implementation (REQUIRED)

```go
func (o YourObservedState) GetTimestamp() time.Time {
    return o.CollectedAt
}
```

**Why required?** FSM uses timestamps to detect stale state and prevent acting on outdated information.

### GetObservedDesiredState() Implementation (REQUIRED)

```go
func (o YourObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &o.YourDesiredState
}
```

**What is this?** The desired state as seen during observation - allows FSM to detect if desired state changed between observation and decision.

### Domain-Specific Fields

Add fields that capture your system's observable state:

**For connection management:**
```go
type ChildObservedState struct {
    CollectedAt time.Time

    ConnectionStatus  string  // "connected", "disconnected"
    ConnectionHealth  string  // "healthy", "degraded", "unhealthy"
    LastError         error   // Last connection error
    ConnectAttempts   int     // Number of connection attempts
}
```

**For parent-child management:**
```go
type ParentObservedState struct {
    CollectedAt time.Time

    ChildCount        int     // Total children
    ChildrenHealthy   int     // Healthy children count
    ChildrenUnhealthy int     // Unhealthy children count
}
```

### Health and Status Tracking

Best practices for health tracking:

```go
type YourObservedState struct {
    CollectedAt time.Time

    // Status fields (what state is it in?)
    ConnectionStatus string  // "connected", "connecting", "disconnected"
    OperationMode    string  // "normal", "degraded", "failed"

    // Health fields (how well is it working?)
    HealthScore      float64 // 0.0 - 1.0
    ErrorRate        float64 // Errors per second
    Latency          time.Duration

    // Error tracking
    LastError        error
    ErrorCount       int
    ConsecutiveErrors int
}
```

## 7. DesiredState Pattern

DesiredState represents the target configuration for the worker.

### Required Fields

```go
// YourDesiredState represents the target configuration
type YourDesiredState struct {
    shutdownRequested bool // REQUIRED: Shutdown flag

    Dependencies YourDependencies // REQUIRED: Dependency injection

    // Domain-specific configuration
    TargetState       string
    Configuration     map[string]interface{}
}
```

### ShutdownRequested() Implementation (REQUIRED)

```go
func (s *YourDesiredState) ShutdownRequested() bool {
    return s.shutdownRequested
}
```

**Checked in EVERY state's Next() method first** - lifecycle states override operational states.

### Domain-Specific Configuration

Add fields for your worker's configuration:

**For connection management:**
```go
type ChildDesiredState struct {
    shutdownRequested bool
    Dependencies      ChildDependencies

    TargetConnectionState string // "connected", "disconnected"
    ConnectionConfig      ConnectionConfig
}
```

**For parent-child management:**
```go
type ParentDesiredState struct {
    shutdownRequested bool
    Dependencies      ParentDependencies

    ChildCount        int
    ChildrenSpecs     []ChildSpec
}
```

### Child Specifications (Parent Workers Only)

If your worker manages children, include child specs:

```go
type ParentDesiredState struct {
    shutdownRequested bool
    Dependencies      ParentDependencies

    ChildrenSpecs     []fsmv2types.ChildSpec // Specs for child workers
}

type ChildSpec struct {
    ID         string
    Name       string
    WorkerType string
    Config     interface{}
}
```

**The FSM will automatically:**
- Spawn children based on specs
- Monitor child health
- Propagate shutdown to children
- Remove children when specs are removed

## 8. Testing Guidelines

### TDD Approach (RED-GREEN-REFACTOR)

Follow Test-Driven Development for all worker code:

1. **RED**: Write failing test showing desired behavior
2. **GREEN**: Write minimal code to pass the test
3. **REFACTOR**: Clean up code while keeping tests green

**Why TDD?** FSM logic is complex. Tests document expected behavior and catch regressions.

### Unit Test Patterns

Test each component in isolation:

**State tests:**
```go
func TestStoppedState_TransitionsToTryingToStart(t *testing.T) {
    state := &StoppedState{}

    snap := snapshot.YourSnapshot{
        Desired: snapshot.YourDesiredState{
            shutdownRequested: false,
        },
    }

    nextState, signal, action := state.Next(snap)

    // Assert next state
    if _, ok := nextState.(*TryingToStartState); !ok {
        t.Errorf("Expected TryingToStartState, got %T", nextState)
    }

    // Assert no action (state change only)
    if action != nil {
        t.Errorf("Expected no action, got %v", action)
    }

    // Assert signal
    if signal != fsmv2.SignalNone {
        t.Errorf("Expected SignalNone, got %v", signal)
    }
}
```

**Action tests:**
```go
func TestConnectAction_Idempotency(t *testing.T) {
    mockPool := &MockConnectionPool{}
    deps := NewTestDependencies(mockPool)
    action := NewConnectAction(deps)

    ctx := context.Background()

    // First execution
    err := action.Execute(ctx)
    if err != nil {
        t.Fatalf("First execution failed: %v", err)
    }

    // Second execution (should be idempotent)
    err = action.Execute(ctx)
    if err != nil {
        t.Fatalf("Second execution failed: %v", err)
    }

    // Assert connection pool called only once
    if mockPool.AcquireCallCount != 1 {
        t.Errorf("Expected 1 Acquire call, got %d", mockPool.AcquireCallCount)
    }
}
```

### Mock Implementations

Create mocks for dependencies:

```go
// MockConnectionPool for testing
type MockConnectionPool struct {
    AcquireCallCount int
    ReleaseCallCount int
    AcquireError     error
    Connection       *MockConnection
}

func (m *MockConnectionPool) Acquire() (Connection, error) {
    m.AcquireCallCount++
    if m.AcquireError != nil {
        return nil, m.AcquireError
    }
    return m.Connection, nil
}

func (m *MockConnectionPool) Release(conn Connection) error {
    m.ReleaseCallCount++
    return nil
}

func (m *MockConnectionPool) HealthCheck(conn Connection) error {
    return nil
}
```

### Test Helpers

Create helpers to reduce test boilerplate:

```go
// NewTestDependencies creates dependencies for testing
func NewTestDependencies(connectionPool ConnectionPool) *YourDependencies {
    logger := zap.NewNop().Sugar()
    return NewYourDependencies(logger, connectionPool)
}

// NewTestSnapshot creates a snapshot for testing
func NewTestSnapshot(shutdownRequested bool) snapshot.YourSnapshot {
    return snapshot.YourSnapshot{
        Observed: snapshot.YourObservedState{
            CollectedAt: time.Now(),
        },
        Desired: snapshot.YourDesiredState{
            shutdownRequested: shutdownRequested,
        },
    }
}
```

### Ginkgo/Gomega Patterns

If using Ginkgo/Gomega (umh-core standard):

```go
var _ = Describe("YourWorker", func() {
    var (
        worker *YourWorker
        ctx    context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        worker = NewYourWorker("test-id", "test-name", logger)
    })

    Context("when collecting observed state", func() {
        It("should include current timestamp", func() {
            observed, err := worker.CollectObservedState(ctx)
            Expect(err).ToNot(HaveOccurred())
            Expect(observed.GetTimestamp()).To(BeTemporally("~", time.Now(), time.Second))
        })
    })

    Context("when shutdown is requested", func() {
        It("should transition to TryingToStop from any state", func() {
            state := &RunningState{}
            snap := NewTestSnapshot(true) // shutdownRequested = true

            nextState, signal, action := state.Next(snap)

            Expect(nextState).To(BeAssignableToTypeOf(&TryingToStopState{}))
            Expect(signal).To(Equal(fsmv2.SignalNone))
            Expect(action).To(BeNil())
        })
    })
})
```

### Coverage Targets

Aim for high test coverage:

- **States**: 100% (every state's Next() method fully tested)
- **Actions**: 100% (every action path covered)
- **Worker**: 90%+ (all interface methods, key scenarios)
- **Dependencies**: 80%+ (core functionality)

**Run coverage:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Pre-Integration Compilation Checklist

**Before running integration tests, verify your worker compiles properly:**

```bash
# 1. Worker compiles
go build ./pkg/fsmv2/workers/your-worker/...

# 2. Worker implements fsmv2.Worker (this will fail if signatures are wrong)
cd pkg/fsmv2/workers/your-worker
cat > verify_interface.go << 'EOF'
package yourworker
import "pkg/fsmv2"
var _ fsmv2.Worker = (*YourWorker)(nil)
EOF
go build .
rm verify_interface.go

# 3. All states implement fsmv2.State
cd pkg/fsmv2/workers/your-worker/state
cat > verify_states.go << 'EOF'
package state
import "pkg/fsmv2"
var (
    _ fsmv2.State = (*StoppedState)(nil)
    _ fsmv2.State = (*TryingToStartState)(nil)
    _ fsmv2.State = (*RunningState)(nil)
    _ fsmv2.State = (*TryingToStopState)(nil)
)
EOF
go build .
rm verify_states.go

# 4. All actions compile
go build ./pkg/fsmv2/workers/your-worker/action/...

# 5. Unit tests pass
go test ./pkg/fsmv2/workers/your-worker/...
```

**If any of these fail, you have an API issue. Fix it before integration testing.**

**Common compilation failures:**

| Error | Cause | Fix |
|-------|-------|-----|
| `GetInitialState() has wrong signature` | Returning custom type | Return `fsmv2.State` |
| `StoppedState does not implement fsmv2.State` | Missing String() or Reason() | Add all three methods |
| `import cycle not allowed` | Using concrete dependency types | Use interfaces from snapshot package |
| `undefined: NewTryingToStartState` | Missing state constructors | Add constructor for each state |

## 9. Common Patterns

### Shutdown Handling (Critical)

**ALWAYS check shutdown first in every state:**

```go
func (s *AnyState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    // THIS MUST BE FIRST
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Rest of state logic...
}
```

**Shutdown flow:**
1. `ShutdownRequested()` returns true
2. Any state transitions to `TryingToStopState`
3. `TryingToStopState` executes stop action
4. After stop action succeeds, transition to `StoppedState`
5. `StoppedState` with shutdown returns `SignalNeedsRemoval`

### Retry Logic with Backoff

Actions automatically retry with exponential backoff when they return errors:

```go
func (a *ConnectAction) Execute(ctx context.Context) error {
    // Attempt connection
    conn, err := a.connectionPool.Acquire()
    if err != nil {
        // FSM will retry with exponential backoff
        return fmt.Errorf("connection failed: %w", err)
    }

    return nil
}
```

**Backoff behavior:**
- First retry: immediate
- Second retry: 1s delay
- Third retry: 2s delay
- Fourth retry: 4s delay
- Max delay: 60s

### Health Monitoring

Monitor health in operational states:

```go
func (s *RunningState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Check health metrics
    errorRate := float64(snap.Observed.ErrorCount) / time.Since(snap.Observed.StartTime).Seconds()
    if errorRate > 0.1 { // More than 0.1 errors/second
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    // Check consecutive errors
    if snap.Observed.ConsecutiveErrors > 5 {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    // All healthy
    return s, fsmv2.SignalNone, nil
}
```

### Parent-Child Relationships (Parent Workers)

If managing child workers, use `ChildrenSpecs`:

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    // Build child specs from configuration
    childSpecs := []fsmv2types.ChildSpec{
        {
            ID:         "child-1",
            Name:       "Worker 1",
            WorkerType: "child-worker-type",
            Config:     childConfig1,
        },
        {
            ID:         "child-2",
            Name:       "Worker 2",
            WorkerType: "child-worker-type",
            Config:     childConfig2,
        },
    }

    return fsmv2types.DesiredState{
        State:         "running",
        ChildrenSpecs: childSpecs,
    }, nil
}
```

**The FSM automatically:**
- Creates workers for new specs
- Updates workers for changed specs
- Removes workers for deleted specs
- Monitors child health
- Propagates shutdown to all children

### Resource Management (Resource-Managing Workers)

Pattern for managing external resources (connections, processes, files):

```go
type ResourceWorker struct {
    *fsmv2.BaseWorker[*ResourceDependencies]
    resource Resource // The managed resource
}

// Acquire resource in TryingToStart
func (s *TryingToStartState) Next(snap snapshot.ResourceSnapshot) (BaseResourceState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    return &RunningState{}, fsmv2.SignalNone, action.NewAcquireResourceAction(snap.Desired.Dependencies)
}

// Monitor resource in Running
func (s *RunningState) Next(snap snapshot.ResourceSnapshot) (BaseResourceState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Check resource health
    if snap.Observed.ResourceHealth == "unhealthy" {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

// Release resource in TryingToStop
func (s *TryingToStopState) Next(snap snapshot.ResourceSnapshot) (BaseResourceState, fsmv2.Signal, fsmv2.Action) {
    return &StoppedState{}, fsmv2.SignalNone, action.NewReleaseResourceAction(snap.Desired.Dependencies)
}
```

## 10. Anti-Patterns and Mistakes

### Not Checking Shutdown First

**❌ WRONG:**
```go
func (s *RunningState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    // Do operational checks first
    if snap.Observed.HealthStatus == "unhealthy" {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    // Shutdown check comes too late
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

**✅ CORRECT:**
```go
func (s *RunningState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    // ALWAYS check shutdown FIRST
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Then do operational checks
    if snap.Observed.HealthStatus == "unhealthy" {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

### Non-Idempotent Actions

**❌ WRONG:**
```go
func (a *StartAction) Execute(ctx context.Context) error {
    // Assumes this is the first execution
    a.worker.connectionCount++
    conn, err := a.connectionPool.Acquire()
    if err != nil {
        return err
    }
    a.worker.connections = append(a.worker.connections, conn)
    return nil
}
```

**✅ CORRECT:**
```go
func (a *StartAction) Execute(ctx context.Context) error {
    // Check if already started (idempotent)
    if a.worker.connection != nil && a.worker.connection.IsAlive() {
        return nil // Already started
    }

    // Clean up stale state
    if a.worker.connection != nil {
        a.worker.connection.Close()
        a.worker.connection = nil
    }

    // Perform operation
    conn, err := a.connectionPool.Acquire()
    if err != nil {
        return err
    }
    a.worker.connection = conn
    return nil
}
```

### Missing Timestamp in ObservedState

**❌ WRONG:**
```go
type YourObservedState struct {
    // Missing CollectedAt field
    ConnectionStatus string
}

func (o YourObservedState) GetTimestamp() time.Time {
    return time.Time{} // Returns zero time
}
```

**✅ CORRECT:**
```go
type YourObservedState struct {
    CollectedAt time.Time // REQUIRED
    ConnectionStatus string
}

func (o YourObservedState) GetTimestamp() time.Time {
    return o.CollectedAt
}

// In CollectObservedState:
func (w *YourWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    observed := snapshot.YourObservedState{
        CollectedAt: time.Now(), // Set to current time
        ConnectionStatus: w.getConnectionStatus(),
    }
    return observed, nil
}
```

### Returning State Change + Action Simultaneously

**❌ WRONG:**
```go
func (s *TryingToStartState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // WRONG: State change AND action
    return &RunningState{}, fsmv2.SignalNone, action.NewStartAction(snap.Desired.Dependencies)
}
```

**✅ CORRECT:**
```go
func (s *TryingToStartState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // CORRECT: Action, stay in same state
    // FSM will call Next() again after action succeeds, then transition
    return s, fsmv2.SignalNone, action.NewStartAction(snap.Desired.Dependencies)
}

// After action succeeds, FSM calls Next() again:
func (s *TryingToStartState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Check if action completed successfully
    if snap.Observed.ConnectionStatus == "connected" {
        // NOW transition to Running (no action)
        return &RunningState{}, fsmv2.SignalNone, nil
    }

    // Action not complete yet, retry
    return s, fsmv2.SignalNone, action.NewStartAction(snap.Desired.Dependencies)
}
```

### Not Implementing GetTimestamp()

**❌ WRONG:**
```go
type YourObservedState struct {
    CollectedAt time.Time
    Status      string
}

// Missing GetTimestamp() implementation
```

**✅ CORRECT:**
```go
type YourObservedState struct {
    CollectedAt time.Time
    Status      string
}

// REQUIRED method
func (o YourObservedState) GetTimestamp() time.Time {
    return o.CollectedAt
}
```

### Forgetting Factory Registration

**❌ WRONG:**
```go
// Worker defined but never registered
type YourWorker struct {
    *fsmv2.BaseWorker[*YourDependencies]
}

// No factory registration - FSM can't create instances
```

**✅ CORRECT:**
```go
type YourWorker struct {
    *fsmv2.BaseWorker[*YourDependencies]
    identity fsmv2.Identity
}

// Worker type is automatically derived from state types via storage.DeriveWorkerType
// No need for WorkerType constant - it's computed from YourObservedState type name

func NewYourWorker(id string, name string, logger *zap.SugaredLogger) *YourWorker {
    return &YourWorker{
        BaseWorker: fsmv2.NewBaseWorker(dependencies),
        identity: fsmv2.Identity{
            ID:         id,
            Name:       name,
            WorkerType: storage.DeriveWorkerType[snapshot.YourObservedState](),
        },
    }
}
```

## 11. Quick Reference

### Checklist for Creating a New Worker

Use this checklist to ensure you implement all required components:

**Setup:**
- [ ] Create worker directory: `pkg/fsmv2/workers/yourworker/`
- [ ] Create subdirectories: `state/`, `action/`, `snapshot/`

**Worker Implementation:**
- [ ] Create `worker.go` with worker struct
- [ ] Implement `CollectObservedState()` - returns current state
- [ ] Implement `DeriveDesiredState()` - determines target state
- [ ] Implement `GetInitialState()` - returns `&state.StoppedState{}`
- [ ] Create `NewYourWorker()` constructor

**Dependencies:**
- [ ] Create `dependencies.go` extending `fsmv2.BaseDependencies`
- [ ] Define dependency interfaces (ConfigLoader, ConnectionPool, etc.)
- [ ] Implement getter methods for each dependency
- [ ] Create `NewYourDependencies()` constructor
- [ ] Create mock implementations for testing

**State Machine:**
- [ ] Create `state/base.go` with base state interface
- [ ] Implement `StoppedState` - initial state
- [ ] Implement `TryingToStartState` - executes start action
- [ ] Implement `RunningState` - operational state
- [ ] Implement `DegradedState` (optional) - degraded operation
- [ ] Implement `TryingToStopState` - executes stop action
- [ ] Each state has `Next()` method checking shutdown FIRST
- [ ] Each state has `String()` and `Reason()` methods

**Actions:**
- [ ] Create start action in `action/start.go`
- [ ] Create stop action in `action/stop.go`
- [ ] Add domain-specific actions as needed
- [ ] Each action implements `Execute()`, `Name()`, `String()`
- [ ] All actions are idempotent
- [ ] All actions handle context cancellation

**Snapshots:**
- [ ] Create `snapshot/snapshot.go`
- [ ] Define `YourObservedState` with `CollectedAt` field
- [ ] Define `YourDesiredState` with `shutdownRequested` field
- [ ] Implement `GetTimestamp()` on ObservedState
- [ ] Implement `GetObservedDesiredState()` on ObservedState
- [ ] Implement `ShutdownRequested()` on DesiredState

**Testing:**
- [ ] Create test files for all components (`*_test.go`)
- [ ] Test all state transitions
- [ ] Test action idempotency
- [ ] Test shutdown handling in all states
- [ ] Test error cases and retries
- [ ] Achieve 90%+ coverage

**Registration:**
- [ ] Register worker factory with `fsmv2.RegisterWorkerFactory()`

**Validation:**
- [ ] Run tests: `go test ./...`
- [ ] Check coverage: `go test -cover ./...`
- [ ] Run linter: `golangci-lint run`
- [ ] Verify no focused tests: `ginkgo -r --fail-on-focused`

### Common Code Snippets

**Worker constructor:**
```go
func NewYourWorker(id, name string, logger *zap.SugaredLogger) *YourWorker {
    dependencies := NewYourDependencies(logger)
    return &YourWorker{
        BaseWorker: fsmv2.NewBaseWorker(dependencies),
        identity:   fsmv2.Identity{
            ID:         id,
            Name:       name,
            WorkerType: storage.DeriveWorkerType[snapshot.YourObservedState](),
        },
        logger:     logger,
    }
}
```

**State Next() template:**
```go
func (s *YourState) Next(snap snapshot.YourSnapshot) (BaseYourState, fsmv2.Signal, fsmv2.Action) {
    // ALWAYS check shutdown first
    if snap.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // State-specific logic here

    return s, fsmv2.SignalNone, nil
}
```

**Action Execute() template:**
```go
func (a *YourAction) Execute(ctx context.Context) error {
    logger := a.dependencies.GetLogger()
    logger.Info("Executing action")

    // Check if already done (idempotency)
    if a.isAlreadyDone() {
        return nil
    }

    // Perform operation
    if err := a.performOperation(ctx); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }

    return nil
}
```

**Test helper:**
```go
func NewTestSnapshot(shutdownRequested bool) snapshot.YourSnapshot {
    return snapshot.YourSnapshot{
        Observed: snapshot.YourObservedState{
            CollectedAt: time.Now(),
        },
        Desired: snapshot.YourDesiredState{
            shutdownRequested: shutdownRequested,
        },
    }
}
```

### File Template Locations

Reference these files for templates:

**Worker implementation:**
- `/pkg/fsmv2/workers/example/example-parent/worker.go` (parent pattern)
- `/pkg/fsmv2/workers/example/example-child/worker.go` (child pattern)

**Dependencies:**
- `/pkg/fsmv2/workers/example/example-parent/dependencies.go` (ConfigLoader)
- `/pkg/fsmv2/workers/example/example-child/dependencies.go` (ConnectionPool)

**State machine:**
- `/pkg/fsmv2/workers/example/example-parent/state/` (parent states)
- `/pkg/fsmv2/workers/example/example-child/state/` (child states)

**Actions:**
- `/pkg/fsmv2/workers/example/example-parent/action/start.go`
- `/pkg/fsmv2/workers/example/example-child/action/connect.go`

**Snapshots:**
- `/pkg/fsmv2/workers/example/example-parent/snapshot/snapshot.go`
- `/pkg/fsmv2/workers/example/example-child/snapshot/snapshot.go`

## Summary

Creating an FSM v2 worker requires:

1. **Worker struct** implementing `fsmv2.Worker` interface
2. **Dependencies** extending `fsmv2.BaseDependencies` with domain-specific interfaces
3. **State machine** with states implementing `Next()` method (shutdown first!)
4. **Actions** that are idempotent and handle context
5. **Snapshots** with ObservedState (timestamp required) and DesiredState (shutdown flag required)
6. **Tests** using TDD approach with high coverage
7. **Factory registration** so FSM can create instances

**Key principles:**
- Shutdown check FIRST in every state
- Actions are idempotent
- State change OR action, never both
- ObservedState must have timestamp
- DesiredState must have shutdown flag
- Test everything (TDD approach)

**Reference implementations:**
- `example-parent`: Parent-child relationship management
- `example-child`: Resource connection management

Follow these patterns and your worker will integrate seamlessly with the FSM v2 framework.
