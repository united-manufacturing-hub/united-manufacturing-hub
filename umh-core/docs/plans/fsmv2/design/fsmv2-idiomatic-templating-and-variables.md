# FSMv2: Idiomatic Templating and Variables System

**Date:** 2025-11-02
**Status:** Design Proposal
**Related:**
- `fsmv1-templating-and-variables.md` - FSMv1 patterns and implementation
- `fsmv2-child-specs-in-desired-state.md` - Hierarchical composition architecture
- `fsmv2-templating-vs-statemapping.md` - Decision to use Variables over StateMapping

## Design Principles

### What Makes Templating/Variables "Idiomatic" for FSMv2?

**1. Natural for Users (Writing YAML)**
- Top-level variable access feels intuitive: `{{ .IP }}` not `{{ .user.IP }}`
- Location path auto-computed from hierarchy
- Parent state available as regular variable
- No surprises: What you define is what you reference

**2. Natural for Developers (Implementing Workers)**
- Variables flow downward (parent → child), never upward
- Workers are standalone: Child doesn't reach into parent
- Parent controls child by passing variables, not by setting child state
- Templates execute where data lives (in worker's DeriveDesiredState)

**3. Unsurprising Patterns**
- Matches FSMv1 (proven in production)
- Follows Kubernetes style (parent passes config to child)
- Type safety where it matters (Go structs), flexibility where it helps (templates)
- No magic: Explicit variable passing, explicit template rendering

---

## Variable Structure

### Three-Tier Namespace (Adopted from FSMv1)

```go
type VariableBundle struct {
    User     map[string]any `yaml:"user,omitempty"`      // User-defined + parent state
    Global   map[string]any `yaml:"global,omitempty"`    // Fleet-wide variables
    Internal map[string]any `yaml:"internal,omitempty"`  // Runtime metadata
}
```

**Namespace Purposes:**

| Namespace | Source | Examples | Serialized |
|-----------|--------|----------|------------|
| **User** | Parent worker, config YAML, computed values | `IP`, `PORT`, `parent_state`, `location`, `location_path` | ✅ Yes |
| **Global** | Central management loop | API endpoints, cluster settings, fleet configuration | ✅ Yes |
| **Internal** | Supervisor runtime | `id`, `bridged_by`, timestamps | ❌ No (runtime-only) |

### Variable Flattening (Critical Pattern)

**User variables become TOP-LEVEL for convenience:**

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

**Template access:**

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

**Why flatten User?**
- User experience: No namespace clutter in templates
- Matches intuition: "My variables are at the top level"
- FSMv1 proven: Production workloads validate this pattern

### Variable Scoping Rules

**Per-Worker Variables:**
- Each worker has its own VariableBundle in UserSpec
- Variables don't leak between siblings
- Parent passes subset of variables to each child

**Hierarchical Variables:**
- Location: Parent location + child location = merged hierarchy
- Parent state: Parent passes `parent_state` to child
- Parent observed: Optional, for child sanity checks

**Example: Bridge → Connection**

```go
// Parent (Bridge) worker
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name: "connection",
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Connection-specific variables
                            "IP":   userSpec.Variables.User["IP"],
                            "PORT": userSpec.Variables.User["PORT"],
                            
                            // Parent context
                            "parent_state": "active",
                            
                            // Location merging
                            "location": mergeLocations(
                                userSpec.Variables.User["location"],
                                map[string]string{"cell": "Cell-5"},
                            ),
                        },
                    },
                },
            },
        },
    }
}
```

---

## Template Execution

### Where Templates Execute

**Distributed execution at the worker level:**

```
┌──────────────────────────────────────┐
│ Parent Worker                        │
│                                      │
│ DeriveDesiredState(userSpec) {       │
│   // 1. Render parent's templates   │ ← Template execution HERE
│   config := RenderTemplate(          │
│       w.configTemplate,              │
│       userSpec.Variables.Flatten()   │
│   )                                  │
│                                      │
│   // 2. Create child specs           │
│   return DesiredState{               │
│       ChildrenSpecs: []ChildSpec{    │
│           {UserSpec: childVars},     │
│       },                             │
│   }                                  │
│ }                                    │
└──────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Child Worker                         │
│                                      │
│ DeriveDesiredState(userSpec) {       │
│   // 3. Render child's templates    │ ← Template execution HERE
│   config := RenderTemplate(          │
│       w.configTemplate,              │
│       userSpec.Variables.Flatten()   │
│   )                                  │
│                                      │
│   // 4. Compute child state          │
│   state := "down"                    │
│   if userSpec.Variables["parent_state"] == "active" {
│       state = "up"                   │
│   }                                  │
│                                      │
│   return DesiredState{State: state}  │
│ }                                    │
└──────────────────────────────────────┘
```

**Key Decision: Each worker renders its own templates**
- NOT in supervisor (supervisor doesn't know worker's template needs)
- NOT in central service (defeats distributed architecture)
- IN worker's DeriveDesiredState (worker knows what it needs)

### Who Executes Templates

**The worker itself, in DeriveDesiredState:**

```go
type ConnectionWorker struct {
    configTemplate string  // Could be struct, proto definition, etc.
}

func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // 1. Flatten variables for template access
    scope := userSpec.Variables.Flatten()

    // 2. Render templates (if worker needs them)
    config, err := RenderTemplate(w.configTemplate, scope)
    if err != nil {
        // Handle template errors
    }

    // 3. Compute state from parent state
    state := "down"
    if parentState, ok := scope["parent_state"].(string); ok {
        if parentState == "active" || parentState == "idle" {
            state = "up"
        }
    }

    // 4. Return desired state
    return DesiredState{
        State:    state,
        Config:   config,  // If worker needs config
    }
}
```

**Workers that DON'T need templates:** Just compute state, skip rendering

**Workers that DO need templates:** Render in DeriveDesiredState

### When Templates Render

**Every tick, in DeriveDesiredState:**

```
┌─────────────────────────────────┐
│ Supervisor.Tick()               │
│                                 │
│ 1. Get UserSpec                 │
│ 2. Call DeriveDesiredState() ───┼──→ Worker renders templates HERE
│ 3. Compare desired vs observed  │
│ 4. Tick children                │
└─────────────────────────────────┘
```

**Why every tick?**
- Variables might change (IP address updated)
- Parent state might change (active → idle)
- Location might change (line moved)
- Simplicity: No caching complexity, no invalidation logic

**Performance impact:**
- Template rendering ~100μs per worker
- 100 workers × 1 tick/sec = 10ms CPU/sec
- Negligible for production workloads

**If optimization needed later:**
- Cache hash of VariableBundle
- Skip rendering if hash unchanged
- Premature optimization avoided initially

---

## Type Strategy

### Strict Types in UserSpec (NOT loose maps)

**Core Decision: UserSpec uses Go struct fields, not map[string]any**

```go
// ✅ CORRECT: Strict Go types
type ConnectionUserSpec struct {
    IPAddress string
    Port      uint16
    Timeout   time.Duration
    Variables VariableBundle  // Only place for loose maps
}

// ❌ WRONG: Loose maps everywhere
type UserSpec struct {
    Config map[string]any  // Loses type safety
}
```

**Why strict types?**
1. **Compile-time validation** - Typos caught before runtime
2. **IDE autocomplete** - Developer experience
3. **Clear contracts** - What fields does worker expect?
4. **Type safety** - Can't pass string to uint16 field

### Templates Use Loose Types (map[string]any)

**Variables contain loose maps for flexibility:**

```go
type VariableBundle struct {
    User     map[string]any  // Flexible for templates
    Global   map[string]any
    Internal map[string]any
}
```

**Why loose maps in Variables?**
1. **Template flexibility** - Can reference any variable
2. **Parent-child decoupling** - Parent doesn't know child's types
3. **Serialization** - JSON/YAML marshal easily
4. **Dynamic values** - Computed location paths, parent states

### No Template → Runtime Type Conversion (Unlike FSMv1)

**FSMv1 needs two-phase conversion because:**
- Config YAML contains templates: `port: "{{ .PORT }}"`
- Runtime needs proper types: `port: 443` (uint16)
- Must convert template string → runtime type

**FSMv2 doesn't need this because:**
- UserSpec already has proper types (Go structs)
- Templates only in worker-specific configs (optional)
- Worker decides if/when to render templates

**Example: Worker with AND without templates**

```go
// Worker WITHOUT templates (no rendering needed)
type SimpleWorker struct{}

func (w *SimpleWorker) DeriveDesiredState(userSpec SimpleUserSpec) DesiredState {
    // Direct use of typed fields
    timeout := userSpec.Timeout
    if timeout > 10*time.Second {
        return DesiredState{State: "slow"}
    }
    return DesiredState{State: "fast"}
}

// Worker WITH templates (renders in DeriveDesiredState)
type TemplatedWorker struct {
    configTemplate ConfigTemplate
}

func (w *TemplatedWorker) DeriveDesiredState(userSpec TemplatedUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Render if needed
    config, _ := RenderTemplate(w.configTemplate, scope)
    
    return DesiredState{
        State:  "active",
        Config: config,
    }
}
```

**Key difference from FSMv1:**
- FSMv1: Config → Template (string) → Runtime (typed) [2-phase]
- FSMv2: UserSpec (typed) → Template (optional) [1-phase]

---

## Parent-Child Variable Flow

### Parent Passes Variables to Child

**Pattern: Parent computes child's VariableBundle in DeriveDesiredState**

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    // 1. Compute parent's desired state
    parentState := "active"
    
    // 2. Get parent's observed state (optional, for child sanity checks)
    parentObserved := w.lastObservedState
    
    // 3. Compute location hierarchy
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    childLocation := map[string]string{"cell": "Cell-5"}
    mergedLocation := mergeLocations(parentLocation, childLocation)
    locationPath := computeLocationPath(mergedLocation)
    
    // 4. Create child spec with variables
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            {
                Name: "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Connection config (from parent's UserSpec)
                            "IP":   userSpec.IPAddress,
                            "PORT": userSpec.Port,
                            
                            // Parent context
                            "parent_state":    parentState,
                            "parent_observed": parentObserved,  // Optional
                            
                            // Location hierarchy
                            "location":      mergedLocation,
                            "location_path": locationPath,
                        },
                        Global: userSpec.Variables.Global,  // Pass through
                        // Internal set by supervisor
                    },
                },
            },
        },
    }
}
```

**What parent controls:**
- ✅ Child's UserSpec (typed fields)
- ✅ Child's Variables (User map)
- ✅ What variables child sees
- ❌ Child's state (child computes from parent_state)

### Child Computes State from Parent Variables

**Pattern: Child reads parent_state, computes own state**

```go
func (w *ConnectionWorker) DeriveDesiredState(userSpec ConnectionUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Read parent state from variables
    parentState, ok := scope["parent_state"].(string)
    if !ok {
        parentState = "unknown"
    }
    
    // Compute child state based on parent state
    state := "down"
    switch parentState {
    case "active", "idle":
        state = "up"
    case "error", "stopping":
        state = "down"
    default:
        state = "unknown"
    }
    
    // Optional: Check parent observed for sanity
    if parentObserved, ok := scope["parent_observed"].(ObservedState); ok {
        if !parentObserved.Healthy {
            state = "down"  // Parent unhealthy, stay down
        }
    }
    
    return DesiredState{State: state}
}
```

**Why child computes state (not parent setting it)?**
1. **Child autonomy** - Child can react to own observations
2. **Decoupling** - Parent doesn't need to know child's state space
3. **Complex logic** - Child can implement multi-condition checks
4. **Standalone workers** - Child doesn't reach into parent

### Children Are Standalone (No Parent Context)

**Critical Design Constraint:**

```go
// ✅ CORRECT: Child standalone, parent passes everything
func (w *ChildWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    parentState := scope["parent_state"]  // From variables
    // ...
}

