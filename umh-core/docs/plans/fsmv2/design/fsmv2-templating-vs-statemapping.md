# FSMv2: Templating vs StateMapping for Child State Control

**Date:** 2025-11-02
**Status:** Analysis
**Related:**
- `fsmv2-child-specs-in-desired-state.md` - Child specifications in DesiredState architecture
- `fsmv1-templating-and-variables.md` - FSMv1's templating and variable system
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Master plan

## The Question

**User's Proposal:**
> "also do we really need the state mapping? maybe that is a helpful simplification, but maybe we can also solve it through templating? so by specifying the parents state as a go template? in fsm_v1 we had templating anyway, so derive desired state also needs the userspec, as well as a variables object. in the variables object, we could put in the desired state and maybe also the observed state of its own object or when declaring children only of the parent. then we can have variables for each supervisor, e.g., IP, and then specify it like {{ .IP }}"

**Current StateMapping Approach (from child-specs doc):**

```go
type ChildSpec struct {
    Name         string
    WorkerType   string
    UserSpec     UserSpec
    StateMapping map[string]string  // {"active": "up", "idle": "down"}
}
```

**Proposed Templating Approach:**

```go
type ChildSpec struct {
    Name       string
    WorkerType string
    UserSpec   UserSpec  // Contains Variables with parent state
}

// Variables object includes parent state
UserSpec{
    Variables: VariableBundle{
        User: map[string]any{
            "parent_state": "active",  // From parent
            "IP": "192.168.1.100",
        },
    },
}

// Child computes state in DeriveDesiredState
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    state := "down"
    if scope["parent_state"] == "active" || scope["parent_state"] == "idle" {
        state = "up"
    }

    return DesiredState{State: state}
}
```

---

## Comparison

### Option 1: Current StateMapping

**Structure:**

```go
type ChildSpec struct {
    StateMapping map[string]string  // Parent state → child state
}

// Parent declares mapping
ChildSpec{
    Name: "connection",
    StateMapping: map[string]string{
        "active": "up",
        "idle":   "down",
    },
}

// Supervisor applies mapping
for name, child := range s.children {
    if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
        child.supervisor.SetDesiredState(childDesiredState)
    }
}
```

**Advantages:**
- ✅ Simple declarative mapping
- ✅ Parent controls child state directly
- ✅ No child code needed (pure config)
- ✅ Easy to validate (finite state mapping)
- ✅ Clear in YAML/JSON serialization

**Disadvantages:**
- ❌ Limited to simple 1:1 state mapping
- ❌ Can't express complex logic (e.g., "if parent=active AND connection=ok")
- ❌ Parent must know child's state space
- ❌ Tight coupling (parent knows child states)
- ❌ Extra field in ChildSpec

**Code Example:**

```go
// Parent worker
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec:   UserSpec{IPAddress: userSpec.IPAddress},
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                    "error":  "down",
                },
            },
        },
    }
}

// Child worker - NO state logic needed
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // State already set by supervisor via SetDesiredState
    // Just return config
    return DesiredState{
        State: w.currentState,  // Set by supervisor
    }
}
```

---

### Option 2: Variables with Parent State (Go Logic)

**Structure:**

```go
type ChildSpec struct {
    UserSpec UserSpec  // Contains parent state in Variables
}

// Parent passes state via Variables
ChildSpec{
    Name: "connection",
    UserSpec: UserSpec{
        Variables: VariableBundle{
            User: map[string]any{
                "parent_state":    "active",
                "parent_observed": observedState,  // Optional
                "IP":              "192.168.1.100",
            },
        },
    },
}

// Child computes state from parent state
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    state := "down"
    if scope["parent_state"] == "active" || scope["parent_state"] == "idle" {
        state = "up"
    }

    return DesiredState{State: state}
}
```

**Advantages:**
- ✅ Flexible - child can implement complex logic
- ✅ Decoupled - parent doesn't know child states
- ✅ Reuses existing Variables pattern from FSMv1
- ✅ No StateMapping field needed
- ✅ Child owns its state transitions
- ✅ Can access parent observed state for sanity checks

**Disadvantages:**
- ❌ Requires child code (not pure config)
- ❌ State logic spread across child workers
- ❌ Harder to validate (arbitrary Go code)
- ❌ Less declarative than StateMapping

**Code Example:**

