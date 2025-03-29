# UMH Backoff Package

The backoff package provides a standardized way to handle operation retries, temporary failures and permanent failures across the UMH system.

## Features

- **Exponential backoff**: Implements increasing delays between retries
- **Permanent failure detection**: Identifies when to stop retrying after max attempts
- **Error wrapping**: Preserves original errors while adding backoff context
- **Thread-safe**: Can be used safely from multiple goroutines
- **Logging integration**: Provides comprehensive logging of retry behavior

## Usage

### Creating a BackoffManager

```go
// Create with default settings
logger := logger.For("MyComponent")
config := backoff.DefaultConfig("MyComponent", logger)
manager := backoff.NewBackoffManager(config)

// Or customize settings
config := backoff.Config{
    InitialInterval: 200 * time.Millisecond,
    MaxInterval:     30 * time.Second,
    MaxRetries:      3,
    Logger:          logger,
}
manager := backoff.NewBackoffManager(config)
```

### Using in Error Handling

```go
// When an operation fails, record the error
func DoSomething() error {
    err := someOperation()
    if err != nil {
        // Record error and check if we're permanently failed
        isPermanent := manager.SetError(err)
        if isPermanent {
            sentry.ReportIssuef(sentry.IssueTypeError, log, "Component has reached permanent failure state")
        }

        // Get a properly formatted backoff error to return
        return manager.GetBackoffError()
    }

    // Success! Reset backoff state
    manager.Reset()
    return nil
}
```

### Checking if Operation Should Be Skipped

```go
func TryOperation() error {
    // Check if we're in backoff state and should skip this operation
    if manager.ShouldSkipOperation() {
        // Get appropriate backoff error
        return manager.GetBackoffError()
    }

    // Proceed with operation...
    // ...
}
```

### Identifying Error Types

```go
// At the caller level, identify error types
err := component.DoSomething()
if err != nil {
    if backoff.IsTemporaryBackoffError(err) {
        // Handle temporary failure (e.g., log and continue)
        log.Info("Operation temporarily suspended, will retry later")
        return nil
    }

    if backoff.IsPermanentFailureError(err) {
        // Handle permanent failure (e.g., alert and abort)
        sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Operation permanently failed, system needs intervention")
        return err
    }

    return nil // Usually this should be nil, so the component can continue
}
```

## Error Handling Pattern

This package enables a consistent error escalation pattern:

1. **Temporary errors**: Components stay in backoff state and retry after delay, allowing systems to heal
2. **Permanent failures**: After max retries, a component signals it can't recover on its own
3. **Parent components**: Can detect permanent failures and take recovery actions (restart, notify, etc.)

This provides graceful degradation while avoiding error spam during temporary issues.
