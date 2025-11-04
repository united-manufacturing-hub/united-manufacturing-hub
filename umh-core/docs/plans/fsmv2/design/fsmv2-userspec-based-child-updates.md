# FSMv2: UserSpec-Based Child Updates (Simplified Approach)

**Date:** 2025-11-02
**Status:** Analysis
**Related:**
- `fsmv2-phase0-worker-child-api.md` - Phase 0 API proposal
- `fsmv2-config-changes-and-dynamic-children.md` - Every-tick reconciliation analysis
- `fsmv2-developer-expectations-current-api.md` - Developer usability analysis

## User's Proposed Idea

> "only change the specs whenever the user spec of the parent changes. and then have a state mapping? or maybe this is then getting to complicated?"

**Interpretation:**
- Call `DeclareChildren()` **only** when parent's UserSpec changes (not every tick)
- Rely on StateMapping for state-driven child state changes
- Separate config-driven updates (what children exist) from state-driven updates (what state children are in)

---

## Proposed API Options

### Option A: UserSpec Parameter

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState

    // Only called when userSpec changes
    DeclareChildren(userSpec UserSpec) *ChildDeclaration
}
```

**Change:** Add `userSpec` parameter to make dependency explicit

### Option B: Combined Return

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // Return both desired state AND children declaration
    DeriveDesiredState(userSpec UserSpec) (DesiredState, *ChildDeclaration)
}
```

**Change:** Combine DeriveDesiredState and DeclareChildren into single method

### Option C: Explicit Change Signal

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState
    DeclareChildren() *ChildDeclaration

    // NEW: Worker notifies when config changed
    OnConfigChange(oldUserSpec, newUserSpec UserSpec) bool
}
```

**Change:** Add callback for worker to signal config changes

---

## UserSpec Change Detection

**Challenge:** How does supervisor know when UserSpec changed?

### Mechanism 1: Deep Equality Comparison

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // Deep equality check
    if !reflect.DeepEqual(newUserSpec, s.lastUserSpec) {
        s.logger.Info("UserSpec changed, reconciling children")

        // Call DeclareChildren with new spec
        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)

        s.lastUserSpec = newUserSpec
    }

    // ... rest of tick
}
```

**Pros:**
- ✅ Automatic change detection
- ✅ Works for any UserSpec structure
- ✅ No developer effort

**Cons:**
- ❌ Deep equality is expensive for complex structs
- ❌ False positives if UserSpec has transient fields
- ❌ Compares fields that might not affect children (e.g., parent's internal state)

### Mechanism 2: Version Field

```go
type UserSpec struct {
    Version   int64  // Incremented on any change
    IPAddress string
    Servers   []string
    // ...
}

func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // Version comparison (cheap)
    if newUserSpec.Version != s.lastUserSpecVersion {
        s.logger.Info("UserSpec changed", "version", newUserSpec.Version)

        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)

        s.lastUserSpecVersion = newUserSpec.Version
    }

    // ... rest of tick
}
```

**Pros:**
- ✅ Very fast (int64 comparison)
- ✅ Explicit versioning
- ✅ No false positives

**Cons:**
- ❌ Developer must manage versions
- ❌ Easy to forget to increment
- ❌ Silent failures if version not updated

### Mechanism 3: Content Hash

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // Hash config (deterministic)
    newHash := hashStruct(newUserSpec)

    if newHash != s.lastUserSpecHash {
        s.logger.Info("UserSpec changed", "hash", newHash)

        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)

        s.lastUserSpecHash = newHash
    }

    // ... rest of tick
}

func hashStruct(v interface{}) string {
    h := sha256.New()
    enc := gob.NewEncoder(h)
    enc.Encode(v)
    return hex.EncodeToString(h.Sum(nil))
}
```

**Pros:**
- ✅ Automatic change detection
- ✅ No version management needed
- ✅ Handles nested structs

**Cons:**
- ❌ Hashing overhead (more expensive than version, less than DeepEqual)
- ❌ Requires deterministic serialization
- ❌ Still compares transient fields

### Mechanism 4: Worker-Controlled (Option C API)

```go
func (w *BridgeWorker) OnConfigChange(oldSpec, newSpec UserSpec) bool {
    // Worker decides if children need updating
    return oldSpec.IPAddress != newSpec.IPAddress ||
           !slicesEqual(oldSpec.Servers, newSpec.Servers)
}

