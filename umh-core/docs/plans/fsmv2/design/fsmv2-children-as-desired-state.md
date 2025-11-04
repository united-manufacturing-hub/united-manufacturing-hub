# FSMv2: Should Children Be Part of DesiredState?

**Date:** 2025-11-02
**Status:** Analysis
**Question:** "but shouldnt child declaration be a subpart of desired state?"
**Related:**
- `fsmv2-config-changes-and-dynamic-children.md` - Every-tick reconciliation analysis
- `fsmv2-combined-vs-separate-methods.md` - Separate vs combined methods
- `fsmv2-userspec-based-child-updates.md` - UserSpec-based approach

## The Question

**Current Proposal:**
```go
type DesiredState struct {
    State    string
    UserSpec UserSpec
}

type Worker interface {
    DeriveDesiredState(userSpec) DesiredState
    DeclareChildren(userSpec) *ChildDeclaration  // Separate method
}
```

**User's Suggested Architecture:**
```go
type DesiredState struct {
    State    string
    UserSpec UserSpec
    Children *ChildDeclaration  // PART OF desired state!
}

type Worker interface {
    DeriveDesiredState(userSpec) DesiredState  // Returns everything
}
```

**Core Question:** Is "what children should exist" conceptually part of "desired state"?

---

## 1. Philosophical Analysis

### What IS "Desired State"?

From TriangularStore pattern:
- **Identity:** Who am I? (immutable characteristics)
- **Desired:** What should I be doing? (intent, goals, configuration)
- **Observed:** What am I actually doing? (reality, measurements)

**Question:** Does "what children should exist" belong in Desired?

### Comparison to Established Systems

#### Kubernetes

```yaml
# Deployment's desired state INCLUDES children
apiVersion: apps/v1
kind: Deployment
spec:
  replicas: 3  # ← Desired child count is PART OF desired state
  template:
    spec:
      containers:
        - name: app
          image: myapp:v1
```

**Observation:** Kubernetes includes child structure in desired state.
- `replicas: 3` = "I want 3 pod children"
- Pod template = "Each child should look like this"

#### Docker Compose

```yaml
# Service's desired state INCLUDES dependencies
services:
  web:
    image: nginx
    depends_on:  # ← Child/dependency structure
      - db
      - redis
  db:
    image: postgres
```

**Observation:** Docker Compose includes service relationships in configuration.

#### Terraform

```hcl
# Resource's desired state INCLUDES child resources
resource "aws_instance" "parent" {
  ami           = "ami-123"
  instance_type = "t2.micro"

  # Child resources declared inline
  ebs_block_device {
    device_name = "/dev/sda1"
    volume_size = 20
  }
}
```

**Observation:** Terraform allows child resources within parent desired state.

#### Erlang/OTP Supervision Trees

```erlang
init(_Args) ->
    {ok, {{one_for_one, 5, 10},
          [
           {child1, {module1, start_link, []}, permanent, 5000, worker, [module1]},
           {child2, {module2, start_link, []}, permanent, 5000, worker, [module2]}
          ]
         }}.
```

**Observation:** Supervisor's initialization returns child specifications as part of desired configuration.

### Pattern Recognition

**ALL established systems include child structure as part of desired state/configuration:**
- Kubernetes: `replicas` field
- Docker Compose: `depends_on` field
- Terraform: Nested resources
- Erlang: Child specs in init

**Philosophical Conclusion:** "What children should exist" IS desired state.

---

## 2. Calling Frequency with Caching

### Current Concern

From `fsmv2-userspec-based-child-updates.md`:
- `DeriveDesiredState` called **every tick**
- `DeclareChildren` called **only when version changes**

**Question:** If children are part of DesiredState, does that break performance optimization?

### Option A: Worker Caches Children

```go
type BridgeWorker struct {
    cachedChildren *ChildDeclaration
    lastVersion    int64
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Only recreate children if version changed
    if userSpec.Version != w.lastVersion {
        w.cachedChildren = &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: supervisor.New(...)},
                {Name: "read_flow", Supervisor: supervisor.New(...)},
            },
        }
        w.lastVersion = userSpec.Version
    }
    
    return DesiredState{
        State:    "active",
        Children: w.cachedChildren,  // Return cached pointer
    }
}
```

