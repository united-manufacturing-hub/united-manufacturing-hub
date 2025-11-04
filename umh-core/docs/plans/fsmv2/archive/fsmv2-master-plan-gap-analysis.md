# FSMv2 Master Plan Gap Analysis

**Date:** 2025-11-02
**Status:** Analysis Complete
**Related:**
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Master implementation plan
- `fsmv2-derive-desired-state-complete-definition.md` - Complete API specification
- `fsmv2-child-specs-in-desired-state.md` - Hierarchical composition architecture
- `fsmv2-idiomatic-templating-and-variables.md` - Templating and variables system

---

## Executive Summary

The master plan predates critical hierarchical composition design decisions made through subsequent analysis documents. This gap analysis identifies:

**CRITICAL GAPS:**
1. **Hierarchical Composition** - Master plan mentions "child declarations" but doesn't define API, data structures, or implementation
2. **Templating & Variables System** - Completely missing from master plan despite being core to FSMv2
3. **DeriveDesiredState API Changes** - Now returns ChildrenSpecs, not just parent state
4. **State Control Mechanism** - StateMapping + Variables pattern not in master plan
5. **WorkerFactory Pattern** - Required for child creation, not mentioned in master plan

**SCOPE IMPACT:**
- Original estimate: 6 weeks (Infrastructure Supervision + Async Actions)
- New estimate: **10-12 weeks** (+66% increase)
- New components add ~40% more code
- Additional complexity in supervisor implementation

**INTEGRATION POINTS:**
- Infrastructure Supervision still valid (Circuit Breaker pattern)
- Async Action Executor still valid (Global worker pool)
- New hierarchical composition integrates cleanly with both

**RECOMMENDATION:**
Update master plan with 4-phase approach:
1. Phase 0: Hierarchical Composition Foundation (NEW - 2 weeks)
2. Phase 1: Infrastructure Supervision (Existing - 2 weeks)
3. Phase 2: Async Action Executor (Existing - 2 weeks)
4. Phase 3: Templating & Variables System (NEW - 2 weeks)
5. Phase 4: Integration & Edge Cases (Updated - 2 weeks)
6. Phase 5: Monitoring & Observability (Existing - 2 weeks)

---

## 1. Missing Components Analysis

### 1.1 Hierarchical Composition (CRITICAL GAP)

**What's Missing:**

Master plan mentions:
> "Infrastructure checks run FIRST (affect all workers), action checks run SECOND (affect individual workers)"

But doesn't define:
- ❌ How children are declared (API signature)
- ❌ ChildSpec structure (Name, WorkerType, UserSpec, StateMapping)
- ❌ How parent creates child specs
- ❌ How supervisor reconciles children
- ❌ WorkerFactory pattern for creating workers from string types
- ❌ Child lifecycle management (add/remove/update)
- ❌ How UserSpec flows from parent to child

**New Design Decision (from `fsmv2-child-specs-in-desired-state.md`):**

```go
type DesiredState struct {
    State         string
    ChildrenSpecs []ChildSpec  // ← NEW: Children in DesiredState
}

type ChildSpec struct {
    Name         string
    WorkerType   string
    UserSpec     UserSpec
    StateMapping map[string]string
}

type Worker interface {
    DeriveDesiredState(userSpec UserSpec) DesiredState  // Returns children!
}
```

**Why It Matters:**

- **Core Architecture:** Hierarchical composition is HOW parents control children
- **Kubernetes Pattern:** Matches industry-proven pattern exactly
- **Serialization:** Full TriangularStore persistence now possible
- **Dynamic Children:** OPC UA browser with N server children requires this

**Where It Fits:**

- Must implement BEFORE Infrastructure Supervision (children must exist to supervise)
- Integrates with Circuit Breaker (circuit affects all children)
- Integrates with Async Actions (actions queue per child)

**Implementation Size:**

- Supervisor.reconcileChildren(): ~80 lines
- WorkerFactory: ~30 lines
- ChildSpec structures: ~50 lines
- Tests: ~200 lines
- **Total: ~360 lines** (not in original estimate)

---

### 1.2 Templating & Variables System (CRITICAL GAP)

**What's Missing:**

Master plan has ZERO mention of:
- ❌ VariableBundle (User/Global/Internal namespaces)
- ❌ Flatten() method (User variables → top-level)
- ❌ RenderTemplate() with strict mode
- ❌ Template execution location (distributed at worker level)
- ❌ Location computation and merging
- ❌ Parent-child variable flow

**New Design Decision (from `fsmv2-idiomatic-templating-and-variables.md`):**

```go
type VariableBundle struct {
    User     map[string]any  // User config + parent state
    Global   map[string]any  // Fleet-wide settings
    Internal map[string]any  // Runtime metadata
}

func (vb VariableBundle) Flatten() map[string]any {
    out := map[string]any{}
    for k, v := range vb.User {
        out[k] = v  // Top-level access
    }
    out["global"] = vb.Global
    out["internal"] = vb.Internal
    return out
}

func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
    // Strict mode: missingkey=error
    // Validates no {{ markers remain
}
```

**Why It Matters:**

- **Benthos Config Generation:** Can't create protocol converters without templates
- **Parent Context Passing:** How parent_state flows to children
- **Location Hierarchy:** Auto-computed paths (ACME.Factory.Line-A)
- **FSMv1 Compatibility:** Proven pattern from production workloads

**Where It Fits:**

- Workers render templates in DeriveDesiredState (every tick)
- Parent passes variables to child via UserSpec
- Location merging happens in parent's DeriveDesiredState

**Implementation Size:**

- VariableBundle structures: ~30 lines
- Flatten() method: ~15 lines
- RenderTemplate(): ~40 lines
- Location merging: ~60 lines
- Location path computation: ~30 lines
- Tests: ~150 lines
- **Total: ~325 lines** (not in original estimate)

---

### 1.3 DeriveDesiredState API Changes (CRITICAL GAP)

**Master Plan Assumption:**

Implied that DeriveDesiredState returns simple state:

```go
// Master plan doesn't define this API at all
func (w *Worker) DeriveDesiredState() State {
    return "active"
}
```

**New Design (from `fsmv2-derive-desired-state-complete-definition.md`):**

```go
func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State:         "active",
        ChildrenSpecs: []ChildSpec{...},  // NEW
    }
}
```

**Changes:**

1. **Return type:** State → DesiredState (struct with children)
2. **Parameter:** No params → userSpec UserSpec (typed config)
3. **Responsibilities:** State only → State + children + template rendering