```go
// Parent worker
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Compute desired and observed states
    desiredState := "active"
    observedState, _ := w.getObservedState()

    return DesiredState{
        State: desiredState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":    desiredState,    // Pass state
                            "parent_observed": observedState,   // Pass observed
                            "IP":              userSpec.IPAddress,
                            "PORT":            userSpec.Port,
                        },
                    },
                },
            },
        },
    }
}

// Child worker - computes state from parent
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    // Access parent state
    parentState := scope["parent_state"].(string)

    // Complex logic possible
    state := "down"
    if parentState == "active" || parentState == "idle" {
        state = "up"
    }

    // Could also access parent observed state for sanity checks
    if parentObserved, ok := scope["parent_observed"].(string); ok {
        if parentObserved == "error" {
            state = "down"  // Override based on parent observed
        }
    }

    return DesiredState{State: state}
}
```

---

### Option 3: Template Strings in ChildSpec

**Structure:**

```go
type ChildSpec struct {
    DesiredStateTemplate string  // Go template executed by supervisor
}

// Parent declares template
ChildSpec{
    Name: "connection",
    DesiredStateTemplate: `{{ if eq .parent_state "active" }}up{{ else }}down{{ end }}`,
}

// Supervisor renders template
scope := map[string]any{
    "parent_state": desiredState.State,
}
childState, _ := renderTemplate(child.DesiredStateTemplate, scope)
child.supervisor.SetDesiredState(childState)
```

**Advantages:**
- ✅ Declarative template in config
- ✅ More flexible than simple mapping
- ✅ Reuses RenderTemplate infrastructure
- ✅ Serializable (string template)

**Disadvantages:**
- ❌ Template syntax in ChildSpec (ugly JSON/YAML)
- ❌ Supervisor must execute templates (extra complexity)
- ❌ Limited logic (only what text/template supports)
- ❌ String-based state (error-prone)
- ❌ Supervisor needs parent state before ticking child

**Code Example:**

```go
// Parent worker
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                DesiredStateTemplate: `{{ if or (eq .parent_state "active") (eq .parent_state "idle") }}up{{ else }}down{{ end }}`,
            },
        },
    }
}

// Supervisor ticks child
func (s *Supervisor) tickChild(child *childInstance, parentState string) {
    scope := map[string]any{"parent_state": parentState}
    childState, _ := RenderTemplate(child.desiredStateTemplate, scope)
    child.supervisor.SetDesiredState(childState)
    child.supervisor.Tick(ctx)
}
```

---

### Option 4: Direct State in UserSpec

**Structure:**

```go
type ChildSpec struct {
    UserSpec UserSpec  // Contains direct state field
}

// Parent computes child state directly
ChildSpec{
    Name: "connection",
    UserSpec: UserSpec{
        DesiredState: "up",  // Parent computed
    },
}

// Child just uses provided state
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: userSpec.DesiredState}
}
```

**Advantages:**
- ✅ Simplest approach
- ✅ Parent fully controls child state
- ✅ No mapping or templating needed

