# UMH Core E2E Tests

This directory contains end-to-end tests for UMH Core that test the complete communication flow between UMH Core and a mock backend API.

## Architecture

The E2E test infrastructure consists of:

1. **Mock API Server** (`mock_server.go`): A simple HTTP server that implements the backend API endpoints that UMH Core expects:
   - `/v2/instance/login` - Authentication endpoint
   - `/v2/instance/pull` - Message delivery to UMH Core
   - `/v2/instance/push` - Message collection from UMH Core

2. **Message Flow Testing**:
   - **Pull Queue**: Messages added here will be delivered to UMH Core when it polls the pull endpoint
   - **Push Channel**: Messages that UMH Core sends to the backend are collected here for verification

3. **Container Management** (`container_helpers.go`): Utilities for starting and managing UMH Core Docker containers for testing

## Key Features

- **Simple Setup**: No need for complex account/instance creation - just start the mock server and connect UMH Core
- **Message Queue**: Add messages to a queue that will be delivered to UMH Core on the next pull
- **Push Collection**: Automatically collect and verify messages that UMH Core pushes back
- **Realistic Testing**: Uses actual Docker containers and HTTP communication like production

## Running the Tests

```bash
# From the umh-core directory
cd e2e

# Initialize go module dependencies
go mod tidy

# Build the UMH Core image first (if not already built)
cd ..
make build

# Run the E2E tests
cd e2e
go test -v

# Or using ginkgo directly for better output
ginkgo -v
```

## Test Structure

The tests verify:

1. **Basic Communication**: UMH Core can authenticate and establish communication with the mock backend
2. **Pull Mechanism**: Messages added to the pull queue are successfully delivered to UMH Core
3. **Push Mechanism**: Messages sent by UMH Core are correctly received and can be verified
4. **Message Flow**: Multiple messages can be processed reliably
5. **Performance**: Rapid message delivery doesn't destabilize the system

## Environment Variables

- `UMH_CORE_E2E_IMAGE`: Override the Docker image name used for testing (default: `umh-core:latest`)

## Example Usage

```go
// Add a message that UMH Core will receive
testMessage := models.UMHMessage{
    Metadata: &models.MessageMetadata{
        TraceID: uuid.New(),
    },
    Email:        "test@example.com",
    Content:      "Hello from test!",
    InstanceUUID: uuid.New(),
}
mockServer.AddMessageToPullQueue(testMessage)

// Wait for processing
time.Sleep(2 * time.Second)

// Check what UMH Core sent back
pushedMessages := mockServer.DrainPushedMessages()
```

## Development Notes

- The mock server automatically finds an available port to avoid conflicts
- Each test gets its own container instance with unique configuration
- Authentication is simplified - any Bearer token is accepted for testing
- All containers and resources are automatically cleaned up after tests
- The mock server provides detailed logging for debugging communication issues

## Future Enhancements

This basic infrastructure can be extended to test:
- Bridge creation and lifecycle
- Configuration changes via API
- Status monitoring and health checks
- Multi-instance scenarios
- Error conditions and recovery