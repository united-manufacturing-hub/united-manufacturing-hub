# FSMv2 Supervision and Async Actions Plan - Critical Evaluation

**Created:** 2025-11-02
**Status:** Critical Review - Major Revisions Required
**Evaluator:** Claude Code
**Context:** UMH-CORE-ENG-3806 - FSMv2 Hierarchical Composition

---

## Executive Summary

### Overall Assessment: ⚠️ NEEDS MAJOR REVISION

The plan has **fundamental architectural issues** that contradict our established design principles. While the basic concepts (circuit breaker, async actions) are sound, the implementation strategy violates the core principle: **"workers remain simple, infrastructure stays in supervisor."**

### Critical Finding

**The plan forces workers to implement infrastructure boilerplate** that should be handled by the supervisor. This is the opposite of what we designed.

### Verdict by User Concern

| User Concern | Status | Summary |
|--------------|--------|---------|
| **CheckChildConsistency() clarity** | ⚠️ PARTIALLY ADDRESSED | Pattern exists but requires worker boilerplate |
| **Too much boilerplate** | ❌ MAJOR ISSUE | Plan adds significant worker complexity |
| **Missing DeriveDesiredState()** | ❌ COMPLETELY MISSING | No mention of dynamic child management |

---

## Table of Contents

1. [User Concerns Analysis](#user-concerns-analysis)
2. [Design Principle Compliance](#design-principle-compliance)
3. [Specific Issues Found](#specific-issues-found)
4. [Boilerplate Analysis](#boilerplate-analysis)
5. [Recommended Simplifications](#recommended-simplifications)
6. [What to Keep vs Revise](#what-to-keep-vs-revise)
7. [Action Items](#action-items)

---

## User Concerns Analysis

### Concern 1: "I don't really get CheckChildConsistency() and the HealthChecker"

**What the plan says:**

```go
// Task 1.3: Child Consistency Check Interface (lines 536-686)
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    redpandaState, err := ihc.supervisor.GetChildState("redpanda")
    benthosObserved, err := ihc.supervisor.getChildObservedInternal("dfc_read")
    
    // Sanity check: Redpanda active but Benthos has no output
    if redpandaState == "active" && benthosObserved.OutputConnections == 0 {
        return fmt.Errorf("redpanda active but benthos has 0 output connections")
    }
}
```

**Who calls it:** Supervisor in `Tick()` loop (line 66-87)

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health (affects all workers)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        time.Sleep(backoff)
        return nil  // Skip entire tick
    }
    // ...
}
```

**Analysis:**

✅ **Good:** Supervisor calls it, not workers
✅ **Good:** Uses supervisor-internal method `getChildObservedInternal()` (line 638)
✅ **Good:** Example shows cross-child sanity check

❌ **Problem:** Requires workers to know about this pattern (Task 1.3 description)
❌ **Problem:** No clear guidance on how to add custom sanity checks
⚠️ **Missing:** How do workers declare what sanity checks they need?

**Clarity Assessment:** PARTIALLY ADDRESSED
- Pattern exists and is supervisor-driven ✅
- But implementation details unclear ⚠️
- Needs better documentation on extensibility

---

### Concern 2: "It looks like a lot of boilerplate code"

**What the plan requires workers to implement:**

From Phase 2 (Async Action Executor), lines 694-705:
```
### Task 2.1: ActionExecutor Core Structure
### Task 2.2: Worker Pool Implementation
### Task 2.3: Action Queuing
### Task 2.4: HasActionInProgress Check
### Task 2.5: Integration with Supervisor.tickWorker()
```

**No task breakdown provided!** This is a RED FLAG - suggests complexity not fully thought through.

**Inferred from Integration Strategy (lines 64-87):**

```go
// Supervisor tick loop
for workerID, workerCtx := range s.workers {
    // ALWAYS collect observations
    observed, _ := workerCtx.collector.GetLatestObservation(ctx)

    if s.actionExecutor.HasActionInProgress(workerID) {
        continue  // Skip derivation for this worker
    }

    // Derive and enqueue action
    nextState, _, action := currentState.Next(snapshot)
    s.actionExecutor.EnqueueAction(workerID, action, registry)
}
```

**Boilerplate Analysis:**

❌ **MAJOR ISSUE:** This code is in SUPERVISOR, not in worker
❌ **But wait:** Workers need to call `EnqueueAction()` somehow?
❌ **Unclear:** Where does worker call `HasActionInProgress()`?

**Looking at existing async-action-executor plan (referenced line 39-44):**

From `docs/plans/async-action-executor-implementation.md`:
- Workers DON'T directly interact with ActionExecutor
- Supervisor wraps action execution
- Workers just return actions from `Next()`

**Boilerplate Assessment:** NEEDS CLARIFICATION
- Plan integration strategy shows supervisor code ✅
- But doesn't show what workers must implement ❌
- Existing async plan suggests workers stay simple ✅
- **Verdict:** Likely OK for async actions, but needs explicit confirmation

---

### Concern 3: "Where is DeriveDesiredState(userSpec)?"

**What the plan says:**

**NOTHING.** The entire plan never mentions `DeriveDesiredState()`.

**Searching the plan:**
```bash
grep -i "derive" /path/to/plan.md
# Result: 0 matches

grep -i "desired.*state" /path/to/plan.md
# Result: Only references to SetDesiredState("stopped"/"running") in child restart logic
```

**Critical Missing Pieces:**

1. **How do workers declare children dynamically?**
   - User adds bridge → Worker calls `DeriveDesiredState()` → Adds child to declarations
   - Plan has no mechanism for this

2. **How does supervisor know WHEN to check consistency?**
   - Only check when children actually declared
   - Plan assumes children always exist (hardcoded "redpanda", "dfc_read")

3. **How do children get REMOVED?**
   - User deletes bridge → Worker stops declaring child in `DeriveDesiredState()`
   - Supervisor removes child
   - Plan has no mechanism for this

4. **State mapping integration:**
   - Parent state → Child desired state mapping
   - Plan shows `SetDesiredState("stopped"/"running")` but no mapping logic

**Example from design docs (fsmv2-supervisor-composition-declarative.md):**

```go
func (w *ProtocolConverterWorker) DeriveDesiredState(ctx context.Context, userSpec UserSpec, observed ObservedState) DesiredState {
    desired := DesiredState{
        Children: []ChildDeclaration{
            {
                ID:         "connection-1",
                WorkerType: "connection",
                StateMapping: map[string]string{
                    "active": "up",      // Parent active → Child up
                    "idle":   "down",    // Parent idle → Child down
                },
            },
        },
    }
    
    // Add bridge child if configured
    if userSpec.BridgeConfig != nil {
        desired.Children = append(desired.Children, ChildDeclaration{
            ID:         "bridge-1",
            WorkerType: "dataflow",
            StateMapping: map[string]string{
                "active": "running",
                "idle":   "stopped",
            },
        })
    }
    
    return desired
}
```

**DeriveDesiredState() Assessment:** ❌ COMPLETELY MISSING
- Plan has no mechanism for declaring/removing children
- Plan assumes static child declarations
- This is a FUNDAMENTAL gap in the architecture

---

## Design Principle Compliance

### From Infrastructure Supervision Patterns

| Principle | Plan Compliance | Evidence |
|-----------|----------------|----------|
| Infrastructure invisible to business logic | ⚠️ PARTIAL | CheckChildConsistency() in supervisor ✅ but unclear worker impact |
| Circuit breaker pattern | ✅ GOOD | Lines 66-87 show circuit breaker in Supervisor.Tick() |
| Automatic restart on failures | ✅ GOOD | restartChild() implementation (lines 169-188) |
| No mixing business states with infrastructure | ✅ GOOD | States stay in FSM, infra in supervisor |
| Sanity checks in supervisor | ✅ GOOD | CheckChildConsistency() is supervisor method |

**Overall:** 4/5 ✅ Strong compliance with infrastructure patterns

---

### From Developer Expectations

| Principle | Plan Compliance | Evidence |
|-----------|----------------|----------|
| Simple API: GetChildState() returns string | ⚠️ UNCLEAR | Plan doesn't show worker-side API |
| No IsChildHealthy() exposed | ✅ GOOD | Not mentioned in plan |
| Supervisor handles crashes/timeouts invisibly | ✅ GOOD | Circuit breaker + restart logic |
| Workers don't handle infrastructure | ⚠️ UNCLEAR | No worker code shown |

**Overall:** 2/4 ✅ Unclear because **plan shows almost no worker code**

---

### From Child State Usage

| Principle | Plan Compliance | Evidence |
|-----------|----------------|----------|
| Workers should NOT access child observed state | ⚠️ UNCLEAR | Plan doesn't show worker API |
| Sanity checks moved to supervisor | ✅ GOOD | CheckChildConsistency() |
| Parents only query child state names | ⚠️ UNCLEAR | No worker examples |

**Overall:** 1/3 ✅ Good sanity check pattern, but missing worker API design

---

## Specific Issues Found

### Issue 1: No Worker API Definition

**Problem:** Plan shows only supervisor implementation, no worker-facing API.

**What's missing:**

```go
// Workers need this API - NOT IN PLAN
type Snapshot struct {
    supervisor *Supervisor  // How do workers get child state?
}

func (s *Snapshot) GetChildState(name string) string {
    // Implementation missing from plan
}
```

**Impact:**
- Can't verify workers stay simple
- Can't verify boilerplate amount
- Can't verify design principles

---

### Issue 2: Static Child Declarations

**Problem:** Plan hardcodes child names ("redpanda", "dfc_read") everywhere.

**Evidence:**

Line 68-69:
```go
if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
    s.restartChild(ctx, "dfc_read")  // Hardcoded!
```

Line 604:
```go
redpandaState, err := ihc.supervisor.GetChildState("redpanda")  // Hardcoded!
benthosObserved, err := ihc.supervisor.getChildObservedInternal("dfc_read")  // Hardcoded!
```

**What's missing:**
- How are children declared initially?
- How does supervisor know which children exist?
- How are children added/removed dynamically?

**This contradicts declarative composition pattern!**

---

### Issue 3: Unclear CheckChildConsistency() Extensibility

**Problem:** Plan shows ONE sanity check (Redpanda vs Benthos). How do workers add custom checks?

**Options:**

**Option A: Workers register custom checks (BAD - adds boilerplate)**
```go
func (w *ProtocolConverterWorker) RegisterSanityChecks(checker *InfrastructureHealthChecker) {
    checker.AddCheck("redpanda-benthos", func() error {
        // Custom check logic
    })
}
```

**Option B: Supervisor implements all checks (BETTER - but not scalable)**
```go
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Hardcoded checks for all possible parent-child combinations
    // Not scalable across different FSMv2 implementations
}
```

**Option C: Declarative sanity checks (BEST - but not in plan)**
```go
func (w *ProtocolConverterWorker) DeriveDesiredState(...) DesiredState {
    return DesiredState{
        Children: []ChildDeclaration{...},
        SanityChecks: []SanityCheck{
            {
                Name: "redpanda-benthos-consistency",
                Check: func(supervisor *Supervisor) error {
                    // Access supervisor-internal state
                    redpanda := supervisor.GetChildState("redpanda")
                    benthos := supervisor.getChildObservedInternal("dfc_read")
                    // Return error if inconsistent
                },
            },
        },
    }
}
```

**Plan doesn't specify which option!**

---

### Issue 4: DeriveDesiredState() Integration Missing

**Problem:** No connection between user configuration changes and child lifecycle.

**What happens when:**

1. **User adds bridge via Management Console**
   - Action: `deploy-protocol-converter`
   - Agent updates `config.yaml`
   - **Then what?** Plan doesn't say
   - **Expected:** Worker's `DeriveDesiredState()` called → Child declared → Supervisor creates child

2. **User deletes bridge**
   - Action: `delete-protocol-converter`
   - Agent updates `config.yaml`
   - **Then what?** Plan doesn't say
   - **Expected:** Worker stops declaring child → Supervisor removes child

3. **User changes bridge configuration**
   - Action: `edit-protocol-converter`
   - Agent updates `config.yaml`
   - **Then what?** Plan doesn't say
   - **Expected:** Worker updates child state mapping → Supervisor applies changes

**Plan has ZERO mention of this critical path!**

---

### Issue 5: Async Actions Integration Unclear

**Problem:** Phase 2 (lines 694-705) has NO task breakdown.

**This suggests:**
- Pattern not fully thought through
- Complexity underestimated
- Integration strategy incomplete

**What's missing:**
- How do workers return actions?
- How does supervisor enqueue actions?
- What happens when action fails?
- How do actions access registry?

**Referenced plan (`async-action-executor-implementation.md`):**
- Supposedly has answers
- But this plan doesn't summarize key points
- Integration strategy (lines 64-87) shows supervisor code, not worker code

---

## Boilerplate Analysis

### What Workers Currently Implement (FSMv1)

```go
// FSMv1 worker
type ProtocolConverterWorker struct {
    // No supervisor interaction needed
}

func (w *ProtocolConverterWorker) GetInitialState() State {
    return &InitializingState{}
}

// States implement Next()
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Business logic only
    if snapshot.GetChildState("connection") != "up" {
        return s, SignalNone, nil
    }
    return &IdleState{}, SignalNone, nil
}
```

**Boilerplate:** Minimal - just state machine logic

---

### What Plan Requires Workers to Implement (Inferred)

**From integration strategy (lines 64-87):**

```go
// FSMv2 worker (INFERRED - not explicitly shown)
type ProtocolConverterWorker struct {
    // Unknown if any supervisor interaction needed
}

func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec, observed) DesiredState {
    // MISSING FROM PLAN - where is this called?
    return DesiredState{
        Children: []ChildDeclaration{...},
        // SanityChecks: ??? - how do workers specify these?
    }
}

func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Same as FSMv1?
    if snapshot.GetChildState("connection") != "up" {
        return s, SignalNone, nil
    }
    return &IdleState{}, SignalNone, nil
}
```

**Boilerplate:** UNKNOWN - plan doesn't show worker code!

---

### Boilerplate Verdict

**Cannot assess boilerplate without seeing worker-side implementation.**

**What we need:**
1. Complete worker example showing all methods
2. Comparison to FSMv1 worker complexity
3. Explicit list of what workers MUST vs MAY implement

**Current plan fails to provide this!**

---

## Recommended Simplifications

### Simplification 1: Explicit Worker API Contract

**Add to plan:**

```go
// pkg/fsmv2/worker/interface.go

type Worker interface {
    // FSMv1 compatibility
    GetInitialState() State
    
    // FSMv2 hierarchical composition
    DeriveDesiredState(ctx context.Context, userSpec UserSpec, observed ObservedState) DesiredState
}

type DesiredState struct {
    // Declarative child declarations
    Children []ChildDeclaration
    
    // Optional: Custom sanity checks (infrastructure layer)
    SanityChecks []SanityCheck
}

// Snapshot is what states receive in Next()
type Snapshot struct {
    // Simple child state query - NO ERRORS
    GetChildState(name string) string
    
    // NEVER EXPOSED: GetChildObserved(), IsChildHealthy(), etc.
}
```

**Why:** Makes worker requirements explicit, verifies simplicity principle.

---

### Simplification 2: Declarative Sanity Checks

**Replace:** Hardcoded checks in InfrastructureHealthChecker

**With:** Declarative checks in DesiredState

```go
func (w *ProtocolConverterWorker) DeriveDesiredState(...) DesiredState {
    return DesiredState{
        Children: []ChildDeclaration{
            {ID: "redpanda", WorkerType: "redpanda"},
            {ID: "dfc_read", WorkerType: "dataflow"},
        },
        SanityChecks: []SanityCheck{
            {
                Name: "redpanda-benthos-output",
                Description: "Redpanda active but Benthos has no output connections",
                Check: func(sup *Supervisor) error {
                    redpanda := sup.GetChildState("redpanda")
                    benthos := sup.getChildObservedInternal("dfc_read")
                    
                    if redpanda == "active" && benthos.OutputConnections == 0 {
                        return fmt.Errorf("inconsistency detected")
                    }
                    return nil
                },
            },
        },
    }
}
```

**Benefits:**
- Supervisor iterates through declared checks ✅
- Workers declare what checks they need ✅
- No hardcoding in supervisor ✅
- Extensible to any parent-child combination ✅

---

### Simplification 3: DeriveDesiredState() Integration

**Add explicit phase to plan:**

```
### Phase 0: DeriveDesiredState() Integration (BEFORE Phase 1)

**Goal:** Establish declarative child lifecycle management

**Deliverables:**
- DeriveDesiredState() method signature
- ChildDeclaration structure
- Supervisor child creation/removal logic
- State mapping application
- Integration with existing Supervisor.Tick()

**Why First:** Foundation for all hierarchical composition
```

**Without this, the entire plan is built on shaky ground!**

---

### Simplification 4: Worker Example End-to-End

**Add to plan:**

```
### Example: ProtocolConverter Worker (Complete Implementation)

#### Step 1: Worker Definition

```go
type ProtocolConverterWorker struct {
    // No supervisor interaction fields needed
}

func (w *ProtocolConverterWorker) GetInitialState() State {
    return &InitializingState{}
}
```

#### Step 2: DeriveDesiredState()

```go
func (w *ProtocolConverterWorker) DeriveDesiredState(
    ctx context.Context,
    userSpec UserSpec,
    observed ObservedState,
) DesiredState {
    desired := DesiredState{}
    
    // Always declare connection child
    desired.Children = append(desired.Children, ChildDeclaration{
        ID:         "connection-1",
        WorkerType: "connection",
        StateMapping: map[string]string{
            "active": "up",
            "idle":   "down",
        },
    })
    
    // Conditionally declare bridge child
    if userSpec.BridgeConfig != nil {
        desired.Children = append(desired.Children, ChildDeclaration{
            ID:         "bridge-1",
            WorkerType: "dataflow",
            StateMapping: map[string]string{
                "active": "running",
                "idle":   "stopped",
            },
        })
    }
    
    // Declare sanity checks
    desired.SanityChecks = []SanityCheck{
        {
            Name: "redpanda-benthos-consistency",
            Check: func(sup *Supervisor) error {
                // Supervisor calls this automatically
                redpanda := sup.GetChildState("redpanda")
                benthos := sup.getChildObservedInternal("dfc_read")
                
                if redpanda == "active" && benthos.OutputConnections == 0 {
                    return fmt.Errorf("redpanda active but benthos has no output")
                }
                return nil
            },
        },
    }
    
    return desired
}
```

#### Step 3: State Machine (Unchanged from FSMv1)

```go
func (s *ActiveState) Next(snapshot Snapshot) (State, Signal, Action) {
    // Simple child state query - NO ERRORS, NO HEALTH CHECKS
    connectionState := snapshot.GetChildState("connection")
    
    if connectionState != "up" {
        return s, SignalNone, nil
    }
    
    return &IdleState{}, SignalNone, nil
}
```

#### What Worker Does NOT Do

❌ Call `CheckChildConsistency()` - Supervisor does this
❌ Call `restartChild()` - Supervisor does this
❌ Check `HasActionInProgress()` - Supervisor does this
❌ Access child observed state in business logic - Only in sanity checks (passed to supervisor)
❌ Handle infrastructure errors - Supervisor abstracts these away

**Total Boilerplate:** DeriveDesiredState() method + optional sanity checks
**Complexity vs FSMv1:** One additional method, but declarative (just data structures)
```

---

## What to Keep vs Revise

### ✅ Keep From Plan

1. **Circuit Breaker Pattern (Phase 1)**
   - Lines 103-231: ExponentialBackoff implementation
   - Lines 344-419: InfrastructureHealthChecker structure
   - Lines 682-689: Circuit breaker integration in Supervisor.Tick()
   - **Why:** Solid infrastructure pattern, well-designed

2. **Child Restart Logic**
   - Lines 169-188: restartChild() implementation
   - Lines 606-655: waitForChildState() helper
   - **Why:** Matches design principles

3. **Sanity Check Concept**
   - Lines 536-686: CheckChildConsistency() method
   - **Why:** Addresses FSMv1 pattern, moves to supervisor

4. **TDD Task Breakdown (Phase 1)**
   - Lines 138-336: Detailed RED-GREEN-REFACTOR steps
   - **Why:** Excellent development discipline

---

### ⚠️ Revise in Plan

1. **Add Phase 0: DeriveDesiredState() Integration**
   - **Current:** Missing completely
   - **Needed:** Foundation for declarative composition
   - **Priority:** CRITICAL - must come before Phase 1

2. **Add Worker API Definition**
   - **Current:** Only supervisor code shown
   - **Needed:** Snapshot.GetChildState() signature, Worker interface
   - **Priority:** HIGH - needed to verify simplicity

3. **Make Sanity Checks Declarative**
   - **Current:** Hardcoded in InfrastructureHealthChecker
   - **Needed:** Declared in DeriveDesiredState()
   - **Priority:** MEDIUM - improves extensibility

4. **Complete Phase 2 Task Breakdown**
   - **Current:** Just headers, no tasks (lines 694-705)
   - **Needed:** Detailed TDD tasks like Phase 1
   - **Priority:** HIGH - underspecified complexity is risky

5. **Add Complete Worker Example**
   - **Current:** Only supervisor code examples
   - **Needed:** End-to-end worker showing all methods
   - **Priority:** HIGH - proves workers stay simple

---

### ❌ Remove From Plan

1. **Hardcoded Child Names**
   - Lines 68, 586, 604: "redpanda", "dfc_read" hardcoded
   - **Replace:** Dynamic child iteration from declarations
   - **Why:** Violates declarative composition principle

2. **Static Sanity Checks**
   - Lines 542-635: Hardcoded Redpanda vs Benthos check
   - **Replace:** Iterate through declared SanityCheck[]
   - **Why:** Not extensible to different parent-child combinations

---

## Action Items

### Immediate (Before Implementing Plan)

1. ❌ **BLOCKER:** Add Phase 0 - DeriveDesiredState() Integration
   - Define Worker.DeriveDesiredState() signature
   - Define ChildDeclaration structure
   - Define SanityCheck structure
   - Implement supervisor child creation/removal
   - Implement state mapping application

2. ❌ **BLOCKER:** Define Worker API Contract
   - Snapshot.GetChildState() signature (no errors)
   - What methods workers MUST implement
   - What methods workers MAY implement
   - What methods workers NEVER access

3. ⚠️ **HIGH:** Complete Worker Example
   - Show DeriveDesiredState() implementation
   - Show state machine using GetChildState()
   - Show sanity check declaration
   - Explicitly list what workers DON'T do

4. ⚠️ **HIGH:** Complete Phase 2 Task Breakdown
   - Match detail level of Phase 1
   - Show supervisor integration points
   - Clarify worker vs supervisor responsibilities

### During Implementation

5. ⚠️ **MEDIUM:** Make Sanity Checks Declarative
   - Move from hardcoded to declared in DesiredState
   - Supervisor iterates through checks
   - Workers declare what checks they need

6. ⚠️ **MEDIUM:** Dynamic Child Iteration
   - Replace hardcoded names with iteration over declarations
   - CheckChildConsistency() uses declared children
   - restartChild() looks up from declarations

### After Implementation

7. ✅ **NICE-TO-HAVE:** Add Metrics
   - Phase 4 is good as-is
   - But wait until Phases 0-3 complete

---

## Conclusion

### Critical Gaps

The plan has **three critical gaps** that prevent implementation:

1. ❌ **No DeriveDesiredState() integration** - Can't add/remove children dynamically
2. ❌ **No worker API definition** - Can't verify workers stay simple
3. ⚠️ **No Phase 2 detail** - Async actions underspecified

### Recommended Next Steps

**DO NOT implement this plan as-is.** Instead:

1. **Add Phase 0 (DeriveDesiredState())**
   - This is the foundation everything else builds on
   - Without it, plan is incomplete

2. **Define worker-facing API**
   - Show what workers see in Snapshot
   - Show what methods workers implement
   - Prove workers stay simple

3. **Complete Phase 2 breakdown**
   - Match detail level of Phase 1
   - Show integration points clearly

4. **Add complete worker example**
   - End-to-end implementation
   - Explicitly show what workers DON'T do

### Overall Assessment

**Plan Quality:** 6/10
- Infrastructure patterns: Strong ✅
- TDD methodology: Excellent ✅
- Worker simplicity: Unverified ⚠️
- Declarative composition: Missing ❌
- Completeness: Critical gaps ❌

**Recommendation:** **Revise plan before implementing**

Address the three blockers (Phase 0, Worker API, Phase 2 detail), then proceed with revised plan.
