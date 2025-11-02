# FSMv2 Child State Observation: Design Alternatives

**Created:** 2025-11-02
**Status:** Design Exploration
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition
**Related:** `fsmv2-supervisor-composition-declarative.md`

---

## Executive Summary

This document explores **5 alternative approaches** for how parent workers can observe child state in FSMv2's hierarchical composition pattern. The goal is to enable parent state machines to make decisions based on child health and state while maintaining a clean separation between **infrastructure concerns** (child availability, health) and **business logic** (state-specific behaviors).

**Key Design Principle:**
> "Infrastructure failures can only happen in infrastructure level and we should not mix up infrastructure and business logic" - User insight

**Recommended Approach:** **Alternative 4: Direct TriangularStore Queries with Helper Methods**
Provides clean separation, no boilerplate, and leverages existing infrastructure.

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [User Insights](#user-insights)
3. [Alternative 1: Snapshot-Based Child State Injection](#alternative-1-snapshot-based-child-state-injection)
4. [Alternative 2: Worker Health API](#alternative-2-worker-health-api)
5. [Alternative 3: Inverted Model - Child Signals Parent](#alternative-3-inverted-model---child-signals-parent)
6. [Alternative 4: Direct TriangularStore Queries with Helper Methods](#alternative-4-direct-triangularstore-queries-with-helper-methods)
7. [Alternative 5: State-Level Child State Tracking](#alternative-5-state-level-child-state-tracking)
8. [Creativity Techniques Applied](#creativity-techniques-applied)
9. [Comparison Matrix](#comparison-matrix)
10. [Recommendation](#recommendation)
11. [ProtocolConverter Use Case Examples](#protocolconverter-use-case-examples)

---

## Problem Statement

In FSMv2's hierarchical composition, parent workers need to observe child state to make transition decisions:

```go
// Parent needs to know:
// 1. Is connection child healthy? (infrastructure concern)
// 2. What state is connection in? ("up", "degraded") (business logic)
// 3. Is DFC child ready to start? (depends on connection health)

func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    // How does parent observe child state?
    // Previous proposal: WhenChildError() API - REJECTED as confusing

    // User insight: "maybe we can actually put it into the states,
    // similar to shutdown requested. something like areChildrenHealthy()"
}
```

**Requirements:**

1. **Separate infrastructure from business logic**
   - Infrastructure: "Is child supervisor responding? Is collector working?"
   - Business logic: "Is connection in 'up' state? Is DFC healthy?"

2. **Keep workers simple**
   - No boilerplate for child queries
   - Natural, readable code in state machines

3. **Leverage existing state**
   - User insight: "wouldnt it be theoretically enough to always just use the latest state of the children? so that parent only has to look at it?"

4. **Infrastructure handles infrastructure problems**
   - User insight: "what is the parent to do anyway if there is an infrastructure issue underneath it, it can not do anything right?"

---

## User Insights

### 1. "Put it into the states, similar to shutdown requested"

**Interpretation:** Child state should be accessible like `snapshot.Desired.ShutdownRequested()` - a simple boolean or query method embedded in the snapshot.

**Key insight:** Snapshot is the information source, not a separate API.

### 2. "Use the latest state of the children"

**Interpretation:** Parent doesn't need real-time queries or event streams. Reading the most recent persisted child state is sufficient.

**Key insight:** TriangularStore already has this data. No custom propagation needed.

### 3. "What is the parent to do anyway if there is an infrastructure issue?"

**Interpretation:** Parent state machines operate at business logic level. Infrastructure failures (collector timeouts, supervisor crashes) are handled by the infrastructure layer (supervisors themselves), not by parent business logic.

**Key insight:** Separation of concerns - infrastructure handles infrastructure, business logic handles business logic.

### 4. "Failures can only happen in infrastructure level"

**Interpretation:** Business-level state machines don't handle collector restarts, timeout logic, or health monitoring. That's supervisor responsibility.

**Key insight:** Parent states ask "Is child in state X?" not "Is child's collector responding?"

---

## Alternative 1: Snapshot-Based Child State Injection

### Concept

Supervisor assembles child states from TriangularStore and injects them into `snapshot.Children` map before calling `state.Next()`.

### Design

```go
// Enhanced Snapshot with child state map
type Snapshot struct {
    Identity Identity
    Observed interface{}
    Desired  interface{}

    // NEW: Child state map (optional, only for composite workers)
    Children map[string]ChildState  // childName → child state
}

// ChildState contains everything parent needs to know about a child
type ChildState struct {
    FSMState       string        // Current state name ("up", "running", "stopped")
    ObservedState  interface{}   // Latest observed state (from TriangularStore)
    DesiredState   interface{}   // Latest desired state (from TriangularStore)
    Healthy        bool          // Computed from supervisor's health check
    LastError      error         // Most recent error from child supervisor
    CollectorAge   time.Duration // Age of observed data (infrastructure health indicator)
}
```

### Implementation

```go
// Supervisor extension for composite workers
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... existing code to load parent snapshot ...

    // NEW: Load child states if this is a composite worker
    snapshot.Children = s.loadChildStates(ctx, workerID)

    // Call state.Next() with enriched snapshot
    nextState, signal, action := currentState.Next(*snapshot)
    // ...
}

func (s *Supervisor) loadChildStates(ctx context.Context, parentWorkerID string) map[string]ChildState {
    childMap := make(map[string]ChildState)

    // For each declared child
    for _, childSpec := range s.getChildDeclarations(parentWorkerID) {
        // Query child state from TriangularStore
        childSnapshot, err := s.childStore.LoadSnapshot(ctx, childSpec.Type, childSpec.ID)
        if err != nil {
            // Infrastructure error - provide stale marker
            childMap[childSpec.Name] = ChildState{
                FSMState: "unknown",
                Healthy:  false,
                LastError: err,
            }
            continue
        }

        // Evaluate health condition
        healthy := childSpec.HealthCheck.IsHealthy(childSnapshot)

        childMap[childSpec.Name] = ChildState{
            FSMState:      childSpec.Supervisor.GetCurrentState(),
            ObservedState: childSnapshot.Observed,
            DesiredState:  childSnapshot.Desired,
            Healthy:       healthy,
            LastError:     nil,
            CollectorAge:  time.Since(childSnapshot.Observed.GetTimestamp()),
        }
    }

    return childMap
}
```

### Usage in State Machines

```go
// ProtocolConverter: StartingConnectionState
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Check shutdown first
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Child state access is natural and readable
    connectionChild, exists := snapshot.Children["connection"]
    if !exists {
        // Child not declared - programming error, panic
        panic("child 'connection' not found in snapshot")
    }

    // Infrastructure check (automatically handled)
    if !connectionChild.Healthy {
        // Wait for infrastructure to resolve
        return s, SignalNone, nil
    }

    // Business logic check
    if connectionChild.FSMState != "up" {
        // Still starting, wait
        return s, SignalNone, nil
    }

    // Connection is up, proceed to next phase
    return &StartingRedpandaState{}, SignalNone, nil
}
```

### Pros

- **Clean API**: Child state embedded in snapshot, no extra APIs to learn
- **Infrastructure separation**: `Healthy` field abstracts infrastructure concerns
- **No boilerplate**: Parent states just read `snapshot.Children["name"]`
- **Consistent with existing pattern**: Similar to `snapshot.Desired`, `snapshot.Observed`
- **Automatic assembly**: Supervisor handles all child state loading

### Cons

- **Snapshot bloat**: Every tick loads all child states even if not needed
- **Stale data risk**: Child state read at parent tick time, might be slightly behind
- **Type safety**: `interface{}` for ObservedState/DesiredState requires type assertions
- **Complexity in supervisor**: More logic in supervisor's tick loop

### Infrastructure vs Business Logic Separation

**Infrastructure concerns (handled by supervisor):**
- Loading child snapshots from TriangularStore
- Computing `Healthy` flag based on `HealthCheck` condition
- Populating `CollectorAge` for debugging
- Handling missing children (panic or error)

**Business logic concerns (handled by parent state):**
- Checking `FSMState` value ("up", "running", etc.)
- Reading `ObservedState`/`DesiredState` for specific fields
- Making transition decisions based on child state

**Verdict:** ✅ Clean separation achieved via `Healthy` flag and `FSMState` abstraction.

---

## Alternative 2: Worker Health API

### Concept

Parent workers get a `ChildHealthChecker` dependency that provides methods like `IsChildHealthy(name)`, `GetChildState(name)`.

### Design

```go
// ChildHealthChecker is injected into parent workers
type ChildHealthChecker interface {
    // IsChildHealthy checks if child meets its health condition (infrastructure)
    IsChildHealthy(childName string) bool

    // GetChildState returns current FSM state name (business logic)
    GetChildState(childName string) string

    // GetChildObserved returns latest observed state (business logic)
    GetChildObserved(childName string) (interface{}, error)

    // GetChildDesired returns latest desired state (business logic)
    GetChildDesired(childName string) (interface{}, error)
}

// BaseWorker provides child health checking for composite workers
type BaseWorker struct {
    childHealthChecker ChildHealthChecker  // Injected by supervisor
}

func (b *BaseWorker) AreChildrenHealthy() bool {
    // Check all declared children
    for _, childName := range b.declaredChildren {
        if !b.childHealthChecker.IsChildHealthy(childName) {
            return false
        }
    }
    return true
}
```

### Implementation

```go
// Supervisor implementation of ChildHealthChecker
type supervisorHealthChecker struct {
    supervisor *Supervisor
    parentWorkerID string
}

func (s *supervisorHealthChecker) IsChildHealthy(childName string) bool {
    childCtx, exists := s.supervisor.childSupervisors[childName]
    if !exists {
        return false
    }

    // Query child supervisor's latest health check result
    return childCtx.LastHealthCheck
}

func (s *supervisorHealthChecker) GetChildState(childName string) string {
    childCtx, exists := s.supervisor.childSupervisors[childName]
    if !exists {
        return "unknown"
    }

    return childCtx.Supervisor.GetCurrentState()
}

func (s *supervisorHealthChecker) GetChildObserved(childName string) (interface{}, error) {
    // Query TriangularStore for latest observed state
    return s.supervisor.store.LoadObserved(ctx, childCtx.Type, childCtx.ID)
}
```

### Usage in State Machines

```go
// ProtocolConverterWorker has ChildHealthChecker injected
type ProtocolConverterWorker struct {
    BaseWorker  // Provides AreChildrenHealthy()

    connectionSupervisor *Supervisor
    dfcReadSupervisor    *Supervisor
    dfcWriteSupervisor   *Supervisor
}

// State accesses checker via snapshot (needs enhancement)
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Problem: How does state access ChildHealthChecker?
    // Option A: Add to snapshot (bloat)
    // Option B: Add to DesiredState (awkward)
    // Option C: State embeds worker reference (tight coupling)

    // This approach has ergonomics issues for state machines
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Awkward access pattern
    worker := snapshot.GetWorker().(ProtocolConverterWorker)
    if !worker.childHealthChecker.IsChildHealthy("connection") {
        return s, SignalNone, nil
    }

    connectionState := worker.childHealthChecker.GetChildState("connection")
    if connectionState != "up" {
        return s, SignalNone, nil
    }

    return &StartingRedpandaState{}, SignalNone, nil
}
```

### Pros

- **Explicit API**: Clear methods for health and state queries
- **Flexible**: Can add more query methods as needed
- **Infrastructure encapsulation**: Health logic hidden behind interface

### Cons

- **Ergonomics problem**: States don't naturally have access to worker dependencies
- **Snapshot pollution**: Need to add worker reference or checker to snapshot
- **Tight coupling**: States now depend on worker type to access checker
- **More boilerplate**: Every state needs to extract worker and call checker methods
- **API surface**: Another interface to learn and maintain

### Infrastructure vs Business Logic Separation

**Infrastructure concerns:**
- `IsChildHealthy()` implementation (reads health check results)
- Handling missing children (returns false or error)

**Business logic concerns:**
- `GetChildState()` - returns FSM state name
- `GetChildObserved()` - returns observed state
- Parent state interprets these values

**Verdict:** ⚠️ Separation achieved but ergonomics suffer. States need indirect access via snapshot.

---

## Alternative 3: Inverted Model - Child Signals Parent

### Concept

**Inversion creativity technique:** Instead of parent querying child, child notifies parent when state changes.

### Design

```go
// Child supervisor notifies parent when state changes
type ParentNotifier interface {
    OnChildStateChanged(childName string, newState string)
    OnChildHealthChanged(childName string, healthy bool)
}

// Parent worker implements ParentNotifier
type ProtocolConverterWorker struct {
    childStateCache map[string]string  // childName → current state
    childHealthCache map[string]bool   // childName → healthy flag
    mu sync.RWMutex
}

func (w *ProtocolConverterWorker) OnChildStateChanged(childName string, newState string) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.childStateCache[childName] = newState
}

func (w *ProtocolConverterWorker) OnChildHealthChanged(childName string, healthy bool) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.childHealthCache[childName] = healthy
}

// Child supervisor calls parent notifier on state transitions
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... existing tick logic ...

    // After state transition
    if nextState != currentState {
        // Notify parent if this supervisor is a child
        if s.parentNotifier != nil {
            s.parentNotifier.OnChildStateChanged(s.childName, nextState.String())
        }
    }

    // After health check
    newHealthy := s.checkHealth()
    if newHealthy != s.lastHealthy {
        if s.parentNotifier != nil {
            s.parentNotifier.OnChildHealthChanged(s.childName, newHealthy)
        }
    }
}
```

### Usage in State Machines

```go
// States read cached child state (simple and fast)
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Access cached child state (no querying)
    worker := snapshot.GetWorker().(ProtocolConverterWorker)

    worker.mu.RLock()
    connectionState := worker.childStateCache["connection"]
    connectionHealthy := worker.childHealthCache["connection"]
    worker.mu.RUnlock()

    if !connectionHealthy {
        return s, SignalNone, nil
    }

    if connectionState != "up" {
        return s, SignalNone, nil
    }

    return &StartingRedpandaState{}, SignalNone, nil
}
```

### Pros

- **No polling overhead**: Parent only updates cache when child changes
- **Real-time propagation**: Parent sees state changes immediately
- **Decoupled storage**: No need to query TriangularStore for child state

### Cons

- **Complexity**: New notification subsystem (callbacks, threading)
- **Race conditions**: Parent tick might read cache mid-update
- **Cache coherency**: If notifications fail, cache becomes stale
- **Tight coupling**: Child supervisor now depends on parent interface
- **Debugging difficulty**: State changes propagate through callbacks, harder to trace
- **Not TriangularStore-aligned**: Duplicates state already in database

### Infrastructure vs Business Logic Separation

**Infrastructure concerns:**
- Notification delivery mechanism
- Cache management and synchronization
- Handling notification failures

**Business logic concerns:**
- Reading cached state values
- Interpreting state names

**Verdict:** ❌ Introduces infrastructure complexity (callbacks, caching) that violates user's insight: "just use the latest state of the children" (from TriangularStore).

---

## Alternative 4: Direct TriangularStore Queries with Helper Methods

### Concept

**Simplification creativity technique:** Eliminate abstraction layers. Parent states query TriangularStore directly using simple helper methods.

### Design

```go
// Add helper methods to Supervisor for child queries
type Supervisor struct {
    // ... existing fields ...

    childDeclarations map[string]ChildSpec  // Child specs from worker's DeclareChildren()
}

// GetChildState reads child FSM state from its supervisor
func (s *Supervisor) GetChildState(ctx context.Context, childName string) (string, error) {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return "", fmt.Errorf("child %s not declared", childName)
    }

    // Query child supervisor directly
    return childSpec.Supervisor.GetCurrentState(), nil
}

// IsChildHealthy evaluates child's health condition
func (s *Supervisor) IsChildHealthy(ctx context.Context, childName string) bool {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return false
    }

    // Load child snapshot from TriangularStore
    childSnapshot, err := s.store.LoadSnapshot(ctx, childSpec.WorkerType, childSpec.ID)
    if err != nil {
        return false  // Infrastructure error = unhealthy
    }

    // Evaluate health condition
    return childSpec.HealthCheck.IsHealthy(childSnapshot)
}

// GetChildObserved reads child's observed state from TriangularStore
func (s *Supervisor) GetChildObserved(ctx context.Context, childName string) (interface{}, error) {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return nil, fmt.Errorf("child %s not declared", childName)
    }

    return s.store.LoadObserved(ctx, childSpec.WorkerType, childSpec.ID)
}

// Enhance Snapshot to include supervisor reference
type Snapshot struct {
    Identity   Identity
    Observed   interface{}
    Desired    interface{}

    // NEW: Supervisor reference for child queries (optional)
    supervisor *Supervisor  // Only set for composite workers
}
```

### Implementation

```go
// Supervisor embeds supervisor reference in snapshot for composite workers
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... load parent snapshot ...

    // Add supervisor reference for child queries
    snapshot.supervisor = s

    // Call state.Next() with enhanced snapshot
    nextState, signal, action := currentState.Next(*snapshot)
    // ...
}
```

### Usage in State Machines

```go
// States query children directly via supervisor reference
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Direct query via supervisor helpers (clean and simple)
    ctx := context.Background()  // TODO: Pass ctx through snapshot?

    // Infrastructure check
    if !snapshot.supervisor.IsChildHealthy(ctx, "connection") {
        return s, SignalNone, nil  // Wait for infrastructure
    }

    // Business logic check
    connectionState, err := snapshot.supervisor.GetChildState(ctx, "connection")
    if err != nil || connectionState != "up" {
        return s, SignalNone, nil
    }

    return &StartingRedpandaState{}, SignalNone, nil
}

// Advanced: Query child observed state for detailed checks
func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    ctx := context.Background()

    // Query child observed state
    connectionObserved, err := snapshot.supervisor.GetChildObserved(ctx, "connection")
    if err != nil {
        return s, SignalNone, nil
    }

    // Type assert to specific observed state
    connState, ok := connectionObserved.(ConnectionObservedState)
    if !ok {
        return s, SignalNone, nil
    }

    // Business logic: Check connection quality
    if connState.Latency > 100*time.Millisecond {
        return &DegradedConnectionState{}, SignalNone, nil
    }

    return &IdleState{}, SignalNone, nil
}
```

### Pros

- **Simplicity**: No new abstractions, just direct queries
- **Flexibility**: States query exactly what they need, when they need it
- **No boilerplate**: Helper methods are one-liners
- **TriangularStore-aligned**: Uses existing persisted state (user's insight)
- **Infrastructure separation**: `IsChildHealthy()` abstracts infrastructure concerns
- **On-demand loading**: Only load child state when actually needed

### Cons

- **Context propagation**: Need to pass `context.Context` through snapshot (design decision)
- **Query overhead**: Each `GetChildState()` call queries supervisor (but O(1) lookup)
- **No type safety**: `GetChildObserved()` returns `interface{}`, requires type assertion
- **Snapshot mutation**: Adding supervisor reference changes snapshot struct

### Infrastructure vs Business Logic Separation

**Infrastructure concerns (supervisor helpers):**
- `IsChildHealthy()` - evaluates health conditions, handles errors
- Loading snapshots from TriangularStore
- Handling missing children (returns error)

**Business logic concerns (parent states):**
- `GetChildState()` - returns FSM state name for logic
- `GetChildObserved()` - returns observed state for detailed checks
- Interpreting state values and making transitions

**Verdict:** ✅ **Clean separation**. Infrastructure logic encapsulated in supervisor helpers. Business logic stays in states.

### Context Propagation Options

**Option A: Add context to Snapshot**
```go
type Snapshot struct {
    Identity   Identity
    Observed   interface{}
    Desired    interface{}
    supervisor *Supervisor
    ctx        context.Context  // NEW
}

// Usage in states
if !snapshot.supervisor.IsChildHealthy(snapshot.ctx, "connection") { ... }
```

**Option B: Use background context**
```go
// States use context.Background() for queries
ctx := context.Background()
if !snapshot.supervisor.IsChildHealthy(ctx, "connection") { ... }
```

**Option C: Methods without context**
```go
// Supervisor helper creates context internally
func (s *Supervisor) IsChildHealthy(childName string) bool {
    ctx := context.Background()
    // ... query with ctx ...
}

// Usage in states (simplest)
if !snapshot.supervisor.IsChildHealthy("connection") { ... }
```

**Recommendation:** Option C for MVP (simplest). Context propagation can be added later if needed.

---

## Alternative 5: State-Level Child State Tracking

### Concept

**Collision-zone creativity technique:** What if states themselves tracked child state, like React components track props?

### Design

```go
// Enhanced State interface with child state awareness
type State interface {
    Next(snapshot Snapshot) (State, Signal, Action)
    String() string
    Reason() string

    // NEW: States can declare which child states they care about
    DeclareChildDependencies() []string  // Returns child names
}

// Parent state tracks child state in its fields (like React component state)
type StartingConnectionState struct {
    lastConnectionState  string  // Cached from previous tick
    lastConnectionHealthy bool   // Cached from previous tick
}

// Supervisor passes previous state's child state cache to next state
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... load snapshot ...

    // Supervisor assembles child state based on state's declared dependencies
    childStates := s.loadChildStatesForDependencies(ctx, currentState.DeclareChildDependencies())

    // Pass child states to Next()
    nextState, signal, action := currentState.Next(snapshot, childStates)

    // Transfer child state cache to next state if it's the same type
    if reflect.TypeOf(nextState) == reflect.TypeOf(currentState) {
        nextState.InheritChildStateCache(currentState)
    }

    // ...
}
```

### Implementation

```go
// State declares dependencies
func (s *StartingConnectionState) DeclareChildDependencies() []string {
    return []string{"connection"}  // Only load connection state, not DFC
}

// State accesses child state via passed-in map
func (s *StartingConnectionState) Next(snapshot Snapshot, childStates map[string]ChildState) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    connectionChild := childStates["connection"]

    if !connectionChild.Healthy {
        return s, SignalNone, nil
    }

    if connectionChild.FSMState != "up" {
        // Cache state for next tick
        s.lastConnectionState = connectionChild.FSMState
        return s, SignalNone, nil
    }

    return &StartingRedpandaState{}, SignalNone, nil
}
```

### Pros

- **Declarative dependencies**: States explicitly declare what they need
- **Efficient loading**: Only load child states that state actually uses
- **State-local caching**: States can track child state changes across ticks

### Cons

- **State mutation**: States now have mutable fields (violates pure function principle)
- **Complexity**: State inheritance, cache transfer logic
- **Breaks immutability**: States were designed to be stateless
- **API bloat**: Another method to implement (`DeclareChildDependencies`)
- **Not FSMv2-aligned**: States should be pure functions, not stateful objects

### Infrastructure vs Business Logic Separation

**Infrastructure concerns:**
- Loading child states based on dependencies
- Transferring state cache between state instances

**Business logic concerns:**
- Declaring dependencies
- Interpreting child state values

**Verdict:** ❌ Violates FSMv2 design principle of stateless states. Adds complexity without clear benefit over Alternative 4.

---

## Creativity Techniques Applied

### 1. Inversion (Alternative 3)

**Technique:** "What if children notify parent instead of parent querying children?"

**Result:** Inverted Model with callbacks and caches. Interesting but introduces complexity (callbacks, race conditions, cache coherency). **Not recommended** because it violates user's insight to "just use the latest state of the children" from TriangularStore.

### 2. Collision-Zone (Alternative 5)

**Technique:** "What if states passed child state like React components pass props?"

**Result:** State-level child state tracking with declarative dependencies. Interesting but violates FSMv2's stateless state principle. **Not recommended** because states should remain pure functions.

### 3. Simplification (Alternative 4)

**Technique:** "Can we eliminate the abstraction layer entirely?"

**Result:** Direct TriangularStore queries via supervisor helper methods. **Recommended** because it's simple, leverages existing infrastructure, and aligns with user insights.

### 4. Analogy: Snapshot is a "View" (Alternative 1)

**Technique:** "Snapshot is like a database view - pre-joined data for easy access"

**Result:** Snapshot-based child state injection. Works well but has performance concern (loads all children on every tick). **Viable alternative** to Alternative 4.

---

## Comparison Matrix

| Criteria | Alt 1: Snapshot Injection | Alt 2: Health API | Alt 3: Inverted (Signals) | Alt 4: Direct Queries | Alt 5: State Tracking |
|----------|---------------------------|-------------------|--------------------------|----------------------|-----------------------|
| **Infrastructure Separation** | ✅ Clean (`Healthy` flag) | ⚠️ Via interface | ❌ Callback complexity | ✅ Clean (helper methods) | ⚠️ Supervisor manages cache |
| **Business Logic Simplicity** | ✅ `snapshot.Children["x"]` | ❌ Indirect access | ❌ Lock + cache access | ✅ `supervisor.GetChildState()` | ⚠️ State fields + cache |
| **No Boilerplate** | ✅ Zero boilerplate | ❌ API calls in every state | ❌ Cache management | ✅ One-liner calls | ❌ Dependency declaration |
| **TriangularStore-Aligned** | ✅ Reads from store | ✅ Reads from store | ❌ Duplicate cache | ✅ Reads from store | ⚠️ Loads on-demand |
| **Performance** | ⚠️ Loads all children every tick | ✅ On-demand queries | ✅ No queries (cached) | ✅ On-demand queries | ✅ On-demand queries |
| **Complexity** | ⚠️ Supervisor enrichment logic | ⚠️ New interface + impl | ❌ Callbacks + threading | ✅ Simple helpers | ❌ State cache management |
| **FSMv2 Principles** | ✅ Snapshot-centric | ⚠️ Extra API surface | ❌ Stateful notifications | ✅ Minimal changes | ❌ Stateful states |
| **Ergonomics** | ✅ Natural map access | ❌ Awkward via snapshot | ❌ Lock + cache | ✅ Natural method calls | ⚠️ Fields + inheritance |
| **Type Safety** | ❌ `interface{}` types | ❌ `interface{}` types | ❌ `interface{}` types | ❌ `interface{}` types | ❌ `interface{}` types |
| **Debugging** | ✅ Snapshot visible in logs | ⚠️ Hidden in interface | ❌ Callback traces | ✅ Query calls visible | ⚠️ State cache opaque |
| **Context Propagation** | ✅ Not needed | ⚠️ Via snapshot | ✅ Not needed | ⚠️ Needs ctx in snapshot or background | ⚠️ Via Next() params |

### Scoring

**Alternative 4 (Direct Queries): 8/10**
- ✅ Best infrastructure/business separation
- ✅ Simplest implementation
- ✅ Aligns with user insights
- ⚠️ Minor: Context propagation decision

**Alternative 1 (Snapshot Injection): 7/10**
- ✅ Clean API, natural access
- ⚠️ Performance concern (loads all children)
- ⚠️ Supervisor enrichment complexity

**Alternative 2 (Health API): 4/10**
- ⚠️ Ergonomics issues (indirect access)
- ❌ More API surface

**Alternative 3 (Inverted Model): 3/10**
- ❌ High complexity (callbacks, caching, races)
- ❌ Not TriangularStore-aligned

**Alternative 5 (State Tracking): 3/10**
- ❌ Violates stateless state principle
- ❌ Cache management complexity

---

## Recommendation

### Primary Recommendation: **Alternative 4 - Direct TriangularStore Queries with Helper Methods**

**Rationale:**

1. **Aligns with user insights**
   - "just use the latest state of the children" ✅ Queries TriangularStore
   - "put it into the states" ✅ States access via snapshot.supervisor
   - "infrastructure handles infrastructure" ✅ `IsChildHealthy()` abstracts infrastructure

2. **Simplicity**
   - No new abstractions (uses existing Supervisor + TriangularStore)
   - Helper methods are one-liners
   - States query exactly what they need

3. **Infrastructure separation**
   - `IsChildHealthy()` = infrastructure (health checks, error handling)
   - `GetChildState()`, `GetChildObserved()` = business logic (state interpretation)

4. **Performance**
   - On-demand loading (only query children when needed)
   - No unnecessary snapshot bloat

5. **Ergonomics**
   - Natural method calls: `supervisor.GetChildState("connection")`
   - No boilerplate, no API proliferation

### Implementation Plan

**Phase 1: Add Helper Methods to Supervisor (Week 1)**

```go
// pkg/fsmv2/supervisor/supervisor.go

// GetChildState returns the current FSM state name for a child worker.
// Returns error if child not declared.
func (s *Supervisor) GetChildState(childName string) (string, error) {
    s.mu.RLock()
    childSpec, exists := s.childDeclarations[childName]
    s.mu.RUnlock()

    if !exists {
        return "", fmt.Errorf("child %s not declared", childName)
    }

    return childSpec.Supervisor.GetCurrentState(), nil
}

// IsChildHealthy evaluates the child's health condition.
// Returns false if child not declared or health check fails.
func (s *Supervisor) IsChildHealthy(childName string) bool {
    s.mu.RLock()
    childSpec, exists := s.childDeclarations[childName]
    s.mu.RUnlock()

    if !exists {
        return false
    }

    ctx := context.Background()
    childSnapshot, err := s.store.LoadSnapshot(ctx, childSpec.WorkerType, childSpec.ID)
    if err != nil {
        s.logger.Warnf("Failed to load child %s snapshot for health check: %v", childName, err)
        return false
    }

    return childSpec.HealthCheck.IsHealthy(childSnapshot)
}

// GetChildObserved returns the child's observed state from TriangularStore.
func (s *Supervisor) GetChildObserved(childName string) (interface{}, error) {
    s.mu.RLock()
    childSpec, exists := s.childDeclarations[childName]
    s.mu.RUnlock()

    if !exists {
        return nil, fmt.Errorf("child %s not declared", childName)
    }

    ctx := context.Background()
    return s.store.LoadObserved(ctx, childSpec.WorkerType, childSpec.ID)
}

// GetChildDesired returns the child's desired state from TriangularStore.
func (s *Supervisor) GetChildDesired(childName string) (interface{}, error) {
    s.mu.RLock()
    childSpec, exists := s.childDeclarations[childName]
    s.mu.RUnlock()

    if !exists {
        return nil, fmt.Errorf("child %s not declared", childName)
    }

    ctx := context.Background()
    return s.store.LoadDesired(ctx, childSpec.WorkerType, childSpec.ID)
}
```

**Phase 2: Enhance Snapshot (Week 2)**

```go
// pkg/fsmv2/worker.go

type Snapshot struct {
    Identity   Identity
    Observed   interface{}
    Desired    interface{}

    // Supervisor reference for child queries (only set for composite workers)
    // States use this to call GetChildState(), IsChildHealthy(), etc.
    supervisor *Supervisor
}
```

**Phase 3: Update Supervisor Tick Loop (Week 2)**

```go
// pkg/fsmv2/supervisor/supervisor.go

func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... load snapshot ...

    // Add supervisor reference if this is a composite worker
    if len(s.childDeclarations) > 0 {
        snapshot.supervisor = s
    }

    // Call state.Next() with enhanced snapshot
    nextState, signal, action := currentState.Next(*snapshot)
    // ...
}
```

**Phase 4: ProtocolConverter Migration (Week 3)**

Use helper methods in ProtocolConverter states as shown in examples above.

**Phase 5: Testing (Week 4)**

- Unit tests for helper methods
- Integration tests with ProtocolConverter
- Performance benchmarks (query overhead)

### Backup Recommendation: **Alternative 1 - Snapshot Injection**

If performance testing shows that loading all children on every tick is acceptable, Alternative 1 provides even cleaner ergonomics:

```go
// States just read from snapshot.Children (no method calls)
connectionChild := snapshot.Children["connection"]
if !connectionChild.Healthy { ... }
if connectionChild.FSMState != "up" { ... }
```

**Trade-off:** Supervisor complexity (enrichment logic) vs state simplicity (map access).

---

## ProtocolConverter Use Case Examples

### Example 1: StartingConnectionState

**Scenario:** Parent waits for connection child to reach "up" state before proceeding.

**Alternative 4 Implementation:**

```go
type StartingConnectionState struct{}

func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Infrastructure check (handled by supervisor)
    if !snapshot.supervisor.IsChildHealthy("connection") {
        // Connection child unavailable, wait
        return s, SignalNone, nil
    }

    // Business logic check
    connectionState, err := snapshot.supervisor.GetChildState("connection")
    if err != nil {
        // Child not declared - programming error
        panic(fmt.Sprintf("child 'connection' not found: %v", err))
    }

    if connectionState != "up" {
        // Connection still starting, wait
        return s, SignalNone, nil
    }

    // Connection is up, proceed to next phase
    return &StartingRedpandaState{}, SignalNone, nil
}

func (s *StartingConnectionState) String() string {
    return "starting_connection"
}

func (s *StartingConnectionState) Reason() string {
    return "waiting for connection child to reach 'up' state"
}
```

**Alternative 1 Implementation (Snapshot Injection):**

```go
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Child state pre-loaded in snapshot
    connectionChild, exists := snapshot.Children["connection"]
    if !exists {
        panic("child 'connection' not found in snapshot")
    }

    // Infrastructure check
    if !connectionChild.Healthy {
        return s, SignalNone, nil
    }

    // Business logic check
    if connectionChild.FSMState != "up" {
        return s, SignalNone, nil
    }

    return &StartingRedpandaState{}, SignalNone, nil
}
```

**Comparison:** Alternative 1 is slightly more concise (map access vs method call), but Alternative 4 is more explicit about what's being queried.

---

### Example 2: ActiveState with Detailed Child Observation

**Scenario:** Parent checks connection quality from child's observed state to detect degradation.

**Alternative 4 Implementation:**

```go
type ActiveState struct{}

func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Infrastructure check
    if !snapshot.supervisor.IsChildHealthy("connection") {
        return &DegradedConnectionState{reason: "connection unhealthy"}, SignalNone, nil
    }

    // Query detailed child observed state for business logic
    connectionObserved, err := snapshot.supervisor.GetChildObserved("connection")
    if err != nil {
        return &DegradedConnectionState{reason: "failed to query connection state"}, SignalNone, nil
    }

    // Type assert to specific observed state
    connState, ok := connectionObserved.(ConnectionObservedState)
    if !ok {
        return &DegradedConnectionState{reason: "unexpected connection state type"}, SignalNone, nil
    }

    // Business logic: Check connection quality metrics
    if connState.Latency > 100*time.Millisecond {
        return &DegradedConnectionState{reason: "high latency"}, SignalNone, nil
    }

    if connState.PacketLoss > 0.01 {  // >1% packet loss
        return &DegradedConnectionState{reason: "packet loss"}, SignalNone, nil
    }

    // All children healthy and performing well
    return s, SignalNone, nil
}

func (s *ActiveState) String() string {
    return "active"
}

func (s *ActiveState) Reason() string {
    return "all children healthy and operating normally"
}
```

**Alternative 1 Implementation:**

```go
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    connectionChild := snapshot.Children["connection"]

    if !connectionChild.Healthy {
        return &DegradedConnectionState{reason: "connection unhealthy"}, SignalNone, nil
    }

    // Access observed state from pre-loaded child state
    connState, ok := connectionChild.ObservedState.(ConnectionObservedState)
    if !ok {
        return &DegradedConnectionState{reason: "unexpected connection state type"}, SignalNone, nil
    }

    if connState.Latency > 100*time.Millisecond {
        return &DegradedConnectionState{reason: "high latency"}, SignalNone, nil
    }

    if connState.PacketLoss > 0.01 {
        return &DegradedConnectionState{reason: "packet loss"}, SignalNone, nil
    }

    return s, SignalNone, nil
}
```

**Comparison:** Equivalent functionality. Alternative 1 has pre-loaded observed state in snapshot. Alternative 4 queries on-demand.

---

### Example 3: StartingDFCState with Multiple Children

**Scenario:** Parent checks multiple children (DFC read, DFC write) and proceeds when at least one is healthy.

**Alternative 4 Implementation:**

```go
type StartingDFCState struct{}

func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    // Check DFC read health
    dfcReadHealthy := snapshot.supervisor.IsChildHealthy("dfc_read")
    dfcReadState, _ := snapshot.supervisor.GetChildState("dfc_read")

    // Check DFC write health
    dfcWriteHealthy := snapshot.supervisor.IsChildHealthy("dfc_write")
    dfcWriteState, _ := snapshot.supervisor.GetChildState("dfc_write")

    // Business logic: At least one DFC must be healthy
    hasHealthyDFC := dfcReadHealthy || dfcWriteHealthy
    if !hasHealthyDFC {
        return &StartingFailedDFCMissingState{}, SignalNone, nil
    }

    // Business logic: At least one DFC must be running
    dfcReadRunning := dfcReadState == "running"
    dfcWriteRunning := dfcWriteState == "running"

    if dfcReadRunning || dfcWriteRunning {
        return &IdleState{}, SignalNone, nil
    }

    // Still waiting for DFCs to start
    return s, SignalNone, nil
}

func (s *StartingDFCState) String() string {
    return "starting_dfc"
}

func (s *StartingDFCState) Reason() string {
    return "waiting for at least one DFC to reach 'running' state"
}
```

**Alternative 1 Implementation:**

```go
func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &StoppingState{}, SignalNone, nil
    }

    dfcRead := snapshot.Children["dfc_read"]
    dfcWrite := snapshot.Children["dfc_write"]

    hasHealthyDFC := dfcRead.Healthy || dfcWrite.Healthy
    if !hasHealthyDFC {
        return &StartingFailedDFCMissingState{}, SignalNone, nil
    }

    dfcReadRunning := dfcRead.FSMState == "running"
    dfcWriteRunning := dfcWrite.FSMState == "running"

    if dfcReadRunning || dfcWriteRunning {
        return &IdleState{}, SignalNone, nil
    }

    return s, SignalNone, nil
}
```

**Comparison:** Alternative 1 is more concise (fewer lines, direct map access). Alternative 4 is more explicit (method calls show intent).

---

## Conclusion

**Alternative 4 (Direct TriangularStore Queries)** is the recommended approach because:

1. ✅ **Simplest implementation** - leverages existing Supervisor + TriangularStore
2. ✅ **Aligns with user insights** - "just use the latest state of the children"
3. ✅ **Clean infrastructure/business separation** - `IsChildHealthy()` abstracts infrastructure, `GetChildState()` provides business data
4. ✅ **No boilerplate** - one-liner method calls
5. ✅ **On-demand loading** - no unnecessary snapshot bloat

**Alternative 1 (Snapshot Injection)** is a strong backup if:
- Performance testing shows loading all children is acceptable
- Even cleaner ergonomics (map access) is prioritized over explicit method calls

Both alternatives achieve the user's goal of separating infrastructure from business logic while keeping workers simple and avoiding confusion.

---

**Next Steps:**

1. Prototype Alternative 4 helper methods
2. Benchmark query overhead vs snapshot injection
3. Implement ProtocolConverter with chosen approach
4. Validate with team and iterate

**Decision Point:** Choose Alternative 4 for MVP. Switch to Alternative 1 if benchmarks show snapshot injection is faster and enrichment complexity is manageable.
