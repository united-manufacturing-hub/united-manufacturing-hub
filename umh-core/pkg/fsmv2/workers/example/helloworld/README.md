# Hello World Worker

A minimal FSMv2 worker demonstrating the WorkerBase API: typed config/status,
action wrapping via `SimpleAction`, and one-line registration.

## Quick Start

```go
// Blank imports to register worker type and initial state at init time
_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
```

## State Diagram

```text
                          !shutdown              HelloSaid       mood="sad"   mood!="sad"
 ┌─────────┐          ┌──────────────────┐          ┌─────────┐ ────────▶ ┌──────────┐
 │ Stopped │────────▶│ TryingToStart    │────────▶│ Running │ ◀──────── │ Degraded │
 └─────────┘          └──────────────────┘          └─────────┘           └──────────┘
      ▲                       │                          │                    │
      └───────────────────────┴──────────────────────────┴────────────────────┘
                                   shutdown
```

## Control-Loop Mapping

| Phase | Implementation |
|-------|---------------|
| **Observe** (`CollectObservedState`) | Reads `deps.HasSaidHello()` + mood file from `cfg.MoodFilePath` |
| **Derive** (`DeriveDesiredState`) | WorkerBase parses YAML into `HelloworldConfig` |
| **Evaluate** (`State.Next()`) | Checks shutdown, hello said, mood — returns state or action |
| **Execute** (Actions) | `SayHello` logs a greeting and calls `deps.SetHelloSaid(true)` |

See the parent [README's control loop section](../../../README.md#the-control-loop) for the general pattern.

## File Layout

```text
helloworld/
├── worker.go         # Worker struct, COS, Actions(), init registration
├── action.go         # SayHello action function + SayHelloActionName const
├── config.go         # HelloworldConfig (TConfig) + HelloworldStatus (TStatus)
├── dependencies.go   # HelloworldDependencies (action state)
└── state/
    ├── stopped.go          # Initial state — waits for !shutdown
    ├── trying_to_start.go  # Emits SayHello action
    ├── running.go          # Steady state — checks mood
    └── degraded.go         # mood="sad" — returns to Running when mood clears
```

## Config Example

```yaml
config: |
  state: running
  moodFilePath: /tmp/helloworld-mood
```

```bash
echo "happy" > /tmp/helloworld-mood   # stays Running
echo "sad"   > /tmp/helloworld-mood   # → Degraded
rm /tmp/helloworld-mood               # → Running (no file = fine)
```

## Common Mistakes

1. **Non-idempotent actions**: `SayHello` checks `HasSaidHello()` before acting — actions may run more than once
2. **Missing registration**: Forgot `register.Worker[...]()` in `init()` — worker type unknown at runtime
3. **Missing blank import**: Forgot to import the package in scenario/main — `init()` never runs
