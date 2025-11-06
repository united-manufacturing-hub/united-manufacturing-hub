# FSMv2: Config Changes and Dynamic Children Analysis

**Date:** 2025-11-02
**Status:** Analysis
**Related Plans:**
- `fsmv2-phase0-worker-child-api.md` - Phase 0 API proposal
- `fsmv2-child-removal-auto.md` - Auto-removal pattern
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Original plan
- `fsmv2-infrastructure-supervision-patterns.md` - Circuit breaker pattern

## Problem Statement

The Phase 0 Worker Child API proposal assumes static child declarations with declarative state mapping. However, real-world scenarios reveal additional requirements:

### Core Questions

1. **When should `DeclareChildren()` be called?**
   - Every tick with smart diffing?
   - Only on config change events?
   - State-aware with current state parameter?

2. **Where should children be modified?**
   - In `DeriveDesiredState()` (config-driven: what children exist)
   - In state transitions (state-driven: what state children should be in)
   - Hybrid approach?

3. **How to handle dynamic N children?**
   - OPC UA browser: One supervisor, N workers (one per server)
   - Servers added/removed dynamically based on config
   - How does this fit with supervisor/worker pattern?

### Real-World Scenarios

#### Scenario 1: Bridge Config Changes

```
Bridge (ProtocolConverter)
├─ Connection (Nmap → S6)
├─ ReadFlow (Benthos → S6)
└─ WriteFlow (Benthos → S6)

Event: User changes IP address in config
Effect: All children need template reapplication
Challenge: How does parent detect and propagate this change?
```

Current Phase 0 API assumes static children with state mapping:
```go
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor, StateMapping: {...}},
            {Name: "read_flow", Supervisor: w.readFlowSupervisor, StateMapping: {...}},
            {Name: "write_flow", Supervisor: w.writeFlowSupervisor, StateMapping: {...}},
        },
    }
}
```

**Problem:** If IP address changes, how do children get updated?
- Do we recreate supervisors with new config?
- Do we call `UpdateConfig()` on existing supervisors?
- How does parent know config changed?

#### Scenario 2: OPC UA Browser

```
OPCUABrowserSupervisor
├─ OPCUABrowserWorker (server 10.0.0.1)
├─ OPCUABrowserWorker (server 10.0.0.2)
└─ OPCUABrowserWorker (server 10.0.0.N)

Event: User adds server 10.0.0.100 to config
Effect: New worker created dynamically
Challenge: N is dynamic, not known at compile time
```

Current Phase 0 API shows fixed-size child array. How to handle:
- Adding server → create new supervisor/worker pair?
- Removing server → auto-remove child (already solved)?
- Iterating over config to generate children array?

### Critical Constraints

From user feedback:
- ❌ No sanity checks - "they mix things up"
- ✅ Infrastructure completely hidden from workers
- ✅ Children detect their own issues in business logic
- ✅ Supervisor handles infrastructure failures invisibly
- ✅ Workers declare intent, supervisor executes

---

## Creativity Techniques Applied

### 1. Inversion Thinking

**Question:** What if we NEVER call `DeclareChildren()` after initial setup?

**Implications:**
- Config changes would require supervisor restart (entire hierarchy)
- Simpler implementation - no diffing needed
- Matches "immutable infrastructure" pattern
- Poor user experience for simple IP change

**Question:** What if we call `DeclareChildren()` EVERY tick with full reconstruction?

**Implications:**
- Supervisor diffs every tick anyway (reconciliation loop pattern)
- Config changes detected automatically
- High CPU for complex hierarchies
- Matches Kubernetes reconciliation pattern exactly

**Question:** What if children weren't supervisors at all, but just worker instances managed by parent?

**Implications:**
- OPC UA browser becomes single worker with N internal goroutines
- No supervisor hierarchy - just business logic composition
- Loses benefits of supervisor pattern (restart, backoff, etc.)
- Might be simpler for some use cases

### 2. Collision-Zone Thinking

Compare FSM composition to established patterns:

#### Git Commits vs Config Changes
```
Git:
1. User edits files (config change)
2. Git diff shows changes (smart diffing)
3. Git commit creates new snapshot (DeclareChildren returns new structure)

FSM:
1. User changes config
2. Supervisor diffs old vs new children (smart diffing)
3. Supervisor applies changes (create/update/remove children)
```
**Insight:** Config is versioned snapshot, DeclareChildren() returns new snapshot

