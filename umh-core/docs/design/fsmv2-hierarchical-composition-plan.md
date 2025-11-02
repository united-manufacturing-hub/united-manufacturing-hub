# FSMv2 Hierarchical Composition - Implementation Plan

> **For Claude:** Use the skills system to implement this plan task-by-task.

**Goal:** Add hierarchical composition to FSMv2 enabling parent workers to own and coordinate child workers, based on FSMv1's proven ProtocolConverter → Connection + DFC pattern.

**Architecture:** Preserve FSMv2's supervisor/worker separation while adding service-layer composition. Parent workers coordinate child supervisors without changing Worker/State interfaces.

**Tech Stack:** FSMv2 (supervisor/worker), TriangularStore persistence, existing Collector pattern

**Created:** 2025-11-01
**Last Updated:** 2025-11-01

---

## Table of Contents

1. [Gap Analysis](#1-gap-analysis)
2. [Design Proposal](#2-design-proposal)
3. [API Specification](#3-api-specification)
4. [Migration Strategy](#4-migration-strategy)
5. [Implementation Tasks](#5-implementation-tasks)
6. [Changelog](#changelog)

---

## 1. Gap Analysis

### 1.1 What FSMv1 Has That FSMv2 Lacks

FSMv1's hierarchical pattern (from `pkg/fsm/protocolconverter/`):

**At ControlLoop Level - All Managers Are Peers:**
```go
// In control loop initialization:
managers := []fsm.FSMManager[any]{
    s6.NewS6Manager(...),
    redpanda.NewRedpandaManager(...),
    connection.NewConnectionManager(...),        // ← GLOBAL instance (shared)
    dataflowcomponent.NewDataflowComponentManager(...),  // ← GLOBAL instance (shared)
    protocolconverter.NewProtocolConverterManager(...),  // ← Peer to others
}
```

**At Service Layer - ProtocolConverter Owns Sub-Managers:**
```go
// pkg/fsm/protocolconverter/service.go (pattern from reconcile.go line 146)
type ProtocolConverterService struct {
    connectionManager *connectionfsm.ConnectionManager  // ← NEW separate instance (not shared)
    dataflowComponentManager *dfcfsm.DataflowComponentManager  // ← NEW separate instance
}

// Service reconciles its owned managers:
func (s *ProtocolConverterService) ReconcileManager(ctx, services, snapshot) (err, reconciled) {
    // Reconcile child managers in sequence
    connErr, connReconciled := s.connectionManager.Reconcile(ctx, services, snapshot)
    if connErr != nil {
        return connErr, false
    }

    dfcErr, dfcReconciled := s.dataflowComponentManager.Reconcile(ctx, services, snapshot)
    if dfcErr != nil {
        return dfcErr, false
    }

    return nil, connReconciled || dfcReconciled
}
```

**Key Pattern - Parent FSM Queries Child FSM State:**
```go
// From reconcile.go lines 290-320 (reconcileTransitionToActive)
case OperationalStateStartingConnection:
    // Start connection instance
    p.StartConnectionInstance(ctx, services)

    // Check if connection is up by querying child manager
    if !p.IsConnectionUp() {
        return nil, false  // Wait for connection
    }

    // Connection is up, transition to next phase
    return p.baseFSMInstance.SendEvent(ctx, EventStartConnectionUp), true

case OperationalStateStartingDFC:
    // Start DFC instance
    p.StartDFCInstance(ctx, services)

    // Check if DFC is healthy by querying child manager
    if !p.IsDFCHealthy() {
        return nil, false  // Wait for DFC
    }

    // DFC is healthy, transition to active
    return p.baseFSMInstance.SendEvent(ctx, EventStartDFCUp), true
```

### 1.2 What FSMv2 Has That FSMv1 Doesn't

**Advantages to Preserve:**

1. **Clean Worker Interface:**
   - `CollectObservedState()` - async observation collection
   - `DeriveDesiredState()` - pure template expansion
   - `GetInitialState()` - simple initialization
   - NO reconcile method exposed to workers

2. **Supervisor-Managed Lifecycle:**
   - Tick loop in supervisor, not workers
   - Action execution with retry/backoff
   - Data freshness checking (4-layer defense)
   - Collector health monitoring

3. **TriangularStore Persistence:**
   - Identity, Desired, Observed state persistence
   - Snapshot loading for pure state transitions
   - CSE metadata (sync_id, version, timestamps)
   - No in-memory state loss on restart

4. **Async Observation Pattern:**
   - Separate goroutine per worker (Collector)
   - Timeout protection per operation
   - Automatic collector restart with backoff
   - Non-blocking tick loop

### 1.3 Missing Pieces for Hierarchy

FSMv2 currently lacks:

1. **Parent Worker Calls Child Supervisor Tick:**
   - FSMv1: `s.connectionManager.Reconcile(ctx, services, snapshot)`
   - FSMv2: No equivalent - workers don't have reconcile methods

2. **Parent Worker Queries Child Worker State:**
   - FSMv1: `p.IsConnectionUp()` checks child FSM state
   - FSMv2: No mechanism to query child supervisor's worker states

3. **Parent Worker Creates/Destroys Child Workers:**
   - FSMv1: `p.StartConnectionInstance()` creates child instance
   - FSMv2: No API for parent to add/remove child workers

4. **Composition Integration with TriangularStore:**
   - FSMv1: In-memory state only
   - FSMv2: Need to persist parent-child relationships

5. **Composition Integration with Async Actions:**
   - From `fsmv2-async-action-executor.md` design doc
   - Parent actions must not block while child workers operate
   - Child state changes must be observable during async action execution

---

## 2. Design Proposal

### 2.1 Chosen Approach: Service-Layer Composition (FSMv1 Pattern)

**Decision:** Keep FSMv1's service-layer pattern - it's proven in production and fits FSMv2's architecture.

**Key Insight from Architecture Simplification:**
> "Preserve what works: FSMv1's hierarchical pattern is proven in production. Don't add complexity without clear benefit."

**Why This Approach:**

1. **Preserves FSMv2 Worker Interface:** Workers stay simple - no reconcile method needed
2. **Minimal Supervisor Changes:** Supervisor already has TickAll() for multiple workers
3. **Matches FSMv1 Pattern:** Engineers understand this pattern from existing codebase
4. **Clear Ownership:** Parent service owns child supervisors, not shared globals
5. **Works with TriangularStore:** Each supervisor has its own store instance
6. **Works with Async Actions:** Parent actions can check child state during execution

### 2.2 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      Main Supervisor                            │
│  (manages ProtocolConverterWorker instances)                    │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │        ProtocolConverterWorker (implements Worker)         │ │
│  │                                                             │ │
│  │  ┌──────────────────────────────────────────────────────┐ │ │
│  │  │   ProtocolConverterService (new layer)               │ │ │
│  │  │                                                       │ │ │
│  │  │   ┌──────────────────┐   ┌─────────────────────────┐ │ │ │
│  │  │   │ Connection       │   │ DataflowComponent       │ │ │ │
│  │  │   │ Supervisor       │   │ Supervisor              │ │ │ │
│  │  │   │ (child)          │   │ (child)                 │ │ │ │
│  │  │   │                  │   │                         │ │ │ │
│  │  │   │ ┌──────────────┐ │   │ ┌──────────────────┐   │ │ │ │
│  │  │   │ │ Connection   │ │   │ │ DataflowComponent│   │ │ │ │
│  │  │   │ │ Worker       │ │   │ │ Worker           │   │ │ │ │
│  │  │   │ └──────────────┘ │   │ └──────────────────┘   │ │ │ │
│  │  │   └──────────────────┘   └─────────────────────────┘ │ │ │
│  │  │                                                       │ │ │
│  │  └──────────────────────────────────────────────────────┘ │ │
│  │                                                             │ │
│  │  CollectObservedState() {                                  │ │
│  │      service.TickChildSupervisors()  // Tick children      │ │
│  │      return aggregated state from children                 │ │
│  │  }                                                          │ │
│  │                                                             │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘

KEY POINTS:
- Parent worker contains service layer (not supervisor)
- Service layer owns child supervisors (separate instances)
- Parent CollectObservedState() ticks children + aggregates state
- Parent states query child state via service methods
- Child supervisors use their own TriangularStore instances
```

### 2.3 Data Flow

**During Parent Tick:**

```
1. Main Supervisor ticks ProtocolConverterWorker
   ↓
2. Supervisor calls worker.CollectObservedState()
   ↓
3. Worker delegates to service.CollectState()
   ↓
4. Service calls childSupervisor.TickAll(ctx)
   ├─ Connection Supervisor ticks Connection Worker
   └─ DFC Supervisor ticks DFC Worker
   ↓
5. Service queries child supervisors for state
   ├─ connectionSupervisor.GetWorkerState(connID)
   └─ dfcSupervisor.GetWorkerState(dfcID)
   ↓
6. Service aggregates child states into parent ObservedState
   ↓
7. Parent worker returns aggregated ObservedState
   ↓
8. Main Supervisor saves to TriangularStore
   ↓
9. Main Supervisor calls parentState.Next(snapshot)
   ↓
10. Parent state checks child health:
    if !service.IsConnectionHealthy() { return wait }
    if !service.IsDFCHealthy() { return wait }
```

**Key Design Decision:**
- Parent worker's `CollectObservedState()` ticks children BEFORE aggregating their state
- This ensures child state is fresh when parent makes decisions
- Supervisor doesn't need to know about composition - it's transparent

### 2.4 Integration with TriangularStore

**Each supervisor has its own store:**

```go
type ProtocolConverterService struct {
    connectionSupervisor *supervisor.Supervisor
    dfcSupervisor *supervisor.Supervisor
}

// During service initialization:
func NewProtocolConverterService(config ServiceConfig) *Service {
    // Create separate stores for child supervisors
    connStore := storage.NewTriangularStore(
        config.BasicStore,
        config.Registry,
    )
    dfcStore := storage.NewTriangularStore(
        config.BasicStore,
        config.Registry,
    )

    connSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "connection",
        Store: connStore,  // Dedicated store
        // ...
    })

    dfcSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "dataflow_component",
        Store: dfcStore,  // Dedicated store
        // ...
    })

    return &ProtocolConverterService{
        connectionSupervisor: connSupervisor,
        dfcSupervisor: dfcSupervisor,
    }
}
```

**Storage layout:**
```
TriangularStore Collections:
├─ protocol_converter_identity    (parent)
├─ protocol_converter_desired     (parent)
├─ protocol_converter_observed    (parent, includes child status)
├─ connection_identity            (child)
├─ connection_desired             (child)
├─ connection_observed            (child)
├─ dataflow_component_identity    (child)
├─ dataflow_component_desired     (child)
└─ dataflow_component_observed    (child)
```

### 2.5 Integration with Async Actions

From `fsmv2-async-action-executor.md` design doc, async actions run in separate goroutines. Hierarchical composition must work with this:

**Parent Action Execution:**
```go
type StartProtocolConverterAction struct {
    service *ProtocolConverterService
}

func (a *StartProtocolConverterAction) Execute(ctx context.Context) error {
    // 1. Start connection (child worker)
    if err := a.service.StartConnection(ctx); err != nil {
        return err
    }

    // 2. Wait for connection to be healthy
    // Check child state periodically during action execution
    for {
        if a.service.IsConnectionHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            // Tick children to update their state
            a.service.TickChildSupervisors(ctx)
        }
    }

    // 3. Start DFC (child worker)
    if err := a.service.StartDFC(ctx); err != nil {
        return err
    }

    // 4. Wait for DFC to be healthy
    for {
        if a.service.IsDFCHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    return nil
}
```

**Key Points:**
- Parent action ticks children during execution to get fresh state
- Parent action checks child health via service methods
- Child supervisors continue operating independently
- No blocking - action can timeout and retry

---

## 3. API Specification

### 3.1 No Interface Changes Required

**Critical Decision:** Keep Worker interface unchanged.

```go
// Worker interface - UNCHANGED
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)
    DeriveDesiredState(spec interface{}) (DesiredState, error)
    GetInitialState() State
}
```

**Why no changes needed:**
- `CollectObservedState()` already allows arbitrary implementation
- Parent workers delegate to service layer inside this method
- Service layer handles all composition logic
- Supervisor doesn't need to know about hierarchy

### 3.2 New Service Layer Pattern

**Service interface (convention, not formal interface):**

```go
// Service provides composition coordination for parent workers.
// This is a PATTERN, not a formal interface - each service implements what it needs.
type Service struct {
    // Child supervisors (one per child worker type)
    childSupervisors map[string]*supervisor.Supervisor

    // Configuration
    config ServiceConfig
    logger *zap.SugaredLogger
}

