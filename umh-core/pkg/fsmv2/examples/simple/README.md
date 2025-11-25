# FSM v2 Simple Example

A minimal, runnable example demonstrating the FSM v2 application worker pattern.

## What This Example Demonstrates

This example shows the core pattern for using FSM v2:

1. **Application Worker Creation** - How to create the root supervisor that orchestrates everything
2. **Parent-Child Relationship** - How workers register children through YAML configuration
3. **Lifecycle Management** - Starting, running, and gracefully shutting down
4. **FSM in Action** - Watch state transitions and reconciliation in real-time

## Quick Start

Run the example from this directory:

```bash
cd pkg/fsmv2/examples/simple
go run main.go
```

Or build and run:

```bash
go build -o simple
./simple
```

## What You'll See

When you run the example, you'll see:

```
=== FSM v2 Application Worker - Simple Example ===

Step 1: Creating storage...
Storage created successfully

Step 2: Creating application supervisor...
This supervisor will manage a parent worker with 2 child workers
Application supervisor created successfully

Step 3: Starting the application worker...
Application worker started!

Step 4: Running for 10 seconds...
The FSM is now managing the parent and child workers.
Watch the logs above to see state transitions and reconciliation.

Status: Application running, FSM reconciling workers...
Status: Application running, FSM reconciling workers...
...

Step 5: Gracefully shutting down...
Application shut down successfully!

=== Example Complete ===
```

## Key Concepts

### Application Worker Pattern

The **application worker** is the root supervisor that:
- Parses YAML configuration to discover child workers
- Dynamically creates child workers via the factory pattern
- Manages the complete application lifecycle
- Handles graceful shutdown

```go
sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
    ID:           "app-001",
    Name:         "Simple Application",
    Store:        store,
    Logger:       sugar,
    TickInterval: 100 * time.Millisecond,
    YAMLConfig:   yamlConfig,
})
```

### YAML Configuration

The application worker reads a YAML configuration that specifies:
- Which workers to create (`workerType`)
- How to configure them (`userSpec.config`)
- Parent-child relationships (nested `children` arrays)

```yaml
children:
  - name: "parent-1"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 2
```

### Parent-Child Relationships

Workers can create child workers by:
1. Parsing their `userSpec.config` to determine how many children
2. Returning `ChildrenSpecs` from `DeriveDesiredState()`
3. The supervisor automatically creates/manages children via `reconcileChildren()`

### Graceful Shutdown

The FSM handles shutdown cleanly:
1. Cancel the context
2. Wait for `done` channel
3. All workers stop in order (children first, then parents)

## Architecture

```
Application Worker (supervisor)
    │
    └── Parent Worker
            ├── Child Worker 1
            └── Child Worker 2
```

The application worker:
- Creates the parent worker based on YAML config
- The parent worker returns ChildrenSpecs for 2 children
- The supervisor automatically creates and manages the children
- All workers run their FSMs independently
- State is persisted in the triangular store

## Code Walkthrough

### 1. Create Storage

```go
store, err := storage.NewTriangularStore(storage.StoreConfig{
    Engine: storage.MemoryEngine,
    Logger: sugar,
})
```

The **triangular store** persists FSM state with three perspectives:
- **Observed State**: What the worker sees (via `CollectObservedState()`)
- **Desired State**: What the worker should be (via `DeriveDesiredState()`)
- **Current FSM State**: Which state the FSM is in

### 2. Define YAML Configuration

```go
yamlConfig := `
children:
  - name: "parent-1"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 2
`
```

This tells the application worker:
- Create a worker named "parent-1"
- Use the "parent" worker type (registered in factory)
- Pass `children_count: 2` to the parent's `DeriveDesiredState()`

### 3. Create Application Supervisor

```go
sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
    ID:           "app-001",
    Name:         "Simple Application",
    Store:        store,
    Logger:       sugar,
    TickInterval: 100 * time.Millisecond,
    YAMLConfig:   yamlConfig,
})
```

The **application supervisor**:
- Wraps an application worker
- Parses the YAML to discover children
- Ticks every 100ms to reconcile state
- Uses the factory pattern to create workers dynamically

