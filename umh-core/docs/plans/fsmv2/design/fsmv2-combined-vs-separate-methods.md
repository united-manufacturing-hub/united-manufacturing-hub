# FSMv2: Combined vs Separate Methods Analysis

**Date:** 2025-11-02
**Status:** Analysis
**Question:** "cant declarechildren and derivedesiredspec be done in the same call?"
**Related:**
- `fsmv2-userspec-based-child-updates.md` - UserSpec-based approach
- `fsmv2-developer-expectations-current-api.md` - Developer usability

## Question

Should `DeclareChildren()` and `DeriveDesiredState()` be combined into a single method?

**Observation:** Both methods:
- Take same parameter (`userSpec`)
- Return related data (parent state + child structure)
- Called when config changes

**Potential simplification?**

---

## Current Separate API

```go
type Worker interface {
    // Called every tick
    DeriveDesiredState(userSpec UserSpec) DesiredState

    // Called only when userSpec.Version changes
    DeclareChildren(userSpec UserSpec) *ChildDeclaration
}
```

### Worker Code

```go
type BridgeWorker struct{}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: "active", UserSpec: userSpec}
}

func (w *BridgeWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: supervisor.New(...)},
            {Name: "read_flow", Supervisor: supervisor.New(...)},
        },
    }
}
```

### Supervisor Code

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // Call DeriveDesiredState every tick
    desiredState := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desiredState)

    // Call DeclareChildren only when version changes
    if userSpec.Version != s.lastUserSpecVersion {
        children := s.worker.DeclareChildren(userSpec)
        s.reconcileChildren(children)
        s.lastUserSpecVersion = userSpec.Version
    }

    // Apply state mapping + tick children
    // ...
}
```

**Key Point:** Different calling frequencies!
- `DeriveDesiredState`: Every tick (1000s of times)
- `DeclareChildren`: Only on config change (~1 time per hour)

---

## Proposed Combined APIs

### Option A: Tuple Return (Always Both)

```go
type Worker interface {
    // Returns both state and children every call
    DeriveDesiredState(userSpec UserSpec) (DesiredState, *ChildDeclaration)
}
```

**Worker Code:**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) (DesiredState, *ChildDeclaration) {
    state := DesiredState{State: "active", UserSpec: userSpec}

    children := &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: supervisor.New(...)},
            {Name: "read_flow", Supervisor: supervisor.New(...)},
        },
    }

    return state, children
}
```

**Supervisor Code Option 1 (Call every tick, always reconcile):**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // Call combined method every tick
    desiredState, children := s.worker.DeriveDesiredState(userSpec)

    s.triangularStore.SetDesiredState(desiredState)

    // Reconcile children every tick (back to "every tick" pattern!)
    s.reconcileChildren(children)

    // ...
}
```

**Problem:** Reconciling children every tick defeats the UserSpec-based optimization!

**Supervisor Code Option 2 (Call every tick, conditionally reconcile):**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // Call combined method every tick
    desiredState, children := s.worker.DeriveDesiredState(userSpec)

    s.triangularStore.SetDesiredState(desiredState)

    // Only reconcile if version changed
    if userSpec.Version != s.lastUserSpecVersion {
        s.reconcileChildren(children)
        s.lastUserSpecVersion = userSpec.Version
    } else {
        // Children returned but ignored - waste!
    }

    // ...
}
```

**Problem:** Worker creates children every tick but supervisor ignores them - complete waste!

### Option B: Nullable Children

```go
type Worker interface {
    // Worker returns nil children when unchanged
    DeriveDesiredState(userSpec UserSpec) (DesiredState, *ChildDeclaration)
}
```

**Worker Code:**
```go
type BridgeWorker struct {
    lastUserSpecVersion int64
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) (DesiredState, *ChildDeclaration) {
    state := DesiredState{State: "active", UserSpec: userSpec}

    // Worker must track version changes!
    var children *ChildDeclaration
    if userSpec.Version != w.lastUserSpecVersion {
        children = &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: supervisor.New(...)},
                {Name: "read_flow", Supervisor: supervisor.New(...)},
            },
        }
        w.lastUserSpecVersion = userSpec.Version
    }

    return state, children  // children = nil when unchanged
}
```