func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // Ask worker if config changed
    if s.worker.OnConfigChange(s.lastUserSpec, newUserSpec) {
        s.logger.Info("Worker signaled config change")

        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)
    }

    s.lastUserSpec = newUserSpec

    // ... rest of tick
}
```

**Pros:**
- ✅ Worker controls what triggers child updates
- ✅ Can ignore transient fields
- ✅ Precise control

**Cons:**
- ❌ Developer must implement change detection
- ❌ Easy to forget fields
- ❌ More boilerplate

### Recommendation: Mechanism 2 (Version Field)

**Rationale:**
- Cheapest performance-wise
- Explicit and predictable
- Aligned with existing patterns (Kubernetes resourceVersion, Etcd modRevision)
- Developer can control when children update by controlling version

**Implementation:**
```go
type UserSpec struct {
    Version int64  // REQUIRED: Increment when children should update
    // ... other fields
}
```

---

## Clean Separation: Config vs State

### Concept

**Config Changes** (UserSpec changed):
- Call `DeclareChildren(userSpec)` → update what children exist
- Example: IP address changed, add/remove servers

**State Changes** (Parent state changed):
- Use `StateMapping` → update what state children are in
- Example: Parent active → idle, children running → stopped

### Bridge Config Change Example

**Scenario:** IP address changes 10.0.0.1 → 10.0.0.2

```go
type BridgeWorker struct {
    config *BridgeConfig
}

// Called when userSpec changes
func (w *BridgeWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    w.config = userSpec

    // Create new supervisors with new config
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                Supervisor: supervisor.New(
                    newConnectionWorker(userSpec.IPAddress),
                    w.logger,
                ),
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            {
                Name: "read_flow",
                Supervisor: supervisor.New(
                    newReadFlowWorker(userSpec.IPAddress),
                    w.logger,
                ),
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    w.config = userSpec

    // Just return desired state (no supervisor recreation here!)
    return DesiredState{
        State:    "active",
        UserSpec: userSpec,
    }
}
```

**Flow:**
```
1. User changes IP: 10.0.0.1 → 10.0.0.2
2. UserSpec.Version incremented: 1 → 2
3. Supervisor detects version change
4. Supervisor calls DeclareChildren(newUserSpec)
5. Worker creates new supervisors with new IP
6. Supervisor diffs: all supervisor pointers changed
7. Supervisor replaces all children
8. Children start with new IP
```

**Key Insight:** Worker doesn't track config changes anymore! Just receives new UserSpec and returns fresh children.

### OPC UA Browser Example

**Scenario:** User adds server 10.0.0.100

```go
func (w *OPCUABrowserWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    children := []ChildSpec{}

    // Create supervisor for each server in config
    for _, serverAddr := range userSpec.Servers {
        children = append(children, ChildSpec{
            Name: serverAddr,
            Supervisor: supervisor.New(
                newServerBrowserWorker(serverAddr),
                w.logger,
            ),
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    }

    return &ChildDeclaration{Children: children}
}
```

**Flow:**
```
1. User adds server 10.0.0.100
2. UserSpec.Servers: [10.0.0.1, 10.0.0.2] → [10.0.0.1, 10.0.0.2, 10.0.0.100]
3. UserSpec.Version: 1 → 2
4. Supervisor detects version change
5. Supervisor calls DeclareChildren(newUserSpec)
6. Worker iterates servers, creates 3 supervisors
7. Supervisor diffs: new child "10.0.0.100"
8. Supervisor adds new child
9. New browser starts for 10.0.0.100
```

**Key Insight:** No lazy creation needed! Just iterate config and create supervisors.

### State Change Example (NOT Config)

**Scenario:** Parent transitions active → idle

```go
// UserSpec unchanged, so DeclareChildren NOT called
// Only StateMapping used

StateMapping: map[string]string{
    "active": "running",  // Was here
    "idle":   "stopped",  // Now here
}
```

**Flow:**
```
1. Parent state changes: active → idle
2. UserSpec unchanged (Version still 2)
3. Supervisor does NOT call DeclareChildren
4. Supervisor looks up StateMapping
5. Supervisor sees: idle → "stopped"
6. Supervisor sets all children desired state to "stopped"
7. Children transition to stopped
```

**Key Insight:** Clean separation - state changes don't trigger child recreation.

---

## Edge Cases

### Edge Case 1: Simultaneous Config + State Change

**Scenario:** User changes IP address AND sets parent to idle in same update

```go
// UserSpec changes
oldUserSpec = {Version: 1, IPAddress: "10.0.0.1", DesiredState: "active"}
newUserSpec = {Version: 2, IPAddress: "10.0.0.2", DesiredState: "idle"}
```

**Flow:**
```
1. Supervisor detects version change (1 → 2)
2. Supervisor calls DeclareChildren(newUserSpec)
3. Worker creates new supervisors with new IP
4. Supervisor replaces children
5. Supervisor calls DeriveDesiredState(newUserSpec)
6. Worker returns DesiredState{State: "idle"}
7. Supervisor looks up StateMapping for "idle"
8. Supervisor sets children desired state to "stopped"
```

**Result:** Both config and state updates applied correctly

**No conflict!**

### Edge Case 2: Transient Fields in UserSpec

**Scenario:** UserSpec has field that doesn't affect children

```go
type UserSpec struct {
    Version      int64
    IPAddress    string
    LastSeen     time.Time  // Transient - changes frequently
}
```

**Problem:** If version auto-incremented on any field change, children would recreate on every LastSeen update!

**Solution:** Developer must control version:
```go
// Only increment version when children-affecting fields change
func UpdateIPAddress(spec *UserSpec, newIP string) {
    spec.IPAddress = newIP
    spec.Version++  // Increment because children need update
}

func UpdateLastSeen(spec *UserSpec, t time.Time) {
    spec.LastSeen = t
    // DON'T increment version - doesn't affect children
}
```

**Trade-off:** Developer must know which fields affect children

### Edge Case 3: Partial Config Changes

**Scenario:** Only one child needs config update, not all

```go
// Old config
{IPAddress: "10.0.0.1", ReadInterval: 1000, WriteInterval: 2000}

// New config: Only write interval changed
{IPAddress: "10.0.0.1", ReadInterval: 1000, WriteInterval: 3000}
```

**Current Approach (UserSpec-Based):**
```go
func (w *BridgeWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    // Must recreate ALL supervisors, even if only write_flow changed
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: supervisor.New(...)},  // Recreated unnecessarily
            {Name: "read_flow", Supervisor: supervisor.New(...)},   // Recreated unnecessarily
            {Name: "write_flow", Supervisor: supervisor.New(...)},  // Actually needs update
        },
    }
}
```

**Problem:** Can't selectively update only affected children

**Alternative (Every-Tick Approach):**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Selective recreation
    if userSpec.WriteInterval != w.config.WriteInterval {
        w.writeFlowSupervisor = supervisor.New(
            newWriteFlowWorker(userSpec.WriteInterval),
            w.logger,
        )
    }

    w.config = userSpec
    return DesiredState{State: "active", UserSpec: userSpec}
}

func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // Return current supervisors (only write_flow pointer changed)
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor},  // Same pointer
            {Name: "read_flow", Supervisor: w.readFlowSupervisor},     // Same pointer
            {Name: "write_flow", Supervisor: w.writeFlowSupervisor},   // New pointer!
        },
    }
}
```