**Why It Matters:**

- All workers must implement new signature
- Supervisor must handle DesiredState.ChildrenSpecs
- TriangularStore must persist DesiredState (not just state string)

**Where It Fits:**

- Called every tick by supervisor
- Integrates with both supervision patterns (infrastructure + actions)
- Templates render inside this method

**Migration Impact:**

- Every existing worker needs update
- Supervisor.Tick() logic changes
- TriangularStore schema changes

---

### 1.4 State Control Mechanism (MAJOR GAP)

**Master Plan Assumption:**

No mention of how parent controls child state. Implied direct control:

```go
// Not defined in master plan
supervisor.SetChildState("connection", "up")
```

**New Design (from `fsmv2-derive-desired-state-complete-definition.md`):**

```go
// Parent declares mapping
StateMapping: map[string]string{
    "active": "up",      // When parent is "active", child should be "up"
    "idle":   "down",
}

// Supervisor applies mapping
func (s *Supervisor) applyStateMapping(parentState string) {
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[parentState]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }
}

// Child can also compute state from parent_state variable
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    parentState := scope["parent_state"].(string)
    
    state := "down"
    if parentState == "active" {
        state = "up"
    }
    
    return DesiredState{State: state}
}
```

**Hybrid Approach:**

1. Parent declares StateMapping (declarative)
2. Supervisor applies mapping (infrastructure enforces)
3. Child can also compute from parent_state variable (autonomy)
4. Child can override for safety checks

**Why It Matters:**

- Clear parent-child relationship in code
- Child autonomy preserved (safety checks)
- Decoupled (child doesn't know parent's state space)
- Matches Kubernetes patterns

**Where It Fits:**

- StateMapping in ChildSpec
- Applied by supervisor before ticking child
- Works with Variables system (parent_state variable)

**Implementation Size:**

- StateMapping field: ~5 lines
- applyStateMapping(): ~15 lines
- Tests: ~50 lines
- **Total: ~70 lines** (not in original estimate)

---

### 1.5 WorkerFactory Pattern (MAJOR GAP)

**Why Needed:**

ChildSpec contains `WorkerType string` (not Worker instance):

```go
type ChildSpec struct {
    Name       string
    WorkerType string  // ← "ConnectionWorker", "BenthosWorker", etc.
    UserSpec   UserSpec
}
```

Supervisor must create Worker from string:

```go
worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
```

**Master Plan Mentions:** NOTHING about worker creation

**New Design:**

```go
type WorkerFactory interface {
    CreateWorker(workerType string, userSpec UserSpec) (Worker, error)
}

type DefaultWorkerFactory struct {
    logger *zap.Logger
}

func (f *DefaultWorkerFactory) CreateWorker(workerType string, userSpec UserSpec) (Worker, error) {
    switch workerType {
    case "ConnectionWorker":
        return newConnectionWorker(userSpec, f.logger), nil
    case "BenthosWorker":
        return newBenthosWorker(userSpec, f.logger), nil
    // ... 10+ worker types
    default:
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
}
```

**Why It Matters:**

- **String-based types:** Serializable ChildSpecs
- **Dynamic creation:** Supervisor creates children at runtime
- **Extensibility:** Register new worker types easily

**Where It Fits:**

- Injected into supervisor constructor
- Called in reconcileChildren() when creating new children
- Required for child specs to be serializable

**Implementation Size:**

- Interface: ~5 lines
- Implementation: ~40 lines (switch statement for 10 workers)
- Tests: ~30 lines
- **Total: ~75 lines** (not in original estimate)

---

### 1.6 Location Computation and Merging (MAJOR GAP)

**Master Plan Mentions:** NOTHING about location

**New Design (from FSMv1 analysis + templating doc):**

```go
// Merge parent + child location
func mergeLocations(parent, child map[string]string) map[string]string {
    merged := make(map[string]string)
    for k, v := range parent {
        merged[k] = v
    }
    for k, v := range child {
        merged[k] = v  // Child extends parent
    }
    return merged
}

// Compute path: ACME.Factory.Line-A.Cell-5
func computeLocationPath(location map[string]string) string {
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

**Usage in Parent Worker:**

```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    childLocation := map[string]string{"bridge": userSpec.Name}
    
    location := mergeLocations(parentLocation, childLocation)
    locationPath := computeLocationPath(location)
    
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
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

**Why It Matters:**

- **ISA-95 Hierarchy:** Enterprise.Site.Area.Line.Cell
- **UNS Topic Generation:** umh.v1.{location_path}.{name}
- **Parent-Child Extension:** Child adds levels to parent location
- **Gap Filling:** Graceful degradation with "unknown"

**Where It Fits:**

- Called in parent's DeriveDesiredState
- Result passed to child via Variables
- Used in template rendering ({{ .location_path }})

**Implementation Size:**

- mergeLocations(): ~10 lines
- computeLocationPath(): ~20 lines
- Gap filling logic: ~30 lines
- Tests: ~80 lines
- **Total: ~140 lines** (not in original estimate)

---

## 2. Conflicts & Resolutions

### 2.1 "Child Declarations" Terminology

**Master Plan Says:**

> "Infrastructure checks run first (affect all workers), action checks run second (affect individual workers)"

Uses "children" and "workers" interchangeably, but never defines:
- What is a "child declaration"?
- How do you declare children?
- What data structure represents a child?

**New Design Resolution:**

- **Child Declaration** = `[]ChildSpec` in `DesiredState.ChildrenSpecs`
- **ChildSpec** = Complete specification (Name, WorkerType, UserSpec, StateMapping)
- **Worker** = Interface implemented by both parent and child
- **Supervisor** = Manages worker lifecycle (parent OR child)

**Resolution Strategy:**

Update master plan terminology:
- "Child declarations" → "ChildrenSpecs in DesiredState"
- "Worker" → Specific: Parent worker, Child worker
- Add section defining data structures

---

### 2.2 Template Rendering Location

**Master Plan Assumption (implied):**

Templates rendered by central service or supervisor (FSMv1 pattern)

**New Design Decision:**

Templates rendered by **worker itself** in DeriveDesiredState:

```go
func (w *BenthosWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    template := w.templateStore.Get(userSpec.TemplateName)
    config, _ := RenderTemplate(template, scope)  // ← Renders HERE
    
    return DesiredState{
        State:  "running",
        Config: config,
    }
}
```