**Supervisor Code:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    desiredState, children := s.worker.DeriveDesiredState(userSpec)

    s.triangularStore.SetDesiredState(desiredState)

    // Only reconcile if worker returned children
    if children != nil {
        s.reconcileChildren(children)
    }

    // ...
}
```

**Problem:** Worker must now track version changes - we moved complexity from supervisor to worker!

**Analysis:**
- ❌ Worker now has version tracking logic
- ❌ Version check happens in both worker AND supervisor (duplication)
- ❌ More complex than separate methods

### Option C: Keep Separate (Current)

Already shown above.

**Analysis:**
- ✅ Clear separation: state every tick, children on version change
- ✅ Supervisor controls calling frequency
- ✅ Worker has minimal logic

---

## Performance Analysis

### Assumptions
- 100 bridges
- 1 second tick interval
- 1 config change per hour per bridge

### Cost of DeclareChildren()

Creating supervisors is NOT free:
- Allocate supervisor struct
- Create worker instance
- Initialize TriangularStore
- Setup logging
- Potentially start goroutines

Estimated: **~100μs per supervisor** on modern CPU

For bridge with 3 children: **300μs per DeclareChildren() call**

### Separate Methods (Current Proposal)

**Per tick:**
- 100 DeriveDesiredState() calls (cheap - just return state)
- 0 DeclareChildren() calls (version unchanged)
- Cost: 100 * 10μs = 1ms

**Per hour:**
- 3600 ticks * 1ms = 3.6 seconds
- 100 config changes * 300μs = 30ms
- **Total: 3.63 seconds per hour**

### Combined Always-Return (Option A)

**Per tick:**
- 100 DeriveDesiredState() calls creating children
- 100 * 3 supervisors = 300 supervisor creations
- Cost: 300 * 100μs = 30ms

**Per hour:**
- 3600 ticks * 30ms = **108 seconds per hour**

**Performance Degradation: 30x worse** (108s vs 3.6s)

### Combined Nullable (Option B)

**Same as separate** - worker just does version check instead of supervisor

No performance difference, but more complex worker code.

---

## Code Complexity Comparison

### Lines of Code

**Separate Methods:**
```go
// Worker (8 lines total)
func (w *BridgeWorker) DeriveDesiredState(userSpec) DesiredState {
    return DesiredState{State: "active"}  // 1 line
}

func (w *BridgeWorker) DeclareChildren(userSpec) *ChildDeclaration {
    return &ChildDeclaration{...}  // 5 lines
}

// Supervisor (10 lines for version check + calling)
if userSpec.Version != s.lastUserSpecVersion {
    children := s.worker.DeclareChildren(userSpec)
    s.reconcileChildren(children)
    s.lastUserSpecVersion = userSpec.Version
}
```

**Combined Always-Return:**
```go
// Worker (7 lines total)
func (w *BridgeWorker) DeriveDesiredState(userSpec) (DesiredState, *ChildDeclaration) {
    state := DesiredState{State: "active"}
    children := &ChildDeclaration{...}  // 5 lines
    return state, children
}

// Supervisor (10 lines - still needs version check to avoid waste!)
if userSpec.Version != s.lastUserSpecVersion {
    desiredState, children := s.worker.DeriveDesiredState(userSpec)
    s.reconcileChildren(children)
    s.lastUserSpecVersion = userSpec.Version
} else {
    desiredState, _ := s.worker.DeriveDesiredState(userSpec)
}
```

**Wait - supervisor must call TWICE?** Once for children (on version change), once for state (every tick)?

**Alternative - call every tick, ignore children:**
```go
// Supervisor (8 lines)
desiredState, children := s.worker.DeriveDesiredState(userSpec)

if userSpec.Version != s.lastUserSpecVersion {
    s.reconcileChildren(children)
    s.lastUserSpecVersion = userSpec.Version
}
// children ignored most of the time - waste!
```

**Combined Nullable:**
```go
// Worker (12 lines total - MORE complex!)
func (w *BridgeWorker) DeriveDesiredState(userSpec) (DesiredState, *ChildDeclaration) {
    state := DesiredState{State: "active"}

    var children *ChildDeclaration
    if userSpec.Version != w.lastUserSpecVersion {  // Version tracking in worker!
        children = &ChildDeclaration{...}
        w.lastUserSpecVersion = userSpec.Version
    }

    return state, children
}

