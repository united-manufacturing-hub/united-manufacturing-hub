# FSMv2 Child Observed State Usage Analysis (FSMv1 Evidence)

**Created:** 2025-11-02
**Status:** Investigation Complete
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition
**Related:** `fsmv2-infrastructure-supervision-patterns.md`, `fsmv2-developer-expectations-child-state.md`

---

## Executive Summary

This document analyzes **how FSMv1 parents access child observed state** to determine requirements for FSMv2. The investigation answers the question: "Do parents access child observed state beyond just FSM state names?"

**Key Finding:** **YES, FSMv1 parents DO access child observed state beyond state names.** The ProtocolConverter implements a **"sanity check" pattern** that compares child states with child metrics to detect infrastructure inconsistencies.

**Critical Pattern:**
```go
// ProtocolConverter checks: Redpanda reports "active" but Benthos has zero connections
isBenthosOutputActive := outputMetrics.ConnectionUp - outputMetrics.ConnectionLost > 0

if redpandaState == "active" && !isBenthosOutputActive {
    return true, "Redpanda is active, but the flow has no output active"
}
```

**Recommendation for FSMv2:** This sanity-check pattern should **move to supervisor (infrastructure layer)**, NOT exposed to business logic. Parents should only see simplified child states.

---

## Table of Contents