**Trade-off:** UserSpec-based approach less granular for partial updates

---

## Performance Comparison

### Current Approach (Every Tick)

```
Assumptions:
- 100 bridges
- 1 second tick interval
- Config changes: 1 per hour per bridge

Cost per tick:
- 100 DeclareChildren() calls
- 300 pointer comparisons (3 children × 100)
- ~1ms total

Cost per hour:
- 3600 ticks × 1ms = 3.6 seconds
- 100 config changes × reconciliation = negligible

Total overhead: 3.6 seconds per hour (0.1% CPU)
```

### Proposed Approach (UserSpec Change)

```
Assumptions:
- Same 100 bridges
- Same 1 second tick interval
- Same 1 config change per hour per bridge

Cost per tick:
- 100 version comparisons (int64)
- ~0.01ms total

Cost per hour:
- 3600 ticks × 0.01ms = 36ms
- 100 config changes × DeclareChildren = 100ms

Total overhead: 136ms per hour (0.004% CPU)
```

**Performance Gain:** 96% reduction in CPU (3.6s → 0.136s per hour)

**Is it significant?**
- For 100 bridges: NO (both negligible)
- For 10,000 bridges: Maybe (360s vs 13.6s per hour)
- For 100,000 bridges: YES (10 hours vs 22 minutes per hour - current unusable)

**Verdict:** Only matters at extreme scale (10k+ workers)

---

## Complexity Comparison

