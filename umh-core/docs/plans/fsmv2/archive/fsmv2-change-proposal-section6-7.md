# FSMv2 Master Plan Change Proposal - Sections 6-7

**Date:** 2025-11-03
**Status:** Updated Timeline and Architecture
**Related:**
- `fsmv2-master-plan-gap-analysis.md` - Complete gap analysis
- `fsmv2-derive-desired-state-complete-definition.md` - API specifications
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Original master plan

---

## Section 6: Updated Timeline

### 6.1 Overview

**Original estimate:** 6 weeks (750 LOC)
**New estimate:** 10-12 weeks (1,720 LOC)
**Increase:** +66% duration, +129% code

**Reason for increase:** Hierarchical composition, templating, and variables system not included in original master plan but are foundational to FSMv2.

### 6.2 Week-by-Week Breakdown

#### **Week 1: Phase 0 Part 1 - ChildSpec & WorkerFactory**

**Tasks:**
- Define ChildSpec structure (Name, WorkerType, UserSpec, StateMapping)
- Define DesiredState structure (State, Config, ChildrenSpecs)
- Update Worker.DeriveDesiredState() signature (add userSpec param, return DesiredState)
- Implement WorkerFactory interface and DefaultWorkerFactory
- Add WorkerType constants (ConnectionWorker, BenthosWorker, etc.)

**Dependencies:** None (foundational)

**Parallel Work:** None (must complete before other work)

**Deliverables:**
- `pkg/fsm/common/types.go` - ChildSpec, DesiredState structures
- `pkg/fsm/common/factory.go` - WorkerFactory implementation
- `pkg/fsm/common/worker.go` - Updated Worker interface

**Review Gates:**
- ✅ Worker interface compiles with new signature
- ✅ WorkerFactory can create test workers
- ✅ ChildSpec serializes to JSON correctly

**Estimated LOC:** ~150 lines (structures + factory + tests)

---

#### **Week 2: Phase 0 Part 2 - reconcileChildren & StateMapping**

**Tasks:**
- Implement Supervisor.reconcileChildren() (add/update/remove children)
- Implement Supervisor.applyStateMapping() (map parent state to child desired state)
- Update Supervisor constructor to accept WorkerFactory
- Child lifecycle management (create, shutdown, replace on UserSpec change)
- Tests for child reconciliation (add/remove/update scenarios)

**Dependencies:** Week 1 (needs ChildSpec & WorkerFactory)

**Parallel Work:** None (blocks Phase 0.5)

**Deliverables:**
- `pkg/fsm/supervisor/reconcile.go` - Child reconciliation logic
- `pkg/fsm/supervisor/supervisor.go` - Updated constructor, applyStateMapping
- Tests for add/remove/update children scenarios

**Review Gates:**
- ✅ Children added when ChildSpecs appear in DesiredState
- ✅ Children removed when ChildSpecs disappear
- ✅ Children recreated when UserSpec changes
- ✅ StateMapping applied before child tick
- ✅ All tests passing

**Estimated LOC:** ~260 lines (reconcile + lifecycle + tests)

**CRITICAL PATH:** Blocks all other phases

---

#### **Week 3: Phase 0.5 - Templating & Variables (Part 1)**