**Why Changed:**

- **Distributed:** Worker knows what template it needs
- **Decoupled:** Supervisor doesn't need template schema knowledge
- **Flexible:** Workers without templates skip rendering
- **Idiomatic:** Each worker owns its rendering logic

**Resolution Strategy:**

Add to master plan:
- Section on template rendering architecture
- Clarify: NOT centralized service, distributed at worker level
- Explain when rendering happens (every tick, inside DeriveDesiredState)

---

### 2.3 Variable Injection Location

**Master Plan Assumption:**

No mention of where variables come from or how they flow

**New Design Decision:**

Variables injected at THREE points:

```go
// 1. USER variables: Parent creates when building ChildSpec
func (w *ParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": "active",
                            "IP":           "192.168.1.100",
                        },
                    },
                },
            },
        },
    }
}

// 2. GLOBAL variables: Supervisor injects
func (s *Supervisor) Tick() {
    userSpec.Variables.Global = s.globalVars  // Injected by supervisor
    desiredState := s.worker.DeriveDesiredState(userSpec)
}

// 3. INTERNAL variables: Supervisor injects
func (s *Supervisor) Tick() {
    userSpec.Variables.Internal = map[string]any{
        "id":         s.supervisorID,
        "created_at": s.createdAt,
    }
}
```

**Resolution Strategy:**

Add to master plan:
- Section on variable injection points
- Clarify three-tier namespace (User/Global/Internal)
- Document who injects what

---

### 2.4 Observation Collection During Circuit Open

**Master Plan Mentions (Gap Analysis section):**

> **Gap 2: Observation Collection During Circuit Open**
> 
> Decision: Keep current behavior (observations continue during circuit open)
> 
> Rationale: Fresh data available immediately when circuit closes

**New Design Consistency:**

Hierarchical composition doesn't change this. Observations still continue:

```go
// Collectors run independently
func (c *Collector) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    for range ticker.C {
        observed, _ := c.worker.CollectObservedState(ctx)
        c.store.StoreObserved(ctx, workerType, workerID, observed)
    }
}

// Circuit breaker doesn't pause collectors
func (s *Supervisor) Tick(ctx context.Context) error {
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        // Skip tick, but collectors keep running
        return nil
    }
    
    // ... rest of tick
}
```

**Resolution:** No conflict, gap analysis still valid

---

## 3. Still Valid Components

### 3.1 Infrastructure Supervision (Circuit Breaker)

**From Master Plan:**

- ✅ InfrastructureHealthChecker structure
- ✅ ExponentialBackoff implementation
- ✅ CheckChildConsistency() sanity checks
- ✅ Circuit breaker logic in Supervisor.Tick()
- ✅ Child restart with exponential backoff
- ✅ Escalation after max attempts

**Integration with Hierarchical Composition:**

No conflicts! Circuit breaker works with children:

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PHASE 1: Infrastructure checks (STILL VALID)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        return nil
    }
    
    // PHASE 2: Derive desired state (NEW - includes children)
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // PHASE 3: Reconcile children (NEW)
    s.reconcileChildren(desiredState.ChildrenSpecs)
    
    // PHASE 4: Tick children (UPDATED - use children map)
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }
}
```

**Changes Needed:**

- CheckChildConsistency() uses `s.children` map instead of hardcoded names
- Child restart uses `s.children[name].supervisor.Restart()`
- Otherwise, implementation stays the same

---

### 3.2 Async Action Executor

**From Master Plan:**

- ✅ Global ActionExecutor instance
- ✅ Worker pool pattern
- ✅ Action queueing and timeout handling
- ✅ HasActionInProgress() check
- ✅ Non-blocking tick loop

**Integration with Hierarchical Composition:**

No conflicts! Actions work per-child:

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // ...
    
    // Tick children (with action checks)
    for _, child := range s.children {
        // Check if child has action in progress
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue  // Skip this child
        }
        
        child.supervisor.Tick(ctx)
    }
}
```

**Changes Needed:**

- Action IDs use child name (not hardcoded worker ID)
- Otherwise, implementation stays the same

---

### 3.3 Integration Strategy (Layered Precedence)

**From Master Plan:**

> **Key Integration Principles:**
> 
> 1. **Layered Precedence:** Infrastructure checks run FIRST (affect all workers), action checks run SECOND (affect individual workers)
> 2. **Different Scopes:**
>    - Infrastructure: Affects ALL workers (circuit breaker stops everything)
>    - Actions: Affects SINGLE worker (just that worker pauses)

**Still Valid!** With hierarchical composition:

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health (affects all children) ← STILL VALID
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        return nil  // Skip entire tick (all children)
    }
    
    // PRIORITY 2: Per-Child Actions (affects single child) ← STILL VALID
    for _, child := range s.children {
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue  // Skip THIS child only
        }
        child.supervisor.Tick(ctx)
    }
}
```

**No changes needed** to integration strategy, just updated to use children

---

### 3.4 ExponentialBackoff

**From Master Plan:**

Complete implementation for backoff with max attempts tracking.

**Still Valid!** No changes needed:

```go
type ExponentialBackoff struct {
    baseDelay   time.Duration
    maxDelay    time.Duration
    attempts    int
    lastAttempt time.Time
}
```

Used by Infrastructure Supervision, no conflicts with hierarchical composition.

---

### 3.5 Observation Collection Pattern

**From Master Plan Gap Analysis:**

> Collectors run as **independent goroutines** (1 per worker)
> Collectors call `worker.CollectObservedState()` every 5 seconds

**Still Valid!** With hierarchical composition:

```go
// Each child has own collector
for _, child := range s.children {
    // Start collector for child
    collector := NewCollector(child.supervisor)
    go collector.Start(ctx)
}
```

**No changes** to collector architecture, just one per child

---

## 4. Integration Points

### 4.1 How Hierarchical Composition Integrates with Infrastructure Supervision

**Circuit Breaker Affects All Children:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // 1. Infrastructure check (affects all children)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        
        // Restart specific child
        if child, exists := s.children["dfc_read"]; exists {
            child.supervisor.Restart()
        }
        
        // Skip tick for ALL children
        return nil
    }
    
    s.circuitOpen = false
    
    // 2. Derive desired state (with children)
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // 3. Reconcile children
    s.reconcileChildren(desiredState.ChildrenSpecs)
    
    // 4. Tick children (if circuit closed)
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }
}
```

**Child Consistency Checks Use Children Map:**