// ❌ WRONG: Child reaches into parent
func (w *ChildWorker) DeriveDesiredState(userSpec UserSpec, parentWorker *ParentWorker) DesiredState {
    parentState := parentWorker.GetState()  // Tight coupling!
    // ...
}
```

**Why standalone?**
- Testability: Can test child without parent
- Composability: Can move child to different parent
- Clarity: All child inputs in UserSpec
- Serialization: ChildSpec fully captures child config

---

## Observed State in Templates

### Can Templates Access Observed State?

**Short answer: NO (templates don't access observed state)**

**Why not?**

1. **Causality issue:**
   ```
   DeriveDesiredState(userSpec) → DesiredState
   CollectObservedState(ctx) → ObservedState
   ```
   - Desired state computed BEFORE observed state collected
   - Templates in DeriveDesiredState can't access future observations
   - Would create circular dependency

2. **Desired vs Observed:**
   - Desired state: "What SHOULD be" (based on user intent)
   - Observed state: "What IS" (based on reality)
   - Templates express desired config, not reactive logic

3. **Reactive logic belongs in code:**
   ```go
   // ❌ WRONG: Template accessing observed state
   template: "{{ if .observed.healthy }}up{{ else }}down{{ end }}"
   
   // ✅ CORRECT: Go code checking observed state
   func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
       state := "up"
       
       // Check last observed state if needed
       if w.lastObserved != nil && !w.lastObserved.Healthy {
           state = "down"
       }
       
       return DesiredState{State: state}
   }
   ```

### Parent CAN Pass Observed State to Child

**Use Case: Child sanity checks parent health**

```go
// Parent passes its observed state to child
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    parentObserved := w.lastObservedState  // From previous tick
    
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_observed": parentObserved,  // Pass as variable
                        },
                    },
                },
            },
        },
    }
}