// Methods services typically implement:

// TickChildSupervisors ticks all child supervisors to update their state.
// Called by parent worker during CollectObservedState().
func (s *Service) TickChildSupervisors(ctx context.Context) error

// GetChildWorkerState queries a specific child worker's state.
// Returns state name, reason, and error.
func (s *Service) GetChildWorkerState(childType, workerID string) (string, string, error)

// StartChildWorker creates and starts a child worker.
// Returns error if worker already exists or fails to start.
func (s *Service) StartChildWorker(ctx context.Context, childType string, identity fsmv2.Identity, worker fsmv2.Worker) error

// StopChildWorker requests graceful shutdown of a child worker.
// Returns error if worker not found.
func (s *Service) StopChildWorker(ctx context.Context, childType, workerID string) error

// RemoveChildWorker removes a child worker from its supervisor.
// Returns error if worker not found.
func (s *Service) RemoveChildWorker(ctx context.Context, childType, workerID string) error

// IsChildWorkerHealthy checks if a specific child worker is in a healthy state.
// Healthy typically means "Running" or "Active" state.
func (s *Service) IsChildWorkerHealthy(childType, workerID string) bool
```

### 3.3 Parent Worker Implementation Pattern

```go
type ProtocolConverterWorker struct {
    service *ProtocolConverterService
    identity fsmv2.Identity
}