#### React Component Trees
```
React:
<Parent>
  <Child1 prop={ip} />
  <Child2 prop={ip} />
</Parent>

User changes 'ip' state:
- React re-renders Parent
- Parent returns new JSX tree
- React diffs old vs new tree
- React updates only changed children

FSM:
DeclareChildren() called with new config:
- Worker returns new child structure
- Supervisor diffs old vs new
- Supervisor updates only changed children
```
**Insight:** Config is props, DeclareChildren() is render function

#### Kubernetes Reconciliation Loop
```
Kubernetes:
while true:
    desired := spec.Replicas
    current := countPods()

    if desired > current:
        createPods(desired - current)
    elif desired < current:
        deletePods(current - desired)

    sleep(reconcileInterval)

FSM:
while supervisor.tick():
    desired := worker.DeclareChildren()
    current := supervisor.currentChildren

    if childAdded(desired, current):
        supervisor.addChild(...)
    elif childRemoved(desired, current):
        supervisor.removeChild(...)
```
**Insight:** Declarative reconciliation every tick is proven pattern

#### Terraform Plan/Apply
```
Terraform:
1. terraform plan (dry-run diff)
2. Show user what will change
3. terraform apply (user confirms)

FSM Option:
1. DeclareChildren() returns desired (dry-run)
2. Supervisor diffs and logs changes
3. Supervisor applies changes (auto-confirm or manual?)
```
**Insight:** Could add preview mode for destructive changes

### 3. First Principles Analysis

**What's the absolute minimum mechanism needed for config changes?**

Components:
1. **Config Input** - How does worker know config changed?
2. **Change Detection** - How does supervisor know to update children?
3. **Change Application** - How does supervisor apply updates?

**Mechanism 1: Pull-Based (Every Tick)**
```
Config Input: Worker reads config on every tick
Change Detection: Supervisor diffs DeclareChildren() output every tick
Change Application: Supervisor creates/updates/removes children
```
Minimal code: `DeclareChildren()` just returns current config-derived structure

**Mechanism 2: Push-Based (Event-Driven)**
```
Config Input: Config watcher notifies worker of change
Change Detection: Worker calls supervisor.UpdateChildren(new)
Change Application: Supervisor creates/updates/removes children
```
Requires: Config change event system, worker callback registration

**Mechanism 3: Hybrid (State-Aware)**
```
Config Input: Worker detects config version change in DeriveDesiredState()
Change Detection: Worker returns new children in specific states
Change Application: Supervisor creates/updates/removes children
```
Requires: Config versioning, state awareness in DeriveDesiredState()

**First Principles Conclusion:** Pull-based (every tick) is simplest - no events, no versioning, just reconciliation.

### 4. Pattern Analysis from Successful Systems

| System | Config Change Pattern | Timing | Complexity |
|--------|----------------------|--------|------------|
| Kubernetes | Reconciliation loop | Every 10s | Low (declarative) |
| Terraform | Manual plan/apply | On demand | Medium (preview) |
| Docker Compose | Restart on change | On file watch | Low (stateless) |
| systemd | Reload config | On `systemctl reload` | Medium (state preservation) |
| Erlang/OTP | Hot code swap | On release | High (complex) |

**Pattern Insights:**
- **Kubernetes:** Declarative every-tick reconciliation is gold standard
- **Terraform:** Preview before apply prevents surprises
- **Docker Compose:** Stateless restart is simple but loses state
- **systemd:** Config reload preserves running state (like updating supervisor config)
- **Erlang:** Hot swap is complex but zero-downtime

---

## Option Evaluation

### Option 1: Every Tick with Smart Diffing (Kubernetes Pattern)

**Approach:** Call `DeclareChildren()` every tick, supervisor diffs and applies changes.

