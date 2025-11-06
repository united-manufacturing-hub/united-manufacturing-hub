# FSMv2: DeriveDesiredState Complete API Definition

**Date:** 2025-11-02
**Status:** Design Specification (Ready for Implementation)
**Related:**
- `fsmv2-idiomatic-templating-and-variables.md` - Templating and variable system
- `fsmv2-child-specs-in-desired-state.md` - Hierarchical composition architecture
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Master implementation plan

---

## Executive Summary

This document defines the complete DeriveDesiredState API for FSMv2, resolving all open questions about:
- Parent-child state control (parent controls via Variables, child computes state)
- Template rendering (distributed at worker level, every tick)
- Type strategy (strict UserSpec, loose Variables)
- Hierarchical composition (Kubernetes pattern with ChildSpecs in DesiredState)

**Key Decisions:**
1. **Parent controls child state indirectly** - Parent passes `parent_state` variable, child computes own state
2. **Workers are standalone** - Child doesn't require parent reference, all inputs in UserSpec
3. **Templates render in workers** - Each worker renders own templates in DeriveDesiredState
4. **Single method interface** - ChildrenSpecs returned in DesiredState (no separate DeclareChildren)

---

## Table of Contents

1. [DeriveDesiredState API Definition](#1-deriveddesiredstate-api-definition)
2. [ChildSpec Structure](#2-childspec-structure)
3. [State Control Pattern](#3-state-control-pattern)
4. [Template Rendering Flow](#4-template-rendering-flow)
5. [Worker Implementation Patterns](#5-worker-implementation-patterns)
6. [UserSpec Design](#6-userspec-design)
7. [Supervisor Integration](#7-supervisor-integration)
8. [Complete Scenarios](#8-complete-scenarios)
9. [Comparison Table](#9-comparison-table)

---

## 1. DeriveDesiredState API Definition

### 1.1 Exact Signature

```go
type Worker interface {
    // CollectObservedState gathers current reality about this worker
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // DeriveDesiredState computes what SHOULD be, including child specifications
    // 
    // Parameters:
    //   userSpec - User configuration + variables from parent
    //
    // Returns:
    //   DesiredState with:
    //     - State: This worker's desired state (string)
    //     - Config: Worker-specific config (optional, if worker needs it)
    //     - ChildrenSpecs: Specifications for children (if parent worker)
    //
    // Access to observations:
    //   - Worker CAN access own previous observed state via w.lastObserved
    //   - Worker CANNOT access current tick's observed state (causality)
    //   - Parent CAN pass parent_observed to child via Variables
    DeriveDesiredState(userSpec UserSpec) DesiredState
}
```

### 1.2 Parameter Description

**`userSpec UserSpec`**

Complete configuration for this worker, containing:
- **Typed fields** - Worker-specific configuration (IPAddress, Port, etc.)
- **Variables** - Loose maps for template rendering and parent context

Worker receives UserSpec from either:
- **Root worker:** Loaded from config.yaml by supervisor
- **Child worker:** Constructed by parent in parent's DeriveDesiredState

**NO access to:**
- Current tick's observed state (causality: DeriveDesiredState called BEFORE CollectObservedState)
- Parent worker reference (standalone principle)
- Sibling workers (isolation)

**CAN access:**
- Previous tick's observed state via `w.lastObserved` (if worker caches it)
- Parent's previous observed state via `userSpec.Variables.User["parent_observed"]` (if parent passes it)

### 1.3 Return Value Structure

```go
type DesiredState struct {
    // State is THIS worker's desired state
    State string

    // Config is worker-specific configuration (optional)
    // Example: Benthos config, protocol converter settings
    Config interface{}

    // ChildrenSpecs defines child workers (if parent)
    // Empty slice if leaf worker (no children)
    ChildrenSpecs []ChildSpec
}
```

**Key Properties:**
- ✅ Fully serializable (all fields JSON-encodable)
- ✅ Contains complete system specification (state + children)
- ✅ Matches Kubernetes pattern (spec, not runtime instances)

### 1.4 Access to Observations

**DeriveDesiredState CANNOT access current observations:**

```go
// ❌ WRONG: This is causality violation
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    observed, _ := w.CollectObservedState(ctx)  // Called AFTER derivation!
    // Can't use observed here
}
```

**DeriveDesiredState CAN access previous observations:**

```go
// ✅ CORRECT: Worker caches previous observation
type Worker struct {
    lastObserved *ObservedState  // From previous tick
}

func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    state := "up"
    
    // Use PREVIOUS observation for sanity checks
    if w.lastObserved != nil && !w.lastObserved.Healthy {
        state = "down"
    }
    
    return DesiredState{State: state}
}

func (w *Worker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    observed := // ... collect current state
    
    w.lastObserved = &observed  // Cache for next tick
    
    return observed, nil
}
```

**Parent CAN pass observed to child:**

```go
// ✅ CORRECT: Parent passes previous observation as variable
func (w *ParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_observed": w.lastObserved,  // Previous tick
                        },
                    },
                },
            },
        },
    }
}
```

**Why this restriction?**
- **Causality:** Tick order is Derive → Collect, can't access future
- **Separation:** Desired = "should be", Observed = "is"
- **Reactive logic belongs in code**, not templates

---

## 2. ChildSpec Structure

### 2.1 All Fields with Types

```go
type ChildSpec struct {
    // Name uniquely identifies this child within parent
    // Example: "connection", "dfc_read", "server-192.168.1.10"
    Name string

    // WorkerType identifies worker implementation
    // Example: "ConnectionWorker", "BenthosWorker"
    // Used by WorkerFactory to instantiate worker
    WorkerType string

    // UserSpec is child's complete configuration
    // Parent constructs this with:
    //   - Child-specific typed fields
    //   - Variables including parent_state
    UserSpec UserSpec

    // StateMapping maps parent state → child desired state
    // Example: {"active": "up", "idle": "down"}
    // Applied by supervisor before ticking child
    StateMapping map[string]string
}
```

### 2.2 StateMapping vs Direct State Decision

**DECISION: Use StateMapping**

Parent declares state mapping, supervisor applies it:

```go
ChildSpec{
    Name:       "connection",
    WorkerType: WorkerTypeConnection,
    UserSpec:   connectionSpec,
    StateMapping: map[string]string{
        "active": "up",      // When parent is "active", child should be "up"
        "idle":   "down",    // When parent is "idle", child should be "down"
        "error":  "down",    // When parent is "error", child should be "down"
    },
}
```

**How supervisor applies StateMapping:**

```go
func (s *Supervisor) tickChild(childInstance *childInstance) {
    parentState := s.triangularStore.GetDesiredState().State
    
    // Map parent state to child desired state
    childDesiredState, ok := childInstance.stateMapping[parentState]
    if !ok {
        // No mapping = child computes own state
        childDesiredState = ""
    }
    
    if childDesiredState != "" {
        // Override child's computed state with mapped state
        childInstance.supervisor.SetDesiredState(childDesiredState)
    }
    
    // Tick child (child may also compute state from parent_state variable)
    childInstance.supervisor.Tick(ctx)
}
```

**Why StateMapping instead of parent setting state directly?**

**Option A: Parent sets child state directly (REJECTED)**
```go
// ❌ Parent directly controls child state
ChildSpec{
    Name:         "connection",
    DesiredState: "up",  // Parent decides child state
}
```

Problems:
- Parent needs to know child's state space
- Child can't react to own observations
- Tight coupling (parent knows child internals)

**Option B: Child computes from parent_state variable (PARTIAL)**
```go
// ⚠️ Child computes from parent_state
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    parentState := scope["parent_state"].(string)
    
    state := "down"
    if parentState == "active" || parentState == "idle" {
        state = "up"
    }
    
    return DesiredState{State: state}
}
```

Problems:
- Every child must implement state computation logic
- No declarative parent control
- Harder to understand parent-child relationship

**Option C: StateMapping (CHOSEN)**
```go
// ✅ Parent declares mapping, supervisor applies
StateMapping: map[string]string{
    "active": "up",
    "idle":   "down",
}
```

Benefits:
- ✅ Declarative (clear parent-child relationship)
- ✅ Decoupled (child doesn't know parent's state space)
- ✅ Flexible (child can still compute state from observations)
- ✅ Supervisor enforces (infrastructure layer)

**Hybrid approach (RECOMMENDED):**

Child can BOTH respect StateMapping AND react to observations:

```go
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Check if supervisor wants us in specific state (via StateMapping)
    // Note: Supervisor sets this via SetDesiredState before calling DeriveDesiredState
    // This is just an example - actual API TBD
    
    // Compute state from parent context
    parentState := scope["parent_state"].(string)
    state := "down"
    if parentState == "active" {
        state = "up"
    }
    
    // Sanity check: Even if parent wants "up", go down if unhealthy
    if w.lastObserved != nil && !w.lastObserved.Healthy {
        state = "down"
    }
    
    return DesiredState{State: state}
}
```

Supervisor applies StateMapping AFTER child computes:
1. Child computes state from logic
2. Supervisor checks StateMapping
3. If mapping exists for parent state, OVERRIDE child's state
4. Child's next observation sees overridden state

### 2.3 Variable Passing Mechanism

Parent constructs child's VariableBundle:

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    parentState := "active"
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{
                    // Typed fields
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                    
                    // Variables
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Parent context
                            "parent_state":    parentState,
                            "parent_observed": w.lastObserved,  // Optional
                            
                            // Location hierarchy
                            "location": mergeLocations(
                                parentLocation,
                                map[string]string{"bridge": userSpec.Name},
                            ),
                            "location_path": computeLocationPath(...),
                            
                            // Connection-specific
                            "IP":   userSpec.IPAddress,
                            "PORT": userSpec.Port,
                        },
                        
                        // Pass through global variables
                        Global: userSpec.Variables.Global,
                        
                        // Internal set by supervisor (parent doesn't touch)
                        // Internal: {...}
                    },
                },
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
        },
    }
}
```

**What parent controls:**
- ✅ Child's UserSpec (typed fields)
- ✅ Child's User variables (loose map)
- ✅ Which variables child sees
- ✅ StateMapping (parent state → child desired state)

**What parent does NOT control:**
- ❌ Child's actual state (child computes, StateMapping overrides)
- ❌ Child's Internal variables (supervisor sets these)
- ❌ How child reacts to observations

### 2.4 Template Fields

**Templates are NOT in ChildSpec.**

Templates are:
- Defined in worker implementation (worker knows what template it needs)
- Referenced in UserSpec (if worker needs template ID/name)
- Rendered in worker's DeriveDesiredState (worker renders own templates)

Example:

```go
// Parent creates child spec
ChildSpec{
    Name:       "dfc_read",
    WorkerType: WorkerTypeBenthos,
    UserSpec: BenthosUserSpec{
        TemplateName: "modbus-tcp",  // ← Template reference in UserSpec
        Variables: VariableBundle{
            User: map[string]any{
                "IP":   "192.168.1.100",
                "PORT": 502,
            },
        },
    },
}

// Child worker renders template
type BenthosWorker struct {
    templateStore TemplateStore  // Worker knows how to get templates
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    // 1. Load template by name
    template := w.templateStore.Get(userSpec.TemplateName)
    
    // 2. Render with variables
    config, _ := RenderTemplate(template, userSpec.Variables.Flatten())
    
    return DesiredState{
        State:  "running",
        Config: config,
    }
}
```

**Key points:**
- Template name/ID in UserSpec (data)
- Template rendering in worker (code)
- No templates in ChildSpec (specs are data, rendering is behavior)

---

## 3. State Control Pattern

### 3.1 Chosen Approach: StateMapping + Child Autonomy

**Architecture:**
1. Parent declares StateMapping in ChildSpec
2. Supervisor applies mapping before ticking child
3. Child ALSO computes state from observations
4. Child can override mapped state for safety

**Code flow:**

```go
// PARENT: Declare state mapping
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name: "connection",
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": "active",  // Also pass as variable
                        },
                    },
                },
            },
        },
    }
}

