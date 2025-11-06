# FSMv2: Developer Expectations for Current API

**Date:** 2025-11-02
**Status:** Analysis
**Related:**
- `fsmv2-phase0-worker-child-api.md` - Phase 0 API proposal
- `fsmv2-config-changes-and-dynamic-children.md` - Every-tick reconciliation analysis

## Purpose

Analyze the current FSMv2 Worker API from a developer usability perspective:
- What do developers expect when they see the interface?
- Where will they get confused?
- What mistakes will they make?
- How does it compare to familiar patterns?

## Current API

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState
    DeclareChildren() *ChildDeclaration  // Called every tick
}

type ChildDeclaration struct {
    Children []ChildSpec
}

type ChildSpec struct {
    Name         string
    Supervisor   *Supervisor
    StateMapping map[string]string  // Parent state → child desired state
}
```

**Implementation Pattern:**
- `DeclareChildren()` called every tick by supervisor
- Supervisor diffs old vs new children via pointer comparison
- Config changes: Worker recreates supervisors in `DeriveDesiredState()`
- StateMapping provides declarative state transitions

---

## First Impressions

### What Developers See

```go
type Worker interface {
    DeclareChildren() *ChildDeclaration
}
```

### What Developers Assume

**Assumption 1: Called Once at Initialization**
```go
// Developer thinks: "This looks like setup"
func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // "I declare my children here, probably called once"
    return &ChildDeclaration{...}
}
```

**Reality:** Called every tick (100s or 1000s of times)

**Why confusing:**
- Method name "Declare" sounds like one-time declaration
- No parameters suggest it's stateless/constant
- Similar to React's `getDerivedStateFromProps()` which is called on every render, BUT React explicitly documents this

### What Developers Assume

**Assumption 2: Supervisor Instances are Immutable**
```go
// Developer thinks: "I create supervisors in constructor"
func NewBridgeWorker() *BridgeWorker {
    return &BridgeWorker{
        connectionSupervisor: supervisor.New(...),
        readFlowSupervisor: supervisor.New(...),
    }
}

func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // "Just return my fixed supervisors"
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor},
        },
    }
}
```

**Problem:** Config changes never propagate to children!

**Reality:** Must recreate supervisors when config changes

**Why confusing:**
- Not obvious that pointer comparison is how supervisor detects changes
- No documentation that supervisors should be recreated
- Looks like "declare once, reference forever" pattern

---

## Cognitive Load Analysis

### Concepts Developer Must Understand

1. **Declarative APIs** (Familiar)
   - Developer declares desired state
   - Supervisor executes changes
   - ✅ Low barrier - matches React, Kubernetes

2. **Reconciliation Loop** (Somewhat familiar)
   - Function called repeatedly
   - System diffs and applies changes
   - ⚠️ Medium barrier - React developers know this, backend developers might not

3. **Pointer Comparison for Change Detection** (Unfamiliar)
   - Supervisor detects changes by comparing `*Supervisor` pointers
   - Same pointer = no change
   - Different pointer = child changed
   - ❌ High barrier - subtle, undocumented, easy to miss

4. **Supervisor Lifecycle Management** (Unfamiliar)
   - Developer must know WHEN to create supervisors
   - Must track config changes themselves
   - Must recreate supervisors on config change
   - ❌ High barrier - requires understanding of entire flow

5. **StateMapping vs Supervisor Recreation** (Unfamiliar)
   - StateMapping for state transitions (active → idle)
   - Supervisor recreation for config changes (IP address)
   - ❌ High barrier - two different mechanisms for similar-looking changes

### Total Cognitive Load: **HIGH**

Developer must understand 5 concepts, 3 of which are unfamiliar and undocumented.

---

## Common Mistakes

### Mistake 1: Static Supervisors (Critical Bug)

**Beginner Implementation:**
```go
type BridgeWorker struct {
    config *BridgeConfig

    // Created once in constructor
    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor   *supervisor.Supervisor
}

