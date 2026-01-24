# FSM v2 factory package

The factory package provides registration mechanisms for FSM v2 workers and supervisors.

## Worker type derivation

Worker types are **derived from Go type names**, not manually specified.

```
ExamplechildObservedState    → "examplechild"
ApplicationObservedState → "application"
ExampleparentObservedState → "exampleparent"
```

**Derivation rules:**
1. Strip `ObservedState` or `DesiredState` suffix
2. Lowercase the result

**Constraint:** Go type names cannot contain hyphens. Worker types like `"example-child"` cannot be derived from any type name, so folder names must match worker types exactly.

## Registries

The factory package maintains two separate registries:

1. **Worker Registry** (`registry`): Maps worker type → worker factory function
2. **Supervisor Registry** (`supervisorRegistry`): Maps worker type → supervisor factory function

These are separate because they have different function signatures and the `interface{}` return type in supervisor factory avoids circular imports.

**Invariant:** Every worker type must be registered in BOTH registries with the SAME key. An architecture test enforces this.

## Registration functions

### `RegisterWorkerType` (preferred)

Registers both factories atomically with automatic type derivation:

```go
func init() {
    // Worker type is automatically derived from ExamplechildObservedState → "examplechild"
    if err := factory.RegisterWorkerType[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
        func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
            worker, _ := NewChildWorker(id, pool, logger)
            return worker
        },
        func(cfg interface{}) interface{} {
            return supervisor.NewSupervisor[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
                cfg.(supervisor.Config))
        },
    ); err != nil {
        panic(err)
    }
}
```

Benefits:
- Derives worker type from the generic type parameter
- Registers both factories atomically
- Rolls back on partial failure
- Prevents mismatched keys

### `RegisterWorkerAndSupervisorFactoryByType`

Use when you need an explicit type string:

```go
func init() {
    workerType, _ := storage.DeriveWorkerType[snapshot.ExamplechildObservedState]()

    err := factory.RegisterWorkerAndSupervisorFactoryByType(
        workerType,
        func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
            return NewChildWorker(id, logger)
        },
        func(raw interface{}) interface{} {
            return supervisor.NewSupervisor[*snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](raw)
        },
    )
    if err != nil {
        panic(err)
    }
}
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
workerTypes := factory.ListWorkerTypes()
supervisorTypes := factory.ListSupervisorTypes()
```

## Folder naming convention

**Invariant: Folder name must equal derived worker type.**

Architecture tests in `architecture_test.go` enforce this.

| Folder | Type Name | Derived Worker Type | Valid? |
|--------|-----------|---------------------|--------|
| `examplechild` | `ExamplechildObservedState` | `"examplechild"` | Yes |
| `exampleparent` | `ExampleparentObservedState` | `"exampleparent"` | Yes |
| `example-child` | ??? | Cannot match | **No** |

If you create a folder `foo`, your types must be named `FooObservedState` and `FooDesiredState`.

## Common mistakes

### Manual string mismatch

**Wrong:**
```go
// supervisor.go derives "parent" from ParentObservedState
_ = factory.RegisterSupervisorFactoryByType("parent", ...)

// worker.go uses explicit string "example-parent"
_ = factory.RegisterFactoryByType("example-parent", ...)
```

**Result:** `no supervisor factory registered for worker type: example-parent`

**Fix:** Use `RegisterWorkerType[TObserved, TDesired]()` which derives the key automatically.

### Hyphenated folder names

**Wrong:** Folder `example-child` with type `ExamplechildObservedState`
- Derived type: `"examplechild"`
- Expected by code: `"example-child"`
- Architecture test: **FAILS**

**Fix:** Rename folder to match derived type (`examplechild`), or use underscores/no separators in folder name.

### Type name doesn't match folder

**Wrong:** Folder `myworker` with type `SomethingElseObservedState`
- Derived type: `"somethingelse"`
- Folder: `"myworker"`
- Architecture test: **FAILS**

**Fix:** Rename type to `MyworkerObservedState` or folder to `somethingelse`.

## Architecture tests

### Folder naming validation

The `ValidateFolderMatchesWorkerType` test validates:
- Every worker folder contains a snapshot with `*ObservedState` type
- The derived worker type equals the folder name

Run with: `ginkgo --focus="Worker Folder Naming" ./pkg/fsmv2/`

### Registry consistency validation

The registry consistency test validates:
- Every worker type in the worker registry exists in the supervisor registry
- Every worker type in the supervisor registry exists in the worker registry

The test catches mismatches caused by using different keys when registering worker vs supervisor factories, or forgetting to register one of the two factories.

Run with: `ginkgo --focus="Worker Factory Registration" ./pkg/fsmv2/`

If this test fails, a `REGISTRY_MISMATCH` violation indicates which types are missing from which registry.
