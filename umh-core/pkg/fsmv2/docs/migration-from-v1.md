# Migrating from FSM v1 to FSM v2

## Quick Reference (Rosetta Stone)

| FSM v1 Concept | FSM v2 Equivalent |
|----------------|-------------------|
| String state constants (`"running"`) | Struct types (`RunningState{}`) |
| `fsm.Event()` triggers | `State.Next()` returns |
| looplab/fsm library | Pure Go with generics |
| Reconcile detects external state | Collector populates ObservedState |
| Single machine instance | Worker + Supervisor pattern |
| Manual state tracking | Triangle Model (Identity/Desired/Observed) |

## Key Differences

1. **States are types, not strings** - Compiler catches invalid transitions
2. **Pure state functions** - `State.Next()` has no side effects, just returns next state
3. **Explicit actions** - Side effects are separate Action structs with `Execute()`
4. **Single-threaded tick loop** - Supervisor orchestrates, no race conditions

## Resources

- **API Reference**: `pkg/fsmv2/api.go` (core interfaces)
- **Package Documentation**: `pkg/fsmv2/doc.go` (overview and patterns)
- **README**: `pkg/fsmv2/README.md` (Triangle Model diagram)
- **Example Workers**: `pkg/fsmv2/workers/example/` (reference implementations)