// Child uses parent observed for sanity checks
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    state := "up"
    
    // Check if parent is healthy
    if parentObserved, ok := scope["parent_observed"].(ObservedState); ok {
        if !parentObserved.Healthy {
            state = "down"  // Parent unhealthy, stay down
        }
    }
    
    return DesiredState{State: state}
}
```

**Pattern: Parent observed from PREVIOUS tick**
- Parent collects observed state at end of tick N
- Parent passes observed state to child at start of tick N+1
- Child uses parent's OLD observed state (not current)
- No circular dependency

---

## User Experience (YAML Config)

### User-Facing Configuration

**Simple bridge example:**

```yaml
# User writes this
protocolConverters:
  - id: "bridge-123"
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

**Templates defined separately:**

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

**What user sees:**
- Define variables in `connection.variables`
- Reference variables in templates: `{{ .IP }}`
- Location hierarchy auto-computed to `{{ .location_path }}`
- No namespace clutter: NOT `{{ .user.IP }}` or `{{ .variables.IP }}`

### User Mental Model

**User thinks:**
1. "I define IP=192.168.1.100"
2. "I reference it as {{ .IP }}"
3. "Location auto-computes from hierarchy"
4. "Templates expand when bridge starts"

**System does:**
1. Parse YAML → UserSpec with Variables.User["IP"] = "192.168.1.100"
2. Flatten() → scope["IP"] = "192.168.1.100"
3. Merge locations → scope["location_path"] = "ACME.Factory.Line-A.Cell-5"
4. RenderTemplate(template, scope) → Replace {{ .IP }} with "192.168.1.100"

