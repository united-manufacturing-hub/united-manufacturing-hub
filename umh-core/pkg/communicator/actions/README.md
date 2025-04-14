# UMH Action System

The actions package handles user-initiated operations within the UMH platform, providing a standardized way to implement commands triggered by users via the Management Console.

## Core Architecture

### Action Interface

Actions implement a common interface:

```go
type Action interface {
    Parse(payload interface{}) error
    Validate() error
    Execute() (interface{}, map[string]interface{}, error)
    getUserEmail() string
    getUuid() uuid.UUID
}
```

### Architectural Principles

1. **Single Responsibility**
   - Actions are responsible for configuration changes and state checks
   - Complex business logic belongs in FSMs outside the communicator

2. **Read/Write/Observe Pattern**
   - **Read**: Get current configuration or system state
   - **Write**: Apply changes to configuration
   - **Observe**: Monitor and report the observed state changes (optional)

3. **Separation of Concerns**
   - Parse: Extract and validate fields from raw payload
   - Validate: Check business rules and constraints
   - Execute: Perform config changes and manage state transitions

4. **Message Flow**
   - Actions send progress updates (ActionConfirmed, ActionExecuting)
   - Actions send failure notifications (ActionFinishedWithFailure)
   - The handler sends success notifications (ActionFinishedSuccessfull)

5. **Idempotence**
   - Actions should be safely repeatable without side effects
   - Use atomic operations when possible

## Action Lifecycle

### Parse
- Extract and validate required fields from raw JSON
- Convert to appropriate data structures
- Run basic structural validation

### Validate
- Perform deeper business rule validation
- Check relationships between fields
- Validate against system constraints

### Execute
- Send ActionConfirmed at start
- Read current configuration/state
- Apply changes to configuration
- Send ActionExecuting for progress updates
- For errors, send ActionFinishedWithFailure
- Return success result (do not send ActionFinishedSuccessfull)

## State Observation Pattern

For actions that modify system state, implement the read/write/observe pattern:

1. **Read Initial State**: Check current configuration/state
2. **Write Changes**: Apply configuration changes
3. **Observe Effects**: Monitor resulting state changes

TODO: write how to fetch observed state from the system

## Testing Strategy

Actions should be tested with a focus on:

1. **Configuration Changes**: Verify correct modifications are made
2. **Message Sequence**: Confirm proper progress messages are sent
3. **Error Handling**: Test both success and failure paths
4. **State Observation**: Verify correct state is detected and reported

Use dependency injection with mock implementations to:
- Simulate configuration operations
- Test error handling with custom mock wrappers
- Verify message flow without actual system changes

## Best Practices

1. **Feedback First**: Always send clear progress updates using ActionConfirmed/ActionExecuting
2. **Early Validation**: Fail fast in Parse/Validate before making any system changes
3. **Atomic Operations**: Use atomic configuration changes when possible
4. **Clear Error Messages**: Include specific error details in failure messages
5. **Timeout Management**: Always use context with timeout for external operations
6. **Idempotent Design**: Actions should be safely repeatable
7. **Proper Testing**: Test all message sequences and error paths