func NewBridgeWorker(config *BridgeConfig) *BridgeWorker {
    return &BridgeWorker{
        config: config,
        connectionSupervisor: supervisor.New(
            newConnectionWorker(config.IPAddress),
            logger,
        ),
        readFlowSupervisor: supervisor.New(
            newReadFlowWorker(config.IPAddress),
            logger,
        ),
    }
}

func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // Just return existing supervisors
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor},
            {Name: "read_flow", Supervisor: w.readFlowSupervisor},
        },
    }
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Update config, but DON'T recreate supervisors
    w.config = userSpec
    return DesiredState{State: "active", UserSpec: userSpec}
}
```

**Bug:** Config changes (IP address 10.0.0.1 → 10.0.0.2) never propagate to children!

**Why it happens:** Developer doesn't know they must recreate supervisors

**Symptom:** Children keep using old config forever

### Mistake 2: Recreating Every Tick (Performance Bug)

**Incorrect Implementation:**
```go
func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // "Make sure children have latest config by recreating every tick"
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                // BUG: Creating new supervisor every tick!
                Supervisor: supervisor.New(
                    newConnectionWorker(w.config.IPAddress),
                    w.logger,
                ),
            },
        },
    }
}
```

**Bug:** Creates new supervisors every tick, even when config unchanged

**Why it happens:** Developer doesn't trust the "pointer comparison" mechanism

**Symptom:** High CPU, children constantly restarting

### Mistake 3: Unclear Supervisor Lifecycle

**Developer Question:**
> "Should I create supervisors eagerly (constructor) or lazily (DeclareChildren)?"

**Current Correct Answer (from analysis doc):**
- Create in constructor for static supervisors
- Recreate in `DeriveDesiredState()` when config changes
- Lazy create in `DeclareChildren()` for dynamic children (OPC UA browser)

**Problem:** Three different patterns for supervisor creation - which one when?

---

## Correct Implementation

### Bridge with Config Changes

```go
type BridgeWorker struct {
    config *BridgeConfig

    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor   *supervisor.Supervisor
}

// Called when config changes
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    oldIPAddress := w.config.IPAddress
    w.config = userSpec

    // Config changed - recreate child supervisors
    if oldIPAddress != userSpec.IPAddress {
        w.connectionSupervisor = supervisor.New(
            newConnectionWorker(userSpec.IPAddress),
            w.logger,
        )
        w.readFlowSupervisor = supervisor.New(
            newReadFlowWorker(userSpec.IPAddress),
            w.logger,
        )
    }

    return DesiredState{State: "active", UserSpec: userSpec}
}

// Called every tick - returns current supervisors
func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name:       "connection",
                Supervisor: w.connectionSupervisor,
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            {
                Name:       "read_flow",
                Supervisor: w.readFlowSupervisor,
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}
```

**Key Points:**
- Supervisors stored as fields
- Recreated in `DeriveDesiredState()` when config changes
- `DeclareChildren()` just returns current supervisors
- Pointer change triggers supervisor's diffing logic

### OPC UA Browser (Dynamic N Children)

```go
type OPCUABrowserWorker struct {
    config *OPCUABrowserConfig

    // Dynamic map of supervisors
    childSupervisors map[string]*supervisor.Supervisor
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    w.config = userSpec
    return DesiredState{State: "active", UserSpec: userSpec}
}

