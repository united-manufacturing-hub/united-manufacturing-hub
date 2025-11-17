# FSMv2 Architecture Validation Results - Phase 1

## Overview

This document records the results of the architectural validation test suite created for ENG-3806. These tests document the **current violations** of FSMv2 architectural invariants in the example worker implementations.

**Test Status**: All 4 architectural validation tests are FAILING as expected (TDD: write failing tests first)

**Test Location**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/architecture_test.go`

**Date**: 2025-11-17

## Test Results Summary

| Test Category | Status | Violations Found | Affected Files |
|--------------|--------|------------------|----------------|
| Empty State Structs | FAIL | 13 violations | 7 state files |
| Stateless Actions | FAIL | 6 violations | 4 action files |
| Type Assertions in Next() | FAIL | 10 violations | 6 state files |
| Pure DeriveDesiredState() | FAIL | 1 violation | 1 worker file |
| **TOTAL** | **4/4 FAIL** | **30 violations** | **18 files** |

## Detailed Violation Breakdown

### 1. Empty State Violations (13 total)

**Invariant**: States should be pure behavior with no fields (only embedded base types)

**Pattern**: States are storing `deps` (dependencies) and `actionSubmitted` flags, which violates statelessness.

**Child States (7 violations)**:
- `TryingToConnectState`: deps, actionSubmitted
- `ConnectedState`: deps
- `DisconnectedState`: deps
- `TryingToStopState`: deps, actionSubmitted
- `StoppedState`: deps

**Parent States (6 violations)**:
- `TryingToStartState`: deps, actionSubmitted
- `RunningState`: deps
- `TryingToStopState`: deps, actionSubmitted
- `StoppedState`: deps

**Root Cause**: States are caching dependencies and tracking submission status internally instead of deriving this information from the context/snapshot.

### 2. Stateless Action Violations (6 total)

**Invariant**: Actions should be idempotent commands with no mutable state

**Pattern**: Actions are storing `dependencies`, `failureCount`, and `maxFailures` as fields.

**Violations**:
- `ConnectAction`: dependencies, failureCount, maxFailures (3 fields)
- `DisconnectAction`: dependencies (1 field)
- `StartAction`: dependencies (1 field)
- `StopAction`: dependencies (1 field)

**Root Cause**: Actions are designed to be instantiated once and reused, maintaining state across multiple invocations. This violates the "actions as pure commands" principle.

### 3. Type Assertion Violations (10 total)

**Invariant**: Use generics instead of type assertions in Next() methods

**Pattern**: States are using `.(ChildDependencies)` and `.(ParentDependencies)` type assertions to access dependency snapshots.

**Child State Violations (6)**:
- `state_trying_to_connect.go`: Line 37, 38
- `state_connected.go`: Line 35, 36
- `state_disconnected.go`: Line 35, 36

**Parent State Violations (4)**:
- `state_trying_to_start.go`: Line 37, 38
- `state_running.go`: Line 35, 36

**Root Cause**: Generic type system not fully leveraged for dependency snapshots. States are receiving `interface{}` and asserting to concrete types.

### 4. Pure DeriveDesiredState Violations (1 total)

**Invariant**: DeriveDesiredState() should only use typed UserSpec parameter, not access dependencies directly

**Violation**:
- `example-parent/worker.go:72`: Calls `GetDependencies()` directly

**Root Cause**: Worker is bypassing the typed UserSpec pattern by directly accessing dependency container.

## Analysis

### Architectural Pattern Violations

The violations reveal a systematic deviation from FSMv2 architectural principles:

1. **Stateful States**: States are caching data instead of being pure behavior
2. **Stateful Actions**: Actions maintain retry counts and dependencies
3. **Type Unsafety**: Runtime type assertions instead of compile-time generic constraints
4. **Dependency Injection Bypass**: Direct dependency access instead of typed specifications

### Impact

These violations affect:
- **Testability**: Stateful components are harder to test in isolation
- **Predictability**: Mutable state makes behavior non-deterministic
- **Type Safety**: Runtime type assertions can panic at runtime
- **Maintainability**: Dependencies scattered across components

### Affected Components

- **Child Worker**: All 5 states violated, both actions violated
- **Parent Worker**: All 4 states violated, both actions violated

## Next Steps (Phase 2)

The test suite is ready. Now we proceed to fix violations:

1. **Refactor States**: Remove all fields, make them pure behavior containers
2. **Refactor Actions**: Remove mutable state, pass all context via Execute() parameters
3. **Add Generic Constraints**: Replace type assertions with proper generic typing
4. **Fix DeriveDesiredState**: Use typed UserSpec pattern consistently

**Success Criteria**: All 4 architectural validation tests pass with 0 violations.

## Test Execution

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
ginkgo -v --focus "Architecture Validation" ./pkg/fsmv2
```

**Expected Outcome (Current)**: 4 failures, 30 total violations documented
**Expected Outcome (After Phase 2)**: 4 passes, 0 violations

## Related Documentation

- **Test Implementation**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/architecture_test.go`
- **ENG-3806**: Parent Linear ticket for FSMv2 architectural improvements
- **TDD Approach**: Tests written first (RED), fixes come next (GREEN), then refactor