**Supervisor Code:**
```go
func (s *Supervisor) Tick() error {
    userSpec := s.triangularStore.GetUserSpec()
    
    // Call DeriveDesiredState every tick
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // Supervisor caches and compares
    if desiredState.Children != s.cachedChildren {
        s.reconcileChildren(desiredState.Children)
        s.cachedChildren = desiredState.Children
    }
    
    s.triangularStore.SetDesiredState(desiredState)
}
```

**Analysis:**
- ✅ Worker tracks version (same as nullable approach analyzed earlier)
- ✅ No performance difference from separate methods
- ✅ Just different structure
- ❌ Worker must cache children and track version

### Option B: Supervisor Caches and Diffs

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Always create children fresh
    return DesiredState{
        State: "active",
        Children: &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: supervisor.New(...)},
                {Name: "read_flow", Supervisor: supervisor.New(...)},
            },
        },
    }
}

func (s *Supervisor) Tick() error {
    desired := s.worker.DeriveDesiredState(userSpec)
    
    // Supervisor diffs (pointer comparison)
    if desired.Children != s.lastChildren {
        s.reconcileChildren(desired.Children)
    }
}
```

**Analysis:**
- ❌ Back to "every tick reconciliation" pattern
- ❌ Worker must recreate supervisors when config changes
- ❌ 30x performance degradation (from `fsmv2-config-changes-and-dynamic-children.md`)

### Comparison to Original "Every Tick" Approach

**Original proposal** (from `fsmv2-config-changes-and-dynamic-children.md`):
```go
DeclareChildren() *ChildDeclaration  // Called every tick
```

**User's new idea:**
```go
type DesiredState struct {
    Children *ChildDeclaration
}
DeriveDesiredState(userSpec) DesiredState  // Called every tick, includes children
```

**These are IDENTICAL in behavior:**
- Both call method every tick
- Both return children every tick
- Both rely on supervisor diffing
- Only difference: Children embedded in DesiredState vs separate

**Conclusion:** This is a STRUCTURAL change, not a performance change.

### Caching Location Analysis

**Question:** Should worker or supervisor handle caching?

**Option A: Worker caches children**
- Pro: Worker controls when children are recreated
- Pro: Supervisor just uses what worker provides
- Con: Worker must track version changes
- Con: Duplicates version tracking between worker and supervisor

**Option B: Supervisor caches and version-checks**
- Pro: Supervisor already tracks version
- Pro: Worker code simpler
- Con: Supervisor must call method conditionally
- Con: Breaks "always call DeriveDesiredState every tick" pattern

**From `fsmv2-userspec-based-child-updates.md` analysis:**
- Separate methods: Worker simple (20 lines), Supervisor complex (70 lines)
- Combined with worker caching: Worker complex (30 lines), Supervisor simple (50 lines)

**Verdict:** Option A (worker caches) moves complexity to worker. Option B (supervisor version-checks) keeps current pattern from UserSpec-based approach.

---

## 3. TriangularStore Integration

### Current TriangularStore

```go
type TriangularStore struct {
    Identity     Identity
    DesiredState DesiredState
    ObservedState ObservedState
}
```

### If DesiredState Includes Children

```go
type DesiredState struct {
    State    string
    Children *ChildDeclaration
}
```

**Question:** Should child structure be persisted in DesiredState?

**Option 1: Store everything**
```go
// Persist children in DesiredState
s.triangularStore.SetDesiredState(DesiredState{
    State: "active",
    Children: &ChildDeclaration{...},  // Includes supervisor pointers!
})
```

**Problem:** Child supervisors aren't serializable!
- Supervisor contains goroutines, channels, mutexes
- Can't persist to disk or send over network
- Would lose information on restart

**Option 2: Store child metadata separately**
```go
type DesiredState struct {
    State         string
    ChildMetadata []ChildMetadata  // Serializable
}

type ChildMetadata struct {
    Name         string
    ConfigHash   string
    StateMapping map[string]string
}

// At runtime, supervisor has actual supervisors
type Supervisor struct {
    childSupervisors map[string]*Supervisor  // Not persisted
}
```

**Analysis:**
- ✅ Can persist metadata
- ✅ Can restore from disk
- ❌ Duplicates information (metadata vs actual supervisors)
- ❌ Adds complexity

**Option 3: Don't persist children at all**
```go
type DesiredState struct {
    State    string
    // Children NOT stored in TriangularStore
}