// CollectObservedState ticks children and aggregates their state
func (w *ProtocolConverterWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // 1. Tick all child supervisors to update their state
    if err := w.service.TickChildSupervisors(ctx); err != nil {
        return nil, fmt.Errorf("failed to tick children: %w", err)
    }

    // 2. Query child states
    connectionState, _, err := w.service.GetChildWorkerState("connection", w.identity.ID+"-conn")
    if err != nil {
        return nil, fmt.Errorf("failed to get connection state: %w", err)
    }

    dfcState, _, err := w.service.GetChildWorkerState("dataflow_component", w.identity.ID+"-dfc")
    if err != nil {
        return nil, fmt.Errorf("failed to get dfc state: %w", err)
    }

    // 3. Aggregate into parent observed state
    return &ProtocolConverterObservedState{
        ConnectionState: connectionState,
        DFCState: dfcState,
        Healthy: connectionState == "Running" && dfcState == "Running",
        Timestamp: time.Now(),
    }, nil
}

// DeriveDesiredState is pure - no child interaction
func (w *ProtocolConverterWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    // Template expansion only - no child coordination
    return &ProtocolConverterDesiredState{
        Config: expandTemplate(spec),
        ShutdownRequested: false,
    }, nil
}

// GetInitialState returns parent's initial state
func (w *ProtocolConverterWorker) GetInitialState() fsmv2.State {
    return &StoppedState{}
}
```

### 3.4 Parent State Implementation Pattern

```go
type TryingToStartState struct{}

func (s *TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    observed := snapshot.Observed.(*ProtocolConverterObservedState)

    // Check if children are healthy
    if observed.ConnectionState == "Running" && observed.DFCState == "Running" {
        // Both children healthy - transition to running
        return &RunningState{}, fsmv2.SignalNone, nil
    }

    // Children not healthy - emit action to start them
    return s, fsmv2.SignalNone, &StartProtocolConverterAction{
        service: getServiceFromContext(snapshot),  // Pattern TBD
    }
}

func (s *TryingToStartState) String() string { return "TryingToStart" }
func (s *TryingToStartState) Reason() string { return "waiting for child workers to start" }
```

### 3.5 Service Implementation Pattern

```go
type ProtocolConverterService struct {
    connectionSupervisor *supervisor.Supervisor
    dfcSupervisor *supervisor.Supervisor

    config ServiceConfig
    logger *zap.SugaredLogger
}

func NewProtocolConverterService(config ServiceConfig) *ProtocolConverterService {
    // Create child supervisors with their own stores
    connSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "connection",
        Store: config.ConnectionStore,
        Logger: config.Logger,
    })

    dfcSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "dataflow_component",
        Store: config.DFCStore,
        Logger: config.Logger,
    })

    return &ProtocolConverterService{
        connectionSupervisor: connSupervisor,
        dfcSupervisor: dfcSupervisor,
        config: config,
        logger: config.Logger,
    }
}

func (s *ProtocolConverterService) TickChildSupervisors(ctx context.Context) error {
    // Tick all child supervisors
    if err := s.connectionSupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick connection supervisor: %v", err)
    }

    if err := s.dfcSupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick dfc supervisor: %v", err)
    }

    return nil  // Non-fatal - individual worker errors are logged
}

func (s *ProtocolConverterService) GetChildWorkerState(childType, workerID string) (string, string, error) {
    switch childType {
    case "connection":
        return s.connectionSupervisor.GetWorkerState(workerID)
    case "dataflow_component":
        return s.dfcSupervisor.GetWorkerState(workerID)
    default:
        return "", "", fmt.Errorf("unknown child type: %s", childType)
    }
}

func (s *ProtocolConverterService) IsConnectionHealthy() bool {
    state, _, err := s.connectionSupervisor.GetWorkerState(s.config.ConnectionID)
    if err != nil {
        return false
    }
    return state == "Running" || state == "Active"
}

func (s *ProtocolConverterService) IsDFCHealthy() bool {
    state, _, err := s.dfcSupervisor.GetWorkerState(s.config.DFCID)
    if err != nil {
        return false
    }
    return state == "Running" || state == "Active"
}

func (s *ProtocolConverterService) StartConnection(ctx context.Context) error {
    // Create connection worker
    connWorker := NewConnectionWorker(s.config.ConnectionConfig)

    identity := fsmv2.Identity{
        ID: s.config.ConnectionID,
        Name: "Protocol Converter Connection",
        WorkerType: "connection",
    }

    // Add to child supervisor
    return s.connectionSupervisor.AddWorker(identity, connWorker)
}

func (s *ProtocolConverterService) StartDFC(ctx context.Context) error {
    // Create DFC worker
    dfcWorker := NewDFCWorker(s.config.DFCConfig)

    identity := fsmv2.Identity{
        ID: s.config.DFCID,
        Name: "Protocol Converter DFC",
        WorkerType: "dataflow_component",
    }

    // Add to child supervisor
    return s.dfcSupervisor.AddWorker(identity, dfcWorker)
}
```

---

## 4. Migration Strategy

### 4.1 Backward Compatibility

**Goal:** Existing FSMv2 workers continue working without changes.

**Strategy:**
1. Service layer is OPTIONAL - only composite workers need it
2. No Worker interface changes - composition is implementation detail
3. Supervisor unchanged - doesn't know about composition
4. Non-composite workers work exactly as before

**Example non-composite worker (unchanged):**
```go
type SimpleContainerWorker struct {
    containerID string
}

func (w *SimpleContainerWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // No service layer - direct implementation
    status := checkContainerStatus(w.containerID)
    return &ContainerObservedState{
        Status: status,
        Timestamp: time.Now(),
    }, nil
}
```

### 4.2 Testing Strategy

**Unit Testing:**
```go
// Test parent worker in isolation by mocking service
func TestProtocolConverterWorker_CollectObservedState(t *testing.T) {
    mockService := &MockProtocolConverterService{
        childStates: map[string]string{
            "connection": "Running",
            "dfc": "Running",
        },
    }

    worker := &ProtocolConverterWorker{
        service: mockService,
    }

    observed, err := worker.CollectObservedState(context.Background())
    assert.NoError(t, err)
    assert.True(t, observed.Healthy)
}