// SUPERVISOR: Apply state mapping
func (s *Supervisor) tickChild(childInstance *childInstance) {
    parentState := s.triangularStore.GetDesiredState().State
    
    // Check if parent declares state mapping
    if childDesiredState, ok := childInstance.stateMapping[parentState]; ok {
        childInstance.supervisor.SetDesiredState(childDesiredState)
    }
    
    childInstance.supervisor.Tick(ctx)
}

// CHILD: Respect mapping but add safety checks
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Compute state from parent context
    parentState := scope["parent_state"].(string)
    state := "up"  // Default to up if parent active
    
    // Safety check: Go down if connection unhealthy
    if w.lastObserved != nil && !w.lastObserved.Connected {
        state = "down"
    }
    
    return DesiredState{State: state}
}
```

### 3.2 Rationale

**Why StateMapping?**
- Declarative parent control (parent says "when I'm X, child should be Y")
- Clear in code (reader sees parent-child relationship immediately)
- Matches Kubernetes patterns (parent declares desired state for children)

**Why child also computes?**
- Child autonomy (child can react to own observations)
- Safety checks (child can go down even if parent says up)
- Complex logic (child can implement multi-condition state transitions)

**Why both?**
- Best of both worlds
- Parent declares intent (mapping)
- Child implements safety (observations)
- Supervisor enforces (infrastructure)

### 3.3 Code Examples

**Simple Mapping (Connection):**

```go
// Parent declares: "When I'm active, connection should be up"
StateMapping: map[string]string{
    "active": "up",
    "idle":   "down",
    "error":  "down",
}

// Child just respects it (no complex logic needed)
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Simple: No state computation, supervisor handles via StateMapping
    return DesiredState{State: "up"}  // Will be overridden by supervisor
}
```

**Complex Logic (Benthos):**

```go
// Parent declares: "When I'm active, benthos should run"
StateMapping: map[string]string{
    "active": "running",
    "idle":   "stopped",
}

// Child adds complex safety checks
func (w *BenthosWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    state := "running"  // Default from StateMapping
    
    // Safety: Check Redpanda health (from parent_observed)
    if parentObserved, ok := scope["parent_observed"].(RedpandaObservedState); ok {
        if !parentObserved.KafkaHealthy {
            state = "stopped"  // Don't run if Kafka down
        }
    }
    
    // Safety: Check own observed state
    if w.lastObserved != nil && w.lastObserved.Errors > 100 {
        state = "stopped"  // Too many errors, back off
    }
    
    return DesiredState{State: state}
}
```

**No Mapping (Dynamic Children):**

```go
// Parent doesn't declare mapping (child fully autonomous)
ChildSpec{
    Name:       "server-192.168.1.10",
    WorkerType: WorkerTypeOPCUAServerBrowser,
    StateMapping: nil,  // No mapping
}

