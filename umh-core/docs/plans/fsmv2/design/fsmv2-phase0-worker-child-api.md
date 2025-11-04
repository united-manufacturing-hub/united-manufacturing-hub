# FSMv2 Phase 0: Worker Child API - Minimal Declarative Approach

**Created:** 2025-11-02
**Status:** Proposal
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition
**Related:** 
- `fsmv2-supervisor-composition-declarative.md` (original DeclareChildren API)
- `fsmv2-infrastructure-supervision-patterns.md` (circuit breaker patterns)
- `2025-11-02-fsmv2-plan-evaluation.md` (critique of previous plan)

---

## Executive Summary

### Phase 0 Goal

**Establish the minimal API that lets workers declare and control children, with zero infrastructure mixing.**

**User's Vision:**
1. Workers declare children (structure)
2. Workers adjust child desired state (parent state → child desired state)
3. NO sanity checks in workers (removed complexity)
4. Clear boundaries maintained (workers never see infrastructure)

**Core Principle:** Infrastructure stays invisible. Workers declare intent, supervisor executes.

### What Phase 0 Delivers

✅ **DeclareChildren() API** - Workers declare child structure once  
✅ **Desired State Control** - Workers map parent state → child desired state  
✅ **Clean Separation** - Workers NEVER see infrastructure (crashes, timeouts, health)  
✅ **Simplicity First** - Minimal boilerplate, maximum clarity  
❌ **NOT in Phase 0:** Sanity checks, circuit breaker, async actions (come later)

---

## Table of Contents