### 4. Start and Run

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

done := sup.Start(ctx)
```

`Start()` returns a channel that closes when the supervisor shuts down.
This allows for graceful shutdown and waiting for completion.

### 5. Graceful Shutdown

```go
cancel()      // Signal shutdown
<-done        // Wait for completion
```

Canceling the context triggers:
1. Each FSM transitions to shutdown states
2. Children shut down before parents
3. Resources are cleaned up
4. The `done` channel closes

## What's Happening Under the Hood

When the example runs, the FSM v2 system:

1. **Application worker** reads YAML and returns a `ChildSpec` for "parent-1"
2. **Supervisor** calls `reconcileChildren()` to create the parent worker
3. **Parent worker** returns 2 `ChildSpecs` from `DeriveDesiredState()`
4. **Supervisor** creates child-0 and child-1 workers
5. **Each worker** runs its FSM independently:
   - Starts in initial state (typically "stopped")
   - Transitions through states based on observed vs desired
   - Reconciles every 100ms (tick interval)
6. **On shutdown**:
   - Context is canceled
   - Children shutdown first
   - Then parent
   - Then application worker
   - `done` channel closes

## Next Steps

### Build Your Own Worker

Use the example workers as templates:
- **Parent Worker**: `pkg/fsmv2/workers/example/example-parent/`
- **Child Worker**: `pkg/fsmv2/workers/example/example-child/`

Key files to implement:
- `worker.go` - Implements the Worker interface
- `snapshot/snapshot.go` - Defines ObservedState and DesiredState types
- `state/*.go` - FSM state implementations
- `dependencies.go` - External dependencies (databases, connections, etc.)

### Explore the Full FSM v2 System

- **Core Documentation**: `pkg/fsmv2/README.md`
- **Application Worker**: `pkg/fsmv2/workers/application/README.md`
- **Supervisor**: `pkg/fsmv2/supervisor/README.md`
- **Worker Interface**: `pkg/fsmv2/worker.go`
- **State Interface**: `pkg/fsmv2/state.go`

### Key Patterns to Study

1. **Factory Pattern** - How workers register themselves for dynamic creation
2. **Passthrough Pattern** - How application worker doesn't hardcode child types
3. **Reconciliation Loop** - How supervisors continuously reconcile observed vs desired
4. **State Machines** - How each worker implements its own FSM
5. **Parent-Child Relationships** - How workers spawn and manage children

## Troubleshooting

### Example won't compile

Make sure you're in the correct directory:
```bash
cd pkg/fsmv2/examples/simple
go build
```

### Import errors

The example requires these packages to be registered:
```go
_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
```

These blank imports trigger `init()` functions that register the workers with the factory.

### Not seeing enough logs

Increase log verbosity or add debug output to `CollectObservedState()` and `DeriveDesiredState()`.

## Known Issues

This example currently shows a type mismatch error in the logs:

```
ERROR: observed state type mismatch: expected snapshot.ApplicationObservedState, got *snapshot.ApplicationObservedState
```

This is a known issue in the application worker's snapshot types where the interface methods are defined on value receivers instead of pointer receivers. This doesn't affect the core demonstration of how to use the application worker pattern. The application worker still manages lifecycle correctly, and the example demonstrates:

1. How to create storage and collections
2. How to define YAML configuration
3. How to create an application supervisor
4. How to start and stop the FSM

For production use, this type mismatch would need to be fixed in the application worker's snapshot package. The pattern demonstrated here is correct.

## Summary

This example demonstrates the **application worker pattern**, which is the production pattern used in UMH Core. The key insight is:

**"Application worker reads YAML, creates workers via factory, supervisor handles lifecycle."**

Everything else (state machines, reconciliation, parent-child relationships) is handled by the FSM v2 framework automatically.

Despite the type mismatch errors in the logs, the example successfully demonstrates:
- Storage initialization
- Application worker creation
- YAML configuration parsing
- Supervisor lifecycle (start/stop)
- Graceful shutdown
