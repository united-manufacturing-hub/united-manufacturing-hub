# FSMv2 Architecture Recommendations Investigation Plan

> **For Claude:** This plan breaks down investigation tasks for FSM_V2_ARCHITECTURE_IMPROVEMENTS.md recommendations. Each task should be assigned to a dedicated sub-agent with access to the codebase and relevant design documents.

**Goal:** Systematically evaluate all architecture recommendations from FSM_V2_ARCHITECTURE_IMPROVEMENTS.md to determine validity, current state, and necessary actions.

**Context:** Another AI system analyzed our fsmv2 architecture and generated recommendations. Some may be based on misunderstandings of our design. We need systematic investigation to determine what's valid and actionable.

**Architecture Context:** FSMv2 is not in production. Breaking changes are acceptable if they improve package structure and API design. Current restructuring plan exists at `fsmv2-package-restructuring.md`.

**Created:** 2025-11-08 15:30 UTC
**Last Updated:** 2025-11-08 15:30 UTC

---

## Investigation Approach

Each recommendation will be investigated by a sub-agent with:
- Full codebase access (umh-core/pkg/fsmv2)
- Current restructuring plan (fsmv2-package-restructuring.md)
- FSM_V2_ARCHITECTURE_IMPROVEMENTS.md source document
- Related analysis documents in same directory

Sub-agents will determine:
1. Is the recommendation valid or based on misunderstanding?
2. Is it already addressed in our restructuring plan?
3. What's the actual current state in the codebase?
4. If valid and not addressed, what changes are needed?
5. Priority level (critical/high/medium/low/not-needed)

---

## Recommendations Summary

The source document identifies 21 distinct architectural issues across 6 categories:

**Category 1: API Design (4 issues)**
- Snapshot immutability via pass-by-value is implicit
- interface{} for Observed/Desired loses type safety
- State.Next() return signature allows invalid combinations
- CollectObservedState vs DeriveDesiredState naming

**Category 2: Naming and Conventions (3 issues)**
- "TryingTo" vs descriptive nouns state naming
- ObservedState vs DesiredState naming confusion
- Worker vs Supervisor terminology

**Category 3: Patterns and Abstractions (4 issues)**
- Dependencies embedding vs explicit parameters
- BaseWorker vs manual implementation
- Action idempotency testing burden
- State machine validation opportunities

**Category 4: Type System (2 issues)**
- VariableBundle three-tier namespace complexity
- types.UserSpec as configuration abstraction

**Category 5: Hierarchies and Composition (3 issues)**
- ChildrenSpecs reconciliation complexity
- StateMapping for parent-child communication
- Multi-level hierarchy limits

**Category 6: Testing and Validation (3 issues)**
- Idempotency testing helpers discoverability
- State machine validation gaps (duplicate of 3.4)
- Integration test patterns

**Category 7: Roadmap Items (2 sections)**
- Prioritized roadmap with 4 phases
- Go idioms comparison analysis

---

## Sub-Agent Investigation Tasks

### Task 1: Snapshot Immutability Pattern

**Recommendation (Issue 1.1):**
Snapshot immutability via pass-by-value is implicit. Recommendation: Add compile-time markers (struct tags + custom linter).

**Investigation Questions:**
- Is pass-by-value for immutability actually working correctly?
- Are there bugs caused by developers mutating snapshots?
- Does our restructuring plan address this?
- Would a linter actually help, or is documentation sufficient?
- What's the cost/benefit of adding struct tags?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go` lines 82-114 (Snapshot struct)
- Design docs: fsmv2-package-restructuring.md
- Tests: Search for snapshot mutation tests
- Related code: All State.Next() implementations

