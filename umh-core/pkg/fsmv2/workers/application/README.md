# Application worker

The application worker is the root coordinator for FSM v2. It creates and manages child workers based on YAML configuration using the passthrough pattern, where the application worker processes child specifications without knowing about specific child types.

## Architecture

The application worker coordinates a complete system with lifecycle management, configuration, and fault tolerance, inspired by Erlang/OTP's Application concept.

### Key concepts

- **Application worker**: Root coordinator that parses YAML configuration to discover and create child workers
- **Passthrough pattern**: Processes ChildSpec arrays from configuration without hardcoding child types
- **Dynamic worker creation**: Any registered worker type can be instantiated as a child via the factory pattern

## Directory structure

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

### Creating an application supervisor

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

### YAML configuration format

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

## Factory registration

The application worker registers itself with the factory on import:

```go
import _ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
```

Create the application worker dynamically:

```go
worker := factory.NewWorkerByType(identity, workerType)
supervisor := factory.NewSupervisorByType(supervisorCfg, workerType)
```

## State types

### ApplicationObservedState

Represents the observed state for an application supervisor:

```go
type ApplicationObservedState struct {
    CollectedAt             time.Time `json:"collected_at"`
    ID                      string    `json:"id"`
    Name                    string    `json:"name"`
    State                   string    `json:"state"`
    ApplicationDesiredState           `json:",inline"`
    deps.MetricsEmbedder              `json:",inline"`
    // Infrastructure health from ChildrenView
    ChildrenHealthy         int       `json:"children_healthy"`
    ChildrenUnhealthy       int       `json:"children_unhealthy"`
    ChildrenCircuitOpen     int       `json:"children_circuit_open"`
    ChildrenStale           int       `json:"children_stale"`
}
```

### ApplicationDesiredState

Represents the desired state with child specifications:

```go
type ApplicationDesiredState struct {
    config.BaseDesiredState `json:",inline"`  // Embeds State and ShutdownRequested
    Name          string                       `json:"name"`
    ChildrenSpecs []config.ChildSpec           `json:"childrenSpecs,omitempty"`
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

## Design patterns

### Passthrough pattern

The application worker:

1. Parses YAML configuration to extract `children` array
2. Returns `config.DesiredState` with `ChildrenSpecs` populated
3. The supervisor's `reconcileChildren()` handles child creation via factory

The application supervisor can manage any registered worker type as children without hardcoding types.

### Factory integration

The application worker registers both:

- **Worker factory**: Creates `ApplicationWorker` instances
- **Supervisor factory**: Creates `Supervisor[ApplicationObservedState, *ApplicationDesiredState]` instances

Dynamic creation works without compile-time dependencies on specific child types.

## Migration from root package

This package replaces `pkg/fsmv2/root` with clearer naming:

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
- [Example Parent Worker](../example/exampleparent/) - Similar pattern with more features