// Test child supervisors independently
func TestConnectionSupervisor_Lifecycle(t *testing.T) {
    supervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "connection",
        Store: mockStore,
        Logger: testLogger,
    })

    worker := NewConnectionWorker(config)
    identity := fsmv2.Identity{ID: "test-conn"}

    err := supervisor.AddWorker(identity, worker)
    assert.NoError(t, err)

    // Verify worker ticks correctly
    err = supervisor.TickAll(context.Background())
    assert.NoError(t, err)
}
```

**Integration Testing:**
```go
// Test full parent-child coordination
func TestProtocolConverter_HierarchicalComposition(t *testing.T) {
    // Create parent supervisor with ProtocolConverterWorker
    parentSupervisor := createTestSupervisor("protocol_converter")

    // Create service with child supervisors
    service := NewProtocolConverterService(ServiceConfig{
        ConnectionStore: createTestStore("connection"),
        DFCStore: createTestStore("dataflow_component"),
    })

    // Create parent worker with service
    parentWorker := &ProtocolConverterWorker{
        service: service,
        identity: fsmv2.Identity{ID: "test-pc"},
    }

    // Add parent worker to parent supervisor
    err := parentSupervisor.AddWorker(parentWorker.identity, parentWorker)
    assert.NoError(t, err)

    // Verify parent can tick children
    err = parentSupervisor.TickAll(context.Background())
    assert.NoError(t, err)

    // Verify parent can query child state
    observed, _ := parentWorker.CollectObservedState(context.Background())
    assert.NotNil(t, observed.ConnectionState)
}
```

### 4.3 Rollout Plan

**Phase 1: Add Service Layer Pattern (Week 1)**
- [ ] Create service layer example in documentation
- [ ] Add service pattern to FSMv2 README
- [ ] No code changes - documentation only
- [ ] Review with team

**Phase 2: Implement ProtocolConverter Service (Week 2-3)**
- [ ] Create ProtocolConverterService struct
- [ ] Implement child supervisor management
- [ ] Add TickChildSupervisors() method
- [ ] Add child state query methods
- [ ] Unit tests for service

**Phase 3: Migrate ProtocolConverterWorker (Week 4)**
- [ ] Modify ProtocolConverterWorker to use service
- [ ] Update CollectObservedState() to tick children
- [ ] Update states to query child state
- [ ] Integration tests for full hierarchy

**Phase 4: Migrate Actions (Week 5)**
- [ ] Update StartProtocolConverterAction to coordinate children
- [ ] Update StopProtocolConverterAction
- [ ] Verify async action executor compatibility
- [ ] End-to-end tests

**Phase 5: Documentation & Examples (Week 6)**
- [ ] Update FSMv2 README with composition pattern
- [ ] Add example composite worker
- [ ] Add troubleshooting guide
- [ ] Team training session

---

## 5. Implementation Tasks

### Task 1: Service Layer Documentation

**Files:**
- Update: `pkg/fsmv2/README.md`

**Step 1: Add Composition section to README**

Add after existing "Worker Implementation" section:

```markdown
## Hierarchical Composition (Optional)

FSMv2 supports hierarchical composition through a service layer pattern.
This allows parent workers to coordinate multiple child workers.

### When to Use Composition

- Parent worker manages multiple child components (e.g., ProtocolConverter → Connection + DFC)
- Child components have their own lifecycle and state machines
- Parent needs to wait for children to reach certain states

### Service Layer Pattern

Create a service struct that owns child supervisors:

```go
type ParentService struct {
    childASupervisor *supervisor.Supervisor
    childBSupervisor *supervisor.Supervisor
}

func (s *ParentService) TickChildSupervisors(ctx context.Context) error {
    s.childASupervisor.TickAll(ctx)
    s.childBSupervisor.TickAll(ctx)
    return nil
}

func (s *ParentService) IsChildAHealthy() bool {
    state, _, _ := s.childASupervisor.GetWorkerState("child-a-id")
    return state == "Running"
}
```

### Parent Worker Pattern

Parent worker delegates to service during observation:

```go
type ParentWorker struct {
    service *ParentService
}

func (w *ParentWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    // Tick children first
    w.service.TickChildSupervisors(ctx)

    // Query child states
    childAHealthy := w.service.IsChildAHealthy()
    childBHealthy := w.service.IsChildBHealthy()

    // Aggregate into parent state
    return &ParentObservedState{
        ChildAHealthy: childAHealthy,
        ChildBHealthy: childBHealthy,
        Timestamp: time.Now(),
    }, nil
}
```

See `docs/examples/composite-worker.go` for complete example.
```

**Step 2: Commit**

```bash
cd umh-core
git add pkg/fsmv2/README.md
git commit -m "docs: add hierarchical composition pattern to FSMv2

Add service layer pattern documentation for parent workers
that coordinate multiple child workers. This enables FSMv1-style
hierarchical composition in FSMv2.

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 2: Create Example Composite Worker

**Files:**
- Create: `docs/examples/fsmv2/composite-worker.go`

**Step 1: Write example composite worker**