**Expected Output:**
```markdown
**Validity Assessment:** [Valid/Invalid/Partially Valid/Misunderstanding]

**Current State:**
- Snapshot defined at: [file:line]
- Current documentation approach: [describe]
- Known mutation bugs: [list or "none found"]
- Pass-by-value correctness: [verified/issues found]

**Restructuring Plan Status:**
- Addressed: [yes/no/partially]
- Plan location: [section reference if applicable]

**Recommendation:**
[Accept/Reject/Modify] with rationale

**If Accept/Modify:**
- Changes needed: [specific actions]
- Priority: [critical/high/medium/low]
- Effort estimate: [hours/days]
- Breaking change: [yes/no]

**Implementation Notes:**
[Technical details for implementation if accepted]
```

**Assigned Agent:** TBD

---

### Task 2: Type Safety for Observed/Desired States

**Recommendation (Issue 1.2):**
interface{} for Snapshot.Observed/Desired loses type safety. Recommendation: Add helper methods short-term, consider generics for FSMv3.

**Investigation Questions:**
- How many type assertion panics have occurred in practice?
- Would helper methods actually prevent these panics?
- Is generics approach feasible given our Go version?
- Does restructuring plan address this?
- What's the migration cost for helper methods?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go` (Snapshot struct), all State implementations
- Search pattern: `snapshot.Observed.(\*` and `snapshot.Desired.(\*`
- Tests: Look for type assertion tests/failures
- Go version: Check go.mod for generics availability

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 3: State.Next() Return Signature Validation

**Recommendation (Issue 1.3):**
State.Next() returns (State, Signal, Action) but documentation says "should not switch state and emit action at same time". Recommendation: Add test helper now, consider type-level split for FSMv3.

**Investigation Questions:**
- Does this invalid combination actually occur in our code?
- Is there supervisor validation that catches this?
- Would test helper catch bugs that slip through?
- Does the "should not" rule make architectural sense?
- Are there valid cases for state+action combination?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go` lines 210-241 (State interface)
- Search pattern: All `Next()` implementations
- Supervisor code: Check for validation logic
- Tests: Look for invalid combination tests

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 4: CollectObservedState vs DeriveDesiredState Naming

**Recommendation (Issue 1.4):**
Method names don't clearly convey "async vs sync" distinction. Recommendation: Keep current with added intent comments.

**Investigation Questions:**
- Is the async/sync distinction actually confusing developers?
- Are there bugs caused by misunderstanding these methods?
- Would renaming (MonitorActualState/ComputeDesiredState) help?
- Does documentation already clarify this?
- What's the actual usage pattern?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go` (Worker interface)
- All Worker implementations
- Documentation: Check for async/sync explanation
- Git history: Look for confusion-related bug fixes

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 5: State Naming Conventions (TryingTo vs Descriptive)

**Recommendation (Issue 2.1):**
Inconsistency between "TryingTo" prefix (active states) and descriptive nouns (passive states). SyncingState breaks pattern (active but uses passive naming). Recommendation: Use gerund verbs consistently for FSMv3.

**Investigation Questions:**
- Is SyncingState actually inconsistent or correctly named?
- Do developers find the "TryingTo" pattern helpful?
- Would "AuthenticatingState" be clearer than "TryingToAuthenticateState"?
- Are there other state naming inconsistencies?
- Does current naming affect code comprehension?

**Investigation Scope:**
- Files: Search for all `*State` struct definitions
- Particularly: `pkg/fsmv2/workers/communicator/` state files
- Pattern analysis: Count TryingTo vs gerund vs noun states
- Documentation: Check naming convention docs

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 6: ObservedState vs DesiredState Terminology

**Recommendation (Issue 2.2):**
ObservedState.GetObservedDesiredState() is confusing tongue-twister. Recommendation: Rename to "DeployedState" for FSMv3.