// Supervisor (5 lines - simpler)
desiredState, children := s.worker.DeriveDesiredState(userSpec)
if children != nil {
    s.reconcileChildren(children)
}
```

### Complexity Summary

| Approach | Worker LoC | Supervisor LoC | Version Tracking | Performance |
|----------|-----------|---------------|------------------|-------------|
| **Separate** | 8 | 10 | Supervisor | ✅ Optimal |
| **Combined Always** | 7 | 8-10 | Supervisor | ❌ 30x worse |
| **Combined Nullable** | 12 | 5 | Worker | ✅ Optimal |

**Analysis:**
- Combined Always: Simpler but terrible performance
- Combined Nullable: Moves version tracking to worker (more complex worker)
- Separate: Clear separation, supervisor controls frequency

---

## Conceptual Separation

### Different Purposes

**DeriveDesiredState:**
- **Purpose:** What state should parent be in?
- **Frequency:** Every tick (state can change anytime)
- **Dependencies:** UserSpec, current observed state
- **Output:** State name ("active", "idle", etc.)

**DeclareChildren:**
- **Purpose:** What children should exist and how are they configured?
- **Frequency:** Only when config changes
- **Dependencies:** UserSpec only
- **Output:** Child structure (supervisors, state mapping)

**These are fundamentally different concerns!**

### Analogy: React Component

```jsx
class MyComponent extends React.Component {
    // Like DeriveDesiredState - called every render
    render() {
        return <div>{this.renderContent()}</div>;
    }

    // Like DeclareChildren - only called when props change
    static getDerivedStateFromProps(props, state) {
        if (props.config !== state.lastConfig) {
            return {children: buildChildren(props.config)};
        }
        return null;
    }
}
```

React keeps these separate because they have different purposes and calling frequencies!

### Analogy: Kubernetes Controller

```go
type Controller struct{}

// Like DeriveDesiredState - determines current desired state
func (c *Controller) GetDesiredSpec() Spec {
    return c.currentDesiredSpec
}

// Like DeclareChildren - only when CRD changes
func (c *Controller) OnConfigChange(newCRD CustomResourceDefinition) {
    c.reconcileResources(newCRD)
}
```

Kubernetes also separates "what should state be" from "what resources should exist"!

---

## Edge Cases

### Edge Case 1: State Change Without Children Update

**Scenario:** Parent goes active → idle (no config change)

**Separate Methods:**
```go
// Tick N: Version 1, State "active"
desiredState := worker.DeriveDesiredState(userSpec)  // Returns "active"
// DeclareChildren NOT called (version unchanged)

// Tick N+1: Version 1, State "idle" (user clicked "stop")
desiredState := worker.DeriveDesiredState(userSpec)  // Returns "idle"
// DeclareChildren NOT called (version unchanged)
// StateMapping handles child state transitions
```

✅ **Works perfectly** - state change doesn't trigger unnecessary DeclareChildren

**Combined Always:**
```go
// Tick N+1
state, children := worker.DeriveDesiredState(userSpec)  // Returns "idle", creates supervisors
supervisor.reconcileChildren(children)  // Replaces all children unnecessarily!
```

❌ **Problem:** State change triggers child replacement even though config unchanged!

**Combined Nullable:**
```go
// Tick N+1
state, children := worker.DeriveDesiredState(userSpec)  // Returns "idle", nil
// children = nil, no reconciliation
```

✅ **Works** - but worker must track version

### Edge Case 2: Children Update Without State Change

**Scenario:** Add new server to OPC UA browser (config change but state stays "active")

**Separate Methods:**
```go
// Version changes 1 → 2, state stays "active"
desiredState := worker.DeriveDesiredState(userSpec)  // Returns "active" (unchanged)

if versionChanged() {
    children := worker.DeclareChildren(userSpec)  // Returns updated children
    supervisor.reconcileChildren(children)
}
```

✅ **Works perfectly** - config change handled independently of state

**Combined:**
```go
state, children := worker.DeriveDesiredState(userSpec)
// state = "active" (same as before)
// children = new structure

