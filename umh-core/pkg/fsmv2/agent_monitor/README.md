# Agent Monitor FSM v2

Agent health monitoring implementation using FSM v2 architecture.

## Overview

The agent_monitor FSM v2 tracks the health status of the UMH agent by continuously monitoring latency, release version, and overall operational health. This is a **passive monitoring FSM** - it observes metrics and transitions states based on health conditions without performing any control actions.

### Key Characteristics

- **Purely passive**: No actions, only state transitions based on observed health
- **Health-driven**: Transitions based on service health metrics (Active, Degraded, Failed)
- **Supervisor-managed**: Relies on supervisor for data freshness validation and lifecycle management
- **100% test coverage**: Comprehensive tests with architectural boundary enforcement
- **Timestamp-independent**: All state transitions are deterministic based on health status alone

### What is Agent Monitoring?

Agent monitoring observes the health of the UMH agent itself by tracking:

1. **Latency Health**: Response time and performance metrics
2. **Release Health**: Version compatibility and update status
3. **Overall Health**: Combined assessment of all metrics

This provides visibility into the agent's operational state and enables automated health-based responses.

## Architecture

The agent_monitor FSM v2 implements the triangular model from the FSM v2 RFC:

```
        IDENTITY
      (Agent Monitor)
           /\
          /  \
         /    \
        /      \
    DESIRED  OBSERVED
  (Monitoring  (Health
   enabled)    metrics)
       \      /
        \    /
      RECONCILIATION
     (State transitions
     based on health)
```

### Triangular Model Components

**Identity**:
- ID: Unique identifier for this worker
- Name: "agent_monitor"
- WorkerType: "agent_monitor"

**Desired State**:
- `shutdownRequested`: Boolean flag for graceful shutdown
- Always derived as `{ shutdownRequested: false }` (monitoring always enabled)

**Observed State**:
- `ServiceInfo`: Health metrics from agent_monitor.Service
  - `OverallHealth`: Active | Degraded | Failed
  - `LatencyHealth`: Active | Degraded | Failed
  - `ReleaseHealth`: Active | Degraded | Failed
- `CollectedAt`: Timestamp when metrics were collected

### Worker Pattern

The `AgentMonitorWorker` implements the FSM v2 Worker interface:

```go
type AgentMonitorWorker struct {
    identity       fsmv2.Identity
    monitorService agent_monitor.IAgentMonitorService
}
```

**Worker Responsibilities**:

1. **CollectObservedState**: Async goroutine that monitors agent health
   - Calls `monitorService.Status()` to gather health metrics
   - Returns `AgentMonitorObservedState` with current health assessment
   - Runs continuously in background, managed by supervisor

2. **DeriveDesiredState**: Pure function converting user config to desired state
   - Always returns `{ shutdownRequested: false }` (monitoring always active)
   - No user configuration needed for agent monitoring

3. **GetInitialState**: Returns `StoppedState` as starting point

### Passive State Machine

This is a **passive** FSM - states observe and transition, but never execute actions:

- **No action execution**: All `Next()` methods return `nil` action
- **No side effects**: State transitions are pure logic based on observations
- **No control**: FSM doesn't start/stop services, just observes and reports health

This differs from active FSMs (like container FSM) that execute actions to achieve desired state.

## State Machine

### State Diagram

```
                    [Supervisor Creates Worker]
                              |
                              v
                        +----------+
                   +--->| Stopped  |<---+
                   |    +----------+    |
                   |         |          |
                   |         | (Monitor always enabled)
                   |         v          |
                   |    +----------+    |
                   |    | Starting |    |
                   |    +----------+    |
                   |         |          |
                   |         | (Verify health first)
                   |         v          |
                   |    +----------+    |
    (Shutdown)     |    | Degraded |<---+--- (Health degrades)
                   |    +----------+    |
                   |         |          |
                   |         | (Health recovers)
                   |         v          |
                   |    +----------+    |
                   |    |  Active  |----+
                   |    +----------+
                   |         |
                   |         | (Shutdown)
                   |         v
                   |    +----------+
                   |    | Stopping |
                   |    +----------+
                   |         |
                   +---------+
```

### State Descriptions