**Disadvantages:**
- ❌ Parent must compute child state (couples parent to child state space)
- ❌ Child loses autonomy (can't react to own observations)
- ❌ Breaks Worker contract (DeriveDesiredState becomes no-op)

---

## Detailed Comparison Table

| Aspect | StateMapping | Variables (Go Logic) | Template String | Direct State |
|--------|--------------|---------------------|-----------------|--------------|
| **Declarative** | ✅ Map config | ⚠️ Code required | ✅ Template config | ✅ Direct value |
| **Flexible** | ❌ 1:1 only | ✅ Arbitrary logic | ⚠️ Template limits | ❌ Parent decides |
| **Serializable** | ✅ Map serializes | ✅ Variables serialize | ✅ String template | ✅ String value |
| **Parent Coupling** | ⚠️ Knows child states | ✅ Doesn't need to know | ⚠️ Knows child states | ❌ Tight coupling |
| **Child Autonomy** | ⚠️ Parent controls | ✅ Child decides | ❌ Parent controls | ❌ Parent controls |
| **Complexity** | Low | Medium | Medium | Very Low |
| **Validation** | ✅ Easy (finite map) | ❌ Hard (arbitrary code) | ⚠️ Template syntax | ✅ Simple |
| **FSMv1 Alignment** | ❌ New concept | ✅ Same pattern | ✅ Uses RenderTemplate | ❌ New concept |
| **Supervisor Complexity** | Low (map lookup) | None (child handles) | High (template execution) | None |
| **Child Code Required** | No | Yes | No | Minimal |
| **Complex Logic** | ❌ Not supported | ✅ Full Go code | ⚠️ Limited | ❌ Not supported |
| **Observability** | ⚠️ Implicit | ✅ Explicit in child code | ⚠️ Template hard to debug | ❌ Opaque |

---

## Use Case Analysis

### Use Case 1: Simple On/Off Control

**Requirement:** Bridge "active" → Connection "up", Bridge "idle/error" → Connection "down"

**StateMapping:**
```go
StateMapping: map[string]string{
    "active": "up",
    "idle":   "down",
    "error":  "down",
}
```
**Winner:** StateMapping (simplest, most declarative)

**Variables:**
```go
// Child code
state := "down"
if scope["parent_state"] == "active" {
    state = "up"
}
```
**Verdict:** Overkill for simple case

---

### Use Case 2: Conditional Start Based on Parent Observed State

**Requirement:** Start child only if parent is active AND parent observed state shows healthy

**StateMapping:**
```go
// Can't express this - no access to observed state
StateMapping: map[string]string{"active": "up"}  // Insufficient
```
**Verdict:** Not supported

**Variables:**
```go
// Child code
state := "down"
parentState := scope["parent_state"].(string)
parentObserved := scope["parent_observed"].(map[string]any)

if parentState == "active" && parentObserved["healthy"] == true {
    state = "up"
}
```
**Winner:** Variables (only option that supports this)

---

### Use Case 3: Multi-Parent Coordination

**Requirement:** Benthos starts only if BOTH Redpanda "active" AND Connection "up"

**StateMapping:**
```go
// Can't express multi-parent dependencies
```
**Verdict:** Not supported

**Variables:**
```go
// Child code
redpandaState := scope["redpanda_state"].(string)
connectionState := scope["connection_state"].(string)

state := "down"
if redpandaState == "active" && connectionState == "up" {
    state = "running"
}
```
**Winner:** Variables (supports multiple parent states)

**Note:** Would require supervisor to pass multiple parent states:

```go
UserSpec{
    Variables: VariableBundle{
        User: map[string]any{
            "redpanda_state":   "active",
            "connection_state": "up",
        },
    },
}
```

---

## Performance Comparison

### StateMapping

**Per-tick cost:**
```go
// O(1) map lookup
childState := child.stateMapping[parentState]
child.supervisor.SetDesiredState(childState)
```
**Cost:** ~10 nanoseconds

### Variables (Go Logic)

**Per-tick cost:**
```go
// Child DeriveDesiredState executes
desiredState := child.worker.DeriveDesiredState(childUserSpec)
```
**Cost:** ~100 nanoseconds (depends on child logic complexity)

**Comparison:** Variables 10x slower, but both negligible (<1μs)

---

## Recommendation

### **ADOPT: Variables with Parent State (Option 2)**

**Rationale:**

1. **Flexibility Matters**
   - Real-world use cases need complex logic (observed state checks, multi-parent coordination)
   - StateMapping too limited (1:1 mapping only)
   - Variables supports arbitrary Go logic

2. **Decoupling is Critical**
   - Parent shouldn't need to know child's state space
   - Child owns its state transitions
   - Matches FSM philosophy (each worker autonomous)

3. **FSMv1 Pattern Reuse**
   - Variables already proven pattern
   - Flatten() already exists
   - No new concepts to learn

4. **Child Autonomy Preserved**
   - Child can react to its own observations
   - Child can implement complex startup conditions
   - Parent just provides context (parent state/observed)

5. **No StateMapping Field Needed**
   - Simpler ChildSpec structure
   - One less concept in the API

6. **Performance Acceptable**
   - 10x slower than StateMapping, but both <1μs
   - Negligible in practice

**Trade-offs Accepted:**

1. **Child Code Required**
   - Not pure config (need Go code in child DeriveDesiredState)
   - Acceptable: Workers already implement DeriveDesiredState anyway

2. **Harder to Validate**
   - Can't statically validate arbitrary Go code
   - Mitigated: Unit tests for child workers

3. **Less Declarative**
   - Logic in code, not config
   - Acceptable: Flexibility more valuable than declarative config for complex cases

---

## Implementation

### ChildSpec Structure

```go
type ChildSpec struct {
    Name       string
    WorkerType string
    UserSpec   UserSpec  // Contains Variables with parent state
}

type UserSpec struct {
    Variables VariableBundle  // Includes parent_state, parent_observed
    // ... other fields
}
```

**No StateMapping field!**

### Parent Passes State in Variables

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    desiredState := "active"
    observedState, _ := w.getObservedState()

    return DesiredState{
        State: desiredState,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Parent context
                            "parent_state":    desiredState,
                            "parent_observed": observedState,

                            // Connection config
                            "IP":   userSpec.IPAddress,
                            "PORT": userSpec.Port,
                        },
                    },
                },
            },
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Parent context
                            "parent_state":    desiredState,

                            // Benthos config
                            "config": generateBenthosConfig(userSpec),
                        },
                    },
                },
            },
        },
    }
}
```

### Child Computes State from Parent

```go
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    // Access parent state
    parentState, ok := scope["parent_state"].(string)
    if !ok {
        parentState = "unknown"
    }

    // Compute child state based on parent
    state := "down"
    if parentState == "active" || parentState == "idle" {
        state = "up"
    }

    // Could also check parent observed for sanity
    if parentObserved, ok := scope["parent_observed"].(map[string]any); ok {
        if healthy, ok := parentObserved["healthy"].(bool); ok && !healthy {
            state = "down"  // Parent unhealthy, stay down
        }
    }

    return DesiredState{State: state}
}
```

### Supervisor Tick Flow

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // Derive parent's desired state
    desiredState := s.worker.DeriveDesiredState(userSpec)

    // Persist parent's desired state
    s.triangularStore.SetDesiredState(desiredState)

    // Reconcile children from specs
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // Tick each child (child derives its own state from Variables)
    for _, child := range s.children {
        child.supervisor.Tick(ctx)  // Child uses userSpec.Variables.parent_state
    }

    return nil
}
```