**No surprises:** What user expects is what happens

---

## Developer Experience (Worker Code)

### Implementing a Worker (Without Templates)

**Simple worker that doesn't need templates:**

```go
type ConnectionWorker struct {
    logger *zap.Logger
}

type ConnectionUserSpec struct {
    IPAddress string
    Port      uint16
    Variables VariableBundle
}

func (w *ConnectionWorker) DeriveDesiredState(userSpec ConnectionUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Compute state from parent state
    parentState := scope["parent_state"].(string)
    state := "down"
    if parentState == "active" {
        state = "up"
    }
    
    return DesiredState{State: state}
}

func (w *ConnectionWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Check TCP connection
    conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ...))
    if err != nil {
        return ObservedState{State: "down", Healthy: false}, nil
    }
    conn.Close()
    return ObservedState{State: "up", Healthy: true}, nil
}
```

**Developer experience:**
- ✅ Clear: UserSpec typed, Variables loose
- ✅ Simple: No templating needed if not used
- ✅ Standalone: All inputs in UserSpec

### Implementing a Worker (With Templates)

**Worker that renders Benthos config from template:**

```go
type BenthosWorker struct {
    logger         *zap.Logger
    configTemplate BenthosConfigTemplate
}

type BenthosUserSpec struct {
    Name      string
    Variables VariableBundle
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // 1. Render Benthos config from template
    config, err := RenderTemplate(w.configTemplate, scope)
    if err != nil {
        w.logger.Error("Failed to render config", zap.Error(err))
        return DesiredState{State: "error"}
    }
    
    // 2. Compute state from parent state
    parentState := scope["parent_state"].(string)
    state := "stopped"
    if parentState == "active" {
        state = "running"
    }
    
    return DesiredState{
        State:  state,
        Config: config,
    }
}
```

