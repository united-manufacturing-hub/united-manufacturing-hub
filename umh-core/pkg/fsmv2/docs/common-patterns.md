# Common Patterns

This document covers patterns that solve common problems in FSM v2 development. Each pattern includes the problem, solution, and where to find the implementation.

## 1. Always Check Desired State

**Problem:** States transition unconditionally, ignoring user intent.

**INCORRECT (the bug we fixed):**
```go
func (s *StoppedState) Next(snapAny any) (State, Signal, Action) {
    snap := ConvertSnapshot[MyObserved, *MyDesired](snapAny)

    // BUG: Always transitions regardless of desired state!
    return &StartingState{}, SignalNone, nil
}
```

**CORRECT:**
```go
func (s *StoppedState) Next(snapAny any) (State, Signal, Action) {
    snap := ConvertSnapshot[MyObserved, *MyDesired](snapAny)

    // 1. Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return s, SignalNeedsRemoval, nil
    }

    // 2. Only transition if desired state says so
    if snap.Desired.ShouldBeRunning {
        return &StartingState{}, SignalNone, nil
    }

    // 3. Stay in current state otherwise
    return s, SignalNone, nil
}
```

**Implementation:** `pkg/fsmv2/examples/parent_child/states.go:39-60`

## 2. Shutdown Priority

**Problem:** Workers don't stop properly because shutdown check is buried in state logic.

**Pattern:** Every state's `Next()` method MUST check shutdown first.

```go
func (s *AnyState) Next(snapAny any) (State, Signal, Action) {
    snap := ConvertSnapshot[MyObserved, *MyDesired](snapAny)

    // FIRST LINE: Always check shutdown
    if snap.Desired.IsShutdownRequested() {
        // Return appropriate cleanup state or signal removal
        return &CleanupState{}, SignalNone, nil
    }

    // ... rest of state logic
}
```

**From Stopped state (already clean):**
```go
if snap.Desired.IsShutdownRequested() {
    return s, SignalNeedsRemoval, nil  // Ready to be removed
}
```

**From Running state (needs cleanup):**
```go
if snap.Desired.IsShutdownRequested() {
    return &StoppingState{}, SignalNone, nil  // Need to stop first
}
```

**Implementation:** Every state in `pkg/fsmv2/examples/parent_child/states.go`

## 3. Parent-Child Coordination

**Problem:** Unclear how parents control child FSM lifecycles.

**Pattern:** Parents use observed state to track children, desired state to control them.

```go
// Parent waits for ALL children before advancing
func (s *StartingChildrenState) Next(snapAny any) (State, Signal, Action) {
    snap := ConvertSnapshot[ParentObserved, *ParentDesired](snapAny)

    // Check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &StoppingChildrenState{}, SignalNone, nil
    }

    // Only advance when ALL children are running
    if snap.Observed.AllChildrenRunning {
        return &ChildrenRunningState{}, SignalNone, nil
    }

    // Still waiting - emit action to ensure children are started
    return s, SignalNone, &StartChildrenAction{
        ChildIDs: snap.Desired.ChildIDs,
    }
}
```

**Key patterns:**
- `Observed.AllChildrenRunning` - Parent monitors child states
- `Desired.ChildrenShouldBeRunning` - User controls child lifecycle
- Actions set child desired states to trigger child transitions

**Implementation:** `pkg/fsmv2/examples/parent_child/states.go:70-106`

## 4. State Mapping Registry

**Problem:** Need to map parent states to child desired states.

**Location:** `pkg/fsmv2/state_mapping.go`

**Usage:**
```go
registry := fsmv2.NewStateMappingRegistry()

// Register mappings
registry.Register(fsmv2.StateMapping{
    ParentState: "StartingChildren",
    ChildDesired: map[string]string{
        "child-1": "running",
        "child-2": "running",
    },
})

registry.Register(fsmv2.StateMapping{
    ParentState: "StoppingChildren",
    ChildDesired: map[string]string{
        "child-1": "stopped",
        "child-2": "stopped",
    },
})

// Use in state logic
childStates := registry.DeriveChildDesiredStates(
    "StartingChildren",
    parentSnapshot,
)
// Returns: {"child-1": "running", "child-2": "running"}
```

**With conditions:**
```go
registry.Register(fsmv2.StateMapping{
    ParentState: "Running",
    ChildDesired: map[string]string{"child-1": "running"},
    Condition: func(snap fsmv2.Snapshot) bool {
        return snap.Observed.(MyObserved).IsHealthy
    },
})
```

**Implementation:**
- Registry: `pkg/fsmv2/state_mapping.go`
- Tests: `pkg/fsmv2/state_mapping_test.go`

## 5. DeriveDesiredState

**Problem:** How does user configuration become desired state?

**Pattern:** `DeriveDesiredState` transforms user config into technical desired state.

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    // Handle nil spec
    if spec == nil {
        return config.DesiredState{State: "running"}, nil
    }

    // Parse user spec
    userSpec, ok := spec.(config.UserSpec)
    if !ok {
        return config.DesiredState{}, fmt.Errorf("invalid spec type")
    }

    var parentSpec ParentUserSpec
    if err := yaml.Unmarshal([]byte(userSpec.Config), &parentSpec); err != nil {
        return config.DesiredState{}, err
    }

    // Generate child specs from user config
    childSpecs := make([]config.ChildSpec, len(parentSpec.ChildIDs))
    for i, id := range parentSpec.ChildIDs {
        childSpecs[i] = config.ChildSpec{
            Name:       id,
            WorkerType: "child",
        }
    }

    return config.DesiredState{
        State:         "running",
        ChildrenSpecs: childSpecs,
    }, nil
}
```

**Key points:**
- Supervisor calls this on each tick
- Pure function - no side effects
- Returns `config.DesiredState` with optional `ChildrenSpecs`

**Implementation:** `pkg/fsmv2/examples/parent_child/parent_worker.go:135-184`

## Quick Reference Table

| Problem | Pattern | File Location |
|---------|---------|---------------|
| State transitions ignore user intent | Check `Desired.ShouldBe*` | `examples/parent_child/states.go:54` |
| Shutdown not handled | Check `IsShutdownRequested()` first | Every state's `Next()` method |
| Parent doesn't wait for children | Check `Observed.AllChildrenRunning` | `examples/parent_child/states.go:97` |
| Need to map parent→child states | Use `StateMappingRegistry` | `state_mapping.go` |
| Config→technical state | Implement `DeriveDesiredState` | `examples/parent_child/parent_worker.go:135` |

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Transitioning without checking desired state | Always check `snap.Desired.ShouldBe*` before returning new state |
| Checking shutdown in the middle of `Next()` | Move shutdown check to FIRST line |
| Advancing before children ready | Check `snap.Observed.AllChildrenRunning` |
| Hardcoding child states | Use `StateMappingRegistry` for declarative mapping |
| Calling `DeriveDesiredState` yourself | Supervisor calls it - you implement it |
