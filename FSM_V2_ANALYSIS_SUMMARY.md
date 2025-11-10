# FSM v2 Worker System Analysis - Executive Summary

**Analysis Date**: November 8, 2025
**Scope**: Complete capability documentation for FSM v2 workers
**Based On**: `worker.go` (318 lines) and communicator worker implementation

---

## What This Analysis Provides

Complete documentation of ALL 13 capabilities that example workers must demonstrate to properly implement the FSM v2 pattern. This includes:

1. **Capability Matrix** - What each capability enables, when to use it, and why it matters
2. **Code Examples** - Real implementations from the communicator worker
3. **Pattern Documentation** - How each capability manifests in working code
4. **Line Number References** - Exact locations in `worker.go` for each concept
5. **Invariant Explanations** - Defense-in-depth for critical safety requirements
6. **Implementation Checklist** - Step-by-step guide for creating new workers

---

## The 13 Core Capabilities

### Worker Interface (5 capabilities)

| # | Capability | What It Does | Where Implemented |
|---|-----------|------------|-------------------|
| 1 | CollectObservedState: Async | Monitor actual system state in separate goroutine | Lines 276-297 |
| 2 | CollectObservedState: Errors | Handle monitoring failures gracefully | Lines 289-292 |
| 3 | DeriveDesiredState: Pure | Transform config to state deterministically | Lines 299-316 |
| 4 | DeriveDesiredState: Compose | Declare child workers via ChildrenSpecs | Lines 305-310 |
| 5 | GetInitialState | Return FSM entry point | Lines 312-316 |

### State Machine (4 capabilities)

| # | Capability | What It Does | Pattern |
|---|-----------|------------|---------|
| 6 | Signal Types | Communicate with supervisor (remove, restart) | Lines 38-49 |
| 7 | Active States | "TryingTo*" states that retry with actions | Lines 180-194 |
| 8 | Passive States | Descriptive noun states that observe only | Lines 180-194 |
| 9 | State Transitions | Guard conditions in priority order | Lines 210-250 |

### System Properties (4 capabilities)

| # | Capability | What It Does | Enforced By |
|---|-----------|------------|-------------|
| 10 | Snapshot Immutability | Snapshot is read-only in state transitions | Go language (pass-by-value) |
| 11 | Action Idempotency | Actions safe to retry multiple times | Check-then-act pattern |
| 12 | Context Cancellation | Graceful shutdown of long operations | Context parameter |
| 13 | ObservedState Interface | Standard observation query interface | Interface contract |

---

## Critical Invariants

Four invariants from `worker.go` form the foundation of FSM v2 safety:

### Invariant I4 (Line 174-175): Shutdown Check Priority
```
States MUST check ShutdownRequested() first in Next()
```
**Why**: Ensures graceful shutdown takes precedence over all other state transitions.
**Impact**: Every state's Next() method must start with shutdown check.

### Invariant I6 (Line 162-164): Context Cancellation
```
CollectObservedState and Action.Execute() must respect context cancellation
```
**Why**: Enforces proper async lifecycle management and clean shutdown.
**Impact**: 5-second grace period enforced by supervisor; violation causes panic.

### Invariant I9 (Line 86-114): Snapshot Immutability
```
Snapshot is passed by value (copied) to State.Next()
```
**Why**: Guarantees pure functional transitions without side effects.
**Impact**: Go compiler enforces this; no defensive copying needed.

### Invariant I10 (Line 116-167): Action Idempotency
```
Every Action must be safe to call multiple times
```
**Why**: Enables safe retries on transient failures without corrupting state.
**Impact**: Required test: `VerifyActionIdempotency(action, 3, ...)`

---

## Documentation Files

### 1. FSM_V2_WORKER_CAPABILITIES.md (1937 lines, 59 KB)

**Comprehensive Analysis**
- 13 detailed capability sections
- Each with: purpose, when to use, code examples, key requirements
- Communicator worker used as reference for all 13 capabilities
- Defense-in-depth explanations for each invariant
- Line number references to worker.go
- Patterns showing both good and bad implementations
- Common mistakes and how to avoid them

**Use For**:
- Deep understanding of each capability
- Code review of worker implementations
- Training new developers
- Designing new worker types

### 2. FSM_V2_QUICK_REFERENCE.md (307 lines, 9.1 KB)