#### 1. StoppedState

**Purpose**: Monitoring is not active

**Transitions**:
- If `ShutdownRequested` → Stay in Stopped, signal `SignalNeedsRemoval`
- Otherwise → Transition to Starting

**Characteristics**:
- Initial state when worker is created
- Final state before removal when shutdown completes
- No observation collection happens in this state

**Reason**: "Monitoring is not active"

#### 2. StartingState

**Purpose**: Transitional state from Stopped to operational monitoring

**Transitions**:
- If `ShutdownRequested` → Transition to Stopping
- Otherwise → Transition to Degraded

**Characteristics**:
- Passive startup (no actual initialization work needed)
- Always transitions to Degraded first to verify health
- Ensures health is checked before declaring Active

**Reason**: "Initializing monitoring"

**Design Note**: Agent monitoring always starts in Degraded state rather than directly to Active. This ensures health metrics are verified before declaring the agent healthy.

#### 3. DegradedState

**Purpose**: Operation with unhealthy metrics or unverified health

**State Data**:
- `reason`: String explaining why degraded (e.g., health status details)

**Transitions**:
- If `ShutdownRequested` → Transition to Stopping
- If `IsFullyHealthy(observed)` → Transition to Active
- Otherwise → Stay in Degraded

**Characteristics**:
- Passive waiting for health to recover
- Continuously re-evaluates health on each tick
- Provides detailed reason string for observability

**Reason**: "Monitoring degraded: [details]" where details include:
- Overall health status
- Latency health status
- Release health status

**Example**: `"Monitoring degraded: Agent health: Degraded (Latency: Active, Release: Degraded)"`

#### 4. ActiveState

**Purpose**: Normal operation with healthy metrics

**Transitions**:
- If `ShutdownRequested` → Transition to Stopping
- If `!IsFullyHealthy(observed)` → Transition to Degraded with reason
- Otherwise → Stay in Active

**Characteristics**:
- Indicates all health metrics are Active
- Continuously monitors for health degradation
- No actions needed - purely observational

**Reason**: "Monitoring active with healthy metrics"

#### 5. StoppingState

**Purpose**: Transitional state from operational to stopped

**Transitions**:
- Always → Transition to Stopped

**Characteristics**:
- Immediate transition to Stopped
- No cleanup work needed (monitoring just stops)
- Passive shutdown (no process termination)

**Reason**: "Stopping monitoring"

### Transition Conditions

#### Health Evaluation

The `IsFullyHealthy()` function determines health status:

```go
func IsFullyHealthy(observed *AgentMonitorObservedState) bool {
    return observed.ServiceInfo != nil &&
           observed.ServiceInfo.OverallHealth == models.Active
}
```

**Health is considered "fully healthy" when**:
- ServiceInfo is available (not nil)
- OverallHealth is exactly `Active` (not Degraded or Failed)

**Any other state is considered degraded**:
- `OverallHealth == Degraded` → Degraded state
- `OverallHealth == Failed` → Degraded state
- `ServiceInfo == nil` → Degraded state with reason "No service info available"

#### Shutdown Handling

Every state follows the **shutdown-first pattern**:

```go
func (s *SomeState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // ALWAYS check shutdown first
    if snapshot.Desired.ShutdownRequested() {
        return &NextShutdownState{}, fsmv2.SignalNone, nil
    }

    // ... rest of state logic
}
```

This ensures graceful shutdown sequences are visible and explicit in every state.

## Implementation Details

### File Structure

- **`worker.go`**: Worker implementation with Identity, CollectObservedState, DeriveDesiredState
- **`snapshot.go`**: DesiredState and ObservedState type definitions
- **`conditions.go`**: Health evaluation logic (IsFullyHealthy, BuildDegradedReason)
- **`state_stopped.go`**: StoppedState implementation
- **`state_starting.go`**: StartingState implementation
- **`state_degraded.go`**: DegradedState implementation
- **`state_active.go`**: ActiveState implementation
- **`state_stopping.go`**: StoppingState implementation
- **`*_test.go`**: Comprehensive test coverage for each component

### Health Evaluation Logic

#### IsFullyHealthy

Checks if agent health is fully operational:

