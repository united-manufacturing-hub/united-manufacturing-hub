
# HTTPClient Service

A context-aware HTTP client service that handles request timeouts automatically based on context deadlines.

## Overview

This package provides a standardized way to make HTTP requests throughout the UMH codebase with proper context handling, error wrapping, and timeout management. The client automatically configures request timeouts based on the context deadline.

## Features

- Context-aware HTTP client that respects deadlines
- Automatic scaling of connection and response timeouts
- Simplified request handling with combined response and body retrieval
- Mockable interface for testing

## Usage

### Basic Usage

```go
import (
    "context"
    "time"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/httpclient"
)

func makeRequest() {
    // Create a context with deadline
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Create HTTP client
    client := httpclient.NewDefaultHTTPClient()
    
    // Make a request
    resp, body, err := client.GetWithBody(ctx, "http://example.com/api/status")
    if err != nil {
        // Handle error
    }
    
    // Use response and body
    fmt.Printf("Status: %d, Body: %s\n", resp.StatusCode, string(body))
}
```

## Testing with MockHTTPClient

For an example of how to mock it, see the [benthos](../benthos/benthos.go) package.

## Implementation Details

The HTTP client automatically configures timeouts based on context deadlines:

- Connection timeout: 50% of available time until deadline
- Response timeout: 50% of available time until deadline
- Other timeouts (TLS, idle connections, etc.): 25% of available time

This scaling ensures that no single operation consumes the entire available time budget.

## Requirements

- All contexts passed to the client **must have a deadline**, either via timeout or explicit deadline.
- The client creates a new transport for each request, optimized for that request's specific deadline.

## Best Practices

1. Always set reasonable deadlines on your contexts
2. Use `GetWithBody` for simple GET requests
3. Use `Do` for more complex requests that need custom headers or request bodies
4. In tests, create mocks that simulate realistic timing behaviors