```go
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Example: Redpanda vs Benthos consistency
    
    // Get Redpanda child
    redpandaChild, exists := ihc.supervisor.children["redpanda"]
    if !exists {
        return nil  // Child not declared, skip
    }
    
    // Get Benthos child
    benthosChild, exists := ihc.supervisor.children["dfc_read"]
    if !exists {
        return nil  // Child not declared, skip
    }
    
    // Check consistency
    redpandaState := redpandaChild.supervisor.GetDesiredState().State
    benthosObserved := benthosChild.supervisor.GetObservedState()
    
    if redpandaState == "active" && benthosObserved.OutputConnections == 0 {
        return fmt.Errorf("redpanda active but benthos has 0 output connections")
    }
    
    return nil
}
```

**Key Points:**

- ✅ Circuit breaker still pauses ALL children
- ✅ Child restart uses children map
- ✅ Consistency checks use children map
- ✅ No changes to backoff logic

---

### 4.2 How Child Supervisors Work with Circuit Breaker

**Recursive Supervision:**

```
┌──────────────────────────────────────┐
│ Parent Supervisor                    │
│                                      │
│ if circuitOpen {                     │
│   return nil  // Skip tick           │
│ }                                    │
│                                      │
│ for _, child := range s.children {  │
│   child.supervisor.Tick(ctx)  ◄─────┼─── Child supervisor ALSO has circuit breaker!
│ }                                    │
└──────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Child Supervisor (Redpanda)          │
│                                      │
│ if circuitOpen {                     │
│   return nil  // Skip tick           │
│ }                                    │
│                                      │
│ // Redpanda's own infrastructure     │
│ // checks (if any)                   │
└──────────────────────────────────────┘
```

**Key Points:**

- Each supervisor has own InfrastructureHealthChecker
- Parent circuit breaker pauses parent + all children
- Child circuit breaker pauses only that child

**Example:**

```
Parent (Bridge):
  - Circuit OPEN: All children (connection, dfc_read, dfc_write) paused
  - Circuit CLOSED: Children tick normally

Child (DFC Read):
  - Circuit OPEN: Only DFC Read paused, siblings keep running
  - Circuit CLOSED: DFC Read ticks normally
```

---

### 4.3 How Templates Render During Tick Loop

**Template Rendering in Child's DeriveDesiredState:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // ...
    
    // Derive desired state (templates render HERE)
    desiredState := s.worker.DeriveDesiredState(userSpec)
    //                                           ↑
    //                              Worker renders templates inside this call
    
    // ...
}

// Inside worker
func (w *BenthosWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()
    
    // Template rendering happens HERE
    template := w.templateStore.Get(userSpec.TemplateName)
    config, _ := RenderTemplate(template, scope)
    
    return DesiredState{
        State:  "running",
        Config: config,
    }
}
```

**Tick Loop Order:**

```
1. Infrastructure check (circuit breaker)
   ↓
2. DeriveDesiredState() ← TEMPLATES RENDER HERE
   ↓
3. Reconcile children
   ↓
4. Apply state mapping
   ↓
5. Tick children (recursive) ← Child templates render in THEIR DeriveDesiredState
   ↓
6. Collect observed state
```

**Every Tick:**

- Parent renders its own templates (if any)
- Parent creates ChildSpecs with Variables
- Child supervisor calls child.DeriveDesiredState()
- Child renders its own templates

**Performance:**

- Template rendering: ~100μs per worker
- 100 workers × 1 tick/sec = 10ms CPU/sec
- Negligible impact

---

### 4.4 How Variables Flow Through Supervisor Layers

**Variable Flow Diagram:**

```
┌─────────────────────────────────────────────────────┐
│ Config.yaml                                         │
│   agent:                                            │
│     location: {enterprise: "ACME", site: "Factory"} │
│     global: {kafka_brokers: "localhost:9092"}      │
│                                                     │
│   protocolConverters:                               │
│     - connection:                                   │
│         variables:                                  │
│           IP: "192.168.1.100"                       │
│           PORT: 502                                 │
│       location: {line: "Line-A"}                    │
└─────────────────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│ Parent Supervisor                                   │
│   userSpec.Variables:                               │
│     User:                                           │
│       IP: "192.168.1.100"                           │
│       PORT: 502                                     │
│       location: {enterprise: "ACME", ...}           │
│     Global:                                         │
│       kafka_brokers: "localhost:9092"               │
│     Internal: (supervisor injects)                  │
│       id: "bridge-123"                              │
└─────────────────────────────────────────────────────┘
                       │
                       ▼ DeriveDesiredState()
┌─────────────────────────────────────────────────────┐
│ Parent Worker                                       │
│   Merges location:                                  │
│     parent: {enterprise: "ACME", site: "Factory"}   │
│     child:  {line: "Line-A", bridge: "PLC"}         │
│     merged: {enterprise: "ACME", ..., bridge: "PLC"}│
│                                                     │
│   Creates ChildSpec with Variables:                 │
│     User:                                           │
│       parent_state: "active"                        │
│       IP: "192.168.1.100"                           │
│       PORT: 502                                     │
│       location: {enterprise: "ACME", ..., bridge}   │
│       location_path: "ACME.Factory.Line-A.PLC"      │
│     Global: (pass through)                          │
│       kafka_brokers: "localhost:9092"               │
└─────────────────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│ Child Supervisor                                    │
│   userSpec.Variables:                               │
│     User: (from parent)                             │
│       parent_state: "active"                        │
│       IP: "192.168.1.100"                           │
│       location_path: "ACME.Factory.Line-A.PLC"      │
│     Global: (passed through)                        │
│       kafka_brokers: "localhost:9092"               │
│     Internal: (child supervisor injects)            │
│       id: "dfc_read"                                │
└─────────────────────────────────────────────────────┘
                       │
                       ▼ DeriveDesiredState()