1. [User's Vision Structured](#users-vision-structured)
2. [API Proposal](#api-proposal)
3. [Approach Comparison](#approach-comparison)
4. [Without Sanity Checks](#without-sanity-checks)
5. [Phase 0 Implementation Steps](#phase-0-implementation-steps)
6. [Complete Worker Example](#complete-worker-example)
7. [What's NOT in Phase 0](#whats-not-in-phase-0)
8. [Open Questions](#open-questions)

---

## User's Vision Structured

### 1. "Remove sanity checks entirely - they mix things up"

**Problem in FSMv1:**
```go
// pkg/fsm/protocolconverter/actions.go:496-501
isBenthosOutputActive := outputMetrics.ConnectionUp - outputMetrics.ConnectionLost > 0

if redpandaState == "active" && !isBenthosOutputActive {
    return true, "Redpanda is active, but the flow has no output active"
}
```

This mixes:
- **Infrastructure check** (detecting cross-child inconsistency)
- **Business logic state** (parent checking child metrics)

**Phase 0 Decision:** **Remove entirely from worker API.**
- Workers do NOT check child health
- Workers do NOT access child observed state
- Workers ONLY query child FSM state name (string)

**Where sanity checks go:** Deferred to future phase (Phase 1 or 2)
- Implemented in supervisor (circuit breaker pattern)
- Workers never know they exist

### 2. "Phase 0 should define API for workers to declare children"

**What workers need to do:**
1. Declare which children they have (structure)
2. Define how parent state affects child desired state (state mapping)

**What supervisor needs to provide:**
1. Tick children before parent
2. Aggregate child states for parent to query
3. Apply state mapping automatically

**API Surface:**
```go
// Workers implement
type Worker interface {
    DeclareChildren() *ChildDeclaration  // Called once during setup
}

// Workers use in state machine
type Snapshot struct {
    GetChildState(name string) string  // Simple, no errors
}
```

### 3. "Workers must be able to adjust child desired state (user specs)"

**What this means:**
- "When parent is in 'active' state, set child desired state to 'running'"
- "When parent is in 'stopping' state, set child desired state to 'down'"

**Two approaches:**

**Approach A: Declarative State Mapping (RECOMMENDED)**
```go
func (w *Worker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                StateMapping: map[string]string{
                    "active": "up",      // Automatic
                    "idle":   "down",    // Automatic
                },
            },
        },
    }
}
```
✅ Simple, declarative  
✅ No boilerplate in state transitions  
❌ Less flexible for complex logic

**Approach B: Explicit Control (Alternative)**
```go
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Explicit control
    snapshot.SetChildDesiredState("connection", "up")
    
    connectionState := snapshot.GetChildState("connection")
    if connectionState != "up" {
        return s, SignalNone, nil  // Wait
    }
    
    return &IdleState{}, SignalNone, nil
}
```
✅ More flexible  
❌ More boilerplate (every state must set child desired state)  
❌ Easy to forget

**Phase 0 Recommendation:** **Approach A (Declarative)** with escape hatch for complex cases.

---

## API Proposal

### Worker Interface (Phase 0)

```go
// pkg/fsmv2/worker.go

type Worker interface {
    // Existing (FSMv2)
    CollectObservedState(ctx context.Context) (ObservedState, error)
    
    // NEW - Phase 0 only
    // Returns nil for non-composite workers (no children)
    DeclareChildren() *ChildDeclaration
}
```

### ChildDeclaration Structure

```go
// pkg/fsmv2/child_declaration.go

type ChildDeclaration struct {
    // List of child supervisors and their configurations
    Children []ChildSpec
}

type ChildSpec struct {
    // Unique identifier within parent
    Name string
    
    // The child supervisor instance
    Supervisor *Supervisor
    
    // How parent state affects child desired state
    // Key: parent state name (e.g., "active", "idle")
    // Value: child desired state (e.g., "up", "running")
    StateMapping map[string]string
}
```

**What's NOT in Phase 0:**
- ❌ `HealthCheck` - Removed (user: "not a fan of sanity checks")
- ❌ `DependsOn` - Deferred to later phase
- ❌ `TimeBudgetWeight` - Deferred to later phase

**Why minimal:** Phase 0 focuses on basic declaration and state control only.

### Snapshot API (What Workers See)

```go
// pkg/fsmv2/snapshot.go

type Snapshot struct {
    // Existing fields
    Tick         int
    SnapshotTime time.Time
    CurrentState State
    Observed     ObservedState
    
    // NEW - Phase 0 only
    supervisor   *Supervisor  // Internal, not exposed
}

// Simple child state query - NO ERRORS
func (s *Snapshot) GetChildState(name string) string {
    childState, exists := s.supervisor.childStates[name]
    if !exists {
        return ""  // Child not declared
    }
    return childState.FSMState
}

// NOT EXPOSED IN PHASE 0
// func (s *Snapshot) GetChildObserved(name string) - Infrastructure mixing
// func (s *Snapshot) IsChildHealthy(name string) - Infrastructure mixing
// func (s *Snapshot) SetChildDesiredState(...) - Declarative mapping instead
```

### Supervisor Changes (Phase 0)

```go
// pkg/fsmv2/supervisor.go

type Supervisor struct {
    // Existing fields
    workers      map[string]*WorkerContext
    store        storage.Store
    stateMachine *StateMachine
    logger       *zap.SugaredLogger
    
    // NEW - Phase 0
    childSupervisors map[string]*ChildContext
    childStates      map[string]ChildState  // Cached for snapshot
}

type ChildContext struct {
    Supervisor   *Supervisor
    Spec         ChildSpec
}

type ChildState struct {
    FSMState  string  // Current FSM state from child
    // NOT in Phase 0: Healthy, LastError, ObservedState
}
```

### Supervisor Methods (Phase 0)

```go
// Called once during supervisor initialization
func (s *Supervisor) RegisterChildSupervisors(ctx context.Context) error {
    // Check if worker declares children
    worker := s.getWorker()
    declaration := worker.DeclareChildren()
    
    if declaration == nil {
        // Non-composite worker, nothing to do
        return nil
    }
    
    // Initialize child contexts
    s.childSupervisors = make(map[string]*ChildContext)
    s.childStates = make(map[string]ChildState)
    
    for _, spec := range declaration.Children {
        s.childSupervisors[spec.Name] = &ChildContext{
            Supervisor: spec.Supervisor,
            Spec:       spec,
        }
    }
    
    return nil
}

// Called every tick - ticks children BEFORE parent
func (s *Supervisor) TickWithChildren(ctx context.Context) error {
    // If no children, use simple tick
    if len(s.childSupervisors) == 0 {
        return s.Tick(ctx)  // Existing simple tick
    }
    
    // 1. Tick children first
    for name, childCtx := range s.childSupervisors {
        if err := childCtx.Supervisor.Tick(ctx); err != nil {
            s.logger.Warnf("Child %s tick failed: %v", name, err)
            // Continue - don't block parent tick
        }
        
        // Update cached child state
        s.childStates[name] = ChildState{
            FSMState: childCtx.Supervisor.GetCurrentState(),
        }
    }
    
    // 2. Update child desired states based on parent state
    s.applyStateMapping()
    
    // 3. Tick parent
    return s.Tick(ctx)
}

// Apply state mapping: parent state → child desired state
func (s *Supervisor) applyStateMapping() {
    parentState := s.stateMachine.CurrentState
    
    for name, childCtx := range s.childSupervisors {
        desiredState, ok := childCtx.Spec.StateMapping[parentState]
        if !ok {
            continue  // No mapping for this parent state
        }
        
        // Set child desired state
        childCtx.Supervisor.SetDesiredState(desiredState)
    }
}
```

---

## Approach Comparison

### Declarative State Mapping (RECOMMENDED)

**Worker declares mapping once:**
```go
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
        },
    }
}
```

**Supervisor applies mapping automatically:**
```go
func (s *Supervisor) applyStateMapping() {
    for name, childCtx := range s.childSupervisors {
        if desiredState, ok := childCtx.Spec.StateMapping[parentState]; ok {
            childCtx.Supervisor.SetDesiredState(desiredState)
        }
    }
}
```

**Pros:**
- ✅ **Minimal boilerplate** - Declare once, applied automatically
- ✅ **Clear intent** - Mapping visible in one place
- ✅ **No per-state logic** - States focus on transitions
- ✅ **Supervisor handles execution** - Workers just declare

**Cons:**
- ⚠️ **Less flexible** - Can't have complex conditional logic
- ⚠️ **No dynamic mapping** - Can't change mapping based on observed state

**When to use:** 95% of cases - simple parent-child state coordination

---

### Explicit State Control (Alternative)

**Worker sets child desired state in each transition:**
```go
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Explicit control each time
    snapshot.SetChildDesiredState("connection", "up")
    
    connectionState := snapshot.GetChildState("connection")
    if connectionState != "up" {
        return s, SignalNone, nil
    }
    
    return &IdleState{}, SignalNone, nil
}
```

**Pros:**
- ✅ **Maximum flexibility** - Full control per state
- ✅ **Conditional logic** - Can set based on observed state
- ✅ **Explicit** - Clear what each state does

**Cons:**
- ❌ **High boilerplate** - Every state must set child desired state
- ❌ **Easy to forget** - Missing one state breaks coordination
- ❌ **Scattered logic** - Mapping split across many states

**When to use:** 5% of cases - complex conditional child management

---

### Hybrid Approach (Escape Hatch)

**Default mapping + overrides when needed:**
```go
func (w *Worker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                StateMapping: map[string]string{
                    "active": "up",  // Default
                    "idle":   "down",
                },
            },
        },
    }
}

// Override in special state if needed
func (s *SpecialState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Override default mapping for this specific case
    snapshot.SetChildDesiredState("connection", "down")
    // ...
}
```

**Pros:**
- ✅ **Simple by default** - Declarative mapping handles 95%
- ✅ **Flexible when needed** - Can override specific cases
- ✅ **Best of both worlds**

**Cons:**
- ⚠️ **Two ways to do same thing** - Can be confusing
- ⚠️ **Override precedence unclear** - Which wins?

**Decision for Phase 0:** **NOT included** - Adds complexity. Revisit if declarative proves insufficient.

---

## Without Sanity Checks

### The User's Vision

**User:** "I am not a fan of having any sanity checks at all, they mix things up."

**What this means:**
- Workers do NOT check cross-child consistency
- Workers do NOT access child observed state
- Workers ONLY query child FSM state name

### Scenario: Redpanda Active, Benthos Zero Connections

**What happens WITHOUT sanity checks:**

```
T+0s:   Parent: "active" state
        Redpanda child: "active" state
        Benthos child: "active" state (FSM state)
        Benthos observed: 0 output connections (infrastructure reality)

T+1s:   Parent queries child states:
        - connectionState = "up" ✅
        - benthosState = "active" ✅
        Parent proceeds normally

T+2s:   Benthos detects its own issue:
        - No Kafka connections
        - Transitions to "error" state

T+3s:   Parent queries child states:
        - connectionState = "up" ✅
        - benthosState = "error" ❌
        Parent sees error, transitions to degraded state
```

**How infrastructure issues are detected:**
1. **Child detects its own inconsistency** - Benthos notices no Kafka connections
2. **Child transitions to error state** - Business logic state reflects reality
3. **Parent sees child error state** - Reacts according to business logic
4. **No cross-child sanity checks needed** - Each child responsible for its own health

### Is This Acceptable?

**Pros:**
- ✅ **Clean separation** - Each child owns its health
- ✅ **No infrastructure mixing** - Workers only see business states
- ✅ **Simpler worker API** - No sanity check boilerplate

**Cons:**
- ⚠️ **Slower detection** - Wait for child to realize issue
- ⚠️ **Cross-child issues invisible** - If issue is relationship between children

**Phase 0 Decision:** **Acceptable for Phase 0.**
- Sanity checks deferred to future phase (circuit breaker pattern)
- Focus on minimal API first
- Validate whether sanity checks actually needed in practice

### When Sanity Checks Might Be Added Later

**If we discover:**
1. Children don't detect their own issues fast enough
2. Cross-child inconsistencies cause data corruption
3. Recovery time too slow without proactive checks

**Then add (in future phase):**
- Supervisor-level circuit breaker
- Infrastructure health checks (invisible to workers)
- Automatic child restart on detected inconsistency

**BUT:** Phase 0 proves whether we need this at all.

---

## Phase 0 Implementation Steps

### Step 1: Define Worker API ✅

**Files to create:**
- `pkg/fsmv2/child_declaration.go`

**Files to modify:**
- `pkg/fsmv2/worker.go` - Add `DeclareChildren()` to interface

**Deliverables:**
```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeclareChildren() *ChildDeclaration  // Returns nil if no children
}

type ChildDeclaration struct {
    Children []ChildSpec
}

type ChildSpec struct {
    Name         string
    Supervisor   *Supervisor
    StateMapping map[string]string
}
```

**Test:**
```go
func TestWorkerDeclareChildren(t *testing.T) {
    worker := &SimpleWorker{}
    assert.Nil(worker.DeclareChildren())  // Non-composite
    
    parentWorker := &ProtocolConverterWorker{...}
    decl := parentWorker.DeclareChildren()
    assert.NotNil(decl)
    assert.Len(decl.Children, 2)  // connection, dfc_read
}
```

---

### Step 2: Add Snapshot.GetChildState() ✅

**Files to modify:**
- `pkg/fsmv2/snapshot.go`

**Deliverables:**
```go
type Snapshot struct {
    supervisor   *Supervisor  // Internal reference
    // ... existing fields
}

func (s *Snapshot) GetChildState(name string) string {
    childState, exists := s.supervisor.childStates[name]
    if !exists {
        return ""  // Child not declared
    }
    return childState.FSMState
}
```

**Test:**
```go
func TestSnapshotGetChildState(t *testing.T) {
    supervisor := setupSupervisorWithChildren()
    snapshot := supervisor.createSnapshot()
    
    // Query child state - simple, no errors
    connectionState := snapshot.GetChildState("connection")
    assert.Equal("up", connectionState)
    
    // Non-existent child returns empty string
    unknownState := snapshot.GetChildState("unknown")
    assert.Equal("", unknownState)
}
```

---

### Step 3: Supervisor Child Registration ✅

**Files to modify:**
- `pkg/fsmv2/supervisor.go`

**Deliverables:**
```go
type Supervisor struct {
    childSupervisors map[string]*ChildContext
    childStates      map[string]ChildState
    // ... existing fields
}

func (s *Supervisor) RegisterChildSupervisors(ctx context.Context) error {
    worker := s.getWorker()
    declaration := worker.DeclareChildren()
    
    if declaration == nil {
        return nil  // No children
    }
    
    s.childSupervisors = make(map[string]*ChildContext)
    s.childStates = make(map[string]ChildState)
    
    for _, spec := range declaration.Children {
        s.childSupervisors[spec.Name] = &ChildContext{
            Supervisor: spec.Supervisor,
            Spec:       spec,
        }
    }
    
    return nil
}
```

**Test:**
```go
func TestRegisterChildSupervisors(t *testing.T) {
    supervisor := NewSupervisor(...)
    
    // Register children
    err := supervisor.RegisterChildSupervisors(ctx)
    assert.NoError(err)
    
    // Verify children registered
    assert.Len(supervisor.childSupervisors, 2)
    assert.Contains(supervisor.childSupervisors, "connection")
    assert.Contains(supervisor.childSupervisors, "dfc_read")
}
```

---

### Step 4: Child Tick Coordination ✅

**Files to modify:**
- `pkg/fsmv2/supervisor.go`

**Deliverables:**
```go
func (s *Supervisor) TickWithChildren(ctx context.Context) error {
    if len(s.childSupervisors) == 0 {
        return s.Tick(ctx)  // Simple tick for non-composite
    }
    
    // 1. Tick children first
    for name, childCtx := range s.childSupervisors {
        if err := childCtx.Supervisor.Tick(ctx); err != nil {
            s.logger.Warnf("Child %s tick failed: %v", name, err)
        }
        
        // Update cached state
        s.childStates[name] = ChildState{
            FSMState: childCtx.Supervisor.GetCurrentState(),
        }
    }
    
    // 2. Apply state mapping
    s.applyStateMapping()
    
    // 3. Tick parent
    return s.Tick(ctx)
}
```

**Test:**
```go
func TestTickWithChildren(t *testing.T) {
    supervisor := setupSupervisorWithChildren()
    
    // Tick
    err := supervisor.TickWithChildren(ctx)
    assert.NoError(err)
    
    // Verify children ticked first
    assert.True(childConnection.TickCalled)
    assert.True(childDFC.TickCalled)
    
    // Verify parent ticked
    assert.True(parentWorker.TickCalled)
    
    // Verify child states cached
    assert.NotEmpty(supervisor.childStates["connection"])
}
```

---

### Step 5: State Mapping Application ✅

**Files to modify:**
- `pkg/fsmv2/supervisor.go`

**Deliverables:**
```go
func (s *Supervisor) applyStateMapping() {
    parentState := s.stateMachine.CurrentState
    
    for name, childCtx := range s.childSupervisors {
        desiredState, ok := childCtx.Spec.StateMapping[parentState]
        if !ok {
            continue  // No mapping for this parent state
        }
        
        // Set child desired state
        childCtx.Supervisor.SetDesiredState(desiredState)
    }
}
```

**Test:**
```go
func TestApplyStateMapping(t *testing.T) {
    supervisor := setupSupervisorWithChildren()
    
    // Parent transitions to "active"
    supervisor.stateMachine.CurrentState = "active"
    
    // Apply mapping
    supervisor.applyStateMapping()
    
    // Verify child desired states set
    assert.Equal("up", childConnection.DesiredState)
    assert.Equal("running", childDFC.DesiredState)
}
```

---

## Complete Worker Example

### Protocol Converter Worker (Phase 0)

```go
package protocolconverter

type ProtocolConverterWorker struct {
    connectionSupervisor *Supervisor
    dfcReadSupervisor    *Supervisor
    dfcWriteSupervisor   *Supervisor
}

func NewProtocolConverterWorker(
    connSup *Supervisor,
    dfcReadSup *Supervisor,
    dfcWriteSup *Supervisor,
) *ProtocolConverterWorker {
    return &ProtocolConverterWorker{
        connectionSupervisor: connSup,
        dfcReadSupervisor:    dfcReadSup,
        dfcWriteSupervisor:   dfcWriteSup,
    }
}

// DeclareChildren - called once during supervisor initialization
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name:       "connection",
                Supervisor: w.connectionSupervisor,
                StateMapping: map[string]string{
                    "starting_connection": "up",
                    "starting_redpanda":   "up",
                    "starting_dfc":        "up",
                    "idle":                "up",
                    "active":              "up",
                    "stopping":            "down",
                },
            },
            {
                Name:       "dfc_read",
                Supervisor: w.dfcReadSupervisor,
                StateMapping: map[string]string{
                    "starting_dfc":  "running",
                    "idle":          "running",
                    "active":        "running",
                    "stopping":      "stopped",
                },
            },
            {
                Name:       "dfc_write",
                Supervisor: w.dfcWriteSupervisor,
                StateMapping: map[string]string{
                    "starting_dfc":  "running",
                    "idle":          "running",
                    "active":        "running",
                    "stopping":      "stopped",
                },
            },
        },
    }
}

// CollectObservedState - just parent-specific observations
func (w *ProtocolConverterWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // No need to query children - supervisor already ticked them
    return map[string]interface{}{
        "protocol": "modbus",
    }, nil
}
```

### State Machine Using Child States

```go
package protocolconverter

type StartingConnectionState struct{}

func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Simple child state query - NO ERRORS
    connectionState := snapshot.GetChildState("connection")
    
    if connectionState != "up" {
        return s, SignalNone, nil  // Wait for connection
    }
    
    // Connection up, transition to next state
    return &StartingRedpandaState{}, SignalNone, nil
}

type StartingDFCState struct{}

func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Check both DFC children
    dfcReadState := snapshot.GetChildState("dfc_read")
    dfcWriteState := snapshot.GetChildState("dfc_write")
    
    // At least one DFC must be running
    hasRunningDFC := dfcReadState == "running" || dfcWriteState == "running"
    
    if !hasRunningDFC {
        return s, SignalNone, nil  // Wait for DFC
    }
    
    return &IdleState{}, SignalNone, nil
}
```

### What Worker Does NOT Do

❌ **Access child observed state**
```go
// NEVER in Phase 0
benthosMetrics := snapshot.GetChildObserved("dfc_read")
```

❌ **Check child health**
```go
// NEVER in Phase 0
isHealthy := snapshot.IsChildHealthy("connection")
```

❌ **Set child desired state explicitly**
```go
// NEVER in Phase 0 (declarative mapping instead)
snapshot.SetChildDesiredState("connection", "up")
```

❌ **Handle child errors**
```go
// NEVER in Phase 0 (supervisor handles invisibly)
if childError := snapshot.GetChildError("dfc_read"); childError != nil {
    // Handle...
}
```

❌ **Tick children manually**
```go
// NEVER in Phase 0 (supervisor ticks automatically)
w.connectionSupervisor.Tick(ctx)
```

### Total Worker Complexity

**Methods to implement:**
1. `DeclareChildren()` - 30 lines (declarative structure)
2. `CollectObservedState()` - 5 lines (just parent observations)

**State machine changes:**
- Use `snapshot.GetChildState(name)` instead of manual queries
- Remove all child health checks
- Remove all child observed state access

**Complexity vs FSMv1:**
- ✅ **Simpler:** No manual child ticking
- ✅ **Simpler:** No child health logic
- ✅ **Simpler:** No child error handling
- ⚠️ **One new method:** `DeclareChildren()` (but declarative, not imperative)

---

## What's NOT in Phase 0

### Deferred to Future Phases

1. **Sanity Checks / Circuit Breaker** ❌
   - Cross-child consistency checks
   - Automatic child restart on failure
   - Exponential backoff and retry
   - **Why deferred:** User wants to remove sanity checks initially

2. **Async Actions** ❌
   - Background action execution
   - Action queuing
   - HasActionInProgress checks
   - **Why deferred:** Separate concern from child declaration

3. **Health Conditions** ❌
   - `HealthCheck` interface
   - `StateIn()`, `StateNotIn()` predicates
   - `AllOf()`, `AnyOf()` combinators
   - **Why deferred:** Tied to sanity checks (removed)

4. **Time Budget Allocation** ❌
   - `TimeBudgetWeight` field
   - Fair time distribution across children
   - Budget violation tracking
   - **Why deferred:** Optimization, not core API

5. **Dependency Ordering** ❌
   - `DependsOn` field
   - Topological sort of children
   - Startup order coordination
   - **Why deferred:** Can add later without API changes

6. **Dynamic Child Management** ❌
   - Add/remove children at runtime
   - Child lifecycle callbacks
   - Child creation from configuration
   - **Why deferred:** Static declarations sufficient for Phase 0

### What Phase 0 MUST Deliver

✅ **Child Declaration** - `DeclareChildren()` API  
✅ **State Mapping** - Parent state → child desired state  
✅ **Child Tick** - Supervisor ticks children before parent  
✅ **State Query** - `snapshot.GetChildState()` API  
✅ **Clear Boundaries** - Workers never see infrastructure

**Goal:** Prove the basic declarative model works before adding complexity.

---

## Open Questions

### 1. State Mapping Completeness

**Question:** What happens if parent state has no mapping?

**Options:**
- A) Leave child desired state unchanged
- B) Set child to default state (e.g., "stopped")
- C) Log warning

**Recommendation:** **Option A** - Leave unchanged.  
**Rationale:** Some parent states may not care about child state.

---

### 2. Empty Child State

**Question:** What does `snapshot.GetChildState("connection")` return if child hasn't ticked yet?

**Options:**
- A) Empty string `""`
- B) Special value `"uninitialized"`
- C) Panic/error

**Recommendation:** **Option A** - Empty string.  
**Rationale:** Simple, no errors, parent can handle as "child not ready."

---

### 3. Multiple State Mappings

**Question:** Can different parent states map to same child desired state?

**Example:**
```go
StateMapping: map[string]string{
    "idle":   "up",
    "active": "up",  // Both map to "up"
}
```

**Answer:** **Yes, this is valid.**  
**Rationale:** Common pattern - child stays in one state across multiple parent states.

---

### 4. Child State Transition Lag

**Question:** Parent sets child desired state to "running", but child still in "starting". What does parent see?

**Timeline:**
```
T+0: Parent state → "active"
     State mapping sets child desired → "running"
     Child current state → "starting"
     Parent queries: snapshot.GetChildState("dfc") → "starting"

T+1: Parent still in "active"
     Parent queries: snapshot.GetChildState("dfc") → "starting"  (wait)

T+2: Child transitions to "running"
     Parent queries: snapshot.GetChildState("dfc") → "running"  (proceed)
```

**Answer:** Parent sees current child state, waits for desired state to be reached.  
**Rationale:** Natural FSM behavior - states transition over time.

---

### 5. Child Errors Without Sanity Checks

**Question:** If child crashes, how does parent know?

**Without sanity checks:**
```
T+0: Child FSM state = "running"
T+1: Child supervisor crashes (collector timeout, panic, etc.)
T+2: Supervisor detects crash, transitions child to "error" state
T+3: Parent queries: snapshot.GetChildState("child") → "error"
```

**Answer:** Parent sees child in "error" state, reacts according to business logic.  
**Rationale:** Child supervisor handles crash detection, parent sees state transition.

---

### 6. Escape Hatch for Complex Logic

**Question:** What if declarative state mapping insufficient?

**Example:** "Set child to 'running' only if observed temperature < 100"

**Options:**
- A) Add conditional state mapping (complex)
- B) Provide `snapshot.SetChildDesiredState()` override (hybrid)
- C) Tell user to add intermediate parent states (better FSM design)

