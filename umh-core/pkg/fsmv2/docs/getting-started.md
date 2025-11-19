# Getting Started with FSM v2

## What is FSM v2?

FSM v2 is a state machine framework for managing worker lifecycles. Each worker has three parts: **Identity** (who am I?), **Desired** (what should I be?), and **Observed** (what am I?). The supervisor reconciles these by calling `State.Next(snapshot)` on every tick.

```
     IDENTITY
        /\
       /  \
   DESIRED  OBSERVED
      \    /
   RECONCILIATION
```

## The Triangle Model

Every FSM v2 worker operates on this model:

| Component | Purpose | Immutable? |
|-----------|---------|------------|
| Identity | `ID`, `Name`, `WorkerType` | Yes |
| Desired | User configuration transformed into technical state | No - versioned |
| Observed | Actual system state collected via monitoring | No - refreshed each tick |

## 5-Minute Example: Create a Simple Worker

### Step 1: Define State Types

```go
// pkg/fsmv2/workers/myworker/models.go
package myworker

import "time"

type MyObservedState struct {
    CollectedAt time.Time
    IsHealthy   bool
}

func (o MyObservedState) GetObservedDesiredState() DesiredState { return nil }
func (o MyObservedState) GetTimestamp() time.Time { return o.CollectedAt }

type MyDesiredState struct {
    shutdownRequested bool
    ShouldBeRunning   bool
}

func (d *MyDesiredState) IsShutdownRequested() bool { return d.shutdownRequested }
```

### Step 2: Implement States

```go
// pkg/fsmv2/workers/myworker/states.go
package myworker

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type StoppedState struct{}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }

    // RULE 2: Only transition if desired state says so
    if snap.Desired.ShouldBeRunning {
        return &RunningState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string { return "Stopped" }
func (s *StoppedState) Reason() string { return "Worker is stopped" }

type RunningState struct{}

func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &StoppedState{}, fsmv2.SignalNone, nil
    }

    // RULE 2: Check if desired state changed
    if !snap.Desired.ShouldBeRunning {
        return &StoppedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *RunningState) String() string { return "Running" }
func (s *RunningState) Reason() string { return "Worker is running" }
```

### Step 3: Implement Worker Interface

```go
// pkg/fsmv2/workers/myworker/worker.go
package myworker

import (
    "context"
    "time"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type MyWorker struct {
    *fsmv2.BaseWorker[*MyDependencies]
    identity fsmv2.Identity
}

func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return MyObservedState{
        CollectedAt: time.Now(),
        IsHealthy:   true,
    }, nil
}

func (w *MyWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    return config.DesiredState{
        State: "running",
    }, nil
}

func (w *MyWorker) GetInitialState() fsmv2.State[any, any] {
    return &StoppedState{}
}
```

## Key Interfaces

| Interface | Location | Purpose |
|-----------|----------|---------|
| `Worker` | `pkg/fsmv2/api.go:287` | Business logic - CollectObservedState, DeriveDesiredState |
| `State` | `pkg/fsmv2/api.go:219` | Decision logic - Next(snapshot) returns next state + action |
| `Action` | `pkg/fsmv2/api.go:169` | Side effects - Execute(ctx, deps) performs work |
| `Snapshot` | `pkg/fsmv2/api.go:118` | Data container - Identity + Observed + Desired |

## Where to Find Examples

| Example | Location | Demonstrates |
|---------|----------|--------------|
| Parent-child coordination | `pkg/fsmv2/examples/parent_child/` | State mapping, child lifecycle |
| Communicator worker | `pkg/fsmv2/workers/communicator/` | Production worker with auth/sync |
| Example workers | `pkg/fsmv2/workers/example/` | Simple parent and child patterns |

## Next Steps

1. Read `common-patterns.md` for essential patterns every state must follow
2. Check `PATTERNS.md` for detailed design rationale
3. Look at `examples/parent_child/states.go` for correct desired state checks