1. [Investigation Summary](#investigation-summary)
2. [Pattern Classification](#pattern-classification)
3. [Detailed Evidence](#detailed-evidence)
4. [The Sanity Check Pattern](#the-sanity-check-pattern)
5. [Why This Matters for FSMv2](#why-this-matters-for-fsmv2)
6. [Recommendation](#recommendation)

---

## Investigation Summary

### Files Analyzed

- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsm/protocolconverter/actions.go`
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsm/protocolconverter/reconcile.go`
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/service/protocolconverter/protocolconverter.go`

### Search Patterns

- `GetObservedState`
- `ObservedState`
- `.Observed`
- Child manager method calls
- State reason access
- Metrics access

### Findings Count

- **State name access:** Very Common (10+ instances)
- **State reason access:** Common (5+ instances)
- **Deep metrics access:** Less common but CRITICAL (3 instances)
- **Health flags access:** Rare (1 instance)

---

## Pattern Classification

| Pattern | Frequency | Purpose | Business or Infrastructure |
|---------|-----------|---------|---------------------------|
| **State name only** | Very Common | Basic transition decisions | Business Logic |
| **State + reason** | Common | Error message composition | Business Logic (user-facing) |
| **Deep metrics** | Less common but CRITICAL | Cross-child consistency checks | **Infrastructure** (sanity checks) |
| **Health flags** | Rare | Error propagation | Infrastructure |

---

## Detailed Evidence

### Pattern 1: State Name Access (Business Logic)

**File:** `pkg/fsm/protocolconverter/actions.go`

**Lines 402-406: IsConnectionUp()**

```go
func (p *ProtocolConverter) IsConnectionUp() (bool, string) {
    if p.ObservedState.ServiceInfo.ConnectionFSMState == connectionfsm.OperationalStateUp {
        return true, ""
    }

    // TODO: Also check connection latency, packet loss from observed state
    originalStatusReason := p.ObservedState.ServiceInfo.ConnectionObservedState.ServiceInfo.StatusReason
    if originalStatusReason == "" {
        return false, "Connection Health status unknown"
    }
    statusReason := "connection: " + originalStatusReason
    return false, statusReason
}
```

**Accessed Data:**
- `ConnectionFSMState` (string) - "up", "down", etc.
- `ConnectionObservedState.ServiceInfo.StatusReason` (string)

**Purpose:** Health check for parent state transitions

**Classification:** **Business Logic** - Parent needs to know if connection is ready

**TODO Found:** Comment suggests future access of latency/packet loss metrics (not currently implemented)

---

**Lines 417-428: IsRedpandaHealthy()**

```go
func (p *ProtocolConverter) IsRedpandaHealthy() (bool, string) {
    if p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle ||
       p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive {
        return true, ""
    }

    originalStatusReason := p.ObservedState.ServiceInfo.RedpandaObservedState.ServiceInfo.StatusReason
    if originalStatusReason == "" {
        return false, "Redpanda Health status unknown"
    }
    statusReason := "redpanda: " + originalStatusReason
    return false, statusReason
}
```

**Accessed Data:**
- `RedpandaFSMState` (string)
- `RedpandaObservedState.ServiceInfo.StatusReason` (string)

**Purpose:** Health check for parent state transitions

**Classification:** **Business Logic**

---

**Lines 439-450: IsDFCHealthy()**

```go
func (p *ProtocolConverter) IsDFCHealthy() (bool, string) {
    if p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateIdle ||
       p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateActive {
        return true, ""
    }

    statusReason := p.ObservedState.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.StatusReason
    if statusReason == "" {
        statusReason = "flow health status unknown"
    }
    return false, statusReason
}
```

**Accessed Data:**
- `DataflowComponentReadFSMState` (string)
- `DataflowComponentReadObservedState.ServiceInfo.StatusReason` (string)

**Purpose:** Health check for parent state transitions

**Classification:** **Business Logic**

---

### Pattern 2: State Reason Access (User-Facing Messages)

**File:** `pkg/fsm/protocolconverter/actions.go`

All three health check functions above access `StatusReason` to build composite error messages:

```go
statusReason := "redpanda: " + originalStatusReason
return false, statusReason
```

**Purpose:** Build meaningful error messages for users/operators

**Example messages:**
- "connection: timeout after 30s"
- "redpanda: broker unavailable"
- "flow health status unknown"

**Classification:** **Business Logic** (user-facing)

**Why needed:** Parent needs to explain to user WHY it's degraded, not just THAT it's degraded

---

### Pattern 3: Deep Metrics Access (Infrastructure Sanity Checks)

**File:** `pkg/fsm/protocolconverter/actions.go`

**Lines 458-462: safeBenthosMetrics() Helper**

```go
func (p *ProtocolConverter) safeBenthosMetrics() (input, output struct{ ConnectionUp, ConnectionLost int64 }) {
    if p.ObservedState.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState == nil {
        return
    }

    metrics := p.ObservedState.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

    return struct{ ConnectionUp, ConnectionLost int64 }{
        ConnectionUp:   metrics.Input.ConnectionUp,
        ConnectionLost: metrics.Input.ConnectionLost,
    }, struct{ ConnectionUp, ConnectionLost int64 }{
        ConnectionUp:   metrics.Output.ConnectionUp,
        ConnectionLost: metrics.Output.ConnectionLost,
    }
}
```

**Accessed Data (4 levels deep!):**
- `DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionUp` (int64)
- `Input.ConnectionLost` (int64)
- `Output.ConnectionUp` (int64)
- `Output.ConnectionLost` (int64)

**Purpose:** Used by `IsOtherDegraded()` for cross-child consistency checks

**Classification:** **INFRASTRUCTURE** - This is sanity checking, not business logic

---

**Lines 473-511: IsOtherDegraded() - The Sanity Check Pattern**

```go
func (p *ProtocolConverter) IsOtherDegraded() (bool, string) {
    inputMetrics, outputMetrics := p.safeBenthosMetrics()

    // Business logic: If everything reports healthy but parent has shutdown requested
    if p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle &&
       p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateIdle &&
       p.ObservedState.ServiceInfo.DataflowComponentWriteFSMState == dataflowfsm.OperationalStateIdle &&
       p.ObservedState.ServiceInfo.ConnectionFSMState == connectionfsm.OperationalStateDown {
        isShuttingDown, _ := p.DesiredState.ShutdownRequested()
        if !isShuttingDown {
            return true, "not shutting down, but all children are down/idle"
        }
    }

    // INFRASTRUCTURE SANITY CHECK: Redpanda active but Benthos has no output
    isBenthosOutputActive := outputMetrics.ConnectionUp - outputMetrics.ConnectionLost > 0

    if (p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle ||
        p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive) &&
        !isBenthosOutputActive {
        return true, fmt.Sprintf("Redpanda is %s, but the flow has no output active (connection up: %d, connection lost: %d)",
            p.ObservedState.ServiceInfo.RedpandaFSMState, outputMetrics.ConnectionUp, outputMetrics.ConnectionLost)
    }

    // INFRASTRUCTURE SANITY CHECK: Connection down but Benthos has input
    isBenthosInputActive := inputMetrics.ConnectionUp - inputMetrics.ConnectionLost > 0

    if p.ObservedState.ServiceInfo.ConnectionFSMState != connectionfsm.OperationalStateUp &&
        isBenthosInputActive {
        return true, fmt.Sprintf("Connection is %s, but the flow has input active (connection up: %d, connection lost: %d)",
            p.ObservedState.ServiceInfo.ConnectionFSMState, inputMetrics.ConnectionUp, inputMetrics.ConnectionLost)
    }

    // Additional business logic checks
    // ...

    return false, ""
}
```

**What this does:**

1. **Sanity check 1:** Redpanda reports "idle" or "active" BUT Benthos output connections = 0
   - **Impossible state** - If Redpanda is active, Benthos should have Kafka connections
   - **Indicates:** Infrastructure misconfiguration or network issue

2. **Sanity check 2:** Connection reports "down" BUT Benthos input connections > 0
   - **Impossible state** - If connection is down, Benthos shouldn't have input connections
   - **Indicates:** FSM state inconsistency or stale metrics

**Classification:** **INFRASTRUCTURE** - Detecting system inconsistencies, not business logic

**Why critical:** Parent can catch impossible states that individual children can't detect

---

### Pattern 4: Health Flag Access (Error Propagation)

**File:** `pkg/fsm/protocolconverter/actions.go`

**Lines 224-227: Setting Health Flags**

```go
infoWithFailedHealthChecks := info.DeepCopy()
infoWithFailedHealthChecks.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive = false
infoWithFailedHealthChecks.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady = false
infoWithFailedHealthChecks.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive = false
infoWithFailedHealthChecks.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady = false
```

**Purpose:** When service status fetch fails, mark child Benthos components as unhealthy

**Classification:** **Infrastructure** - Error propagation

**Usage:** Rare - only in error handling paths

---

## The Sanity Check Pattern

### What It Is

The sanity check pattern **compares child states across multiple children** to detect infrastructure inconsistencies:

```go
// Child A says: "I'm active"
redpandaState == "active"

// Child B's metrics say: "No connections to Child A"
benthosOutputConnections == 0

// Conclusion: Impossible state, infrastructure issue
return "degraded: Redpanda active but Benthos has no output"
```

### Why It Exists

**Children can lie or become inconsistent due to:**
- Network partitions
- Stale metrics
- Race conditions during startup
- Configuration errors
- FSM bugs

**Example scenario:**
1. Redpanda reports "active" (FSM state)
2. Benthos lost Kafka connection due to network issue
3. Benthos hasn't updated its FSM state yet (still reports "active")
4. **Parent detects:** Redpanda="active" + BenthosConnections=0 = inconsistent

### Cross-Child Checks in FSMv1

| Check | Child A | Child B | Inconsistency Detected |
|-------|---------|---------|------------------------|
| Redpanda vs Benthos Output | Redpanda: "active" | Benthos: 0 output connections | Redpanda claims active but no data flowing out |
| Connection vs Benthos Input | Connection: "down" | Benthos: >0 input connections | Connection claims down but data flowing in |
| All Children Down | All: "idle"/"down" | Parent: not shutting down | All children idle but parent not requested shutdown |

### Why This Requires Observed State Access

**State names alone are insufficient:**
- Benthos state: "active" (FSM state)
- Benthos connections: 0 (metric from observed state)
- **Need both** to detect inconsistency

**State + reason insufficient:**
- Redpanda state: "active"
- Redpanda reason: "broker healthy"
- Benthos state: "active"
- Benthos reason: "processing messages"
- **Cannot detect** connection count mismatch without metrics

---

## Why This Matters for FSMv2

### Problem: Where Should Sanity Checks Live?

**Option A: Keep in parent business logic (FSMv1 pattern)**
```go
// Parent FSM state machine
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Access child observed state
    benthosObserved := snapshot.GetChildObserved("dfc_read")
    redpandaState := snapshot.GetChildState("redpanda")

    // Sanity check
    if redpandaState == "active" && benthosObserved.OutputConnections == 0 {
        return &DegradedState{}, SignalNone, nil
    }
}
```

**Problem:** Mixes infrastructure (sanity checks) with business logic (state transitions)

---

**Option B: Move to supervisor infrastructure layer (Recommended)**
```go
// Supervisor infrastructure health checker
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    redpandaState := ihc.supervisor.GetChildState("redpanda")
    benthosObserved := ihc.supervisor.GetChildObserved("dfc_read")

    // Sanity check (infrastructure layer)
    if redpandaState == "active" && benthosObserved.OutputConnections == 0 {
        return fmt.Errorf("redpanda active but benthos has no output")
    }

    return nil
}

// Parent FSM state machine
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Parent only sees simplified child states
    // Supervisor handled sanity check, restarted child if needed
    // Parent sees state transitions: active → stopped → initializing → active

    dfcState := snapshot.GetChildState("dfc_read")
    if dfcState != "active" {
        return s, SignalNone, nil  // Wait
    }

    return &IdleState{}, SignalNone, nil
}
```

**Benefits:**
- ✅ Infrastructure (sanity checks) separated from business logic
- ✅ Parent FSM stays simple
- ✅ Supervisor handles recovery automatically
- ✅ Parent sees normal state transitions

---

## Recommendation

### For FSMv2: Move Sanity Checks to Supervisor

**Parent workers should NOT access child observed state directly.** Instead:

1. **Supervisor performs sanity checks** in infrastructure layer
2. **Supervisor detects inconsistencies** (Redpanda active, Benthos no connections)
3. **Supervisor restarts affected child** (see `fsmv2-infrastructure-supervision-patterns.md`)
4. **Parent sees state transitions** (active → stopped → initializing → active)

### Implementation

```go
// pkg/fsmv2/supervisor/infrastructure_health.go

type InfrastructureHealthChecker struct {
    supervisor *Supervisor
}

func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Get child states
    redpandaState, _ := ihc.supervisor.GetChildState("redpanda")

    // Get child observed state (supervisor-internal access)
    benthosObserved, err := ihc.supervisor.getChildObservedInternal("dfc_read")
    if err != nil {
        return nil // Can't check, skip
    }

    // Sanity check: Cross-child consistency
    if redpandaState == "active" {
        outputActive := benthosObserved.OutputConnectionsUp - benthosObserved.OutputConnectionsLost
        if outputActive == 0 {
            return fmt.Errorf("redpanda active but benthos has %d output connections", outputActive)
        }
    }

    return nil
}

// Internal method - NOT exposed to parent workers
func (s *Supervisor) getChildObservedInternal(childName string) (interface{}, error) {
    childSpec := s.childDeclarations[childName]
    return s.store.LoadObserved(ctx, childSpec.WorkerType, childSpec.ID)
}
```

### Parent Worker Code

```go
// Parent worker NEVER accesses child observed state
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Simple business logic
    dfcState := snapshot.GetChildState("dfc_read")

    if dfcState != "active" {
        return s, SignalNone, nil
    }

    return &IdleState{}, SignalNone, nil
}
```

**Benefits:**
1. ✅ Clean separation: infrastructure vs business logic
2. ✅ Parent workers stay simple (no deep metrics access)
3. ✅ Supervisor handles sanity checks automatically
4. ✅ Cross-child consistency validated in infrastructure layer
5. ✅ FSMv1 sanity check pattern preserved (just moved to right layer)

### Exceptions: When Parents MIGHT Need Observed State

**Scenario:** Parent needs to display detailed child information to user.

**Example:** Management Console showing bridge status with connection latency.

**Solution:** This is **presentation layer concern**, not business logic. Should be handled by:
- Separate status aggregation service
- GraphQL queries to TriangularStore
- NOT in FSM state machine logic

**FSM states should NOT access observed state for UI display.**

---

## Conclusion

**Evidence shows:**
1. ✅ FSMv1 parents DO access child observed state
2. ✅ Most common: State names + reasons (business logic)
3. ✅ Critical pattern: Deep metrics for sanity checks (infrastructure)

**Recommendation for FSMv2:**
1. ❌ Parents should NOT access child observed state in business logic
2. ✅ Supervisor performs sanity checks in infrastructure layer
3. ✅ Parents only query child state names (simple API)
4. ✅ Supervisor restarts children when inconsistencies detected
5. ✅ Clean separation: infrastructure (supervisor) vs business logic (FSM states)

**Related documents:**
- `fsmv2-infrastructure-supervision-patterns.md` - How supervisor handles sanity check failures
- `fsmv2-developer-expectations-child-state.md` - What developers should see (state names only)