**Recommendation for Phase 0:** **Option C** - Encourage better FSM design.  
**Revisit later:** If many users hit this, add escape hatch in future phase.

---

## Conclusion

### Phase 0 Scope Summary

**What Phase 0 delivers:**
1. ✅ `DeclareChildren()` API - Workers declare child structure
2. ✅ Declarative state mapping - Parent state → child desired state
3. ✅ Supervisor ticks children first - Automatic coordination
4. ✅ `snapshot.GetChildState()` - Simple child state query
5. ✅ Clean separation - Workers never see infrastructure

**What Phase 0 does NOT include:**
- ❌ Sanity checks (user: "not a fan")
- ❌ Circuit breaker pattern
- ❌ Async actions
- ❌ Health conditions
- ❌ Time budget allocation
- ❌ Dependency ordering

### Validation Criteria

**Phase 0 succeeds if:**
1. Workers can declare children in <50 LOC
2. State machine logic simpler than FSMv1 (no manual child queries)
3. Zero infrastructure code in workers
4. All tests pass (child tick, state mapping, state query)

### Next Steps

1. **Implement Phase 0** (Steps 1-5)
2. **Migrate ProtocolConverter** to prove pattern works
3. **Evaluate:** Do we need sanity checks?
4. **If yes:** Add circuit breaker in Phase 1
5. **If no:** Phase 0 is sufficient, proceed to other features

**Key Principle:** Start minimal, add complexity only if proven necessary.

---

**End of Phase 0 Proposal**