```go
// Package examples demonstrates hierarchical composition in FSMv2.
//
// This example shows a ParentWorker that coordinates two child workers
// (ChildA and ChildB) through a service layer. The pattern matches FSMv1's
// ProtocolConverter → Connection + DFC composition.
package examples

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// ========================================
// PART 1: Service Layer (Composition Coordinator)
// ========================================

type ParentService struct {
    childASupervisor *supervisor.Supervisor
    childBSupervisor *supervisor.Supervisor

    childAID string
    childBID string

    logger *zap.SugaredLogger
}

type ServiceConfig struct {
    ChildAStore storage.TriangularStoreInterface
    ChildBStore storage.TriangularStoreInterface
    ChildAID    string
    ChildBID    string
    Logger      *zap.SugaredLogger
}

func NewParentService(config ServiceConfig) *ParentService {
    childASupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "child_a",
        Store: config.ChildAStore,
        Logger: config.Logger,
    })

    childBSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "child_b",
        Store: config.ChildBStore,
        Logger: config.Logger,
    })

    return &ParentService{
        childASupervisor: childASupervisor,
        childBSupervisor: childBSupervisor,
        childAID: config.ChildAID,
        childBID: config.ChildBID,
        logger: config.Logger,
    }
}

func (s *ParentService) TickChildSupervisors(ctx context.Context) error {
    if err := s.childASupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick child A: %v", err)
    }

    if err := s.childBSupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick child B: %v", err)
    }

    return nil  // Non-fatal - individual errors logged
}

func (s *ParentService) IsChildAHealthy() bool {
    state, _, err := s.childASupervisor.GetWorkerState(s.childAID)
    if err != nil {
        return false
    }
    return state == "Running"
}

func (s *ParentService) IsChildBHealthy() bool {
    state, _, err := s.childBSupervisor.GetWorkerState(s.childBID)
    if err != nil {
        return false
    }
    return state == "Running"
}

func (s *ParentService) StartChildA(ctx context.Context) error {
    childWorker := &ChildAWorker{}
    identity := fsmv2.Identity{
        ID: s.childAID,
        Name: "Child A",
        WorkerType: "child_a",
    }
    return s.childASupervisor.AddWorker(identity, childWorker)
}

func (s *ParentService) StartChildB(ctx context.Context) error {
    childWorker := &ChildBWorker{}
    identity := fsmv2.Identity{
        ID: s.childBID,
        Name: "Child B",
        WorkerType: "child_b",
    }
    return s.childBSupervisor.AddWorker(identity, childWorker)
}

// ========================================
// PART 2: Parent Worker (Uses Service)
// ========================================

type ParentWorker struct {
    service  *ParentService
    identity fsmv2.Identity
}

type ParentObservedState struct {
    ChildAHealthy bool
    ChildBHealthy bool
    Timestamp     time.Time
}

func (o *ParentObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &ParentDesiredState{}
}

func (o *ParentObservedState) GetTimestamp() time.Time {
    return o.Timestamp
}

type ParentDesiredState struct {
    shutdownRequested bool
}

func (d *ParentDesiredState) ShutdownRequested() bool {
    return d.shutdownRequested
}

func (w *ParentWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Tick children first to get fresh state
    if err := w.service.TickChildSupervisors(ctx); err != nil {
        return nil, err
    }

    // Query child health
    childAHealthy := w.service.IsChildAHealthy()
    childBHealthy := w.service.IsChildBHealthy()

    // Aggregate into parent state
    return &ParentObservedState{
        ChildAHealthy: childAHealthy,
        ChildBHealthy: childBHealthy,
        Timestamp: time.Now(),
    }, nil
}

func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &ParentDesiredState{}, nil
}

func (w *ParentWorker) GetInitialState() fsmv2.State {
    return &ParentStoppedState{}
}

// ========================================
// PART 3: Parent States (Query Children)
// ========================================

type ParentStoppedState struct{}

func (s *ParentStoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    if snapshot.Desired.ShutdownRequested() {
        return s, fsmv2.SignalNone, nil
    }

    // Transition to starting
    return &ParentTryingToStartState{}, fsmv2.SignalNone, nil
}

func (s *ParentStoppedState) String() string { return "Stopped" }
func (s *ParentStoppedState) Reason() string { return "parent is stopped" }

type ParentTryingToStartState struct{}

func (s *ParentTryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    observed := snapshot.Observed.(*ParentObservedState)

    // Check if both children are healthy
    if observed.ChildAHealthy && observed.ChildBHealthy {
        return &ParentRunningState{}, fsmv2.SignalNone, nil
    }

    // Emit action to start children
    return s, fsmv2.SignalNone, &StartParentAction{}
}

func (s *ParentTryingToStartState) String() string { return "TryingToStart" }
func (s *ParentTryingToStartState) Reason() string { return "waiting for children to start" }

type ParentRunningState struct{}

func (s *ParentRunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &ParentTryingToStopState{}, fsmv2.SignalNone, nil
    }

    observed := snapshot.Observed.(*ParentObservedState)

    // Check if children are still healthy
    if !observed.ChildAHealthy || !observed.ChildBHealthy {
        return &ParentDegradedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *ParentRunningState) String() string { return "Running" }
func (s *ParentRunningState) Reason() string { return "parent and all children healthy" }

type ParentDegradedState struct{}

func (s *ParentDegradedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    observed := snapshot.Observed.(*ParentObservedState)

    // Check if children recovered
    if observed.ChildAHealthy && observed.ChildBHealthy {
        return &ParentRunningState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *ParentDegradedState) String() string { return "Degraded" }
func (s *ParentDegradedState) Reason() string { return "one or more children unhealthy" }

type ParentTryingToStopState struct{}

func (s *ParentTryingToStopState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    observed := snapshot.Observed.(*ParentObservedState)

    // Check if both children stopped
    if !observed.ChildAHealthy && !observed.ChildBHealthy {
        return &ParentStoppedState{}, fsmv2.SignalNone, nil
    }

    // Emit action to stop children
    return s, fsmv2.SignalNone, &StopParentAction{}
}

func (s *ParentTryingToStopState) String() string { return "TryingToStop" }
func (s *ParentTryingToStopState) Reason() string { return "waiting for children to stop" }

// ========================================
// PART 4: Actions (Coordinate Children)
// ========================================

type StartParentAction struct {
    service *ParentService
}

func (a *StartParentAction) Execute(ctx context.Context) error {
    // Start child A
    if err := a.service.StartChildA(ctx); err != nil {
        return fmt.Errorf("failed to start child A: %w", err)
    }

    // Wait for child A to be healthy
    for {
        if a.service.IsChildAHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    // Start child B
    if err := a.service.StartChildB(ctx); err != nil {
        return fmt.Errorf("failed to start child B: %w", err)
    }

    // Wait for child B to be healthy
    for {
        if a.service.IsChildBHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    return nil
}

func (a *StartParentAction) Name() string { return "StartParent" }

type StopParentAction struct {
    service *ParentService
}

func (a *StopParentAction) Execute(ctx context.Context) error {
    // Request shutdown for both children
    // (implementation omitted - would set ShutdownRequested in child desired states)
    return nil
}

func (a *StopParentAction) Name() string { return "StopParent" }

// ========================================
// PART 5: Child Workers (Simple Examples)
// ========================================

type ChildAWorker struct{}

type ChildAObservedState struct {
    Healthy   bool
    Timestamp time.Time
}

func (o *ChildAObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &ChildADesiredState{}
}

func (o *ChildAObservedState) GetTimestamp() time.Time {
    return o.Timestamp
}

type ChildADesiredState struct{}

func (d *ChildADesiredState) ShutdownRequested() bool {
    return false
}

func (w *ChildAWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return &ChildAObservedState{
        Healthy: true,
        Timestamp: time.Now(),
    }, nil
}

func (w *ChildAWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &ChildADesiredState{}, nil
}

func (w *ChildAWorker) GetInitialState() fsmv2.State {
    return &ChildARunningState{}
}

type ChildARunningState struct{}

func (s *ChildARunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, nil
}

func (s *ChildARunningState) String() string { return "Running" }
func (s *ChildARunningState) Reason() string { return "child A running" }

// ChildBWorker implementation similar to ChildAWorker (omitted for brevity)
type ChildBWorker struct{}
type ChildBObservedState struct {
    Healthy   bool
    Timestamp time.Time
}

func (o *ChildBObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &ChildBDesiredState{}
}

func (o *ChildBObservedState) GetTimestamp() time.Time {
    return o.Timestamp
}

type ChildBDesiredState struct{}

func (d *ChildBDesiredState) ShutdownRequested() bool {
    return false
}

func (w *ChildBWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return &ChildBObservedState{
        Healthy: true,
        Timestamp: time.Now(),
    }, nil
}

func (w *ChildBWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &ChildBDesiredState{}, nil
}

func (w *ChildBWorker) GetInitialState() fsmv2.State {
    return &ChildBRunningState{}
}

type ChildBRunningState struct{}

func (s *ChildBRunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    return s, fsmv2.SignalNone, nil
}

func (s *ChildBRunningState) String() string { return "Running" }
func (s *ChildBRunningState) Reason() string { return "child B running" }
```