┌─────────────────────────────────────────────────────┐
│ Child Worker (Benthos)                              │
│   Flattens variables:                               │
│     scope = {                                       │
│       "parent_state": "active",                     │
│       "IP": "192.168.1.100",                        │
│       "location_path": "ACME.Factory.Line-A.PLC",   │
│       "global": {kafka_brokers: "localhost:9092"},  │
│       "internal": {id: "dfc_read"}                  │
│     }                                               │
│                                                     │
│   Renders template:                                 │
│     "{{ .IP }}:{{ .PORT }}" → "192.168.1.100:502"   │
│     "{{ .location_path }}" → "ACME.Factory.Line-A.PLC"│
└─────────────────────────────────────────────────────┘
```

**Three Injection Points:**

1. **User Variables:** Parent constructs when creating ChildSpec
2. **Global Variables:** Supervisor injects before calling DeriveDesiredState
3. **Internal Variables:** Supervisor injects (id, timestamps)

**Key Points:**

- ✅ Variables flow downward (parent → child)
- ✅ Global variables passed through unchanged
- ✅ Location merges at each level
- ✅ Parent state injected as variable
- ✅ Each level flattens for template access

---

### 4.5 Where WorkerFactory Fits in Supervisor Creation

**Supervisor Creation Flow:**

```go
// 1. Root supervisor created by Agent
func (a *Agent) CreateBridgeSupervisor(bridgeConfig BridgeConfig) *Supervisor {
    factory := NewDefaultWorkerFactory(a.logger)
    
    worker, _ := factory.CreateWorker("BridgeWorker", bridgeConfig.UserSpec)
    
    supervisor := NewSupervisor(worker, factory, a.logger)
    //                                  ↑
    //                          Factory passed to supervisor
    
    return supervisor
}

// 2. Supervisor uses factory to create child supervisors
func (s *Supervisor) reconcileChildren(specs []ChildSpec) error {
    for name, spec := range desiredChildren {
        if _, exists := s.children[name]; !exists {
            // Create child worker from spec
            worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
            //                ↑
            //         Uses injected factory
            
            // Create child supervisor (with same factory!)
            childSupervisor := NewSupervisor(worker, s.factory, s.logger)
            //                                       ↑
            //                              Factory passed down
            
            s.children[name] = &childInstance{
                supervisor: childSupervisor,
            }
        }
    }
}
```

**Factory Injection Pattern:**

```
Agent
  ↓ creates factory
  ↓ passes to root supervisor
  ↓
Root Supervisor (Bridge)
  ↓ uses factory to create child workers
  ↓ passes factory to child supervisors
  ↓
Child Supervisor (Connection)
  ↓ uses factory to create grandchild workers
  ↓ passes factory to grandchild supervisors
  ↓
Grandchild Supervisor (...)
```

**Key Points:**

- ✅ Single factory instance for entire hierarchy
- ✅ Factory injected at root supervisor creation
- ✅ Factory passed down to all child supervisors
- ✅ Each supervisor uses factory to create its children

**Constructor Signature Change:**

```go
// OLD (master plan assumption)
func NewSupervisor(worker Worker, logger *zap.Logger) *Supervisor

// NEW (with factory)
func NewSupervisor(worker Worker, factory WorkerFactory, logger *zap.Logger) *Supervisor
```

---

## 5. Scope Changes

### 5.1 Original Scope Estimate (Master Plan)

**From Master Plan:**

- Phase 1: Infrastructure Supervision Foundation (Week 1-2)
- Phase 2: Async Action Executor (Week 3-4)
- Phase 3: Integration & Edge Cases (Week 5)
- Phase 4: Monitoring & Observability (Week 6)

**Total:** 6 weeks

**Components:**

1. ExponentialBackoff (~50 lines)
2. InfrastructureHealthChecker (~100 lines)
3. Circuit breaker integration (~50 lines)
4. ActionExecutor (~150 lines)
5. Worker pool (~100 lines)
6. Integration tests (~200 lines)
7. Monitoring metrics (~100 lines)

**Total LOC (original):** ~750 lines

---

### 5.2 New Scope Estimate (With Hierarchical Composition)

**New Components:**

1. **Hierarchical Composition** (Phase 0 - NEW)
   - ChildSpec structures: ~50 lines
   - Supervisor.reconcileChildren(): ~80 lines
   - WorkerFactory: ~30 lines
   - Child lifecycle management: ~50 lines
   - Tests: ~200 lines
   - **Subtotal: ~410 lines**

2. **Templating & Variables** (Phase 3 - NEW)
   - VariableBundle structures: ~30 lines
   - Flatten() method: ~15 lines
   - RenderTemplate(): ~40 lines
   - Location merging: ~60 lines
   - Location path computation: ~30 lines
   - Tests: ~150 lines
   - **Subtotal: ~325 lines**

3. **DeriveDesiredState API Changes**
   - DesiredState struct update: ~10 lines
   - Worker interface update: ~5 lines
   - TriangularStore updates: ~50 lines
   - Migration guide: documentation only
   - **Subtotal: ~65 lines**

4. **StateMapping Integration**
   - StateMapping field: ~5 lines
   - applyStateMapping(): ~15 lines
   - Tests: ~50 lines
   - **Subtotal: ~70 lines**

5. **Original Components** (from master plan)
   - Infrastructure Supervision: ~300 lines
   - Async Action Executor: ~250 lines
   - Integration tests: ~200 lines
   - Monitoring: ~100 lines
   - **Subtotal: ~850 lines**

**Total LOC (new):** ~1,720 lines (+129% increase)

**New Timeline:**

- Phase 0: Hierarchical Composition Foundation (Week 1-2) - NEW
- Phase 1: Infrastructure Supervision (Week 3-4)
- Phase 2: Async Action Executor (Week 5-6)
- Phase 3: Templating & Variables System (Week 7-8) - NEW
- Phase 4: Integration & Edge Cases (Week 9-10)
- Phase 5: Monitoring & Observability (Week 11-12)

**Total:** 12 weeks (+100% increase)

**Scope Breakdown:**

| Component | Original Estimate | New Estimate | Change |
|-----------|------------------|--------------|--------|
| **Code Lines** | 750 | 1,720 | +129% |
| **Weeks** | 6 | 12 | +100% |
| **Phases** | 4 | 6 | +50% |
| **Core Patterns** | 2 (Supervision + Actions) | 5 (+ Composition + Templates + StateMapping) | +150% |

---

### 5.3 What's Bigger Than Originally Planned

**1. Supervisor Complexity**

**Original:** Simple tick loop with action checks

**New:** Complex tick loop with:
- Child reconciliation (add/remove/update)
- State mapping application
- Template rendering coordination
- Variable injection (3 tiers)
- Location merging

**Impact:** Supervisor.Tick() grew from ~30 lines to ~80 lines

---

**2. Worker Responsibilities**

**Original:** Just DeriveDesiredState() + CollectObservedState()

**New:** Workers also:
- Create ChildSpecs in DeriveDesiredState()
- Render templates (if needed)
- Merge location hierarchies
- Pass variables to children
- Compute location paths

**Impact:** Worker implementations grew from ~15 lines to ~40 lines

---

**3. Data Structures**

**Original:** Simple state strings

**New:** Rich structures:
- DesiredState (State + ChildrenSpecs)
- ChildSpec (Name + WorkerType + UserSpec + StateMapping)
- VariableBundle (User + Global + Internal)
- UserSpec (typed fields + Variables)

**Impact:** 4 new complex structures, ~150 lines total

---

**4. Testing Surface Area**

**Original:**
- Test Infrastructure Supervision
- Test Async Actions
- Test integration

**New:** Also test:
- Child reconciliation (add/remove/update)
- Template rendering (strict mode, variable access)
- Variable flattening
- Location merging and computation
- StateMapping application
- WorkerFactory creation
- Parent-child variable flow
- Serialization/deserialization

**Impact:** ~400 more lines of tests

---

### 5.4 What's Simpler Than Originally Planned

**1. No "child observed state usage" complexity**

**Original assumption (from master plan references):**

Master plan referenced:
> `fsmv2-child-observed-state-usage.md` - Parents should NOT access child observed state in business logic

**New design:** Parent CAN pass observed to child as variable, but:
- Parent's own observed state (from previous tick)
- No complex child state access patterns
- Child decides what to do with parent_observed

**Simplification:** No special API for parent accessing child observed state

---

**2. No double-calling pattern**

**Potential complexity avoided:**

Could have had separate methods:
- `DeclareChildren()` called when config changes
- `DeriveDesiredState()` called every tick

**New design:** Single method returns everything:

```go
DeriveDesiredState(userSpec) DesiredState  // Returns state + children
```

**Simplification:** One method, one call, one return

---

**3. No template → runtime type conversion**

**FSMv1 pattern (avoided):**

- Phase 1: Render template (strings)
- Phase 2: Convert to runtime types (uint16, etc.)

**FSMv2 pattern:**

- UserSpec already has typed fields
- Templates only for worker-internal configs
- No conversion needed

**Simplification:** One-phase instead of two-phase

---

### 5.5 What's Entirely New

**1. Hierarchical Composition** (100% new)

Not mentioned in master plan:
- ChildSpec structure
- Parent creates child specifications
- Supervisor reconciles children
- Dynamic children (OPC UA browser)

**Lines of code:** ~410 lines

---

**2. Templating & Variables System** (100% new)

Not mentioned in master plan:
- VariableBundle (User/Global/Internal)
- Flatten() for template access
- RenderTemplate() with strict mode
- Location merging and computation

**Lines of code:** ~325 lines

---

**3. StateMapping Pattern** (100% new)

Not mentioned in master plan:
- Parent declares mapping
- Supervisor applies mapping
- Child can override

**Lines of code:** ~70 lines

---

**4. WorkerFactory Pattern** (100% new)

Not mentioned in master plan:
- String-based worker types
- Dynamic worker creation
- Factory injection

**Lines of code:** ~75 lines

---

### 5.6 What Can Be Deferred

**1. Template Rendering Optimization**

**Current plan:** Render templates every tick

**Possible optimization (defer to Phase 6+):**

```go
type Worker struct {
    lastVariablesHash string
    cachedConfig      Config
}