```go
func IsFullyHealthy(observed *AgentMonitorObservedState) bool {
    return observed.ServiceInfo != nil &&
           observed.ServiceInfo.OverallHealth == models.Active
}
```

**Used by**: ActiveState (to detect degradation), DegradedState (to detect recovery)

#### BuildDegradedReason

Creates detailed reason string for observability:

```go
func BuildDegradedReason(observed *AgentMonitorObservedState) string {
    if observed.ServiceInfo == nil {
        return "No service info available"
    }

    return fmt.Sprintf("Agent health: %s (Latency: %s, Release: %s)",
        observed.ServiceInfo.OverallHealth,
        observed.ServiceInfo.LatencyHealth,
        observed.ServiceInfo.ReleaseHealth,
    )
}
```

**UX Standards Compliance**: Shows real values instead of generic messages, enabling users to understand exactly what's wrong.

### Integration with agent_monitor.Service

The worker wraps the existing `agent_monitor.Service`:

```go
serviceInfo, err := w.monitorService.Status(ctx, fsm.SystemSnapshot{})
```

**What this replaces**: The entire `_monitor` logic from FSM v1. Previously, the monitoring logic was embedded in the FSM callbacks. Now it's cleanly separated:

- **Service layer** (`pkg/service/agent_monitor`): Collects health metrics
- **FSM layer** (`pkg/fsmv2/agent_monitor`): Makes state transition decisions based on metrics

### Shutdown Handling

Graceful shutdown follows a specific sequence:

1. Supervisor sets `ShutdownRequested = true` in desired state
2. Current state checks shutdown in `Next()` and transitions to appropriate next state
3. Shutdown sequence depends on current state:
   - `Active` or `Degraded` → `Stopping` → `Stopped`
   - `Starting` → `Stopping` → `Stopped`
   - `Stopping` → `Stopped`
   - `Stopped` → Signal `SignalNeedsRemoval` for cleanup

**Key principle**: No shortcuts. Shutdown always progresses through proper state sequences to ensure visibility and consistency.

### Data Freshness

The supervisor (not the FSM states) is responsible for data freshness:

- **Supervisor checks**: Is the collector goroutine working? Is data stale?
- **States assume**: Observations are always fresh and valid
- **Separation of concerns**: Infrastructure concern (supervisor) vs application concern (states)

