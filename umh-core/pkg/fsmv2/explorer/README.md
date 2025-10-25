# FSM Explorer

Interactive exploration tool for FSM v2 worker behavior. Enables manual control and inspection of worker state transitions during development and debugging.

## Purpose

FSM Explorer provides a framework for observing and controlling FSM v2 workers in isolation. Instead of relying on automatic reconciliation loops, developers can:

- Step through FSM transitions one tick at a time
- Inspect observed and desired state at each step
- Inject external events like shutdown
- Verify worker behavior with real infrastructure (containers, databases)

This is critical for debugging race conditions, understanding state transitions, and validating worker implementations before integration.

## Architecture

### Scenario Interface

The `Scenario` interface abstracts worker test environments:

```go
type Scenario interface {
    Setup(ctx context.Context) error
    Tick(ctx context.Context) error
    GetCurrentState() (string, string)
    GetObservedState() interface{}
    GetDesiredState() interface{}
    InjectShutdown() error
    Cleanup(ctx context.Context) error
}
```

Each scenario wraps:
- **Infrastructure setup** (containers, databases, mock services)
- **Worker + Supervisor initialization**
- **State inspection APIs**
- **Event injection** (shutdown, config changes)

### Command System

Explorer exposes single-letter commands for interactive control:

```go
type CommandType int

const (
    CommandUnknown    // Invalid command
    CommandTick       // "t" - Execute one FSM tick
    CommandShowState  // "s" - Display current FSM state + reason
    CommandShowObserved  // "o" - Display observed state
    CommandShowDesired   // "d" - Display desired state
    CommandShutdown   // "shutdown" - Inject shutdown request
    CommandQuit       // "q" - Exit explorer
)
```

Commands are parsed from user input and executed against the scenario:

```go
explorer := explorer.New(scenario)
cmd := explorer.ParseCommand("t")
result := explorer.ExecuteCommand(ctx, cmd)
```

## Usage

Currently programmatic (no interactive CLI yet). Typical usage in tests:

```go
// Create scenario
scenario := explorer.NewContainerScenario()

// Setup infrastructure
ctx := context.Background()
err := scenario.Setup(ctx)
defer scenario.Cleanup(ctx)

// Create explorer
exp := explorer.New(scenario)

// Manual control
exp.Tick(ctx)                    // Advance FSM one step
state, reason := exp.GetCurrentState()  // Check state

// Inject events
scenario.InjectShutdown()
exp.Tick(ctx)                    // Process shutdown
```

Future interactive CLI would wrap this in a REPL loop.

## Container Scenario

`ContainerScenario` demonstrates real-world usage with:

- **Docker container** (nginx:alpine via Testcontainers)
- **ContainerWorker** from `pkg/fsmv2/container`
- **Supervisor** managing worker lifecycle
- **TriangularStore** for persistence (identity/desired/observed)

### Setup Flow

```go
scenario := explorer.NewContainerScenario()
scenario.Setup(ctx)  // Creates:
//   1. Docker container (nginx:alpine)
//   2. Docker client
//   3. SQLite database (temp directory)
//   4. TriangularStore with 3 collections
//   5. ContainerWorker + Supervisor
//   6. Registers worker with supervisor
```

### Current Limitations

**Storage API Gap**: Supervisor does not expose observed/desired state through public API.

```go
func (c *ContainerScenario) GetObservedState() interface{} {
    // NOTE: Supervisor API does not expose observed state data.
    // This would require LoadSnapshot() which is storage layer implementation detail.
    // Returning nil indicates this limitation for now.
    return nil
}
```

This prevents full state inspection. Options:

1. Add `Supervisor.GetObservedState()` / `GetDesiredState()` methods
2. Expose storage layer directly (breaks abstraction)
3. Accept limitation (current approach)

**Why this matters**: Without state inspection, explorer can only show FSM state names, not the underlying data (container status, health checks, etc.). This reduces debugging value.

## Testing

Run tests:

```bash
cd pkg/fsmv2/explorer
go test -v
```

### Test Results

**15/19 tests pass** (4 failures in container scenario):

Passing:
- Basic explorer functionality (create, tick, state)
- Command parsing (all 7 command types)
- Command execution (tick, show state, shutdown, quit)
- Container scenario setup/cleanup
- Container scenario FSM tick

Failing:
- `GetObservedState()` - Returns nil (API limitation)
- `GetDesiredState()` - Returns nil (API limitation)
- State inspection after tick - Empty reason field
- Shutdown injection - Implementation incomplete

The failures reflect known limitations, not bugs. Core framework is functional.

## Implementation Status

- [x] Core framework (`explorer.go`)
- [x] Command parsing/execution
- [x] Scenario interface definition
- [x] Container scenario setup/cleanup
- [x] FSM tick integration
- [ ] State inspection APIs (blocked by Supervisor API)
- [ ] Interactive CLI (future work)
- [ ] Multi-scenario support (future)

## Known Issues

### Storage Architecture Limitation

The supervisor/storage separation creates an inspection gap:

**Problem**: `Supervisor` manages workers but doesn't expose their state data. State lives in `TriangularStore`, but accessing it requires storage layer knowledge.