#### Worker Code
```go
type ProtocolConverterWorker struct {
    config *ProtocolConverterConfig // Updated by supervisor via DeriveDesiredState

    // Child supervisors created lazily
    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor   *supervisor.Supervisor
    writeFlowSupervisor  *supervisor.Supervisor
}

// Called every tick by supervisor
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
    // Create child supervisors with current config if needed
    if w.connectionSupervisor == nil {
        w.connectionSupervisor = supervisor.New(
            newConnectionWorker(w.config.IPAddress),
            w.logger,
        )
    }

    // Return desired children structure
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
            // ...
        },
    }
}

// Config changes propagate here
func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Config changed - recreate child supervisors with new config
    if userSpec.IPAddress != w.config.IPAddress {
        w.connectionSupervisor = supervisor.New(
            newConnectionWorker(userSpec.IPAddress),
            w.logger,
        )
        w.readFlowSupervisor = supervisor.New(
            newReadFlowWorker(userSpec.IPAddress),
            w.logger,
        )
    }

    w.config = userSpec

    return DesiredState{
        State:    "active",
        UserSpec: userSpec,
    }
}
```

#### Supervisor Diffing Logic
```go
func (s *Supervisor) reconcileChildren(ctx context.Context) error {
    // Get desired children from worker
    desired := s.worker.DeclareChildren()

    // Build maps for diffing
    desiredMap := make(map[string]ChildSpec)
    for _, child := range desired.Children {
        desiredMap[child.Name] = child
    }

    currentMap := make(map[string]*childInstance)
    for name, instance := range s.children {
        currentMap[name] = instance
    }

    // Find children to add
    for name, spec := range desiredMap {
        if _, exists := currentMap[name]; !exists {
            s.logger.Info("Adding child", "name", name)
            s.addChild(ctx, name, spec)
        } else {
            // Child exists - check if supervisor changed (config change)
            current := currentMap[name]
            if current.supervisor != spec.Supervisor {
                s.logger.Info("Child supervisor changed, replacing", "name", name)
                s.removeChild(ctx, name)
                s.addChild(ctx, name, spec)
            }
        }
    }

    // Find children to remove
    for name := range currentMap {
        if _, desired := desiredMap[name]; !desired {
            s.logger.Info("Removing child", "name", name)
            s.removeChild(ctx, name)
        }
    }

    return nil
}
```

#### Pros
- ✅ Automatic config change detection
- ✅ Matches proven Kubernetes pattern
- ✅ Simple worker code - just return desired state
- ✅ No event system needed
- ✅ Works for dynamic N children (OPC UA browser)
- ✅ Idempotent and declarative

#### Cons
- ⚠️ Called every tick (performance concern for deep hierarchies)
- ⚠️ Config change detection requires supervisor pointer comparison
- ⚠️ Worker must track config changes in DeriveDesiredState

#### Performance Analysis
```
Assumptions:
- 100 bridges
- 3 children each (connection + 2 flows)
- 1 second tick interval

Cost per tick:
- 100 DeclareChildren() calls
- 300 pointer comparisons (3 children × 100 bridges)
- ~0 allocations if children unchanged
- ~10μs per bridge on modern CPU

Total: 1ms per tick for 100 bridges
```

**Performance Verdict:** Negligible for realistic workloads (<1000 workers).

---

### Option 2: Only on Config Change Event

**Approach:** Call `DeclareChildren()` only when config changes, triggered by event.

#### Worker Code
```go
type ProtocolConverterWorker struct {
    config *ProtocolConverterConfig
    configVersion int64 // Incremented on config change

    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor   *supervisor.Supervisor
    writeFlowSupervisor  *supervisor.Supervisor
}

// Only called when config changes
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
    // Recreate child supervisors with new config
    w.connectionSupervisor = supervisor.New(
        newConnectionWorker(w.config.IPAddress),
        w.logger,
    )
    w.readFlowSupervisor = supervisor.New(
        newReadFlowWorker(w.config.IPAddress),
        w.logger,
    )

    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor, StateMapping: {...}},
            {Name: "read_flow", Supervisor: w.readFlowSupervisor, StateMapping: {...}},
        },
    }
}

func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    oldVersion := w.configVersion
    w.config = userSpec
    w.configVersion = userSpec.Version

    // Notify supervisor if config changed
    if oldVersion != w.configVersion {
        // HOW? Worker doesn't have reference to supervisor
        // Would need: supervisor.OnConfigChange() callback?
    }

    return DesiredState{State: "active", UserSpec: userSpec}
}
```

#### Supervisor Logic
```go
type Supervisor struct {
    worker Worker
    children map[string]*childInstance
    lastDeclaredChildren *ChildDeclaration
}

// Worker calls this when config changes
func (s *Supervisor) OnConfigChange() {
    s.logger.Info("Config change detected, reconciling children")

    desired := s.worker.DeclareChildren()
    s.lastDeclaredChildren = desired

    s.reconcileChildren(desired)
}

func (s *Supervisor) Tick(ctx context.Context) error {
    // Don't call DeclareChildren() on every tick
    // Just tick existing children
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }

    return nil
}
```

