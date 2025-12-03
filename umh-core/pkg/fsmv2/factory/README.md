# FSM v2 Factory Package

The factory package provides registration mechanisms for FSM v2 workers and supervisors.

## Key Concept: Worker Type Derivation

Worker types are **derived from Go type names**, not manually specified. This prevents registration mismatches.

```
ExamplechildObservedState    → "examplechild"
ApplicationObservedState → "application"
ExampleparentObservedState → "exampleparent"
```

**Derivation rules:**
1. Strip `ObservedState` or `DesiredState` suffix
2. Lowercase the result

**Critical constraint:** Go type names cannot contain hyphens. Therefore, worker types like `"example-child"` are **impossible** to derive from any type name. This is why folder names must match worker types exactly.

## Two Registries: Why and How

The factory package maintains two separate registries:

1. **Worker Registry** (`registry`): Maps worker type → worker factory function
2. **Supervisor Registry** (`supervisorRegistry`): Maps worker type → supervisor factory function

These are separate because:
- They have different function signatures
- The `interface{}` return type in supervisor factory avoids circular imports
- Both are required for the FSM system to function correctly

**Critical invariant:** Every worker type must be registered in BOTH registries with the SAME key.
An architecture test enforces this invariant.

## Registration Functions

### Preferred: `RegisterWorkerType` (Generic, Type-Safe)

Use `RegisterWorkerType` to register both factories atomically with automatic type derivation:

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
- Worker type is derived from the generic type parameter (no manual strings)
- Registers both factories atomically
- Rolls back on partial failure
- **Impossible to have mismatched keys**

### Alternative: `RegisterWorkerAndSupervisorFactoryByType` (Explicit String)

Use when you need an explicit type string (rare):

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

### Low-Level Functions (Tests Only)

Individual registration functions exist for testing purposes:

```go
// Worker factory registration
factory.RegisterFactoryByType(workerType, workerFactory)

// Supervisor factory registration
factory.RegisterSupervisorFactoryByType(workerType, supervisorFactory)
```

**Warning:** Using these in production code can lead to mismatches if different keys are used.
The architecture test will catch such mismatches.

## Validation Functions

### Check Registry Consistency

```go
workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
if len(workerOnly) > 0 || len(supervisorOnly) > 0 {
    // Mismatched registrations detected
}
```

### List Registered Types

```go
workerTypes := factory.ListWorkerTypes()
supervisorTypes := factory.ListSupervisorTypes()
```

## Folder Naming Convention

**Invariant: Folder name MUST equal derived worker type.**

This is enforced by architecture tests in `architecture_test.go`.

| Folder | Type Name | Derived Worker Type | Valid? |
|--------|-----------|---------------------|--------|
| `examplechild` | `ExamplechildObservedState` | `"examplechild"` | Yes |
| `exampleparent` | `ExampleparentObservedState` | `"exampleparent"` | Yes |
| `example-child` | ??? | Cannot match | **No** |

If you create a folder `foo`, your types must be named `FooObservedState` and `FooDesiredState`.

## Common Mistakes

### 1. Manual String Mismatch

**Wrong:**
```go
// supervisor.go derives "parent" from ParentObservedState
_ = factory.RegisterSupervisorFactoryByType("parent", ...)

// worker.go uses explicit string "example-parent"
_ = factory.RegisterFactoryByType("example-parent", ...)
```

**Result:** `no supervisor factory registered for worker type: example-parent`

**Fix:** Use `RegisterWorkerType[TObserved, TDesired]()` which derives the key automatically.

### 2. Hyphenated Folder Names

**Wrong:** Folder `example-child` with type `ExamplechildObservedState`
- Derived type: `"examplechild"`
- Expected by code: `"example-child"`
- Architecture test: **FAILS**

**Fix:** Rename folder to match derived type (`examplechild`), or use underscores/no separators in folder name.

### 3. Type Name Doesn't Match Folder

**Wrong:** Folder `myworker` with type `SomethingElseObservedState`
- Derived type: `"somethingelse"`
- Folder: `"myworker"`
- Architecture test: **FAILS**

**Fix:** Rename type to `MyworkerObservedState` or folder to `somethingelse`.

## Architecture Tests

### Folder Naming Validation

The architecture test `ValidateFolderMatchesWorkerType` automatically validates:
- Every worker folder contains a snapshot with `*ObservedState` type
- The derived worker type equals the folder name

Run with: `ginkgo --focus="Worker Folder Naming" ./pkg/fsmv2/`

### Registry Consistency Validation

The architecture test for registry consistency validates:
- Every worker type in the worker registry also exists in the supervisor registry
- Every worker type in the supervisor registry also exists in the worker registry

This catches mismatches caused by:
- Using different keys when registering worker vs supervisor factories
- Forgetting to register one of the two factories

Run with: `ginkgo --focus="Worker Factory Registration" ./pkg/fsmv2/`

If this test fails, you'll see a `REGISTRY_MISMATCH` violation indicating which types are missing from which registry.