// Child computes state independently
func (w *OPCUAServerBrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    parentState := scope["parent_state"].(string)
    state := "stopped"
    
    // Complex logic: Browse only if parent active
    if parentState == "active" {
        // Check if server reachable
        if w.lastObserved != nil && w.lastObserved.ServerReachable {
            state = "browsing"
        } else {
            state = "connecting"
        }
    }
    
    return DesiredState{State: state}
}
```

---

## 4. Template Rendering Flow

### 4.1 Where Templates Live

**Two locations:**

**1. Template Definitions (Data)**

Stored in config.yaml or template store:

```yaml
templates:
  - id: "modbus-tcp"
    config: |
      input:
        modbus_tcp:
          address: "{{ .IP }}:{{ .PORT }}"
      output:
        kafka:
          topic: "umh.v1.{{ .location_path }}.{{ .name }}"
```

**2. Template References (Worker Code)**

Worker knows how to load and render templates:

```go
type BenthosWorker struct {
    templateStore TemplateStore  // Access to template definitions
}

type BenthosUserSpec struct {
    TemplateName string  // Reference to template ID
    Variables    VariableBundle
}
```

### 4.2 Who Renders Templates

**THE WORKER ITSELF, in DeriveDesiredState:**

```go
func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    // 1. Flatten variables for template access
    scope := userSpec.Variables.Flatten()
    // scope = {
    //   "IP": "192.168.1.100",           // User variables (top-level)
    //   "PORT": 502,
    //   "parent_state": "active",
    //   "location_path": "ACME.Factory.Line-A",
    //   "global": {                       // Global variables (nested)
    //     "api_endpoint": "https://..."
    //   }
    // }
    
    // 2. Load template
    template := w.templateStore.Get(userSpec.TemplateName)
    
    // 3. Render template
    config, err := RenderTemplate(template, scope)
    if err != nil {
        w.logger.Error("Failed to render template", zap.Error(err))
        return DesiredState{State: "error"}
    }
    
    // 4. Compute state
    state := "stopped"
    if scope["parent_state"] == "active" {
        state = "running"
    }
    
    return DesiredState{
        State:  state,
        Config: config,
    }
}
```

**NOT rendered in:**
- ❌ Supervisor (supervisor doesn't know worker's template needs)
- ❌ Parent worker (parent doesn't render child's templates)
- ❌ Central service (defeats distributed architecture)

### 4.3 When Templates Render

**EVERY TICK, in DeriveDesiredState:**

```
┌─────────────────────────────────┐
│ Supervisor.Tick()               │
│                                 │
│ 1. userSpec = store.GetUserSpec()
│ 2. desired = worker.DeriveDesiredState(userSpec)  ← TEMPLATES RENDER HERE
│ 3. observed = worker.CollectObservedState()
│ 4. if desired != observed:
│ 5.   triggerAction()
│ 6. tickChildren()
└─────────────────────────────────┘
```

**Why every tick?**
- Variables might change (IP updated, parent state changed)
- Simpler (no caching, no invalidation)
- Fast enough (~100μs per worker)

**Performance:**
- Template rendering: ~100μs per worker
- 100 workers × 1 tick/sec = 10ms CPU/sec
- Negligible for production workloads

**Optimization (future):**
```go
type Worker struct {
    lastVariablesHash string
    cachedConfig      Config
}

func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    hash := hashVariables(userSpec.Variables)
    if hash != w.lastVariablesHash {
        // Variables changed, re-render
        w.cachedConfig = RenderTemplate(...)
        w.lastVariablesHash = hash
    }
    return DesiredState{Config: w.cachedConfig}
}
```

Don't implement caching initially (premature optimization).

### 4.4 Example Template Usage

**Config.yaml:**

```yaml
templates:
  - id: "modbus-tcp"
    config: |
      input:
        label: "modbus_{{ .name }}"
        modbus_tcp:
          address: "{{ .IP }}:{{ .PORT }}"
      
      output:
        broker:
          outputs:
            - kafka:
                addresses: ["{{ .global.kafka_brokers }}"]
                topic: "umh.v1.{{ .location_path }}.{{ .name }}"

protocolConverters:
  - id: "bridge-plc"
    name: "Factory-PLC"
    templateRef: "modbus-tcp"
    connection:
      variables:
        IP: "192.168.1.100"
        PORT: 502
    location:
      - line: "Line-A"
      - cell: "Cell-5"
```

**Parent Worker:**

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    location := mergeLocations(
        userSpec.AgentLocation,    // {"enterprise": "ACME", "site": "Factory"}
        userSpec.BridgeLocation,   // {"line": "Line-A", "cell": "Cell-5"}
    )
    
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef,  // "modbus-tcp"
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  "active",
                            "IP":            "192.168.1.100",
                            "PORT":          502,
                            "name":          "Factory-PLC_read",
                            "location":      location,
                            "location_path": "ACME.Factory.Line-A.Cell-5",
                        },
                        Global: userSpec.Variables.Global,
                    },
                },
            },
        },
    }
}
```

**Child Worker (Benthos):**

```go
func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    // scope = {
    //   "IP": "192.168.1.100",
    //   "PORT": 502,
    //   "name": "Factory-PLC_read",
    //   "location_path": "ACME.Factory.Line-A.Cell-5",
    //   "global": {
    //     "kafka_brokers": "localhost:9092"
    //   }
    // }
    
    template := w.templateStore.Get(userSpec.TemplateName)
    config, _ := RenderTemplate(template, scope)
    
    // config = 
    // input:
    //   label: "modbus_Factory-PLC_read"
    //   modbus_tcp:
    //     address: "192.168.1.100:502"
    // 
    // output:
    //   broker:
    //     outputs:
    //       - kafka:
    //           addresses: ["localhost:9092"]
    //           topic: "umh.v1.ACME.Factory.Line-A.Cell-5.Factory-PLC_read"
    
    state := "stopped"
    if scope["parent_state"] == "active" {
        state = "running"
    }
    
    return DesiredState{
        State:  state,
        Config: config,
    }
}
```

**Rendered Config:**

```yaml
input:
  label: "modbus_Factory-PLC_read"
  modbus_tcp:
    address: "192.168.1.100:502"

output:
  broker:
    outputs:
      - kafka:
          addresses: ["localhost:9092"]
          topic: "umh.v1.ACME.Factory.Line-A.Cell-5.Factory-PLC_read"
```

---

## 5. Worker Implementation Patterns

### 5.1 Simple Worker (No Children)

**Characteristics:**
- No children declared
- Just computes own state
- May or may not use templates

**Example: ConnectionWorker**

```go
type ConnectionWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type ConnectionUserSpec struct {
    IPAddress string
    Port      uint16
    Timeout   time.Duration
    Variables VariableBundle
}

func (w *ConnectionWorker) DeriveDesiredState(userSpec ConnectionUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Compute state from parent state
    parentState, _ := scope["parent_state"].(string)
    state := "down"
    
    if parentState == "active" || parentState == "idle" {
        state = "up"
    }
    
    // Safety: Check previous observation
    if w.lastObserved != nil && !w.lastObserved.Connected {
        state = "down"
    }
    
    return DesiredState{
        State:         state,
        ChildrenSpecs: nil,  // No children
    }
}

func (w *ConnectionWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    ipPort := fmt.Sprintf("%s:%d", userSpec.IPAddress, userSpec.Port)
    conn, err := net.DialTimeout("tcp", ipPort, userSpec.Timeout)
    
    observed := ObservedState{
        State:     "down",
        Connected: false,
        Healthy:   false,
    }
    
    if err == nil {
        conn.Close()
        observed.State = "up"
        observed.Connected = true
        observed.Healthy = true
    }
    
    w.lastObserved = &observed  // Cache for next tick
    
    return observed, nil
}
```