**Rapid Implementation Guide**
- 13 capabilities condensed to code snippets
- Implementation order (10 steps)
- Key invariants summary table
- Testing patterns (idempotency, transitions)
- Common mistakes checklist
- Reference file locations
- One-page overview for quick lookup

**Use For**:
- Quick implementation checklist
- Code review verification
- Implementation order when creating workers
- Testing pattern reference

---

## Key Findings from Source Analysis

### Communicator Worker Demonstrates All 13 Capabilities

**File Structure**:
```
communicator/
├── worker.go (270 lines)          ← Worker interface implementation
├── state/
│   ├── state_stopped.go           ← Passive state pattern
│   ├── state_trying_to_authenticate.go  ← Active state pattern
│   ├── state_syncing.go           ← Loop state (stays in self)
│   └── base.go
├── action/
│   ├── authenticate.go            ← Idempotent action example
│   └── sync.go                    ← Context-aware action
└── snapshot/
    └── snapshot.go                ← ObservedState interface
```

### Pattern Highlights

**State Naming Convention**:
- Active: `TryingToAuthenticateState` - emits action every tick
- Passive: `SyncingState`, `StoppedState` - observes, transitions only
- Naming clearly conveys behavior in code

**State Transitions**:
```
Stopped → TryingToAuthenticate → Syncing → Syncing (loop)
   ↓             ↓                  ↓
  Error ←──── Error ←────────── Error
```

**Action Pattern**:
- SyncAction: Simple idempotent operation (GET/POST with idempotency)
- No side effects beyond network calls
- Respects context cancellation
- Safe to retry on transient failures

**ObservedState Pattern**:
- Always includes CollectedAt timestamp
- Embeds DesiredState for comparison
- Provides helper methods (IsTokenExpired, IsSyncHealthy)
- Implements required interface methods

---

## Implementation Workflow

### For Creating a New Example Worker

**Phase 1: Define Structures** (1-2 hours)
1. ObservedState struct with timestamp
2. DesiredState struct with ShutdownRequested()
3. Implement ObservedState interface

**Phase 2: Worker Interface** (2-3 hours)
1. Implement CollectObservedState (with context, error handling)
2. Implement DeriveDesiredState (pure function)
3. Implement GetInitialState

**Phase 3: State Machine** (3-4 hours)
1. Create initial/stopped state (passive)
2. Create active state ("TryingTo*" prefix)
3. Create operational state (passive noun)
4. Add state transitions with guard conditions
5. Use Signal types correctly

**Phase 4: Actions** (2-3 hours)
1. Implement action with context handling
2. Ensure idempotency (check-then-act)
3. Implement Name() method

**Phase 5: Testing** (2-3 hours)
1. Idempotency test using VerifyActionIdempotency
2. State transition tests
3. Integration tests with supervisor

**Total Estimated Time**: 10-16 hours for complete example worker

---

## Critical Success Factors

### 1. Action Idempotency (Most Important)
- **Why**: Supervisor retries failed actions with exponential backoff
- **Pattern**: Check if work is done before doing it
- **Example**: `if fileExists() { return nil }`
- **Test**: Every action MUST have `VerifyActionIdempotency` test

### 2. Context Cancellation
- **Why**: Supervisor enforces 5-second grace period
- **Pattern**: Pass ctx to all blocking operations, check in loops
- **Violation**: Failure to exit causes panic
- **Example**: `select { case <-ctx.Done(): return ctx.Err() }`

### 3. Shutdown Check Priority
- **Why**: Graceful shutdown must take precedence
- **Pattern**: Always check `ShutdownRequested()` first in Next()
- **Impact**: Every state needs this check
- **Benefit**: Clean, predictable shutdown behavior

### 4. Pure DeriveDesiredState
- **Why**: Deterministic state computation enables testing
- **Pattern**: No side effects, no external state
- **Validation**: Same input always produces same output
- **Benefit**: Easy to test, no hidden dependencies

### 5. Snapshot as Read-Only
- **Why**: Guarantees pure functional transitions
- **Pattern**: Read from snapshot, don't mutate
- **Enforcement**: Go compiler enforces pass-by-value
- **Benefit**: Safe concurrent access, no race conditions

---

## Common Mistakes and Corrections

