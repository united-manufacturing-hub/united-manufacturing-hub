# FSMv2 Infrastructure Supervision Patterns

**Created:** 2025-11-02
**Status:** Design Investigation
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition
**Related:** `fsmv2-child-state-observation.md`, `fsmv2-supervisor-composition-declarative.md`

---

## Executive Summary

This document investigates how FSMv2 supervisors can handle **infrastructure failures** without mixing with **business logic states**. The core challenge: supervisors need to detect and recover from infrastructure issues (e.g., "Redpanda reports active but Benthos has zero connections") without overriding child FSM business states (e.g., "degraded" is a business state for Benthos).

**Key Principle:** Infrastructure concerns NEVER leak into business logic. Supervisor handles failures invisibly through restarts and lifecycle management.

**Recommended Approach:** **Pattern 1 (Circuit Breaker with Restart)** - Supervisor detects infrastructure inconsistency, stops ticking, restarts child, resumes when healthy. Parent business logic sees normal state transitions.

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [The Separation Challenge](#the-separation-challenge)
3. [Pattern 1: Circuit Breaker with Restart](#pattern-1-circuit-breaker-with-restart)
4. [Pattern 2: Transparent Restart with Lifecycle States](#pattern-2-transparent-restart-with-lifecycle-states)
5. [Pattern 3: Infrastructure Health Layer](#pattern-3-infrastructure-health-layer)
6. [Pattern 4: Fail-Stop Semantics](#pattern-4-fail-stop-semantics)
7. [Comparison Matrix](#comparison-matrix)
8. [Recommended Approach](#recommended-approach)
9. [Concrete Examples](#concrete-examples)
10. [Restart Policies](#restart-policies)
11. [Edge Cases](#edge-cases)

---

## Problem Statement

### The Core Issue

In FSMv1, the ProtocolConverter parent FSM performs **sanity checks** by comparing child states:

```go
// pkg/fsm/protocolconverter/actions.go:496-501
isBenthosOutputActive := outputMetrics.ConnectionUp - outputMetrics.ConnectionLost > 0

if redpandaState == "active" && !isBenthosOutputActive {
    return true, "Redpanda is active, but the flow has no output active"
}
```

This detects **infrastructure inconsistencies**: Redpanda reports "active" but Benthos has zero Kafka output connections. This is an impossible state indicating infrastructure failure (misconfiguration, network issues, etc.).

### Why This is Problematic in FSMv2

**User insight:**
> "degraded is actually business logic for Benthos (high error rate). But for Connection, detecting 'reports up but no connections' is infrastructure. Supervisor should NOT mix business logic (which are the states) and infrastructure logic."

**The conflict:**
- **Business logic states** (owned by child FSM): "idle", "active", "degraded", "error"
  - "degraded" for Benthos = high processing error rate (business concern)
- **Infrastructure sanity check** (owned by supervisor): Detecting child state inconsistencies
  - Supervisor cannot override Benthos to "degraded" - violates separation of concerns

### What Needs to Happen

1. **Supervisor detects infrastructure issue** (Redpanda active, Benthos no connections)
2. **Supervisor handles recovery** (restart child, reset connections)
3. **Parent business logic sees state transitions** (active → stopped → initializing → active)
4. **Parent never knows WHY restart happened** (infrastructure detail)

---

## The Separation Challenge

### Infrastructure Concerns (Supervisor)

- Child supervisor crashed
- Collector timeout
- Cross-child state inconsistencies (sanity checks)
- Network failures
- Configuration validation

### Business Logic Concerns (FSM States)

- "Connection is up" vs "Connection is down"
- "Benthos is processing" vs "Benthos has high error rate" (degraded)
- "Redpanda is idle" vs "Redpanda is active"

### The Boundary

**Clear separation:**
- Supervisor detects infrastructure issues → **Restarts child**
- Child FSM transitions through lifecycle states → **Parent sees state transitions**
- Parent business logic: "If child not in expected state, wait"

**NO leakage:**
- Parent never queries "IsChildHealthy()" (infrastructure)
- Parent never sees "infrastructure_degraded" state (mixing)
- Child business states not overridden by supervisor (separation)

---

## Pattern 1: Circuit Breaker with Restart

### Concept

When supervisor detects infrastructure issue:
1. **Open circuit** - Stop ticking parent FSM and all children
2. **Restart affected child** - Trigger graceful stop → start lifecycle
3. **Retry with backoff** - Exponential backoff if issue persists
4. **Close circuit** - Resume ticking when infrastructure healthy

**Based on:** Erlang/OTP supervision trees, circuit breaker pattern

### Design

```go
// Supervisor detects infrastructure inconsistency
type InfrastructureChecker struct {
    supervisor *Supervisor
    backoff    *ExponentialBackoff
}

func (ic *InfrastructureChecker) CheckChildConsistency() error {
    // Example: Redpanda child vs Benthos child
    redpandaState := ic.getChildState("redpanda")
    benthosMetrics := ic.getChildMetrics("dfc_read")

    if redpandaState == "active" && benthosMetrics.OutputConnectionsActive == 0 {
        return fmt.Errorf("redpanda active but benthos has no output connections")
    }

    return nil
}

// Supervisor tick with circuit breaker
func (s *Supervisor) Tick(ctx context.Context) error {
    // Check infrastructure health
    if err := s.infraChecker.CheckChildConsistency(); err != nil {
        s.logger.Warn("Infrastructure inconsistency detected", "error", err)

        // Open circuit - stop ticking
        s.circuitOpen = true

        // Restart affected child
        if err := s.restartChild(ctx, "dfc_read"); err != nil {
            s.logger.Error("Failed to restart child", "error", err)
            s.infraChecker.backoff.RecordFailure()
            return nil // Don't propagate error to parent
        }

        // Wait for restart to complete
        time.Sleep(s.infraChecker.backoff.NextDelay())

        // Close circuit - resume ticking
        s.circuitOpen = false
        s.infraChecker.backoff.Reset()
        return nil
    }

    // Normal tick when circuit closed
    if s.circuitOpen {
        return nil // Skip tick
    }

    return s.tickWorkers(ctx)
}

func (s *Supervisor) restartChild(ctx context.Context, childName string) error {
    childSpec := s.childDeclarations[childName]

    // Graceful stop
    if err := childSpec.Supervisor.SetDesiredState("stopped"); err != nil {
        return err
    }

    // Wait for child to stop (with timeout)
    if err := s.waitForChildState(ctx, childName, "stopped", 10*time.Second); err != nil {
        return err
    }

    // Start child
    if err := childSpec.Supervisor.SetDesiredState("running"); err != nil {
        return err
    }

    return nil
}
```

### What Parent Sees

Parent business logic queries child state:

```go
func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    dfcState := snapshot.GetChildState("dfc_read")

    // Parent sees state transitions:
    // - Before inconsistency: "active"
    // - During restart: "stopped"
    // - After restart: "initializing" → "active"

    if dfcState != "active" {
        return s, SignalNone, nil  // Wait for DFC to be active
    }

    return &IdleState{}, SignalNone, nil
}
```

**Parent never knows:**
- Why child restarted
- That there was an infrastructure inconsistency
- That supervisor opened circuit and stopped ticking

### Pros

- ✅ **Perfect separation** - Infrastructure completely hidden from business logic
- ✅ **Automatic recovery** - Supervisor handles restarts without parent involvement
- ✅ **Natural state transitions** - Parent sees expected FSM lifecycle
- ✅ **Leverages existing patterns** - Reuses CollectorHealth backoff logic
- ✅ **Circuit breaker prevents cascading failures** - Stops ticking when infrastructure unstable

### Cons

- ⚠️ **Tick interruption** - Parent FSM stops ticking during recovery
- ⚠️ **Recovery latency** - Restart takes time (graceful stop + start)
- ⚠️ **Potential restart loops** - Needs backoff and start limits

---

## Pattern 2: Transparent Restart with Lifecycle States

### Concept

Add lifecycle states that indicate infrastructure operations:
- `lifecycle_restarting` - Supervisor is restarting child due to infrastructure issue
- `lifecycle_initializing` - Child is starting up
- `lifecycle_stopped` - Child is stopped

Parent FSM continues ticking but sees child in lifecycle state.

### Design

```go
// Enhanced state query returns lifecycle + business state
type ChildState struct {
    Lifecycle string // "running", "restarting", "stopped"
    Business  string // "active", "degraded", "error"
}

func (s *Supervisor) GetChildState(childName string) ChildState {
    childSpec := s.childDeclarations[childName]

    // Infrastructure layer - lifecycle
    lifecycle := s.getChildLifecycle(childName)

    // Business layer - FSM state
    business := childSpec.Supervisor.GetCurrentState()

    return ChildState{
        Lifecycle: lifecycle,
        Business:  business,
    }
}

// Supervisor maintains lifecycle separately
func (s *Supervisor) restartChildTransparent(ctx context.Context, childName string) {
    // Mark child as restarting (infrastructure state)
    s.childLifecycles[childName] = "restarting"

    // Perform restart
    s.stopChild(ctx, childName)
    s.startChild(ctx, childName)

    // Mark as running once started
    s.childLifecycles[childName] = "running"
}
```

### What Parent Sees

```go
func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    dfcState := snapshot.GetChildState("dfc_read")

    // Check lifecycle first (infrastructure)
    if dfcState.Lifecycle != "running" {
        return s, SignalNone, nil  // Wait for infrastructure
    }

    // Check business state
    if dfcState.Business != "active" {
        return s, SignalNone, nil  // Wait for business logic
    }

    return &IdleState{}, SignalNone, nil
}
```

### Pros

- ✅ **Parent continues ticking** - No circuit breaker interruption
- ✅ **Clear lifecycle visibility** - Parent knows child is restarting
- ✅ **Separation maintained** - Lifecycle (infra) vs Business (FSM) states

### Cons

- ❌ **Lifecycle states leak infrastructure** - Parent sees "restarting" (infrastructure detail)
- ❌ **API complexity** - Parent queries both lifecycle and business state
- ⚠️ **Violates separation principle** - Parent aware of infrastructure operations

**User concern:** This violates "infrastructure should be invisible" principle.

---

## Pattern 3: Infrastructure Health Layer

### Concept

Supervisor maintains **separate infrastructure health tracking** alongside business states:
- **Child business state:** "active" (owned by child FSM, immutable by supervisor)
- **Child infrastructure health:** "healthy", "inconsistent", "recovering" (owned by supervisor)

When infrastructure health is not "healthy", supervisor stops ticking child and initiates recovery.

### Design

```go
// Supervisor tracks infrastructure health separately
type ChildInfrastructureHealth struct {
    Status      string // "healthy", "inconsistent", "recovering"
    LastChecked time.Time
    IssueReason string
}

type Supervisor struct {
    childInfraHealth map[string]*ChildInfrastructureHealth
    // ... existing fields
}

func (s *Supervisor) checkChildInfrastructure(childName string) {
    health := s.childInfraHealth[childName]

    // Run sanity checks (cross-child consistency)
    if err := s.sanityCheckChild(childName); err != nil {
        health.Status = "inconsistent"
        health.IssueReason = err.Error()
        health.LastChecked = time.Now()

        s.logger.Warn("Child infrastructure inconsistent",
            "child", childName,
            "reason", err.Error())

        // Trigger recovery
        s.recoverChildInfrastructure(childName)
        return
    }

    health.Status = "healthy"
    health.LastChecked = time.Now()
}

func (s *Supervisor) recoverChildInfrastructure(childName string) {
    health := s.childInfraHealth[childName]
    health.Status = "recovering"

    // Restart child (stop → start)
    if err := s.restartChild(context.Background(), childName); err != nil {
        s.logger.Error("Infrastructure recovery failed", "child", childName, "error", err)
        return
    }

    health.Status = "healthy"
    s.logger.Info("Infrastructure recovery complete", "child", childName)
}

// Parent queries child state - infrastructure health is INVISIBLE
func (s *Supervisor) GetChildState(childName string) string {
    childSpec := s.childDeclarations[childName]

    // Parent ONLY sees business state from child FSM
    // Infrastructure health check happens in background
    return childSpec.Supervisor.GetCurrentState()
}
```

### What Parent Sees

```go
func (s *StartingDFCState) Next(snapshot Snapshot) (State, Signal, Action) {
    dfcState := snapshot.GetChildState("dfc_read")

    // Parent sees business states only
    // During infrastructure recovery, parent sees state transitions:
    // "active" → "stopped" → "initializing" → "active"

    if dfcState != "active" {
        return s, SignalNone, nil
    }

    return &IdleState{}, SignalNone, nil
}
```

### Pros

- ✅ **Perfect separation** - Infrastructure health completely hidden
- ✅ **Parent API unchanged** - Queries business state only
- ✅ **Background recovery** - Infrastructure issues handled automatically
- ✅ **Detailed health tracking** - Supervisor knows why/when health degraded

### Cons

- ⚠️ **Complexity** - Dual state tracking (business + infrastructure)
- ⚠️ **State sync** - Need to coordinate business state with infrastructure health
- ⚠️ **Potential confusion** - Developer may wonder about "recovering" status in logs

---

## Pattern 4: Fail-Stop Semantics

### Concept

**Erlang/OTP approach:** Treat infrastructure inconsistency as **child crash**. Apply restart strategy (immediate, backoff, give up).

### Design

```go
// When infrastructure issue detected, mark child as "crashed"
func (s *Supervisor) detectInfrastructureFailure(childName string) {
    if err := s.sanityCheckChild(childName); err != nil {
        s.logger.Warn("Child infrastructure failed, marking as crashed",
            "child", childName,
            "reason", err.Error())

        // Treat as crash - apply restart strategy
        s.handleChildCrash(childName, err)
    }
}

func (s *Supervisor) handleChildCrash(childName string, crashReason error) {
    restartStrategy := s.childDeclarations[childName].RestartStrategy

    switch restartStrategy {
    case "always":
        s.restartChild(context.Background(), childName)
    case "on_failure":
        if isTransientError(crashReason) {
            s.restartWithBackoff(childName)
        }
    case "never":
        s.logger.Error("Child crashed, no restart configured", "child", childName)
    }
}

func (s *Supervisor) restartWithBackoff(childName string) {
    attempts := s.restartAttempts[childName]
    if attempts >= s.maxRestartAttempts {
        s.logger.Error("Max restart attempts reached, giving up",
            "child", childName,
            "attempts", attempts)
        return
    }

    delay := time.Duration(math.Pow(2, float64(attempts))) * time.Second
    time.Sleep(delay)

    s.restartChild(context.Background(), childName)
    s.restartAttempts[childName]++
}
```

### What Parent Sees

Same as Pattern 1 - parent sees state transitions (active → stopped → initializing → active).

### Pros

- ✅ **Simplest model** - Infrastructure failure = crash
- ✅ **Proven pattern** - Erlang/OTP supervisor semantics
- ✅ **Clear restart policies** - Always, on_failure, never

### Cons

- ⚠️ **Aggressive** - Restarts for every inconsistency (may be overkill)
- ⚠️ **Potential disruption** - Restart interrupts child processing

---

## Comparison Matrix

| Criteria | Pattern 1: Circuit Breaker | Pattern 2: Lifecycle States | Pattern 3: Health Layer | Pattern 4: Fail-Stop |
|----------|---------------------------|----------------------------|------------------------|---------------------|
| **Infrastructure Hidden** | ✅ Completely | ❌ Leaks via lifecycle state | ✅ Completely | ✅ Completely |
| **Parent Ticking** | ❌ Stops during recovery | ✅ Continues | ✅ Continues | ❌ Stops during recovery |
| **Separation Quality** | ✅ Perfect | ❌ Mixed | ✅ Perfect | ✅ Perfect |
| **API Simplicity** | ✅ Single state query | ❌ Dual state query | ✅ Single state query | ✅ Single state query |
| **Recovery Speed** | ⚠️ Slower (circuit breaker delay) | ✅ Faster (transparent) | ✅ Faster (background) | ⚠️ Slower (backoff) |
| **Restart Loop Prevention** | ✅ Exponential backoff + limits | ✅ Backoff | ✅ Backoff | ✅ Backoff + max attempts |
| **Implementation Complexity** | ⚠️ Medium (circuit logic) | ⚠️ Medium (dual states) | ❌ High (dual tracking) | ✅ Low (simple restart) |
| **Proven Pattern** | ✅ Circuit breaker widely used | ❌ Novel pattern | ⚠️ Less common | ✅ Erlang/OTP standard |
| **Edge Case Handling** | ✅ Circuit prevents cascading | ⚠️ Needs careful handling | ✅ Health tracking helps | ⚠️ Can cascade failures |

---

## Recommended Approach

### Primary Recommendation: **Pattern 1 - Circuit Breaker with Restart**

**Rationale:**

1. **Perfect separation** - Infrastructure completely hidden from business logic
2. **Proven pattern** - Circuit breaker widely used in distributed systems
3. **Prevents cascading failures** - Opening circuit stops ticking when infrastructure unstable
4. **Reuses existing patterns** - Leverages FSMv2 CollectorHealth backoff logic
5. **Natural state transitions** - Parent sees expected FSM lifecycle states

### Implementation Details

```go
// pkg/fsmv2/supervisor/infrastructure_health.go

type InfrastructureHealthChecker struct {
    supervisor *Supervisor
    backoff    *ExponentialBackoff
    maxAttempts int
    logger     *zap.SugaredLogger
}

func NewInfrastructureHealthChecker(supervisor *Supervisor) *InfrastructureHealthChecker {
    return &InfrastructureHealthChecker{
        supervisor: supervisor,
        backoff:    NewExponentialBackoff(1*time.Second, 60*time.Second),
        maxAttempts: 5,
        logger:     supervisor.logger,
    }
}

// CheckChildConsistency runs sanity checks across children
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Example: Redpanda vs Benthos consistency
    redpandaState, err := ihc.supervisor.GetChildState("redpanda")
    if err != nil {
        return nil // Child not declared, skip check
    }

    benthosObserved, err := ihc.supervisor.GetChildObserved("dfc_read")
    if err != nil {
        return nil // Child not declared, skip check
    }

    // Type assert to get metrics
    benthosState, ok := benthosObserved.(DataFlowObservedState)
    if !ok {
        return nil // Can't check, skip
    }

    // Sanity check: Redpanda active but Benthos has no output connections
    if redpandaState == "active" && benthosState.OutputConnectionsActive == 0 {
        return fmt.Errorf("redpanda active but benthos has %d output connections",
            benthosState.OutputConnectionsActive)
    }

    return nil
}

// Supervisor tick with circuit breaker
func (s *Supervisor) Tick(ctx context.Context) error {
    // Infrastructure health check
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.logger.Warnf("Infrastructure inconsistency detected: %v", err)

        // Record failure for backoff
        s.infraHealthChecker.backoff.RecordFailure()

        // Check if max attempts reached
        if s.infraHealthChecker.backoff.Attempts() >= s.infraHealthChecker.maxAttempts {
            s.logger.Error("Max infrastructure recovery attempts reached, escalating to parent")
            // Escalate to parent FSM (set parent to error state)
            return fmt.Errorf("child infrastructure repeatedly failing: %w", err)
        }

        // Restart affected child
        if err := s.restartChild(ctx, "dfc_read"); err != nil {
            s.logger.Errorf("Failed to restart child: %v", err)
            return nil // Don't propagate to parent yet
        }

        // Wait backoff duration before next check
        delay := s.infraHealthChecker.backoff.NextDelay()
        s.logger.Infof("Infrastructure recovery initiated, waiting %v before resuming", delay)
        time.Sleep(delay)

        return nil // Skip normal tick
    }

    // Reset backoff on success
    s.infraHealthChecker.backoff.Reset()

    // Normal tick
    return s.tickWorkers(ctx)
}

func (s *Supervisor) restartChild(ctx context.Context, childName string) error {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return fmt.Errorf("child %s not declared", childName)
    }

    s.logger.Infof("Restarting child due to infrastructure issue", "child", childName)

    // Graceful stop
    if err := childSpec.Supervisor.SetDesiredState("stopped"); err != nil {
        return fmt.Errorf("failed to stop child: %w", err)
    }

    // Wait for child to stop (with timeout)
    stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    if err := s.waitForChildState(stopCtx, childName, "stopped"); err != nil {
        s.logger.Warnf("Child did not stop gracefully, forcing: %v", err)
        // Force stop if needed
    }

    // Start child
    if err := childSpec.Supervisor.SetDesiredState("running"); err != nil {
        return fmt.Errorf("failed to start child: %w", err)
    }

    s.logger.Infof("Child restarted successfully", "child", childName)
    return nil
}

func (s *Supervisor) waitForChildState(ctx context.Context, childName string, expectedState string) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            state, err := s.GetChildState(childName)
            if err != nil {
                return err
            }
            if state == expectedState {
                return nil
            }
        }
    }
}
```

### When to Use

**Trigger infrastructure health check when:**
1. Parent FSM transitions to operational states (Active, Idle)
2. On every tick (lightweight checks)
3. After child state transitions (verify consistency)

**Skip health check when:**
1. Parent FSM in lifecycle states (Starting, Stopping)
2. Circuit is already open (recovery in progress)
3. Child is not in operational state

---

## Concrete Examples

### Example 1: ProtocolConverter "Redpanda Active but Benthos No Connections"

**Scenario:** Redpanda reports "active" state, but Benthos DFC has zero Kafka output connections. This is an infrastructure inconsistency.

**Step-by-Step Recovery:**

```
T+0ms:   Supervisor ticks ProtocolConverter
         → infraHealthChecker.CheckChildConsistency()
         → Detects: redpanda="active", benthosOutputConnections=0
         → Error: "redpanda active but benthos has 0 output connections"

T+1ms:   Supervisor opens circuit breaker
         → Logs: "Infrastructure inconsistency detected"
         → backoff.RecordFailure() // Attempt 1

T+10ms:  Supervisor restarts dfc_read child
         → SetDesiredState("stopped")
         → Wait for state="stopped" (with 10s timeout)

T+2s:    Child stopped
         → SetDesiredState("running")

T+5s:    Child restarted, going through lifecycle
         → State transitions: "initializing" → "active"

T+6s:    Next tick
         → infraHealthChecker.CheckChildConsistency()
         → Success: redpanda="active", benthosOutputConnections=2
         → backoff.Reset()
         → Circuit closed, normal ticking resumes
```

**What Parent FSM Sees:**

```go
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    dfcState := snapshot.GetChildState("dfc_read")

    // T+0ms: "active"
    // T+2s:  "stopped"     (parent waits)
    // T+3s:  "initializing" (parent waits)
    // T+5s:  "active"      (parent proceeds)

    if dfcState != "active" {
        return s, SignalNone, nil  // Wait for DFC
    }

    return &IdleState{}, SignalNone, nil
}
```

**Logs:**

```
WARN  Infrastructure inconsistency detected: redpanda active but benthos has 0 output connections
      supervisor=protocol_converter_worker child=dfc_read

INFO  Restarting child due to infrastructure issue
      child=dfc_read

INFO  Child restarted successfully
      child=dfc_read

INFO  Infrastructure recovery initiated, waiting 1s before resuming
      supervisor=protocol_converter_worker
```

### Example 2: Persistent Infrastructure Issue (Bad Configuration)

**Scenario:** Parent sent bad configuration to child, causing repeated infrastructure failures.

**Step-by-Step:**

```
Attempt 1 (T+0s):   Detect inconsistency → Restart → Wait 1s → Check again
                     Still inconsistent

Attempt 2 (T+3s):   Detect inconsistency → Restart → Wait 2s → Check again
                     Still inconsistent

Attempt 3 (T+8s):   Detect inconsistency → Restart → Wait 4s → Check again
                     Still inconsistent

Attempt 4 (T+16s):  Detect inconsistency → Restart → Wait 8s → Check again
                     Still inconsistent

Attempt 5 (T+30s):  Detect inconsistency → Max attempts reached
                     ERROR: "Max infrastructure recovery attempts reached, escalating to parent"
                     → Parent FSM transitions to "error" state
                     → Parent state reason: "child infrastructure repeatedly failing"
```

**This prevents infinite restart loops** and escalates to parent for manual intervention.

---

## Restart Policies

### Exponential Backoff

Reuse existing FSMv2 CollectorHealth pattern:

```go
type ExponentialBackoff struct {
    baseDelay    time.Duration
    maxDelay     time.Duration
    attempts     int
    lastAttempt  time.Time
}

func (eb *ExponentialBackoff) NextDelay() time.Duration {
    if eb.attempts == 0 {
        return eb.baseDelay
    }

    delay := time.Duration(math.Pow(2, float64(eb.attempts))) * eb.baseDelay
    if delay > eb.maxDelay {
        delay = eb.maxDelay
    }

    return delay
}

func (eb *ExponentialBackoff) RecordFailure() {
    eb.attempts++
    eb.lastAttempt = time.Now()
}

func (eb *ExponentialBackoff) Reset() {
    eb.attempts = 0
}

func (eb *ExponentialBackoff) Attempts() int {
    return eb.attempts
}
```

**Backoff schedule:**
- Attempt 1: 1s
- Attempt 2: 2s
- Attempt 3: 4s
- Attempt 4: 8s
- Attempt 5: 16s
- Max: 60s (1 minute)

### Start Limits

Prevent infinite restart loops:

```go
const (
    MaxInfrastructureRecoveryAttempts = 5
    RecoveryAttemptWindow            = 5 * time.Minute
)

type InfrastructureHealthChecker struct {
    // ...
    maxAttempts     int
    attemptWindow   time.Duration
    windowStart     time.Time
}

func (ihc *InfrastructureHealthChecker) RecordFailure() {
    now := time.Now()

    // Reset window if last attempt was too long ago
    if now.Sub(ihc.windowStart) > ihc.attemptWindow {
        ihc.backoff.Reset()
        ihc.windowStart = now
    }

    ihc.backoff.RecordFailure()
}

func (ihc *InfrastructureHealthChecker) ShouldGiveUp() bool {
    return ihc.backoff.Attempts() >= ihc.maxAttempts
}
```

**Policy:** Max 5 restart attempts within 5-minute window. After 5 failures, escalate to parent.

### Escalation Strategy

When max attempts reached:

1. **Stop attempting recovery** - Don't restart child anymore
2. **Escalate to parent** - Return error from Supervisor.Tick()
3. **Parent transitions to error state** - Parent FSM handles degraded child
4. **Parent state reason explains issue** - "child infrastructure repeatedly failing: redpanda active but benthos has 0 output connections"

---

## Edge Cases

### Edge Case 1: Parent Caused the Infrastructure Issue

**Scenario:** Parent sent invalid configuration to child, causing infrastructure inconsistency.

**Problem:** Restarting child with same config causes infinite loop.

**Solution:**

```go
func (s *Supervisor) restartChild(ctx context.Context, childName string) error {
    // Before restart, validate configuration
    childSpec := s.childDeclarations[childName]

    if err := s.validateChildConfig(childSpec); err != nil {
        s.logger.Error("Child configuration invalid, cannot restart",
            "child", childName,
            "error", err)
        return fmt.Errorf("invalid child config: %w", err)
    }

    // Proceed with restart
    // ...
}

func (s *Supervisor) validateChildConfig(spec ChildSpec) error {
    // Example: Validate state mapping
    for parentState, childState := range spec.StateMapping {
        if !s.isValidState(childState) {
            return fmt.Errorf("invalid state mapping: %s → %s", parentState, childState)
        }
    }

    return nil
}
```

**If validation fails:**
1. Don't restart child (would fail again)
2. Escalate to parent immediately (don't burn through all retry attempts)
3. Parent transitions to error state with reason "invalid child configuration"

### Edge Case 2: Transient Network Issue

**Scenario:** Brief network partition causes Benthos to lose Kafka connection, but recovers on its own.

**Solution:**

```go
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Don't trigger restart on first detection
    // Wait to see if issue is transient

    if ihc.lastIssueDetected.IsZero() {
        ihc.lastIssueDetected = time.Now()
        return nil // Give it a grace period
    }

    if time.Since(ihc.lastIssueDetected) < 5*time.Second {
        return nil // Still in grace period
    }

    // Issue persisted for 5s, trigger restart
    return fmt.Errorf("persistent infrastructure inconsistency")
}
```

**Grace period:** Wait 5 seconds before triggering restart. If issue resolves within grace period, no restart needed.

### Edge Case 3: Cascading Failures

**Scenario:** Child A fails, causing Child B to fail, causing Child C to fail.

**Solution:**

Circuit breaker prevents cascading:

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    if s.circuitOpen {
        // Circuit open, don't tick children
        // This prevents cascading failures
        return nil
    }

    // Check infrastructure before ticking
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true // Open circuit immediately
        // ... handle recovery
    }

    // Tick children only when circuit closed
    return s.tickWorkers(ctx)
}
```

**Opening circuit stops all ticking**, preventing failures from propagating to other children.

### Edge Case 4: Child Restart Takes Too Long

**Scenario:** Child graceful stop takes 30 seconds, blocking supervisor tick.

**Solution:**

```go
func (s *Supervisor) restartChild(ctx context.Context, childName string) error {
    // Set aggressive timeout
    stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    // Attempt graceful stop
    childSpec.Supervisor.SetDesiredState("stopped")

    if err := s.waitForChildState(stopCtx, childName, "stopped"); err != nil {
        s.logger.Warn("Graceful stop timeout, forcing child termination",
            "child", childName)

        // Force stop by setting collector to failed state
        // This triggers immediate shutdown
        childSpec.Supervisor.ForceStop()
    }

    // Proceed with start
    // ...
}
```

**Timeout:** If graceful stop takes > 10s, force stop and proceed with restart.

---

## Implementation Phases

### Phase 1: Basic Circuit Breaker (Week 1)

- Add `InfrastructureHealthChecker` to Supervisor
- Implement `CheckChildConsistency()` for simple sanity checks
- Add circuit breaker logic to `Supervisor.Tick()`
- Implement basic `restartChild()` (stop → start)

### Phase 2: Exponential Backoff (Week 2)

- Add `ExponentialBackoff` to health checker
- Implement restart attempt tracking
- Add max attempts limit with escalation
- Add logging for backoff delays

### Phase 3: Edge Case Handling (Week 3)

- Add configuration validation before restart
- Implement grace period for transient issues
- Add timeout for graceful stop
- Test cascading failure prevention

### Phase 4: Metrics and Observability (Week 4)

- Add Prometheus metrics for infrastructure recovery
- Track recovery success rate, attempt count, duration
- Add detailed logging for debugging
- Create runbook for manual intervention

---

## Conclusion

**Pattern 1 (Circuit Breaker with Restart)** provides the cleanest separation between infrastructure and business logic:

- ✅ Infrastructure failures are completely hidden from parent business logic
- ✅ Supervisor handles recovery automatically through restarts
- ✅ Parent sees natural FSM state transitions (stopped → initializing → up)
- ✅ Circuit breaker prevents cascading failures
- ✅ Exponential backoff and start limits prevent infinite restart loops
- ✅ Escalation to parent when recovery repeatedly fails

This approach reuses existing FSMv2 patterns (CollectorHealth backoff) and aligns with established supervision patterns (Erlang/OTP, Kubernetes).

**Next steps:**
1. Implement basic circuit breaker in supervisor
2. Add sanity check examples (Redpanda vs Benthos consistency)
3. Test with ProtocolConverter use case
4. Iterate based on real-world behavior