**Lines of code:** ~20 lines
**Complexity:** Low (no children, simple state logic)

### 5.2 Parent with Static Children

**Characteristics:**
- Fixed set of children (always same children)
- Children declared in DeriveDesiredState
- StateMapping controls child states

**Example: BridgeWorker**

```go
type BridgeWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type BridgeUserSpec struct {
    Name         string
    IPAddress    string
    Port         uint16
    TemplateRef  string
    Variables    VariableBundle
}

func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    parentState := "active"
    
    // Merge location hierarchy
    parentLocation, _ := userSpec.Variables.User["location"].(map[string]string)
    bridgeLocation := map[string]string{"bridge": userSpec.Name}
    location := mergeLocations(parentLocation, bridgeLocation)
    locationPath := computeLocationPath(location)
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            // Child 1: Connection monitor
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: ConnectionUserSpec{
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": parentState,
                        },
                    },
                },
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            
            // Child 2: Read data flow
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  parentState,
                            "name":          userSpec.Name + "_read",
                            "IP":            userSpec.IPAddress,
                            "PORT":          userSpec.Port,
                            "location":      location,
                            "location_path": locationPath,
                        },
                        Global: userSpec.Variables.Global,
                    },
                },
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
            
            // Child 3: Write data flow (if bidirectional)
            {
                Name:       "dfc_write",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef + "_write",
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  parentState,
                            "name":          userSpec.Name + "_write",
                            "IP":            userSpec.IPAddress,
                            "PORT":          userSpec.Port,
                            "location":      location,
                            "location_path": locationPath,
                        },
                        Global: userSpec.Variables.Global,
                    },
                },
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}

func (w *BridgeWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Bridge observed state = aggregated child health
    return ObservedState{
        State:   "active",
        Healthy: true,
    }, nil
}
```

**Lines of code:** ~50 lines
**Complexity:** Medium (3 static children, location merging)

### 5.3 Parent with Dynamic Children

**Characteristics:**
- Variable number of children (N children based on config)
- Children created in loop
- Each child gets unique name

**Example: OPCUABrowserWorker**

```go
type OPCUABrowserWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type OPCUABrowserUserSpec struct {
    Servers   []string  // List of OPC UA server endpoints
    Variables VariableBundle
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec OPCUABrowserUserSpec) DesiredState {
    parentState := "active"
    
    // Create child spec for each server
    childSpecs := make([]ChildSpec, len(userSpec.Servers))
    for i, serverAddr := range userSpec.Servers {
        childSpecs[i] = ChildSpec{
            Name:       serverAddr,  // Unique name per server
            WorkerType: WorkerTypeOPCUAServerBrowser,
            UserSpec: OPCUAServerBrowserUserSpec{
                ServerAddress: serverAddr,
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state": parentState,
                        "server_addr":  serverAddr,
                    },
                    Global: userSpec.Variables.Global,
                },
            },
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        }
    }
    
    return DesiredState{
        State:         parentState,
        ChildrenSpecs: childSpecs,
    }
}

func (w *OPCUABrowserWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Aggregated health of all server browsers
    return ObservedState{
        State:   "active",
        Healthy: true,
    }, nil
}
```

**Lines of code:** ~25 lines
**Complexity:** Low-Medium (dynamic children, simple loop)

### 5.4 Parent with Conditional Children

**Characteristics:**
- Children present only if certain conditions met
- Children added/removed based on config
- Different children for different modes

**Example: ProtocolConverterWorker**

```go
type ProtocolConverterWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type ProtocolConverterUserSpec struct {
    Name         string
    Protocol     string  // "modbus", "opcua", "s7", etc.
    Bidirectional bool   // Has both read and write flows?
    Variables    VariableBundle
}

func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec ProtocolConverterUserSpec) DesiredState {
    parentState := "active"
    
    childSpecs := []ChildSpec{}
    
    // Always have connection child
    childSpecs = append(childSpecs, ChildSpec{
        Name:       "connection",
        WorkerType: WorkerTypeConnection,
        UserSpec:   connectionSpec,
        StateMapping: map[string]string{
            "active": "up",
            "idle":   "down",
        },
    })
    
    // Always have read flow
    childSpecs = append(childSpecs, ChildSpec{
        Name:       "dfc_read",
        WorkerType: WorkerTypeBenthos,
        UserSpec:   readFlowSpec,
        StateMapping: map[string]string{
            "active": "running",
            "idle":   "stopped",
        },
    })
    
    // Conditional: Write flow only if bidirectional
    if userSpec.Bidirectional {
        childSpecs = append(childSpecs, ChildSpec{
            Name:       "dfc_write",
            WorkerType: WorkerTypeBenthos,
            UserSpec:   writeFlowSpec,
            StateMapping: map[string]string{
                "active": "running",
                "idle":   "stopped",
            },
        })
    }
    
    // Conditional: Protocol-specific child
    switch userSpec.Protocol {
    case "opcua":
        childSpecs = append(childSpecs, ChildSpec{
            Name:       "opcua_browser",
            WorkerType: WorkerTypeOPCUABrowser,
            UserSpec:   opcuaBrowserSpec,
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    case "s7":
        childSpecs = append(childSpecs, ChildSpec{
            Name:       "s7_diagnostics",
            WorkerType: WorkerTypeS7Diagnostics,
            UserSpec:   s7DiagnosticsSpec,
            StateMapping: map[string]string{
                "active": "monitoring",
                "idle":   "stopped",
            },
        })
    }
    
    return DesiredState{
        State:         parentState,
        ChildrenSpecs: childSpecs,
    }
}

func (w *ProtocolConverterWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    return ObservedState{State: "active", Healthy: true}, nil
}
```

**Lines of code:** ~40 lines
**Complexity:** Medium (conditional children, protocol-specific logic)

---

## 6. UserSpec Design

### 6.1 Type Strategy: Strict vs Loose

**DECISION: Strict types for UserSpec fields, loose maps for Variables**

```go
// ✅ CORRECT: Strict types
type ConnectionUserSpec struct {
    IPAddress string        // Typed field
    Port      uint16        // Typed field
    Timeout   time.Duration // Typed field
    Variables VariableBundle // Loose maps inside
}

// ❌ WRONG: Loose maps everywhere
type UserSpec struct {
    Config map[string]any  // Loses type safety
}
```

**Rationale:**
1. **Compile-time validation** - Typos caught early
2. **IDE autocomplete** - Developer experience
3. **Clear contracts** - What does worker expect?
4. **Type safety** - Can't pass string to uint16

### 6.2 Variables Integration

**Variables are PART OF UserSpec, not separate:**

```go
type UserSpec struct {
    // Worker-specific typed fields
    IPAddress string
    Port      uint16
    
    // Variables for templates and parent context
    Variables VariableBundle
}

type VariableBundle struct {
    User     map[string]any  // User config + parent state
    Global   map[string]any  // Fleet-wide settings
    Internal map[string]any  // Runtime metadata
}
```

**Usage:**

```go
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Access typed fields directly
    ip := userSpec.IPAddress
    port := userSpec.Port
    
    // Access variables via Flatten()
    scope := userSpec.Variables.Flatten()
    parentState := scope["parent_state"]
    locationPath := scope["location_path"]
    
    return DesiredState{...}
}
```