// Called every tick - creates supervisors lazily
func (w *OPCUABrowserWorker) DeclareChildren() *ChildDeclaration {
    children := []ChildSpec{}

    for _, serverAddr := range w.config.Servers {
        // Lazy create supervisor for new server
        if _, exists := w.childSupervisors[serverAddr]; !exists {
            w.childSupervisors[serverAddr] = supervisor.New(
                newServerBrowserWorker(serverAddr),
                w.logger,
            )
        }

        children = append(children, ChildSpec{
            Name:       serverAddr,
            Supervisor: w.childSupervisors[serverAddr],
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    }

    return &ChildDeclaration{Children: children}
}
```

**Key Points:**
- Supervisors created lazily in `DeclareChildren()`
- Config changes handled automatically (new servers → new supervisors)
- Servers removed from config → not in children array → auto-removed

**Developer Confusion:** Why lazy here but eager in Bridge example?

**Answer:**
- Bridge: Fixed N children, config changes need detection → eager + recreate
- OPC UA: Dynamic N children, config iteration → lazy create

**Problem:** Two different patterns, not obvious which to use when

---

## Comparison to Familiar Patterns

### React Component Pattern

**React:**
```jsx
class MyComponent extends React.Component {
    render() {
        return (
            <div>
                <Child1 prop={this.props.data} />
                <Child2 prop={this.props.data} />
            </div>
        );
    }
}
```

**Key Points:**
- `render()` called on every state/props change
- Developer returns declarative structure
- React diffs virtual DOM, updates only changed elements
- **Documented:** React explicitly documents render is called frequently

**FSMv2 Parallel:**
```go
func (w *Worker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "child1", Supervisor: w.child1Supervisor},
            {Name: "child2", Supervisor: w.child2Supervisor},
        },
    }
}
```

**Differences:**
- ✅ Both use declarative returns
- ✅ Both rely on framework diffing
- ❌ React creates new elements every render (cheap)
- ❌ FSMv2 reuses supervisor pointers (developer must manage)
- ❌ React documents "render called frequently"
- ❌ FSMv2 doesn't document "DeclareChildren called every tick"

### Kubernetes Controller Pattern

**Kubernetes Reconcile:**
```go
func (r *MyReconciler) Reconcile(ctx context.Context, req Request) (Result, error) {
    // Get desired state
    var obj MyObject
    r.Get(ctx, req.NamespacedName, &obj)

    // Reconcile to desired state
    // ...

    return Result{}, nil
}
```

**Key Points:**
- `Reconcile()` called on events OR every 10 seconds
- Controller compares desired vs observed state
- Controller applies changes
- **Documented:** K8s explicitly documents reconciliation timing

**FSMv2 Parallel:**
```go
func (w *Worker) DeclareChildren() *ChildDeclaration {
    // Return desired children
    return &ChildDeclaration{...}
}
```

**Differences:**
- ✅ Both use reconciliation loop pattern
- ✅ Both declare desired state
- ❌ K8s reconcile includes both "what" and "how"
- ❌ FSMv2 separates "what" (DeclareChildren) from "how" (supervisor)
- ❌ K8s documents timing (event-driven + periodic)
- ❌ FSMv2 doesn't document "every tick"

---

## Developer Questions (FAQ)

### Q1: Why is DeclareChildren() called every tick if children rarely change?

**Answer:** Reconciliation loop pattern - supervisor diffs to detect changes

**Problem:** Not obvious from API, requires documentation

**Suggestion:** Add comment or rename method:
```go
// Called every tick by supervisor to reconcile children
DeclareChildren() *ChildDeclaration

// Or rename to make it obvious:
ReconcileChildren() *ChildDeclaration
GetDesiredChildren() *ChildDeclaration
```

### Q2: Do I need to cache supervisors or create new ones?

**Answer:** Cache supervisors as fields, recreate only when config changes

**Problem:** Not obvious - pointer comparison is undocumented

**Suggestion:** Document in interface or provide examples

### Q3: How does supervisor know config changed?

**Answer:** Supervisor compares supervisor pointers - if different, config changed

**Problem:** Indirect, relies on developer recreating supervisors

**Suggestion:** Make change detection explicit:
```go
// Option 1: Version field
type ChildSpec struct {
    ConfigVersion int64  // Increment on config change
}