**Developer experience:**
- ✅ Explicit: Template rendering visible in DeriveDesiredState
- ✅ Flexible: Worker decides what/when to template
- ✅ Type-safe: Config result typed (BenthosConfig)

### Declaring Children with Variables

**Parent worker creating child specs:**

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    // Compute parent state
    parentState := "active"
    
    // Merge location hierarchy
    location := mergeLocations(
        userSpec.Variables.User["location"].(map[string]string),
        map[string]string{"bridge": userSpec.Name},
    )
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: ConnectionUserSpec{
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": parentState,
                            "location":     location,
                            "location_path": computePath(location),
                        },
                        Global: userSpec.Variables.Global,  // Pass through
                    },
                },
            },
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    Name: userSpec.Name + "_read",
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": parentState,
                            "name":         userSpec.Name + "_read",
                            "location":     location,
                            "location_path": computePath(location),
                            "IP":           userSpec.IPAddress,
                            "PORT":         userSpec.Port,
                        },
                        Global: userSpec.Variables.Global,
                    },
                },
            },
        },
    }
}
```

**Developer experience:**
- ✅ Explicit: Parent decides what variables child gets
- ✅ Clear: All variable sources visible
- ✅ Type-safe: UserSpec typed, Variables loose for flexibility

---

## Code Examples (Complete Scenarios)

### Scenario 1: Bridge → Connection → No Templates

**User YAML:**
```yaml
protocolConverters:
  - id: "bridge-plc"
    connection:
      variables:
        IP: "192.168.1.100"
        PORT: 502
```

**Parent Worker:**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: ConnectionUserSpec{
                    IPAddress: userSpec.Variables.User["IP"].(string),
                    Port:      uint16(userSpec.Variables.User["PORT"].(float64)),
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": "active",
                        },
                    },
                },
            },
        },
    }
}
```

**Child Worker:**
```go
func (w *ConnectionWorker) DeriveDesiredState(userSpec ConnectionUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    state := "down"
    if scope["parent_state"] == "active" {
        state = "up"
    }
    
    return DesiredState{State: state}
}
```

**No templates:** Simple state computation from parent state

---

### Scenario 2: Bridge → Benthos → With Templates

**User YAML:**
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

protocolConverters:
  - id: "bridge-plc"
    templateRef: "modbus-tcp"
    connection:
      variables:
        IP: "192.168.1.100"
        PORT: 502
    location:
      - line: "Line-A"
```

**Parent Worker:**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    location := mergeLocations(
        userSpec.AgentLocation,  // From agent: {"enterprise": "ACME", "site": "Factory"}
        userSpec.BridgeLocation, // From bridge: {"line": "Line-A"}
    )
    locationPath := "ACME.Factory.Line-A"
    
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  "active",
                            "IP":            "192.168.1.100",
                            "PORT":          502,
                            "name":          "bridge-plc_read",
                            "location":      location,
                            "location_path": locationPath,
                        },
                    },
                },
            },
        },
    }
}
```

**Child Worker:**
```go
func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // 1. Load template
    template := w.templateStore.Get(userSpec.TemplateName)
    
    // 2. Render template
    config, err := RenderTemplate(template, scope)
    if err != nil {
        return DesiredState{State: "error"}
    }
    
    // 3. Compute state
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
  modbus_tcp:
    address: "192.168.1.100:502"
output:
  kafka:
    topic: "umh.v1.ACME.Factory.Line-A.bridge-plc_read"
```

---

### Scenario 3: OPC UA Browser → Multiple Server Browsers

