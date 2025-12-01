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

## Registration Functions

### Recommended: Combined Registration

Use `RegisterWorkerAndSupervisorFactoryByType` to register both factories atomically:

```go
func init() {
    workerType, _ := storage.DeriveWorkerType[snapshot.ExamplechildObservedState]()

    err := factory.RegisterWorkerAndSupervisorFactoryByType(
        workerType,
        func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
            return NewChildWorker(id, logger)
        },
        func(raw interface{}) interface{} {
            return helpers.NewTypedSupervisor[*snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](raw)
        },
    )
    if err != nil {
        panic(err)
    }
}
```

This function:
- Registers both worker factory and supervisor factory with the **same** key
- Rolls back on partial failure (atomic registration)
- Prevents mismatches between registries

### Low-Level Functions (Use with Caution)

If you need fine-grained control, use the individual registration functions:

```go
// Worker factory registration
factory.RegisterFactoryByType(workerType, workerFactory)

// Supervisor factory registration
factory.RegisterSupervisorFactoryByType(workerType, supervisorFactory)
```

**Warning:** Using these separately can lead to mismatches if different keys are used.

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
_ = factory.RegisterSupervisorFactory[snapshot.ParentObservedState, ...]

// worker.go uses explicit string "example-parent"
_ = factory.RegisterFactoryByType("example-parent", ...)
```

**Result:** `no supervisor factory registered for worker type: example-parent`

**Fix:** Use `RegisterWorkerAndSupervisorFactoryByType` with derived type.

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

## Architecture Test

The architecture test `ValidateFolderMatchesWorkerType` automatically validates:
- Every worker folder contains a snapshot with `*ObservedState` type
- The derived worker type equals the folder name

Run with: `ginkgo --focus="Worker Folder Naming" ./pkg/fsmv2/`