// Supervisor keeps children separately
type Supervisor struct {
    triangularStore TriangularStore
    children        map[string]*childInstance  // Not in store
}
```

**Analysis:**
- ✅ Simple - no duplication
- ✅ Matches current implementation
- ❌ Children not part of persisted desired state
- ❌ Philosophically inconsistent (children ARE desired state but not stored?)

### Conclusion: TriangularStore Conflict

**The TriangularStore is designed for SERIALIZABLE state.**

If children are part of desired state conceptually, but supervisors aren't serializable:
- Either we accept that TriangularStore doesn't contain full desired state
- Or we create metadata/proxy representation

**This suggests children might be better kept SEPARATE from DesiredState:**
- DesiredState = serializable parent state
- Children = runtime-only structure managed by supervisor

---

## 4. Code Examples

### Bridge Worker (Option A: Worker Caches)

```go
type BridgeWorker struct {
    // Worker caches children
    cachedChildren *ChildDeclaration
    lastVersion    int64
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Check if config changed
    if userSpec.Version != w.lastVersion {
        w.cachedChildren = &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: supervisor.New(...)},
                {Name: "read_flow", Supervisor: supervisor.New(...)},
            },
        }
        w.lastVersion = userSpec.Version
    }
    
    return DesiredState{
        State:    "active",
        Children: w.cachedChildren,  // Return cached
    }
}
```

**Lines of code:** 15 lines (vs 20 in separate methods)

### Bridge Worker (Option B: Supervisor Version-Checks)

**This is IDENTICAL to current UserSpec-based approach!**

```go
type BridgeWorker struct {
    // No caching in worker
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        Children: &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: supervisor.New(...)},
                {Name: "read_flow", Supervisor: supervisor.New(...)},
            },
        },
    }
}
```

**Supervisor:**
```go
func (s *Supervisor) Tick() error {
    userSpec := s.triangularStore.GetUserSpec()
    
    // Only call when version changes
    if userSpec.Version != s.lastVersion {
        desired := s.worker.DeriveDesiredState(userSpec)
        s.reconcileChildren(desired.Children)
        s.lastVersion = userSpec.Version
    }
    
    // Call for state every tick
    desired := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desired)
}
```

**Wait - supervisor calls DeriveDesiredState TWICE?**
- Once for children (when version changes)
- Once for state (every tick)

**This is clearly wrong!**

### OPC UA Browser (Dynamic N Children)

```go
func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    children := []ChildSpec{}
    
    for _, serverAddr := range userSpec.Servers {
        children = append(children, ChildSpec{
            Name: serverAddr,
            Supervisor: supervisor.New(
                newServerBrowserWorker(serverAddr),
                logger,
            ),
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    }
    
    return DesiredState{
        State:    "active",
        Children: &ChildDeclaration{Children: children},
    }
}
```

**Same code as separate methods** - just returns everything together.

---

## 5. Performance Analysis

### If Worker Creates Children Every Tick

**Assumptions:**
- 100 bridges × 3 children = 300 supervisors created per tick
- 300 × 100μs = 30ms per tick

**Cost per hour:**
- 3600 ticks × 30ms = 108 seconds per hour

**From `fsmv2-config-changes-and-dynamic-children.md`:**
- Separate methods: 3.6 seconds per hour
- **Combined always-create: 30x worse**

### If Worker Caches

**Worker tracks version** (same as nullable approach):
- No performance difference from separate methods
- Just different structure

**Cost per hour:**
- Same as UserSpec-based: 136ms per hour

### If Supervisor Diffs Pointers

**Back to "every tick reconciliation" pattern:**
- Supervisor compares Children pointer
- Only reconciles if pointer changed
- Worker must recreate supervisors when config changes

**This is the ORIGINAL "every tick" approach!**

**Conclusion:** Performance depends on CACHING STRATEGY, not whether children are part of DesiredState.

---

## 6. Comparison to Original "Every Tick" Approach

### Original Proposal

From `fsmv2-config-changes-and-dynamic-children.md`:
```go
DeclareChildren() *ChildDeclaration  // Called every tick
```

Supervisor calls every tick, diffs, reconciles.

### User's New Idea

```go
type DesiredState struct {
    Children *ChildDeclaration
}
DeriveDesiredState(userSpec) DesiredState  // Called every tick, includes children
```

**These are FUNCTIONALLY IDENTICAL:**
- Both call method every tick
- Both return children every tick
- Both rely on supervisor diffing
- Both have same performance characteristics

**Only difference:** Structural - children embedded in DesiredState vs separate.

**Question:** Is this just a naming/packaging change, or fundamentally different?

**Answer:** It's a STRUCTURAL change only. Behavior is identical.

---

## 7. Conceptual Clarity

### Separate Methods View

**DesiredState = "What state should parent be in?"**
- State name ("active", "idle", "error")
- Parent configuration
- Parent-specific intent

**DeclareChildren = "What children should exist?"**
- Child structure
- Child configuration
- Hierarchical composition

**Two different concerns** with different lifecycles.

### Unified View

**DesiredState = "Complete desired configuration"**
- Includes parent state AND child structure
- Single source of truth
- Matches Kubernetes/Docker Compose pattern

**Single concern:** "What should the complete system look like?"

### Which is Clearer?

**Arguments for Separate:**
- Clear separation of concerns
- Parent state can change without affecting children
- StateMapping handles child state propagation
- Matches current TriangularStore design (serializable parent state)

**Arguments for Unified:**
- Matches industry patterns (Kubernetes, Docker Compose, Terraform)
- Philosophically consistent (children ARE desired state)
- Single method to call
- More intuitive mental model

---

## 8. State Transitions and Children

**Question:** Can parent state transition change children without config change?

**Example:** Parent idle → active

### Separate Methods

```go
DeriveDesiredState(userSpec) -> "active"
DeclareChildren() -> not called (version unchanged)
StateMapping handles child state changes
```

**Flow:**
1. Parent state changes: idle → active
2. DeclareChildren NOT called (version unchanged)
3. Supervisor looks up StateMapping
4. Supervisor sets children desired state based on new parent state
5. Children transition (running → stopped or vice versa)

**Child state changes handled DECLARATIVELY via StateMapping.**

### Unified Approach

```go
DeriveDesiredState(userSpec) -> DesiredState{State: "active", Children: ...}
```

**Option 1: Worker caches, returns same Children pointer**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec) DesiredState {
    // Children cached, same pointer returned
    return DesiredState{
        State: "active",
        Children: w.cachedChildren,  // Same pointer as before
    }
}
```

Supervisor compares pointers, sees no change, doesn't reconcile.
StateMapping still needed for child state changes.

**Option 2: Children include desired child states**
```go
type DesiredState struct {
    State        string
    ChildStates  map[string]string  // child name -> desired state
}
```

**NO!** This duplicates StateMapping logic.

**Conclusion:** Unified approach STILL needs StateMapping. Including children in DesiredState doesn't eliminate state propagation mechanism.

---

## 9. Recommendation

### Core Trade-off

**Philosophically:** Children ARE desired state (matches Kubernetes, Docker Compose, Terraform)

**Practically:** Children aren't serializable (TriangularStore conflict)

**Performance:** Identical for both approaches if caching strategy is same

**Complexity:** Similar for both (worker caches vs supervisor version-checks)

### Recommendation: **KEEP SEPARATE**

**Rationale:**

1. **TriangularStore Compatibility**
   - DesiredState should be serializable
   - Children contain supervisors (goroutines, channels, mutexes)
   - Keeping children separate avoids creating non-serializable DesiredState

2. **Clear Separation of Concerns**
   - DesiredState = "What state should parent be in?" (simple, serializable)
   - DeclareChildren = "What children should exist?" (complex, runtime-only)
   - StateMapping = "How does parent state affect child state?" (declarative)

3. **No Performance Benefit**
   - Both approaches require caching
   - Both need version tracking
   - Unified doesn't simplify anything

4. **Avoids Double-Calling**
   - Separate methods: Call DeriveDesiredState every tick, DeclareChildren on version change
   - Unified: Either call twice (once for state, once for children) or recreate children every tick

5. **Matches Current UserSpec-Based Design**
   - Already analyzed and recommended in `fsmv2-userspec-based-child-updates.md`
   - Worker code simpler (20 lines vs 30)
   - No "forgot to recreate supervisor" bugs

### Alternative: Metadata Approach

If we REALLY want children in DesiredState philosophically:

```go
type DesiredState struct {
    State         string
    ChildMetadata []ChildMetadata  // Serializable description
}

type ChildMetadata struct {
    Name         string
    TemplateRef  string
    ConfigHash   string
    StateMapping map[string]string
}

// Supervisor maintains runtime supervisors separately
type Supervisor struct {
    triangularStore  TriangularStore  // Has ChildMetadata
    childSupervisors map[string]*Supervisor  // Runtime instances
}
```

**This is MORE complex** and doesn't solve the double-calling problem.

---

## 10. Final Comparison

### Current Proposal (Separate Methods)

```go
type Worker interface {
    DeriveDesiredState(userSpec) DesiredState
    DeclareChildren(userSpec) *ChildDeclaration
}

type DesiredState struct {
    State    string
    UserSpec UserSpec
}
```

**Supervisor:**
```go
func (s *Supervisor) Tick() error {
    userSpec := s.triangularStore.GetUserSpec()
    
    // Detect config changes via version
    if userSpec.Version != s.lastVersion {
        children := s.worker.DeclareChildren(userSpec)
        s.reconcileChildren(children)
        s.lastVersion = userSpec.Version
    }
    
    // Derive desired state every tick
    desiredState := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desiredState)
    
    // Apply state mapping to children
    // ...
}
```

**Characteristics:**
- ✅ Clear separation: state vs children
- ✅ Worker code simple (20 lines)
- ✅ No double-calling
- ✅ DesiredState serializable
- ✅ Matches UserSpec-based recommendation

### Proposed Alternative (Unified)

```go
type Worker interface {
    DeriveDesiredState(userSpec) DesiredState
}

type DesiredState struct {
    State    string
    UserSpec UserSpec
    Children *ChildDeclaration
}
```

**Supervisor Option 1 (Worker caches):**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec) DesiredState {
    if userSpec.Version != w.lastVersion {
        w.cachedChildren = createChildren(userSpec)
        w.lastVersion = userSpec.Version
    }
    
    return DesiredState{
        State: "active",
        Children: w.cachedChildren,
    }
}

