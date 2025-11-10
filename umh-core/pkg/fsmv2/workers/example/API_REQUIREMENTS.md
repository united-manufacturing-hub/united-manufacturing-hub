# FSM v2 Worker API Requirements

**Quick Reference: Exact API signatures required for FSM v2 workers**

## Worker Interface (EXACT SIGNATURES REQUIRED)

✅ **GetInitialState() fsmv2.State** - NOT custom type
✅ **CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error)**
✅ **DeriveDesiredState(spec interface{}) (config.DesiredState, error)**

### Worker Implementation

```go
// ✅ CORRECT
func (w *YourWorker) GetInitialState() fsmv2.State {
    return state.NewStoppedState(w.deps)
}

// ❌ WRONG - Returns custom type
func (w *YourWorker) GetInitialState() state.BaseYourState {
    return &state.StoppedState{}
}
```

## State Interface (EXACT SIGNATURES REQUIRED)

✅ **Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action)**
✅ **String() string** - Return state name
✅ **Reason() string** - Return description

### State Implementation

```go
// ✅ CORRECT - All three methods implemented
type StoppedState struct {
    deps snapshot.YourDependencies  // Interface type
}

func NewStoppedState(deps snapshot.YourDependencies) *StoppedState {
    return &StoppedState{deps: deps}
}

func (s *StoppedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return NewTryingToStartState(s.deps), fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
    return "Stopped"
}

func (s *StoppedState) Reason() string {
    return "Worker is stopped"
}
```

```go
// ❌ WRONG - Missing String() and Reason() methods
type StoppedState struct {
    deps snapshot.YourDependencies
}

func (s *StoppedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, nil
}
// Won't compile!
```

## State Structure (REQUIRED PATTERN)

```go
// ✅ CORRECT
type YourState struct {
    deps snapshot.YourDependencies  // Interface type to avoid import cycles
}

func NewYourState(deps snapshot.YourDependencies) *YourState {
    return &YourState{deps: deps}
}
```

```go
// ❌ WRONG - Missing dependencies
type YourState struct{}

// ❌ WRONG - Concrete type causes import cycles
type YourState struct {
    deps *parent.ParentDependencies
}
```

## State Transitions (REQUIRED PATTERN)

```go
// ✅ CORRECT - Uses constructor to preserve dependencies
func (s *StoppedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return NewTryingToStartState(s.deps), fsmv2.SignalNone, nil
}
```

```go
// ❌ WRONG - Direct struct creation loses dependencies
func (s *StoppedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return &TryingToStartState{}, fsmv2.SignalNone, nil  // deps is nil!
}
```

## Quick Verification

**These MUST compile without errors:**

```go
// Verify worker implements interface
var _ fsmv2.Worker = (*YourWorker)(nil)

// Verify all states implement interface
var (
    _ fsmv2.State = (*StoppedState)(nil)
    _ fsmv2.State = (*TryingToStartState)(nil)
    _ fsmv2.State = (*RunningState)(nil)
    _ fsmv2.State = (*TryingToStopState)(nil)
)
```

**If these fail:**
- GetInitialState() signature wrong → Return `fsmv2.State`
- State missing methods → Add String() and Reason()
- Import cycle → Use interface types from snapshot package
- Nil dependencies → Add constructor and use it in transitions

## Common Compilation Errors

| Error Message | Fix |
|---------------|-----|
| `GetInitialState() has wrong signature` | Return `fsmv2.State` not custom type |
| `StoppedState does not implement fsmv2.State` | Add String() and Reason() methods |
| `import cycle not allowed` | Use `snapshot.YourDependencies` interface |
| `undefined: NewSomeState` | Add constructor for each state |
| `s.deps undefined` | Add deps field to state struct |

## File Organization Pattern

```
worker/
├── snapshot/
│   └── snapshot.go          # Dependency interfaces (breaks import cycles)
├── state/
│   ├── state_stopped.go     # Uses snapshot.YourDependencies
│   ├── state_trying_to_start.go
│   └── state_running.go
├── action/
│   ├── action_start.go
│   └── action_stop.go
└── worker.go                # Concrete dependency implementation
```

## Pre-Integration Checklist

Before running integration tests:

```bash
# 1. Verify worker compiles
go build ./pkg/fsmv2/workers/your-worker/...

# 2. Verify interface compliance
cd pkg/fsmv2/workers/your-worker
echo 'package yourworker; import "pkg/fsmv2"; var _ fsmv2.Worker = (*YourWorker)(nil)' | go build -

# 3. Verify states compile
go build ./pkg/fsmv2/workers/your-worker/state/...

# 4. Run unit tests
go test ./pkg/fsmv2/workers/your-worker/...
```

If any fail → API mismatch. Fix before integration tests.

## See Also

- **PATTERN.md** - Full implementation guide
- **INTEGRATION_TEST_PLAN.md** - Testing strategy
- **/pkg/fsmv2/workers/example/example-parent/** - Parent worker reference
- **/pkg/fsmv2/workers/example/example-child/** - Child worker reference