func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    hash := hashVariables(userSpec.Variables)
    if hash != w.lastVariablesHash {
        w.cachedConfig = RenderTemplate(...)
        w.lastVariablesHash = hash
    }
    return DesiredState{Config: w.cachedConfig}
}
```

**Rationale for deferral:**
- Premature optimization
- Template rendering ~100μs (negligible)
- Only matters at 1000+ workers
- Can add later if profiling shows need

---

**2. Complex Location Validation**

**Current plan:** Gap filling with "unknown"

**Possible enhancement (defer to Phase 6+):**

- Validate location hierarchy at config parsing
- Reject configs with gaps
- Enforce ISA-95 compliance

**Rationale for deferral:**
- Gap filling works (graceful degradation)
- Validation belongs in config parsing, not runtime
- Can add stricter validation later

---

**3. Advanced Child Observed Access**

**Current plan:** Parent can pass parent_observed as variable

**Possible enhancement (defer to Phase 6+):**

```go
type Worker interface {
    // NEW: Access child observed state
    GetChildObservedState(childName string) (ObservedState, error)
}
```

**Rationale for deferral:**
- Current pattern (parent_observed variable) sufficient
- Design doc says "parents should NOT access child observed state in business logic"
- Infrastructure Supervision covers sanity checks
- Can add if use case emerges

---

**4. Metadata Approach for TriangularStore**

**Current plan:** ChildrenSpecs in DesiredState (full specs)

**Possible enhancement (defer to Phase 6+):**

```go
type DesiredState struct {
    State         string
    ChildMetadata []ChildMetadata  // Serializable metadata only
}