### 6.3 Template Fields vs Rendered Fields

**Template references in UserSpec, rendered config in DesiredState:**

```go
type BenthosUserSpec struct {
    TemplateName string          // Template reference (data)
    Variables    VariableBundle  // Variables for rendering
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    // Load and render template
    template := w.templateStore.Get(userSpec.TemplateName)
    config, _ := RenderTemplate(template, userSpec.Variables.Flatten())
    
    return DesiredState{
        State:  "running",
        Config: config,  // Rendered config (data)
    }
}
```

**Key distinction:**
- UserSpec.TemplateName = which template to use (reference)
- DesiredState.Config = rendered result (actual config)
- Rendering happens in worker, not in UserSpec

### 6.4 Templatable vs Non-Templatable Fields

**ALL UserSpec fields are non-templatable (already typed):**

```go
type UserSpec struct {
    IPAddress string  // NOT "{{ .IP }}" - already a value
    Port      uint16  // NOT "{{ .PORT }}" - already a value
}
```

**Templates only in Variables:**

```yaml
# User writes this:
connection:
  variables:
    IP: "192.168.1.100"  # Value
    PORT: 502            # Value

# NOT this:
connection:
  ipAddress: "{{ .IP }}"  # Wrong! UserSpec fields are typed, not templated
```

**Why?**
- UserSpec is Go struct (typed)
- Templates are strings (need rendering)
- Parent constructs UserSpec with ACTUAL VALUES, not templates
- Only worker-internal configs use templates (Benthos config, etc.)

**Example flow:**

```yaml
# 1. User config (config.yaml)
connection:
  variables:
    IP: "192.168.1.100"
    PORT: 502

# 2. Parent creates child UserSpec (Go code)
UserSpec{
    IPAddress: "192.168.1.100",  // ACTUAL VALUE
    Port:      502,               // ACTUAL VALUE
    Variables: VariableBundle{
        User: map[string]any{
            "IP":   "192.168.1.100",  // Also in variables for templates
            "PORT": 502,
        },
    },
}

# 3. Child renders template (Go code)
template := "input:\n  address: \"{{ .IP }}:{{ .PORT }}\""
config := RenderTemplate(template, scope)
// config = "input:\n  address: \"192.168.1.100:502\""
```

### 6.5 Example UserSpec Structures

**Simple UserSpec (no templates):**

```go
type ConnectionUserSpec struct {
    IPAddress string
    Port      uint16
    Timeout   time.Duration
    Variables VariableBundle
}

// Usage
userSpec := ConnectionUserSpec{
    IPAddress: "192.168.1.100",
    Port:      502,
    Timeout:   5 * time.Second,
    Variables: VariableBundle{
        User: map[string]any{
            "parent_state": "active",
        },
    },
}
```

**Template-based UserSpec:**

```go
type BenthosUserSpec struct {
    Name         string
    TemplateName string  // Reference to template
    Variables    VariableBundle
}

// Usage
userSpec := BenthosUserSpec{
    Name:         "Factory-PLC_read",
    TemplateName: "modbus-tcp",
    Variables: VariableBundle{
        User: map[string]any{
            "parent_state":  "active",
            "IP":            "192.168.1.100",
            "PORT":          502,
            "location_path": "ACME.Factory.Line-A",
        },
    },
}
```

**Complex UserSpec (protocol-specific):**

```go
type OPCUAServerBrowserUserSpec struct {
    ServerAddress string
    NamespaceURI  string
    Credentials   Credentials
    BrowseDepth   int
    Variables     VariableBundle
}

// Usage
userSpec := OPCUAServerBrowserUserSpec{
    ServerAddress: "opc.tcp://192.168.1.100:4840",
    NamespaceURI:  "http://example.com/UA/",
    Credentials: Credentials{
        Username: "admin",
        Password: "secret",
    },
    BrowseDepth: 5,
    Variables: VariableBundle{
        User: map[string]any{
            "parent_state": "active",
            "server_addr":  "opc.tcp://192.168.1.100:4840",
        },
    },
}
```

---

## 7. Supervisor Integration

### 7.1 How Supervisor Calls DeriveDesiredState

```go
type Supervisor struct {
    worker          Worker
    factory         WorkerFactory
    children        map[string]*childInstance
    triangularStore TriangularStore
    logger          *zap.Logger
}

type childInstance struct {
    name         string
    supervisor   *Supervisor
    userSpec     UserSpec
    stateMapping map[string]string
}

func (s *Supervisor) Tick(ctx context.Context) error {
    // 1. Load UserSpec from TriangularStore
    userSpec := s.triangularStore.GetUserSpec()
    
    // 2. Call worker's DeriveDesiredState
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // 3. Persist desired state (includes children!)
    s.triangularStore.SetDesiredState(desiredState)
    
    // 4. Reconcile children from specs
    s.reconcileChildren(desiredState.ChildrenSpecs)
    
    // 5. Apply state mapping to children
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }
    
    // 6. Tick each child with its userSpec
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }
    
    // 7. Collect observed state
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)
    
    return nil
}
```

### 7.2 How Supervisor Handles ChildrenSpecs

```go
func (s *Supervisor) reconcileChildren(specs []ChildSpec) error {
    // Build map of desired children
    desiredChildren := make(map[string]ChildSpec)
    for _, spec := range specs {
        desiredChildren[spec.Name] = spec
    }
    
    // Add or update children
    for name, spec := range desiredChildren {
        if child, exists := s.children[name]; exists {
            // Child exists - check if UserSpec changed
            if !reflect.DeepEqual(child.userSpec, spec.UserSpec) {
                s.logger.Info("Child UserSpec changed, recreating",
                    "child", name,
                )
                
                // Shutdown old supervisor
                child.supervisor.Shutdown()
                
                // Create new worker and supervisor
                worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
                if err != nil {
                    s.logger.Error("Failed to create worker", "error", err)
                    continue
                }
                
                child.supervisor = NewSupervisor(worker, s.logger)
                child.userSpec = spec.UserSpec
                child.stateMapping = spec.StateMapping
            }
        } else {
            // Child doesn't exist - create it
            s.logger.Info("Adding child", "name", name, "type", spec.WorkerType)
            
            worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
            if err != nil {
                s.logger.Error("Failed to create worker", "error", err)
                continue
            }
            
            supervisor := NewSupervisor(worker, s.logger)
            
            s.children[name] = &childInstance{
                name:         name,
                supervisor:   supervisor,
                userSpec:     spec.UserSpec,
                stateMapping: spec.StateMapping,
            }
        }
    }
    
    // Remove children not in desired specs
    for name := range s.children {
        if _, desired := desiredChildren[name]; !desired {
            s.logger.Info("Removing child", "name", name)
            s.removeChild(name)
        }
    }
    
    return nil
}

func (s *Supervisor) removeChild(name string) {
    if child, exists := s.children[name]; exists {
        child.supervisor.Shutdown()
        delete(s.children, name)
    }
}
```

### 7.3 How Supervisor Applies State Control

```go
func (s *Supervisor) applyStateMapping(parentState string) {
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[parentState]; ok {
            s.logger.Debug("Applying state mapping",
                "child", name,
                "parent_state", parentState,
                "child_desired_state", childDesiredState,
            )
            
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }
}
```