### Current Approach (Every Tick)

**Worker Code:**
```go
// 30 lines total

type BridgeWorker struct {
    config *BridgeConfig
    connectionSupervisor *supervisor.Supervisor
    readFlowSupervisor *supervisor.Supervisor
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // 10 lines: Detect config changes, recreate supervisors
    oldIP := w.config.IPAddress
    w.config = userSpec

    if oldIP != userSpec.IPAddress {
        w.connectionSupervisor = supervisor.New(...)
        w.readFlowSupervisor = supervisor.New(...)
    }

    return DesiredState{State: "active", UserSpec: userSpec}
}

func (w *BridgeWorker) DeclareChildren() *ChildDeclaration {
    // 10 lines: Return current supervisors
    return &ChildDeclaration{
        Children: []ChildSpec{
            {Name: "connection", Supervisor: w.connectionSupervisor, StateMapping: {...}},
            {Name: "read_flow", Supervisor: w.readFlowSupervisor, StateMapping: {...}},
        },
    }
}
```

**Supervisor Code:**
```go
// 50 lines total

func (s *Supervisor) Tick(ctx context.Context) error {
    // 20 lines: Call DeclareChildren every tick, diff, reconcile
    desired := s.worker.DeclareChildren()
    s.reconcileChildren(desired)

    // ... rest
}

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    // 30 lines: Diff logic, add/remove/update children
}
```

**Total:** ~80 lines, split between worker and supervisor

### Proposed Approach (UserSpec Change)

**Worker Code:**
```go
// 20 lines total (simpler!)

type BridgeWorker struct {
    // No supervisor fields needed!
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // 3 lines: Just return desired state
    return DesiredState{State: "active", UserSpec: userSpec}
}

func (w *BridgeWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    // 15 lines: Create supervisors fresh each time
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                Supervisor: supervisor.New(
                    newConnectionWorker(userSpec.IPAddress),
                    w.logger,
                ),
                StateMapping: {...},
            },
            {
                Name: "read_flow",
                Supervisor: supervisor.New(
                    newReadFlowWorker(userSpec.IPAddress),
                    w.logger,
                ),
                StateMapping: {...},
            },
        },
    }
}
```

**Supervisor Code:**
```go
// 70 lines total (more complex)

func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // 15 lines: Change detection
    if newUserSpec.Version != s.lastUserSpecVersion {
        s.logger.Info("UserSpec changed", "version", newUserSpec.Version)
        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)
        s.lastUserSpecVersion = newUserSpec.Version
    }

    // ... rest
}

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    // Same 30 lines: Diff logic
}
```

**Total:** ~90 lines, more in supervisor

**Complexity Comparison:**

| Aspect | Current (Every Tick) | Proposed (UserSpec) | Winner |
|--------|---------------------|---------------------|--------|
| Worker LoC | 30 lines | 20 lines | ✅ Proposed |
| Supervisor LoC | 50 lines | 70 lines | ❌ Current |
| Total LoC | 80 lines | 90 lines | ≈ Tie |
| Worker concepts | 4 (config tracking, supervisor lifecycle, pointer comparison, DeclareChildren) | 2 (DeclareChildren, StateMapping) | ✅ Proposed |
| Supervisor concepts | 2 (every-tick diffing, reconciliation) | 3 (change detection, diffing, reconciliation) | ❌ Current |
| Config change handling | Worker tracks changes | Supervisor detects changes | ✅ Proposed |
| Silent failures | Yes (forget to recreate) | No (recreated automatically) | ✅ Proposed |
| Partial updates | Possible (selective recreation) | Not possible (all children recreated) | ❌ Current |

**Verdict:** Worker code simpler, supervisor code slightly more complex, overall comparable

---

## Recommendation

### Adopt UserSpec-Based Approach with Version Field

**Rationale:**

1. **Simpler Worker Code** (-33% lines, -50% concepts)
   - No supervisor lifecycle management
   - No config change tracking
   - Just: "Given this config, what children should exist?"

2. **No Silent Failures**
   - Developer can't forget to recreate supervisors
   - Supervisors automatically recreated on config change
   - Config changes always propagate

3. **Clear Separation**
   - Config changes (UserSpec.Version) → DeclareChildren
   - State changes (StateMapping) → child state updates
   - Intuitive mental model

4. **Performance Benefit** (at scale)
   - 96% less overhead (only matters at 10k+ workers)
   - Version comparison is cheapest

