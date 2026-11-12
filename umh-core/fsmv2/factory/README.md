# FSM v2 factory package

The factory package holds the registries that map a worker type to its worker factory and its supervisor factory. The worker type is a string such as `"examplechild"`. Config YAML and storage use the same string.

## Registering a worker

Workers do not call this package directly. They register in `init()` with `register.Worker` from the `register` package:

```go
func init() {
    register.Worker[ExamplechildConfig, ExamplechildStatus, *ExamplechildDependencies]("examplechild",
        func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
            return NewChildWorker(id, &DefaultConnectionPool{}, logger, sr)
        })
}
```

`register.Worker` calls `RegisterWorkerAndSupervisorFactoryByType` with the worker type you pass. It also registers the observed and desired types for storage. The `register` package doc covers workers without dependencies (`register.NoDeps`) and dependencies a parent passes to its children.

Use the worker's folder name as the worker type, as `workers/example/examplechild/` does.

## Registries

The factory package maintains two separate registries:

1. **Worker Registry** (`registry`): Maps worker type → worker factory function
2. **Supervisor Registry** (`supervisorRegistry`): Maps worker type → supervisor factory function

These are separate because they have different function signatures and the `interface{}` return type in supervisor factory avoids circular imports.

**Invariant:** Every worker type must be registered in BOTH registries with the SAME key. An architecture test enforces this.

## Registration functions

### `RegisterWorkerAndSupervisorFactoryByType`

Registers both factories under an explicit worker type. `register.Worker` uses it. If the supervisor factory fails to register, it removes the worker factory it just registered.

```go
err := factory.RegisterWorkerAndSupervisorFactoryByType(
    "myworker",
    func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) fsmv2.Worker {
        return NewMyWorker(id, logger, sr)
    },
    func(cfg interface{}) interface{} {
        return supervisor.NewSupervisor[fsmv2.Observation[MyStatus], *fsmv2.WrappedDesiredState[MyConfig]](
            cfg.(supervisor.Config))
    },
)
```

### `RegisterWorkerType`

Registers both factories for a worker with its own ObservedState type. It derives the worker type from that type's name: it strips the `ObservedState` suffix and lowercases the rest (`MyworkerObservedState` → `"myworker"`). No worker in `workers/` uses it today; a worker that returns `fsmv2.NewObservation` uses `register.Worker`.

```go
err := factory.RegisterWorkerType[snapshot.MyworkerObservedState, *snapshot.MyworkerDesiredState](
    func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) fsmv2.Worker {
        return NewMyWorker(id, logger, sr)
    },
    func(cfg interface{}) interface{} {
        return supervisor.NewSupervisor[snapshot.MyworkerObservedState, *snapshot.MyworkerDesiredState](
            cfg.(supervisor.Config))
    },
)
```

### Low-level functions (tests only)

Individual registration functions for testing:

```go
// Worker factory registration
factory.RegisterFactoryByType(workerType, workerFactory)

// Supervisor factory registration
factory.RegisterSupervisorFactoryByType(workerType, supervisorFactory)
```

**Warning:** Using these in production code can lead to mismatches if different keys are used. The architecture test catches such mismatches.

## Validation functions

### Check registry consistency

```go
workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
if len(workerOnly) > 0 || len(supervisorOnly) > 0 {
    // Mismatched registrations detected
}
```

### List registered types

```go
workerTypes := factory.ListRegisteredTypes()
supervisorTypes := factory.ListSupervisorTypes()
```

## Architecture tests

### Folder naming validation

`ValidateFolderMatchesWorkerType` reads each `snapshot.go` under `workers/` (except `workers/communicator/`). For every type named `*ObservedState`, it derives the worker type from the name and checks that it equals the worker's folder name. A worker registered with `register.Worker` declares no such type, so for it the check finds nothing; folder name = worker type is then a convention.

Run with: `ginkgo --focus="Worker Folder Naming" ./pkg/fsmv2/`

### Registry consistency validation

The registry consistency test validates:
- Every worker type in the worker registry exists in the supervisor registry
- Every worker type in the supervisor registry exists in the worker registry

The test catches mismatches caused by using different keys when registering worker vs supervisor factories, or forgetting to register one of the two factories.

Run with: `ginkgo --focus="Worker Factory Registration" ./pkg/fsmv2/`

If this test fails, a `REGISTRY_MISMATCH` violation indicates which types are missing from which registry.