**User YAML:**
```yaml
opcuaBrowser:
  servers:
    - "opc.tcp://192.168.1.10:4840"
    - "opc.tcp://192.168.1.11:4840"
```

**Parent Worker:**
```go
func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec OPCUABrowserUserSpec) DesiredState {
    childSpecs := []ChildSpec{}
    
    for _, serverAddr := range userSpec.Servers {
        childSpecs = append(childSpecs, ChildSpec{
            Name:       serverAddr,  // Unique child per server
            WorkerType: WorkerTypeOPCUAServerBrowser,
            UserSpec: OPCUAServerBrowserUserSpec{
                ServerAddress: serverAddr,
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state": "active",
                        "server_addr":  serverAddr,
                    },
                },
            },
        })
    }
    
    return DesiredState{
        State:         "active",
        ChildrenSpecs: childSpecs,
    }
}
```

**Child Worker:**
```go
func (w *OPCUAServerBrowserWorker) DeriveDesiredState(userSpec OPCUAServerBrowserUserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    state := "stopped"
    if scope["parent_state"] == "active" {
        state = "browsing"
    }
    
    return DesiredState{State: state}
}
```

**Dynamic children:** Parent creates N children based on config

---

## Comparison to FSMv1

### What FSMv2 Adopts from FSMv1

| Pattern | FSMv1 | FSMv2 | Notes |
|---------|-------|-------|-------|
| **VariableBundle structure** | ✅ User/Global/Internal | ✅ Same | Proven in production |
| **Flatten() method** | ✅ User top-level, G/I nested | ✅ Same | User convenience |
| **RenderTemplate() strict mode** | ✅ missingkey=error | ✅ Same | Catches undefined vars |
| **Location merging** | ✅ Agent + PC | ✅ Parent + child | Hierarchical |
| **Gap filling with "unknown"** | ✅ Fill location gaps | ✅ Same | Graceful degradation |
| **Template syntax** | ✅ Go text/template | ✅ Same | No custom functions |

### What FSMv2 Changes from FSMv1

