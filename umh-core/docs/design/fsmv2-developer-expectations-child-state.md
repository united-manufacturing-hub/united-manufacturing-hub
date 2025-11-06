# FSMv2 Developer Expectations for Child State

**Created:** 2025-11-02
**Status:** Design Investigation
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition
**Related:** `fsmv2-infrastructure-supervision-patterns.md`, `fsmv2-child-state-observation.md`

---

## Executive Summary

This document defines what developers should expect when querying child state in FSMv2 hierarchical composition. The core principle: **infrastructure should be completely invisible** to business logic.

**Key Finding:** Across all major systems (Kubernetes, Erlang/OTP, React, Docker Compose), developers expect to query child state without handling infrastructure concerns. Infrastructure failures are abstracted away by the supervisor layer.

**Recommended API:**
```go
// Simple state query - no infrastructure concerns
connectionState := snapshot.GetChildState("connection")
if connectionState != "up" {
    return s, SignalNone, nil  // Wait
}
```

---

## Table of Contents

1. [Developer Mental Model](#developer-mental-model)
2. [Infrastructure Abstraction Principles](#infrastructure-abstraction-principles)
3. [State Query Behavior Matrix](#state-query-behavior-matrix)
4. [Recommended API](#recommended-api)
5. [Supervisor Responsibilities](#supervisor-responsibilities)
6. [Comparison to Established Patterns](#comparison-to-established-patterns)
7. [What Developers Should NOT See](#what-developers-should-not-see)

---

## Developer Mental Model

### What Developers Expect

When a developer writes a parent FSM worker, they expect to:

1. **Declare children** in `DeriveDesiredState()`
2. **Query child state** in state transitions
3. **Make business logic decisions** based on child state

**What they do NOT expect:**
- Handling infrastructure failures (supervisor crashes, timeouts)
- Distinguishing "child stopped" vs "child crashed" vs "child unreachable"
- Implementing retry logic or backoff
- Dealing with stale state or collector timeouts

### Natural Developer Code

```go
func (s *StartingConnectionState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Developer thinks: "I need connection to be up before proceeding"

    connectionState := snapshot.GetChildState("connection")

    if connectionState != "up" {
        // Developer thinks: "Connection not ready yet, wait"
        return s, SignalNone, nil
    }

    // Developer thinks: "Connection ready, proceed to next step"
    return &StartingRedpandaState{}, SignalNone, nil
}
```

**Developer mental model:**
- "I query child state like checking a struct field"
- "If state not what I expect, I wait"
- "Supervisor handles everything else automatically"

---

## Infrastructure Abstraction Principles

### Principle 1: Infrastructure Failures Are Invisible

**User insight:**
> "infrastructure is supposed to only live in supervisor and the developer should not know about it, therefore it needs to work like a developer would expect it"

**What this means:**
- Child supervisor crash → Developer never knows
- Collector timeout → Developer never knows
- TriangularStore error → Developer never knows
- Network partition → Developer never knows

**How it works:**
- Supervisor handles all infrastructure failures automatically
- Supervisor retries, restarts, or escalates as needed
- Developer queries child state, gets answer (even if stale)

### Principle 2: Stale State is Acceptable

**Scenario:** Child supervisor crashed 30 seconds ago, collector stopped reporting.

**Developer query:**
```go
state := snapshot.GetChildState("connection")
// Returns: "up" (last known state from TriangularStore)
```

**Why this is OK:**
- Developer makes business decision based on last known state
- Supervisor detects infrastructure failure in background
- Supervisor restarts child automatically
- Developer sees state transition: "up" → "stopped" → "initializing" → "up"
- Developer doesn't know there was a crash (infrastructure detail)

### Principle 3: Error States Are Business Logic

**Distinction:**
- **Infrastructure error:** Collector timeout, supervisor crash
  - Handled by supervisor, invisible to developer
- **Business error:** Child FSM in "error" state
  - Visible to developer, part of business logic

```go
state := snapshot.GetChildState("connection")

if state == "error" {
    // Business logic error - connection child explicitly in error state
    // Developer handles this as business logic (e.g., transition to degraded)
    return &DegradedConnectionState{}, SignalNone, nil
}

// Infrastructure errors (supervisor crash, collector timeout) NEVER reach here
// Supervisor handles them invisibly
```

### Principle 4: State Transitions Are Always Valid

**Developer expectation:** Child state transitions follow FSM lifecycle.

**Valid transitions:**
- "stopped" → "initializing" → "up"
- "up" → "degraded" → "up"
- "up" → "error"

**Invalid transitions** (developer should NEVER see):
- "up" → "" (empty due to infrastructure failure)
- "up" → nil (supervisor crashed)
- "up" → "infrastructure_unhealthy" (mixing concerns)

**Supervisor's job:** Ensure state transitions are always valid, even during infrastructure failures.

---

## State Query Behavior Matrix

### Scenario 1: Child Healthy, in "up" State

**Infrastructure:** Child supervisor running, collector reporting, TriangularStore synced

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:** `"up"`

**Developer sees:** Normal business state

---

### Scenario 2: Child Healthy, in "error" State (Business Logic)

**Infrastructure:** Child supervisor running, collector reporting

**Business:** Child FSM determined it's in error state (e.g., connection timed out)

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:** `"error"`

**Developer sees:** Business logic error, handles accordingly

---

### Scenario 3: Child Supervisor Crashed (Infrastructure)

**Infrastructure:** Child supervisor process crashed, collector not running

**What supervisor does:**
1. Detects crash via health check
2. Restarts child (stop → start lifecycle)
3. Logs infrastructure event (DEBUG/WARN level)

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:**
- During crash: `"up"` (last persisted state from TriangularStore)
- During restart: `"stopped"` or `"initializing"`
- After restart: `"up"` (back to normal)

**Developer sees:** State transition from "up" → "stopped" → "initializing" → "up"

**Developer does NOT see:**
- That supervisor crashed
- Why restart happened
- Infrastructure health details

---

### Scenario 4: Collector Timeout (Infrastructure)

**Infrastructure:** Child collector hasn't reported in 30 seconds

**What supervisor does:**
1. Uses cached state from last successful collection
2. Logs staleness warning (WARN level)
3. Retries collector with exponential backoff
4. If timeout persists, restarts child

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:** `"up"` (cached state, possibly stale)

**Developer sees:** Cached state until supervisor restarts child

**Developer does NOT see:**
- Collector timeout
- Staleness
- Retry attempts

---

### Scenario 5: Child Not Yet Created

**Infrastructure:** Parent called `DeriveDesiredState()` declaring child, but supervisor hasn't created it yet

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:** `""` (empty string) or `"initializing"`

**Developer sees:** Child not ready yet

**Developer handles:**
```go
if state == "" || state == "initializing" {
    return s, SignalNone, nil  // Wait for child to be created
}
```

---

### Scenario 6: Child Being Removed

**Infrastructure:** Parent stopped declaring child in `DeriveDesiredState()`, supervisor is removing it

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:**
- Option A: `""` (empty - child no longer exists)
- Option B: Last known state until fully removed
- Option C: `"removing"` (lifecycle state)

**Recommended:** Option A (empty string) - cleanest, child doesn't exist anymore

**Developer handles:**
```go
if state == "" {
    // Child was removed, transition accordingly
    return &IdleState{}, SignalNone, nil
}
```

---

### Scenario 7: TriangularStore Error (Infrastructure)

**Infrastructure:** TriangularStore query failed (disk error, corruption, etc.)

**What supervisor does:**
1. Logs error (ERROR level)
2. Returns last cached state
3. Retries query
4. If persistent, escalates to parent (triggers parent error state)

**Query:**
```go
state := snapshot.GetChildState("connection")
```

**Returns:** Cached state from previous successful query

**Developer sees:** Stale state (acceptable for business logic)

**Developer does NOT see:**
- TriangularStore error
- Retry attempts
- Cache usage

---

## Recommended API

### Simple State Query (Primary)

```go
// Returns: state name (string) or "" if child doesn't exist
func (s *Supervisor) GetChildState(childName string) string
```

**Usage:**
```go
connectionState := snapshot.supervisor.GetChildState("connection")

if connectionState == "" {
    // Child doesn't exist yet
    return s, SignalNone, nil
}

if connectionState != "up" {
    // Child not ready
    return s, SignalNone, nil
}

// Child ready, proceed
return &StartingRedpandaState{}, SignalNone, nil
```

### Alternative: State Query with Exists Check

```go
// Returns: (state, exists)
func (s *Supervisor) GetChildState(childName string) (string, bool)
```

**Usage:**
```go
connectionState, exists := snapshot.supervisor.GetChildState("connection")

if !exists {
    // Child not declared or doesn't exist
    return s, SignalNone, nil
}

if connectionState != "up" {
    // Child exists but not ready
    return s, SignalNone, nil
}

// Child ready
return &StartingRedpandaState{}, SignalNone, nil
```

### Recommendation

**Use simple string return** (returns `""` if child doesn't exist):
- Matches "natural" developer expectations
- Consistent with how optional values are handled in Go
- No need for error handling or boolean checks
- Cleaner code

```go
// Simple and natural
if snapshot.supervisor.GetChildState("connection") != "up" {
    return s, SignalNone, nil
}
```

vs.

```go
// More verbose
state, exists := snapshot.supervisor.GetChildState("connection")
if !exists || state != "up" {
    return s, SignalNone, nil
}
```

---

## Supervisor Responsibilities

### What Supervisor Handles Automatically

1. **Infrastructure Monitoring**
   - Child supervisor health checks
   - Collector timeout detection
   - TriangularStore query failures
   - Cross-child consistency checks (sanity checks)

2. **Automatic Recovery**
   - Restart crashed child supervisors
   - Restart child collectors on timeout
   - Retry TriangularStore queries with backoff
   - Escalate to parent when recovery repeatedly fails

3. **State Management**
   - Persist child state to TriangularStore
   - Cache state for fast queries
   - Return last known state on infrastructure failure
   - Track staleness and log warnings

4. **Logging**
   - Infrastructure events at DEBUG/WARN/ERROR level
   - Restart attempts and backoff delays
   - Escalation to parent
   - Recovery success/failure

5. **Metrics**
   - Infrastructure health status
   - Restart attempt count
   - Recovery duration
   - State staleness

### What Supervisor Does NOT Handle

1. **Business Logic**
   - Interpreting child state names
   - Making transition decisions based on child state
   - Determining what "healthy" means for business logic

2. **Child State Ownership**
   - Cannot override child FSM states
   - Cannot inject infrastructure states into business state machine
   - Cannot force child to specific state (except desired state changes)

---

## Comparison to Established Patterns

### Kubernetes Controllers

**Pattern:** Controller queries Pod status, sees phases (Pending, Running, Failed)

```yaml
# Pod crashed, kubelet restarts it
# Controller sees phase transitions: Running → Pending → Running
# Controller doesn't know WHY restart happened
```

**Parent controller code:**
```go
if pod.Status.Phase != corev1.Running {
    return ctrl.Result{Requeue: true}, nil  // Wait
}
// Pod running, proceed
```

**Infrastructure handled by kubelet:**
- Liveness/readiness probes
- Restart on failure
- Container crash detection
- Resource limits enforcement

**Matches FSMv2 pattern:** ✅ Parent sees state transitions, infrastructure invisible

---

### Erlang/OTP Supervisors

**Pattern:** Supervisor detects child crash, applies restart strategy

```erlang
% Child gen_server crashes
% Supervisor restarts it automatically
% Parent gen_server sees child go through init again
% Parent doesn't know there was a crash
```

**Parent gen_server code:**
```erlang
case gen_server:call(ChildPid, get_state) of
    {ok, ready} -> proceed();
    _ -> {noreply, State}  % Wait
end
```

**Infrastructure handled by supervisor:**
- Crash detection
- Restart strategy (one_for_one, rest_for_one, etc.)
- Restart frequency limits
- Escalation to parent supervisor

**Matches FSMv2 pattern:** ✅ Crash recovery invisible, parent sees state changes

---

### React Error Boundaries

**Pattern:** Child component errors, error boundary catches and recovers

```jsx
// Child component throws error
// Error boundary catches it
// Parent component sees fallback UI or recovered state
// Parent doesn't handle error directly
```

**Parent component code:**
```jsx
function ParentComponent() {
    return (
        <div>
            {childState.isReady ? <NextStep /> : <Loading />}
        </div>
    );
}
```

**Infrastructure handled by error boundary:**
- Error catching
- Fallback rendering
- Recovery attempts
- Logging

**Matches FSMv2 pattern:** ✅ Parent uses state, error handling invisible

---

### Docker Compose Healthchecks

**Pattern:** Dependent service waits for dependency to be healthy

```yaml
depends_on:
  database:
    condition: service_healthy

healthcheck:
  test: ["CMD", "pg_isready"]
  interval: 10s
  retries: 5
```

**Parent service:**
- Waits for dependency to be "healthy"
- Doesn't know if dependency restarted
- Doesn't handle health check failures

**Infrastructure handled by Docker:**
- Health check execution
- Retry logic
- Restart on failure
- Status reporting

**Matches FSMv2 pattern:** ✅ Parent queries status, health checks invisible

---

## What Developers Should NOT See

### ❌ Infrastructure States

```go
// BAD - Leaks infrastructure into business logic
state := snapshot.supervisor.GetChildState("connection")

if state == "infrastructure_restarting" {  // ❌ Infrastructure state
    return s, SignalNone, nil
}

if state == "collector_timeout" {  // ❌ Infrastructure failure
    return s, SignalNone, nil
}

if state == "supervisor_crashed" {  // ❌ Infrastructure failure
    return s, SignalNone, nil
}
```

**Why bad:** Developer has to handle infrastructure concerns.

**Correct:**
```go
// GOOD - Only business states
state := snapshot.supervisor.GetChildState("connection")

if state != "up" {
    // Could be stopped, initializing, down, error, etc.
    // Developer doesn't care WHY, just waits
    return s, SignalNone, nil
}
```

---

### ❌ Infrastructure Health Checks

```go
// BAD - Exposes infrastructure health
if !snapshot.supervisor.IsChildHealthy("connection") {  // ❌ Infrastructure API
    return s, SignalNone, nil
}

connectionState := snapshot.supervisor.GetChildState("connection")
if connectionState != "up" {
    return s, SignalNone, nil
}
```

**Why bad:** Developer has to check infrastructure health.

**Correct:**
```go
// GOOD - Only business state
connectionState := snapshot.supervisor.GetChildState("connection")

if connectionState != "up" {
    // Supervisor handles health checks internally
    // If unhealthy, supervisor restarts child
    // Developer sees state transitions
    return s, SignalNone, nil
}
```

---

### ❌ Error Handling for Infrastructure

```go
// BAD - Developer handles infrastructure errors
state, err := snapshot.supervisor.GetChildState("connection")

if err != nil {  // ❌ Infrastructure error exposed
    if errors.Is(err, ErrSupervisorCrashed) {
        // Developer handles supervisor crash?
        return &ErrorState{}, SignalNone, nil
    }
    if errors.Is(err, ErrCollectorTimeout) {
        // Developer handles collector timeout?
        return &ErrorState{}, SignalNone, nil
    }
}
```

**Why bad:** Infrastructure errors leak into business logic.

**Correct:**
```go
// GOOD - No error handling needed
state := snapshot.supervisor.GetChildState("connection")

// Supervisor handles infrastructure errors internally
// Returns last known state even on infrastructure failure
// Developer never sees infrastructure errors

if state != "up" {
    return s, SignalNone, nil
}
```

---

### ❌ Retry Logic

```go
// BAD - Developer implements retry
state := snapshot.supervisor.GetChildState("connection")

if state == "" {
    s.retryCount++
    if s.retryCount > 5 {  // ❌ Developer handles retries
        return &ErrorState{}, SignalNone, nil
    }
    return s, SignalNone, nil
}
```

**Why bad:** Retry logic is infrastructure concern.

**Correct:**
```go
// GOOD - Supervisor handles retries
state := snapshot.supervisor.GetChildState("connection")

if state == "" {
    // Supervisor retries internally with exponential backoff
    // If retries exhausted, supervisor escalates to parent
    // Developer just waits
    return s, SignalNone, nil
}
```

---

## Implementation Guidelines

### 1. GetChildState() Never Returns Errors

```go
// CORRECT signature
func (s *Supervisor) GetChildState(childName string) string

// INCORRECT signature
func (s *Supervisor) GetChildState(childName string) (string, error)
```

**Why:** Infrastructure errors should not propagate to business logic.

**How supervisor handles errors:**
```go
func (s *Supervisor) GetChildState(childName string) string {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return ""  // Child not declared
    }

    // Try to query child supervisor
    state := childSpec.Supervisor.GetCurrentState()
    if state == "" {
        // Supervisor may have crashed, return last known state
        cached, err := s.store.LoadIdentity(ctx, childSpec.WorkerType, childSpec.ID)
        if err != nil {
            s.logger.Warn("Failed to load child state, returning empty",
                "child", childName,
                "error", err)
            return ""  // Infrastructure error, return empty
        }
        return cached.State
    }

    return state
}
```

### 2. Supervisor Logs Infrastructure Events

```go
// Infrastructure events logged at appropriate levels
s.logger.Debug("Child state queried", "child", childName, "state", state)
s.logger.Warn("Child supervisor not responding, using cached state", "child", childName)
s.logger.Error("Failed to restart child after max attempts", "child", childName, "attempts", attempts)
```

**Developer sees:** Only state transitions in their FSM logic
**Operator sees:** Detailed infrastructure logs for debugging

### 3. Sentinel Values for Special Cases

```go
const (
    StateEmpty        = ""            // Child doesn't exist
    StateInitializing = "initializing" // Child being created
    StateStopped      = "stopped"      // Child stopped
    StateError        = "error"        // Child in error state (business logic)
)
```

**Never use:**
- `"infrastructure_error"` - Mixes concerns
- `"crashed"` - Infrastructure detail
- `"timeout"` - Infrastructure detail
- `"unhealthy"` - Ambiguous (business or infrastructure?)

---

## Conclusion

**Developer expectations for FSMv2 child state queries:**

1. ✅ **Simple API** - `GetChildState()` returns string, no errors
2. ✅ **Infrastructure invisible** - Supervisor handles crashes, timeouts, retries
3. ✅ **Stale state acceptable** - Last known state returned on infrastructure failure
4. ✅ **Natural state transitions** - Developer sees lifecycle states, not infrastructure states
5. ✅ **No boilerplate** - No health checks, no error handling, no retry logic

**Supervisor responsibilities:**

1. ✅ **Monitor infrastructure** - Health checks, collector timeouts, crashes
2. ✅ **Automatic recovery** - Restart children, retry with backoff, escalate when needed
3. ✅ **State management** - Persist, cache, return last known state
4. ✅ **Logging** - Detailed infrastructure events for operators

**This approach matches universal patterns** from Kubernetes, Erlang/OTP, React, and Docker Compose - infrastructure is abstracted away, developers focus on business logic.