**Tasks:**
- Define VariableBundle structure (User/Global/Internal)
- Implement VariableBundle.Flatten() method
- Implement RenderTemplate() with strict mode (missingkey=error)
- Add template validation (no {{ markers remain after rendering)
- Tests for variable flattening and template rendering

**Dependencies:** Week 2 (needs UserSpec integration)

**Parallel Work:** Can start while Week 2 tests are finishing

**Deliverables:**
- `pkg/templating/variables.go` - VariableBundle + Flatten()
- `pkg/templating/render.go` - RenderTemplate() implementation
- Tests for variable access patterns ({{ .IP }}, {{ .global.kafka }})

**Review Gates:**
- ✅ User variables flatten to top-level
- ✅ Global/Internal variables nested under namespace
- ✅ Template rendering fails on missing variables (strict mode)
- ✅ No {{ markers in rendered output
- ✅ All tests passing

**Estimated LOC:** ~105 lines (structures + flatten + render + tests)

---

#### **Week 4: Phase 1 Part 1 - Infrastructure: Backoff & HealthChecker**

**Tasks:**
- Implement ExponentialBackoff (baseDelay, maxDelay, attempts tracking)
- Implement InfrastructureHealthChecker structure
- Add CheckChildConsistency() method (use s.children map)
- Tests for backoff delay calculation and max attempts

**Dependencies:** Week 2 (needs s.children map)

**Parallel Work:** Can run in parallel with Week 3

**Deliverables:**
- `pkg/fsm/supervision/backoff.go` - ExponentialBackoff implementation
- `pkg/fsm/supervision/health_checker.go` - InfrastructureHealthChecker
- Tests for backoff exponential growth and consistency checks

**Review Gates:**
- ✅ Backoff starts at baseDelay and grows exponentially
- ✅ Backoff caps at maxDelay
- ✅ CheckChildConsistency() accesses children via map (not hardcoded)
- ✅ All tests passing

**Estimated LOC:** ~150 lines (backoff + health checker + tests)

---

#### **Week 5: Phase 1 Part 2 - Infrastructure: Circuit Breaker & Child Restart**

**Tasks:**
- Integrate InfrastructureHealthChecker into Supervisor.Tick()
- Implement circuit breaker logic (circuitOpen flag)
- Add child restart logic (via s.children[name].supervisor.Restart())
- Escalation after max backoff attempts
- Tests for circuit breaker pauses all children

**Dependencies:** Week 4 (needs backoff & health checker)

**Parallel Work:** None (blocks integration)

**Deliverables:**
- `pkg/fsm/supervisor/tick.go` - Updated Tick() with infrastructure checks
- `pkg/fsm/supervisor/restart.go` - Child restart with backoff
- Tests for circuit open/close and child restart scenarios

**Review Gates:**
- ✅ Circuit breaker pauses all children when open
- ✅ Child restart uses exponential backoff
- ✅ Escalation after max attempts
- ✅ Circuit closes when health check passes
- ✅ All tests passing

**Estimated LOC:** ~170 lines (circuit breaker + restart + tests)

---

#### **Week 6: Phase 2 Part 1 - Async Actions: Core Structure & Worker Pool**

**Tasks:**
- Implement ActionExecutor structure (global instance)
- Implement worker pool pattern (goroutine pool)
- Add action queueing with timeout handling
- Implement HasActionInProgress(workerID) check
- Tests for action queuing and worker pool behavior

**Dependencies:** Week 2 (needs child names for action IDs)

**Parallel Work:** Can run in parallel with Week 5

**Deliverables:**
- `pkg/fsm/actions/executor.go` - ActionExecutor implementation
- `pkg/fsm/actions/pool.go` - Worker pool
- Tests for queuing, timeout, and concurrency

**Review Gates:**
- ✅ Actions queue per child (not hardcoded worker IDs)
- ✅ Worker pool processes actions concurrently
- ✅ HasActionInProgress() returns correct status
- ✅ Timeouts handled gracefully
- ✅ All tests passing

**Estimated LOC:** ~250 lines (executor + pool + tests)

---

#### **Week 7: Phase 2 Part 2 - Async Actions: Integration**

**Tasks:**
- Integrate ActionExecutor into Supervisor.Tick()
- Add action check before ticking each child
- Non-blocking tick loop (skip children with actions in progress)
- Tests for integration (infrastructure + actions)

**Dependencies:** Week 5 (needs circuit breaker), Week 6 (needs action executor)

**Parallel Work:** None (integration phase)

**Deliverables:**
- Updated `pkg/fsm/supervisor/tick.go` - Integrated action checks
- Tests for layered precedence (infrastructure > actions)

**Review Gates:**
- ✅ Infrastructure checks run FIRST (affect all children)
- ✅ Action checks run SECOND (affect individual children)
- ✅ Tick loop non-blocking (continues for children without actions)
- ✅ All tests passing

**Estimated LOC:** ~80 lines (integration + tests)

---

#### **Week 8: Phase 3 Part 1 - Templating & Variables (Part 2): Location Computation**

**Tasks:**
- Implement mergeLocations() (parent + child location maps)
- Implement computeLocationPath() (ISA-95 hierarchy with gap filling)
- Integrate location computation into parent workers
- Tests for location merging and path computation

**Dependencies:** Week 3 (needs VariableBundle)

**Parallel Work:** Can run in parallel with Week 7

**Deliverables:**
- `pkg/templating/location.go` - Location merging and path computation
- Tests for location hierarchy (enterprise.site.area.line.cell.bridge)

**Review Gates:**
- ✅ Parent location + child location = merged location
- ✅ Gap filling with "unknown" for missing levels
- ✅ Location path computed correctly (dot-separated)
- ✅ All tests passing

**Estimated LOC:** ~170 lines (location merge + path + tests)

---

#### **Week 9: Phase 4 Part 1 - Integration: Basic Tests**

**Tasks:**
- Combined tick loop with all features (infrastructure + actions + children + templates)
- Test parent → child variable flow (parent_state, location_path)
- Test template rendering in child workers (every tick)
- Basic integration tests (bridge → connection → benthos hierarchy)

**Dependencies:** All previous phases

**Parallel Work:** None (integration phase)

**Deliverables:**
- Integration tests covering full tick loop
- Tests for variable propagation through hierarchy
- Tests for template rendering with flattened variables

**Review Gates:**
- ✅ Full tick loop executes without errors
- ✅ Variables flow from parent to child
- ✅ Templates render correctly in child workers
- ✅ 3-level hierarchy works (grandparent → parent → child)
- ✅ All tests passing

**Estimated LOC:** ~150 lines (integration tests)

---

#### **Week 10: Phase 4 Part 2 - Integration: Edge Cases**

**Tasks:**
- Action behavior during child restart (action should pause)
- Observation collection during circuit open (continues)
- Template rendering performance tests (100+ workers)
- Variable flow edge cases (nil parent_observed, missing global vars)
- Location hierarchy edge cases (all levels missing)

**Dependencies:** Week 9 (needs basic integration)

**Parallel Work:** None (integration phase)

**Deliverables:**
- Edge case tests for all integration points
- Performance benchmarks for template rendering
- Tests for error handling (missing variables, template errors)

**Review Gates:**
- ✅ Actions pause during child restart
- ✅ Observations continue during circuit open
- ✅ Template rendering <100μs per worker
- ✅ Graceful degradation for missing variables
- ✅ All edge case tests passing

**Estimated LOC:** ~80 lines (edge case tests)

---

#### **Week 11: Phase 5 - Monitoring & Observability**

**Tasks:**
- Prometheus metrics (infrastructure + actions + children)
- Detailed logging (state transitions, action queuing, template rendering)
- Runbook for common issues
- Performance benchmarks (baseline for future optimization)

**Dependencies:** Week 10 (needs complete integration)

**Parallel Work:** None (monitoring after implementation)

**Deliverables:**
- `pkg/metrics/supervision.go` - Prometheus metrics
- `pkg/metrics/actions.go` - Action executor metrics
- `docs/runbook-fsmv2.md` - Operational runbook
- Benchmark results (template rendering, reconciliation, tick loop)

**Review Gates:**
- ✅ Metrics exposed for all key operations
- ✅ Logging covers state transitions and errors
- ✅ Runbook covers common scenarios
- ✅ Benchmarks establish baseline
- ✅ All tests passing

**Estimated LOC:** ~150 lines (metrics + logging)

---

#### **Week 12: Buffer, Documentation, Polish**

**Tasks:**
- Final documentation review (master plan update, design docs)
- Code cleanup (remove debug logging, optimize hot paths)
- Final integration testing (full system with 100+ workers)
- Migration guide (FSMv1 → FSMv2 patterns)
- Contingency for slippage from previous weeks

**Dependencies:** All previous phases

**Parallel Work:** None (final polish)

**Deliverables:**
- Updated master plan with all design decisions
- Migration guide for existing workers
- Final integration test suite
- Code ready for production

**Review Gates:**
- ✅ Documentation complete and accurate
- ✅ No TODOs or debug code left
- ✅ Full system tests passing
- ✅ Migration guide validated
- ✅ Ready for production deployment

---

### 6.3 Critical Path Analysis

**Longest dependency chain:**
```
Week 1 (ChildSpec)
  → Week 2 (reconcileChildren)
  → Week 3 (Templates)
  → Week 8 (Location)
  → Week 9 (Basic Integration)
  → Week 10 (Edge Cases)
  → Week 11 (Monitoring)
  → Week 12 (Polish)
```

**Total critical path:** 8 weeks (no parallelization possible on critical path)

**Parallel opportunities:**
- Week 4 (Backoff) can run parallel with Week 3 (Templates)
- Week 6 (Actions) can run parallel with Week 5 (Circuit Breaker)
- Week 8 (Location) can run parallel with Week 7 (Action Integration)

**Risk areas with no buffer:**
- Week 2: Blocks ALL other work (no parallelization)
- Week 9-10: Integration complexity may reveal issues in earlier phases
- Week 12: Contingency week (absorbs slippage)

**Optimistic timeline (with perfect execution):** 10 weeks
**Realistic timeline (accounting for integration issues):** 12 weeks

### 6.4 Resource Allocation

**Can one developer do all phases?**

Yes, but with caveats:
- **Weeks 1-2:** One developer sufficient (foundational, must get right)
- **Weeks 3-8:** One developer sufficient (clear scope, no blockers)
- **Weeks 9-10:** Integration may benefit from pairing (catch edge cases)
- **Weeks 11-12:** One developer sufficient (monitoring, polish)

**Where would 2 developers help?**

**High value:**
- Week 2 + Week 3 in parallel (reduce critical path by 1 week)
- Week 9-10 integration (one on tests, one on debugging)

**Medium value:**
- Week 4 + Week 5 in parallel (infrastructure supervision)
- Week 6 + Week 7 in parallel (async actions)

**Low value:**
- Week 1 (too small, coordination overhead not worth it)
- Week 11-12 (monitoring and polish, sequential work)

**Skill requirements per phase:**

**Week 1-2 (Hierarchical Composition):**
- Strong Go generics understanding (WorkerFactory)
- Kubernetes pattern familiarity (specs vs instances)
- Data structure design experience

**Week 3-8 (Templating & Variables):**
- Go text/template proficiency
- ISA-95 location hierarchy knowledge
- Template strict mode understanding

**Week 4-7 (Infrastructure & Actions):**
- Concurrency patterns (goroutine pools, channels)
- Circuit breaker pattern understanding
- Exponential backoff experience

**Week 9-10 (Integration):**
- Strong testing skills (edge cases, integration tests)
- Debugging complex state machines
- Performance testing experience

**Week 11-12 (Monitoring & Polish):**
- Prometheus metrics expertise
- Technical writing (runbooks, migration guides)
- Code review and optimization skills

**Recommended approach:**
- **Single developer:** 12 weeks (realistic with buffer)
- **Two developers:** 10 weeks (parallel critical path items)
- **Skill level:** Senior engineer (strong Go, FSM patterns, testing)

---

## Section 7: Updated Architecture Diagrams

### 7.1 Supervisor.Tick() Integration - BEFORE vs AFTER

#### **BEFORE (Current Master Plan)**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")  // ← Hardcoded child name
        time.Sleep(backoff)
        return nil
    }

    // PRIORITY 2: Per-Worker Actions
    for workerID := range s.workers {  // ← "workers" collection
        if s.actionExecutor.HasActionInProgress(workerID) {
            continue
        }
        
        // Derive and enqueue action
        snapshot, _ := s.store.LoadSnapshot(ctx, workerType, workerID)
        nextState, _, action := currentState.Next(snapshot)
        s.actionExecutor.EnqueueAction(workerID, action, registry)
    }
    
    return nil
}
```

**Issues with BEFORE:**
- ❌ Hardcoded child names ("dfc_read")
- ❌ No hierarchical composition (no ChildrenSpecs)
- ❌ No template rendering
- ❌ No variable propagation
- ❌ No StateMapping application
- ❌ "workers" collection instead of "children"

---

#### **AFTER (With Hierarchical Composition)**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // ====== PHASE 1: INFRASTRUCTURE HEALTH ======
    // (Affects all children)
    
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        
        // Restart specific child (via children map, not hardcoded)
        if child, exists := s.children["dfc_read"]; exists {
            child.supervisor.Restart()
        }
        
        // Skip entire tick (all children)
        return nil
    }
    
    s.circuitOpen = false
    
    // ====== PHASE 2: DERIVE DESIRED STATE ======
    // (NEW: Returns ChildrenSpecs)
    
    userSpec := s.triangularStore.GetUserSpec()
    
    // Inject Global variables (supervisor-level)
    userSpec.Variables.Global = s.globalVars
    
    // Inject Internal variables (supervisor-level)
    userSpec.Variables.Internal = map[string]any{
        "id":         s.supervisorID,
        "created_at": s.createdAt,
    }
    
    // Worker derives desired state (includes ChildrenSpecs!)
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // Persist desired state
    s.triangularStore.SetDesiredState(desiredState)
    
    // ====== PHASE 3: RECONCILE CHILDREN ======
    // (NEW: Add/update/remove children from specs)
    
    s.reconcileChildren(desiredState.ChildrenSpecs)
    
    // ====== PHASE 4: APPLY STATE MAPPING ======
    // (NEW: Map parent state to child desired state)
    
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }
    
    // ====== PHASE 5: TICK CHILDREN ======
    // (NEW: Check actions first, then recursive tick)
    
    for _, child := range s.children {
        // Per-child action check
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue  // Skip THIS child only
        }
        
        // Recursive tick (child supervisor has same structure)
        child.supervisor.Tick(ctx)
    }
    
    // ====== PHASE 6: COLLECT OBSERVED STATE ======
    
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)
    
    return nil
}
```

**Key differences in AFTER:**
- ✅ Children accessed via s.children map (not hardcoded)
- ✅ DeriveDesiredState returns ChildrenSpecs
- ✅ reconcileChildren() handles dynamic child creation
- ✅ StateMapping applied before child tick
- ✅ Global/Internal variables injected by supervisor
- ✅ Recursive tick (child supervisors have same structure)

---

### 7.2 DeriveDesiredState with Templates - Complete Flow

#### **UserSpec → Template Rendering → ChildrenSpecs**

```go
// ====== STEP 1: PARENT WORKER RECEIVES USERSPEC ======

type BridgeUserSpec struct {
    Name         string
    IPAddress    string
    Port         uint16
    TemplateRef  string
    Variables    VariableBundle
}

// UserSpec passed to parent worker:
userSpec := BridgeUserSpec{
    Name:        "Factory-PLC",
    IPAddress:   "192.168.1.100",
    Port:        502,
    TemplateRef: "modbus-tcp",
    Variables: VariableBundle{
        User: map[string]any{
            // From config.yaml
            "IP":   "192.168.1.100",
            "PORT": 502,
            
            // From agent location
            "location": map[string]string{
                "enterprise": "ACME",
                "site":       "Factory",
            },
        },
        Global: map[string]any{
            "kafka_brokers": "localhost:9092",
        },
        Internal: map[string]any{
            "id":         "bridge-123",
            "created_at": "2025-11-03T10:00:00Z",
        },
    },
}

// ====== STEP 2: PARENT WORKER MERGES LOCATION ======

func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) DesiredState {
    parentState := "active"
    
    // Merge parent + child location
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    // parentLocation = {"enterprise": "ACME", "site": "Factory"}
    
    childLocation := map[string]string{
        "bridge": userSpec.Name,  // "Factory-PLC"
    }
    
    location := mergeLocations(parentLocation, childLocation)
    // location = {
    //   "enterprise": "ACME",
    //   "site":       "Factory",
    //   "bridge":     "Factory-PLC",
    // }
    
    locationPath := computeLocationPath(location)
    // locationPath = "ACME.Factory.unknown.unknown.unknown.Factory-PLC"
    //                                ↑ gap filling for area/line/cell
    
    // ====== STEP 3: PARENT CREATES CHILDSPECS ======
    
    return DesiredState{
        State: parentState,
        ChildrenSpecs: []ChildSpec{
            // Child 1: Connection monitor
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: ConnectionUserSpec{
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state": parentState,  // Parent context
                        },
                    },
                },
                StateMapping: map[string]string{
                    "active": "up",    // Parent active → Child up
                    "idle":   "down",  // Parent idle → Child down
                },
            },
            
            // Child 2: Benthos data flow (with template)
            {
                Name:       "dfc_read",
                WorkerType: WorkerTypeBenthos,
                UserSpec: BenthosUserSpec{
                    TemplateName: userSpec.TemplateRef,  // "modbus-tcp"
                    Variables: VariableBundle{
                        User: map[string]any{
                            // Parent context
                            "parent_state": parentState,
                            
                            // Connection info (for template)
                            "IP":   userSpec.IPAddress,
                            "PORT": userSpec.Port,
                            
                            // Names
                            "name": userSpec.Name + "_read",  // "Factory-PLC_read"
                            
                            // Location
                            "location":      location,
                            "location_path": locationPath,
                        },
                        Global: userSpec.Variables.Global,  // Pass through
                    },
                },
                StateMapping: map[string]string{
                    "active": "running",  // Parent active → Benthos running
                    "idle":   "stopped",  // Parent idle → Benthos stopped
                },
            },
        },
    }
}
```

#### **STEP 4: CHILD WORKER RENDERS TEMPLATE**

```go
type BenthosWorker struct {
    logger        *zap.Logger
    templateStore TemplateStore  // Access to templates
    lastObserved  *ObservedState
}

type BenthosUserSpec struct {
    TemplateName string
    Variables    VariableBundle
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) DesiredState {
    // 1. Flatten variables for template access
    scope := userSpec.Variables.Flatten()
    
    // scope = {
    //   // User variables → TOP-LEVEL (no namespace)
    //   "parent_state":  "active",
    //   "IP":            "192.168.1.100",
    //   "PORT":          502,
    //   "name":          "Factory-PLC_read",
    //   "location_path": "ACME.Factory.unknown.unknown.unknown.Factory-PLC",
    //   "location": {
    //     "enterprise": "ACME",
    //     "site":       "Factory",
    //     "bridge":     "Factory-PLC",
    //   },
    //   
    //   // Global variables → NESTED under "global" key
    //   "global": {
    //     "kafka_brokers": "localhost:9092",
    //   },
    //   
    //   // Internal variables → NESTED under "internal" key
    //   "internal": {
    //     "id":         "dfc_read",
    //     "created_at": "2025-11-03T10:00:05Z",
    //   },
    // }
    
    // 2. Load template by name
    template := w.templateStore.Get(userSpec.TemplateName)
    
    // Template content (from config.yaml):
    // input:
    //   label: "modbus_{{ .name }}"
    //   modbus_tcp:
    //     address: "{{ .IP }}:{{ .PORT }}"
    // 
    // output:
    //   broker:
    //     outputs:
    //       - kafka:
    //           addresses: ["{{ .global.kafka_brokers }}"]
    //           topic: "umh.v1.{{ .location_path }}.{{ .name }}"
    
    // 3. Render template with flattened scope
    config, err := RenderTemplate(template, scope)
    if err != nil {
        w.logger.Error("Failed to render template", zap.Error(err))
        return DesiredState{State: "error"}
    }
    
    // Rendered config:
    // input:
    //   label: "modbus_Factory-PLC_read"
    //   modbus_tcp:
    //     address: "192.168.1.100:502"
    // 
    // output:
    //   broker:
    //     outputs:
    //       - kafka:
    //           addresses: ["localhost:9092"]
    //           topic: "umh.v1.ACME.Factory.unknown.unknown.unknown.Factory-PLC.Factory-PLC_read"
    
    // 4. Compute state from parent context
    parentState, _ := scope["parent_state"].(string)
    state := "stopped"
    if parentState == "active" {
        state = "running"
    }
    
    return DesiredState{
        State:  state,
        Config: config,  // Rendered benthos config
    }
}
```

**Key points:**
- ✅ Template rendering happens IN worker (not supervisor)
- ✅ Variables flattened: User variables → top-level, Global/Internal → nested
- ✅ Template access: `{{ .IP }}` not `{{ .user.IP }}`
- ✅ Location path auto-computed by parent
- ✅ Parent state passed via variable

---

### 7.3 Variable Propagation Through Hierarchy - 3 Levels

#### **Grandparent (Agent) → Parent (Bridge) → Child (Connection)**

```
┌─────────────────────────────────────────────────────────────────┐
│ GRANDPARENT: Agent                                               │
│                                                                   │
│ UserSpec:                                                         │
│   Variables:                                                      │
│     User:                                                         │
│       location: {enterprise: "ACME", site: "Factory"}            │
│     Global:                                                       │
│       kafka_brokers: "localhost:9092"                             │
│       api_endpoint: "https://api.umh.app"                         │
│     Internal: (injected by Agent supervisor)                      │
│       id: "agent-001"                                             │
│       created_at: "2025-11-03T09:00:00Z"                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ DeriveDesiredState()
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ PARENT: Bridge                                                   │
│                                                                   │
│ func (w *BridgeWorker) DeriveDesiredState(userSpec) {           │
│   // Merge location                                              │
│   parentLocation := userSpec.Variables.User["location"]          │
│   // {"enterprise": "ACME", "site": "Factory"}                   │
│                                                                   │
│   childLocation := {"bridge": "Factory-PLC"}                     │
│                                                                   │
│   location := mergeLocations(parentLocation, childLocation)      │
│   // {"enterprise": "ACME", "site": "Factory", "bridge": "..."}  │
│                                                                   │
│   locationPath := "ACME.Factory.unknown.unknown.unknown.Factory-PLC" │
│                                                                   │
│   return DesiredState{                                           │
│     State: "active",                                             │
│     ChildrenSpecs: [{                                            │
│       Name: "connection",                                        │
│       UserSpec: ConnectionUserSpec{                              │
│         Variables: VariableBundle{                               │
│           User: {                                                │
│             "parent_state": "active",        // NEW              │
│             "parent_observed": w.lastObserved, // Optional       │
│             "location": location,            // Merged           │
│             "location_path": locationPath,   // Computed         │
│             "IP": "192.168.1.100",           // From config      │
│           },                                                     │
│           Global: userSpec.Variables.Global, // Pass through     │
│         },                                                       │
│       },                                                         │
│     }],                                                          │
│   }                                                              │
│ }                                                                │
│                                                                   │
│ UserSpec (received from Agent):                                  │
│   Variables:                                                      │
│     User:                                                         │
│       location: {enterprise: "ACME", site: "Factory"}            │
│     Global:                                                       │
│       kafka_brokers: "localhost:9092"                             │
│       api_endpoint: "https://api.umh.app"                         │
│     Internal: (injected by Bridge supervisor)                     │
│       id: "bridge-123"                                            │
│       created_at: "2025-11-03T10:00:00Z"                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ reconcileChildren()
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ CHILD: Connection                                                │
│                                                                   │
│ UserSpec (received from Bridge):                                 │
│   IPAddress: "192.168.1.100"                                     │
│   Port: 502                                                      │
│   Variables:                                                      │
│     User:                                                         │
│       parent_state: "active"       // From parent                │
│       parent_observed: {...}       // Optional                   │
│       location: {                  // Merged by parent           │
│         enterprise: "ACME",                                      │
│         site: "Factory",                                         │
│         bridge: "Factory-PLC",                                   │
│       }                                                          │
│       location_path: "ACME.Factory.unknown.unknown.unknown.Factory-PLC" │
│       IP: "192.168.1.100"          // From connection config     │
│     Global:                        // Passed through             │
│       kafka_brokers: "localhost:9092"                             │
│       api_endpoint: "https://api.umh.app"                         │
│     Internal: (injected by Connection supervisor)                 │
│       id: "connection-456"                                        │
│       created_at: "2025-11-03T10:00:05Z"                          │
│                                                                   │
│ func (w *ConnectionWorker) DeriveDesiredState(userSpec) {       │
│   scope := userSpec.Variables.Flatten()                          │
│   // scope = {                                                   │
│   //   "parent_state": "active",              // Top-level       │
│   //   "IP": "192.168.1.100",                 // Top-level       │
│   //   "location_path": "ACME.Factory...",    // Top-level       │
│   //   "global": {kafka_brokers: "..."},      // Nested          │
│   //   "internal": {id: "..."},               // Nested          │
│   // }                                                           │
│                                                                   │
│   parentState := scope["parent_state"].(string)                  │
│                                                                   │
│   state := "down"                                                │
│   if parentState == "active" {                                   │
│     state = "up"                                                 │
│   }                                                              │
│                                                                   │
│   // Safety check from own observations                          │
│   if w.lastObserved != nil && !w.lastObserved.Connected {       │
│     state = "down"  // Override even if parent wants "up"        │
│   }                                                              │
│                                                                   │
│   return DesiredState{State: state}                              │
│ }                                                                │
└─────────────────────────────────────────────────────────────────┘
```

**Variable injection code:**

```go
// 1. USER VARIABLES: Parent constructs when creating ChildSpec
func (w *ParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        ChildrenSpecs: []ChildSpec{
            {
                UserSpec: UserSpec{
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":    "active",
                            "parent_observed": w.lastObserved,
                            "IP":              "192.168.1.100",
                        },
                    },
                },
            },
        },
    }
}

// 2. GLOBAL VARIABLES: Supervisor injects BEFORE calling DeriveDesiredState
func (s *Supervisor) Tick() {
    userSpec := s.triangularStore.GetUserSpec()
    
    userSpec.Variables.Global = s.globalVars  // Injected by supervisor
    
    desiredState := s.worker.DeriveDesiredState(userSpec)
}

// 3. INTERNAL VARIABLES: Supervisor injects BEFORE calling DeriveDesiredState
func (s *Supervisor) Tick() {
    userSpec := s.triangularStore.GetUserSpec()
    
    userSpec.Variables.Internal = map[string]any{
        "id":         s.supervisorID,
        "created_at": s.createdAt,
    }
    
    desiredState := s.worker.DeriveDesiredState(userSpec)
}
```

**Key points:**
- ✅ Variables flow downward (grandparent → parent → child)
- ✅ Location merges at each level (parent + child)
- ✅ Global variables passed through unchanged
- ✅ Internal variables unique per supervisor
- ✅ Each level flattens for template access

---

### 7.4 reconcileChildren() Flow - Add/Update/Remove Logic

```go
func (s *Supervisor) reconcileChildren(specs []ChildSpec) error {
    // ====== STEP 1: BUILD DESIRED MAP ======
    
    desiredChildren := make(map[string]ChildSpec)
    for _, spec := range specs {
        desiredChildren[spec.Name] = spec
    }
    
    // Example:
    // desiredChildren = {
    //   "connection":  ChildSpec{Name: "connection", WorkerType: "ConnectionWorker", ...},
    //   "dfc_read":    ChildSpec{Name: "dfc_read", WorkerType: "BenthosWorker", ...},
    // }
    
    // ====== STEP 2: ADD OR UPDATE CHILDREN ======
    
    for name, spec := range desiredChildren {
        if child, exists := s.children[name]; exists {
            // ────────────────────────────────────
            // Child EXISTS: Check if UserSpec changed
            // ────────────────────────────────────
            
            if !reflect.DeepEqual(child.userSpec, spec.UserSpec) {
                s.logger.Info("Child UserSpec changed, recreating",
                    "child", name,
                    "old_spec", child.userSpec,
                    "new_spec", spec.UserSpec,
                )
                
                // Shutdown old supervisor
                child.supervisor.Shutdown()
                
                // Create new worker from spec
                worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
                if err != nil {
                    s.logger.Error("Failed to create worker",
                        "child", name,
                        "worker_type", spec.WorkerType,
                        "error", err,
                    )
                    continue
                }
                
                // Create new child supervisor (with same factory!)
                childSupervisor := NewSupervisor(worker, s.factory, s.logger)
                
                // Update child instance
                child.supervisor = childSupervisor
                child.userSpec = spec.UserSpec
                child.stateMapping = spec.StateMapping
                
                s.logger.Info("Child recreated",
                    "child", name,
                )
            } else {
                // UserSpec unchanged, just update StateMapping
                child.stateMapping = spec.StateMapping
            }
        } else {
            // ────────────────────────────────────
            // Child DOES NOT EXIST: Create it
            // ────────────────────────────────────
            
            s.logger.Info("Adding child",
                "name", name,
                "worker_type", spec.WorkerType,
            )
            
            // Create worker from spec
            worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
            if err != nil {
                s.logger.Error("Failed to create worker",
                    "child", name,
                    "worker_type", spec.WorkerType,
                    "error", err,
                )
                continue
            }
            
            // Create child supervisor (with same factory!)
            supervisor := NewSupervisor(worker, s.factory, s.logger)
            
            // Add to children map
            s.children[name] = &childInstance{
                name:         name,
                supervisor:   supervisor,
                userSpec:     spec.UserSpec,
                stateMapping: spec.StateMapping,
            }
            
            s.logger.Info("Child added",
                "child", name,
            )
        }
    }
    
    // ====== STEP 3: REMOVE CHILDREN NOT IN SPECS ======
    
    for name := range s.children {
        if _, desired := desiredChildren[name]; !desired {
            s.logger.Info("Removing child",
                "name", name,
            )
            
            s.removeChild(name)
        }
    }
    
    return nil
}

func (s *Supervisor) removeChild(name string) {
    if child, exists := s.children[name]; exists {
        // Shutdown child supervisor
        child.supervisor.Shutdown()
        
        // Remove from map
        delete(s.children, name)
        
        s.logger.Info("Child removed",
            "child", name,
        )
    }
}
```

**Example scenarios:**

**ADD scenario:**
```
Current children: {}
Desired children: {"connection": ChildSpec{...}}

reconcileChildren() adds "connection" child
```

**UPDATE scenario:**
```
Current children: {"connection": ChildInstance{userSpec: {IP: "192.168.1.100"}}}
Desired children: {"connection": ChildSpec{UserSpec: {IP: "192.168.1.101"}}}

reconcileChildren() detects IP changed
  → Shuts down old supervisor
  → Creates new worker with new IP
  → Creates new supervisor
  → Replaces in s.children map
```

**REMOVE scenario:**
```
Current children: {"connection": ChildInstance{...}, "dfc_read": ChildInstance{...}}
Desired children: {"connection": ChildSpec{...}}

reconcileChildren() detects "dfc_read" not in desired
  → Calls removeChild("dfc_read")
  → Shuts down supervisor
  → Deletes from map
```

**NO CHANGE scenario:**
```
Current children: {"connection": ChildInstance{userSpec: {IP: "192.168.1.100"}}}
Desired children: {"connection": ChildSpec{UserSpec: {IP: "192.168.1.100"}}}

reconcileChildren() detects UserSpec unchanged
  → No action (child continues running)
```

---

## Summary

### Timeline Summary

**10-12 weeks** for complete FSMv2 implementation (vs. 6 weeks original estimate)

**Critical path:** Weeks 1-2 (hierarchical composition) → Week 9-10 (integration)

**Parallelization opportunities:** Weeks 3-8 (some phases can run in parallel)

**Resource recommendation:** One senior engineer (12 weeks) or two engineers (10 weeks)

### Architecture Summary

**Key changes from original master plan:**

1. **Hierarchical composition** - Parents declare ChildrenSpecs, supervisor reconciles
2. **Template rendering** - Distributed at worker level (not centralized)
3. **Variable propagation** - Three-tier namespace (User/Global/Internal)
4. **State control** - Hybrid StateMapping + child autonomy
5. **WorkerFactory** - String-based worker types for serialization

**Integration points:**
- Infrastructure Supervision (circuit breaker) still valid
- Async Action Executor still valid
- New hierarchical composition integrates cleanly with both

**No conflicts** - All new components integrate with existing patterns

---

**Ready for implementation** - Timeline and architecture fully defined.
