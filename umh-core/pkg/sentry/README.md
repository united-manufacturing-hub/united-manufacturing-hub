# Sentry Integration

This package contains utilities for reporting errors to Sentry with proper context and structure.

## Basic Usage

To report an issue to Sentry, you can use one of the basic reporter functions:

```go
// Simple error reporting
sentry.ReportIssue(err, sentry.IssueTypeError, logger)

// Formatted error messages
sentry.ReportIssuef(sentry.IssueTypeError, logger, "Failed to process item %d: %s", itemID, err)

// Fatal errors will automatically terminate the application
sentry.ReportIssuef(sentry.IssueTypeFatal, logger, "Failed to load config: %s", err)
```

## Context-Based Reporting (Recommended)

The context-based API is the recommended approach as it provides structured data for better error grouping and analysis in Sentry:

```go
// Report with context
context := map[string]interface{}{
    "item_id": 123,
    "operation": "process_record",
    "batch_size": 50,
}
sentry.ReportIssueWithContext(err, sentry.IssueTypeError, logger, context)

// Formatted version with context
sentry.ReportIssuefWithContext(
    sentry.IssueTypeWarning, 
    logger,
    context,
    "Config has warnings: %v", 
    config.Warnings,
)
```

## Domain-Specific Helpers

For common scenarios, specialized helper functions are available:

### FSM-Related Errors

```go
// Report an FSM error with contextual information
sentry.ReportFSMError(
    logger,
    "benthos-instance-1",
    "benthosfsm",
    "reconcile", 
    err,
)

// Formatted FSM error
sentry.ReportFSMErrorf(
    logger,
    "benthos-instance-1",
    "benthosfsm", 
    "create_failure",
    "Failed to create Benthos instance: %v", 
    err,
)
```

### Service-Related Errors

```go
// Report a service error with contextual information
sentry.ReportServiceError(
    logger,
    "kafka-broker-1",
    "kafka",
    "start_service",
    err,
)

// Formatted service error
sentry.ReportServiceErrorf(
    logger,
    "s6-service-abc",
    "s6",
    "status_check",
    "Failed to check service status: %v",
    err,
)
```

## Benefits of Context-Based Reporting

The context-based approach offers several advantages:

1. **Better Error Grouping**: Errors are grouped by operation and error type in Sentry, not by specific instance IDs or variable data.

2. **Enhanced Filtering**: You can filter errors in Sentry based on tags like `service_type`, `operation`, or `instance_id`.

3. **Structured Context**: All relevant contextual data is stored in a structured format, making it easier to analyze patterns.

4. **Consistent Fingerprinting**: Ensures similar errors are grouped together even if they occur on different instances.

## Sentry Output Example

With context-based reporting, errors in Sentry will appear like:

```
Error: failed to check if service exists: context deadline exceeded

Tags:
  service_id: benthos-benthos-2
  service_type: benthos
  operation: check_exists

Events: 5
```

Rather than:

```
Error: Error checking if service exists for benthos-benthos-2: failed to check if S6 service exists: context deadline exceeded
```

This makes it much easier to analyze and group related errors in Sentry.

## Panic Recovery

The package provides automatic panic recovery to catch unexpected crashes:

### Global Panic Recovery

Set up in your main function to catch any unhandled panics:

```go
func main() {
    // Initialize Sentry first
    sentry.InitSentry(version.GetAppVersion(), true)
    
    // Set up global panic recovery
    defer func() {
        if eventID := sentry.Recover(); eventID != nil {
            sentry.Flush(time.Second * 5)
        }
    }()
    
    // Your application code...
}
```

### Goroutine Panic Recovery

For individual goroutines, you have two options:

#### Option 1: Recover and Continue (Recommended for workers)

```go
func myWorkerGoroutine() {
    defer sentry.RecoverAndReport()
    
    // Your goroutine code that might panic...
    // If a panic occurs, it's logged to Sentry and execution continues
}
```

#### Option 2: Report and Crash (For critical failures)

```go
func myCriticalGoroutine() {
    defer sentry.RecoverReportAndRePanic()
    
    // Your goroutine code...
    // If a panic occurs, it's logged to Sentry then the program crashes
}
```

#### Custom Recovery

For more control, use the full pattern:

```go
func myWorkerGoroutine() {
    defer func() {
        if eventID := sentry.Recover(); eventID != nil {
            sentry.Flush(time.Second * 2)
        }
    }()
    
    // Your goroutine code...
}
```

## Safe Goroutine Launching

For even easier panic-safe goroutine management, use the `SafeGo` functions instead of the `go` keyword:

### Basic Safe Goroutines

```go
// Instead of: go myFunction()
sentry.SafeGo(myFunction)

// Or with anonymous functions
sentry.SafeGo(func() {
    // Your goroutine code that might panic...
    doSomethingRisky()
})
```

### Safe Goroutines with Context

```go
// For functions that need context
sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
    // Your context-aware goroutine code...
    processDataWithContext(ctx, data)
})

// Real-world example
sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // This work is now panic-safe
            doPeriodicWork()
        }
    }
})
```

### Benefits of SafeGo

- **Automatic panic recovery**: No need to remember `defer sentry.RecoverAndReport()`
- **Consistent across codebase**: Replace `go` with `sentry.SafeGo` everywhere
- **Zero overhead when no panic occurs**: Only adds recovery, no performance impact
- **Centralized error handling**: All goroutine panics go through the same recovery path

When a panic occurs, it will be automatically captured and sent to Sentry with full stack traces and context information.
