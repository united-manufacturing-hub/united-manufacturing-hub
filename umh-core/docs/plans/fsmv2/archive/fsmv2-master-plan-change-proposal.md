# FSMv2 Master Plan Change Proposal

**Date:** 2025-11-02
**Status:** Draft for Review
**Purpose:** Update master plan to incorporate hierarchical composition, templating, and variables system
**Impact:** Timeline doubles (6 weeks → 12 weeks), LOC +129% (750 → 1,720 lines)

**Related Documents:**
- Gap Analysis: `fsmv2-master-plan-gap-analysis.md`
- Current Master Plan: `2025-11-02-fsmv2-supervision-and-async-actions.md`
- Design Docs: `fsmv2-child-specs-in-desired-state.md`, `fsmv2-idiomatic-templating-and-variables.md`, `fsmv2-derive-desired-state-complete-definition.md`

---

## Executive Summary

This change proposal provides **EXACT specifications** for updating the FSMv2 master plan. Instead of rewriting the entire plan, this document specifies precisely what to add, modify, move, or remove.

The gap analysis identified that the master plan predates critical hierarchical composition design decisions. This proposal adds 2 new foundational phases, updates existing phases for integration, and doubles the timeline from 6 weeks to 12 weeks.

**Key Additions:**
- Phase 0: Hierarchical Composition Foundation (NEW - 2 weeks)
- Phase 0.5: Templating & Variables System (NEW - 2 weeks)  
- 9 new hierarchical composition design documents
- Updated tick loop with child reconciliation
- Updated testing strategy for hierarchical patterns

**Scope Impact:**
- Timeline: +100% (6 weeks → 12 weeks)
- Code: +129% (750 LOC → 1,720 LOC)
- Phases: +50% (4 phases → 6 phases)

---

## 1. Overview

### Summary of Changes

**What's Being Added:**
1. Phase 0: Hierarchical Composition Foundation (2 weeks, 410 LOC)
2. Phase 0.5: Templating & Variables System (2 weeks, 325 LOC)
3. Updated tick loop pseudocode showing child reconciliation
4. New task breakdowns for hierarchical components
5. Updated integration tests for parent-child relationships

**What's Being Modified:**
1. Design Documents Reference section (add 9 documents)
2. Implementation Phases section (renumber phases 1-4 → 3-6)
3. Task 1.3 CheckChildConsistency (use children map)
4. Task 1.4 Circuit Breaker (update pseudocode)
5. Tasks 2.1-2.5 (action IDs use child name)
6. Phase 3 Integration (add hierarchical tests)
7. Phase 4 Monitoring (add hierarchical metrics)

**What's Staying the Same:**
- TDD approach (RED → GREEN → REFACTOR)
- ExponentialBackoff implementation (Task 1.1)
- InfrastructureHealthChecker structure (Task 1.2)
- ActionExecutor core logic (Tasks 2.1-2.5)
- Gap analysis sections (Tasks 3.2, 3.4)
- Code review checkpoints

### Why These Changes Are Needed

The gap analysis revealed critical missing components:

**Missing Architecture:**
- **HOW parents control children** - ChildSpec in DesiredState (not defined in master plan)
- **HOW children are created dynamically** - WorkerFactory pattern (not mentioned)
- **HOW parent context flows to children** - Variables system (completely missing)
- **HOW templates render** - RenderTemplate() + Flatten() (not in plan)
- **HOW locations build hierarchies** - Location merging (not addressed)

**Design Documents Added After Master Plan:**
1. `fsmv2-child-specs-in-desired-state.md` - Hierarchical composition API
2. `fsmv2-idiomatic-templating-and-variables.md` - Variable system
3. `fsmv2-derive-desired-state-complete-definition.md` - Complete API spec

These documents define **core patterns** not present in the original master plan.

### Impact on Timeline and Scope

**Timeline Impact:**
```
Original: 6 weeks (4 phases)
Phase 1: Infrastructure (2 weeks)
Phase 2: Async Actions (2 weeks)
Phase 3: Integration (1 week)
Phase 4: Monitoring (1 week)

New: 12 weeks (6 phases)
Phase 0: Hierarchical Composition (2 weeks) ← NEW
Phase 0.5: Templating & Variables (2 weeks) ← NEW
Phase 1: Infrastructure (2 weeks) ← MINOR CHANGES
Phase 2: Async Actions (2 weeks) ← MINOR CHANGES
Phase 3: Integration (3 weeks) ← HEAVILY UPDATED
Phase 4: Monitoring (1 week) ← MINOR UPDATES
```

