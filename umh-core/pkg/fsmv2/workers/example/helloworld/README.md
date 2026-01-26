# Hello World Worker

A minimal FSMv2 worker example demonstrating the core patterns for building
state-machine-based workers. Use this as a template when creating new workers.

## Quick Start

```go
// Import to register the worker (blank import in your main.go or scenario)
_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
```

## State Machine

```
┌──────────┐    ShouldBeRunning()    ┌──────────────────┐    HelloSaid    ┌─────────┐
│ stopped  │ ──────────────────────▶ │ trying_to_start  │ ─────────────▶ │ running │
└──────────┘                         └──────────────────┘                └─────────┘
     ▲                                        │                              │
     │        IsShutdownRequested()           │                              │
     └────────────────────────────────────────┴──────────────────────────────┘
```

## File Structure

```
helloworld/
├── README.md           # This file - start here!
├── worker.go           # Main worker implementation (3 required methods)
├── dependencies.go     # Action dependencies and state storage
├── userspec.go         # User configuration parsing
├── snapshot/
│   └── snapshot.go     # ObservedState and DesiredState definitions
├── state/
│   ├── stopped.go      # Initial state - waiting to start
│   ├── trying_to_start.go  # Transitional state - emits action
│   └── running.go      # Running state - worker is active
└── action/
    └── say_hello.go    # Action that transitions to running
```

## Critical Naming Convention

**GOTCHA**: The folder name determines the type prefix. The worker type is derived
from `{TypePrefix}ObservedState` by lowercasing `{TypePrefix}`.

| Folder Name | Type Prefix | Example Type |
|-------------|-------------|--------------|
| `helloworld` | `Helloworld` | `HelloworldObservedState` |
| `examplechild` | `Examplechild` | `ExamplechildObservedState` |

**NOT**: `HelloWorldObservedState` (two capitals would derive to `helloworld` but
the type name wouldn't match the folder convention).

## Creating a New Worker (Step by Step)

### 1. Create directory structure

```bash
mkdir -p pkg/fsmv2/workers/yourworker/{snapshot,state,action}
```

### 2. Define snapshot types (`snapshot/snapshot.go`)

```go
// CRITICAL: Type name must be {FolderName}ObservedState with EXACT capitalization
type YourworkerObservedState struct {
    CollectedAt time.Time `json:"collected_at"`
    deps.MetricsEmbedder `json:",inline"`  // Required for metrics
    // ... your fields
}

type YourworkerDesiredState struct {
    config.BaseDesiredState  // Provides ShutdownRequested, etc.
}
```

### 3. Implement states (`state/stopped.go`, etc.)

Each state needs two methods:
- `Next()` - Decision logic: return NextResult (state, signal, optional action, and **reason**)
- `String()` - State name for logging (use `helpers.DeriveStateName(s)`)

The **reason** is required and should explain why the transition/state is chosen.

### 4. Implement actions (`action/say_hello.go`)

Actions perform side effects. They must be **idempotent** (running twice has same
effect as running once).

### 5. Implement worker (`worker.go`)

Three required methods:
- `CollectObservedState()` - Read current state from dependencies
- `DeriveDesiredState()` - Parse user spec to desired state
- `GetInitialState()` - Return the starting state

### 6. Register with factory (`init()` in worker.go)

```go
func init() {
    if err := factory.RegisterWorkerType[snapshot.YourworkerObservedState, *snapshot.YourworkerDesiredState](
        workerFactory, supervisorFactory,
    ); err != nil {
        panic(err)
    }
}
```

## Testing

Run the hello world scenario:
```bash
go run pkg/fsmv2/cmd/runner/main.go --scenario=helloworld --duration=5s
```

## Common Mistakes

1. **Wrong type name capitalization**: `HelloWorldObservedState` vs `HelloworldObservedState`
2. **Missing factory registration**: Forgot the `init()` function
3. **Missing blank import**: Forgot to import the package in scenario/main
4. **Missing MetricsEmbedder**: ObservedState must embed `deps.MetricsEmbedder`
5. **Non-idempotent actions**: Action must be safe to call multiple times
