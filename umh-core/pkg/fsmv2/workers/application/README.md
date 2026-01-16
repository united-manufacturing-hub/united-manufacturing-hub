# Application Worker

The Application worker is the root coordinator for FSM v2. It dynamically creates and manages child workers based on YAML configuration, implementing the "passthrough pattern" where the application worker processes child specifications without needing to know about specific child types.

## Architecture

The Application worker is inspired by Erlang/OTP's Application concept - it coordinates a complete system with lifecycle management, configuration, and fault tolerance.

### Key Concepts

- **Application Worker**: The root coordinator that parses YAML configuration to discover and create child workers
- **Passthrough Pattern**: The application worker doesn't hardcode child types; it processes ChildSpec arrays from configuration
- **Dynamic Worker Creation**: Any registered worker type can be instantiated as a child via the factory pattern

## Directory Structure

```
application/
├── snapshot/           # State snapshot types
│   ├── snapshot.go    # ApplicationObservedState and ApplicationDesiredState
│   └── snapshot_test.go
├── worker.go          # ApplicationWorker implementation and factory registration
├── worker_test.go     # Unit tests
└── integration_test.go # Integration tests
```

## Usage

### Creating an Application Supervisor

```go
import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
)

sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
    ID:           "app-001",
    Name:         "My Application",
    Store:        myStore,
    Logger:       myLogger,
    TickInterval: 100 * time.Millisecond,
    YAMLConfig: `
children:
  - name: "worker-1"
    workerType: "example-child"
    userSpec:
      config: |
        value: 10
  - name: "worker-2"
    workerType: "example-child"
    userSpec:
      config: |
        value: 20
`,
})
if err != nil {
    return err
}

done := sup.Start(ctx)
<-done
```

### YAML Configuration Format

The application worker expects a YAML configuration with a `children` array:

```yaml
children:
  - name: "child-1"
    workerType: "example-child"  # Must be registered in factory
    userSpec:
      config: |
        # Child-specific configuration
        value: 10
```

## Factory Registration

The application worker automatically registers itself with the factory on import:

```go
import _ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
```

The application worker can be created dynamically:

```go
worker := factory.NewWorkerByType(identity, workerType)
supervisor := factory.NewSupervisorByType(supervisorCfg, workerType)
```

## State Types

### ApplicationObservedState

Represents the minimal observed state for an application supervisor:

```go
type ApplicationObservedState struct {
    ID          string
    CollectedAt time.Time
    Name        string
    DeployedDesiredState ApplicationDesiredState
}
```

### ApplicationDesiredState

Represents the desired state with children specifications:

```go
type ApplicationDesiredState struct {
    config.DesiredState  // Embeds State and ChildrenSpecs
    Name string
}
```

## Testing

Run tests:

```bash
go test ./pkg/fsmv2/workers/application/...
```

Run specific test suite:

```bash
go test ./pkg/fsmv2/workers/application -v
```

## Design Patterns

### Passthrough Pattern

The application worker doesn't need to know about specific child types. It simply:

1. Parses YAML configuration to extract `children` array
2. Returns `config.DesiredState` with `ChildrenSpecs` populated
3. The supervisor's `reconcileChildren()` handles actual child creation via factory

The application supervisor can manage any registered worker type as children without hardcoding types.

### Factory Integration

The application worker registers both:

- **Worker factory**: Creates `ApplicationWorker` instances
- **Supervisor factory**: Creates `Supervisor[ApplicationObservedState, *ApplicationDesiredState]` instances

Dynamic creation works without compile-time dependencies on specific child types.

## Migration from Root Package

This package replaces the previous `pkg/fsmv2/root` package with clearer naming:

| Old Name | New Name |
|----------|----------|
| `PassthroughWorker` | `ApplicationWorker` |
| `PassthroughObservedState` | `ApplicationObservedState` |
| `PassthroughDesiredState` | `ApplicationDesiredState` |
| `NewRootSupervisor` | `NewApplicationSupervisor` |
| `pkg/fsmv2/root` | `pkg/fsmv2/workers/application` |

## References

- [Erlang/OTP Applications](https://www.erlang.org/doc/design_principles/applications.html)
- [FSM v2 Architecture](../../README.md)
- [Example Parent Worker](../example/example-parent/) - Similar pattern with more features
