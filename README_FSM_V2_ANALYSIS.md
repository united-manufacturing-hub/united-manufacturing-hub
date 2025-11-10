# FSM v2 Worker System - Complete Analysis

This directory contains comprehensive documentation of the FSM v2 worker system, analyzing all capabilities that example workers must demonstrate.

## Documentation Files

### 1. FSM_V2_ANALYSIS_SUMMARY.md (Start Here)
**Size**: 13 KB | **Read Time**: 15 minutes

Executive summary providing:
- Overview of all 13 capabilities
- What each capability does and why it matters
- Critical invariants (I4, I6, I9, I10)
- Implementation workflow with time estimates
- Common mistakes with corrections
- How to use all three documents

**Best For**: First-time readers, managers, training overview

### 2. FSM_V2_QUICK_REFERENCE.md (Quick Lookup)
**Size**: 9.1 KB | **Read Time**: 10 minutes

Rapid implementation guide with:
- Code snippets for all 13 capabilities
- Implementation order (10 steps)
- Invariants summary table
- Testing patterns (copy-paste ready)
- Common mistakes checklist
- File location reference

**Best For**: Implementation, code review, quick lookup

### 3. FSM_V2_WORKER_CAPABILITIES.md (Deep Dive)
**Size**: 59 KB | **Read Time**: 60 minutes

Comprehensive analysis with:
- 13 detailed capability sections
- Purpose, when to use, and requirements for each
- Code examples from communicator worker
- Defense-in-depth explanations
- Line number references to worker.go
- Good and bad implementation patterns
- Minimum viable example worker checklist

**Best For**: Developers, code review, training, design decisions

## The 13 Core Capabilities

Every FSM v2 worker must demonstrate these capabilities:

### Worker Interface
1. **CollectObservedState: Async** - Monitor system state in separate goroutine
2. **CollectObservedState: Errors** - Handle monitoring failures gracefully  
3. **DeriveDesiredState: Pure** - Transform configuration deterministically
4. **DeriveDesiredState: Compose** - Declare child workers via ChildrenSpecs
5. **GetInitialState** - Return FSM entry point

### State Machine
6. **Signal Types** - Supervisor communication (remove, restart)
7. **Active States** - "TryingTo*" states that emit actions on every tick
8. **Passive States** - Descriptive noun states that only observe
9. **State Transitions** - Guard conditions in priority order

### System Properties
10. **Snapshot Immutability** - Read-only snapshot in state transitions
11. **Action Idempotency** - Safe to retry multiple times (CRITICAL)
12. **Context Cancellation** - Graceful shutdown of long operations
13. **ObservedState Interface** - Standard observation query interface

## Source Code References

All capabilities are demonstrated in the communicator worker:

```
Core Interface:
  /umh-core/pkg/fsmv2/worker.go (318 lines)

Example Implementation:
  /umh-core/pkg/fsmv2/workers/communicator/
    ├── worker.go (270 lines)
    ├── state/state_stopped.go (91 lines)
    ├── state/state_trying_to_authenticate.go (115 lines)
    ├── state/state_syncing.go (136 lines)
    ├── action/sync.go (119 lines)
    └── snapshot/snapshot.go (168 lines)

Supporting Examples:
  /umh-core/pkg/fsmv2/examples/template_worker.go (143 lines)

Test Patterns:
  /umh-core/pkg/fsmv2/supervisor/execution/action_idempotency_test.go
```

## How to Use This Analysis

### For New Worker Implementation
1. Read FSM_V2_ANALYSIS_SUMMARY.md (15 min)
2. Follow 10-step order in FSM_V2_QUICK_REFERENCE.md
3. Refer to specific capabilities in FSM_V2_WORKER_CAPABILITIES.md
4. Study communicator worker source code
5. Use code examples as templates

**Estimated Time**: 10-16 hours for complete worker

### For Code Review
1. Use 13-capability checklist from FSM_V2_QUICK_REFERENCE.md
2. Compare patterns against FSM_V2_WORKER_CAPABILITIES.md examples
3. Verify all actions have idempotency tests
4. Confirm I4, I6, I9, I10 invariants are respected
5. Check "Common Mistakes" section

### For Training
1. Start with FSM_V2_ANALYSIS_SUMMARY.md overview
2. Deep dive with each section in FSM_V2_WORKER_CAPABILITIES.md
3. Hands-on: Implement minimal worker from FSM_V2_QUICK_REFERENCE.md
4. Reference communicator worker alongside docs

### For Architecture Decisions
1. Read FSM_V2_ANALYSIS_SUMMARY.md "Critical Success Factors"
2. Review communicator pattern highlights
3. Consult specific capability sections in FSM_V2_WORKER_CAPABILITIES.md
4. Reference key invariants for system design

## Critical Success Factors

### 1. Action Idempotency (Most Important)
Every action must be safe to call multiple times. Use check-then-act pattern:
```go
if alreadyDone() { return nil }
return doWork()
```
**Required**: Every action must have `VerifyActionIdempotency` test.

### 2. Context Cancellation
Pass context to all blocking operations. Supervisor enforces 5-second grace period.
Violation causes panic.

### 3. Shutdown Check Priority
Always check `ShutdownRequested()` first in `State.Next()`.
Ensures graceful shutdown takes precedence.

### 4. Pure DeriveDesiredState
No side effects, no external state. Same input → same output.
Enables easy testing and deterministic behavior.

### 5. Snapshot as Read-Only
Go's pass-by-value enforces immutability automatically.
Safe concurrent access without defensive copying.

## Key Invariants

| Line | ID | Requirement |
|------|-----|-------------|
| 86-114 | I9 | Snapshot immutability via pass-by-value |
| 116-167 | I10 | Action idempotency (check-then-act) |
| 162-164 | I6 | Context cancellation in async operations |
| 174-175 | I4 | Shutdown check priority (first in Next()) |

## Common Mistakes

See "Common Mistakes to Avoid" in FSM_V2_QUICK_REFERENCE.md for:
- Not checking shutdown first
- Non-idempotent actions
- Not respecting context
- Passive states emitting actions
- Mutating snapshot
- Missing timestamp in ObservedState
- No error handling in CollectObservedState
- And 3 more...

## Testing Patterns

### Idempotency Test (Required)
```go
VerifyActionIdempotency(action, 3, func() {
    // Verify final state identical after 3 calls
})
```

### State Transition Test
```go
nextState, signal, action := state.Next(snapshot)
Expect(nextState).To(BeAssignableToTypeOf(&ExpectedState{}))
```

See FSM_V2_QUICK_REFERENCE.md for complete examples.

## Questions?

1. **What capability does X enable?** → See FSM_V2_ANALYSIS_SUMMARY.md capability table
2. **How do I implement Y?** → See code snippet in FSM_V2_QUICK_REFERENCE.md
3. **Why does the code do Z?** → See explanation in FSM_V2_WORKER_CAPABILITIES.md
4. **Is my implementation correct?** → Check against communicator worker example
5. **Am I missing anything?** → Use 13-capability checklist in FSM_V2_QUICK_REFERENCE.md

## Status

- **Analysis Date**: November 8, 2025
- **Scope**: Complete FSM v2 worker capabilities
- **Coverage**: All 13 capabilities documented
- **Source**: worker.go (318 lines) + communicator worker (1313 lines)
- **Documentation**: 2544 lines across 3 files
- **Ready For**: Production worker implementation

---

**Start with**: FSM_V2_ANALYSIS_SUMMARY.md
**Quick lookup**: FSM_V2_QUICK_REFERENCE.md
**Deep reference**: FSM_V2_WORKER_CAPABILITIES.md