| Mistake | Why It's Wrong | Correct Pattern |
|---------|---------------|-----------------|
| Not checking shutdown first | Prevents graceful shutdown | Check `ShutdownRequested()` first |
| Non-idempotent actions | Fails on retries (counter++) | Check-then-act: `if done() return nil` |
| Not respecting context | Blocks shutdown indefinitely | Pass ctx to all blocking ops |
| Passive state emitting action | Violates naming convention | Passive states: transition only |
| Mutating snapshot | Breaks pure function guarantee | Treat snapshot as read-only |
| No timestamp in ObservedState | Prevents staleness detection | Always include CollectedAt |
| Action without test | Hides non-idempotency bugs | Use VerifyActionIdempotency |
| Long operations without timeout | Can hang indefinitely | Supervisor enforces 2.2s timeout |
| Missing error handling | Monitoring failure stops FSM | Log errors, return last state |
| Multiple actions per tick | Violates supervisor assumption | Emit action OR change state, not both |

---

## Testing Patterns

### Idempotency Test (Required for Every Action)

```go
It("should be idempotent", func() {
	action := &MyAction{}
	VerifyActionIdempotency(action, 5, func() {
		// Verify final state is identical after 5 calls
		Expect(finalState).To(Equal(expectedState))
	})
})
```

### State Transition Test

```go
It("should transition on success", func() {
	state := &TryingToStartState{}
	snapshot := Snapshot{
		Observed: &MyObservedState{Running: true},
		Desired: &MyDesiredState{},
	}
	
	nextState, signal, action := state.Next(snapshot)
	
	Expect(nextState).To(BeAssignableToTypeOf(&RunningState{}))
	Expect(signal).To(Equal(fsmv2.SignalNone))
	Expect(action).To(BeNil())
})
```

### Context Cancellation Test

```go
It("should respect context cancellation", func() {
	ctx, cancel := context.WithCancel(context.Background())
	action := &MyAction{}
	
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	
	err := action.Execute(ctx)
	Expect(err).To(Equal(context.Canceled))
})
```

---

## How to Use These Documents

### For Implementation

1. **Start**: Read FSM_V2_QUICK_REFERENCE.md (20 minutes)
2. **Plan**: Follow the 10-step implementation order
3. **Code**: Refer to specific capabilities in FSM_V2_WORKER_CAPABILITIES.md
4. **Reference**: Use code examples from communicator worker
5. **Test**: Copy testing patterns from FSM_V2_QUICK_REFERENCE.md

### For Code Review

1. **Checklist**: Use "13 Capabilities" from FSM_V2_QUICK_REFERENCE.md
2. **Patterns**: Compare against code examples in FSM_V2_WORKER_CAPABILITIES.md
3. **Mistakes**: Check "Common Mistakes to Avoid" section
4. **Tests**: Verify idempotency tests present for all actions
5. **Invariants**: Confirm I4, I6, I9, I10 are respected

### For Training

1. **Overview**: FSM_V2_ANALYSIS_SUMMARY.md (this document)
2. **Deep Dive**: Each capability section in FSM_V2_WORKER_CAPABILITIES.md
3. **Hands-On**: Implement minimal worker following FSM_V2_QUICK_REFERENCE.md
4. **Reference**: Study communicator worker source code in parallel

---

## File Locations

**Documentation**:
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/FSM_V2_ANALYSIS_SUMMARY.md` (this file)
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/FSM_V2_WORKER_CAPABILITIES.md` (comprehensive)
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/FSM_V2_QUICK_REFERENCE.md` (quick guide)

**Source Code**:
- Core: `/pkg/fsmv2/worker.go`
- Example: `/pkg/fsmv2/workers/communicator/`
- Patterns: `/pkg/fsmv2/examples/template_worker.go`
- Tests: `/pkg/fsmv2/supervisor/execution/action_idempotency_test.go`

---

## Conclusion

The FSM v2 system provides a robust, well-designed pattern for implementing complex state machines with proper async handling, error resilience, and safe retry semantics. This analysis documents all 13 capabilities required for workers to properly participate in the FSM v2 ecosystem.

The communicator worker serves as an excellent reference implementation demonstrating all 13 capabilities in production-quality code. New workers should follow the same patterns and structure to ensure consistency and maintainability across the system.

---

**Analysis Completion**: ✓ All 13 capabilities identified
**Documentation**: ✓ 2244 lines across 3 files
**Code Coverage**: ✓ All capabilities demonstrated in communicator
**Ready For**: New worker creation, code review, developer training