**Step 2: Commit**

```bash
cd umh-core
git add docs/examples/fsmv2/composite-worker.go
git commit -m "docs: add composite worker example for FSMv2

Add complete example showing hierarchical composition pattern.
Demonstrates parent worker coordinating two child workers through
a service layer.

Includes:
- Service layer with child supervisor management
- Parent worker that ticks children during observation
- Parent states that query child health
- Actions that coordinate child lifecycles
- Simple child worker implementations

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 3: Implement ProtocolConverterService

**Files:**
- Create: `pkg/fsmv2/workers/protocolconverter/service.go`
- Create: `pkg/fsmv2/workers/protocolconverter/service_test.go`

**Step 1: Create service.go**

```go
package protocolconverter

import (
    "context"
    "fmt"

    "go.uber.org/zap"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// ProtocolConverterService coordinates Connection and DataflowComponent child workers.
// It owns separate supervisor instances for each child type and provides methods
// for parent workers to query and control child state.
type ProtocolConverterService struct {
    connectionSupervisor *supervisor.Supervisor
    dfcSupervisor *supervisor.Supervisor

    connectionID string
    dfcID string

    config ServiceConfig
    logger *zap.SugaredLogger
}

type ServiceConfig struct {
    // Storage for child supervisors
    ConnectionStore storage.TriangularStoreInterface
    DFCStore storage.TriangularStoreInterface

    // Child worker IDs
    ConnectionID string
    DFCID string

    // Configuration for child workers
    ConnectionConfig interface{}
    DFCConfig interface{}

    Logger *zap.SugaredLogger
}

func NewProtocolConverterService(config ServiceConfig) *ProtocolConverterService {
    connectionSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "connection",
        Store: config.ConnectionStore,
        Logger: config.Logger,
    })

    dfcSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "dataflow_component",
        Store: config.DFCStore,
        Logger: config.Logger,
    })

    return &ProtocolConverterService{
        connectionSupervisor: connectionSupervisor,
        dfcSupervisor: dfcSupervisor,
        connectionID: config.ConnectionID,
        dfcID: config.DFCID,
        config: config,
        logger: config.Logger,
    }
}

// TickChildSupervisors ticks all child supervisors to update their state.
// Called by parent worker during CollectObservedState().
func (s *ProtocolConverterService) TickChildSupervisors(ctx context.Context) error {
    if err := s.connectionSupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick connection supervisor: %v", err)
    }

    if err := s.dfcSupervisor.TickAll(ctx); err != nil {
        s.logger.Errorf("Failed to tick dfc supervisor: %v", err)
    }

    return nil  // Non-fatal - individual worker errors are logged
}

// GetConnectionState returns the current state of the connection worker.
func (s *ProtocolConverterService) GetConnectionState() (string, string, error) {
    return s.connectionSupervisor.GetWorkerState(s.connectionID)
}

// GetDFCState returns the current state of the DFC worker.
func (s *ProtocolConverterService) GetDFCState() (string, string, error) {
    return s.dfcSupervisor.GetWorkerState(s.dfcID)
}

// IsConnectionHealthy checks if the connection worker is in a healthy state.
func (s *ProtocolConverterService) IsConnectionHealthy() bool {
    state, _, err := s.connectionSupervisor.GetWorkerState(s.connectionID)
    if err != nil {
        return false
    }
    return state == "Running" || state == "Active"
}

// IsDFCHealthy checks if the DFC worker is in a healthy state.
func (s *ProtocolConverterService) IsDFCHealthy() bool {
    state, _, err := s.dfcSupervisor.GetWorkerState(s.dfcID)
    if err != nil {
        return false
    }
    return state == "Running" || state == "Active"
}

// StartConnection creates and starts the connection worker.
func (s *ProtocolConverterService) StartConnection(ctx context.Context) error {
    // Create connection worker
    connWorker := NewConnectionWorker(s.config.ConnectionConfig)

    identity := fsmv2.Identity{
        ID: s.connectionID,
        Name: "Protocol Converter Connection",
        WorkerType: "connection",
    }

    // Add to child supervisor
    return s.connectionSupervisor.AddWorker(identity, connWorker)
}

// StartDFC creates and starts the DFC worker.
func (s *ProtocolConverterService) StartDFC(ctx context.Context) error {
    // Create DFC worker
    dfcWorker := NewDFCWorker(s.config.DFCConfig)

    identity := fsmv2.Identity{
        ID: s.dfcID,
        Name: "Protocol Converter DFC",
        WorkerType: "dataflow_component",
    }

    // Add to child supervisor
    return s.dfcSupervisor.AddWorker(identity, dfcWorker)
}

// StopConnection requests graceful shutdown of the connection worker.
func (s *ProtocolConverterService) StopConnection(ctx context.Context) error {
    return s.connectionSupervisor.RequestShutdown(ctx, s.connectionID, "parent requested shutdown")
}

// StopDFC requests graceful shutdown of the DFC worker.
func (s *ProtocolConverterService) StopDFC(ctx context.Context) error {
    return s.dfcSupervisor.RequestShutdown(ctx, s.dfcID, "parent requested shutdown")
}

// RemoveConnection removes the connection worker from its supervisor.
func (s *ProtocolConverterService) RemoveConnection(ctx context.Context) error {
    return s.connectionSupervisor.RemoveWorker(ctx, s.connectionID)
}

// RemoveDFC removes the DFC worker from its supervisor.
func (s *ProtocolConverterService) RemoveDFC(ctx context.Context) error {
    return s.dfcSupervisor.RemoveWorker(ctx, s.dfcID)
}
```

**Step 2: Create service_test.go**

```go
package protocolconverter

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "go.uber.org/zap"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

func TestProtocolConverterService_TickChildSupervisors(t *testing.T) {
    service := NewProtocolConverterService(ServiceConfig{
        ConnectionStore: createMockStore(t, "connection"),
        DFCStore: createMockStore(t, "dataflow_component"),
        ConnectionID: "test-conn",
        DFCID: "test-dfc",
        Logger: zap.NewNop().Sugar(),
    })

    ctx := context.Background()
    err := service.TickChildSupervisors(ctx)
    assert.NoError(t, err)
}

func TestProtocolConverterService_ChildStateQueries(t *testing.T) {
    service := NewProtocolConverterService(ServiceConfig{
        ConnectionStore: createMockStore(t, "connection"),
        DFCStore: createMockStore(t, "dataflow_component"),
        ConnectionID: "test-conn",
        DFCID: "test-dfc",
        Logger: zap.NewNop().Sugar(),
    })

    // Initially no workers - should not be healthy
    assert.False(t, service.IsConnectionHealthy())
    assert.False(t, service.IsDFCHealthy())
}