| Aspect | FSMv1 | FSMv2 | Why Changed |
|--------|-------|-------|-------------|
| **Template execution location** | Centralized (BuildRuntimeConfig) | Distributed (worker's DeriveDesiredState) | Workers know their template needs |
| **Parent-child variables** | N/A (no hierarchy) | Parent passes via Variables.User | Hierarchical composition |
| **Type conversion** | Template (string) → Runtime (typed) | UserSpec already typed | Simpler, no 2-phase |
| **StateMapping** | N/A | ❌ Not used (Variables instead) | Flexibility > declarative |

### Key FSMv2 Extensions

**1. Parent State in Variables:**
```go
// FSMv1: No parent-child hierarchy
// FSMv2: Parent passes state to child
Variables: VariableBundle{
    User: map[string]any{
        "parent_state":    "active",
        "parent_observed": observedState,  // Optional
    },
}
```

**2. Child Computes State from Parent:**
```go
// FSMv1: N/A
// FSMv2: Child reads parent_state and decides own state
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    state := computeFromParentState(scope["parent_state"])
    return DesiredState{State: state}
}
```

**3. Distributed Template Rendering:**
```go
// FSMv1: Centralized BuildRuntimeConfig()
config := BuildRuntimeConfig(spec, agentLoc, globalVars, ...)

// FSMv2: Each worker renders its own templates
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    config, _ := RenderTemplate(w.template, userSpec.Variables.Flatten())
    return DesiredState{Config: config}
}
```

---

## Decision Summary

### 1. Variable Structure → ADOPT Three-Tier (User/Global/Internal)

**Decision:** ✅ Use FSMv1's VariableBundle exactly

**Rationale:**
- Proven in production (FSMv1)
- Clear separation: User config, Global fleet, Internal runtime
- Flatten() pattern intuitive for users

**Child-specific variables:**
- Live in child's UserSpec.Variables.User
- Parent populates when creating ChildSpec
- Each child has own VariableBundle

**Scoping:**
- Per-worker (not shared between siblings)
- Hierarchical (parent → child flow only)

---

### 2. Templating Location → Workers' DeriveDesiredState

**Decision:** ✅ Each worker renders its own templates in DeriveDesiredState

**Rationale:**
- Worker knows what templates it needs
- No centralized service (defeats distributed FSM)
- Supervisor doesn't need to know worker's template schema
- Workers without templates skip rendering

**When executed:** Every tick in DeriveDesiredState

**Who executes:** The worker itself

**Performance:** Negligible (<100μs per worker), optimize later if needed

---

### 3. Type Safety → Strict UserSpec, Loose Variables

**Decision:** ✅ UserSpec uses Go struct fields, Variables uses map[string]any

**Rationale:**
- Best of both worlds: Type safety + template flexibility
- UserSpec typed: Compile-time checks, IDE autocomplete
- Variables loose: Template flexibility, dynamic values

**NO Template → Runtime conversion:**
- FSMv1 needs it (templates in config)
- FSMv2 doesn't (UserSpec already typed)
- Simpler: One-phase instead of two

---

### 4. Parent-Child Variable Flow → Parent Passes in ChildSpec.UserSpec

**Decision:** ✅ Parent computes ChildSpec with Variables containing parent_state

**Rationale:**
- Kubernetes pattern (parent passes config to child)
- Explicit variable passing (no magic)
- Child standalone (doesn't reach into parent)

**Parent controls:**
- What variables child sees
- Child's UserSpec (typed fields)

**Parent does NOT control:**
- Child's state (child computes from parent_state)

**Child computes:**
- Own state from parent_state variable
- Complex logic allowed (multi-condition, observed checks)

---

### 5. Observed State in Templates → NO

**Decision:** ❌ Templates cannot access observed state

**Rationale:**
- Causality: DeriveDesiredState called before CollectObservedState
- Desired state = "should be", Observed state = "is"
- Reactive logic belongs in Go code, not templates

**Parent CAN pass observed to child:**
- From PREVIOUS tick (no circular dependency)
- As variable in ChildSpec.UserSpec.Variables.User["parent_observed"]
- For child sanity checks

---

### 6. User Experience → Top-Level Variable Access

**Decision:** ✅ Flatten() puts User variables at top level

**Rationale:**
- User-friendly: `{{ .IP }}` not `{{ .user.IP }}`
- FSMv1 proven pattern
- Matches user intuition

**Template syntax:**
```yaml
# User variables (flattened):
{{ .IP }}
{{ .PORT }}
{{ .parent_state }}

# Global/Internal (nested):
{{ .global.api_endpoint }}
{{ .internal.id }}
```

---

### 7. Developer Experience → Explicit Variable Injection

**Decision:** ✅ Parent explicitly populates ChildSpec.UserSpec.Variables

**Rationale:**
- Clear: All child inputs visible in parent code
- No magic: Parent decides what child sees
- Testable: Can test parent variable construction

**How explicit:**
```go
// ✅ Explicit variable passing
ChildSpec{
    UserSpec: UserSpec{
        Variables: VariableBundle{
            User: map[string]any{
                "parent_state": parentState,
                "IP":           userSpec.IP,
            },
        },
    },
}
```

---

## Implementation Checklist

### Phase 1: Core Infrastructure

- [ ] Define VariableBundle struct (User/Global/Internal)
- [ ] Implement Flatten() method
- [ ] Implement RenderTemplate[T](template T, scope map[string]any) (T, error)
- [ ] Add strict mode: Option("missingkey=error")
- [ ] Add validation: No {{ markers in output
- [ ] Write tests for template rendering edge cases

### Phase 2: Location Computation

- [ ] Implement mergeLocations(parent, child map[string]string) map[string]string
- [ ] Implement computeLocationPath(location map[string]string) string
- [ ] Implement gap filling with "unknown"
- [ ] Write tests for location hierarchy edge cases

### Phase 3: Supervisor Integration

- [ ] Update Supervisor to pass Variables to child supervisors
- [ ] Remove StateMapping field and logic (if exists)
- [ ] Implement Global variable injection
- [ ] Implement Internal variable injection (id, timestamps)

### Phase 4: Worker Migration

- [ ] Update BridgeWorker to pass parent_state in Variables
- [ ] Update ConnectionWorker to compute state from parent_state
- [ ] Update BenthosWorker to render templates in DeriveDesiredState
- [ ] Update OPCUABrowserWorker for dynamic children
- [ ] Remove any SetDesiredState() calls from workers

### Phase 5: Documentation

- [ ] Document variable namespaces for users
- [ ] Document template syntax and available variables
- [ ] Document location hierarchy merging
- [ ] Document best practices for worker development
- [ ] Create migration guide from FSMv1 patterns

### Phase 6: Validation

- [ ] Add UserSpec serialization validation
- [ ] Add template syntax validation
- [ ] Add variable reference validation (catch undefined vars early)
- [ ] Add location hierarchy validation

---

## Open Questions

### 1. Should Global variables be injected by Supervisor or passed from parent?

**Option A: Supervisor injects** (Current FSMv1 pattern)
```go
func (s *Supervisor) Tick() {
    userSpec.Variables.Global = s.globalVars  // Supervisor adds
    desiredState := s.worker.DeriveDesiredState(userSpec)
}
```

**Option B: Parent passes through**
```go
func (w *ParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        Global: userSpec.Variables.Global,  // Parent passes
                    },
                },
            },
        },
    }
}
```

**Recommendation:** Supervisor injects (Option A)
- Global variables are fleet-wide (not parent-specific)
- Supervisor already has global state
- Simpler parent code (no pass-through needed)

---

### 2. Should Internal variables be worker-specific or supervisor-specific?

**Option A: Supervisor-specific** (id, timestamps)
```go
Variables.Internal = map[string]any{
    "id":         supervisorID,
    "created_at": supervisorCreatedAt,
}
```

**Option B: Worker-specific** (worker decides what's internal)
```go
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Worker adds its own internal vars
    userSpec.Variables.Internal["worker_type"] = "BenthosWorker"
    // ...
}
```

**Recommendation:** Supervisor-specific (Option A)
- Internal = runtime metadata (not user-facing)
- Supervisor owns lifecycle (id, timestamps)
- Workers shouldn't modify Internal (readonly)

---

### 3. Should parent_observed be serialized or ephemeral?

**Option A: Serialized** (in UserSpec.Variables.User)
```go
Variables.User = map[string]any{
    "parent_observed": observedState,  // Serialized to JSON
}
```

**Option B: Ephemeral** (separate field)
```go
type UserSpec struct {
    Variables      VariableBundle
    ParentObserved ObservedState  // Not serialized
}
```

**Recommendation:** Serialized (Option A)
- Simpler: One variable system
- Already in Variables, works with Flatten()
- Restoring from disk: Parent observed from previous run might be useful

---

### 4. Should template rendering be cached?

**Current proposal:** Render every tick (no caching)

**Alternative:** Cache with hash-based invalidation
```go
type Worker struct {
    lastVariablesHash string
    cachedConfig      Config
}

func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    hash := hashVariables(userSpec.Variables)
    if hash != w.lastVariablesHash {
        w.cachedConfig, _ = RenderTemplate(w.template, userSpec.Variables.Flatten())
        w.lastVariablesHash = hash
    }
    return DesiredState{Config: w.cachedConfig}
}
```

**Recommendation:** No caching initially
- Premature optimization (render ~100μs)
- Add caching later if profiling shows need
- Simpler code without cache invalidation

---

## Conclusion

FSMv2's templating and variables system builds on FSMv1's proven patterns while extending them for hierarchical composition.

**Core Design:**
- ✅ Three-tier variables (User/Global/Internal) with Flatten()
- ✅ Distributed template rendering (in worker's DeriveDesiredState)
- ✅ Strict UserSpec types + loose Variables maps
- ✅ Parent passes variables to child (explicit injection)
- ✅ Child computes state from parent_state (autonomy preserved)
- ❌ No observed state in templates (causality + separation of concerns)

**User Experience:**
- Natural YAML: Define variables, reference as `{{ .IP }}`
- Location auto-computed from hierarchy
- No surprises: What you define is what you reference

**Developer Experience:**
- Clear contracts: Typed UserSpec fields
- Flexible templates: Loose Variables maps
- Explicit variable flow: Parent decides what child sees
- Standalone workers: All inputs in UserSpec

**This design feels natural and unsurprising to both users and developers.**