**Investigation Questions:**
- Is "ObservedDesiredState" actually confusing in practice?
- Does "what's actually deployed" vs "what we want to deploy" make sense?
- Would "DeployedState" be clearer?
- Is this method used frequently enough to matter?
- Are there better alternatives (ActualState, CurrentState)?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go` lines 59-71 (ObservedState interface)
- Usage: `grep -r "GetObservedDesiredState" pkg/`
- Documentation: Check explanation at lines 64-66

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 7: Worker vs Supervisor Naming

**Recommendation (Issue 2.3):**
Worker/Supervisor borrowed from Erlang/OTP but may confuse non-Erlang developers. Recommendation: Keep current with clarifying documentation.

**Investigation Questions:**
- Do new team members struggle with Worker/Supervisor concepts?
- Would Controller/Reconciler (Kubernetes-style) be clearer?
- Is Erlang/OTP reference helpful or harmful?
- Does existing documentation explain the relationship?
- What do similar Go projects use?

**Investigation Scope:**
- Files: `pkg/fsmv2/worker.go`, `pkg/fsmv2/supervisor/`
- Documentation: Check for Worker/Supervisor explanation
- Compare: Kubernetes controller pattern, Go concurrency patterns
- Team feedback: Check git history for confusion

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 8: Dependencies Interface Pattern

**Recommendation (Issue 3.1):**
Dependencies interface hides actual requirements. Recommendation: Add subset interfaces for better testing.

**Investigation Questions:**
- Does Dependencies bag actually hinder testing?
- Are there cases where we mock more than needed?
- Would subset interfaces (AuthActionDeps, etc.) help?
- What's the migration cost?
- Does explicit parameters make sense for our scale?

**Investigation Scope:**
- Files: Search for `Dependencies` interface definitions
- Actions: `pkg/fsmv2/workers/communicator/action/`
- Tests: Check mocking patterns in *_test.go files
- Count: How many different dependency combinations exist?

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 9: BaseWorker Helper Pattern

**Recommendation (Issue 3.2):**
No BaseWorker exists, each worker implements full interface manually. Recommendation: Add helper constructors for common patterns.

**Investigation Questions:**
- Is there significant boilerplate across workers?
- Would helper constructors actually reduce duplication?
- Does Go idiom "a little copying is better than a little dependency" apply?
- What are the common patterns to extract?
- Would BaseWorker embedding be un-idiomatic?

**Investigation Scope:**
- Files: All Worker implementations
- Pattern analysis: Compare implementation similarities
- Go best practices: Check for stdlib patterns
- Count: How many simple vs complex workers?

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 10: Action Idempotency Testing

**Recommendation (Issue 3.3):**
Idempotency is critical (Invariant I10) but relies on developer discipline. Recommendation: Add linter check + declarative framework.

**Investigation Questions:**
- Are there non-idempotent actions in the codebase?
- Does VerifyActionIdempotency helper exist and work?
- Would linter actually catch missing tests?
- Is declarative CheckThenActAction pattern useful?
- What's the current test coverage for idempotency?

**Investigation Scope:**
- Files: `pkg/fsmv2/supervisor/execution/action_idempotency_test.go`
- All Action implementations: Search for `Execute(` methods
- Tests: Count idempotency tests vs total actions
- CI: Check for test coverage enforcement

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 11: State Machine Validation Tools

**Recommendation (Issue 3.4):**
No validation of state machine structure (unreachable states, missing shutdown paths). Recommendation: Add test helper + diagram generation.

**Investigation Questions:**
- Are there actually unreachable states in our FSMs?
- Would validation helper catch real bugs?
- Is diagram generation valuable for debugging?
- What's the implementation complexity?
- Do similar tools exist in Go ecosystem?

**Investigation Scope:**
- Files: All `*State` implementations, check for unreachable states
- State transitions: Build graph from Next() methods
- Tools: Search Go ecosystem for FSM validation tools
- Bugs: Check git history for state machine bugs

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 12: VariableBundle Three-Tier Namespace

**Recommendation (Issue 4.1):**
User/Global/Internal distinction with different flattening rules is confusing. Recommendation: Document now, simplify to two-tier (User/Global) long-term.

**Investigation Questions:**
- Is three-tier actually confusing in practice?
- Are Internal variables actually needed as separate namespace?
- Would two-tier (User/Global) work for all cases?
- Are there flattening bugs or documentation gaps?
- What do templates actually use?

**Investigation Scope:**
- Files: Search for `VariableBundle` definition
- Usage: Template files that reference variables
- Flattening: Code that implements Flatten()
- Documentation: Check variable namespace explanation

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 13: UserSpec Configuration Abstraction

**Recommendation (Issue 4.2):**
UserSpec with opaque string Config lacks type safety. Recommendation: Add ValidatableConfig interface for gradual improvement.

**Investigation Questions:**
- Are there config validation bugs in practice?
- Would ValidatableConfig actually help?
- Is generics approach (Worker[C any]) better long-term?
- What's the migration path?
- Does current pattern support all use cases?

**Investigation Scope:**
- Files: Search for `UserSpec` definition
- Usage: How DeriveDesiredState uses spec parameter
- Validation: Current config validation code
- Errors: Config-related bugs in git history

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 14: ChildrenSpecs Reconciliation

**Recommendation (Issue 5.1):**
Kubernetes-style declarative reconciliation is powerful but not obvious. Recommendation: Document algorithm + add optional hooks.

**Investigation Questions:**
- Is reconciliation algorithm actually confusing?
- Are there bugs caused by misunderstanding reconciliation?
- Would explicit lifecycle methods be better?
- Do we need reconciliation hooks?
- Does Kubernetes comparison help or confuse?

**Investigation Scope:**
- Files: Supervisor reconciliation code for children
- Documentation: Check ChildrenSpecs explanation
- Bugs: Child management bugs in git history
- Tests: Reconciliation test coverage

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 15: StateMapping for Parent-Child Communication

**Recommendation (Issue 5.2):**
Parent-child state communication not yet implemented. Recommendation: Add StateMapping in ChildSpec + parent reference for complex cases.

**Investigation Questions:**
- Do we actually need parent-child communication?
- What are the use cases?
- Would StateMapping pattern work?
- Is parent reference approach better?
- What's the implementation complexity?

**Investigation Scope:**
- Files: ChildSpec definition
- Use cases: Where parent-child communication is needed
- Patterns: How other systems handle this
- Current workarounds: How we handle this now

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 16: Multi-Level Hierarchy Limits

**Recommendation (Issue 5.3):**
No documentation of depth limits or guidelines for deep hierarchies. Recommendation: Document current behavior + add visualization tool.

**Investigation Questions:**
- Are deep hierarchies actually used?
- What's the practical depth limit?
- Would depth limit configuration help?
- Is visualization tool valuable?
- Are there hierarchy-related performance issues?

**Investigation Scope:**
- Files: Supervisor hierarchy handling code
- Usage: Actual hierarchy depths in tests/examples
- Performance: Resource usage for different depths
- Documentation: Current hierarchy guidance

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 17: Idempotency Testing Discoverability

**Recommendation (Issue 6.1):**
Test helper exists but no enforcement. Recommendation: Improve docs + add CI check + provide examples.

**Investigation Questions:**
- Is test helper actually discoverable?
- Would CI check catch missing tests?
- Do examples help developers?
- What's the current test coverage?
- Are there better discoverability approaches?

**Investigation Scope:**
- Files: `pkg/fsmv2/supervisor/execution/action_idempotency_test.go`
- Documentation: README/contributing docs about testing
- CI: Current test coverage checks
- Usage: How many actions have idempotency tests?

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 18: Integration Test Patterns

**Recommendation (Issue 6.3):**
No standard pattern for testing full Worker lifecycle. Recommendation: Document patterns + provide mock dependencies now, framework later.

**Investigation Questions:**
- Is lack of test framework actually a problem?
- What patterns do current tests use?
- Would WorkerTestHarness be valuable?
- Are mock dependencies sufficient?
- What's the most common testing pain point?

**Investigation Scope:**
- Files: All *_test.go files in pkg/fsmv2
- Patterns: How workers are currently tested
- Coverage: Integration vs unit test ratio
- Pain points: Complex test setups

**Expected Output:**
(Same format as Task 1)

**Assigned Agent:** TBD

---

### Task 19: Roadmap Phase Analysis

**Recommendation (Section: Prioritized Roadmap):**
Document proposes 4 phases with effort estimates and priorities. We need to validate these estimates against our restructuring plan.

**Investigation Questions:**
- Do phase priorities align with our goals?
- Are effort estimates realistic?
- Which items are already in restructuring plan?
- What's the dependency order?
- What should we tackle first?

**Investigation Scope:**
- Document: fsmv2-package-restructuring.md
- Roadmap: FSM_V2_ARCHITECTURE_IMPROVEMENTS.md phases 1-4
- Comparison: What's planned vs what's recommended
- Dependencies: Which items block others

**Expected Output:**
```markdown
**Phase Priority Assessment:**

**Phase 1 (Quick Wins):**
- Items already planned: [list]
- Items not planned: [list]
- Effort estimate accuracy: [realistic/optimistic/pessimistic]
- Priority alignment: [matches/conflicts]

**Phase 2 (Naming/Docs):**
[Same format]

**Phase 3 (Structural):**
[Same format]

**Phase 4 (Future):**
[Same format]

**Recommendation:**
[Suggested execution order with rationale]
```

**Assigned Agent:** TBD

---

### Task 20: Go Idioms Comparison Validation

**Recommendation (Section: Comparison with Go Idioms):**
Document claims FSMv2 aligns well with Go idioms but uses pre-generics patterns. We need to verify these claims.

**Investigation Questions:**
- Are the Go idiom comparisons accurate?
- Do we actually diverge where claimed?
- Are the stdlib pattern comparisons valid?
- Should we adopt generics in FSMv3?
- What are the real Go idiom violations?

**Investigation Scope:**
- Files: Core FSMv2 interfaces and patterns
- Go version: Check go.mod
- Stdlib comparison: Verify claims about io.Reader, http.Handler, etc.
- Community practices: Check Go project survey, style guides

**Expected Output:**
```markdown
**Idiom Alignment Assessment:**

**Accurate Comparisons:**
- [List claims that are correct]

**Inaccurate/Misleading Comparisons:**
- [List claims that need correction]
- [Why they're wrong]

**Actual Go Idiom Status:**
- Strengths: [What we do well]
- Divergences: [Where we differ and why]
- Concerns: [Legitimate issues vs acceptable tradeoffs]

**Recommendation:**
[Should we change anything based on idioms?]
```

**Assigned Agent:** TBD

---

### Task 21: Cross-Cutting Analysis - Restructuring Plan Integration

**Special Task:** This task synthesizes findings from all other tasks to update the restructuring plan.

**Investigation Questions:**
- Which recommendations duplicate existing plan items?
- Which recommendations conflict with plan?
- Which recommendations fill gaps in plan?
- What new items should be added to plan?
- What's the integrated priority order?

**Investigation Scope:**
- Input: All task outputs (1-20)
- Document: fsmv2-package-restructuring.md
- Analysis: Gap analysis and conflict resolution
- Output: Updated restructuring plan with recommendations integrated

**Expected Output:**
```markdown
**Recommendations vs Restructuring Plan:**

**Already Planned (duplicate):**
- [List recommendations already in plan]

**Conflicts with Plan:**
- [List recommendations that contradict plan]
- [Resolution for each conflict]

**Gaps Filled by Recommendations:**
- [List new valuable recommendations]
- [Integration approach for each]

**Rejected Recommendations:**
- [List with rationale]

**Updated Priority Matrix:**
[Integrated priority table with all work items]

**Next Steps:**
[Concrete action items with order]
```

**Assigned Agent:** TBD (Run after all other tasks complete)

---

## Execution Plan

### Phase 1: Parallel Investigation (All at once)

Run Tasks 1-18 in parallel (independent investigations):
- Each sub-agent has full context
- No inter-task dependencies
- Can execute simultaneously

### Phase 2: Roadmap Analysis (After Phase 1)

Run Tasks 19-20:
- Task 19: Validate phase priorities
- Task 20: Validate Go idioms claims

### Phase 3: Integration (After Phases 1-2)

Run Task 21:
- Synthesize all findings
- Update restructuring plan
- Create final recommendation

### Phase 4: Implementation Planning

Based on Task 21 output:
- Create Linear tickets for accepted items
- Update fsmv2-package-restructuring.md
- Archive completed investigation plan

---

## Priority Matrix (Initial, will be refined by investigation)

Document's proposed priorities, to be validated:

| Priority | Low Effort | Medium Effort | High Effort |
|----------|------------|---------------|-------------|
| Critical | - | 3.3 (idempotency linter) | - |
| High     | 1.3 (state+action test helper) | 3.4 (FSM validation), 6.1 (test docs/CI) | - |
| Medium   | 1.4 (method comments), 4.1 (variable docs), 5.1 (reconciliation docs) | 3.1 (subset interfaces), 3.2 (helper constructors), 5.3 (hierarchy docs) | - |
| Low      | 1.1 (immutability markers) | 1.2 (type helpers), 4.2 (validatable config) | - |
| FSMv3    | - | 2.1 (state naming), 2.2 (rename to DeployedState) | 1.2 (generics), 1.3 (type-level split), 4.1 (two-tier variables) |

---

## Open Questions

1. **Scope Clarification:** Should sub-agents propose solutions or just assess validity?
   - **Answer:** Assess validity + propose if recommendation is accepted

2. **Breaking Changes:** What's our tolerance for breaking changes in FSMv2?
   - **Answer:** High tolerance (not in production yet)

3. **FSMv3 Items:** Should we track FSMv3 recommendations separately?
   - **Answer:** Yes, mark clearly but don't investigate deeply now

4. **Conflicting Recommendations:** How to handle if tasks conflict?
   - **Answer:** Task 21 resolves conflicts with restructuring plan context

5. **Implementation Timing:** Should investigation include implementation estimates?
   - **Answer:** Yes, effort estimates are part of output format

---

## Success Criteria

Investigation complete when:
- [ ] All 21 tasks have outputs in required format
- [ ] Each recommendation has validity assessment
- [ ] Current state vs restructuring plan documented
- [ ] Priority levels assigned with rationale
- [ ] Task 21 integration complete
- [ ] Restructuring plan updated if needed
- [ ] Linear tickets created for accepted items

---

## Notes for Sub-Agents

**Context to provide:**
- FSM_V2_ARCHITECTURE_IMPROVEMENTS.md (source document)
- fsmv2-package-restructuring.md (current plan)
- Access to pkg/fsmv2/ codebase
- Git history access for bug patterns

**Critical mindset:**
- Question the original AI's assumptions
- Verify claims against actual code
- Consider if "problem" is actually design choice
- Distinguish confusion from actual issues
- Check if documentation would suffice vs code changes

**Output discipline:**
- Use exact file:line references
- Quote relevant code snippets
- Link to related issues/PRs if applicable
- Be specific about implementation if recommending acceptance
- Estimate effort realistically

---

## Changelog

### 2025-11-08 15:30 UTC - Plan created
Initial investigation plan created from FSM_V2_ARCHITECTURE_IMPROVEMENTS.md. Plan covers 21 distinct recommendations across 6 categories plus roadmap/idioms analysis. Designed for parallel sub-agent execution with final integration phase. Breaking changes acceptable as FSMv2 not in production.