### 7.4 Tick Loop Pseudocode

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // ====== PHASE 1: INFRASTRUCTURE CHECKS ======
    // (From Infrastructure Supervision design)
    
    // Check infrastructure health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        time.Sleep(s.infraHealthChecker.Backoff().NextDelay())
        return nil  // Skip entire tick
    }
    
    s.circuitOpen = false
    
    // ====== PHASE 2: DERIVE DESIRED STATE ======
    
    // Load UserSpec
    userSpec := s.triangularStore.GetUserSpec()
    
    // Derive desired state (with children!)
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // Persist
    s.triangularStore.SetDesiredState(desiredState)
    
    // ====== PHASE 3: RECONCILE CHILDREN ======
    
    // Reconcile children from specs
    s.reconcileChildren(desiredState.ChildrenSpecs)
    
    // Apply state mapping
    s.applyStateMapping(desiredState.State)
    
    // ====== PHASE 4: TICK CHILDREN ======
    
    for _, child := range s.children {
        // Check if child has action in progress
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue  // Skip this child
        }
        
        // Tick child
        child.supervisor.Tick(ctx)
    }
    
    // ====== PHASE 5: COLLECT OBSERVED STATE ======
    
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)
    
    return nil
}
```

---

## 8. Complete Scenarios

### 8.1 Bridge → Connection (Simple Parent-Child)

**User Config:**

```yaml
protocolConverters:
  - id: "bridge-plc"
    name: "Factory-PLC"
    connection:
      variables:
        IP: "192.168.1.100"
        PORT: 502
    location:
      - line: "Line-A"
```

**Parent Worker (Bridge):**

```go
type BridgeWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type BridgeUserSpec struct {
    Name      string
    IPAddress string
    Port      uint16
    Variables VariableBundle
}

func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    parentState := "active"
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: ConnectionUserSpec{
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                    Timeout:   5 * time.Second,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": parentState,
                        },
                    },
                },
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
        },
    }
}
```

**Child Worker (Connection):**

```go
type ConnectionWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type ConnectionUserSpec struct {
    IPAddress string
    Port      uint16
    Timeout   time.Duration
    Variables VariableBundle
}

func (w *ConnectionWorker) DeriveDesiredState(userSpec ConnectionUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Read parent state
    parentState, _ := scope["parent_state"].(string)
    
    // Compute state from parent state
    state := "down"
    if parentState == "active" || parentState == "idle" {
        state = "up"
    }
    
    // Safety check from observations
    if w.lastObserved != nil && !w.lastObserved.Connected {
        state = "down"
    }
    
    return DesiredState{
        State:         state,
        ChildrenSpecs: nil,
    }
}

func (w *ConnectionWorker) CollectObservedState(ctx context.Context, userSpec ConnectionUserSpec) (ObservedState, error) {
    ipPort := fmt.Sprintf("%s:%d", userSpec.IPAddress, userSpec.Port)
    conn, err := net.DialTimeout("tcp", ipPort, userSpec.Timeout)
    
    observed := ObservedState{
        State:     "down",
        Connected: false,
        Healthy:   false,
    }
    
    if err == nil {
        conn.Close()
        observed.State = "up"
        observed.Connected = true
        observed.Healthy = true
    }
    
    w.lastObserved = &observed
    
    return observed, nil
}
```

**Supervisor Tick Flow:**

```
1. Supervisor loads BridgeUserSpec from config
2. Supervisor calls BridgeWorker.DeriveDesiredState(spec)
3. BridgeWorker returns DesiredState with:
   - State: "active"
   - ChildrenSpecs: [ConnectionSpec]
4. Supervisor reconciles children:
   - Creates ConnectionWorker
   - Creates child Supervisor for ConnectionWorker
   - Stores ChildSpec.StateMapping
5. Supervisor applies StateMapping:
   - Parent state = "active"
   - StateMapping["active"] = "up"
   - Calls childSupervisor.SetDesiredState("up")
6. Supervisor ticks child:
   - Child supervisor calls ConnectionWorker.DeriveDesiredState(connectionSpec)
   - ConnectionWorker computes state = "up" (from parent_state="active")
   - ConnectionWorker checks lastObserved.Connected (true)
   - ConnectionWorker returns DesiredState{State: "up"}
7. Child supervisor collects observed state:
   - Dials TCP connection to 192.168.1.100:502
   - Connection successful
   - Returns ObservedState{State: "up", Connected: true}
```

**No templates in this scenario** (simple state control only)

### 8.2 Bridge → Benthos (Templated Config)

**User Config:**

```yaml
templates:
  - id: "modbus-tcp"
    config: |
      input:
        label: "modbus_{{ .name }}"
        modbus_tcp:
          address: "{{ .IP }}:{{ .PORT }}"
      
      output:
        broker:
          outputs:
            - kafka:
                addresses: ["{{ .global.kafka_brokers }}"]
                topic: "umh.v1.{{ .location_path }}.{{ .name }}"

protocolConverters:
  - id: "bridge-plc"
    name: "Factory-PLC"
    templateRef: "modbus-tcp"
    connection:
      variables:
        IP: "192.168.1.100"
        PORT: 502
    location:
      - line: "Line-A"
      - cell: "Cell-5"

agent:
  location:
    - enterprise: "ACME"
    - site: "Factory"
  
  global:
    kafka_brokers: "localhost:9092"
```

**Parent Worker (Bridge):**

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    parentState := "active"
    
    // Merge location hierarchy
    agentLocation := map[string]string{
        "enterprise": "ACME",
        "site":       "Factory",
    }
    bridgeLocation := map[string]string{
        "line": "Line-A",
        "cell": "Cell-5",
    }
    location := mergeLocations(agentLocation, bridgeLocation)
    locationPath := "ACME.Factory.Line-A.Cell-5"
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef,  // "modbus-tcp"
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  parentState,
                            "name":          "Factory-PLC_read",
                            "IP":            "192.168.1.100",
                            "PORT":          502,
                            "location":      location,
                            "location_path": locationPath,
                        },
                        Global: map[string]any{
                            "kafka_brokers": "localhost:9092",
                        },
                    },
                },
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}
```

**Child Worker (Benthos):**

```go
type BenthosWorker struct {
    logger         *zap.Logger
    templateStore  TemplateStore
    lastObserved   *ObservedState
}

type BenthosUserSpec struct {
    TemplateName string
    Variables    VariableBundle
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    // scope = {
    //   "parent_state": "active",
    //   "name": "Factory-PLC_read",
    //   "IP": "192.168.1.100",
    //   "PORT": 502,
    //   "location_path": "ACME.Factory.Line-A.Cell-5",
    //   "global": {
    //     "kafka_brokers": "localhost:9092"
    //   }
    // }
    
    // 1. Load template
    template := w.templateStore.Get(userSpec.TemplateName)
    
    // 2. Render template
    config, err := RenderTemplate(template, scope)
    if err != nil {
        w.logger.Error("Failed to render template", zap.Error(err))
        return DesiredState{State: "error"}
    }
    
    // 3. Compute state from parent state
    parentState, _ := scope["parent_state"].(string)
    state := "stopped"
    if parentState == "active" {
        state = "running"
    }
    
    return DesiredState{
        State:  state,
        Config: config,
    }
}

func (w *BenthosWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Check Benthos process health
    // Check metrics (input/output connections, message throughput)
    
    observed := ObservedState{
        State:   "running",
        Healthy: true,
        Metrics: map[string]float64{
            "input_connections":  1,
            "output_connections": 1,
            "messages_processed": 1234,
        },
    }
    
    w.lastObserved = &observed
    
    return observed, nil
}
```