func (s *Supervisor) Tick() error {
    desired := s.worker.DeriveDesiredState(userSpec)
    
    if desired.Children != s.lastChildren {
        s.reconcileChildren(desired.Children)
    }
    
    s.triangularStore.SetDesiredState(desired)
}
```

**Characteristics:**
- ❌ Worker must cache and track version (30 lines vs 20)
- ❌ DesiredState not serializable (contains supervisors)
- ❌ More complex worker code
- ✅ Single method call
- ✅ Philosophically consistent (children are desired state)

**Supervisor Option 2 (Call conditionally):**
```go
func (s *Supervisor) Tick() error {
    userSpec := s.triangularStore.GetUserSpec()
    
    // Call for children when version changes
    if userSpec.Version != s.lastVersion {
        desired := s.worker.DeriveDesiredState(userSpec)
        s.reconcileChildren(desired.Children)
        s.lastVersion = userSpec.Version
    }
    
    // Call for state every tick
    desired := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desired)
}
```

**Characteristics:**
- ❌ Calls DeriveDesiredState TWICE (wasteful)
- ❌ Worker recreates children every call (performance hit if not cached)
- ✅ Worker code simple (15 lines)
- ❌ Confusing pattern (why call twice?)

---

## Conclusion

### Answer to Question

**"but shouldnt child declaration be a subpart of desired state?"**

**Philosophically: YES** - Matches Kubernetes, Docker Compose, Terraform patterns.

**Practically: NO** - Creates TriangularStore serialization conflict and doesn't simplify implementation.

### Recommendation: KEEP SEPARATE

**Why:**
1. DesiredState should be serializable (TriangularStore design)
2. Children contain non-serializable supervisors
3. No performance or complexity benefit
4. Avoids double-calling pattern
5. Clear separation of concerns

**Keep:**
```go
type Worker interface {
    DeriveDesiredState(userSpec) DesiredState
    DeclareChildren(userSpec) *ChildDeclaration
}
```

### Key Insight

**The real issue isn't whether children are "desired state" philosophically.**

**The real issue is:**
- Should DesiredState be serializable? (TriangularStore says yes)
- Should children be recreated every tick? (Performance says no)
- Should worker or supervisor track version? (UserSpec-based analysis says supervisor)

**Separate methods solve all three problems elegantly.**

### If User Still Wants Unified

**Acceptable compromise:**

```go
type DesiredState struct {
    State         string
    UserSpec      UserSpec
    ChildMetadata []ChildMetadata  // Serializable metadata ONLY
}

// Supervisor maintains runtime supervisors separately
// Reconciles when metadata changes
```

But this is **MORE complex** than separate methods.