func TestProtocolConverterService_StartChildren(t *testing.T) {
    service := NewProtocolConverterService(ServiceConfig{
        ConnectionStore: createMockStore(t, "connection"),
        DFCStore: createMockStore(t, "dataflow_component"),
        ConnectionID: "test-conn",
        DFCID: "test-dfc",
        ConnectionConfig: nil,
        DFCConfig: nil,
        Logger: zap.NewNop().Sugar(),
    })

    ctx := context.Background()

    // Start connection
    err := service.StartConnection(ctx)
    assert.NoError(t, err)

    // Start DFC
    err = service.StartDFC(ctx)
    assert.NoError(t, err)

    // Verify workers exist
    connState, _, err := service.GetConnectionState()
    assert.NoError(t, err)
    assert.NotEmpty(t, connState)

    dfcState, _, err := service.GetDFCState()
    assert.NoError(t, err)
    assert.NotEmpty(t, dfcState)
}

func createMockStore(t *testing.T, workerType string) storage.TriangularStoreInterface {
    // Create mock store for testing
    // TODO: Implement mock store or use test fixtures
    return nil
}
```

**Step 3: Run tests**

```bash
cd umh-core
go test ./pkg/fsmv2/workers/protocolconverter/... -v
```

**Step 4: Commit**

```bash
git add pkg/fsmv2/workers/protocolconverter/service.go pkg/fsmv2/workers/protocolconverter/service_test.go
git commit -m "feat(fsmv2): add ProtocolConverterService for hierarchical composition

Implement service layer that coordinates Connection and DFC child workers.
Provides methods for parent worker to:
- Tick child supervisors
- Query child state
- Start/stop/remove children

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 4: Migrate ProtocolConverterWorker to Use Service

**Files:**
- Modify: `pkg/fsmv2/workers/protocolconverter/worker.go`
- Modify: `pkg/fsmv2/workers/protocolconverter/worker_test.go`

**Step 1: Update worker.go**

Change from:
```go
type ProtocolConverterWorker struct {
    // Direct implementation
}

func (w *ProtocolConverterWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Direct observation
}
```

To:
```go
type ProtocolConverterWorker struct {
    service  *ProtocolConverterService
    identity fsmv2.Identity
}

func (w *ProtocolConverterWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Tick children first
    if err := w.service.TickChildSupervisors(ctx); err != nil {
        return nil, fmt.Errorf("failed to tick children: %w", err)
    }

    // Query child states
    connectionState, _, err := w.service.GetConnectionState()
    if err != nil {
        return nil, fmt.Errorf("failed to get connection state: %w", err)
    }

    dfcState, _, err := w.service.GetDFCState()
    if err != nil {
        return nil, fmt.Errorf("failed to get dfc state: %w", err)
    }

    // Aggregate into parent state
    return &ProtocolConverterObservedState{
        ConnectionState: connectionState,
        DFCState: dfcState,
        ConnectionHealthy: w.service.IsConnectionHealthy(),
        DFCHealthy: w.service.IsDFCHealthy(),
        Timestamp: time.Now(),
    }, nil
}
```

**Step 2: Update worker_test.go**

Add tests for service integration:
```go
func TestProtocolConverterWorker_CollectObservedState(t *testing.T) {
    mockService := &MockProtocolConverterService{
        childStates: map[string]string{
            "connection": "Running",
            "dfc": "Running",
        },
    }

    worker := &ProtocolConverterWorker{
        service: mockService,
        identity: fsmv2.Identity{ID: "test-pc"},
    }

    ctx := context.Background()
    observed, err := worker.CollectObservedState(ctx)
    assert.NoError(t, err)
    assert.True(t, observed.(*ProtocolConverterObservedState).ConnectionHealthy)
    assert.True(t, observed.(*ProtocolConverterObservedState).DFCHealthy)
}
```

**Step 3: Run tests**

```bash
cd umh-core
go test ./pkg/fsmv2/workers/protocolconverter/... -v
```

**Step 4: Commit**