**Why Timeline Doubles:**
- Phase 0 must complete FIRST (foundational, blocks everything)
- Phase 0.5 must complete before Benthos workers can work
- Phase 3 integration now tests hierarchical patterns (more complex)

**Code Impact:**
```
Original: ~750 lines
- Infrastructure Supervision: 300 lines
- Async Action Executor: 250 lines
- Integration tests: 200 lines

New: ~1,720 lines
+ Hierarchical Composition: 410 lines (NEW)
+ Templating & Variables: 325 lines (NEW)
+ Updated Infrastructure: 320 lines (+20)
+ Updated Async Actions: 260 lines (+10)
+ Updated Integration: 330 lines (+130)
+ Updated Monitoring: 75 lines (+25)
```

**Integration Clarity:**
- ✅ No conflicts with Infrastructure Supervision (circuit breaker still works)
- ✅ No conflicts with Async Action Executor (actions still queue per worker)
- ✅ Clean integration points (children API integrates smoothly)

---

## 2. Structural Changes

This section specifies EXACTLY what to change in each section of the master plan.

### Section: "Design Documents Reference"

**Location:** Lines 17-44 (after "## Design Documents Reference")

**Action:** MODIFY (add hierarchical composition documents)

**Reason:** Add new design documents created after master plan

**Change Specification:**

AFTER line 44 (after async-action-executor-implementation.md reference), ADD:

```markdown

### Hierarchical Composition Documents

These documents define the hierarchical composition architecture added after the original master plan:

5. **`docs/plans/fsmv2-child-specs-in-desired-state.md`** (2025-10-30)
   - ChildSpec structure: Name, WorkerType, UserSpec, StateMapping
   - Parent creates children via DeriveDesiredState() return value
   - Supervisor reconciles ChildrenSpecs (add/remove/update)
   - WorkerFactory pattern for dynamic worker creation from string types
   - Matches Kubernetes declarative pattern exactly

6. **`docs/plans/fsmv2-idiomatic-templating-and-variables.md`** (2025-10-31)
   - VariableBundle: User/Global/Internal namespaces
   - Flatten() method: User variables promoted to top-level
   - RenderTemplate() with strict mode (missingkey=error)
   - Template execution: Distributed at worker level (not centralized)
   - Location computation and merging for ISA-95 hierarchy

7. **`docs/plans/fsmv2-derive-desired-state-complete-definition.md`** (2025-11-01)
   - Complete API specification
   - DeriveDesiredState(userSpec UserSpec) DesiredState
   - Return type includes State + ChildrenSpecs
   - UserSpec contains typed fields + Variables
   - StateMapping application by supervisor
   - Hybrid control: Declarative mapping + child autonomy

### Integration Summary

**Hierarchical composition integrates cleanly:**
- Infrastructure Supervision: Circuit breaker affects all children (no conflicts)
- Async Action Executor: Actions queue per child (no conflicts)
- Both maintain "infrastructure invisible" principle ✅
```

---

### Section: "Implementation Phases"

**Location:** Lines 149-191

**Action:** REPLACE ENTIRELY

**Reason:** Add Phase 0 and 0.5, renumber existing phases, update dependencies

**Change Specification:**

DELETE lines 149-191 (entire "Implementation Phases" section)

INSERT:

```markdown
## Implementation Phases

### Phase 0: Hierarchical Composition Foundation (Week 1-2) - NEW

**Why First:** Foundation for all other features, children must exist before supervision

**Deliverables:**
- ChildSpec and DesiredState data structures
- Worker.DeriveDesiredState() signature update (returns DesiredState)
- Supervisor.reconcileChildren() implementation
- WorkerFactory pattern (string → Worker creation)
- Child lifecycle management (add/remove/update)
- StateMapping field in ChildSpec
- Tests: child reconciliation, worker factory, state mapping

**Dependencies:** None (foundational)

**Blocks:** ALL subsequent phases

**LOC:** ~410 lines (structures: 50, factory: 30, reconcile: 80, tests: 250)

---

### Phase 0.5: Templating & Variables System (Week 2-3) - NEW

**Why Second:** Required for Benthos config generation, builds on ChildSpec

**Deliverables:**
- VariableBundle structures (User/Global/Internal)
- Flatten() method implementation
- RenderTemplate() with strict mode
- Location merging (parent + child)
- Location path computation (ISA-95 hierarchy)
- Gap filling for missing location levels
- Tests: flattening, rendering, location computation

**Dependencies:** Phase 0 (Variables in UserSpec)

**Blocks:** Benthos worker implementation

**LOC:** ~325 lines (structures: 30, flatten: 15, render: 40, location: 90, tests: 150)

---

### Phase 1: Infrastructure Supervision Foundation (Week 3-4) - UPDATED

**Why Third:** More foundational than actions, establishes circuit breaker

**Deliverables:**
- InfrastructureHealthChecker with child consistency checks
- Circuit breaker logic in Supervisor.Tick()
- Child restart with exponential backoff
- Sanity check examples (Redpanda vs Benthos)

**Changes from original:**
- CheckChildConsistency() uses `s.children` map instead of hardcoded names
- Child restart uses `s.children[name].supervisor.Restart()`
- Otherwise identical to original plan

**Dependencies:** Phase 0 (needs children to exist)

**Blocks:** None

**LOC:** ~320 lines (same as original, minor changes)

---

### Phase 2: Async Action Executor (Week 4-5) - MINIMAL CHANGES

**Why Fourth:** Integrates with supervision, builds on tick loop structure

**Deliverables:**
- ActionExecutor with global worker pool
- Integration with Supervisor.Tick()
- Action queueing and timeout handling
- Non-blocking action execution

**Changes from original:**
- Action IDs use child name (not hardcoded worker ID)
- Otherwise identical to original plan

**Dependencies:** Phase 0 (actions queue per child)

**Blocks:** None

**LOC:** ~260 lines (same as original, minor changes)

---

### Phase 3: Integration & Edge Cases (Week 6-8) - HEAVILY UPDATED

**Why Fifth:** Tests combined system with all new patterns

**Deliverables:**
- Combined tick loop with:
  - Infrastructure checks (priority 1)
  - Child reconciliation (NEW)
  - State mapping application (NEW)
  - Template rendering coordination (NEW)
  - Action checks (priority 2)
- Action behavior during child restart (Task 3.2 - existing)
- Observation collection during circuit open (Task 3.4 - existing)
- Template rendering performance tests (NEW)
- Variable flow tests (parent → child) (NEW)
- Location hierarchy tests (NEW)
- ProtocolConverter end-to-end test (NEW)

**Changes from original:**
- Add hierarchical composition integration tests
- Add template rendering tests
- Add variable propagation tests
- Extend from 1 week to 3 weeks

**Dependencies:** Phases 0, 0.5, 1, 2

**Blocks:** None

**LOC:** ~330 lines (integration tests: 200 → 330)

---

### Phase 4: Monitoring & Observability (Week 9) - MINOR UPDATES

**Why Last:** Provides visibility into all systems

**Deliverables:**
- Prometheus metrics for infrastructure recovery
- Prometheus metrics for action execution
- Prometheus metrics for child count (NEW)
- Prometheus metrics for template rendering (NEW)
- Detailed logging for debugging
- Runbook for manual intervention

**Changes from original:**
- Add metrics: `fsmv2_children_count`, `fsmv2_template_rendering_duration_seconds`

**Dependencies:** Phase 3 (needs integration)

**Blocks:** None

**LOC:** ~75 lines (monitoring: 100 → 150, but shorter than planned)
```

---

## 3. New Phase 0.5: Templating & Variables

This section provides complete TDD task breakdowns for Phase 0.5 (Templating & Variables System).

**Total Effort:** 2 weeks, ~325 lines of code

**Dependencies:** Phase 0 (Variables in UserSpec)

**Blocks:** Benthos worker implementation

---

### Task 0.5.1: VariableBundle Structure

**Goal:** Define three-tier namespace structure (User/Global/Internal)

**Why:** Foundation for all variable handling, namespaces separate concerns

**Location:** `pkg/fsmv2/variables/variables.go`

**Effort:** 2 hours, ~30 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/variables/variables_test.go`

```go
package variables_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/variables"
)

