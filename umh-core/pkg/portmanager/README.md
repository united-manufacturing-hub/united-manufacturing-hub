# Port Manager

The `portmanager` package provides functionality to allocate, reserve, and manage ports for services in the UMH core. It ensures that each service has a unique port for metrics endpoints.

## Usage

### Creating a Port Manager

```go
// Create a default port manager with a range of 9000-9999
pm, err := portmanager.NewDefaultPortManager(9000, 9999)
if err != nil {
    // Handle error
}
```

### Allocating a Port

```go
// Allocate a port for a service
port, err := pm.AllocatePort("my-service")
if err != nil {
    // Handle error
}
fmt.Printf("Allocated port %d for my-service\n", port)
```

### Reserving a Specific Port

```go
// Reserve a specific port (if the user specified one)
err := pm.ReservePort("my-service", 9500)
if err != nil {
    // Handle error (port might be already in use)
}
```

### Getting a Port

```go
// Get the port for a service
port, exists := pm.GetPort("my-service")
if !exists {
    // Service doesn't have a port
}
```

### Releasing a Port

```go
// Release a port when a service is removed
err := pm.ReleasePort("my-service")
if err != nil {
    // Handle error
}
```

## Testing

For testing, you can use the provided `MockPortManager`:

```go
// Create a mock port manager
pm := portmanager.NewMockPortManager()

// Configure predefined behavior if needed
pm.AllocatePortResult = 9500
pm.ReleasePortError = errors.New("test error")

// Use it in your tests
port, err := pm.AllocatePort("test-service")
// port will be 9500
```

## Thread Safety

Both the `DefaultPortManager` and `MockPortManager` are thread-safe and can be used concurrently. 