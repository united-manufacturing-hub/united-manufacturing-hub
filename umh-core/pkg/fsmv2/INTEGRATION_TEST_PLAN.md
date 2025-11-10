# FSM v2 Integration Test Plan

> **For Claude:** Use TDD workflow (RED-GREEN-REFACTOR) for all test implementation. Follow `skills/testing/test-driven-development/SKILL.md` strictly.

**Goal:** Build comprehensive integration test suite for FSM v2 covering all edge cases, race conditions, and failure modes.

**Architecture:** Two example workers (example-parent, example-child) demonstrating parent-child relationships, state transitions, dependencies, and resource lifecycle. Tests verify supervisor coordination, action execution, state collection, and error handling across the full system.

**Tech Stack:**
- Ginkgo (BDD test framework)
- Gomega (matcher library)
- Go race detector (`-race`)
- Go coverage tools (`go test -cover`)

**Created:** 2025-11-10 16:00

**Last Updated:** 2025-11-10 16:00

---

## Executive Summary

### Overview

This plan defines a systematic approach to building a comprehensive integration test suite for FSM v2. The test suite uses two example workers (example-parent and example-child) to verify all edge cases, race conditions, and failure modes that can occur in production.

### Test Count Breakdown

**Total Tests:** ~90 tests across 8 categories and 5 phases

| Category | Count | Priority | Focus |
|----------|-------|----------|-------|
| **1. Critical Failures** | 20 | P0 | System hangs, deadlocks, resource leaks |
| **2. High-Priority Edge Cases** | 25 | P1 | Race conditions, timing issues, concurrent state changes |
| **3. Medium-Priority Edge Cases** | 20 | P2 | Error propagation, recovery, state consistency |
| **4. Low-Priority Edge Cases** | 15 | P3 | Observer notifications, metrics, edge conditions |
| **5. Integration & End-to-End** | 10 | P1 | Full system workflows, multi-worker scenarios |

### Estimated Timeline

**Total: ~15 days (3 weeks)**

- **Phase 1 (Critical Failures):** 4 days
- **Phase 2 (High-Priority Edge Cases):** 5 days
- **Phase 3 (Medium-Priority Edge Cases):** 4 days
- **Phase 4 (Low-Priority Edge Cases):** 3 days
- **Phase 5 (Integration & E2E):** 2 days
- **Buffer:** 2 days for unexpected issues

### Success Criteria

- [ ] All 90 tests passing
- [ ] >80% coverage on supervisor, action_executor, collector
- [ ] No race conditions detected (`go test -race`)
- [ ] No resource leaks detected (goroutine leak tests)
- [ ] No deadlocks in 1000+ test runs
- [ ] All critical production scenarios covered
- [ ] Documentation complete (this document + inline comments)

---

## Example Workers Design

### example-parent Worker

**Purpose:** Demonstrates parent worker that manages children, depends on ConfigLoader, and handles complex state transitions.

**Dependencies:**
- `ConfigLoader` (dependency injection)

**States:**
```
Stopped         → Initial state, no children spawned
TryingToStart   → Loading config, spawning children
Running         → Children active, normal operation
Degraded        → Some children failed, partial functionality
TryingToStop    → Shutting down children, cleanup
```

**Actions:**
- `StartAction` - Load config, spawn children (returns ChildrenSpecs)
- `StopAction` - Gracefully stop children, cleanup resources

**State Transitions:**
```
Stopped → StartAction → TryingToStart
TryingToStart → (success) → Running
TryingToStart → (partial failure) → Degraded
TryingToStart → (total failure) → Stopped
Running → StopAction → TryingToStop
TryingToStop → (success) → Stopped
Degraded → StopAction → TryingToStop
Any → (emergency) → Stopped
```

**Child Management:**
- Returns `ChildrenSpecs` from `DeriveDesiredState` when in Running/Degraded
- Spawns 2 example-child workers with unique configs
- Monitors child health, transitions to Degraded if child fails
- Stops all children during shutdown

**Key Behaviors:**
- Demonstrates dependency injection pattern
- Shows parent-child lifecycle coordination
- Handles partial failures (Degraded state)
- Demonstrates action that spawns children

**Files:**
```
pkg/fsmv2/examples/workers/example-parent/
├── worker.go                    # Worker implementation
├── actions.go                   # StartAction, StopAction
├── states.go                    # State definitions
└── config.go                    # Config types
```

### example-child Worker

**Purpose:** Demonstrates child worker that manages external resources (connections), handles reconnection logic, and responds to parent control.

**Dependencies:**
- `ConnectionPool` (dependency injection)

**States:**
```
Stopped        → Initial state, not connected
TryingToConnect → Establishing connection
Connected       → Active connection, processing data
Disconnected    → Connection lost, may auto-reconnect
TryingToStop    → Closing connection, cleanup
```

**Actions:**
- `ConnectAction` - Establish connection to external resource
- `DisconnectAction` - Close connection gracefully

**State Transitions:**
```
Stopped → ConnectAction → TryingToConnect
TryingToConnect → (success) → Connected
TryingToConnect → (failure) → Disconnected
Connected → (connection lost) → Disconnected
Disconnected → (auto-retry) → TryingToConnect
Connected → DisconnectAction → TryingToStop
TryingToStop → (success) → Stopped
Any → (parent stopped) → Stopped
```

**Connection Handling:**
- Retries on connection failure (exponential backoff)
- Detects connection loss and attempts reconnection
- Graceful shutdown with connection cleanup
- Reports connection metrics to parent

**Key Behaviors:**
- Demonstrates resource lifecycle management
- Shows retry/reconnection patterns
- Handles external dependency failures
- Responds to parent shutdown signals

**Files:**
```
pkg/fsmv2/examples/workers/example-child/
├── worker.go                    # Worker implementation
├── actions.go                   # ConnectAction, DisconnectAction
├── states.go                    # State definitions
└── connection.go                # Connection handling
```

### Folder Structure

```
pkg/fsmv2/
├── examples/
│   ├── workers/
│   │   ├── example-parent/
│   │   │   ├── worker.go
│   │   │   ├── actions.go
│   │   │   ├── states.go
│   │   │   └── config.go
│   │   └── example-child/
│   │       ├── worker.go
│   │       ├── actions.go
│   │       ├── states.go
│   │       └── connection.go
│   └── integration_test/
│       ├── phase1_critical_test.go
│       ├── phase2_high_priority_test.go
│       ├── phase3_medium_priority_test.go
│       ├── phase4_low_priority_test.go
│       ├── phase5_integration_test.go
│       └── test_helpers.go
├── supervisor.go
├── action_executor.go
├── collector.go
└── INTEGRATION_TEST_PLAN.md  # This document
```

---

## Test Categories

### Category 1: Critical Failures (System Hangs/Deadlocks)

**Why It Matters:** These are the highest-severity production failures. A deadlock or hang requires manual intervention (restart), causes downtime, and can cascade to other systems. These must be prevented at all costs.

**Production Impact:**
- 🔴 **Severity: P0** - Requires immediate intervention
- System becomes unresponsive
- Workers stuck in transitional states forever
- Resource leaks accumulate over time
- Manual restart required (downtime)
- Can cascade to dependent systems

#### Tests (20 total)