supervisor.reconcileChildren(children)
```

✅ **Also works** - no issue here

**Conclusion:** Separate methods handle state-only changes better (no unnecessary child reconciliation)

---

## Return Value Options (If Combined)

### Option 1: Tuple

```go
DeriveDesiredState(userSpec) (DesiredState, *ChildDeclaration)
```

**Pros:** Simple, standard Go pattern
**Cons:** No semantic meaning to nil children

### Option 2: Struct

```go
type WorkerUpdate struct {
    DesiredState     DesiredState
    ChildDeclaration *ChildDeclaration  // nil if unchanged
}

DeriveDesiredState(userSpec) WorkerUpdate
```

**Pros:** Self-documenting, can add fields later
**Cons:** More verbose

### Option 3: Interface

```go
type WorkerUpdate interface {
    GetDesiredState() DesiredState
    GetChildren() *ChildDeclaration  // nil if unchanged
}

DeriveDesiredState(userSpec) WorkerUpdate
```

**Pros:** Flexible, can have different implementations
**Cons:** Over-engineered for this use case

**If combined, Option 1 (tuple) is simplest**

---

## Recommendation: **KEEP SEPARATE**

### Primary Reason: Different Calling Frequencies

**DeriveDesiredState:**
- Called every tick (1000s of times)
- Cheap operation (just return state)
- May depend on current observed state, not just userSpec

**DeclareChildren:**
- Called only on config change (~1 time per hour)
- Expensive operation (creates supervisors)
- Depends only on userSpec

**Combining them forces one of two bad choices:**

1. **Call every tick** → 30x performance degradation
2. **Worker tracks version** → Complexity moved to worker, no API simplification

### Secondary Reasons

1. **Conceptual Separation**
   - State: "What should I be doing?"
   - Children: "What children should exist?"
   - These are different questions with different answers

2. **Clear Supervisor Control**
   - Supervisor decides when to check version
   - Supervisor decides when to reconcile children
   - Worker just provides data when asked

3. **Edge Case Handling**
   - State changes don't trigger child reconciliation
   - Clean separation prevents unnecessary work

4. **Aligns with Established Patterns**
   - React: render() vs getDerivedStateFromProps()
   - Kubernetes: getDesiredSpec() vs reconcileResources()
   - Both separate "current state" from "structural changes"

### Alternative Considered

If we really wanted to combine, **nullable return** (Option B) is the only viable approach:

```go
DeriveDesiredState(userSpec) (DesiredState, *ChildDeclaration)
// Worker returns nil children when version unchanged
```

But this requires worker to track version, adding complexity with no real benefit.

---

## Conclusion

### Answer to Question

**"cant declarechildren and derivedesiredspec be done in the same call?"**

**No** - they should remain separate.

**Why:**
- **Different calling frequencies** (every tick vs on config change)
- **Performance impact** (30x worse if called together every tick)
- **Conceptual separation** (state vs structure are different concerns)
- **Supervisor control** (supervisor optimizes when to call each)

### API Stays As

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState
    DeclareChildren(userSpec UserSpec) *ChildDeclaration
}
```

### Key Insight

**Just because two methods take the same parameter doesn't mean they should be combined.**

The parameter is just input - what matters is:
- **Purpose** (what question are we answering?)
- **Frequency** (how often is the answer needed?)
- **Cost** (how expensive is computing the answer?)

DeriveDesiredState and DeclareChildren differ on all three axes, so they should remain separate.

---

## Future Optimization (If Needed)

If calling frequency becomes an issue, supervisor could batch:

```go
type WorkerUpdateBatch struct {
    DesiredState     *DesiredState      // nil if not needed this tick
    ChildDeclaration *ChildDeclaration  // nil if version unchanged
}

func (s *Supervisor) Tick() error {
    batch := WorkerUpdateBatch{}

    // Always need state
    state := s.worker.DeriveDesiredState(userSpec)
    batch.DesiredState = &state

    // Only need children on version change
    if s.versionChanged() {
        children := s.worker.DeclareChildren(userSpec)
        batch.ChildDeclaration = &children
    }

    s.applyBatch(batch)
}
```

But current separate API is clearer and doesn't need this optimization yet.
