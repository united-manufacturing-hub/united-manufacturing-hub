# Root Supervisor Example

This package demonstrates how to use the generic `root` package with custom child worker types. It shows the passthrough pattern where a root supervisor dynamically creates children based on YAML configuration.

## Overview

This example implements:
- **ChildWorker** - A leaf worker type that can be managed by the passthrough root
- **Factory registration** - Automatic registration via `init()`
- **State types** - ObservedState and DesiredState for the child worker

The root supervisor itself comes from the generic `root` package - this example only defines the custom child worker type.

## Package Structure

```
root_supervisor/
  types.go                  # Re-exports child types for convenience
  child_worker.go           # ChildWorker implementation
  factory_registration.go   # Factory registration via init()
  integration_test.go       # Usage examples
  snapshot/
    snapshot.go             # State types (ChildObservedState, ChildDesiredState)
  state/
    child_stopped_state.go  # FSM state implementation
```

## Creating Custom Child Workers

### 1. Define State Types

Create observed and desired state types in a `snapshot` package:

```go
// snapshot/snapshot.go
type ChildObservedState struct {
    ID               string    `json:"id"`
    CollectedAt      time.Time `json:"collected_at"`
    ConnectionStatus string    `json:"connection_status"`
    ConnectionHealth string    `json:"connection_health"`
}

type ChildDesiredState struct {
    config.DesiredState
    // Add custom fields
}
```

### 2. Implement the Worker

```go
type ChildWorker struct {
    identity fsmv2.Identity
    logger   *zap.SugaredLogger
}

func (w *ChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Return current system state
}

func (w *ChildWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    // Child workers are leaf nodes - return nil ChildrenSpecs
    return config.DesiredState{
        State:         "connected",
        ChildrenSpecs: nil,  // KEY: Leaf nodes have no children
    }, nil
}

func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
    return &state.ChildStoppedState{}
}
```

### 3. Register with Factory

```go
// factory_registration.go
func init() {
    factory.RegisterFactory[ChildObservedState, *ChildDesiredState](
        func(identity fsmv2.Identity) fsmv2.Worker {
            return NewChildWorker(identity.ID, identity.Name, nil)
        })

    factory.RegisterSupervisorFactory[ChildObservedState, *ChildDesiredState](
        func(cfg interface{}) interface{} {
            return supervisor.NewSupervisor[ChildObservedState, *ChildDesiredState](
                cfg.(supervisor.Config))
        })
}
```

## Usage with Generic Root Package

```go
import (
    "github.com/.../pkg/fsmv2/root"
    _ "github.com/.../pkg/fsmv2/workers/example/root_supervisor"  // Register factories
)

func main() {
    sup, err := root.NewRootSupervisor(root.SupervisorConfig{
        ID:     "root-001",
        Name:   "Example Root",
        Store:  store,
        Logger: logger,
        YAMLConfig: `
children:
  - name: "child-1"
    workerType: "example/root_supervisor/snapshot.ChildObservedState"
    userSpec:
      config: |
        connection_timeout: 5s
`,
    })

    done := sup.Start(ctx)
    <-done
}
```

The `workerType` string is derived from the `ChildObservedState` type using `storage.DeriveWorkerType[T]()`.

## Key Concepts

### Leaf Workers vs Branch Workers

- **Leaf workers** (like ChildWorker) return `nil` for `ChildrenSpecs`
- **Branch workers** return a `[]config.ChildSpec` to specify their children

This example shows a leaf worker. For branch workers that manage their own children, see how the passthrough root worker parses YAML to return `ChildrenSpecs`.

### Factory Registration

Import the package to trigger automatic registration:

```go
import _ "github.com/.../root_supervisor"  // Blank import registers factories
```

In tests, use `factory.ResetRegistry()` before registering to ensure a clean state.

## Integration Tests

The `integration_test.go` file demonstrates:

1. **Basic supervisor creation** - Creating a root supervisor with children
2. **Multiple children** - Managing several child workers
3. **Dynamic child creation** - How `reconcileChildren()` creates child supervisors
4. **Store setup** - Creating required collections for each worker type

Run tests with:

```bash
go test ./pkg/fsmv2/workers/example/root_supervisor/...
```

## Related Documentation

- **[pkg/fsmv2/root](../../root/README.md)** - Generic passthrough root package documentation
- **pkg/fsmv2/factory** - Worker and supervisor factory system
- **pkg/fsmv2/supervisor** - Supervisor implementation with `reconcileChildren()`
