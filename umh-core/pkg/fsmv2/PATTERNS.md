# FSMv2 Established Patterns

This document explains patterns that are already implemented and working correctly in FSMv2, but may cause confusion if undocumented. These are **intentional design decisions**, not bugs or missing features.

If you're reviewing FSMv2 code or building workers, read this first to understand **WHY** things work the way they do.

---

## Table of Contents

1. [State Types: Structs, Not Strings](#state-types-structs-not-strings)
2. [Variable Namespaces: Three Tiers](#variable-namespaces-three-tiers)
3. [State Naming Conventions](#state-naming-conventions)
4. [Validation Layers: Defense-in-Depth](#validation-layers-defense-in-depth)
5. [Pass-by-Value Immutability](#pass-by-value-immutability)
6. [StateMapping: FSM Coordination, Not Data Passing](#statemapping-fsm-coordination-not-data-passing)
7. [BaseWorker Generic Pattern](#baseworker-generic-pattern)
8. [User-Defined Config: map[string]any](#user-defined-config-mapstringany)

---

## State Types: Structs, Not Strings

### Pattern

FSMv2 states are **concrete struct types** implementing the `State` interface, not string constants or enums.

```go
// This is the FSMv2 pattern ✅
type RunningState struct {
    BaseWorker[MyDeps]
}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    // State logic here
}

// NOT this ❌
const (
    StateRunning = "running"
    StateStopped = "stopped"
)
```

### Why This Way

**Type Safety**: Go's type system enforces that only valid states can be returned from `Next()`. You cannot accidentally create invalid states.

**Explicit Transitions**: Each state struct knows exactly which states it can transition to. The code documents valid transitions:

```go
// In RunningState.Next():
if needsReconfiguration {
    return ReconfiguringState{}, SignalNone, nil  // Type-safe transition
}
```

**Encapsulated Logic**: Each state's behavior is fully contained in its struct. Adding a new state means creating a new type, which forces you to implement all required methods.

**Prevents Stringly-Typed Code**: String-based states allow typos (`"runing"` vs `"running"`) and invalid states to compile. Struct types catch these errors at compile time.

### When NOT to Use Strings

Never use string constants for state machine states. Strings are for:
- Logging/debugging (via `State.String()`)
- Serialization to databases
- User-facing display

### Related Code

- `pkg/fsmv2/worker.go` - State interface definition
- `pkg/fsmv2/supervisor/supervisor.go` - State transitions
- Worker implementations (benthos, redpanda, etc.) - Concrete state structs

---

## Variable Namespaces: Three Tiers

### Pattern

FSMv2 uses a three-tier namespace structure for variables: **User**, **Global**, and **Internal**.

```go
type VariableBundle struct {
    User     map[string]any  `json:"user,omitempty"`     // Top-level in templates
    Global   map[string]any  `json:"global,omitempty"`   // {{ .global.varname }}
    Internal map[string]any  `json:"-"`                  // {{ .internal.varname }} (not serialized)
}
```

### Why Three Tiers

Each tier serves a distinct purpose:

**User Namespace** (`variables.user`):
- Contains user-defined variables from config files
- Parent state variables passed to children
- Computed values derived at runtime
- **Accessible in templates as top-level**: `{{ .IP }}`, `{{ .PORT }}`
- **Serialized to YAML/JSON** (persisted)

**Global Namespace** (`variables.global`):
- Fleet-wide settings from management system
- Cluster-level configuration
- Shared values across all workers
- **Accessible with prefix**: `{{ .global.api_endpoint }}`
- **Serialized to YAML/JSON** (persisted)

**Internal Namespace** (`variables.internal`):
- Runtime metadata (worker IDs, timestamps, bridged_by)
- System-generated values
- Temporary execution context
- **Accessible with prefix**: `{{ .internal.id }}`
- **NOT serialized** (runtime-only, never persisted)

### Template Access Pattern

Variables are flattened when rendering templates:

```go
bundle := VariableBundle{
    User: map[string]any{"IP": "192.168.1.100", "PORT": 502},
    Global: map[string]any{"cluster_id": "production"},
    Internal: map[string]any{"timestamp": time.Now()},
}

flattened := bundle.Flatten()
// Result:
// {
//   "IP": "192.168.1.100",          // Top-level access
//   "PORT": 502,                     // Top-level access
//   "global": {"cluster_id": "..."},  // Nested access
//   "internal": {"timestamp": "..."}  // Nested access
// }
```

### Why Serialization Differs

**Internal variables are NOT serialized** because:
- They're runtime-specific (timestamps, process IDs)
- Re-creating them from saved state would be incorrect
- They should be recomputed fresh on each execution

**User and Global ARE serialized** because:
- They represent configuration that should persist
- They're needed to reproduce exact worker behavior
- They're user-controlled or management-system-controlled

### Related Code

- `pkg/fsmv2/types/variables.go` - VariableBundle definition and Flatten()
- `pkg/fsmv2/types/childspec.go` - UserSpec with VariableBundle

---

## State Naming Conventions

### Pattern

State names follow semantic conventions that indicate their behavior:

**Active States** (prefix: "TryingTo" or "Ensuring"):
- Emit actions on every tick until success condition met
- Represent ongoing operations needing retries
- Examples: `TryingToStartState`, `EnsuringConnectedState`

**Passive States** (descriptive nouns):
- Only observe and transition based on conditions
- No actions emitted unless something changes
- Examples: `RunningState`, `StoppedState`, `DegradedState`

### Why This Convention

The naming pattern communicates state behavior without reading the code:

```go
// Active state - continuously tries to achieve goal
type TryingToStartState struct{}

func (s TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
    if processIsRunning(snapshot.Observed) {
        return RunningState{}, SignalNone, nil  // Goal achieved, move to passive state
    }
    return s, SignalNone, &StartProcessAction{}  // Keep trying
}

// Passive state - stable, only reacts to changes
type RunningState struct{}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return TryingToStopState{}, SignalNone, nil  // Transition to active state
    }
    return s, SignalNone, nil  // Stay stable, no action needed
}
```

### Active vs Passive Decision

**Use Active States** when:
- Operation may fail and needs retries (starting process, connecting to network)
- External system needs to reach desired state (waiting for service readiness)
- Action must be repeated until condition met

**Use Passive States** when:
- System is in stable condition (running, stopped)
- Only monitoring for changes (waiting for external trigger)
- No action needed unless something changes

### Related Code

- `pkg/fsmv2/worker.go` - State interface and Next() behavior
- Worker implementations - Concrete state naming examples

---

## Validation Layers: Defense-in-Depth

### Pattern

FSMv2 validates data at **multiple layers**, not in one centralized validator. This is **intentional**, not redundant or scattered code.

### The Four Layers

**Layer 1: API Entry** (`supervisor.AddWorker()`)
- Fast fail on obviously invalid input
- Validates before any state changes
- Example: Empty worker ID, nil dependencies

**Layer 2: Reconciliation Entry** (`supervisor.reconcileChildren()`)
- Runtime edge cases that evolved since Layer 1
- State consistency checks
- Example: Duplicate child IDs, circular references

**Layer 3: Factory** (`WorkerFactory`)
- Validates WorkerType is registered
- Checks factory exists before attempting creation
- Example: Unknown worker types, missing factories

**Layer 4: Worker Constructor** (`NewMyWorker()`)
- Validates dependencies are compatible
- Worker-specific requirements
- Example: Logger not nil, required interfaces implemented

### Why Multiple Layers

**Security**: Never trust data, even from internal callers. Each layer defends against different attack vectors.

**Debuggability**: Errors caught closest to source with maximum context. Layer 1 error is easier to debug than Layer 4.

**Robustness**: One layer failing doesn't compromise system. If factory validation is bypassed, constructor still validates.

**Separation of Concerns**: Each layer validates what it's responsible for:
- Layer 1: Public API contracts
- Layer 2: System state invariants
- Layer 3: Registry consistency
- Layer 4: Business logic requirements

### Example: Adding a Worker

```go
// Layer 1: API entry point
func (s *Supervisor) AddWorker(id string, workerType string, spec interface{}) error {
    if id == "" {
        return errors.New("worker ID cannot be empty")  // Fast fail
    }
    // ... continue processing
}

// Layer 2: Reconciliation
func (s *Supervisor) reconcileChildren(parent *worker, desired types.DesiredState) error {
    childIDs := make(map[string]bool)
    for _, childSpec := range desired.ChildrenSpecs {
        if childIDs[childSpec.Name] {
            return errors.New("duplicate child ID")  // Runtime consistency
        }
        childIDs[childSpec.Name] = true
    }
    // ... continue processing
}

// Layer 3: Factory
func (s *Supervisor) createWorkerFromFactory(workerType string, deps Dependencies) (Worker, error) {
    factory, exists := s.workerRegistry[workerType]
    if !exists {
        return nil, errors.New("unknown worker type")  // Registry validation
    }
    return factory(deps), nil
}

// Layer 4: Worker constructor
func NewMyWorker(deps MyDeps) (*MyWorker, error) {
    if deps.Logger == nil {
        return nil, errors.New("logger is required")  // Business logic
    }
    return &MyWorker{BaseWorker: fsmv2.NewBaseWorker(deps)}, nil
}
```

### When NOT to Add Validation

Don't add validation if:
- It duplicates validation in the same layer (actually redundant)
- It checks internal state that should always be valid (invariants)
- It validates after the operation succeeded (too late)

### Related Code

- `pkg/fsmv2/supervisor/supervisor.go` - AddWorker (Layer 1), reconcileChildren (Layer 2)
- `pkg/fsmv2/worker_factory.go` - Factory validation (Layer 3)
- Worker implementations - Constructor validation (Layer 4)

---

## Pass-by-Value Immutability

### Pattern

FSMv2 achieves immutability through **pass-by-value semantics**, not getters or defensive copying.

```go
// Snapshot is passed BY VALUE (copied)
func (s MyState) Next(snapshot Snapshot) (State, Signal, Action) {
    // snapshot is a COPY - mutations don't affect supervisor's snapshot
    snapshot.Identity.Name = "modified"  // Only affects local copy
    return s, SignalNone, nil
}
```

### Why This Way

**Language-Level Enforcement**: Go copies structs when passing by value. The compiler guarantees immutability, no runtime validation needed.

**No Getters Needed**: Getters add boilerplate without adding safety. Pass-by-value makes mutation impossible.

**Idiomatic Go**: This is how Go's standard library works (`time.Time`, `net.IP`). We follow established Go patterns, not OOP patterns.

### Immutability Guarantees

When `State.Next(snapshot Snapshot)` is called:
1. Go copies the Snapshot struct
2. Fields (Identity, Observed, Desired) are copied as values or interface pointers
3. States can mutate their local copy
4. Supervisor's snapshot remains unchanged
5. No defensive copying required

### Defense-in-Depth Layers

- **Layer 1**: Pass-by-value (Go language design)
- **Layer 2**: Documentation (this file and godoc)
- **Layer 3**: Tests (`supervisor/immutability_test.go`)

### Why NOT Getters

```go
// This adds no safety ❌
func (s Snapshot) GetIdentity() Identity {
    return s.Identity  // Still returns a copy (pass-by-value)
}

// This is equivalent and simpler ✅
snapshot.Identity  // Go's pass-by-value handles it
```

Getters don't prevent mutation in Go because the language already prevents it via value semantics.

### Related Code

- `pkg/fsmv2/worker.go` - Snapshot struct and Next() signature
- `pkg/fsmv2/supervisor/immutability_test.go` - Immutability tests

---

## StateMapping: FSM Coordination, Not Data Passing

### Pattern

`StateMapping` in ChildSpec allows parent FSM states to trigger child FSM state transitions. **This is NOT data passing** - it's state synchronization.

```go
ChildSpec{
    Name:       "mqtt-connection",
    WorkerType: "mqtt_client",
    StateMapping: map[string]string{
        "running":  "connected",   // When parent is running, child should be connected
        "stopping": "disconnected", // When parent is stopping, child should disconnect
    },
}
```

### Why This Way

**Lifecycle Coordination**: Parent FSM controls child FSM lifecycle without tight coupling. Parent doesn't need to know child's internal states.

**State Synchronization**: When parent enters "stopping" state, supervisor automatically transitions child to "disconnected" state.

**NOT Data Passing**: StateMapping doesn't transfer data between FSMs. Use VariableBundle for data.

### When to Use StateMapping

Use StateMapping when:
- Parent lifecycle controls child lifecycle (Stopping → Cleanup)
- Parent operational state affects child behavior (Paused → Idle)
- You need FSM state coordination, not data sharing

### When NOT to Use StateMapping

Don't use StateMapping for:
- **Passing data between states** → Use VariableBundle instead
- **Triggering actions** → Use signals instead
- **One-time initialization** → Pass via UserSpec
- **Runtime communication** → Use channels or shared state

### Example: Protocol Converter

```go
// Parent (protocol converter) manages children
DesiredState{
    State: "active",
    ChildrenSpecs: []ChildSpec{
        {
            Name:       "modbus-connection",
            WorkerType: "modbus_client",
            StateMapping: map[string]string{
                "idle":    "stopped",    // Converter idle → disconnect
                "active":  "connected",  // Converter active → connect
                "closing": "stopped",    // Converter closing → disconnect
            },
        },
        {
            Name:       "data-flow",
            WorkerType: "benthos_flow",
            StateMapping: map[string]string{
                "active": "running",  // Only run when parent is active
            },
        },
    },
}
```

### Related Code

- `pkg/fsmv2/types/childspec.go` - ChildSpec with StateMapping
- `pkg/fsmv2/supervisor/supervisor.go` - StateMapping application during reconciliation

---

## BaseWorker Generic Pattern

### Pattern

FSMv2 provides `BaseWorker[D Dependencies]` generic type for type-safe dependency injection. **This pattern has existed since Go 1.18 generics** (added Nov 2, 2025 to FSMv2).

```go
// Define dependencies
type MyWorkerDeps struct {
    Logger    *zap.Logger
    APIClient *http.Client
}

// Embed BaseWorker with type parameter
type MyWorker struct {
    fsmv2.BaseWorker[MyWorkerDeps]
}

// Constructor
func NewMyWorker(deps MyWorkerDeps) *MyWorker {
    return &MyWorker{
        BaseWorker: fsmv2.NewBaseWorker(deps),
    }
}

// Access dependencies with type safety
func (w *MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    deps := w.GetDependencies()
    deps.Logger.Info("collecting state")  // Type-safe access, no casting
    resp, err := deps.APIClient.Get("https://api.example.com/status")
    // ...
}
```

### Why This Way

**Type Safety**: Dependencies are strongly typed. `deps.Logger` returns `*zap.Logger`, not `interface{}`. Compiler catches type errors.

**No Casting**: Access dependencies directly without type assertions or reflection.

**Inherited Fields**: Workers automatically inherit common functionality from BaseWorker (GetDependencies method, potential future additions).

**Better Than interface{}**: Old pattern required `deps.(*MyDeps).Logger`, which could panic at runtime. Generics catch errors at compile time.

### Why NOT interface{} or Reflection

```go
// Old pattern (before generics) ❌
type Worker interface {
    GetDependencies() interface{}  // Untyped
}

func (w *MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    deps := w.GetDependencies().(*MyWorkerDeps)  // Runtime cast, can panic
    deps.Logger.Info("collecting state")
}

// Generic pattern (current) ✅
func (w *MyWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    deps := w.GetDependencies()  // Compile-time type-safe
    deps.Logger.Info("collecting state")
}
```

### When to Use BaseWorker[D]

Use BaseWorker[D] when:
- Worker needs access to shared dependencies (logger, metrics, DB)
- Dependencies are specific to worker type (not global)
- Type safety is important (compile-time vs runtime errors)

### When NOT to Use BaseWorker[D]

Don't use BaseWorker[D] if:
- Worker has no dependencies (rare)
- Dependencies change at runtime (use channels instead)

### Related Code

- `pkg/fsmv2/worker_base.go` - BaseWorker[D] definition
- Worker implementations - Examples of BaseWorker[D] usage

---

## User-Defined Config: map[string]any

### Pattern

`VariableBundle.User` uses `map[string]any` for user-defined configuration, not typed structs.

```go
type VariableBundle struct {
    User map[string]any `json:"user,omitempty"`  // NOT a struct
}
```

### Why This Way

**User-Defined Fields**: Users write arbitrary YAML configuration with custom field names. We cannot pre-define struct fields for user-controlled keys.

```yaml
# User's YAML config
variables:
  CustomIP: "192.168.1.100"      # User-defined field name
  CustomPort: 502                # User-defined field name
  MySpecialFlag: true            # User-defined field name
  AnyFieldNameTheyWant: "value"  # Arbitrary user choice
```

**Template Variables**: Templates reference user-defined variables: `{{ .CustomIP }}`, `{{ .MySpecialFlag }}`. We can't know these field names ahead of time.

**Type Safety at Template Render**: Golang templates validate variable references when rendering. If user references `{{ .UndefinedVar }}`, template rendering fails with clear error.

### Why NOT Structs

```go
// We CANNOT do this ❌
type UserVariables struct {
    IP   string  // What if user names it "IPAddress"?
    Port int     // What if user names it "PortNumber"?
    // ... we'd need infinite fields for all possible user choices
}
```

Struct fields are defined at compile time. User configuration fields are defined at runtime in YAML files we don't control.

### Type Safety Model

Type safety happens at **template rendering time**, not config parsing time:

1. **Config Parsing**: Accept any fields as `map[string]any`
2. **Template Rendering**: Golang template engine validates variable references
3. **Error Reporting**: Undefined variables in templates produce clear errors

This is **intentional**: We trade compile-time type safety for runtime flexibility, then enforce correctness at template rendering.

### When to Use map[string]any

Use `map[string]any` when:
- User defines field names in YAML/JSON
- Configuration schema is user-controlled
- Template variables are dynamic

### When NOT to Use map[string]any

Don't use `map[string]any` for:
- **System-defined configuration** → Use typed structs
- **Internal state** → Use typed structs
- **API contracts** → Use typed structs with validation

### Related Code

- `pkg/fsmv2/types/variables.go` - VariableBundle with map[string]any
- `pkg/fsmv2/types/childspec.go` - UserSpec with VariableBundle
- Template rendering code - Golang template execution with variable validation

---

## Summary

These patterns are **intentional design decisions** based on Go idioms, type safety requirements, and real-world usage:

1. **State Types: Structs** → Compile-time type safety
2. **Variable Namespaces: Three Tiers** → Clear separation of concerns
3. **State Naming: Active vs Passive** → Behavior communication
4. **Validation Layers: Defense-in-Depth** → Multiple safety nets
5. **Pass-by-Value Immutability** → Language-level enforcement
6. **StateMapping: FSM Coordination** → Lifecycle synchronization
7. **BaseWorker[D]: Generics** → Type-safe dependencies
8. **map[string]any: User Config** → Runtime flexibility with template validation

If a code review tool or external analysis suggests changing these patterns, **reference this document** to explain why they're correct as-is.
