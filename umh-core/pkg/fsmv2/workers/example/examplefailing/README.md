# Example Failing Worker

This worker demonstrates predictable failure scenarios for testing FSM error handling and retry behavior.

## Purpose

The `example-failing` worker is a test fixture that simulates connection failures in a controlled manner. It's useful for:

- Testing FSM error handling and retry logic
- Demonstrating how actions can fail and recover
- Validating supervisor behavior during failures
- Creating reproducible failure scenarios for integration tests

## Configuration

The worker is configured via YAML with a `ShouldFail` boolean flag:

```yaml
workerType: "failing"
config:
  shouldFail: true  # When true, connect action will fail
```

## Behavior

### When `shouldFail: false` (default)

The worker behaves like a normal connection worker:

1. Starts in `stopped` state
2. Transitions to `trying_to_connect`
3. Connect action succeeds
4. Reaches `connected` state

### When `shouldFail: true`

The worker simulates connection failures:

1. Starts in `stopped` state
2. Transitions to `trying_to_connect`
3. Connect action returns error: "simulated connection failure"
4. FSM retries with exponential backoff
5. Remains in `trying_to_connect` state until `shouldFail` is changed to `false`

## Usage Example

```go
// Create a failing worker that will fail connections
worker := example_failing.NewFailingWorker(
    "test-worker-1",
    "Test Failing Worker",
    connectionPool,
    logger,
)

// Register with supervisor
supervisor.AddWorker(worker, fsmv2types.UserSpec{
    Config: `shouldFail: true`,
})

// Worker will repeatedly fail connect attempts

// Later, to allow connection to succeed:
supervisor.UpdateWorker(worker.Identity.ID, fsmv2types.UserSpec{
    Config: `shouldFail: false`,
})
```

## Factory Registration

The worker is automatically registered with the factory system:

- Worker type: `"failing"`
- Supervisor factory for `FailingObservedState` and `FailingDesiredState`

Creating instances via YAML configuration in the Application worker.

## State Transitions

```
stopped → trying_to_connect → (if shouldFail=false) → connected
   ↑            ↓ (retry loop if shouldFail=true)
   └────────────┘
```

## Files Structure

```
example-failing/
├── worker.go              # Main worker implementation
├── userspec.go           # Configuration structure (ShouldFail flag)
├── dependencies.go       # Dependencies with failure flag management
├── snapshot/
│   └── snapshot.go       # Observed/desired state definitions
├── state/
│   ├── base.go          # Base state interface
│   ├── state_stopped.go
│   ├── state_trying_to_connect.go
│   ├── state_connected.go
│   ├── state_disconnected.go
│   └── state_trying_to_stop.go
└── action/
    ├── connect.go       # Connect action (fails when shouldFail=true)
    └── disconnect.go    # Disconnect action
```

## Key Implementation Details

1. **Thread-safe failure flag**: `FailingDependencies` uses mutex to protect the `shouldFail` flag
2. **Action failure simulation**: `ConnectAction.Execute()` checks the flag and returns error if set
3. **Configuration parsing**: `DeriveDesiredState()` parses YAML config and updates the failure flag
4. **Standard FSM pattern**: Follows the same state machine structure as production workers

## Testing FSM Behavior

This worker enables testing:

- **Retry mechanisms**: How many times does the FSM retry before giving up?
- **Backoff strategies**: Does exponential backoff work correctly?
- **Error propagation**: Are errors properly logged and reported?
- **State consistency**: Does the FSM maintain correct state during failures?
- **Recovery**: Can the worker recover when `shouldFail` changes to `false`?
