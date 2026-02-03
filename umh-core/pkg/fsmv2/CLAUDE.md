# FSMv2 Development Guide

This file captures patterns and insights for working with the FSMv2 framework.

## Architecture Test Compliance

The `architecture_test.go` validates ALL workers via file-system scanning (not runtime reflection). Compliance must be satisfied from day one - tests will fail if any invariant is violated.

**Key invariants to follow:**

| Rule | Description |
|------|-------------|
| Empty State Structs | States have no fields (except embedded base) |
| Shutdown Check First | Check `IsShutdownRequested()` as FIRST conditional in `Next()` |
| State XOR Action | Return state OR action, never both |
| Single Type Assertion | `Next()` has exactly one type assertion at entry |
| Pure DeriveDesiredState | No dependency access - only use `spec` parameter |
| Context Cancellation | Handle `ctx.Done()` at entry in `CollectObservedState` |
| Pointer Receivers | Use `*WorkerType` for all Worker methods |

Run architecture tests after every change:
```bash
go test ./pkg/fsmv2/... -run "Architecture" -v
```

## Parent-Child Worker Pattern

Parent workers orchestrate children via `DeriveDesiredState` - they return `ChildrenSpecs` and the supervisor handles child spawning automatically. Parents don't manually create child workers.

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &config.DesiredState{
        BaseDesiredState: config.BaseDesiredState{State: "running"},
        ChildrenSpecs: []config.ChildSpec{
            {Name: "child1", WorkerType: "childworker", ...},
        },
    }, nil
}
```

Children aggregation (health counts) is handled by the supervisor, not in `CollectObservedState`. The supervisor calls `SetChildrenCounts()` after collection.

## Channel Singleton Pattern

For workers that share channels (like TransportWorker with Push/Pull children), use a singleton `ChannelProvider`:

```go
// Set before creating workers
transport.SetChannelProvider(provider)

// Dependencies get channels from singleton
func NewDependencies(...) *Dependencies {
    provider := GetChannelProvider()
    inbound, outbound := provider.GetChannels(identity.ID)
    // ...
}
```

This enables parent-child channel sharing without tight coupling.

## State Machine States

Each state file follows this pattern:

```go
type RunningState struct {
    helpers.BaseRunningState  // Embed exactly one base type
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
    snap := helpers.ConvertSnapshot[...](snapAny)  // Single type assertion

    // Shutdown check FIRST
    if snap.Desired.IsShutdownRequested() {
        return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested")
    }

    // Business logic...

    // Catch-all return at end
    return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Staying in running")
}
```

## Factory Registration

Workers register in `init()` with both worker and supervisor factories:

```go
func init() {
    if err := factory.RegisterWorkerType[ObservedState, *DesiredState](
        workerFactory,
        supervisorFactory,
    ); err != nil {
        panic(err)
    }
}
```

The folder name must match the worker type (e.g., `transport/` for type `"transport"`).

## Testing Patterns

- Use Ginkgo/Gomega for tests
- Test files use `package foo_test` (external black-box testing)
- Mock dependencies with interfaces
- Test context cancellation explicitly
- Verify architecture compliance continuously

```bash
# Run all tests for a worker
go test ./pkg/fsmv2/workers/transport/... -v

# Check coverage
go test ./pkg/fsmv2/workers/transport/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```
