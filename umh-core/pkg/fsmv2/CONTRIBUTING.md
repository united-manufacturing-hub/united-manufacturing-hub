# Contributing to FSMv2

Guidelines for contributing to the FSMv2 framework.

## Development Setup

```bash
# Clone and navigate
cd pkg/fsmv2

# Run tests
ginkgo -r .

# Run with race detection
ginkgo -r -race .

# Run specific tests
ginkgo run --focus="State transitions" -v .

# Run local scenarios
go run cmd/runner/main.go --scenario=simple
```

## Code Style

### Naming Conventions

| Item | Convention | Example |
|------|------------|---------|
| State names | lowercase_snake_case | `stopped`, `trying_to_start`, `running` |
| Action names | PascalCase | `StartProcessAction`, `AuthenticateAction` |
| Log messages | snake_case | `"action_failed"`, `"state_transition"` |
| Metrics | snake_case with suffix | `_total` for counters, `_seconds` for durations |

### State Implementation Pattern

```go
type MyState struct{}

func (s MyState) String() string { return "my_state" }
func (s MyState) Reason() string { return "description of why in this state" }

func (s MyState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    desired := snap.Desired.(MyDesiredState)
    observed := snap.Observed.(MyObservedState)

    // 1. ALWAYS check shutdown first
    if desired.IsShutdownRequested() {
        return StoppedState{}, fsmv2.SignalNone, nil
    }

    // 2. Check observations for state transitions
    if observed.SomeCondition {
        return NewState{}, fsmv2.SignalNone, nil
    }

    // 3. Emit action if needed (stay in same state)
    return s, fsmv2.SignalNone, &SomeAction{}
}
```

### Action Implementation Pattern

```go
type MyAction struct{}

func (a *MyAction) Name() string { return "MyAction" }

func (a *MyAction) Execute(ctx context.Context, deps any) error {
    d := deps.(MyDependencies)

    // 1. Check if already done (idempotency)
    if d.AlreadyDone() {
        return nil
    }

    // 2. Perform the actual operation
    return d.DoSomething(ctx)
}
```

## Testing Requirements

### Unit Tests

Every new state and action must have tests:

```go
var _ = Describe("MyState", func() {
    It("should check shutdown first", func() {
        snap := fsmv2.Snapshot{
            Desired: &MyDesiredState{ShutdownRequested: true},
            Observed: &MyObservedState{},
        }
        state := MyState{}
        nextState, signal, action := state.Next(snap)

        Expect(nextState).To(BeAssignableToTypeOf(StoppedState{}))
        Expect(action).To(BeNil())
    })

    It("should transition on condition", func() {
        snap := fsmv2.Snapshot{
            Desired: &MyDesiredState{},
            Observed: &MyObservedState{SomeCondition: true},
        }
        state := MyState{}
        nextState, _, _ := state.Next(snap)

        Expect(nextState).To(BeAssignableToTypeOf(NewState{}))
    })
})
```

### Integration Tests

For complex worker interactions, add integration tests in `integration/`:

```go
var _ = Describe("MyWorker Integration", func() {
    It("should complete full lifecycle", func() {
        // Setup supervisor with test dependencies
        // Verify state transitions over time
    })
})
```

### Architecture Tests

Run to verify patterns:

```bash
ginkgo run --focus="Architecture" -v .
```

## Pull Request Checklist

Before submitting:

- [ ] All tests pass: `ginkgo -r -race .`
- [ ] Linting passes: `golangci-lint run .`
- [ ] No focused tests: `ginkgo -r --fail-on-focused .`
- [ ] State names are lowercase_snake_case
- [ ] Actions are idempotent
- [ ] Shutdown is checked first in all states
- [ ] Documentation updated if API changed
- [ ] Error messages are static (for Sentry grouping)

## Error Message Guidelines

For proper Sentry grouping, keep error messages static:

```go
// GOOD: Static message, dynamic values in log
s.logger.Errorw("worker not found",
    "worker_id", workerID,
    "operation", "restart")
return fmt.Errorf("worker not found")

// BAD: Dynamic values in error message
return fmt.Errorf("worker %s not found", workerID)
```

## Adding a New Worker

1. Create directory structure (see README.md Quick Start)
2. Implement `Worker` interface (3 methods)
3. Register with `fsmv2.RegisterWorkerFactory()`
4. Add unit tests for all states and actions
5. Add integration test for lifecycle
6. Update documentation

## Debugging

### Local Runner

```bash
# List scenarios
go run cmd/runner/main.go --list

# Run with debug logging
go run cmd/runner/main.go --scenario=myworker --log-level=debug
```

### Common Issues

| Symptom | Likely Cause | Solution |
|---------|--------------|----------|
| State stuck | Action failing | Check action logs, verify dependencies |
| Observation stale | Collector error | Check CollectObservedState error handling |
| Won't shutdown | Missing check | Add IsShutdownRequested() to all states |
| Action repeats | Not idempotent | Add "already done" check |

## Getting Help

- Read `doc.go` for API contracts
- Check `README.md` for concepts
- Look at `workers/communicator/` for reference implementation
- Run architecture tests for pattern validation
