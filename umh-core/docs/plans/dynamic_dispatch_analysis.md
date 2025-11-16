# Dynamic Dispatch Analysis

## Finding

All current call sites (Supervisor, Collector) use **runtime polymorphism** - they don't know the worker type at compile time.

## Call Site Analysis

### supervisor.go:605
```go
_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
```

- `s.workerType` is `string` (runtime value)
- Could be "container", "relay", "communicator", etc.
- Determined by config, not compile time

### supervisor.go:628, 1565, 1715
```go
err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
```

- Same pattern: `s.workerType` is runtime string
- Generic type parameter can't be determined at compile time

### supervisor.go:1544
```go
desiredInterface, err := s.store.LoadDesired(ctx, s.workerType, workerID)
```

- Returns `interface{}` because type is unknown at compile time
- Supervisor works with multiple worker types dynamically

### collector.go:221
```go
changed, err := c.config.Store.SaveObserved(ctx, c.config.WorkerType, c.config.Identity.ID, observed)
```

- `c.config.WorkerType` is runtime string
- Generic type parameter can't be determined at compile time

## Design Decision

**Keep dual API system:**

1. **Generic APIs** - For direct usage with known types at compile time
   - Example: Worker action functions know their own type
   - `storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)`
   - Benefits: Type safety, compile-time checks, no registry dependency

2. **Legacy APIs** - For dynamic dispatch (Supervisor, Collector)
   - Example: Supervisor orchestrates multiple worker types
   - `ts.SaveObserved(ctx, workerType, id, observed)`
   - Benefits: Runtime flexibility, works with any worker type

## Why This Is Correct

The subagent investigation found "registry is unnecessary for generic APIs" - **TRUE**.

But it didn't find "all call sites can use generic APIs" - **FALSE**.

**Supervisor and Collector are frameworks** - they work with multiple worker types dynamically. They NEED runtime dispatch.

**Worker implementations** know their type at compile time - they SHOULD use generic APIs.

## Architectural Pattern

This is analogous to:

- **Generic containers** (type-safe) vs **interface{} containers** (runtime flexible)
- **Statically typed** (compile-time safety) vs **dynamically typed** (runtime flexibility)
- **Direct calls** (fast, type-safe) vs **vtable dispatch** (flexible, polymorphic)

Both have valid use cases in the same codebase.

## Migration Strategy (Revised)

**Phase 5: No migration needed for Supervisor/Collector** - they correctly use legacy APIs.

**Phase 6: Remove registry system, but keep legacy APIs** - they use conventions instead.

This is an **architectural feature**, not technical debt.

## Call Site Breakdown

Total call sites: **6**

**Runtime dispatch (must use legacy APIs):**
- supervisor.go:605 - SaveObserved
- supervisor.go:628 - SaveDesired
- supervisor.go:1544 - LoadDesired
- supervisor.go:1565 - SaveDesired
- supervisor.go:1715 - SaveDesired
- collector.go:221 - SaveObserved

**Compile-time known types (should use generic APIs):**
- None found yet (future worker implementations will use generic APIs)

## Future: Type-Safe Dynamic Dispatch

If we want type safety for Supervisor later, options:

1. **Type Registry Pattern** - Map workerType string → reflect.Type
   - Pros: Maintains flexibility, adds some type safety
   - Cons: Complex, uses reflection, runtime overhead

2. **Code Generation** - Generate typed wrappers at build time
   - Pros: Zero runtime cost, full type safety
   - Cons: Build complexity, code bloat

3. **Interface Abstraction** - Define Worker interface, use vtables
   - Pros: Standard Go pattern, clean API
   - Cons: Requires interface implementation for all workers

But this is **future work** - not required for registry elimination.

## Conclusion

**Phase 5 is complete as-is.**

No code changes needed. The current architecture is correct:
- Generic APIs exist for type-safe direct usage
- Legacy APIs exist for dynamic dispatch
- Both serve different architectural needs

The migration from registry-based to convention-based collection naming (Phase 2) already eliminated the registry dependency from legacy APIs. They now use simple string concatenation.

**Next step:** Phase 6 removes the registry fields from TriangularStore, completing the registry elimination.