**Key difference from StateMapping:**
- Supervisor does NOT set child state via SetDesiredState()
- Child derives its own state in DeriveDesiredState() using parent state from Variables

---

## Migration Path

### Phase 1: Add Variables Support

1. Extend UserSpec with Variables field
2. Implement VariableBundle.Flatten()
3. Update Supervisor to pass parent state in Variables

### Phase 2: Update Child Workers

1. Update ConnectionWorker to use parent_state from Variables
2. Update BenthosWorker similarly
3. Remove SetDesiredState() calls from Supervisor

### Phase 3: Remove StateMapping

1. Remove StateMapping field from ChildSpec
2. Remove StateMapping application logic from Supervisor
3. Update tests

---

## Edge Cases and Considerations

### What if Parent State Changes Rapidly?

**Scenario:** Parent state oscillates: active → idle → active → idle

**Variables Approach:**
- Child DeriveDesiredState called each tick
- Child recomputes state based on current parent state
- State changes propagated immediately

**StateMapping Approach:**
- Same behavior (map lookup each tick)

**Verdict:** No difference in behavior

### What if Child Needs Multi-Parent Coordination?

**Scenario:** Benthos needs BOTH Redpanda "active" AND Connection "up"

**Variables Approach:**
```go
// Grandparent (Bridge) passes both states
UserSpec{
    Variables: VariableBundle{
        User: map[string]any{
            "redpanda_state":   "active",
            "connection_state": "up",
        },
    },
}

// Child checks both
if scope["redpanda_state"] == "active" && scope["connection_state"] == "up" {
    state = "running"
}
```

**StateMapping Approach:**
- Not supported (only maps single parent state)

**Verdict:** Variables wins

### What if Parent Needs to Know Child State?

**Scenario:** Parent wants to go "error" if child is "failed"

**Variables Approach:**
- Parent can check child observed state via supervisor.GetChildState()
- Same as current design

**StateMapping Approach:**
- Same

**Verdict:** No difference

---

## Conclusion

**Eliminate StateMapping, use Variables with parent state.**

**Benefits:**
- ✅ More flexible (supports complex logic)
- ✅ Decoupled (parent doesn't know child states)
- ✅ Reuses FSMv1 pattern (Variables, Flatten)
- ✅ Simpler ChildSpec (no StateMapping field)
- ✅ Child autonomy preserved

**Costs:**
- ❌ Child code required (not pure config)
- ❌ Less declarative (logic in code)
- ❌ Harder to validate (arbitrary Go)

**Trade-off is worth it:** Flexibility and decoupling outweigh declarative simplicity for real-world use cases.

**Next Steps:**
1. Update ChildSpec to remove StateMapping
2. Implement VariableBundle.Flatten() (reuse from FSMv1)
3. Update parent workers to pass parent_state in Variables
4. Update child workers to compute state from parent_state
5. Remove SetDesiredState() calls from Supervisor tick
