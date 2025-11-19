# Root Package - Generic Passthrough Root Supervisor

Package `root` provides a generic passthrough root supervisor for FSMv2. The root supervisor dynamically creates children based on YAML configuration, allowing any registered worker type to be instantiated as a child.

## Overview

The passthrough pattern allows a root supervisor to manage children without hardcoding specific child types. The root worker parses YAML configuration to extract `ChildrenSpecs`, and the supervisor's `reconcileChildren()` automatically creates child supervisors using the factory registry.

This pattern is ideal when you need a root supervisor that:
- Manages multiple child worker types
- Configures children via YAML at runtime
- Doesn't need root-level business logic

## Key Components

### PassthroughWorker

The core worker type that parses YAML to discover children:

```go
type PassthroughWorker struct {
    id   string
    name string
}
```

Key method: `DeriveDesiredState()` parses YAML and returns `config.DesiredState` with `ChildrenSpecs`.

### State Types

- **PassthroughObservedState** - Minimal observed state tracking the deployed desired state
- **PassthroughDesiredState** - Embeds `config.DesiredState` for `ChildrenSpecs` and shutdown handling

### NewRootSupervisor

The main API for creating a root supervisor with a passthrough worker pre-configured:

```go
func NewRootSupervisor(cfg SupervisorConfig) (*supervisor.Supervisor[...], error)
```

This helper encapsulates the key pattern: root workers need explicit `AddWorker()`, while children are created automatically via `reconcileChildren()`.

## Usage

### 1. Register Child Worker Types

Child worker types must be registered with the factory before use. Registration typically happens in an `init()` function:

```go
func init() {
    // Register worker factory
    factory.RegisterFactory[MyChildObserved, *MyChildDesired](
        func(identity fsmv2.Identity) fsmv2.Worker {
            return NewMyChildWorker(identity.ID, identity.Name)
        })

    // Register supervisor factory
    factory.RegisterSupervisorFactory[MyChildObserved, *MyChildDesired](
        func(cfg interface{}) interface{} {
            return supervisor.NewSupervisor[MyChildObserved, *MyChildDesired](
                cfg.(supervisor.Config))
        })
}
```

### 2. Create Root Supervisor with YAML Config

```go
sup, err := root.NewRootSupervisor(root.SupervisorConfig{
    ID:     "root-001",
    Name:   "My Root Supervisor",
    Store:  myStore,
    Logger: myLogger,
    YAMLConfig: `
children:
  - name: "worker-1"
    workerType: "example-child"
    userSpec:
      config: |
        value: 10
  - name: "worker-2"
    workerType: "example-child"
    userSpec:
      config: |
        value: 20
`,
})
if err != nil {
    return err
}

// Start the supervisor
done := sup.Start(ctx)
```

### 3. YAML Configuration Format

```yaml
children:
  - name: "child-name"          # Unique identifier for the child
    workerType: "worker-type"   # Must match a registered factory type
    userSpec:
      config: |                 # Worker-specific configuration
        key: value
```

The `workerType` must match the type derived from the registered `ObservedState` type using `storage.DeriveWorkerType[T]()`.

## Factory Registration

### How It Works

1. Worker types register factories with `factory.RegisterFactory[O, D]()` and `factory.RegisterSupervisorFactory[O, D]()`
2. The type name is derived from the `ObservedState` type: `storage.DeriveWorkerType[MyObservedState]()` produces `"example/root_supervisor/snapshot.ChildObservedState"`
3. When the root supervisor's `reconcileChildren()` runs, it calls `factory.NewSupervisorByType()` to create child supervisors

### Auto-Registration via init()

The `factory_registration.go` file uses `init()` to automatically register factories when the package is imported:

```go
func init() {
    if err := factory.RegisterFactory[...](fn); err != nil {
        panic(...)
    }
    if err := factory.RegisterSupervisorFactory[...](fn); err != nil {
        panic(...)
    }
}
```

Simply importing the package registers its worker types with the global factory registry.

## Key Patterns

### Root Workers Need Explicit AddWorker()

Root workers must be added explicitly to the supervisor:

```go
sup := supervisor.NewSupervisor[O, D](cfg)
err := sup.AddWorker(identity, worker)  // Required for root
```

This is handled automatically by `NewRootSupervisor()`.

### Children Created Automatically

Child workers specified in `ChildrenSpecs` are created automatically by `reconcileChildren()`:

1. Root worker's `DeriveDesiredState()` returns `ChildrenSpecs`
2. Supervisor's reconciliation loop calls `reconcileChildren()`
3. For each spec, `factory.NewSupervisorByType()` creates the child supervisor
4. Child supervisor's `AddWorker()` is called with a new worker from `factory.NewWorkerByType()`

### State Storage Collections

The `TriangularStore` needs collections for each worker type:

```go
workerType := storage.DeriveWorkerType[MyObservedState]()
store.CreateCollection(ctx, workerType+"_identity", nil)
store.CreateCollection(ctx, workerType+"_desired", nil)
store.CreateCollection(ctx, workerType+"_observed", nil)
```

## Examples

See the `pkg/fsmv2/workers/example/root_supervisor` package for a complete example showing:
- Custom child worker types
- Factory registration
- Integration tests demonstrating the full flow

## Testing

The `integration_test.go` file in this package demonstrates:
- Setting up stores with required collections
- Registering factories in tests (using `factory.ResetRegistry()`)
- Creating root supervisors with YAML config
- Verifying child supervisor creation