type ChildMetadata struct {
    Name         string
    TemplateRef  string
    ConfigHash   string
}
```

**Rationale for deferral:**
- Current approach (full ChildSpecs) works
- Serialization works (UserSpec is serializable)
- Metadata approach adds complexity
- Can optimize later if serialization becomes issue

---

## 6. Recommendations Summary

### Top 5 Changes Needed to Master Plan

**1. Add Phase 0: Hierarchical Composition Foundation (CRITICAL)**

**What:** 2-week phase BEFORE Infrastructure Supervision

**Contents:**
- Define ChildSpec structure
- Implement reconcileChildren()
- Create WorkerFactory pattern
- Update Worker interface (DeriveDesiredState returns DesiredState)
- Test child add/remove/update

**Why:** Children must exist before we can supervise them

**Impact:** +2 weeks to timeline

**Priority:** MUST DO (blocking for all other phases)

---

**2. Add Phase 3: Templating & Variables System (CRITICAL)**

**What:** 2-week phase for templating infrastructure

**Contents:**
- Implement VariableBundle (User/Global/Internal)
- Implement Flatten() method
- Implement RenderTemplate() with strict mode
- Implement location merging and computation
- Test template rendering and variable access

**Why:** Benthos workers need templates to generate configs

**Impact:** +2 weeks to timeline

**Priority:** MUST DO (blocks Benthos integration)

---

**3. Update Integration Strategy Section (HIGH)**

**What:** Clarify how hierarchical composition integrates with existing patterns

**Add:**
- Section: "Hierarchical Composition Integration"
- How circuit breaker affects children
- How actions queue per child
- How templates render in DeriveDesiredState
- How variables flow through supervisor layers
- WorkerFactory injection pattern

**Why:** Implementation clarity

**Impact:** +1 section to master plan

**Priority:** SHOULD DO (clarity for implementers)

---

**4. Update Data Structures Section (HIGH)**

**What:** Define all data structures upfront

**Add:**
- DesiredState (State + ChildrenSpecs)
- ChildSpec (Name + WorkerType + UserSpec + StateMapping)
- VariableBundle (User + Global + Internal)
- UserSpec pattern (strict types + Variables)

**Why:** API contracts must be clear

**Impact:** +1 section to master plan

**Priority:** SHOULD DO (contracts before implementation)

---

**5. Update Timeline & Scope Section (MEDIUM)**

**What:** Realistic timeline with all new components

**Update:**
- Original: 6 weeks → New: 12 weeks
- Original: 750 LOC → New: 1,720 LOC
- Original: 4 phases → New: 6 phases
- Original: 2 patterns → New: 5 patterns

**Why:** Accurate project planning

**Impact:** Timeline doubled

**Priority:** SHOULD DO (stakeholder expectation management)

---

### Recommended Phase Order

**Phase 0: Hierarchical Composition Foundation (Week 1-2) - NEW**

Tasks:
1. Define ChildSpec, DesiredState structures
2. Update Worker.DeriveDesiredState() signature
3. Implement Supervisor.reconcileChildren()
4. Implement WorkerFactory
5. Test child add/remove/update
6. Test WorkerType → Worker creation

Dependencies: None (foundational)
Blocks: ALL other phases

---

**Phase 1: Infrastructure Supervision (Week 3-4) - EXISTING**

Tasks:
1. Implement ExponentialBackoff
2. Implement InfrastructureHealthChecker
3. Implement CheckChildConsistency (using s.children map)
4. Circuit breaker integration
5. Child restart logic

Dependencies: Phase 0 (needs children to exist)
Blocks: None

---

**Phase 2: Async Action Executor (Week 5-6) - EXISTING**

Tasks:
1. Implement ActionExecutor
2. Worker pool
3. Action queueing
4. HasActionInProgress() check
5. Integration with Supervisor.Tick()

Dependencies: Phase 0 (actions queue per child)
Blocks: None

---

**Phase 3: Templating & Variables System (Week 7-8) - NEW**

Tasks:
1. Implement VariableBundle structures
2. Implement Flatten() method
3. Implement RenderTemplate()
4. Implement location merging
5. Implement location path computation
6. Test template rendering
7. Test variable access in templates

Dependencies: Phase 0 (variables in UserSpec)
Blocks: Benthos worker implementation

---

**Phase 4: StateMapping Integration (Week 9) - NEW**

Tasks:
1. Add StateMapping field to ChildSpec
2. Implement applyStateMapping()
3. Test state mapping application
4. Test hybrid approach (mapping + child autonomy)

Dependencies: Phase 0, Phase 3 (uses Variables)
Blocks: None

---

**Phase 5: Integration & Edge Cases (Week 10-11) - UPDATED**

Tasks:
1. Combined tick loop with all features
2. Action behavior during child restart
3. Observation collection during circuit open
4. Template rendering performance tests
5. Variable flow tests (parent → child)
6. Location hierarchy tests

Dependencies: ALL previous phases
Blocks: None

---

**Phase 6: Monitoring & Observability (Week 12) - EXISTING**

Tasks:
1. Prometheus metrics (infrastructure + actions + children)
2. Detailed logging
3. Runbook
4. Performance benchmarks

Dependencies: Phase 5 (needs integration)
Blocks: None

---

### Critical vs Nice-to-Have Updates

**CRITICAL (Must Do):**

1. ✅ Add Phase 0: Hierarchical Composition Foundation
2. ✅ Add Phase 3: Templating & Variables System
3. ✅ Update Worker.DeriveDesiredState() signature
4. ✅ Define ChildSpec, DesiredState structures
5. ✅ Update timeline (6 weeks → 12 weeks)

**HIGH (Should Do):**

6. ✅ Add Integration Strategy section (how components fit together)
7. ✅ Add Data Structures section (API contracts)
8. ✅ Update InfrastructureHealthChecker to use s.children map
9. ✅ Document WorkerFactory pattern
10. ✅ Document variable injection points

**MEDIUM (Nice-to-Have):**

11. ⚠️ Add examples for each worker pattern
12. ⚠️ Add sequence diagrams for tick loop
13. ⚠️ Add comparison table (original vs new scope)
14. ⚠️ Add migration guide from current API
15. ⚠️ Add optimization strategies (deferred to future)

**LOW (Can Skip Initially):**

16. ❌ Detailed performance analysis
17. ❌ Template rendering optimization strategies
18. ❌ Advanced child observed access patterns
19. ❌ Metadata approach for TriangularStore
20. ❌ Location hierarchy validation strategies

---

## 7. Conclusion

The master plan is a solid foundation for Infrastructure Supervision and Async Action Executor, but **hierarchical composition design decisions made since the master plan was written represent a significant scope expansion**.

**Key Findings:**

1. **Scope doubled:** 6 weeks → 12 weeks, 750 LOC → 1,720 LOC
2. **New patterns introduced:** Hierarchical composition, templating, variables, StateMapping
3. **Integration points clear:** New components integrate cleanly with existing patterns
4. **Original components still valid:** Infrastructure Supervision and Async Actions need minor updates, not rewrites

**Immediate Actions:**

1. Update master plan with new Phase 0 and Phase 3
2. Define all data structures upfront (ChildSpec, DesiredState, VariableBundle)
3. Update timeline and communicate scope change
4. Add integration strategy section
5. Prioritize Phase 0 (blocks everything else)

**Risk Mitigation:**

- Phase 0 is foundational - must get right before proceeding
- Template rendering is unfamiliar - allocate time for learning/iteration
- Scope creep risk - defer optimizations (caching, metadata, advanced patterns)

**Next Steps:**

1. Review this gap analysis with team
2. Approve updated timeline (12 weeks)
3. Update master plan document
4. Begin Phase 0 implementation
5. Schedule checkpoints after each phase

---

## Appendix A: Master Plan vs New Design API Comparison

### Worker Interface

**Master Plan Assumption (implied):**

```go
type Worker interface {
    DeriveDesiredState() State
    CollectObservedState(ctx context.Context) (ObservedState, error)
}
```

**New Design:**

```go
type Worker interface {
    DeriveDesiredState(userSpec UserSpec) DesiredState
    CollectObservedState(ctx context.Context) (ObservedState, error)
}

