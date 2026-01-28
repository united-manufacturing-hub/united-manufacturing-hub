# Example failing worker

Test fixture that simulates connection failures for validating FSM error handling and retry behavior.

## Purpose

- Test FSM error handling and retry logic
- Validate supervisor behavior during failures
- Create reproducible failure scenarios for integration tests

## Configuration

Configure the worker via YAML with a `ShouldFail` boolean flag:

```yaml
workerType: "failing"
config:
  shouldFail: true  # When true, connect action will fail
```

## Behavior

### When `shouldFail: false` (default)

The worker behaves like a standard connection worker:

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

## Usage example

```go
// Create a failing worker
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

// Allow connection to succeed
supervisor.UpdateWorker(worker.Identity.ID, fsmv2types.UserSpec{
    Config: `shouldFail: false`,
})
```

## Factory registration

The worker registers with the factory system:

- Worker type: `"failing"`
- Supervisor factory for `FailingObservedState` and `FailingDesiredState`

Create instances via YAML configuration in the Application worker.

## State transitions

```
stopped → trying_to_connect → (if shouldFail=false) → connected
   ↑            ↓ (retry loop if shouldFail=true)
   └────────────┘
```

## File structure

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

## Implementation details

1. **Thread-safe failure flag**: `FailingDependencies` uses mutex to protect the `shouldFail` flag
2. **Action failure simulation**: `ConnectAction.Execute()` checks the flag and returns an error if set
3. **Configuration parsing**: `DeriveDesiredState()` parses YAML config and updates the failure flag
4. **FSM pattern**: Follows the same state machine structure as production workers

## Testing FSM behavior

Use this worker to test:

- **Retry mechanisms**: FSM retry count before giving up
- **Backoff strategies**: Exponential backoff correctness
- **Error propagation**: Error logging and reporting
- **State consistency**: FSM state correctness during failures
- **Recovery**: Worker recovery when `shouldFail` changes to `false`
