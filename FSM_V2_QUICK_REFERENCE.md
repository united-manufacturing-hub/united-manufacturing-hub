# FSM v2 Worker Capabilities: Quick Reference

## The 13 MUST-HAVE Capabilities

Every example worker must demonstrate these capabilities. Use this checklist during implementation.

### 1-2: CollectObservedState (Lines 276-297 in worker.go)

```go
func (w *Worker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// MUST: Respect context cancellation
	// MUST: Include timestamp for staleness detection
	// MUST: Handle errors gracefully (log, don't fail)
	return &ObservedState{
		CollectedAt: time.Now(),
		// ... observed state fields
	}, nil
}
```

**Capability 1**: Async monitoring with timeout protection (ctx parameter)
**Capability 2**: Error handling (log errors, return last known state)

### 3-4: DeriveDesiredState (Lines 299-316 in worker.go)

```go
func (w *Worker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	// MUST: Pure function - no side effects
	// MUST: Can use ChildrenSpecs for hierarchical composition
	return types.DesiredState{
		State: "running",
		ChildrenSpecs: []types.ChildSpec{
			// Optionally declare child workers
		},
	}, nil
}
```

**Capability 3**: Pure function transformation (deterministic, no I/O)
**Capability 4**: Templating and composition via ChildrenSpecs

### 5: GetInitialState (Lines 312-316 in worker.go)

```go
func (w *Worker) GetInitialState() State {
	return &StoppedState{}  // MUST return starting state
}
```

**Capability 5**: Clear FSM entry point

### 6: Signal Types (Lines 38-49 in worker.go)

```go
// MUST use in state transitions:
return nextState, fsmv2.SignalNone, action      // Normal operation
return nextState, fsmv2.SignalNeedsRemoval, nil // Worker cleanup complete
return nextState, fsmv2.SignalNeedsRestart, nil // Request restart cycle
```

**Capability 6**: Supervisor-level communication via signals

### 7: Active States - "TryingTo*" (Lines 180-194 in worker.go)

```go
func (s *TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
	// MUST check shutdown first
	if snapshot.Desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}
	
	// MUST check success condition
	if snapshot.Observed.ProcessRunning {
		return &RunningState{}, fsmv2.SignalNone, nil  // Exit loop
	}
	
	// MUST emit action on retry
	return s, fsmv2.SignalNone, &StartAction{}  // Loop: returns self
}
```

**Capability 7**: Action-emitting states that retry until success

### 8: Passive States - Descriptive Nouns (Lines 180-194 in worker.go)

```go
func (s *RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
	// MUST check conditions only, don't emit action
	if snapshot.Desired.ShutdownRequested() {
		return &StoppingState{}, fsmv2.SignalNone, nil  // Transition only
	}
	
	if !snapshot.Observed.ProcessRunning {
		return &CrashedState{}, fsmv2.SignalNone, nil  // Transition only
	}
	
	return s, fsmv2.SignalNone, nil  // MUST: No action in passive state
}
```

**Capability 8**: Observation-only states that monitor for changes

### 9: State Transitions (Lines 210-250 in worker.go)

```go
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
	// 1. PRIORITY: Always check shutdown first (Invariant I4)
	if snapshot.Desired.ShutdownRequested() {
		return nextState, fsmv2.SignalNone, nil
	}
	
	// 2. CRITICAL: Check fatal errors
	if snapshot.Observed.HasError {
		return &ErrorState{}, fsmv2.SignalNone, nil
	}
	
	// 3. OPERATIONAL: Check success conditions
	if snapshot.Observed.Ready {
		return &ReadyState{}, fsmv2.SignalNone, nil
	}
	
	// 4. RECOVERY: Retry action
	return s, fsmv2.SignalNone, &RetryAction{}
}
```

**Capability 9**: Explicit, condition-based transitions with guard conditions

### 10: Snapshot Immutability (Lines 82-114 in worker.go)

```go
func (s *MyState) Next(snapshot Snapshot) (State, Signal, Action) {
	// MUST: Treat snapshot as read-only
	// Snapshot is passed by value (copied) to your method
	// Mutations only affect local copy, supervisor's original is safe
	
	if snapshot.Observed.Healthy {  // Read from snapshot
		return &HealthyState{}, fsmv2.SignalNone, nil
	}
	
	// WRONG: Don't mutate
	// snapshot.Observed = nil  // This doesn't help anything
	
	return s, fsmv2.SignalNone, &HealthCheckAction{}
}
```

**Capability 10**: Read-only snapshot handling (Go pass-by-value enforces this)