5. **Aligns with Established Patterns**
   - Kubernetes: resourceVersion triggers reconciliation
   - Etcd: modRevision for change detection
   - Git: SHA-based change detection

**Trade-offs Accepted:**

1. **Version Management Required**
   - Developer must increment UserSpec.Version when children should update
   - Could forget to increment (but better than forgetting to recreate supervisors)
   - Mitigation: Provide UpdateUserSpec() helper that auto-increments

2. **No Partial Updates**
   - Config change recreates all children, not just affected ones
   - Performance impact negligible (supervisor recreation is cheap)

3. **Slightly More Complex Supervisor**
   - Supervisor needs change detection logic (+15 lines)
   - Worth it for simpler worker code

---

## Proposed API

### Final API

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(userSpec UserSpec) DesiredState

    // Called when userSpec.Version changes
    // Worker should create fresh supervisors based on userSpec
    DeclareChildren(userSpec UserSpec) *ChildDeclaration
}

type ChildDeclaration struct {
    Children []ChildSpec
}

type ChildSpec struct {
    Name         string
    Supervisor   *Supervisor
    StateMapping map[string]string  // Parent state → child desired state
}

// UserSpec MUST have Version field
type UserSpec struct {
    Version int64  // REQUIRED: Increment when children should update
    // ... other fields
}
```

### Supervisor Implementation

```go
type Supervisor struct {
    worker Worker
    children map[string]*childInstance
    lastUserSpecVersion int64
}

func (s *Supervisor) Tick(ctx context.Context) error {
    newUserSpec := s.triangularStore.GetUserSpec()

    // Detect config changes via version
    if newUserSpec.Version != s.lastUserSpecVersion {
        s.logger.Info("UserSpec changed, reconciling children",
            "old_version", s.lastUserSpecVersion,
            "new_version", newUserSpec.Version,
        )

        // Call DeclareChildren with new config
        desired := s.worker.DeclareChildren(newUserSpec)
        s.reconcileChildren(desired)

        s.lastUserSpecVersion = newUserSpec.Version
    }

    // Derive desired state
    desiredState := s.worker.DeriveDesiredState(newUserSpec)
    s.triangularStore.SetDesiredState(desiredState)

    // Apply state mapping to children
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }

    // Tick children
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }

    // Collect observed state
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)

    return nil
}
```

---

## Code Examples

### Bridge with Config Change

```go
type BridgeWorker struct{}

func (w *BridgeWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                Supervisor: supervisor.New(
                    newConnectionWorker(userSpec.IPAddress),
                    logger,
                ),
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            {
                Name: "read_flow",
                Supervisor: supervisor.New(
                    newReadFlowWorker(userSpec.IPAddress),
                    logger,
                ),
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: "active", UserSpec: userSpec}
}
```

**Total:** 20 lines (vs 30 in current approach)

### OPC UA Browser

```go
type OPCUABrowserWorker struct{}

func (w *OPCUABrowserWorker) DeclareChildren(userSpec UserSpec) *ChildDeclaration {
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

    return &ChildDeclaration{Children: children}
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: "active", UserSpec: userSpec}
}
```

**Total:** 15 lines (vs 25 in current approach with lazy creation)

---

## Summary

### Core Insight

**Current approach:** Worker manages supervisor lifecycle, supervisor just diffs
**Proposed approach:** Supervisor manages supervisor lifecycle, worker just declares

**Result:** Complexity moved from worker to supervisor (good - worker code is written more often)

### Decision Criteria

**Choose UserSpec-Based Approach If:**
- ✅ Want simpler worker code (less for developers to learn)
- ✅ Want to eliminate "forgot to recreate supervisor" bugs
- ✅ Want clear config vs state separation
- ✅ Have >1000 workers (performance matters)

**Choose Every-Tick Approach If:**
- ✅ Need partial child updates (only recreate affected children)
- ✅ Don't want version management overhead
- ✅ Prefer worker control over when children update

### Recommendation: **UserSpec-Based**

The simplicity for worker developers outweighs the minor supervisor complexity increase. Preventing "forgot to recreate" bugs is critical for production reliability.

### API Changes Required

1. Add `userSpec` parameter to `DeclareChildren(userSpec UserSpec)`
2. Require `Version int64` field in all UserSpec types
3. Update supervisor to use version-based change detection
4. Update documentation/examples

**Migration Impact:** Low - workers just need to add parameter and use it instead of stored config