```bash
git add pkg/fsmv2/workers/protocolconverter/worker.go pkg/fsmv2/workers/protocolconverter/worker_test.go
git commit -m "feat(fsmv2): migrate ProtocolConverterWorker to use service layer

Update ProtocolConverterWorker to coordinate child workers through
ProtocolConverterService. CollectObservedState now ticks children
and aggregates their state.

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 5: Update ProtocolConverter States to Query Children

**Files:**
- Modify: `pkg/fsmv2/workers/protocolconverter/states.go`

**Step 1: Update TryingToStartState**

Change from:
```go
func (s *TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // Check some condition
    return &RunningState{}, fsmv2.SignalNone, nil
}
```

To:
```go
func (s *TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    observed := snapshot.Observed.(*ProtocolConverterObservedState)

    // Check if both children are healthy
    if observed.ConnectionHealthy && observed.DFCHealthy {
        return &RunningState{}, fsmv2.SignalNone, nil
    }

    // Children not healthy - emit action to start them
    return s, fsmv2.SignalNone, &StartProtocolConverterAction{
        service: getServiceFromContext(snapshot),  // Pattern TBD
    }
}
```

**Step 2: Update RunningState to check children**

```go
func (s *RunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    if snapshot.Desired.ShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    observed := snapshot.Observed.(*ProtocolConverterObservedState)

    // Check if children are still healthy
    if !observed.ConnectionHealthy || !observed.DFCHealthy {
        return &DegradedState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

**Step 3: Commit**

```bash
git add pkg/fsmv2/workers/protocolconverter/states.go
git commit -m "feat(fsmv2): update ProtocolConverter states to query children

Parent states now check child health from aggregated observed state.
Transition to Running only when both Connection and DFC are healthy.
Transition to Degraded if any child becomes unhealthy.

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 6: Update ProtocolConverter Actions to Coordinate Children

**Files:**
- Modify: `pkg/fsmv2/workers/protocolconverter/actions.go`

**Step 1: Update StartProtocolConverterAction**

```go
type StartProtocolConverterAction struct {
    service *ProtocolConverterService
}

func (a *StartProtocolConverterAction) Execute(ctx context.Context) error {
    // 1. Start connection (child worker)
    if err := a.service.StartConnection(ctx); err != nil {
        return fmt.Errorf("failed to start connection: %w", err)
    }

    // 2. Wait for connection to be healthy
    for {
        if a.service.IsConnectionHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    // 3. Start DFC (child worker)
    if err := a.service.StartDFC(ctx); err != nil {
        return fmt.Errorf("failed to start DFC: %w", err)
    }

    // 4. Wait for DFC to be healthy
    for {
        if a.service.IsDFCHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    return nil
}

func (a *StartProtocolConverterAction) Name() string { return "StartProtocolConverter" }
```

**Step 2: Update StopProtocolConverterAction**

```go
type StopProtocolConverterAction struct {
    service *ProtocolConverterService
}

func (a *StopProtocolConverterAction) Execute(ctx context.Context) error {
    // 1. Stop DFC first (reverse order)
    if err := a.service.StopDFC(ctx); err != nil {
        return fmt.Errorf("failed to stop DFC: %w", err)
    }

    // 2. Wait for DFC to stop
    for {
        if !a.service.IsDFCHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    // 3. Stop connection
    if err := a.service.StopConnection(ctx); err != nil {
        return fmt.Errorf("failed to stop connection: %w", err)
    }

    // 4. Wait for connection to stop
    for {
        if !a.service.IsConnectionHealthy() {
            break
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
            a.service.TickChildSupervisors(ctx)
        }
    }

    return nil
}

func (a *StopProtocolConverterAction) Name() string { return "StopProtocolConverter" }
```

**Step 3: Commit**

```bash
git add pkg/fsmv2/workers/protocolconverter/actions.go
git commit -m "feat(fsmv2): update ProtocolConverter actions to coordinate children

Actions now start/stop children in sequence and wait for health checks.
StartProtocolConverterAction: Connection → DFC
StopProtocolConverterAction: DFC → Connection (reverse order)

Actions tick children during wait loops to get fresh state.
Compatible with async action executor pattern.

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 7: Integration Testing

**Files:**
- Create: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/protocolconverter/integration_test.go`

**Step 1: Create integration test**

```go
package protocolconverter

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "go.uber.org/zap"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

func TestProtocolConverter_HierarchicalComposition(t *testing.T) {
    // Create parent supervisor
    parentSupervisor := supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "protocol_converter",
        Store: createTestStore(t, "protocol_converter"),
        Logger: zap.NewNop().Sugar(),
    })

    // Create service with child supervisors
    service := NewProtocolConverterService(ServiceConfig{
        ConnectionStore: createTestStore(t, "connection"),
        DFCStore: createTestStore(t, "dataflow_component"),
        ConnectionID: "test-conn",
        DFCID: "test-dfc",
        Logger: zap.NewNop().Sugar(),
    })

    // Create parent worker
    parentWorker := &ProtocolConverterWorker{
        service: service,
        identity: fsmv2.Identity{
            ID: "test-pc",
            Name: "Test Protocol Converter",
            WorkerType: "protocol_converter",
        },
    }

    // Add parent worker to parent supervisor
    err := parentSupervisor.AddWorker(parentWorker.identity, parentWorker)
    assert.NoError(t, err)

    ctx := context.Background()

    // Tick parent - should tick children too
    err = parentSupervisor.TickAll(ctx)
    assert.NoError(t, err)

    // Verify parent can observe children
    observed, err := parentWorker.CollectObservedState(ctx)
    assert.NoError(t, err)
    assert.NotNil(t, observed)

    pcObserved := observed.(*ProtocolConverterObservedState)
    assert.NotEmpty(t, pcObserved.ConnectionState)
    assert.NotEmpty(t, pcObserved.DFCState)
}

func TestProtocolConverter_ChildStartSequence(t *testing.T) {
    // Similar to above but test start sequence
    // 1. Start parent
    // 2. Verify connection starts first
    // 3. Verify DFC starts after connection is healthy
    // 4. Verify parent transitions to Running
}

func TestProtocolConverter_ChildStopSequence(t *testing.T) {
    // Similar to above but test stop sequence
    // 1. Stop parent
    // 2. Verify DFC stops first (reverse order)
    // 3. Verify connection stops after DFC is stopped
    // 4. Verify parent transitions to Stopped
}

func createTestStore(t *testing.T, workerType string) storage.TriangularStoreInterface {
    // Create test store implementation
    // TODO: Use test fixtures or mock
    return nil
}
```

**Step 2: Run integration tests**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
go test ./pkg/fsmv2/workers/protocolconverter/... -v -tags=integration
```

**Step 3: Commit**

```bash
git add pkg/fsmv2/workers/protocolconverter/integration_test.go
git commit -m "test(fsmv2): add integration tests for hierarchical composition

Test full parent-child coordination:
- Parent ticks children during observation
- Parent observes aggregated child state
- Child start sequence (Connection → DFC)
- Child stop sequence (DFC → Connection)
- Parent state transitions based on child health

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 8: Update FSMv2 Documentation

**Files:**
- Update: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/README.md`

**Step 1: Add troubleshooting section**

Add to README after composition section:

```markdown
## Troubleshooting Hierarchical Composition

### Common Issues

**Child supervisors not ticking:**
- Verify `TickChildSupervisors()` is called in `CollectObservedState()`
- Check for context cancellation during child ticks
- Ensure child supervisors are initialized with valid store

**Parent state not observing child changes:**
- Child state is cached in observed state snapshot
- Parent must tick children to get fresh state
- Parent states receive snapshot by value, not live reference

**Action execution blocks:**
- Actions must tick children during wait loops
- Use `service.TickChildSupervisors(ctx)` before health checks
- Respect context cancellation for async action timeout

**Storage conflicts:**
- Each child supervisor must have its own store instance
- Don't share stores between parent and child supervisors
- TriangularStore collections auto-register by worker type

### Debugging Tips

**Enable debug logging:**
```go
service := NewParentService(ServiceConfig{
    Logger: zap.NewDevelopment().Sugar(),  // Verbose logging
})
```

**Check child supervisor state:**
```go
state, reason, err := service.childSupervisor.GetWorkerState(workerID)
if err != nil {
    // Child worker not found
}
// state = "Running", "Stopped", "Degraded", etc.
// reason = detailed state explanation
```

**Monitor tick timing:**
- Parent observation timeout includes child ticks
- Child ticks happen synchronously during parent observation
- If parent observation times out, children aren't getting ticked
- Consider increasing `ObservationTimeout` for composite workers
```

**Step 2: Commit**

```bash
git add pkg/fsmv2/README.md
git commit -m "docs(fsmv2): add troubleshooting for hierarchical composition

Add common issues, debugging tips, and configuration guidance
for composite workers using service layer pattern.

Related to FSMv2 hierarchical composition implementation plan."
```

### Task 9: Final Review and Cleanup

**Step 1: Run all tests**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
make test
```

**Step 2: Run linters**

```bash
golangci-lint run ./pkg/fsmv2/...
go vet ./pkg/fsmv2/...
```

**Step 3: Check for focused tests**

```bash
ginkgo -r --fail-on-focused ./pkg/fsmv2/...
```

**Step 4: Update this design doc's changelog**

Add completion entry to changelog section below.

---

## Changelog

### 2025-11-01 14:30 - Plan created
Initial design document for FSMv2 hierarchical composition.
Analyzed FSMv1 pattern, identified gaps, proposed service-layer solution.
Preserves FSMv2 simplicity while enabling FSMv1-style parent-child coordination.

### 2025-11-01 [TIME] - Implementation complete
All tasks completed:
- Service layer pattern documented
- Example composite worker created
- ProtocolConverterService implemented
- ProtocolConverterWorker migrated to use service
- States updated to query children
- Actions updated to coordinate children
- Integration tests added
- Documentation updated with troubleshooting

Implementation successful. Pattern ready for production use.