### 11: Action Idempotency (Lines 116-167 in worker.go) - CRITICAL

```go
func (a *MyAction) Execute(ctx context.Context) error {
	// MUST: Check if work is already done
	if alreadyDone(a.target) {
		return nil  // Idempotent: calling again is safe
	}
	
	// MUST: Perform work
	return doWork(a.target)
}

func (a *MyAction) Name() string {
	return "MyActionName"
}

// MUST: Add idempotency test
func TestMyActionIdempotency(t *testing.T) {
	action := &MyAction{}
	VerifyActionIdempotency(action, 3, func() {
		// Verify final state same whether called 1 or 3 times
	})
}
```

**Capability 11**: Idempotent actions (can be retried safely)

### 12: Context Cancellation (Lines 162-164 in worker.go)

```go
func (a *MyAction) Execute(ctx context.Context) error {
	// MUST: Pass context to all blocking operations
	req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
	
	// MUST: Check context in long loops
	select {
	case <-ctx.Done():
		return ctx.Err()  // Respect cancellation
	default:
	}
	
	// MUST: Never block indefinitely
	return client.Do(req)  // Respects ctx deadline
}
```

**Capability 12**: Context cancellation handling for graceful shutdown

### 13: ObservedState Interface (Lines 59-71 in worker.go)

```go
type MyObservedState struct {
	CollectedAt time.Time  // MUST have timestamp
	// ... other fields
}

// MUST: Implement GetTimestamp()
func (o *MyObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// MUST: Implement GetObservedDesiredState()
func (o *MyObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &MyDesiredState{}
}
```

**Capability 13**: ObservedState interface implementation

---

## Implementation Order (Recommended)

1. **Define ObservedState and DesiredState structures** (Capability 13)
2. **Implement Worker interface** (Capabilities 1-5)
3. **Create initial state** (Capability 5)
4. **Add one passive state** (Capability 8)
5. **Add one active state** (Capability 7)
6. **Implement state transitions** (Capability 9)
7. **Add snapshot handling** (Capability 10)
8. **Create action** (Capabilities 11-12)
9. **Add Signal usage** (Capability 6)
10. **Write tests** (All capabilities)

---

## Key Invariants from worker.go

| Line | Invariant | Description |
|------|-----------|-------------|
| 86-114 | I9 | Snapshot immutability via pass-by-value |
| 116-167 | I10 | Action idempotency requirement |
| 162-164 | I6 | Context cancellation in async operations |
| 174-175 | I4 | Shutdown check priority in Next() |
| 180-194 | - | Active vs passive state naming convention |

---

## Testing Patterns

### Action Idempotency Test

```go
import . "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"

It("should be idempotent", func() {
	action := &MyAction{target: "test"}
	VerifyActionIdempotency(action, 5, func() {
		Expect(fileExists("test")).To(BeTrue())
		// Verify final state is identical
	})
})
```

### State Transition Test

```go
It("should transition from TryingToStart to Running", func() {
	state := &TryingToStartState{}
	snapshot := Snapshot{
		Observed: &MyObservedState{Running: true},
		Desired: &MyDesiredState{},
	}
	
	nextState, signal, action := state.Next(snapshot)
	Expect(nextState).To(BeAssignableToTypeOf(&RunningState{}))
	Expect(signal).To(Equal(fsmv2.SignalNone))
	Expect(action).To(BeNil())
})
```

---

## Common Mistakes to Avoid

1. **Not checking shutdown first** - Always check `ShutdownRequested()` first in Next()
2. **Non-idempotent actions** - Don't mutate counters, use check-then-act pattern
3. **Not respecting context** - Pass context to all blocking operations
4. **Emitting action and changing state together** - Do one or the other per tick
5. **No error handling in CollectObservedState** - Log errors, don't fail
6. **Passive states emitting actions** - Passive states ONLY observe and transition
7. **Mutating snapshot** - Snapshot is immutable, don't try to modify it
8. **Missing timestamp in ObservedState** - Always include CollectedAt for staleness checks
9. **Not implementing DesiredState interface** - Must return type that implements ShutdownRequested()
10. **Forgetting idempotency tests** - Every action MUST have VerifyActionIdempotency test

---

## Reference Files

- **Core interface**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go`
- **Example worker**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go`
- **Example states**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/state/*.go`
- **Example action**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/action/sync.go`
- **Test helpers**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/supervisor/execution/action_idempotency_test.go`
- **Full analysis**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/FSM_V2_WORKER_CAPABILITIES.md`