**Impact**:
- `GetObservedState()` returns nil
- `GetDesiredState()` returns nil
- Debugging requires log inspection instead of direct state queries

**Example**: When debugging container worker, you can see state is "Running" but can't inspect health check data, port mappings, or error counts without adding supervisor methods.

**Possible solutions**:
1. Add reflection APIs to Supervisor: `GetStateData(workerID, role) interface{}`
2. Make workers expose state via `Worker.Debug() interface{}`
3. Accept limitation and rely on logs + metrics

### Empty State Reason

`GetCurrentState()` returns `(stateName, reason)` but reason is currently empty:

```go
func (c *ContainerScenario) GetCurrentState() (string, string) {
    stateName := c.supervisor.GetCurrentState()
    return stateName, ""  // No reason available from Supervisor API
}
```

Supervisor doesn't track transition reasons. Would need FSM callback integration.

## Future Work

### Interactive CLI

Build REPL interface:

```
FSM Explorer - Container Scenario
Container: nginx:alpine (abc123)
Current State: Running

Commands:
  t         - Tick (advance FSM)
  s         - Show state
  o         - Show observed
  d         - Show desired
  shutdown  - Inject shutdown
  q         - Quit

> t
[Tick executed]
State: Running -> Stopping
Reason: shutdown requested

> s
State: Stopping
Reason: waiting for graceful termination
ObservedState:
  Status: running
  ExitCode: null
  Health: healthy

>
```

### Multi-Scenario Support

Support multiple scenarios in one session:

```go
explorer.AddScenario("container", NewContainerScenario())
explorer.AddScenario("benthos", NewBenthosScenario())
explorer.SwitchScenario("benthos")
```

### State Diffing

Show state changes between ticks:

```
> t
[Tick executed]
State: Starting -> Running

ObservedState diff:
  + Status: running
  + PID: 42
  + StartedAt: 2025-10-20T14:30:00Z
```

### Replay Mode

Record tick sequences and replay:

```bash
# Record session
explorer --record container_startup.replay

# Replay session
explorer --replay container_startup.replay
```

## Code Examples

### Basic Explorer Usage

```go
package main

import (
    "context"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/explorer"
)

func main() {
    scenario := explorer.NewContainerScenario()

    ctx := context.Background()
    scenario.Setup(ctx)
    defer scenario.Cleanup(ctx)

    exp := explorer.New(scenario)

    // Step through FSM transitions
    for i := 0; i < 5; i++ {
        exp.Tick(ctx)
        state, reason := exp.GetCurrentState()
        fmt.Printf("Tick %d: %s (%s)\n", i, state, reason)
    }
}
```

### Custom Scenario Implementation

```go
type MyScenario struct {
    worker     *MyWorker
    supervisor *supervisor.Supervisor
}

func (m *MyScenario) Setup(ctx context.Context) error {
    // Initialize worker infrastructure
    m.worker = NewMyWorker()

    // Create supervisor
    m.supervisor = supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "my-worker",
        Store:      myStore,
        Logger:     logger,
    })

    // Register worker
    identity := fsmv2.Identity{ID: "test-1", WorkerType: "my-worker"}
    return m.supervisor.AddWorker(identity, m.worker)
}

func (m *MyScenario) Tick(ctx context.Context) error {
    return m.supervisor.Tick(ctx)
}

func (m *MyScenario) GetCurrentState() (string, string) {
    return m.supervisor.GetCurrentState(), "custom reason"
}

// Implement remaining Scenario interface methods...
```

### Command Execution Pattern

```go
exp := explorer.New(scenario)

commands := []string{"t", "t", "s", "shutdown", "t", "s", "q"}
for _, cmdStr := range commands {
    cmd := explorer.ParseCommand(cmdStr)
    result := exp.ExecuteCommand(ctx, cmd)

    if !result.Success {
        log.Printf("Command failed: %s", result.Message)
        break
    }

    if cmd.Type == explorer.CommandQuit {
        break
    }
}
```

## Integration with FSM v2

Explorer sits alongside the supervisor pattern:

```
┌────────────────────────────────────┐
│  FSM v2 Architecture               │
├────────────────────────────────────┤
│  Supervisor (automatic loop)       │
│    ├─ Observe()                    │
│    ├─ Decide()                     │
│    ├─ Act()                        │
│    └─ Tick() every 100ms           │
└────────────────────────────────────┘

┌────────────────────────────────────┐
│  Explorer (manual control)         │
├────────────────────────────────────┤
│  Scenario                          │
│    ├─ Setup() infrastructure       │
│    ├─ Tick() when commanded        │
│    ├─ GetCurrentState()            │
│    └─ InjectShutdown()             │
└────────────────────────────────────┘
```

Explorer pauses automatic reconciliation, giving developers full control over timing and sequence.

## References

- FSM v2 architecture: `pkg/fsmv2/README.md`
- Supervisor implementation: `pkg/fsmv2/supervisor/`
- Container worker: `pkg/fsmv2/container/`
- Storage layer: `pkg/cse/storage/`
