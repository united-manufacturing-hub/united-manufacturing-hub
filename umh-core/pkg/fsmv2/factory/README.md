# FSM v2 factory package

The factory package provides low-level registration mechanisms for FSM v2 workers and supervisors. Most workers register via the high-level [`register.Worker`](../register/register.go) one-liner, which wraps the factory primitives documented here.

## Worker type derivation

Worker types are the string keys stored in config YAML and CSE storage. They are derived automatically from Go type names when using `register.Worker[TConfig, TStatus, TDeps]`: the framework builds `Observation[TStatus]` / `WrappedDesiredState[TConfig]` wrappers internally, and the worker type string is the explicit first argument to `register.Worker`. The folder name must match that string exactly.

**Constraint:** Go type names cannot contain hyphens. Worker types like `"example-child"` cannot be derived from any type name, so folder names must match worker types exactly (no hyphens).

## Registries

The factory package maintains two separate registries:

1. **Worker Registry** (`registry`): Maps worker type → worker factory function
2. **Supervisor Registry** (`supervisorRegistry`): Maps worker type → supervisor factory function

These are separate because they have different function signatures and the `interface{}` return type in supervisor factory avoids circular imports.

**Invariant:** Every worker type must be registered in BOTH registries with the SAME key. An architecture test enforces this.

## Registration functions

### `register.Worker` (one-line API, preferred)

Defined in the sibling [`register`](../register/) package. Registers worker factory, supervisor factory, and CSE `TypeRegistry` atomically:

```go
func init() {
    register.Worker[HelloworldConfig, HelloworldStatus, register.NoDeps](
        "helloworld", NewHelloworldWorker)
}
```

Benefits:
- Single call wires factory + supervisor + CSE types
- Panics at init time on duplicate or collision (fail-fast)
- Use `register.NoDeps` for zero-dep workers; parameterize with a struct for typed deps

See [`register/register.go`](../register/register.go) for the full contract.

### Low-level functions (tests and framework internals only)

Individual registration functions, used by `register.Worker` internally and by tests that need direct access:

```go
// Worker factory registration
factory.RegisterFactoryByType(workerType, workerFactory)

// Supervisor factory registration
factory.RegisterSupervisorFactoryByType(workerType, supervisorFactory)
```

**Warning:** Using these directly in production code can lead to mismatches if different keys are used. The architecture test catches such mismatches. Always prefer `register.Worker` in worker packages.

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

**Invariant: Folder name must equal the worker type string passed to `register.Worker`.**

Architecture tests in `architecture_test.go` enforce this.

| Folder | Type Name | Worker Type | Valid? |
|--------|-----------|-------------|--------|
| `helloworld` | `HelloworldConfig` / `HelloworldStatus` | `"helloworld"` | Yes |
| `example-failing` | any | Cannot match (hyphen) | **No** |

If you create a folder `foo`, pass `"foo"` to `register.Worker` and name your Go types `FooConfig` / `FooStatus`.

## Common mistakes

### Manual string mismatch

**Wrong:** mixing low-level registration functions with inconsistent keys:
```go
_ = factory.RegisterSupervisorFactoryByType("failing", ...)
_ = factory.RegisterFactoryByType("example-failing", ...)
```

**Result:** `no supervisor factory registered for worker type: example-failing`

**Fix:** Use `register.Worker[TConfig, TStatus, TDeps]("type", constructor)` which registers both registries with one key.

### Hyphenated folder names

**Wrong:** Folder `example-failing` with worker type `"example-failing"`
- Folder contains a hyphen
- Go type names cannot contain hyphens, so derivation-based conventions break
- Architecture test: **FAILS**

**Fix:** Rename folder to drop the hyphen (e.g. `examplefailing`), then pass the same string to `register.Worker`.

### Type name doesn't match folder

**Wrong:** Folder `myworker` with registered type string `"somethingelse"`
- Architecture test: **FAILS**

**Fix:** Keep folder name, worker type string, and the `<Type>Config` / `<Type>Status` Go type prefix aligned.

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