// Option 2: Explicit change signal
supervisor.UpdateChildConfig("connection", newConfig)
```

### Q4: What if I forget to recreate supervisors?

**Answer:** Config changes silently ignored - children keep old config

**Problem:** Silent failure, no error message

**Suggestion:**
- Add validation/warnings in supervisor
- Or make config part of ChildSpec:
```go
type ChildSpec struct {
    Name       string
    Supervisor *Supervisor
    Config     interface{}  // Supervisor diffs this instead of pointer
}
```

### Q5: When should I create supervisors? Constructor, DeriveDesiredState, or DeclareChildren?

**Answer:** Depends on use case:
- Static children + config changes: Constructor + recreate in DeriveDesiredState
- Dynamic N children: Lazy create in DeclareChildren

**Problem:** Multiple patterns, not obvious which to use

**Suggestion:** Provide clear decision tree in documentation

---

## Issues Summary

### High Severity

1. **Silent Config Change Failures**
   - Developer forgets to recreate supervisors
   - Config changes ignored
   - No error, no warning
   - **Impact:** Critical production bugs

2. **Undocumented "Every Tick" Calling**
   - Interface doesn't indicate frequency
   - Developer assumptions wrong
   - **Impact:** Performance bugs (recreating every tick) or correctness bugs (static supervisors)

3. **Pointer Comparison is Subtle**
   - Not obvious that pointer change = config change
   - Requires developer to manage supervisor lifecycle
   - **Impact:** Misunderstandings, incorrect implementations

### Medium Severity

4. **Multiple Supervisor Creation Patterns**
   - Bridge: eager + recreate
   - OPC UA: lazy create
   - Not obvious which to use when
   - **Impact:** Developer confusion

5. **No Validation**
   - Supervisor can't detect if developer forgot to recreate
   - No runtime warnings
   - **Impact:** Hard to debug issues

---

## Recommendations

### 1. Document Calling Frequency (Immediate)

```go
type Worker interface {
    // DeclareChildren is called every tick by the supervisor to reconcile
    // the desired child state. Return the current set of children that should
    // exist. The supervisor will automatically add/remove/update children as needed.
    //
    // IMPORTANT: To update child configuration, recreate the supervisor instance
    // in DeriveDesiredState() when config changes. The supervisor detects changes
    // by comparing supervisor pointers.
    DeclareChildren() *ChildDeclaration
}
```

### 2. Add Examples (Immediate)

Provide canonical examples in documentation:
- Bridge with config changes (eager supervisors)
- OPC UA browser with dynamic children (lazy supervisors)
- Common mistakes to avoid

### 3. Consider Config Versioning (Medium Term)

Make config changes explicit instead of pointer comparison:

```go
type ChildSpec struct {
    Name         string
    Supervisor   *Supervisor
    ConfigHash   string  // Supervisor compares this
    StateMapping map[string]string
}

func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name:       "connection",
                Supervisor: w.connectionSupervisor,  // Reused
                ConfigHash: hashConfig(w.config),     // Changes when config changes
            },
        },
    }
}
```

**Benefits:**
- Don't need to recreate supervisors on config change
- Supervisor can log config changes
- More explicit change detection

**Trade-offs:**
- Need config hashing mechanism
- Need UpdateConfig() method on supervisor

### 4. Add Validation (Medium Term)

Supervisor could detect mistakes:

```go
func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    for name, spec := range desired.Children {
        if current, exists := s.children[name]; exists {
            if current.supervisor == spec.Supervisor {
                // Same pointer - check if config actually changed
                if s.detectConfigMismatch(current, spec) {
                    s.logger.Warn(
                        "Child config may have changed but supervisor not recreated",
                        "child", name,
                        "hint", "Recreate supervisor in DeriveDesiredState() when config changes",
                    )
                }
            }
        }
    }
}
```

---

## Conclusion

### Usability Assessment: **MEDIUM**

**Strengths:**
- ✅ Declarative API matches familiar patterns (React, K8s)
- ✅ StateMapping is intuitive
- ✅ Auto-removal works well

**Weaknesses:**
- ❌ Pointer comparison for change detection is subtle
- ❌ "Every tick" calling not obvious from interface
- ❌ Multiple supervisor creation patterns confusing
- ❌ Silent failures when config changes missed
- ❌ High cognitive load (5 concepts, 3 unfamiliar)

### Primary Concern

**Developers will forget to recreate supervisors on config change**, leading to silent failures in production.

This is a critical usability issue that needs mitigation through:
1. Clear documentation
2. Canonical examples
3. Runtime validation/warnings
4. OR alternative API that makes config changes explicit

### Comparison to User's Simplified Idea

The "only call DeclareChildren when UserSpec changes" approach might address some of these issues by:
- Making config change timing explicit
- Removing need for pointer comparison
- Clarifying when supervisors should be created

See `fsmv2-userspec-based-child-updates.md` for detailed analysis of that approach.