**1.1 Action Never Completes**
- **Description:** Action enters "running" state but never returns (blocked on I/O, infinite loop, etc.)
- **Expected:** Supervisor should timeout action execution, transition worker to error state, not hang forever
- **TDD Steps:**
  - RED: Write test that blocks action forever, expect timeout after X seconds
  - Verify RED: Test hangs forever (proves timeout doesn't exist)
  - GREEN: Add timeout mechanism to action_executor
  - Verify GREEN: Test times out correctly, supervisor continues
  - REFACTOR: Extract timeout config, add logging

**1.2 Action Timeout During Parent Stop**
- **Description:** Parent stopping but child's DisconnectAction times out
- **Expected:** Parent should force-stop child after timeout, not hang waiting
- **TDD Steps:**
  - RED: Parent stop waits for child action that never completes
  - Verify RED: Test hangs forever
  - GREEN: Add force-stop after timeout
  - Verify GREEN: Parent stops successfully after timeout
  - REFACTOR: Centralize timeout logic

**1.3 State Deadlock (Circular Wait)**
- **Description:** Worker A waits for B, B waits for C, C waits for A
- **Expected:** Detect circular dependency, fail fast with error
- **TDD Steps:**
  - RED: Create 3 workers with circular wait pattern
  - Verify RED: Test deadlocks
  - GREEN: Add dependency cycle detection
  - Verify GREEN: Fast failure with cycle error
  - REFACTOR: Add cycle detection unit tests

**1.4 Channel Blocking (No Receiver)**
- **Description:** Worker sends to channel with no receiver, blocks forever
- **Expected:** Use non-blocking sends or buffered channels, log dropped messages
- **TDD Steps:**
  - RED: Worker sends to channel with no receiver
  - Verify RED: Goroutine leak (worker blocked forever)
  - GREEN: Use select with default case for non-blocking send
  - Verify GREEN: Worker doesn't block, message logged as dropped
  - REFACTOR: Create helper for non-blocking channel send

**1.5 Channel Blocking (Buffer Full)**
- **Description:** Buffered channel fills up, sender blocks
- **Expected:** Handle buffer-full case explicitly (drop, log, apply backpressure)
- **TDD Steps:**
  - RED: Fill channel buffer, verify sender blocks
  - Verify RED: Goroutine leak detected
  - GREEN: Check buffer space before send, handle full case
  - Verify GREEN: No blocking, backpressure applied
  - REFACTOR: Create channel wrapper with backpressure

**1.6 Mutex Deadlock (Lock Order Violation)**
- **Description:** Thread A locks mutex1 then mutex2, Thread B locks mutex2 then mutex1
- **Expected:** Enforce lock ordering or detect deadlock
- **TDD Steps:**
  - RED: Create lock order violation scenario
  - Verify RED: Deadlock detected (test timeout)
  - GREEN: Enforce lock ordering by priority
  - Verify GREEN: No deadlock, locks acquired in order
  - REFACTOR: Document lock ordering in comments

**1.7 Goroutine Leak (Action Never Returns)**
- **Description:** Launched goroutine blocks forever, never cleaned up
- **Expected:** All goroutines should either complete or be cancellable
- **TDD Steps:**
  - RED: Launch goroutine that blocks forever, check count after test
  - Verify RED: Goroutine count increases, leak detected
  - GREEN: Add context cancellation to all goroutines
  - Verify GREEN: Goroutine count returns to baseline
  - REFACTOR: Create goroutine launch helper with context

**1.8 Goroutine Leak (Forgotten Context Cancel)**
- **Description:** Context created but cancel function never called
- **Expected:** Defer cancel() immediately after context creation
- **TDD Steps:**
  - RED: Create context, start goroutine, don't call cancel
  - Verify RED: Goroutine leak detected
  - GREEN: Add defer cancel() after context creation
  - Verify GREEN: No leak, goroutine stops
  - REFACTOR: Add linter rule for missing defer cancel()

**1.9 Supervisor Hang (Worker Never Responds)**
- **Description:** Supervisor waits for worker state update, worker never responds
- **Expected:** Timeout waiting for worker, mark as unhealthy
- **TDD Steps:**
  - RED: Worker stops responding to supervisor, check if supervisor hangs
  - Verify RED: Supervisor blocks forever
  - GREEN: Add timeout to supervisor's wait-for-state
  - Verify GREEN: Supervisor times out, marks worker failed
  - REFACTOR: Centralize supervisor timeout config

**1.10 Action Blocked on Full Queue**
- **Description:** Action queue is full, new action blocks forever
- **Expected:** Reject new actions with error when queue full (no blocking)
- **TDD Steps:**
  - RED: Fill action queue, submit new action, verify blocking
  - Verify RED: Action submission blocks forever
  - GREEN: Use non-blocking send, return error if full
  - Verify GREEN: Action rejected immediately, no blocking
  - REFACTOR: Add queue capacity monitoring

**1.11 Parent Shutdown Waits Forever**
- **Description:** Parent tries to stop but child never responds to stop signal
- **Expected:** Force-kill child after timeout, parent proceeds
- **TDD Steps:**
  - RED: Child ignores stop signal, parent waits forever
  - Verify RED: Parent hangs during shutdown
  - GREEN: Add force-kill timeout in parent
  - Verify GREEN: Parent force-stops child, completes shutdown
  - REFACTOR: Extract force-kill logic to utility

**1.12 Supervisor Restart Loop (Immediate Crash)**
- **Description:** Worker crashes immediately on start, supervisor restarts forever
- **Expected:** Exponential backoff, give up after N attempts
- **TDD Steps:**
  - RED: Worker crashes on start, supervisor restarts 1000x
  - Verify RED: Infinite restart loop
  - GREEN: Add exponential backoff and max retry limit
  - Verify GREEN: Supervisor gives up after N retries
  - REFACTOR: Add restart policy config

**1.13 Resource Leak (File Descriptors)**
- **Description:** Action opens file/socket but never closes it
- **Expected:** All resources closed on action completion/cancellation
- **TDD Steps:**
  - RED: Action opens file, crashes, check open FD count
  - Verify RED: FD count increases on each crash
  - GREEN: Use defer file.Close(), track FDs
  - Verify GREEN: FD count returns to baseline
  - REFACTOR: Create resource tracking helper

**1.14 Resource Leak (Memory)**
- **Description:** Action allocates memory but never frees it
- **Expected:** Memory released on action completion
- **TDD Steps:**
  - RED: Run action 1000x, check memory growth
  - Verify RED: Memory grows unbounded
  - GREEN: Ensure all allocations are scoped to action lifetime
  - Verify GREEN: Memory returns to baseline
  - REFACTOR: Add memory profiling to tests

**1.15 Double Stop Deadlock**
- **Description:** StopAction called twice on same worker
- **Expected:** Second stop is no-op, doesn't block or error
- **TDD Steps:**
  - RED: Call stop twice, verify second call behavior
  - Verify RED: Deadlock or panic on second stop
  - GREEN: Add stopped flag, check before executing stop
  - Verify GREEN: Second stop returns immediately
  - REFACTOR: Add state guards for idempotent actions

**1.16 Race Condition (Shared State)**
- **Description:** Two actions modify shared state without synchronization
- **Expected:** Use mutex or atomic operations, no data races
- **TDD Steps:**
  - RED: Run with `-race`, trigger concurrent state modification
  - Verify RED: Race detector reports data race
  - GREEN: Add mutex around shared state access
  - Verify GREEN: No race detected
  - REFACTOR: Audit all shared state for races

**1.17 Panic in Action (Not Recovered)**
- **Description:** Action panics, takes down entire worker or supervisor
- **Expected:** Recover panic, transition worker to error state
- **TDD Steps:**
  - RED: Action panics, verify supervisor crashes
  - Verify RED: Supervisor dies, test fails
  - GREEN: Add defer recover() in action execution
  - Verify GREEN: Supervisor survives, worker in error state
  - REFACTOR: Add panic logging and metrics

**1.18 Panic in Observer (Not Recovered)**
- **Description:** Observer callback panics, blocks state updates
- **Expected:** Recover panic, continue notifying other observers
- **TDD Steps:**
  - RED: Observer panics, verify other observers don't get notified
  - Verify RED: State update stops at panicking observer
  - GREEN: Add recover() around each observer call
  - Verify GREEN: Other observers still notified
  - REFACTOR: Add observer error metrics

**1.19 Context Cancelled But Action Continues**
- **Description:** Context cancelled but action doesn't check ctx.Done()
- **Expected:** Action should check context regularly, return on cancellation
- **TDD Steps:**
  - RED: Cancel context, verify action still runs to completion
  - Verify RED: Action ignores cancellation
  - GREEN: Add ctx.Done() checks in action loop
  - Verify GREEN: Action stops promptly on cancellation
  - REFACTOR: Add context check linter rule

**1.20 Stuck in Transitional State Forever**
- **Description:** Worker enters TryingToStart but never leaves
- **Expected:** Timeout transitional states, fallback to error/stopped
- **TDD Steps:**
  - RED: Worker stuck in TryingToStart, verify no timeout
  - Verify RED: Worker stays in transitional state forever
  - GREEN: Add transitional state timeout
  - Verify GREEN: Worker transitions to error state after timeout
  - REFACTOR: Add state machine timeout config

---

### Category 2: High-Priority Edge Cases (Race Conditions)

**Why It Matters:** Race conditions cause intermittent failures that are extremely difficult to debug in production. They pass 99% of the time but fail unpredictably under load, causing customer-visible bugs and eroding trust in the system.

**Production Impact:**
- 🟠 **Severity: P1** - Intermittent failures under load
- Hard to reproduce in dev/staging
- Causes flaky behavior ("it worked yesterday!")
- Erodes customer trust
- Difficult to diagnose in production
- Can cause data corruption

#### Tests (25 total)

**2.1 Concurrent Start/Stop**
- **Description:** Start and Stop actions triggered simultaneously
- **Expected:** One wins, other is no-op or queued
- **TDD Steps:**
  - RED: Trigger Start and Stop concurrently, check for races
  - Verify RED: Data race detected or inconsistent state
  - GREEN: Add state lock, check current state before action
  - Verify GREEN: One action wins cleanly, no race
  - REFACTOR: Document state transition guards

**2.2 State Change During Action Execution**
- **Description:** State changes while action is running (e.g., stop during start)
- **Expected:** Action observes cancellation, stops gracefully
- **TDD Steps:**
  - RED: Change state during action, verify action doesn't observe it
  - Verify RED: Action completes despite state change
  - GREEN: Pass context to action, check ctx.Done() regularly
  - Verify GREEN: Action stops on state change
  - REFACTOR: Add context checking to action template

**2.3 Multiple Observers Racing to Update**
- **Description:** Multiple observers notified simultaneously, modify shared state
- **Expected:** Observer notifications are serialized or state is atomic
- **TDD Steps:**
  - RED: Notify 10 observers concurrently, check for races
  - Verify RED: Race detector fires
  - GREEN: Serialize observer notifications
  - Verify GREEN: No race, all observers see consistent state
  - REFACTOR: Add observer ordering guarantees

**2.4 Parent Stop While Child Starting**
- **Description:** Parent stops while child's StartAction is in progress
- **Expected:** Child start is cancelled, child transitions to stopped
- **TDD Steps:**
  - RED: Parent stop during child start, check child state
  - Verify RED: Child start completes despite parent stop
  - GREEN: Pass parent context to child, cancel on parent stop
  - Verify GREEN: Child start cancelled, child stopped
  - REFACTOR: Add parent-child cancellation tests

**2.5 Child Crash During Parent State Collection**
- **Description:** Child crashes while parent is calling GetState()
- **Expected:** Parent handles nil/error from crashed child gracefully
- **TDD Steps:**
  - RED: Crash child during GetState(), check parent behavior
  - Verify RED: Parent panics or returns inconsistent state
  - GREEN: Add nil checks, return partial state with error
  - Verify GREEN: Parent continues, marks child as failed
  - REFACTOR: Add error handling to GetState calls

**2.6 Rapid State Transitions**
- **Description:** Worker transitions Stopped→Starting→Running→Stopping→Stopped rapidly
- **Expected:** All transitions complete cleanly, no race conditions
- **TDD Steps:**
  - RED: Trigger rapid transitions, run with -race
  - Verify RED: Race detected in state transitions
  - GREEN: Add state transition lock
  - Verify GREEN: No race, all transitions complete
  - REFACTOR: Add state transition validation

**2.7 Action Queue Overflow**
- **Description:** Actions submitted faster than they can be executed
- **Expected:** Queue fills up, new actions rejected with error
- **TDD Steps:**
  - RED: Submit 1000 actions rapidly, check queue behavior
  - Verify RED: Queue grows unbounded or crashes
  - GREEN: Set queue limit, reject when full
  - Verify GREEN: Queue stays bounded, rejections logged
  - REFACTOR: Add queue metrics

**2.8 Concurrent DeriveDesiredState Calls**
- **Description:** Multiple goroutines call DeriveDesiredState simultaneously
- **Expected:** DeriveDesiredState is idempotent and thread-safe
- **TDD Steps:**
  - RED: Call DeriveDesiredState 100x concurrently, check for races
  - Verify RED: Race detected
  - GREEN: Add mutex or ensure stateless implementation
  - Verify GREEN: No race, consistent results
  - REFACTOR: Document thread-safety requirements

**2.9 Observer Added During Notification**
- **Description:** New observer registered while notification loop is running
- **Expected:** New observer either included or excluded consistently (not partially)
- **TDD Steps:**
  - RED: Add observer during notification, check if it's called
  - Verify RED: Race or inconsistent behavior
  - GREEN: Lock observer list during iteration
  - Verify GREEN: Observer either fully in or fully out
  - REFACTOR: Add observer registration tests

**2.10 Observer Removed During Notification**
- **Description:** Observer unregistered while notification loop is running
- **Expected:** Observer either called or not called (no panic)
- **TDD Steps:**
  - RED: Remove observer during notification, check for panic
  - Verify RED: Panic on accessing removed observer
  - GREEN: Copy observer list before iteration
  - Verify GREEN: No panic, removed observer not called
  - REFACTOR: Use copy-on-write for observer list

**2.11 Parent and Child Action Concurrent Execution**
- **Description:** Parent and child both executing actions simultaneously
- **Expected:** No interference, both complete successfully
- **TDD Steps:**
  - RED: Run parent and child actions concurrently, check for races
  - Verify RED: Race in shared resources
  - GREEN: Isolate parent/child resources
  - Verify GREEN: No race, both complete
  - REFACTOR: Document resource isolation

**2.12 State Collection During Supervisor Shutdown**
- **Description:** Supervisor shutting down while collecting state
- **Expected:** State collection cancelled or completed quickly
- **TDD Steps:**
  - RED: Shutdown during state collection, check for hang
  - Verify RED: State collection blocks shutdown
  - GREEN: Add cancellation to state collection
  - Verify GREEN: Shutdown completes promptly
  - REFACTOR: Add shutdown timeout

**2.13 Action Submitted After Worker Stopped**
- **Description:** Action submitted after worker already stopped
- **Expected:** Action rejected immediately with error
- **TDD Steps:**
  - RED: Stop worker, submit action, check response
  - Verify RED: Action accepted but never executes
  - GREEN: Check worker state before accepting action
  - Verify GREEN: Action rejected immediately
  - REFACTOR: Add state guards to action submission

**2.14 Child State Change Not Detected by Parent**
- **Description:** Child changes state but parent doesn't notice
- **Expected:** Parent polls or is notified of child state changes
- **TDD Steps:**
  - RED: Child changes state, verify parent's view
  - Verify RED: Parent has stale child state
  - GREEN: Add child state notification to parent
  - Verify GREEN: Parent sees child state change promptly
  - REFACTOR: Add state change latency test

**2.15 Double Start Race**
- **Description:** Two StartActions triggered simultaneously
- **Expected:** One succeeds, other is no-op or queued
- **TDD Steps:**
  - RED: Submit two Start actions concurrently, check outcome
  - Verify RED: Both start actions execute, invalid state
  - GREEN: Add started flag, check-and-set atomically
  - Verify GREEN: Only one start executes
  - REFACTOR: Add idempotent action tests

**2.16 State Snapshot Inconsistency**
- **Description:** State snapshot taken during transition shows inconsistent state
- **Expected:** Snapshot is atomic or marked as "in transition"
- **TDD Steps:**
  - RED: Take snapshot during transition, verify consistency
  - Verify RED: Snapshot shows mixed old/new state
  - GREEN: Lock state during snapshot
  - Verify GREEN: Snapshot is consistent
  - REFACTOR: Add snapshot validation

**2.17 Action Context Cancelled Mid-Execution**
- **Description:** Action running, context cancelled externally
- **Expected:** Action checks ctx.Done(), returns promptly
- **TDD Steps:**
  - RED: Cancel context during action, measure stop time
  - Verify RED: Action runs to completion despite cancellation
  - GREEN: Add ctx.Done() checks in action loop
  - Verify GREEN: Action stops within timeout
  - REFACTOR: Add cancellation latency test

**2.18 Parent Starts While Child Still Stopping**
- **Description:** Parent starts before child from previous run finished stopping
- **Expected:** Parent waits for child cleanup or starts with new child instance
- **TDD Steps:**
  - RED: Start parent while child stopping, check for conflicts
  - Verify RED: Resource conflict (e.g., port already bound)
  - GREEN: Wait for child cleanup before starting parent
  - Verify GREEN: No resource conflict
  - REFACTOR: Add cleanup verification

**2.19 Supervisor State Collection Timeout**
- **Description:** Worker takes too long to respond to GetState()
- **Expected:** Supervisor times out, uses last known state or marks worker unhealthy
- **TDD Steps:**
  - RED: GetState blocks forever, verify supervisor behavior
  - Verify RED: Supervisor hangs waiting for state
  - GREEN: Add timeout to GetState calls
  - Verify GREEN: Supervisor continues with timeout error
  - REFACTOR: Add GetState timeout config

**2.20 Concurrent Child Spawn and Despawn**
- **Description:** Parent spawning new child while old child being despawned
- **Expected:** Children tracked by ID, no confusion between old and new
- **TDD Steps:**
  - RED: Spawn child with same ID as despawning child
  - Verify RED: State confusion, wrong child stopped
  - GREEN: Use unique child IDs (UUID or counter)
  - Verify GREEN: Old and new children tracked separately
  - REFACTOR: Enforce unique child IDs

**2.21 Observer Callback Modifies State During Notification**
- **Description:** Observer callback modifies worker state while notification in progress
- **Expected:** State modification is deferred or safely handled
- **TDD Steps:**
  - RED: Observer modifies state, verify consistency
  - Verify RED: Race or inconsistent state
  - GREEN: Queue state modifications from observers
  - Verify GREEN: Modifications applied after notification
  - REFACTOR: Document observer safety rules

**2.22 Action Executor Stopped While Action Running**
- **Description:** Action executor shuts down while action in flight
- **Expected:** Action completed or cancelled cleanly, no leak
- **TDD Steps:**
  - RED: Stop executor during action, check goroutine leaks
  - Verify RED: Goroutine leak detected
  - GREEN: Cancel action context on executor shutdown
  - Verify GREEN: No leak, action stopped
  - REFACTOR: Add executor shutdown tests

**2.23 Parent DeriveDesiredState Returns Different Children**
- **Description:** Parent's DeriveDesiredState returns different ChildrenSpecs each call
- **Expected:** Supervisor detects diff, stops old children, starts new
- **TDD Steps:**
  - RED: Return different children on each call, verify supervisor behavior
  - Verify RED: Supervisor confused, old children not stopped
  - GREEN: Add child diffing logic in supervisor
  - Verify GREEN: Old children stopped, new children started
  - REFACTOR: Add child lifecycle tests

**2.24 Collector Concurrent Access**
- **Description:** Multiple goroutines call Collector methods simultaneously
- **Expected:** Collector is thread-safe, returns consistent state
- **TDD Steps:**
  - RED: Call Collector 100x concurrently, check for races
  - Verify RED: Race detected
  - GREEN: Add mutex to Collector state
  - Verify GREEN: No race, consistent state
  - REFACTOR: Document Collector thread-safety

**2.25 Worker State Rollback on Action Failure**
- **Description:** Action fails halfway through, needs to rollback state change
- **Expected:** Worker returns to previous state or error state (no partial state)
- **TDD Steps:**
  - RED: Action fails mid-execution, check final state
  - Verify RED: Worker in inconsistent partial state
  - GREEN: Wrap action in transaction, rollback on failure
  - Verify GREEN: Worker in previous or error state
  - REFACTOR: Add state transaction pattern

---

### Category 3: Medium-Priority Edge Cases (Error Propagation)

**Why It Matters:** Error propagation failures cause silent failures or cascading errors. A localized error (e.g., one child fails) shouldn't take down the entire system, but the error must be visible for debugging. These tests ensure errors are handled gracefully and propagated correctly.

**Production Impact:**
- 🟡 **Severity: P2** - Degraded functionality but system continues
- Errors not visible to operators (silent failures)
- Local failures cascade to other components
- Debugging difficult due to missing error context
- System degrades ungracefully

#### Tests (20 total)

**3.1 Child Error Propagated to Parent**
- **Description:** Child action fails, parent should be notified
- **Expected:** Parent receives child error, can react (Degraded state)
- **TDD Steps:**
  - RED: Child action fails, verify parent awareness
  - Verify RED: Parent unaware of child failure
  - GREEN: Add child error propagation to parent
  - Verify GREEN: Parent sees child error, transitions to Degraded
  - REFACTOR: Add error propagation tests for all child states

**3.2 Action Error Not Swallowed**
- **Description:** Action returns error but it's ignored
- **Expected:** Action error logged and propagated to supervisor
- **TDD Steps:**
  - RED: Action returns error, verify it's logged/propagated
  - Verify RED: Error swallowed silently
  - GREEN: Log and propagate action errors
  - Verify GREEN: Error visible in logs and supervisor state
  - REFACTOR: Add error propagation validation

**3.3 Dependency Failure During Start**
- **Description:** ConfigLoader fails during parent StartAction
- **Expected:** Start fails, worker transitions to Stopped with error
- **TDD Steps:**
  - RED: Make ConfigLoader fail, verify worker state
  - Verify RED: Worker in inconsistent state
  - GREEN: Check dependency health before starting
  - Verify GREEN: Worker in Stopped state with error
  - REFACTOR: Add dependency health checks

**3.4 Partial Child Spawn Failure**
- **Description:** Parent spawns 5 children, 2 fail to start
- **Expected:** Parent in Degraded state with 3 healthy children
- **TDD Steps:**
  - RED: Fail 2 child starts, check parent state
  - Verify RED: Parent in Running or Stopped (not Degraded)
  - GREEN: Add Degraded state when some children fail
  - Verify GREEN: Parent in Degraded with 3 children
  - REFACTOR: Add partial failure tests

**3.5 Observer Error Handling**
- **Description:** Observer callback returns error or panics
- **Expected:** Error logged, other observers still notified
- **TDD Steps:**
  - RED: Observer returns error, verify other observers
  - Verify RED: Notification stops at error
  - GREEN: Catch observer errors, continue notification loop
  - Verify GREEN: All observers notified despite one error
  - REFACTOR: Add observer error metrics

**3.6 Action Timeout vs Cancellation**
- **Description:** Distinguish between action timing out and being cancelled
- **Expected:** Different error types, logged differently
- **TDD Steps:**
  - RED: Trigger timeout and cancellation, check error types
  - Verify RED: Both show same error
  - GREEN: Return different errors for timeout vs cancellation
  - Verify GREEN: Errors distinguishable in logs
  - REFACTOR: Add error type enum

**3.7 Child Error Recovered by Retry**
- **Description:** Child fails but succeeds on retry, parent should know
- **Expected:** Parent sees retry success, transitions back to Running
- **TDD Steps:**
  - RED: Child fails then succeeds, check parent state
  - Verify RED: Parent stays in Degraded
  - GREEN: Add child recovery detection to parent
  - Verify GREEN: Parent transitions Degraded→Running
  - REFACTOR: Add recovery detection tests

**3.8 Supervisor Error During State Collection**
- **Description:** GetState() panics or returns invalid state
- **Expected:** Supervisor handles error gracefully, marks worker unhealthy
- **TDD Steps:**
  - RED: Make GetState panic, verify supervisor behavior
  - Verify RED: Supervisor crashes
  - GREEN: Add recover() around GetState calls
  - Verify GREEN: Supervisor survives, worker marked unhealthy
  - REFACTOR: Add state validation

**3.9 Action Error Contains Stack Trace**
- **Description:** Action error doesn't include stack trace or context
- **Expected:** Errors include stack trace for debugging
- **TDD Steps:**
  - RED: Action fails, check error for stack trace
  - Verify RED: No stack trace in error
  - GREEN: Wrap errors with stack traces
  - Verify GREEN: Stack trace present in logs
  - REFACTOR: Add error wrapping helper

**3.10 Dependency Injection Failure Detected Early**
- **Description:** Required dependency is nil at construction time
- **Expected:** Fail fast during worker creation (not during Start)
- **TDD Steps:**
  - RED: Pass nil dependency, verify when error detected
  - Verify RED: Error during Start (too late)
  - GREEN: Validate dependencies in constructor
  - Verify GREEN: Error during worker creation
  - REFACTOR: Add dependency validation

**3.11 Child State Invalid (Not in Enum)**
- **Description:** Child returns state not in expected enum
- **Expected:** Supervisor detects invalid state, marks child unhealthy
- **TDD Steps:**
  - RED: Child returns invalid state, verify supervisor
  - Verify RED: Supervisor accepts invalid state
  - GREEN: Add state validation in supervisor
  - Verify GREEN: Supervisor detects and rejects invalid state
  - REFACTOR: Add state enum validation

**3.12 Parent Degraded→Stopped Transition**
- **Description:** Parent in Degraded state, stop action initiated
- **Expected:** All remaining children stopped cleanly
- **TDD Steps:**
  - RED: Stop parent in Degraded, verify all children stopped
  - Verify RED: Some children not stopped
  - GREEN: Iterate all children regardless of health
  - Verify GREEN: All children stopped
  - REFACTOR: Add Degraded state tests

**3.13 Action Error Causes Retry Storm**
- **Description:** Action fails, supervisor retries immediately, fails again
- **Expected:** Exponential backoff between retries
- **TDD Steps:**
  - RED: Action fails 100x, verify retry timing
  - Verify RED: Retries happen immediately (retry storm)
  - GREEN: Add exponential backoff
  - Verify GREEN: Retries spread out over time
  - REFACTOR: Add backoff config

**3.14 Supervisor Continues After Worker Unrecoverable Error**
- **Description:** Worker enters unrecoverable error state
- **Expected:** Supervisor marks worker as failed, continues managing others
- **TDD Steps:**
  - RED: Worker fails unrecoverably, verify supervisor
  - Verify RED: Supervisor stops managing other workers
  - GREEN: Isolate failed worker, continue with others
  - Verify GREEN: Supervisor continues normally
  - REFACTOR: Add multi-worker failure tests

**3.15 Child Connection Lost, Auto-Reconnect**
- **Description:** example-child loses connection, should auto-reconnect
- **Expected:** Child transitions Disconnected→TryingToConnect→Connected
- **TDD Steps:**
  - RED: Simulate connection loss, verify reconnection
  - Verify RED: Child stays Disconnected
  - GREEN: Add auto-reconnect logic to child
  - Verify GREEN: Child reconnects automatically
  - REFACTOR: Add reconnection tests

**3.16 Parent Config Reload Without Restart**
- **Description:** example-parent config changes, needs reload
- **Expected:** Parent reloads config, respawns children with new config
- **TDD Steps:**
  - RED: Change config, verify parent reacts
  - Verify RED: Parent uses old config
  - GREEN: Add config change detection
  - Verify GREEN: Parent respawns children with new config
  - REFACTOR: Add config hot-reload tests

**3.17 Action Error Includes Retry Attempt Number**
- **Description:** Action fails on retry, error should include attempt number
- **Expected:** Error message includes "attempt 3/5 failed"
- **TDD Steps:**
  - RED: Fail action on retry, check error message
  - Verify RED: No retry count in error
  - GREEN: Include retry count in error context
  - Verify GREEN: Error shows "attempt 3/5"
  - REFACTOR: Add retry context to all errors

**3.18 Supervisor State Inconsistent After Worker Removal**
- **Description:** Worker removed but supervisor state not updated
- **Expected:** Supervisor state reflects worker removal immediately
- **TDD Steps:**
  - RED: Remove worker, check supervisor state
  - Verify RED: Supervisor state stale
  - GREEN: Update supervisor state on worker removal
  - Verify GREEN: State updated immediately
  - REFACTOR: Add state consistency checks

**3.19 Child Action Error Includes Parent Context**
- **Description:** Child action fails but error doesn't mention parent
- **Expected:** Error includes parent ID for debugging
- **TDD Steps:**
  - RED: Child action fails, check if error includes parent
  - Verify RED: No parent context in error
  - GREEN: Add parent ID to child error context
  - Verify GREEN: Error shows parent ID
  - REFACTOR: Add context to all child errors

**3.20 Action Timeout Error Includes Duration**
- **Description:** Action times out but error doesn't show how long it ran
- **Expected:** Error includes "timed out after 30s"
- **TDD Steps:**
  - RED: Timeout action, check error message
  - Verify RED: No duration in error
  - GREEN: Include duration in timeout error
  - Verify GREEN: Error shows duration
  - REFACTOR: Add duration to all timeout errors

---

### Category 4: Low-Priority Edge Cases (Observer/Metrics)

**Why It Matters:** These don't cause system failures but impact observability and debugging. Missing metrics or dropped notifications make production issues harder to diagnose. While not critical, they significantly impact operational experience.

**Production Impact:**
- 🟢 **Severity: P3** - System functions but observability impaired
- Missing metrics make debugging harder
- Dropped notifications hide important events
- Operators lack visibility into system health
- Performance issues harder to detect

#### Tests (15 total)

**4.1 Observer Receives All State Changes**
- **Description:** Worker transitions through multiple states rapidly
- **Expected:** Observer notified of every state change (no drops)
- **TDD Steps:**
  - RED: Rapid state transitions, count observer notifications
  - Verify RED: Some notifications dropped
  - GREEN: Queue notifications, ensure delivery
  - Verify GREEN: All notifications received
  - REFACTOR: Add notification reliability tests

**4.2 Observer Notification Order**
- **Description:** Multiple observers registered, verify notification order
- **Expected:** Observers notified in registration order (or documented order)
- **TDD Steps:**
  - RED: Register 5 observers, check notification order
  - Verify RED: Order random or undefined
  - GREEN: Document and enforce notification order
  - Verify GREEN: Observers notified in expected order
  - REFACTOR: Add order guarantees to API

**4.3 Observer Unregistered After First Notification**
- **Description:** Observer unregisters after receiving first notification
- **Expected:** Observer not called for subsequent notifications
- **TDD Steps:**
  - RED: Unregister observer, verify no further calls
  - Verify RED: Observer still called after unregistration
  - GREEN: Remove observer from list on unregister
  - Verify GREEN: Observer not called again
  - REFACTOR: Add unregister tests

**4.4 Metrics Emitted for Every Action**
- **Description:** Action starts/completes/fails, verify metrics emitted
- **Expected:** Metrics for action duration, success/failure, retry count
- **TDD Steps:**
  - RED: Run actions, check for metrics
  - Verify RED: No metrics emitted
  - GREEN: Emit metrics on action lifecycle events
  - Verify GREEN: All metrics present
  - REFACTOR: Add metrics validation helper

**4.5 Metrics Include Worker ID and Type**
- **Description:** Metrics emitted but don't include identifying labels
- **Expected:** All metrics tagged with worker_id and worker_type
- **TDD Steps:**
  - RED: Check metrics for labels
  - Verify RED: Labels missing
  - GREEN: Add labels to all metrics
  - Verify GREEN: Labels present
  - REFACTOR: Add label validation

**4.6 Supervisor Metrics for Child Count**
- **Description:** Parent has N children, verify child count metric
- **Expected:** Metric shows current child count at all times
- **TDD Steps:**
  - RED: Check for child count metric
  - Verify RED: Metric missing
  - GREEN: Emit child count metric on change
  - Verify GREEN: Metric reflects current count
  - REFACTOR: Add child count tests

**4.7 Action Duration Histogram Accurate**
- **Description:** Actions take 10ms/100ms/1s, verify histogram buckets
- **Expected:** Histogram has appropriate buckets for action durations
- **TDD Steps:**
  - RED: Run actions, check histogram buckets
  - Verify RED: Buckets inappropriate (e.g., 0-1s, 1s-10s only)
  - GREEN: Define buckets for expected durations
  - Verify GREEN: Histogram captures durations accurately
  - REFACTOR: Add histogram validation

**4.8 Observer Notification Includes Previous State**
- **Description:** Observer notified of state change, needs previous state
- **Expected:** Notification includes both old and new state
- **TDD Steps:**
  - RED: Check observer callback signature
  - Verify RED: Only new state provided
  - GREEN: Add previous state to notification
  - Verify GREEN: Observer receives both states
  - REFACTOR: Update observer interface

**4.9 Supervisor Emits Health Check Metric**
- **Description:** Supervisor periodically checks worker health
- **Expected:** Metric shows worker health status (healthy/degraded/failed)
- **TDD Steps:**
  - RED: Check for health metric
  - Verify RED: No health metric
  - GREEN: Emit health metric on collection
  - Verify GREEN: Metric shows health status
  - REFACTOR: Add health check tests

**4.10 Observer Slow (Blocks Notification)**
- **Description:** Observer takes 10s to process notification
- **Expected:** Other observers still notified, slow observer logged as warning
- **TDD Steps:**
  - RED: Add slow observer, verify others notified
  - Verify RED: All observers blocked by slow one
  - GREEN: Use goroutine per observer or timeout
  - Verify GREEN: Other observers not blocked
  - REFACTOR: Add observer performance tests

**4.11 Metrics Emitted on Supervisor Start/Stop**
- **Description:** Supervisor starts/stops, verify lifecycle metrics
- **Expected:** Metrics for supervisor_started, supervisor_stopped
- **TDD Steps:**
  - RED: Check for supervisor lifecycle metrics
  - Verify RED: No metrics
  - GREEN: Emit metrics on supervisor start/stop
  - Verify GREEN: Metrics present
  - REFACTOR: Add supervisor lifecycle tests

**4.12 Action Retry Count Metric**
- **Description:** Action retries 3x before succeeding
- **Expected:** Metric shows retry count distribution
- **TDD Steps:**
  - RED: Check for retry metric
  - Verify RED: No retry metric
  - GREEN: Emit retry count on action completion
  - Verify GREEN: Metric shows retry count
  - REFACTOR: Add retry tracking

**4.13 Observer Receives Error State with Error Details**
- **Description:** Worker transitions to error state
- **Expected:** Observer receives error state plus error message/stack
- **TDD Steps:**
  - RED: Transition to error, check observer callback
  - Verify RED: Only state, no error details
  - GREEN: Add error details to notification
  - Verify GREEN: Observer receives error context
  - REFACTOR: Update observer error handling

**4.14 Child State Change Latency Metric**
- **Description:** Child changes state, parent notified after delay
- **Expected:** Metric shows parent notification latency
- **TDD Steps:**
  - RED: Child changes state, measure parent notification time
  - Verify RED: No latency metric
  - GREEN: Emit latency metric on parent notification
  - Verify GREEN: Metric shows latency
  - REFACTOR: Add latency tracking

**4.15 Collector State Snapshot Includes Timestamps**
- **Description:** Collector returns state snapshot
- **Expected:** Snapshot includes timestamp of each state transition
- **TDD Steps:**
  - RED: Get snapshot, check for timestamps
  - Verify RED: No timestamps
  - GREEN: Add timestamps to state
  - Verify GREEN: Snapshot includes timestamps
  - REFACTOR: Add timestamp validation

---

### Category 5: Integration & End-to-End (Full System)

**Why It Matters:** Unit tests verify individual components, but integration tests verify the system works as a whole. These tests catch issues that only appear when components interact (e.g., supervisor + action_executor + collector + workers).

**Production Impact:**
- 🔵 **Severity: P1** - Verifies production workflows
- Catches component integration issues
- Validates realistic scenarios
- Ensures system behavior matches expectations
- Gives confidence in production deployment

#### Tests (10 total)

**5.1 Full Lifecycle: Start→Run→Stop**
- **Description:** example-parent created, started, running, stopped cleanly
- **Expected:** All state transitions clean, no leaks, metrics correct
- **TDD Steps:**
  - RED: Full lifecycle test, check for leaks/errors
  - Verify RED: Leak or error in lifecycle
  - GREEN: Fix leak/error
  - Verify GREEN: Clean lifecycle
  - REFACTOR: Add lifecycle assertions

**5.2 Parent→Child Cascade Start**
- **Description:** Start parent, verify children start automatically
- **Expected:** Parent Running, both children Connected
- **TDD Steps:**
  - RED: Start parent, check child states
  - Verify RED: Children not started
  - GREEN: Add child spawning to parent Start
  - Verify GREEN: Children started
  - REFACTOR: Add cascade tests

**5.3 Parent→Child Cascade Stop**
- **Description:** Stop parent, verify children stop automatically
- **Expected:** Parent Stopped, all children Stopped
- **TDD Steps:**
  - RED: Stop parent, check child states
  - Verify RED: Children not stopped
  - GREEN: Add child stopping to parent Stop
  - Verify GREEN: Children stopped
  - REFACTOR: Add cleanup verification

**5.4 Child Failure Triggers Parent Degraded**
- **Description:** One child fails, parent transitions to Degraded
- **Expected:** Parent Degraded, one child failed, other children healthy
- **TDD Steps:**
  - RED: Fail one child, check parent state
  - Verify RED: Parent stays Running
  - GREEN: Add child health monitoring to parent
  - Verify GREEN: Parent transitions to Degraded
  - REFACTOR: Add health monitoring tests

**5.5 Child Recovery Triggers Parent Running**
- **Description:** Failed child recovers, parent transitions back to Running
- **Expected:** Parent Running, all children Connected
- **TDD Steps:**
  - RED: Recover child, check parent state
  - Verify RED: Parent stays Degraded
  - GREEN: Add recovery detection to parent
  - Verify GREEN: Parent transitions to Running
  - REFACTOR: Add recovery tests

**5.6 Supervisor Manages 10 Workers Concurrently**
- **Description:** 10 example-parent workers running simultaneously
- **Expected:** All workers managed independently, no interference
- **TDD Steps:**
  - RED: Start 10 workers, verify isolation
  - Verify RED: Workers interfere (shared state?)
  - GREEN: Ensure worker isolation
  - Verify GREEN: No interference
  - REFACTOR: Add concurrency tests

**5.7 Supervisor Shuts Down Gracefully with Active Workers**
- **Description:** Supervisor shutdown initiated while workers active
- **Expected:** All workers stopped, supervisor exits cleanly
- **TDD Steps:**
  - RED: Shutdown with active workers, check for leaks
  - Verify RED: Goroutine leak or hang
  - GREEN: Add graceful shutdown
  - Verify GREEN: Clean shutdown
  - REFACTOR: Add shutdown tests

**5.8 Collector Returns Consistent State During Transitions**
- **Description:** Collect state while workers transitioning
- **Expected:** Snapshot consistent (not partially transitioned)
- **TDD Steps:**
  - RED: Collect during transition, verify consistency
  - Verify RED: Inconsistent snapshot
  - GREEN: Lock state during collection
  - Verify GREEN: Consistent snapshot
  - REFACTOR: Add snapshot consistency tests

**5.9 Observer Receives All Events in Order**
- **Description:** Observer monitoring parent + 2 children
- **Expected:** All events received in causal order
- **TDD Steps:**
  - RED: Monitor 3 workers, check event order
  - Verify RED: Events out of order
  - GREEN: Add ordering guarantees
  - Verify GREEN: Events in order
  - REFACTOR: Add event ordering tests

**5.10 Action Executor Handles 1000 Concurrent Actions**
- **Description:** Submit 1000 actions across all workers simultaneously
- **Expected:** All actions executed, no drops, no crashes
- **TDD Steps:**
  - RED: Submit 1000 actions, check results
  - Verify RED: Drops or crashes
  - GREEN: Increase queue capacity or add backpressure
  - Verify GREEN: All actions complete
  - REFACTOR: Add load tests

---

## Phase Breakdown

### Phase 1: Critical Failures (Days 1-4)

**Focus:** System hangs, deadlocks, resource leaks - the "never ship" bugs

**Estimated Time:** 4 days

**Tests:** 20 tests from Category 1

**Implementation Order:**
1. **Day 1: Deadlocks (Tests 1.1-1.6)**
   - 1.1 Action Never Completes (timeout)
   - 1.2 Action Timeout During Parent Stop
   - 1.3 State Deadlock (Circular Wait)
   - 1.4 Channel Blocking (No Receiver)
   - 1.5 Channel Blocking (Buffer Full)
   - 1.6 Mutex Deadlock (Lock Order Violation)

2. **Day 2: Goroutine Leaks (Tests 1.7-1.12)**
   - 1.7 Goroutine Leak (Action Never Returns)
   - 1.8 Goroutine Leak (Forgotten Context Cancel)
   - 1.9 Supervisor Hang (Worker Never Responds)
   - 1.10 Action Blocked on Full Queue
   - 1.11 Parent Shutdown Waits Forever
   - 1.12 Supervisor Restart Loop (Immediate Crash)

3. **Day 3: Resource Leaks (Tests 1.13-1.17)**
   - 1.13 Resource Leak (File Descriptors)
   - 1.14 Resource Leak (Memory)
   - 1.15 Double Stop Deadlock
   - 1.16 Race Condition (Shared State)
   - 1.17 Panic in Action (Not Recovered)

4. **Day 4: Context and State (Tests 1.18-1.20)**
   - 1.18 Panic in Observer (Not Recovered)
   - 1.19 Context Cancelled But Action Continues
   - 1.20 Stuck in Transitional State Forever

**Success Criteria:**
- All 20 tests passing
- No deadlocks in 1000 test runs
- No goroutine leaks detected
- No resource leaks detected
- All tests pass with `-race` flag

**Daily Checkpoints:**
- End of each day: Run full Phase 1 suite with `-race`
- Check goroutine counts before/after each test
- Profile resource usage (FDs, memory)
- Document any discovered edge cases

---

### Phase 2: High-Priority Edge Cases (Days 5-9)

**Focus:** Race conditions, timing issues, concurrent state changes

**Estimated Time:** 5 days

**Tests:** 25 tests from Category 2

**Implementation Order:**
1. **Day 5: Basic Race Conditions (Tests 2.1-2.5)**
   - 2.1 Concurrent Start/Stop
   - 2.2 State Change During Action Execution
   - 2.3 Multiple Observers Racing to Update
   - 2.4 Parent Stop While Child Starting
   - 2.5 Child Crash During Parent State Collection

2. **Day 6: State Transitions (Tests 2.6-2.10)**
   - 2.6 Rapid State Transitions
   - 2.7 Action Queue Overflow
   - 2.8 Concurrent DeriveDesiredState Calls
   - 2.9 Observer Added During Notification
   - 2.10 Observer Removed During Notification

3. **Day 7: Parent-Child Races (Tests 2.11-2.15)**
   - 2.11 Parent and Child Action Concurrent Execution
   - 2.12 State Collection During Supervisor Shutdown
   - 2.13 Action Submitted After Worker Stopped
   - 2.14 Child State Change Not Detected by Parent
   - 2.15 Double Start Race

4. **Day 8: Snapshot and Context (Tests 2.16-2.20)**
   - 2.16 State Snapshot Inconsistency
   - 2.17 Action Context Cancelled Mid-Execution
   - 2.18 Parent Starts While Child Still Stopping
   - 2.19 Supervisor State Collection Timeout
   - 2.20 Concurrent Child Spawn and Despawn

5. **Day 9: Complex Races (Tests 2.21-2.25)**
   - 2.21 Observer Callback Modifies State During Notification
   - 2.22 Action Executor Stopped While Action Running
   - 2.23 Parent DeriveDesiredState Returns Different Children
   - 2.24 Collector Concurrent Access
   - 2.25 Worker State Rollback on Action Failure

**Success Criteria:**
- All 25 tests passing
- No data races detected with `-race`
- Consistent behavior across 1000 test runs
- All race conditions documented and fixed
- Observer notification guarantees documented

**Daily Checkpoints:**
- Run each test 100x to catch intermittent failures
- Check for race detector warnings
- Profile concurrent operations
- Document synchronization patterns used

---

### Phase 3: Medium-Priority Edge Cases (Days 10-13)

**Focus:** Error propagation, recovery, state consistency

**Estimated Time:** 4 days

**Tests:** 20 tests from Category 3

**Implementation Order:**
1. **Day 10: Error Propagation (Tests 3.1-3.5)**
   - 3.1 Child Error Propagated to Parent
   - 3.2 Action Error Not Swallowed
   - 3.3 Dependency Failure During Start
   - 3.4 Partial Child Spawn Failure
   - 3.5 Observer Error Handling

2. **Day 11: Error Context (Tests 3.6-3.10)**
   - 3.6 Action Timeout vs Cancellation
   - 3.7 Child Error Recovered by Retry
   - 3.8 Supervisor Error During State Collection
   - 3.9 Action Error Contains Stack Trace
   - 3.10 Dependency Injection Failure Detected Early

3. **Day 12: State Validation (Tests 3.11-3.15)**
   - 3.11 Child State Invalid (Not in Enum)
   - 3.12 Parent Degraded→Stopped Transition
   - 3.13 Action Error Causes Retry Storm
   - 3.14 Supervisor Continues After Worker Unrecoverable Error
   - 3.15 Child Connection Lost, Auto-Reconnect

4. **Day 13: Config and Context (Tests 3.16-3.20)**
   - 3.16 Parent Config Reload Without Restart
   - 3.17 Action Error Includes Retry Attempt Number
   - 3.18 Supervisor State Inconsistent After Worker Removal
   - 3.19 Child Action Error Includes Parent Context
   - 3.20 Action Timeout Error Includes Duration

**Success Criteria:**
- All 20 tests passing
- Degraded state handling works correctly
- All errors include sufficient context for debugging
- Retry logic works with exponential backoff
- State validation catches invalid transitions

**Daily Checkpoints:**
- Verify error messages contain useful context
- Test partial failure scenarios
- Check retry timing and backoff
- Document error handling patterns

---

### Phase 4: Low-Priority Edge Cases (Days 14-16)

**Focus:** Observer notifications, metrics, observability

**Estimated Time:** 3 days

**Tests:** 15 tests from Category 4

**Implementation Order:**
1. **Day 14: Observer Behavior (Tests 4.1-4.5)**
   - 4.1 Observer Receives All State Changes
   - 4.2 Observer Notification Order
   - 4.3 Observer Unregistered After First Notification
   - 4.4 Metrics Emitted for Every Action
   - 4.5 Metrics Include Worker ID and Type

2. **Day 15: Metrics (Tests 4.6-4.10)**
   - 4.6 Supervisor Metrics for Child Count
   - 4.7 Action Duration Histogram Accurate
   - 4.8 Observer Notification Includes Previous State
   - 4.9 Supervisor Emits Health Check Metric
   - 4.10 Observer Slow (Blocks Notification)

3. **Day 16: Advanced Metrics (Tests 4.11-4.15)**
   - 4.11 Metrics Emitted on Supervisor Start/Stop
   - 4.12 Action Retry Count Metric
   - 4.13 Observer Receives Error State with Error Details
   - 4.14 Child State Change Latency Metric
   - 4.15 Collector State Snapshot Includes Timestamps

**Success Criteria:**
- All 15 tests passing
- Observer notification guarantees documented
- All metrics defined and emitted correctly
- Histogram buckets appropriate for expected values
- Observer interface finalized and documented

**Daily Checkpoints:**
- Verify all state changes emit notifications
- Check metrics cardinality (not too many unique labels)
- Test slow observers don't block system
- Document observer and metrics contracts

---

### Phase 5: Integration & End-to-End (Days 17-18)

**Focus:** Full system workflows, multi-worker scenarios

**Estimated Time:** 2 days

**Tests:** 10 tests from Category 5

**Implementation Order:**
1. **Day 17: Lifecycle Tests (Tests 5.1-5.5)**
   - 5.1 Full Lifecycle: Start→Run→Stop
   - 5.2 Parent→Child Cascade Start
   - 5.3 Parent→Child Cascade Stop
   - 5.4 Child Failure Triggers Parent Degraded
   - 5.5 Child Recovery Triggers Parent Running

2. **Day 18: Multi-Worker Tests (Tests 5.6-5.10)**
   - 5.6 Supervisor Manages 10 Workers Concurrently
   - 5.7 Supervisor Shuts Down Gracefully with Active Workers
   - 5.8 Collector Returns Consistent State During Transitions
   - 5.9 Observer Receives All Events in Order
   - 5.10 Action Executor Handles 1000 Concurrent Actions

**Success Criteria:**
- All 10 tests passing
- Full system workflows validated
- Multi-worker scenarios handled correctly
- No interference between workers
- Load tests pass (1000+ concurrent actions)

**Daily Checkpoints:**
- Run full test suite (all 90 tests)
- Check overall system behavior
- Profile under load
- Document integration patterns

---

## Implementation Guidelines

### TDD Workflow (Strict RED-GREEN-REFACTOR)

**This is mandatory. No exceptions.**

For EVERY test:

1. **RED - Write Failing Test**
   ```go
   var _ = Describe("Action Timeout", func() {
       It("should timeout after 5 seconds", func() {
           // Arrange: Create worker with action that never returns
           worker := NewExampleParent(deps)
           action := &BlockingAction{} // Blocks forever

           // Act: Execute action with timeout
           err := supervisor.ExecuteAction(worker, action)

           // Assert: Should timeout, not hang forever
           Expect(err).To(MatchError(ErrActionTimeout))
       })
   })
   ```

2. **Verify RED - Watch It Fail**
   ```bash
   go test ./pkg/fsmv2/examples/integration_test -run TestActionTimeout -v
   ```
   **Expected:** Test hangs forever OR fails with wrong error
   **Don't proceed until you see the CORRECT failure**

3. **GREEN - Minimal Implementation**
   ```go
   func (e *ActionExecutor) ExecuteAction(action Action) error {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()

       done := make(chan error, 1)
       go func() {
           done <- action.Execute(ctx)
       }()

       select {
       case err := <-done:
           return err
       case <-ctx.Done():
           return ErrActionTimeout
       }
   }
   ```

4. **Verify GREEN - Watch It Pass**
   ```bash
   go test ./pkg/fsmv2/examples/integration_test -run TestActionTimeout -v
   ```
   **Expected:** PASS with no errors or warnings

5. **REFACTOR - Clean Up**
   - Extract timeout config
   - Add logging
   - Improve error messages
   - Extract helper functions
   **Re-run test after each refactor to stay green**

### Test Helpers and Utilities

**Location:** `pkg/fsmv2/examples/integration_test/test_helpers.go`

**Required Helpers:**

```go
// Goroutine leak detection
func CheckForGoroutineLeaks(t *testing.T) func() {
    before := runtime.NumGoroutine()
    return func() {
        After := runtime.NumGoroutine()
        if after > before+5 { // Allow small variance
            t.Errorf("Goroutine leak detected: %d before, %d after", before, after)
        }
    }
}

// Resource tracking
func TrackFileDescriptors(t *testing.T) func() {
    before := getOpenFDCount()
    return func() {
        after := getOpenFDCount()
        if after > before {
            t.Errorf("FD leak detected: %d before, %d after", before, after)
        }
    }
}

// Timeout helper
func WithTimeout(duration time.Duration, fn func()) error {
    done := make(chan struct{})
    go func() {
        fn()
        close(done)
    }()

    select {
    case <-done:
        return nil
    case <-time.After(duration):
        return ErrTimeout
    }
}

// State assertion helper
func ExpectState(worker Worker, expected State) {
    Eventually(func() State {
        return worker.GetState()
    }, "5s", "100ms").Should(Equal(expected))
}

// Event collector for observer tests
type EventCollector struct {
    mu     sync.Mutex
    events []StateChangeEvent
}

func (c *EventCollector) OnStateChange(evt StateChangeEvent) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.events = append(c.events, evt)
}

func (c *EventCollector) GetEvents() []StateChangeEvent {
    c.mu.Lock()
    defer c.mu.Unlock()
    return append([]StateChangeEvent{}, c.events...)
}

// Mock dependencies for testing
type MockConfigLoader struct {
    configs map[string]Config
    err     error
}

type MockConnectionPool struct {
    connections map[string]Connection
    err         error
}
```

### Race Detector Usage

**MANDATORY for all test runs:**

```bash
# Single test
go test -race ./pkg/fsmv2/examples/integration_test -run TestConcurrentStartStop -v

# Full suite
go test -race ./pkg/fsmv2/examples/integration_test -v

# Repeat 100x to catch intermittent races
for i in {1..100}; do
    go test -race ./pkg/fsmv2/examples/integration_test -run TestRapidStateTransitions -v || break
done
```

**Interpreting race detector output:**

```
WARNING: DATA RACE
Read at 0x00c0001a2080 by goroutine 23:
  github.com/umh/pkg/fsmv2.(*Worker).GetState()
      worker.go:45 +0x38

Previous write at 0x00c0001a2080 by goroutine 19:
  github.com/umh/pkg/fsmv2.(*Worker).setState()
      worker.go:52 +0x5c
```

**Fix:** Add mutex around state access

### Coverage Targets

**Minimum Coverage:** >80% on core components

**Components:**
- `supervisor.go` - >80%
- `action_executor.go` - >80%
- `collector.go` - >80%
- `examples/workers/example-parent/` - >70%
- `examples/workers/example-child/` - >70%

**Check coverage:**

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./pkg/fsmv2/...

# View in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

**Coverage targets by phase:**
- Phase 1 end: >50% (deadlock/leak tests)
- Phase 2 end: >65% (race condition tests)
- Phase 3 end: >75% (error propagation tests)
- Phase 5 end: >80% (full integration)

### Resource Profiling

**Check for leaks after each phase:**

```bash
# CPU profile
go test -cpuprofile=cpu.prof ./pkg/fsmv2/examples/integration_test -v
go tool pprof cpu.prof

# Memory profile
go test -memprofile=mem.prof ./pkg/fsmv2/examples/integration_test -v
go tool pprof mem.prof

# Goroutine profile (leak detection)
go test -trace=trace.out ./pkg/fsmv2/examples/integration_test -v
go tool trace trace.out
```

**Red flags in profiles:**
- Goroutine count grows over time
- Memory usage grows unbounded
- File descriptors not released
- CPU spikes during shutdown

---

## Test File Organization

### Folder Structure

```
pkg/fsmv2/examples/integration_test/
├── phase1_critical_test.go          # 20 tests (deadlocks, leaks)
├── phase2_high_priority_test.go     # 25 tests (race conditions)
├── phase3_medium_priority_test.go   # 20 tests (error propagation)
├── phase4_low_priority_test.go      # 15 tests (observers, metrics)
├── phase5_integration_test.go       # 10 tests (full system)
├── test_helpers.go                  # Shared utilities
└── suite_test.go                    # Ginkgo test suite setup
```

### Naming Conventions

**Test files:**
- `phaseN_category_test.go` - Tests for phase N
- `test_helpers.go` - Shared helpers and utilities
- `suite_test.go` - Ginkgo suite configuration

**Test names:**
- `It("should timeout action after 5 seconds")` - Clear, readable
- NOT `It("test1")` or `It("timeout")` - Too vague

**Test structure:**
```go
var _ = Describe("ActionExecutor", func() {
    var (
        executor *ActionExecutor
        worker   *ExampleParent
        cleanup  func()
    )

    BeforeEach(func() {
        executor = NewActionExecutor()
        worker = NewExampleParent(mockDeps)
        cleanup = CheckForGoroutineLeaks(GinkgoT())
    })

    AfterEach(func() {
        executor.Shutdown()
        cleanup() // Check for leaks
    })

    Context("when action times out", func() {
        It("should return timeout error", func() {
            // Test implementation
        })
    })

    Context("when action completes normally", func() {
        It("should return success", func() {
            // Test implementation
        })
    })
})
```

### Ginkgo/Gomega Patterns

**Use Eventually for async assertions:**
```go
Eventually(func() State {
    return worker.GetState()
}).Should(Equal(StateRunning))
```

**Use Consistently for stability checks:**
```go
Consistently(func() int {
    return runtime.NumGoroutine()
}).Should(BeNumerically("<=", initialCount+5))
```

**Use BeforeEach/AfterEach for setup/teardown:**
```go
BeforeEach(func() {
    // Setup
})

AfterEach(func() {
    // Cleanup
})
```

**Use Context for grouping related tests:**
```go
Context("when worker is stopped", func() {
    BeforeEach(func() {
        worker.Stop()
    })

    It("should reject new actions", func() {
        // Test
    })
})
```

---

## Acceptance Criteria

Before marking this test plan as complete:

### Functional Criteria

- [ ] **All 90 tests implemented and passing**
  - Phase 1: 20 tests (Critical Failures)
  - Phase 2: 25 tests (High-Priority Edge Cases)
  - Phase 3: 20 tests (Medium-Priority Edge Cases)
  - Phase 4: 15 tests (Low-Priority Edge Cases)
  - Phase 5: 10 tests (Integration & E2E)

- [ ] **Example workers fully implemented**
  - example-parent worker with all states and actions
  - example-child worker with all states and actions
  - Mock dependencies (ConfigLoader, ConnectionPool)

- [ ] **No critical issues detected**
  - No deadlocks in 1000+ test runs
  - No goroutine leaks detected
  - No resource leaks (FDs, memory)
  - No data races detected with `-race`
  - No panics in production code

### Coverage Criteria

- [ ] **Coverage targets met**
  - supervisor.go: >80%
  - action_executor.go: >80%
  - collector.go: >80%
  - example-parent: >70%
  - example-child: >70%
  - Overall: >75%

- [ ] **All core code paths covered**
  - All state transitions tested
  - All error paths tested
  - All action types tested
  - All observer notifications tested

### Quality Criteria

- [ ] **All tests follow TDD**
  - Every test written RED-first
  - Every test verified RED (correct failure)
  - Every test verified GREEN (passes)
  - Refactoring completed after green

- [ ] **All tests are reliable**
  - No flaky tests (pass 100/100 times)
  - No timing-dependent tests without timeouts
  - All async tests use Eventually/Consistently
  - All tests clean up resources

- [ ] **All tests are maintainable**
  - Clear test names
  - Shared helpers extracted
  - No duplicated test code
  - Test helpers documented

### Documentation Criteria

- [ ] **Test plan complete** (this document)
  - All test categories documented
  - All tests described with TDD steps
  - Implementation order documented
  - Success criteria defined

- [ ] **Test helpers documented**
  - All helpers have doc comments
  - Usage examples provided
  - Gotchas documented

- [ ] **Worker examples documented**
  - example-parent behavior documented
  - example-child behavior documented
  - State machines documented
  - Action contracts documented

### Performance Criteria

- [ ] **No performance regressions**
  - Test suite completes in <5 minutes
  - No tests timeout
  - No unbounded resource growth
  - Load tests pass (1000+ concurrent actions)

- [ ] **Resource usage acceptable**
  - Goroutine count returns to baseline
  - Memory usage returns to baseline
  - File descriptors released promptly
  - No CPU spikes during normal operation

### Integration Criteria

- [ ] **Full system tested**
  - Multi-worker scenarios pass
  - Parent-child coordination works
  - Supervisor manages workers correctly
  - Observer notifications work
  - Metrics are emitted correctly

- [ ] **Production scenarios covered**
  - Start/stop workflows validated
  - Error recovery validated
  - Degraded state handling validated
  - Partial failure handling validated

---

## Changelog

### 2025-11-10 16:00 - Plan created
Initial comprehensive integration test plan for FSM v2:
- 90 tests across 8 categories and 5 phases
- Two example workers (example-parent, example-child) fully specified
- Detailed TDD steps for each test
- Implementation order and timeline (~15 days)
- Success criteria and acceptance criteria defined
- Test helpers and utilities documented
- Ginkgo/Gomega patterns documented

**Why this plan:**
- Systematic approach to catch all edge cases
- TDD ensures tests are meaningful (watch them fail first)
- Phased approach prioritizes critical bugs first
- Example workers provide realistic test scenarios
- Comprehensive coverage of production failure modes
