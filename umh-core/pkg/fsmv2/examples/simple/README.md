# FSM v2 Simple Example

A minimal, runnable example demonstrating the FSM v2 application worker pattern.

## Run It

```bash
cd pkg/fsmv2/examples/simple
go run main.go
```

## What It Demonstrates

- **Application supervisor as entry point** - Root supervisor that parses YAML and orchestrates workers
- **YAML configuration** - Workers declared in config, created via factory pattern
- **Parent-child worker hierarchy** - Parent returns `ChildrenSpecs`, supervisor creates children
- **Graceful shutdown** - Cancel context → children stop → parent stops → done channel closes

## Architecture

```
Application Worker (supervisor)
    │
    └── Parent Worker
            ├── Child Worker 1
            └── Child Worker 2
```

The application worker reads YAML config and creates the parent worker. The parent worker returns `ChildrenSpecs` for 2 children. The supervisor automatically creates and manages children. All workers run their FSMs independently.

## Key Code Pattern

```go
// Create application supervisor from YAML config
sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
    ID:           "app-001",
    Name:         "Simple Application",
    Store:        store,
    Logger:       sugar,
    TickInterval: 100 * time.Millisecond,
    YAMLConfig:   yamlConfig,
})

// Start and gracefully shutdown
ctx, cancel := context.WithCancel(context.Background())
done := sup.Start(ctx)
// ... run for a while ...
cancel()  // Signal shutdown
<-done    // Wait for completion
```

## Next Steps

1. **Read core docs**: `pkg/fsmv2/README.md`
2. **Run architecture tests**: `ginkgo run --focus="Architecture" -v ./pkg/fsmv2/`
3. **Study reference implementations**: `pkg/fsmv2/workers/example/`