**Rendered Config:**

```yaml
input:
  label: "modbus_Factory-PLC_read"
  modbus_tcp:
    address: "192.168.1.100:502"

output:
  broker:
    outputs:
      - kafka:
          addresses: ["localhost:9092"]
          topic: "umh.v1.ACME.Factory.Line-A.Cell-5.Factory-PLC_read"
```

**Supervisor Tick Flow:**

```
1. Bridge supervisor derives desired state
2. BridgeWorker creates Benthos ChildSpec with:
   - TemplateName: "modbus-tcp"
   - Variables.User: {IP, PORT, name, location_path, parent_state}
   - Variables.Global: {kafka_brokers}
3. Bridge supervisor reconciles children:
   - Creates BenthosWorker
   - Passes BenthosUserSpec to child supervisor
4. Bridge supervisor applies StateMapping:
   - Parent state = "active" → Child desired state = "running"
5. Bridge supervisor ticks Benthos child
6. Benthos supervisor calls BenthosWorker.DeriveDesiredState(spec)
7. BenthosWorker flattens variables to scope
8. BenthosWorker loads template "modbus-tcp"
9. BenthosWorker renders template with scope:
   - {{ .IP }} → "192.168.1.100"
   - {{ .PORT }} → 502
   - {{ .name }} → "Factory-PLC_read"
   - {{ .location_path }} → "ACME.Factory.Line-A.Cell-5"
   - {{ .global.kafka_brokers }} → "localhost:9092"
10. BenthosWorker returns DesiredState{State: "running", Config: renderedConfig}
11. Benthos supervisor compares desired vs observed
12. If config changed, supervisor triggers Benthos process restart
```

**Key points:**
- Template rendering happens in BenthosWorker, NOT in supervisor
- Variables flattened to top-level ({{ .IP }} not {{ .user.IP }})
- Location path auto-computed by parent
- Global variables passed through from agent

### 8.3 OPC UA Browser → N Server Browsers (Dynamic Children)

**User Config:**

```yaml
opcuaBrowser:
  servers:
    - "opc.tcp://192.168.1.10:4840"
    - "opc.tcp://192.168.1.11:4840"
    - "opc.tcp://192.168.1.12:4840"
```

**Parent Worker (OPC UA Browser):**

```go
type OPCUABrowserWorker struct {
    logger       *zap.Logger
    lastObserved *ObservedState
}

type OPCUABrowserUserSpec struct {
    Servers   []string
    Variables VariableBundle
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec OPCUABrowserUserSpec) DesiredState {
    parentState := "active"
    
    // Create child spec for each server
    childSpecs := make([]ChildSpec, len(userSpec.Servers))
    for i, serverAddr := range userSpec.Servers {
        childSpecs[i] = ChildSpec{
            Name:       serverAddr,  // Unique name per server
            WorkerType: WorkerTypeOPCUAServerBrowser,
            UserSpec: OPCUAServerBrowserUserSpec{
                ServerAddress: serverAddr,
                BrowseDepth:   5,
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state": parentState,
                        "server_addr":  serverAddr,
                    },
                    Global: userSpec.Variables.Global,
                },
            },
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        }
    }
    
    return DesiredState{
        State:         parentState,
        ChildrenSpecs: childSpecs,
    }
}
```

**Child Worker (OPC UA Server Browser):**

```go
type OPCUAServerBrowserWorker struct {
    logger       *zap.Logger
    opcuaClient  *opcua.Client
    lastObserved *ObservedState
}

type OPCUAServerBrowserUserSpec struct {
    ServerAddress string
    BrowseDepth   int
    Variables     VariableBundle
}

func (w *OPCUAServerBrowserWorker) DeriveDesiredState(userSpec OPCUAServerBrowserUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    parentState, _ := scope["parent_state"].(string)
    
    // Compute state from parent and observations
    state := "stopped"
    if parentState == "active" {
        if w.lastObserved != nil && w.lastObserved.ServerReachable {
            state = "browsing"
        } else {
            state = "connecting"
        }
    }
    
    return DesiredState{
        State:         state,
        ChildrenSpecs: nil,
    }
}

func (w *OPCUAServerBrowserWorker) CollectObservedState(ctx context.Context, userSpec OPCUAServerBrowserUserSpec) (ObservedState, error) {
    // Try to connect to OPC UA server
    err := w.opcuaClient.Connect(ctx, userSpec.ServerAddress)
    
    observed := ObservedState{
        State:           "connecting",
        ServerReachable: false,
        Healthy:         false,
    }
    
    if err == nil {
        // Browse nodes
        nodes, err := w.opcuaClient.Browse(ctx, userSpec.BrowseDepth)
        
        if err == nil {
            observed.State = "browsing"
            observed.ServerReachable = true
            observed.Healthy = true
            observed.NodesDiscovered = len(nodes)
        }
    }
    
    w.lastObserved = &observed
    
    return observed, nil
}
```

**Supervisor Tick Flow:**

```
1. Browser supervisor derives desired state
2. BrowserWorker creates 3 ChildSpecs (one per server)
3. Browser supervisor reconciles children:
   - Creates 3 ServerBrowserWorker instances
   - Each gets unique name (server address)
   - Each gets own UserSpec with server address
4. Browser supervisor applies StateMapping:
   - Parent state = "active"
   - All children desired state = "browsing"
5. Browser supervisor ticks all 3 children in parallel
6. Each child:
   - Derives state ("browsing" if connected, "connecting" if not)
   - Collects observed state (tries to connect and browse)
```

**Dynamic children characteristics:**
- Number of children varies (1-N based on config)
- Each child gets unique name (server address)
- All children have same type (ServerBrowserWorker)
- Supervisor creates/removes children when config changes

**Example config change:**

```yaml
# Before: 3 servers
servers:
  - "opc.tcp://192.168.1.10:4840"
  - "opc.tcp://192.168.1.11:4840"
  - "opc.tcp://192.168.1.12:4840"

# After: 2 servers (removed .12)
servers:
  - "opc.tcp://192.168.1.10:4840"
  - "opc.tcp://192.168.1.11:4840"
```

**Reconciliation:**
1. Next tick: BrowserWorker returns only 2 ChildSpecs
2. Supervisor.reconcileChildren() compares:
   - Desired: [.10, .11]
   - Current: [.10, .11, .12]
3. Supervisor removes child ".12":
   - Calls childSupervisor.Shutdown()
   - Deletes from s.children map
4. Only 2 children tick on next cycle

---

## 9. Comparison Table

### 9.1 Approaches Compared