#### Pros
- ✅ Minimal DeclareChildren() calls (only on config change)
- ✅ Clear performance optimization
- ✅ Explicit config change handling

#### Cons
- ❌ Requires config versioning system
- ❌ Worker needs callback mechanism to supervisor
- ❌ Circular dependency (worker → supervisor → worker)
- ❌ Edge case: What if config changes during child removal?
- ❌ More complex than Option 1
- ❌ Violates single responsibility (worker knows about supervisor lifecycle)

#### Decision: **REJECTED**
Too complex for marginal performance gain. Circular dependencies are architectural smell.

---

### Option 3: State-Aware DeriveDesiredState

**Approach:** `DeclareChildren()` called on every tick, but also receives current state to make state-aware decisions.

#### Worker Code
```go
// Extended API
func (w *ProtocolConverterWorker) DeclareChildren(currentState string) *ChildDeclaration {
    // State-aware child management
    if currentState == "idle" {
        // When idle, don't declare flow children (only connection)
        return &ChildDeclaration{
            Children: []ChildSpec{
                {Name: "connection", Supervisor: w.connectionSupervisor, StateMapping: {...}},
            },
        }
    }

    // When active, declare all children
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor, StateMapping: {...}},
            {Name: "read_flow", Supervisor: w.readFlowSupervisor, StateMapping: {...}},
            {Name: "write_flow", Supervisor: w.writeFlowSupervisor, StateMapping: {...}},
        },
    }
}

func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // State transitions control child existence
    if userSpec.Enabled {
        return DesiredState{State: "active", UserSpec: userSpec}
    } else {
        return DesiredState{State: "idle", UserSpec: userSpec}
    }
}
```

#### Supervisor Logic
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // Get current state
    currentState := s.triangularStore.GetDesiredState()

    // Call DeclareChildren with state awareness
    desired := s.worker.DeclareChildren(currentState)

    // Reconcile children
    s.reconcileChildren(desired)

    // Tick children
    // ...

    return nil
}
```

#### Pros
- ✅ State-driven child lifecycle (create flows only when active)
- ✅ Optimizes resource usage (don't create unnecessary children)
- ✅ Clean separation: DeriveDesiredState controls state, DeclareChildren controls children

#### Cons
- ⚠️ More complex API (DeclareChildren takes parameter)
- ⚠️ Mixes state-driven and config-driven child management
- ⚠️ Question: Should state transitions happen before or after child reconciliation?

#### Use Case Analysis

**When is state-aware child management needed?**

Example: Bridge should only create flow children when connection is up.

**Problem:** This mixes concerns:
- **Config-driven:** "Bridge should have 1 connection + 2 flows" (what children exist)
- **State-driven:** "Flows should only run when connection is up" (what state children should be in)

**Better Solution:** Use StateMapping to handle this:
```go
{
    Name: "read_flow",
    Supervisor: w.readFlowSupervisor,
    StateMapping: map[string]string{
        "active": "running",    // Connection up → flow running
        "idle":   "stopped",    // Connection down → flow stopped
    },
}
```

Flow child always exists, but only runs when parent is active.

**Decision:** State-awareness might be useful, but StateMapping handles most cases. Defer until proven needed.

---

## OPC UA Browser Pattern Analysis

### Challenge

```
User Config:
servers:
  - 10.0.0.1
  - 10.0.0.2
  - 10.0.0.N

Desired Structure:
OPCUABrowserSupervisor
├─ OPCUABrowserWorker (10.0.0.1)
├─ OPCUABrowserWorker (10.0.0.2)
└─ OPCUABrowserWorker (10.0.0.N)
```

N is dynamic, determined by config.

### Solution 1: Parent Worker with N Child Supervisors (Recommended)

#### Worker Code
```go
type OPCUABrowserParentWorker struct {
    config *OPCUABrowserConfig

    // Child supervisors created dynamically
    childSupervisors map[string]*supervisor.Supervisor // serverAddr → supervisor
}