type DesiredState struct {
    State         string
    ChildrenSpecs []ChildSpec
}
```

**Changes:**

1. Parameter added: `userSpec UserSpec`
2. Return type: `State` → `DesiredState`
3. New field: `ChildrenSpecs []ChildSpec`

---

### Supervisor Tick Loop

**Master Plan Pseudocode:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        time.Sleep(backoff)
        return nil
    }

    // PRIORITY 2: Per-Worker Actions
    for workerID, workerCtx := range s.workers {
        snapshot, _ := s.store.LoadSnapshot(ctx, workerType, workerID)

        if s.actionExecutor.HasActionInProgress(workerID) {
            continue
        }

        nextState, _, action := currentState.Next(snapshot)
        s.actionExecutor.EnqueueAction(workerID, action, registry)
    }
}
```

**New Design:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PHASE 1: Infrastructure Health (SAME)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        return nil
    }

    // PHASE 2: Derive Desired State (NEW - includes children)
    userSpec := s.triangularStore.GetUserSpec()
    desiredState := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desiredState)

    // PHASE 3: Reconcile Children (NEW)
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // PHASE 4: Apply State Mapping (NEW)
    s.applyStateMapping(desiredState.State)

    // PHASE 5: Tick Children (NEW - recursive)
    for _, child := range s.children {
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue
        }
        child.supervisor.Tick(ctx)
    }

    // PHASE 6: Collect Observed State
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)

    return nil
}
```

**Changes:**

1. Added Phase 2: Derive desired state (with children)
2. Added Phase 3: Reconcile children
3. Added Phase 4: Apply state mapping
4. Updated Phase 5: Tick children (instead of workers)
5. Children tick recursively (each has own supervisor)

---

## Appendix B: Lines of Code Breakdown

### Original Master Plan Estimate

| Component | LOC | Phase |
|-----------|-----|-------|
| ExponentialBackoff | 50 | 1 |
| InfrastructureHealthChecker | 100 | 1 |
| CheckChildConsistency | 30 | 1 |
| Circuit breaker integration | 50 | 1 |
| ActionExecutor | 100 | 2 |
| Worker pool | 80 | 2 |
| Action queueing | 70 | 2 |
| Integration tests | 200 | 3 |
| Monitoring metrics | 70 | 4 |
| **TOTAL** | **750** | |

---

### New Design Estimate

| Component | LOC | Phase |
|-----------|-----|-------|
| **Phase 0: Hierarchical Composition** | | |
| ChildSpec structures | 50 | 0 |
| Supervisor.reconcileChildren() | 80 | 0 |
| WorkerFactory | 30 | 0 |
| Child lifecycle management | 50 | 0 |
| DesiredState update | 10 | 0 |
| Tests (composition) | 200 | 0 |
| **Phase 0 Subtotal** | **420** | |
| | | |
| **Phase 1: Infrastructure Supervision** | | |
| ExponentialBackoff | 50 | 1 |
| InfrastructureHealthChecker | 100 | 1 |
| CheckChildConsistency (updated) | 40 | 1 |
| Circuit breaker integration | 50 | 1 |
| Child restart logic | 30 | 1 |
| Tests (infrastructure) | 100 | 1 |
| **Phase 1 Subtotal** | **370** | |
| | | |
| **Phase 2: Async Action Executor** | | |
| ActionExecutor | 100 | 2 |
| Worker pool | 80 | 2 |
| Action queueing | 70 | 2 |
| Tests (actions) | 80 | 2 |
| **Phase 2 Subtotal** | **330** | |
| | | |
| **Phase 3: Templating & Variables** | | |
| VariableBundle structures | 30 | 3 |
| Flatten() method | 15 | 3 |
| RenderTemplate() | 40 | 3 |
| Location merging | 60 | 3 |
| Location path computation | 30 | 3 |
| Tests (templating) | 150 | 3 |
| **Phase 3 Subtotal** | **325** | |
| | | |
| **Phase 4: StateMapping** | | |
| StateMapping field | 5 | 4 |
| applyStateMapping() | 15 | 4 |
| Tests (state mapping) | 50 | 4 |
| **Phase 4 Subtotal** | **70** | |
| | | |
| **Phase 5: Integration** | | |
| Integration tests | 150 | 5 |
| Edge case tests | 80 | 5 |
| **Phase 5 Subtotal** | **230** | |
| | | |
| **Phase 6: Monitoring** | | |
| Prometheus metrics | 100 | 6 |
| Logging | 50 | 6 |
| Runbook | 0 (docs) | 6 |
| **Phase 6 Subtotal** | **150** | |
| | | |
| **GRAND TOTAL** | **1,895** | |

**Adjusted estimate:** ~1,720 lines (accounting for some overlap)

**Increase:** +129% from original estimate

---

## Appendix C: Timeline Gantt Chart

```
Week 1-2:  Phase 0: Hierarchical Composition Foundation (NEW)
           ████████████████████████████

Week 3-4:  Phase 1: Infrastructure Supervision
                                          ████████████████████████████

Week 5-6:  Phase 2: Async Action Executor
                                                                    ████████████████████████████

Week 7-8:  Phase 3: Templating & Variables (NEW)
                                                                                              ████████████████████████████

Week 9-10: Phase 4: StateMapping + Integration
                                                                                                                        ████████████████████████████

Week 11-12: Phase 5: Monitoring & Observability
                                                                                                                                                  ████████████████████████████

Legend:
████ = Active development
```

**Critical Path:**

Phase 0 → Phase 1 (Infrastructure needs children)
Phase 0 → Phase 3 (Templates need ChildSpecs)
Phase 3 → Benthos worker implementation
ALL → Phase 5 (Integration)

**Parallelization Opportunities:**

- Phase 1 and Phase 2 can run in parallel (after Phase 0)
- Phase 3 can start after Phase 0 completes

**Optimized Timeline (with parallelization):**

- Weeks 1-2: Phase 0
- Weeks 3-4: Phase 1 + Phase 2 (parallel)
- Weeks 5-6: Phase 3
- Weeks 7-8: Phase 4
- Weeks 9-10: Phase 5
- Week 11: Phase 6

**Best case:** 11 weeks (if parallelization successful)
**Realistic:** 12 weeks (accounting for integration overhead)

---