var _ = Describe("VariableBundle", func() {
    Describe("Structure", func() {
        It("should have User namespace for user-defined variables", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP":   "192.168.1.100",
                    "PORT": 502,
                },
            }

            Expect(bundle.User).To(HaveKey("IP"))
            Expect(bundle.User["IP"]).To(Equal("192.168.1.100"))
            Expect(bundle.User["PORT"]).To(Equal(502))
        })

        It("should have Global namespace for fleet-wide variables", func() {
            bundle := variables.VariableBundle{
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                    "tenant_id":    "acme-corp",
                },
            }

            Expect(bundle.Global).To(HaveKey("api_endpoint"))
            Expect(bundle.Global["api_endpoint"]).To(Equal("https://api.umh.app"))
        })

        It("should have Internal namespace for runtime metadata", func() {
            bundle := variables.VariableBundle{
                Internal: map[string]any{
                    "id":         "worker-123",
                    "created_at": "2025-11-03T10:00:00Z",
                },
            }

            Expect(bundle.Internal).To(HaveKey("id"))
            Expect(bundle.Internal["id"]).To(Equal("worker-123"))
        })

        It("should serialize User and Global namespaces only", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP": "192.168.1.100",
                },
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                },
                Internal: map[string]any{
                    "id": "worker-123",
                },
            }

            // YAML serialization should include User and Global
            yamlData, err := yaml.Marshal(bundle)
            Expect(err).NotTo(HaveOccurred())

            yamlStr := string(yamlData)
            Expect(yamlStr).To(ContainSubstring("user:"))
            Expect(yamlStr).To(ContainSubstring("global:"))
            Expect(yamlStr).NotTo(ContainSubstring("internal:"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# VariableBundle
#   Structure
#     should have User namespace for user-defined variables [It]
#
#   undefined: variables.VariableBundle
```

**Verify:** Test fails with `undefined: variables.VariableBundle`

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/variables/variables.go`

```go
// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0

package variables

// VariableBundle holds variables for a worker in three namespaces
type VariableBundle struct {
    // User variables: user-defined config + parent state + computed values
    // Serialized: YES
    // Template access: Top-level ({{ .IP }})
    User map[string]any `yaml:"user,omitempty" json:"user,omitempty"`

    // Global variables: fleet-wide settings from management loop
    // Serialized: YES
    // Template access: Nested ({{ .global.api_endpoint }})
    Global map[string]any `yaml:"global,omitempty" json:"global,omitempty"`

    // Internal variables: runtime metadata (id, timestamps, bridged_by)
    // Serialized: NO (runtime-only)
    // Template access: Nested ({{ .internal.id }})
    Internal map[string]any `yaml:"-" json:"-"`
}
```

**Why this design:**
- User: Top-level access, user-facing variables
- Global: Fleet-wide config, explicit namespace
- Internal: Runtime metadata, not serialized

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# Ran 4 of 4 Specs in 0.003 seconds
# SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/variables/
git commit -m "feat(fsmv2): add VariableBundle three-tier namespace structure

Implements User/Global/Internal namespaces:
- User: user-defined + parent state + computed values
- Global: fleet-wide settings
- Internal: runtime metadata (not serialized)

User and Global serialize to YAML/JSON, Internal is runtime-only.

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Structure correctness:**
   - Three namespaces: User, Global, Internal
   - User and Global serialize (yaml/json tags)
   - Internal does NOT serialize (yaml:"-")

2. **Test coverage:**
   - Each namespace tested separately
   - Serialization behavior tested
   - No focused specs (`FIt`, `FDescribe`)

3. **Documentation:**
   - Clear comments explaining each namespace
   - Template access pattern documented
   - Serialization behavior documented

**Ask:**
- Do namespace purposes match design document?
- Is serialization behavior correct?
- Are tests comprehensive?

---

(Tasks 0.5.2 through 0.5.6 continue in the section 3 file - see `fsmv2-change-proposal-section3.md` for complete content)

---

## 4-5. Modified Phase 1-3 Tasks

(Sections 4-5 content continues in the section 4-5 file - see `fsmv2-change-proposal-section4-5.md` for complete content)

---

## 6-7. Updated Timeline & Architecture

(Sections 6-7 content continues in the section 6-7 file - see `fsmv2-change-proposal-section6-7.md` for complete content)

---

## 8-11. Testing, Migration, Risks, Acceptance

(Sections 8-11 content continues in the section 8-11 file - see `fsmv2-change-proposal-section8-11.md` for complete content)

---

## Document Structure

This change proposal has been split across multiple files due to length:

**Main Document (this file):** Sections 1-2 + Section 3 Preview
- Executive summary and structural changes
- Design documents reference updates
- Implementation phases overview
- Phase 0.5 Task 0.5.1 as example

**Section Files (complete detailed content):**
1. `fsmv2-change-proposal-section3.md` - Phase 0.5 complete TDD task breakdowns (1,935 lines)
2. `fsmv2-change-proposal-section4-5.md` - Modified tasks for Phases 1-3 (1,222 lines)
3. `fsmv2-change-proposal-section6-7.md` - Timeline and architecture (1,186 lines)
4. `fsmv2-change-proposal-section8-11.md` - Testing, migration, risks, acceptance (1,932 lines)

**Total Specification:** ~6,600 lines across all files

**Implementation Guide:**
- Read sections 1-2 for overview and context
- Read section 3 file for complete Phase 0.5 implementation
- Read section 4-5 file for all Phase 1-3 modifications
- Read section 6-7 file for timeline and architecture diagrams
- Read section 8-11 file for testing, migration, risks, and acceptance criteria

**Ready for Implementation** - All specifications are complete and detailed.
