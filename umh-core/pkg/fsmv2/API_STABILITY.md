# FSMv2 API Stability Guide

TODO: is this required?

Migration reference for FSMv1 to FSMv2. Stability tiers indicate API commitment level.

**Stability Tiers:**
- **STABLE**: Backward-compatible. Breaking changes only in major versions.
- **EXPERIMENTAL**: May change with notice. Feedback welcomed.
- **INTERNAL**: Not part of public API. May change without notice.

---

## Core Interfaces (STABLE)

### Worker
Main interface developers implement. Location: `pkg/fsmv2/api.go`

```go
type Worker interface {
    // CollectObservedState monitors actual system state (called in goroutine with timeout)
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // DeriveDesiredState derives target state from user config (pure function)
    DeriveDesiredState(spec interface{}) (DesiredState, error)

    // GetInitialState returns starting state (called once during creation)
    GetInitialState() State[any, any]
}
```

### State
FSM state interface. Location: `pkg/fsmv2/api.go`

```go
type State[TSnapshot any, TDeps any] interface {
    // Next evaluates snapshot, returns next state, signal, and optional action
    Next(snapshot TSnapshot) (State[TSnapshot, TDeps], Signal, Action[TDeps])

    // String returns state name for logging
    String() string

    // Reason returns human-readable explanation of current state
    Reason() string
}
```

### Action
Idempotent side effects. Location: `pkg/fsmv2/api.go`

```go
type Action[TDeps any] interface {
    // Execute performs the action (must handle context cancellation)
    Execute(ctx context.Context, deps TDeps) error

    // Name returns descriptive name for logging
    Name() string
}
```

### ObservedState
Actual system state. Location: `pkg/fsmv2/api.go`

```go
type ObservedState interface {
    // GetObservedDesiredState returns deployed desired state for verification
    GetObservedDesiredState() DesiredState

    // GetTimestamp returns collection time for staleness checks
    GetTimestamp() time.Time
}
```

### DesiredState
Target system state. Location: `pkg/fsmv2/api.go`

```go
type DesiredState interface {
    // IsShutdownRequested checks if supervisor requested shutdown
    IsShutdownRequested() bool

    // GetState returns desired lifecycle state ("running", "stopped")
    GetState() string
}
```

### Dependencies
Injected tools for actions. Location: `pkg/fsmv2/dependencies.go`

```go
type Dependencies interface {
    // GetLogger returns structured logger
    GetLogger() *zap.SugaredLogger

    // ActionLogger returns logger enriched with action context
    ActionLogger(actionType string) *zap.SugaredLogger

    // GetStateReader returns read-only store access (may be nil)
    GetStateReader() StateReader
}
```

---

## Configuration Types (STABLE)

Location: `pkg/fsmv2/config/`

### UserSpec
Raw user configuration before templating.

```go
type UserSpec struct {
    Config    string         `json:"config" yaml:"config"`
    Variables VariableBundle `json:"variables" yaml:"variables"`
}
```

### VariableBundle
Three-tier namespace for template variables.

```go
type VariableBundle struct {
    User     map[string]any  // Top-level in templates: {{ .varname }}
    Global   map[string]any  // Nested: {{ .global.varname }}
    Internal map[string]any  // Runtime only (not serialized): {{ .internal.varname }}
}
```

### ChildSpec
Declarative child worker specification.

```go
type ChildSpec struct {
    Name             string         // Unique name within parent scope
    WorkerType       string         // Registered factory key
    UserSpec         UserSpec       // Raw config for DeriveDesiredState
    ChildStartStates []string       // Parent states where child runs (empty = always)
    Dependencies     map[string]any // Additional deps merged with parent
}
```

### BaseDesiredState (for embedding)
Provides shutdown handling. Embed in your DesiredState types.

```go
type BaseDesiredState struct {
    ShutdownRequested bool   `json:"ShutdownRequested"`
    State             string `json:"state"`
}
```

### BaseUserSpec (for embedding)
Provides state field with default. Embed in your UserSpec types.

```go
type BaseUserSpec struct {
    State string `json:"state,omitempty" yaml:"state,omitempty"`
}
```

---

## Factory Registration (STABLE)

Location: `pkg/fsmv2/factory/worker_factory.go`

### RegisterWorkerType
**Preferred** registration API. Registers both worker and supervisor factories atomically.