func (w *OPCUABrowserParentWorker) DeclareChildren() *ChildDeclaration {
    children := []ChildSpec{}

    // Iterate over configured servers
    for _, serverAddr := range w.config.Servers {
        // Create child supervisor if needed
        if _, exists := w.childSupervisors[serverAddr]; !exists {
            childWorker := newOPCUABrowserWorker(serverAddr)
            w.childSupervisors[serverAddr] = supervisor.New(childWorker, w.logger)
        }

        // Add child spec
        children = append(children, ChildSpec{
            Name:       serverAddr, // e.g., "10.0.0.1"
            Supervisor: w.childSupervisors[serverAddr],
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    }

    // Auto-remove: Servers removed from config won't be in children array
    // Supervisor will automatically call removeChild() for them

    return &ChildDeclaration{Children: children}
}

func (w *OPCUABrowserParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    w.config = userSpec
    return DesiredState{State: "active", UserSpec: userSpec}
}

func (w *OPCUABrowserParentWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Parent's observed state could aggregate child states
    // But infrastructure is hidden - parent shouldn't know about child health
    // Just return parent's own state
    return ObservedState{State: "active"}, nil
}
```

#### Actual OPC UA Browser Worker
```go
type OPCUABrowserWorker struct {
    serverAddr string
    client     *opcua.Client
    nodeTree   map[string]NodeInfo
}

func (w *OPCUABrowserWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Connect to OPC UA server and browse
    if err := w.client.Connect(ctx); err != nil {
        return ObservedState{State: "error"}, err
    }

    // Browse nodes
    nodes, err := w.client.Browse(ctx)
    if err != nil {
        return ObservedState{State: "error"}, err
    }

    w.nodeTree = nodes
    return ObservedState{State: "browsing"}, nil
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: "browsing", UserSpec: userSpec}
}

// No DeclareChildren - leaf worker
```

#### Behavior

**Adding Server:**
```
User adds 10.0.0.100 to config
→ Supervisor calls DeriveDesiredState with new config
→ Supervisor calls DeclareChildren every tick
→ DeclareChildren sees new server, creates supervisor
→ DeclareChildren returns updated children array with 10.0.0.100
→ Supervisor diffs, sees new child, calls addChild
→ New OPCUABrowserWorker starts browsing 10.0.0.100
```

**Removing Server:**
```
User removes 10.0.0.2 from config
→ Supervisor calls DeriveDesiredState with new config
→ Supervisor calls DeclareChildren every tick
→ DeclareChildren doesn't include 10.0.0.2 in children array
→ Supervisor diffs, sees missing child, calls removeChild
→ OPCUABrowserWorker(10.0.0.2) stopped and cleaned up
```

**Pros:**
- ✅ Clean separation: Parent manages config, children implement browsing
- ✅ Each server gets isolated supervisor (independent restart, backoff)
- ✅ Auto-removal works perfectly
- ✅ Scales to N servers
- ✅ Follows hierarchical composition pattern

**Cons:**
- ⚠️ Parent worker has minimal logic (just config iteration)
- ⚠️ Could be seen as "extra layer"

### Solution 2: Single Worker with Internal Goroutines (Alternative)

```go
type OPCUABrowserWorker struct {
    config   *OPCUABrowserConfig
    browsers map[string]*serverBrowser // Internal goroutines
}

func (w *OPCUABrowserWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Start/stop internal browsers based on config
    for _, serverAddr := range w.config.Servers {
        if _, exists := w.browsers[serverAddr]; !exists {
            // Start new browser goroutine
            w.browsers[serverAddr] = newServerBrowser(serverAddr)
            go w.browsers[serverAddr].Run(ctx)
        }
    }

    // Stop removed browsers
    for serverAddr, browser := range w.browsers {
        if !w.config.HasServer(serverAddr) {
            browser.Stop()
            delete(w.browsers, serverAddr)
        }
    }

    // Aggregate state from all browsers
    return ObservedState{State: "browsing"}, nil
}

// No DeclareChildren - single worker manages everything
```

**Pros:**
- ✅ Simpler hierarchy (no parent layer)
- ✅ Single worker manages all servers

**Cons:**
- ❌ Loses supervisor benefits for individual servers (no independent restart)
- ❌ Error in one server affects entire worker
- ❌ Manual goroutine management (vs supervisor handles this)
- ❌ Doesn't utilize hierarchical composition pattern

**Decision:** Solution 1 (Parent with N Child Supervisors) is better - uses supervisor pattern benefits.

---

## Recommendation

### Adopt Option 1: Every Tick with Smart Diffing

**Rationale:**
1. **Proven Pattern** - Matches Kubernetes reconciliation (industry standard)
2. **Simple Worker Code** - Workers just return desired state, no events or versioning
3. **Automatic Config Changes** - Supervisor detects changes via diffing
4. **Handles Dynamic Children** - OPC UA browser pattern works perfectly
5. **Performance** - Negligible overhead (<1ms for 100 bridges)
6. **Idempotent** - Declarative, no side effects
7. **Aligns with Constraints** - Infrastructure hidden, workers declarative

### Worker API (No Changes from Phase 0)

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState
    DeclareChildren() *ChildDeclaration  // Called every tick
}
```

### Config Change Handling Pattern

**Worker Responsibilities:**
1. Store config in `DeriveDesiredState()`
2. Recreate child supervisors when config changes
3. Return updated children in `DeclareChildren()`

**Supervisor Responsibilities:**
1. Call `DeclareChildren()` every tick
2. Diff old vs new children structure
3. Add/update/remove children as needed
4. Log all changes verbosely

### Code Example: Bridge Config Change

```go
type ProtocolConverterWorker struct {
    config *ProtocolConverterConfig

    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor   *supervisor.Supervisor
    writeFlowSupervisor  *supervisor.Supervisor
}

// Called every tick - supervisor diffs output
func (w *ProtocolConverterWorker) DeclareChildren() *ChildDeclaration {
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
            {
                Name:       "write_flow",
                Supervisor: w.writeFlowSupervisor,
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}

// Config changes detected here
func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
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
        w.writeFlowSupervisor = supervisor.New(
            newWriteFlowWorker(userSpec.IPAddress),
            w.logger,
        )
    }

    return DesiredState{
        State:    "active",
        UserSpec: userSpec,
    }
}
```

**What happens when IP changes:**
```
1. User changes IP: 10.0.0.1 → 10.0.0.2
2. Supervisor calls DeriveDesiredState(new config)
3. Worker detects config change, recreates child supervisors
4. Supervisor calls DeclareChildren() (next tick or same tick)
5. DeclareChildren() returns children with new supervisor instances
6. Supervisor diffs: supervisor pointers changed for all 3 children
7. Supervisor logs: "Child supervisor changed, replacing connection"
8. Supervisor removes old children, adds new children with new config
9. New children start with IP 10.0.0.2
```

### Code Example: OPC UA Browser

```go
type OPCUABrowserParentWorker struct {
    config *OPCUABrowserConfig
    childSupervisors map[string]*supervisor.Supervisor
}

func (w *OPCUABrowserParentWorker) DeclareChildren() *ChildDeclaration {
    children := []ChildSpec{}

    for _, serverAddr := range w.config.Servers {
        // Lazy create child supervisor
        if _, exists := w.childSupervisors[serverAddr]; !exists {
            childWorker := newOPCUABrowserWorker(serverAddr)
            w.childSupervisors[serverAddr] = supervisor.New(childWorker, w.logger)
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

func (w *OPCUABrowserParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    w.config = userSpec
    return DesiredState{State: "active", UserSpec: userSpec}
}
```

**What happens when server added:**
```
1. User adds server 10.0.0.100
2. Supervisor calls DeriveDesiredState(new config)
3. Supervisor calls DeclareChildren()
4. DeclareChildren() sees new server, creates supervisor, adds to children
5. Supervisor diffs: new child "10.0.0.100" not in current children
6. Supervisor logs: "Adding child 10.0.0.100"
7. Supervisor calls addChild, starts new OPCUABrowserWorker
```

---

## Trade-offs and Decision Criteria

### When to Use Every-Tick Reconciliation (Recommended Default)

**Use When:**
- ✅ Config changes need to propagate to children
- ✅ Dynamic N children (OPC UA browser pattern)
- ✅ Declarative child management desired
- ✅ Performance overhead acceptable (<1000 workers)

**Don't Use When:**
- ❌ Workers are completely stateless (no children)
- ❌ Extreme performance requirements (>10k workers, <100ms tick)

### Alternative: Lazy Reconciliation (If Needed)

If performance becomes issue (unlikely), add reconciliation interval:

```go
type Supervisor struct {
    reconcileInterval time.Duration // e.g., 10 seconds
    lastReconcile     time.Time
}

func (s *Supervisor) Tick(ctx context.Context) error {
    // Reconcile children every 10 seconds (like Kubernetes)
    if time.Since(s.lastReconcile) > s.reconcileInterval {
        desired := s.worker.DeclareChildren()
        s.reconcileChildren(desired)
        s.lastReconcile = time.Now()
    }

    // Tick children every tick
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }

    return nil
}
```

**Trade-off:** Config changes delayed up to reconcileInterval.

---

## Integration with Phase 0 Plan

### Changes Required to Phase 0

**NO API CHANGES** - Phase 0 Worker API is sufficient:
```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState
    DeclareChildren() *ChildDeclaration
}
```

**Clarifications Added:**
1. `DeclareChildren()` called **every tick** (reconciliation loop)
2. Supervisor diffs old vs new children via **pointer comparison**
3. Config changes handled in `DeriveDesiredState()` by **recreating supervisors**
4. Dynamic children handled by **iterating config in DeclareChildren()**

### Supervisor Implementation Requirements

```go
type Supervisor struct {
    worker Worker
    children map[string]*childInstance
    lastDeclaredChildren *ChildDeclaration // For diffing
}

type childInstance struct {
    name       string
    supervisor *supervisor.Supervisor
    stateMapping map[string]string
}

func (s *Supervisor) Tick(ctx context.Context) error {
    // 1. Reconcile children every tick
    desired := s.worker.DeclareChildren()
    s.reconcileChildren(desired)
    s.lastDeclaredChildren = desired

    // 2. Derive desired state
    desiredState := s.worker.DeriveDesiredState(s.triangularStore.GetUserSpec())
    s.triangularStore.SetDesiredState(desiredState)

    // 3. Apply state mapping to children
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }

    // 4. Tick children
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }

    // 5. Collect observed state
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)

    return nil
}

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    desiredMap := make(map[string]ChildSpec)
    for _, child := range desired.Children {
        desiredMap[child.Name] = child
    }

    // Add or update children
    for name, spec := range desiredMap {
        if current, exists := s.children[name]; exists {
            // Child exists - check if supervisor changed
            if current.supervisor != spec.Supervisor {
                s.logger.Info("Child supervisor changed, replacing", "name", name)
                s.removeChild(name)
                s.addChild(name, spec)
            }
        } else {
            s.logger.Info("Adding child", "name", name)
            s.addChild(name, spec)
        }
    }

    // Remove children not in desired
    for name := range s.children {
        if _, desired := desiredMap[name]; !desired {
            s.logger.Info("Removing child", "name", name)
            s.removeChild(name)
        }
    }
}

func (s *Supervisor) addChild(name string, spec ChildSpec) {
    s.children[name] = &childInstance{
        name:         name,
        supervisor:   spec.Supervisor,
        stateMapping: spec.StateMapping,
    }
}

func (s *Supervisor) removeChild(name string) {
    if child, exists := s.children[name]; exists {
        child.supervisor.Shutdown()
        delete(s.children, name)
    }
}
```

---

## Summary

### Answers to Core Questions

1. **When should DeclareChildren() be called?**
   - **Every tick** with smart diffing (Kubernetes reconciliation pattern)
   - Supervisor diffs via pointer comparison
   - Negligible performance overhead

2. **Where should children be modified?**
   - **Config-driven (what children exist):** `DeclareChildren()` iterates config
   - **State-driven (what state children should be in):** StateMapping in ChildSpec
   - **Config changes:** `DeriveDesiredState()` recreates supervisors

3. **How to handle dynamic N children?**
   - **Parent worker pattern:** One parent worker, N child supervisors
   - Parent iterates config, creates child supervisors lazily
   - Auto-removal when children not in desired structure

### Key Design Principles

- ✅ **Declarative over imperative** - Workers declare desired state, supervisor executes
- ✅ **Reconciliation over events** - Every-tick diffing simpler than event system
- ✅ **Infrastructure hidden** - Workers don't know about supervisor internals
- ✅ **Config in DeriveDesiredState** - Single source of truth for config changes
- ✅ **Pointer comparison for change detection** - Simple and reliable

### Next Steps

1. ✅ No API changes needed to Phase 0 proposal
2. Implement supervisor reconciliation logic
3. Test with bridge config change scenario
4. Test with OPC UA browser dynamic children
5. Performance benchmark with 1000 workers
