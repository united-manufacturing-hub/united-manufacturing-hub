# FSM v2 Architecture Fixes - Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Fix critical bugs and add missing features in the FSM v2 architecture to enable proper parent-child coordination and desired state flow.

**Created:** 2025-11-18
**Context:** Team standup feedback on FSM v2 (INS-58, PR #2315)

---

## Executive Summary

The FSM v2 architecture has **three critical issues** causing team confusion:

1. **Critical Bug:** `DeriveDesiredState` is defined but **NEVER CALLED** by the supervisor
2. **Missing Feature:** No state mapping API exists for parent-child state coordination
3. **Missing Feature:** No hierarchical FSM support - only flat architecture works

These issues explain why the team is confused about parent→child vs child→parent flow - **neither works correctly**.

---

## Problem Statement

### Why the Team is Confused

During the 2025-11-18 standup, the team reviewed FSM v2 and identified these pain points:

1. **State mapping API not discoverable** - Jeremy couldn't find it when asked because it doesn't exist
2. **Parent-child relationship direction unclear** - No documentation on how parents dictate child states
3. **Examples don't show desired state checks** - Transitions happen without checking if they should

### Observed Symptoms from Standup

> "Gibt's da hier eine API drin. Okay. United States. Ich weiß es grad nicht. Nicht mehr auswendig."
> — Jeremy, when asked where state mapping is configured

> "Ohne die historische Erfahrung ist es auch sehr schwer zu folgen... da sind so viele Informationen drin, dass überflutet mich grade"
> — Janik, on information overload

### Root Cause Analysis

**Location:** `pkg/fsmv2/supervisor/supervisor.go:tickWorker`

The supervisor's tick loop **never calls `DeriveDesiredState`**. This means:
- Desired state from user config never reaches workers
- State transitions happen based only on observed state
- Parent-child coordination is impossible

---

## Architecture Gaps

### Gap #1: DeriveDesiredState Never Called

**Interface defined in:** `pkg/fsmv2/worker.go:269-276`
```go
// DeriveDesiredState converts user configuration into desired state
DeriveDesiredState(spec interface{}) (DesiredState, error)
```

**Implementations exist:**
- `pkg/fsmv2/container/worker.go:92-106`
- `pkg/fsmv2/communicator/worker.go:188-197`
- `pkg/fsmv2/agent/worker.go:83-87`

**BUT: The supervisor never calls it!**

Per the README documentation (lines 60-65), it SHOULD be called on each tick:
```
3. On each tick:
   - Supervisor calls DeriveDesiredState() with latest config
   - Supervisor reads latest ObservedState from DB
   ...
```

### Gap #2: No State Mapping API

**What's needed:** A `StateMappingRegistry` that maps parent states to child desired states.

**Example use case:**
- Parent FSM is in `StartingChildren` state
- Registry maps this to: all children should be in `running` desired state
- Children check registry and transition accordingly

**Current situation:** No such registry exists. Engineers have no discoverable way to declare these mappings.

### Gap #3: No Parent-Child Coordination

The only "parent" references in the codebase are for context management, not FSM hierarchies.

**What's needed:**
- Parent FSM controls lifecycle of child FSMs
- Parent states trigger child transitions
- Children only transition when parent allows
- Parent waits for all children before advancing

---

## Solution Design

### Solution 1: Call DeriveDesiredState in Supervisor

**File:** `pkg/fsmv2/supervisor/supervisor.go`

Modify `tickWorker` to call `DeriveDesiredState` before evaluating transitions:

```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... existing code until snapshot loading

    // NEW: Derive desired state from config
    spec := s.configProvider.GetWorkerConfig(workerID)
    derived, err := workerCtx.worker.DeriveDesiredState(spec)
    if err != nil {
        return fmt.Errorf("failed to derive desired state: %w", err)
    }

    // NEW: Save derived desired state
    if err := s.store.SaveDesired(ctx, s.workerType, workerID, derived); err != nil {
        return fmt.Errorf("failed to save desired state: %w", err)
    }

    // NOW load snapshot (will have fresh desired state)
    snapshot, err := s.store.LoadSnapshot(ctx, s.workerType, workerID)
    // ... rest of tick
}
```

### Solution 2: Create State Mapping Registry

**New file:** `pkg/fsmv2/state_mapping.go`

```go
package fsmv2

// StateMapping defines how parent state maps to child desired states
type StateMapping struct {
    ParentState  string
    ChildDesired map[string]string // childID -> desiredState
    Condition    func(parentSnapshot Snapshot) bool // optional
}

// StateMappingRegistry holds mappings for hierarchical FSMs
type StateMappingRegistry struct {
    mappings []StateMapping
}

// DeriveChildDesiredStates returns desired states for all children
func (r *StateMappingRegistry) DeriveChildDesiredStates(
    parentState string,
    parentSnapshot Snapshot,
) map[string]string {
    result := make(map[string]string)

    for _, mapping := range r.mappings {
        if mapping.ParentState == parentState {
            if mapping.Condition == nil || mapping.Condition(parentSnapshot) {
                for childID, desiredState := range mapping.ChildDesired {
                    result[childID] = desiredState
                }
            }
        }
    }

    return result
}
```

**Usage example:**
```go
registry := &StateMappingRegistry{
    mappings: []StateMapping{
        {
            ParentState: "StartingChildren",
            ChildDesired: map[string]string{
                "s6-service-1": "running",
                "s6-service-2": "running",
            },
        },
        {
            ParentState: "StoppingChildren",
            ChildDesired: map[string]string{
                "s6-service-1": "stopped",
                "s6-service-2": "stopped",
            },
        },
    },
}
```

### Solution 3: Parent-Child Coordination Example

**New example:** `pkg/fsmv2/examples/parent_child/`

**Parent states with child coordination:**

```go
// ParentStoppedState - no children should be running
type ParentStoppedState struct{}

func (s *ParentStoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    desired := snapshot.Desired.(*ParentDesiredState)

    // Only start children if desired state says so
    if desired.ChildrenShouldBeRunning {
        return &ParentStartingChildrenState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

// ParentStartingChildrenState - waiting for children to start
type ParentStartingChildrenState struct{}

func (s *ParentStartingChildrenState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    desired := snapshot.Desired.(*ParentDesiredState)
    observed := snapshot.Observed.(*ParentObservedState)

    // Check shutdown first
    if desired.ShutdownRequested() {
        return &ParentStoppingChildrenState{}, fsmv2.SignalNone, nil
    }

    // Only advance when ALL children are running
    if observed.AllChildrenRunning {
        return &ParentChildrenRunningState{}, fsmv2.SignalNone, nil
    }

    // Still waiting - emit action to ensure children are started
    return s, fsmv2.SignalNone, &StartChildrenAction{
        childIDs: desired.ChildIDs,
    }
}

// ParentChildrenRunningState - normal operation
type ParentChildrenRunningState struct{}

func (s *ParentChildrenRunningState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    desired := snapshot.Desired.(*ParentDesiredState)
    observed := snapshot.Observed.(*ParentObservedState)

    if desired.ShutdownRequested() || !desired.ChildrenShouldBeRunning {
        return &ParentStoppingChildrenState{}, fsmv2.SignalNone, nil
    }

    // Monitor children - if any stop unexpectedly, restart them
    if !observed.AllChildrenRunning {
        return &ParentStartingChildrenState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}
```

### Solution 4: Fix Desired State Checks in Examples

Current examples have this bug:
```go
// INCORRECT: Transitions without checking desired state
if current == StopState {
    return TryingToStartState, nil  // Always transitions!
}
```

Should be:
```go
// CORRECT: Check desired state before transitioning
if current == StopState && desired.ShouldBeRunning() {
    return TryingToStartState, nil
}
```

---

## Implementation Phases

### Phase 1: Fix Critical Bug (1-2 hours)

**Priority:** CRITICAL - Must be first

| Task | File | Effort |
|------|------|--------|
| 1.1 Add ConfigProvider interface | `supervisor.go` | 30 min |
| 1.2 Call DeriveDesiredState in tickWorker | `supervisor.go` | 1 hour |
| 1.3 Add tests verifying the call | `supervisor_test.go` | 30 min |

### Phase 2: Create State Mapping API (2-3 hours)

**Priority:** HIGH - Enables discoverable parent-child coordination

| Task | File | Effort |
|------|------|--------|
| 2.1 Create StateMappingRegistry | `state_mapping.go` | 1 hour |
| 2.2 Add unit tests | `state_mapping_test.go` | 1 hour |
| 2.3 Document the API | `docs/state-mapping.md` | 1 hour |

### Phase 3: Parent-Child Example (3-4 hours)

**Priority:** HIGH - Addresses team's specific question

| Task | File | Effort |
|------|------|--------|
| 3.1 Create parent worker | `examples/parent_child/parent_worker.go` | 1.5 hours |
| 3.2 Create parent states | `examples/parent_child/states.go` | 1.5 hours |
| 3.3 Create actions | `examples/parent_child/actions.go` | 30 min |
| 3.4 Add integration test | `examples/parent_child/integration_test.go` | 1 hour |

### Phase 4: Fix Existing Examples (1-2 hours)

**Priority:** MEDIUM - Shows correct patterns

| Task | File | Effort |
|------|------|--------|
| 4.1 Add desired state checks | All example states | 1 hour |
| 4.2 Update tests | All example tests | 1 hour |

### Phase 5: Documentation (2-3 hours)

**Priority:** MEDIUM - Addresses team confusion

| Task | File | Effort |
|------|------|--------|
| 5.1 Getting started guide | `docs/getting-started.md` | 1.5 hours |
| 5.2 Common patterns | `docs/common-patterns.md` | 1 hour |
| 5.3 Update README | `README.md` | 30 min |

**Total estimated effort: 9-14 hours**

---

## Success Metrics

### Technical Verification

1. **DeriveDesiredState is called** - Test proves every tick calls it
2. **State mapping works** - Can map parent "StartingChildren" → child "running"
3. **Parent waits for children** - Integration test shows parent doesn't advance until children ready
4. **All examples check desired state** - No unconditional transitions

### Team Acceptance Criteria

From the standup, the goal was stated as:
> "Das ist praktisch das Ziel, dass Sie einfach ihr müsst nicht verstehen, warum existieren die Pattern... Aber das wird einfach im Lage sein, was Neues zu erstellen."

**Translation:** Team members should be able to create new FSMs without understanding Supervisor internals.

### Definition of Done

- [ ] DeriveDesiredState called in tickWorker
- [ ] StateMappingRegistry exists with tests
- [ ] Parent-child example works end-to-end
- [ ] All example states check desired state before transitioning
- [ ] Getting started guide exists
- [ ] Team can find state mapping API < 5 minutes
- [ ] PR #2315 updated with fixes

---

## Appendix

### Key File Locations

| Component | Current Location |
|-----------|------------------|
| Worker interface | `pkg/fsmv2/worker.go:269` |
| Supervisor tick | `pkg/fsmv2/supervisor/supervisor.go` (tickWorker) |
| Container worker | `pkg/fsmv2/container/worker.go` |
| Container states | `pkg/fsmv2/container/state_*.go` |
| Example FSMs | `pkg/fsmv2/examples/` |

### Files to Create

1. `pkg/fsmv2/state_mapping.go` - State mapping registry
2. `pkg/fsmv2/state_mapping_test.go` - Unit tests
3. `pkg/fsmv2/examples/parent_child/` - Complete example directory
4. `pkg/fsmv2/docs/getting-started.md` - Developer guide
5. `pkg/fsmv2/docs/common-patterns.md` - Common patterns
6. `pkg/fsmv2/docs/state-mapping.md` - State mapping guide

### Related Issues

- Linear: ENG-3806 (FSMv2 implementation)
- PR: #2315 (FSM v2 minimal in-memory architecture)
- Standup: INS-58 (2025-11-18 daily standup)

---

## Changelog

### 2025-11-18 - Plan created
Initial plan covering critical bug fix (DeriveDesiredState never called), state mapping API, parent-child example, and documentation improvements. Created in response to team confusion identified during standup triage (INS-58).