| Aspect | Option A: Parent Sets State | Option B: Child Computes | Option C: StateMapping (CHOSEN) |
|--------|----------------------------|--------------------------|--------------------------------|
| **Parent Control** | Direct (parent sets child state) | None (child autonomous) | Declarative (mapping) |
| **Child Autonomy** | ❌ None | ✅ Full | ✅ Partial (can override) |
| **Decoupling** | ❌ Tight (parent knows child internals) | ✅ Loose | ✅ Moderate |
| **Clarity** | ⚠️ Imperative | ⚠️ Implicit | ✅ Declarative |
| **Safety Checks** | ❌ No (child can't override) | ✅ Yes | ✅ Yes (child can override) |
| **Code Location** | Parent | Child | Parent (mapping) + Supervisor (apply) |
| **Flexibility** | ❌ Low | ✅ High | ✅ High |
| **Complexity** | Low | Low | Medium |
| **Kubernetes Match** | ❌ No | Partial | ✅ Yes |

### 9.2 Template Rendering Approaches

| Aspect | Centralized (Supervisor) | Distributed (Worker) (CHOSEN) |
|--------|-------------------------|------------------------------|
| **Who Renders** | Supervisor | Worker |
| **When Renders** | Before calling DeriveDesiredState | Inside DeriveDesiredState |
| **Who Knows Template** | Supervisor (needs template schema) | Worker (owns template) |
| **Decoupling** | ❌ Tight (supervisor knows worker details) | ✅ Loose (worker owns rendering) |
| **Flexibility** | ❌ Low (all workers must use templates) | ✅ High (workers opt-in) |
| **Performance** | Same | Same |
| **Code Location** | Supervisor (one place) | Workers (distributed) |
| **Cache Complexity** | Medium | Low (per-worker) |

### 9.3 Type Safety Approaches

| Aspect | Loose Maps | Strict Types (CHOSEN) |
|--------|-----------|----------------------|
| **UserSpec Fields** | map[string]any | struct fields |
| **Compile-time Checks** | ❌ No | ✅ Yes |
| **IDE Autocomplete** | ❌ No | ✅ Yes |
| **Runtime Errors** | ✅ More | ❌ Fewer |
| **Serialization** | ✅ Easy | ⚠️ Needs care |
| **Flexibility** | ✅ High | ⚠️ Medium |
| **Type Safety** | ❌ None | ✅ Full |
| **Variables** | ✅ Loose (for templates) | ✅ Loose (for templates) |

### 9.4 Hierarchical Composition Approaches

| Aspect | Separate Methods | Specs in DesiredState (CHOSEN) |
|--------|------------------|------------------------------|
| **API Methods** | 2 (DeriveDesiredState + DeclareChildren) | 1 (DeriveDesiredState) |
| **Worker Code** | 20 lines | 15 lines |
| **Supervisor Code** | 70 lines | 90 lines |
| **Serializable** | ❌ Partial (no children) | ✅ Full |
| **TriangularStore** | ❌ Incomplete | ✅ Complete |
| **Kubernetes Match** | Partial | ✅ Exact |
| **Type Safety** | ✅ Strong | ⚠️ Weak (string types) |
| **Dependencies** | None | WorkerFactory |
| **Performance** | 390ms/hour | 3.6s/hour (9x worse) |
| **State Restoration** | ❌ Manual | ✅ Automatic |

### 9.5 Final Recommendations

**State Control: StateMapping + Child Autonomy**
- ✅ Best of both worlds (parent control + child safety)
- ✅ Declarative (clear in code)
- ✅ Flexible (child can override)

**Template Rendering: Distributed (Worker)**
- ✅ Worker owns rendering logic
- ✅ Supervisor doesn't know template details
- ✅ Workers without templates skip rendering

**Type Safety: Strict UserSpec + Loose Variables**
- ✅ Compile-time validation for worker fields
- ✅ Template flexibility for dynamic values
- ✅ Clear contracts

**Hierarchical Composition: Specs in DesiredState**
- ✅ Kubernetes pattern match
- ✅ Full serialization
- ✅ Single method interface
- ⚠️ Requires WorkerFactory (acceptable trade-off)

---

## Appendix A: WorkerFactory Pattern

### A.1 Interface

```go
type WorkerFactory interface {
    CreateWorker(workerType string, userSpec UserSpec) (Worker, error)
}
```

### A.2 Implementation

```go
type DefaultWorkerFactory struct {
    logger *zap.Logger
}

func (f *DefaultWorkerFactory) CreateWorker(workerType string, userSpec UserSpec) (Worker, error) {
    switch workerType {
    case WorkerTypeConnection:
        return newConnectionWorker(userSpec, f.logger), nil
    case WorkerTypeBenthos:
        return newBenthosWorker(userSpec, f.logger), nil
    case WorkerTypeOPCUABrowser:
        return newOPCUABrowserWorker(userSpec, f.logger), nil
    case WorkerTypeOPCUAServerBrowser:
        return newOPCUAServerBrowserWorker(userSpec, f.logger), nil
    default:
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
}
```

### A.3 Type Constants

```go
const (
    WorkerTypeConnection         = "ConnectionWorker"
    WorkerTypeBenthos           = "BenthosWorker"
    WorkerTypeOPCUABrowser      = "OPCUABrowserWorker"
    WorkerTypeOPCUAServerBrowser = "OPCUAServerBrowserWorker"
)
```

---

## Appendix B: Variable Flattening

### B.1 Flatten() Method

```go
func (vb VariableBundle) Flatten() map[string]any {
    out := map[string]any{}
    
    // User variables → top-level (no namespace prefix)
    for k, v := range vb.User {
        out[k] = v
    }
    
    // Global and Internal → nested (explicit namespace)
    if len(vb.Global) > 0 {
        out["global"] = vb.Global
    }
    if len(vb.Internal) > 0 {
        out["internal"] = vb.Internal
    }
    
    return out
}
```

### B.2 Template Access

```yaml
# User variables (flattened):
{{ .IP }}                   # ✅ Short, intuitive
{{ .PORT }}                 # ✅ No namespace needed
{{ .parent_state }}         # ✅ Direct access
{{ .location_path }}        # ✅ Computed value

# Global/Internal (nested):
{{ .global.api_endpoint }}  # ✅ Explicit namespace
{{ .internal.id }}          # ✅ Runtime metadata
```

---

## Appendix C: Location Merging

### C.1 mergeLocations()

```go
func mergeLocations(parent, child map[string]string) map[string]string {
    merged := make(map[string]string)
    
    // Copy parent
    for k, v := range parent {
        merged[k] = v
    }
    
    // Add child (overwrite if collision)
    for k, v := range child {
        merged[k] = v
    }
    
    return merged
}
```

### C.2 computeLocationPath()

```go
func computeLocationPath(location map[string]string) string {
    // ISA-95 hierarchy
    hierarchy := []string{"enterprise", "site", "area", "line", "cell", "bridge"}
    
    parts := []string{}
    for _, key := range hierarchy {
        if val, ok := location[key]; ok && val != "" {
            parts = append(parts, val)
        } else {
            parts = append(parts, "unknown")  // Gap filling
        }
    }
    
    return strings.Join(parts, ".")
}
```

### C.3 Example

```go
agentLocation := map[string]string{
    "enterprise": "ACME",
    "site":       "Factory",
}

bridgeLocation := map[string]string{
    "line": "Line-A",
    "cell": "Cell-5",
}

merged := mergeLocations(agentLocation, bridgeLocation)
// merged = {
//   "enterprise": "ACME",
//   "site":       "Factory",
//   "line":       "Line-A",
//   "cell":       "Cell-5",
// }

path := computeLocationPath(merged)
// path = "ACME.Factory.unknown.Line-A.Cell-5"
//                       ↑
//                  "area" missing, filled with "unknown"
```

---

## Summary

This document provides the complete, unambiguous API definition for FSMv2's DeriveDesiredState.

**Key Takeaways:**

1. **Single method interface** - DeriveDesiredState returns DesiredState with ChildrenSpecs
2. **StateMapping for control** - Parent declares mapping, supervisor applies, child can override
3. **Templates in workers** - Each worker renders own templates in DeriveDesiredState
4. **Strict + Loose types** - UserSpec typed, Variables loose for flexibility
5. **Standalone workers** - All inputs in UserSpec, no parent reference needed

**Ready for implementation** - All open questions resolved, patterns defined, examples provided.