```go
func RegisterWorkerType[TObserved ObservedState, TDesired DesiredState](
    workerFactory func(Identity, *zap.SugaredLogger, StateReader, map[string]any) Worker,
    supervisorFactory func(interface{}) interface{},
) error
```

### NewWorkerByType
Creates worker by runtime type name (used by supervisors for children).

```go
func NewWorkerByType(
    workerType string,
    identity Identity,
    logger *zap.SugaredLogger,
    stateReader StateReader,
    deps map[string]any,
) (Worker, error)
```

---

## Helper Types (STABLE)

### BaseDependencies (for embedding)
Location: `pkg/fsmv2/dependencies.go`

Provides common dependency infrastructure. Embed and extend for worker-specific deps.

```go
type BaseDependencies struct {
    // Private fields - use constructor
}

func NewBaseDependencies(logger *zap.SugaredLogger, stateReader StateReader, identity Identity) *BaseDependencies
func (d *BaseDependencies) GetLogger() *zap.SugaredLogger
func (d *BaseDependencies) ActionLogger(actionType string) *zap.SugaredLogger
func (d *BaseDependencies) GetStateReader() StateReader
func (d *BaseDependencies) Metrics() *MetricsRecorder
```

### BaseWorker (for embedding)
Location: `pkg/fsmv2/internal/helpers/worker_base.go`

Provides typed dependency access. Reduces boilerplate.

```go
type BaseWorker[D Dependencies] struct {
    // Private field
}

func NewBaseWorker[D Dependencies](dependencies D) *BaseWorker[D]
func (w *BaseWorker[D]) GetDependencies() D
func (w *BaseWorker[D]) GetDependenciesAny() any  // For DependencyProvider interface
```

---

## Known Limitations

### GetHierarchyPath() - Limited Context in Child Logs
**Issue**: Calling `GetHierarchyPath()` from child supervisor while parent holds lock causes deadlock.

**Workaround**: HierarchyPath is set once at child creation and stored in Identity. Access via `identity.HierarchyPath` instead of calling the method.

**Tracking**: See `pkg/fsmv2/supervisor/api.go:513-520` for technical details.

### Duplicate State Accessors
**Issue**: Multiple methods return state information (`GetState()`, `GetCurrentStateName()`).

**Recommendation**: Use `GetCurrentStateName()` for consistency when querying supervisor state. Use `GetState()` on DesiredState types only.

---

## Migration Checklist

### Structural Changes
- [ ] Replace `FSM` struct with `Worker` interface implementation
- [ ] Split state logic into `State[TSnapshot, TDeps]` implementations
- [ ] Move side effects into `Action[TDeps]` implementations
- [ ] Define `ObservedState` and `DesiredState` types

### Registration
- [ ] Use `factory.RegisterWorkerType[TObserved, TDesired]()` in `init()`
- [ ] Register both worker and supervisor factories atomically
- [ ] Verify registration with `factory.ListRegisteredTypes()`

### Dependencies
- [ ] Create worker-specific deps type embedding `BaseDependencies`
- [ ] Pass deps to actions via `Action[TDeps].Execute(ctx, deps)`
- [ ] Use `ActionLogger()` for action-context logging

### States
- [ ] Implement `Next()`, `String()`, `Reason()` for each state
- [ ] Return `SignalNone` for normal operation
- [ ] Return `SignalNeedsRemoval` when shutdown complete
- [ ] Return `SignalNeedsRestart` for unrecoverable errors

### Configuration
- [ ] Embed `config.BaseDesiredState` in your DesiredState
- [ ] Embed `config.BaseUserSpec` in your UserSpec (optional)
- [ ] Use `VariableBundle` for template variables
- [ ] Implement `GetChildrenSpecs()` if worker has children

### Testing
- [ ] Use `factory.ResetRegistry()` in test setup
- [ ] Test state transitions with typed snapshots
- [ ] Verify action idempotency

---

## Quick Reference

| FSMv1 Concept | FSMv2 Equivalent |
|---------------|------------------|
| FSM struct | Worker interface |
| State constants | State[T,D] implementations |
| Callbacks | Action[TDeps].Execute() |
| Machine transitions | State.Next() returns |
| Global registry | factory.RegisterWorkerType |

| Signal | Purpose |
|--------|---------|
| SignalNone | Normal operation |
| SignalNeedsRemoval | Cleanup complete, remove worker |
| SignalNeedsRestart | Unrecoverable error, restart needed |
