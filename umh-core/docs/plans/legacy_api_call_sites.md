# Legacy API Call Sites Inventory

Generated: 2025-01-14

## SaveObserved Calls

Total: **2 call sites**

1. `pkg/fsmv2/supervisor/supervisor.go:605`
   - Context: Supervisor saving observed state snapshot
   - Worker type: Dynamic (s.workerType)
   - Data: observedDoc (Document)
   - Migration complexity: Medium (needs type registry)

2. `pkg/fsmv2/supervisor/collection/collector.go:221`
   - Context: Collector saving observed state
   - Worker type: Dynamic (c.config.WorkerType)
   - Data: observed (interface{})
   - Migration complexity: Medium (needs type registry)

## LoadObserved Calls

Total: **0 call sites**

No LoadObserved calls found in production code.

## SaveDesired Calls

Total: **3 call sites**

1. `pkg/fsmv2/supervisor/supervisor.go:628`
   - Context: Supervisor setting desired state
   - Worker type: Dynamic (s.workerType)
   - Data: desiredDoc (Document)
   - Migration complexity: Medium (needs type registry)

2. `pkg/fsmv2/supervisor/supervisor.go:1565`
   - Context: Supervisor updating desired state
   - Worker type: Dynamic (s.workerType)
   - Data: desiredDoc (Document)
   - Migration complexity: Medium (needs type registry)

3. `pkg/fsmv2/supervisor/supervisor.go:1715`
   - Context: Supervisor creating child worker desired state
   - Worker type: Dynamic (spec.WorkerType)
   - Data: desiredDoc (Document)
   - Migration complexity: Medium (needs type registry)

## LoadDesired Calls

Total: **1 call site**

1. `pkg/fsmv2/supervisor/supervisor.go:1544`
   - Context: Supervisor loading desired state for validation
   - Worker type: Dynamic (s.workerType)
   - Data: returns interface{}
   - Migration complexity: Medium (needs type registry)

## Migration Priority

Total call sites to migrate: **6**

### High Priority (Frequent Operations)

1. **SaveObserved** (2 calls) - Executed on every state observation (high frequency)
   - supervisor.go:605 - Snapshot creation
   - collector.go:221 - Collector state save

2. **SaveDesired** (3 calls) - Executed on configuration changes (medium frequency)
   - supervisor.go:628 - Initial desired state set
   - supervisor.go:1565 - Desired state update
   - supervisor.go:1715 - Child worker desired state

### Medium Priority (Infrequent Operations)

3. **LoadDesired** (1 call) - Executed during validation/reads (low-medium frequency)
   - supervisor.go:1544 - Desired state validation

## Migration Strategy

All call sites use **dynamic workerType** (string passed via config), but generic APIs need **compile-time type parameters**.

### Solution: Type Registry Pattern

Phase 5 will implement a type registry that maps `workerType` (string) to Go type at compile time:

```go
// Supervisor initialization
supervisor := NewSupervisor(cfg)
supervisor.RegisterObservedType(ContainerObservedState{})

// Later, in runtime code:
// Registry dispatches to SaveObservedTyped[ContainerObservedState] based on workerType
```

This allows:
- Dynamic workerType configuration (maintains flexibility)
- Type-safe storage operations (compile-time guarantees)
- Gradual migration (legacy API fallback during transition)

## Migration Notes

- All call sites are in supervisor package (centralized migration)
- No LoadObserved calls (simplifies migration)
- SaveDesired has 3 call sites (all similar patterns)
- LoadDesired has 1 call site (straightforward migration)
- Migration can be done incrementally with type registry fallback

## Next Steps (Phase 5)

1. Add type registry to Supervisor struct
2. Implement RegisterObservedType() and RegisterDesiredType() methods
3. Create generic save/load helpers that use registry
4. Migrate call sites one by one
5. Add tests for each migrated call site
6. Remove fallback to legacy APIs once all types registered