**What happens if collector fails**:
1. Supervisor detects stale observations
2. Supervisor pauses FSM (doesn't call state transitions)
3. Supervisor attempts to restart collector with exponential backoff
4. If collector cannot be recovered, supervisor escalates to graceful shutdown

**Why states never see stale data**: Safety. Prevents making health decisions based on outdated metrics.

## Testing

### Test Coverage

**Achievement**: 100.0% of statements covered

```
Ran 75 of 75 Specs in 0.002 seconds
SUCCESS! -- 75 Passed | 0 Failed | 0 Pending | 0 Skipped
coverage: 100.0% of statements
```

### Test Categories

#### 1. State Transition Tests

Each state has comprehensive tests for:
- Shutdown handling (highest priority transition)
- Normal state transitions (health-based)
- Edge cases (nil observed state, invalid types)

**Example** (`state_active_test.go`):
```go
It("should transition to Stopped when shutdown requested", func() { ... })
It("should transition to Degraded when health degrades", func() { ... })
It("should stay in Active when fully healthy", func() { ... })
```

#### 2. Timestamp Independence Tests

Critical architectural boundary enforcement:

```go
// state_timestamp_independence_test.go
Describe("Timestamp Independence", func() {
    It("should make identical decisions regardless of observation timestamp", func() {
        // Same health metrics with different timestamps
        // MUST produce identical state transitions
    })
})
```

**Why this matters**: State transitions must be deterministic based on health status alone, not observation timing. This ensures predictable behavior and prevents timestamp-dependent bugs.

#### 3. Condition Tests

Health evaluation logic verification:

```go
// conditions_test.go
Describe("IsFullyHealthy", func() {
    It("should return true when OverallHealth is Active", func() { ... })
    It("should return false when ServiceInfo is nil", func() { ... })
})

Describe("BuildDegradedReason", func() {
    It("should include all health statuses", func() { ... })
})
```

#### 4. Worker Tests

Worker interface implementation:

```go
// worker_test.go
Describe("CollectObservedState", func() {
    It("should collect health metrics from service", func() { ... })
})

Describe("DeriveDesiredState", func() {
    It("should always return monitoring enabled", func() { ... })
})
```

### Running Tests

```bash
# Run all tests
cd pkg/fsmv2/agent_monitor
go test

# Run with coverage report
go test -cover

# Run with verbose output
go test -v

# Run specific test
go test -run TestStateStopped
```

### Timestamp Independence Enforcement

**Architectural requirement**: State transitions must be based on health status alone, not when the observation was collected.

**Test pattern**:
```go
observedOld := &AgentMonitorObservedState{
    ServiceInfo: healthyInfo,
    CollectedAt: time.Now().Add(-1 * time.Hour), // 1 hour ago
}

observedNew := &AgentMonitorObservedState{
    ServiceInfo: healthyInfo,
    CollectedAt: time.Now(), // Now
}

// MUST produce identical transitions
stateOld, _, _ := state.Next(snapshotWithObserved(observedOld))
stateNew, _, _ := state.Next(snapshotWithObserved(observedNew))

Expect(stateOld).To(Equal(stateNew))
```

This ensures the FSM is deterministic and timestamp bugs cannot occur.

## Migration Notes

### Differences from FSM v1

**FSM v1** (`pkg/fsm/agent_monitor`):
- Mixed observation and state logic in FSM callbacks
- State management spread across machine.go, reconcile.go, actions.go
- No separation between infrastructure (data freshness) and application (health) concerns
- Manual retry logic and error handling
- State stored only in memory

**FSM v2** (`pkg/fsmv2/agent_monitor`):
- Clean separation: Service collects metrics, FSM makes decisions
- Worker pattern with explicit Identity/Desired/Observed model
- Supervisor handles data freshness, states assume fresh data
- Automatic retry with exponential backoff (supervisor-managed)
- State persistence ready (CSE integration deferred)

### Why This Approach Was Chosen

**Simplicity**: States contain pure business logic without infrastructure concerns. Easier to write, test, and reason about.

**Safety**: No risk of making decisions on stale data. Supervisor validates data freshness before calling state transitions.

**Reliability**: Automatic collector recovery with clear escalation paths. Infrastructure failures don't corrupt application logic.

**Observability**: Clear separation makes it obvious where failures occur:
- Collector infrastructure issue? Check supervisor logs.
- Health assessment issue? Check state transition logs.

**Testability**: 100% coverage achieved because logic is pure and deterministic. Timestamp independence tests prevent timing bugs.

### Integration Considerations

**Current status**: Standalone FSM v2 worker, not yet integrated with CSE or main agent.

**Deferred to future work**:
1. CSE integration for state persistence
2. Integration with main agent startup/shutdown
3. Health-based alerting or automated responses
4. Historical health tracking and trending

**Why deferred**: FSM v2 and agent_monitor implementation are complete and tested. Integration requires broader system changes and is tracked separately.

## References

### Linear Tickets

- **ENG-3646**: Parent EPIC - FSM v2 migration
- **ENG-3647**: FSM v2 RFC - Architecture and design
- **ENG-3649**: Phase 5 - Agent Monitor FSM v2 implementation (this work)

### Reference Implementations

- **Container FSM v2**: `pkg/fsmv2/container/` - Reference implementation, similar passive monitoring pattern
- **FSM v2 RFC**: `pkg/fsmv2/README.md` - Architecture documentation
- **CSE Storage**: `pkg/cse/storage/README.md` - State persistence design (future integration)

### Related Code

- **Agent Monitor Service**: `pkg/service/agent_monitor/` - Health metric collection
- **FSM v2 Supervisor**: `pkg/fsmv2/supervisor/` - Generic supervisor managing workers
- **FSM v2 Worker Interface**: `pkg/fsmv2/worker.go` - Worker contract definitions

---

**Version**: FSM v2
**Status**: Implementation complete, integration deferred
**Test Coverage**: 100%
**Last Updated**: 2025-10-19
