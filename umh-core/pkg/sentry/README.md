# UMH Sentry Package

The sentry package provides a standardized way to handle error reporting and issue tracking across the UMH system. It integrates logging with different severity levels and ensures consistent error reporting behavior.

## Features

- **Severity levels**: Supports warning, error, and fatal issue types
- **Logger integration**: Seamlessly works with zap logger
- **Structured reporting**: Consistent error reporting format
- **Nil-safe**: Handles nil logger cases gracefully
- **Version tracking**: Integrates with application version reporting

## Usage

### Basic Error Reporting

```go
// Get a logger for your component
logger := logger.For("MyComponent")

// Report an error with different severity levels
sentry.ReportIssue(err, sentry.IssueTypeWarning, logger)
sentry.ReportIssue(err, sentry.IssueTypeError, logger)
sentry.ReportIssue(err, sentry.IssueTypeFatal, logger)
```

### Using Formatted Error Reporting

```go
// Report formatted errors directly
sentry.ReportIssuef(sentry.IssueTypeError, logger, "Failed to process item %d: %s", itemID, err)

// Use in error handling flows
if err != nil {
    sentry.ReportIssuef(sentry.IssueTypeFatal, logger, "Failed to load config: %s", err)
    return err
}
```

### Initializing Sentry

```go
// Initialize sentry with version tracking
appVersion := "1.0.0" // typically set by build system
sentry.InitSentry(appVersion)
```

### Error Severity Levels

The package provides three severity levels:

1. **Warning** (`IssueTypeWarning`): For non-critical issues that need attention
2. **Error** (`IssueTypeError`): For significant issues that might affect functionality
3. **Fatal** (`IssueTypeFatal`): For critical issues requiring immediate attention

## Error Handling Pattern

This package enables a consistent error reporting pattern:

1. **Component-level errors**: Use appropriate severity level based on impact
2. **System-wide tracking**: All errors are tracked centrally through sentry
3. **Version context**: Errors are associated with specific app versions
4. **Graceful degradation**: Nil-safe operations prevent cascading failures

## Best Practices

1. Always provide a context-specific logger
2. Use appropriate severity levels based on impact
3. Include relevant context in error messages
4. Initialize sentry early in application startup
5. Use formatted reporting for dynamic error messages

## Example Integration

```go
func ProcessConfig(ctx context.Context) error {
    logger := logger.For("ConfigProcessor")

    config, err := loadConfig()
    if err != nil {
        // Report fatal error for critical configuration issues
        sentry.ReportIssuef(sentry.IssueTypeFatal, logger, "Failed to load config: %s", err)
        return err
    }

    if config.HasWarnings() {
        // Report warning for non-critical issues
        sentry.ReportIssuef(sentry.IssueTypeWarning, logger, "Config has warnings: %v", config.Warnings)
    }

    return nil
}
```
