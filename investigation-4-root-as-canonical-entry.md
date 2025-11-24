# Investigation 4: Root Worker as Canonical Entry Point for FSM v2

**Date**: 2025-11-20
**Investigator**: Claude Code
**Working Directory**: /Users/jeremytheocharis/umh-git/umh-core-eng-3806

---

## Executive Summary

**Current State**: The root worker exists in production code (`pkg/fsmv2/root/`) but is **NOT positioned as the canonical entry point** in documentation or architectural guidance.

**User's Hypothesis**: "Every application should start with the root worker - this is the recommended way. The root worker is not an example, it's THE way to use the library."

**Investigation Verdict**: **PARTIALLY SUPPORTED**

- **Architecture**: Root worker is designed as production code, not an example
- **Documentation**: Does NOT position root as mandatory or recommended starting point
- **Tests**: Mixed - some use root, others create supervisors directly
- **Production Code**: No evidence of root supervisor being used yet
- **Gap**: Documentation presents FSM v2 as "create workers and supervisors directly" rather than "start with root, register types, add UserSpecs"

---

## 1. Current Documentation Positioning

### 1.1 Getting Started Guide (`docs/getting-started.md`)

**How it describes "starting"**:

```markdown
## 5-Minute Example: Create a Simple Worker

### Step 1: Define State Types
### Step 2: Implement States
### Step 3: Implement Worker Interface
```

**Verdict**: Documents creating workers from scratch, NOT using root worker.

**Quote (line 6)**:
> "Each worker has three parts: **Identity** (who am I?), **Desired** (what should I be?), and **Observed** (what am I?). The supervisor reconciles these by calling `State.Next(snapshot)` on every tick."

**Analysis**: Presents workers as the fundamental unit. Root supervisor is mentioned in "Where to Find Examples" but NOT as "how to start using FSM v2."

**Line 149-156**:
```markdown
| Example | Location | Demonstrates |
|---------|----------|--------------|
| Parent-child coordination | `pkg/fsmv2/examples/parent_child/` | State mapping, child lifecycle |
| Communicator worker | `pkg/fsmv2/workers/communicator/` | Production worker with auth/sync |
| Example workers | `pkg/fsmv2/workers/example/` | Simple parent and child patterns |
```

**Missing**: No mention of root supervisor as the starting point or canonical pattern.

---

### 1.2 Main README (`pkg/fsmv2/README.md`)

**How it describes FSM v2**:

**Line 1-16 (The Problem statement)**:
- Focuses on simplifying FSM creation compared to looplab/fsm
- No mention of root supervisor as THE way to start

**Line 338-360 (Responsibilities of the worker)**:
- Lists worker interface methods
- No mention of "first create root supervisor, then register worker types"

**Line 386-400 (Responsibilities of the supervisor)**:
- Describes supervisor tick loop
- No mention of root supervisor being the canonical supervisor type

**Mental Model #5 (Line 150-182) - Declarative Child Management**:
```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (DesiredState, error) {
    return DesiredState{
        State: "running",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "mqtt-connection",
                WorkerType: "mqtt_client",
                UserSpec:   UserSpec{Config: "url: tcp://localhost:1883"},
            },
        },
    }, nil
}
```

**Analysis**: Shows parent-child pattern but NOT "root worker + register types + UserSpecs" pattern.

**Verdict**: README presents FSM v2 as "create workers directly" NOT "start with root worker."

---

### 1.3 Root Package Documentation (`pkg/fsmv2/root/README.md`)

**Line 1-13**:
```markdown
Package `root` provides a generic passthrough root supervisor for FSMv2. The root supervisor dynamically creates children based on YAML configuration, allowing any registered worker type to be instantiated as a child.

## Overview

The passthrough pattern allows a root supervisor to manage children without hardcoding specific child types. The root worker parses YAML configuration to extract `ChildrenSpecs`, and the supervisor's `reconcileChildren()` automatically creates child supervisors using the factory registry.
```

**Key phrase**: "**allows** a root supervisor" (NOT "applications SHOULD use")

**Line 9-13**:
```markdown
This pattern is ideal when you need a root supervisor that:
- Manages multiple child worker types
- Configures children via YAML at runtime
- Doesn't need root-level business logic
```

**Analysis**: Positioned as "ideal when..." (optional pattern) NOT "the way to use FSM v2"

**Verdict**: Root package describes itself as ONE option, not THE canonical entry point.

---

### 1.4 Example Documentation (`workers/example/root_supervisor/README.md`)

**Line 1-12**:
```markdown
This package demonstrates how to use the generic `root` package with custom child worker types. It shows the passthrough pattern where a root supervisor dynamically creates children based on YAML configuration.

## Overview

This example implements:
- **ChildWorker** - A leaf worker type that can be managed by the passthrough root
- **Factory registration** - Automatic registration via `init()`
- **State types** - ObservedState and DesiredState for the child worker
```

**Analysis**: Uses language of "demonstrates" and "shows the passthrough pattern" - example language, not canonical-pattern language.

**Verdict**: Positioned as an example, not THE recommended approach.

---

## 2. Integration Test Patterns

### 2.1 Root-Based Tests

**File**: `pkg/fsmv2/root/integration_test.go`

**Pattern Used**:
```go
rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
    ID:     "root-001",
    Name:   "Test Root Supervisor",
    Store:  mockStore,
    Logger: logger,
})
```

**Count**: 7 test contexts, all use `root.NewRootSupervisor()`

**Verdict**: Root package's own tests use root supervisor exclusively (expected).

---

**File**: `pkg/fsmv2/workers/example/root_supervisor/integration_test.go`

**Pattern Used**:
```go
rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
    ID:           "root-002",
    Name:         "Test Root Worker",
    Store:        mockStore,
    Logger:       logger,
    TickInterval: 100 * time.Millisecond,
})
```

**Count**: 6 test contexts, all use `root.NewRootSupervisor()`

**Verdict**: Example tests use root supervisor exclusively.

---

### 2.2 Direct Supervisor Creation Tests

**File**: `pkg/fsmv2/integration/integration_test.go`

**Pattern Used**:
```go
// No supervisor creation - tests config utilities only
Describe("Phase 0.5 Integration Tests", func() {
    Describe("Scenario 1: Variable Flattening", func() {
        // Tests VariableBundle.Flatten()
    })
})
```

**Count**: 7 scenarios, NONE create supervisors (utility testing only)

**Verdict**: Not applicable - tests config utilities, not supervisor creation.

---

**File**: `pkg/fsmv2/supervisor/supervisor_action_integration_test.go`

**Pattern Used**:
```go
sup := supervisor.NewSupervisor[TestObservedState, *TestDesiredState](supervisor.Config{
    WorkerType:   workerType,
    Store:        store,
    Logger:       logger,
    TickInterval: 50 * time.Millisecond,
})

err := sup.AddWorker(identity, worker)
```

**Count**: Multiple tests, all create supervisors directly via `supervisor.NewSupervisor()`

**Verdict**: Supervisor-level tests create supervisors directly (NOT via root).

---

**File**: `pkg/fsmv2/workers/communicator/integration_test.go`

**Pattern Not Found**: Could not locate file or pattern.

**Verdict**: Production communicator worker likely doesn't use root supervisor yet.

---

### 2.3 Summary: Test Pattern Ratio

| Test Type | Root-Based | Direct Supervisor | Ratio |
|-----------|------------|-------------------|-------|
| Root package tests | 7 | 0 | 100% root |
| Example tests | 6 | 0 | 100% root |
| Supervisor tests | 0 | ~10 | 0% root |
| Config utility tests | N/A | N/A | N/A |

**Conclusion**: Tests use BOTH patterns depending on what's being tested:
- Root supervisor tests use root (naturally)
- Supervisor internals tests create supervisors directly (also natural)
- NO evidence of production code using root supervisor yet

---

## 3. Architectural Intent

### 3.1 Design Document Analysis (`docs/plan.md`)

**Critical Finding**: This file describes the FSM v2 architecture fix plan, NOT the root supervisor design.

**Lines 1-40 (Executive Summary)**:
```markdown
The FSM v2 architecture has **three critical issues** causing team confusion:

1. **Critical Bug:** `DeriveDesiredState` is defined but **NEVER CALLED** by the supervisor
2. **Missing Feature:** No state mapping API exists for parent-child state coordination
3. **Missing Feature:** No hierarchical FSM support - only flat architecture works
```

**Analysis**: Plan focuses on fixing FSM v2 fundamentals, not defining root supervisor as canonical entry.

**Lines 154-192 (Solution 2: Create State Mapping Registry)**:
- Describes parent-child coordination
- NO mention of root supervisor as the way to achieve this

**Verdict**: Original FSM v2 plan did NOT envision root supervisor as canonical entry point.

---

### 3.2 Root Supervisor Plan (`workers/example/root_supervisor/plan.md`)

**Lines 1-14 (Metadata and Overview)**:
```markdown
## Overview

### Problem Statement
FSM v2 needs a production-ready, generic root supervisor that can dynamically manage any registered worker types based on configuration. The current approach of hardcoding child types in examples doesn't scale and isn't suitable for production use.
```

**Key phrase**: "FSM v2 **needs** a production-ready, generic root supervisor" (implies it was ADDED, not the original design)

**Line 15-23 (Architecture Decision)**:
```markdown
### Architecture Decision: Option C - Passthrough Root Worker
After analysis, we're implementing Option C: a generic passthrough root worker that:
1. Lives in production code (`/pkg/fsmv2/root/`) not examples
2. Parses YAML config to extract `children:` array
3. Passes through ChildrenSpecs without hardcoding types
4. Allows any registered worker type to be a child
```

**Analysis**: Root supervisor was a LATER addition to solve "hardcoding child types" problem.

**Lines 629-657 (Success Criteria)**:
```markdown
### Architecture Requirements
- [ ] Production code in pkg/fsmv2/root/ (not examples)
- [ ] Generic passthrough pattern - no hardcoded child types
- [ ] YAML config dynamically drives children
```

**Analysis**: Success criteria focus on "production code" and "dynamic children" - NOT "canonical entry point for all FSM v2 applications."

**Verdict**: Root supervisor was designed as **production-quality infrastructure** for managing dynamic children, NOT as "the way users should start using FSM v2."

---

### 3.3 Version History Analysis

**Root package version**: v2.1.0 (2025-11-19)
**FSM v2 plan version**: 2025-11-18

**Timeline**:
1. FSM v2 core architecture designed (plan.md)
2. Root supervisor added 1 day later to solve dynamic child management
3. Root positioned in **production code** (`pkg/fsmv2/root/`) not examples

**Conclusion**: Root supervisor is NEW (Nov 2025) infrastructure, not original FSM v2 design.

---

## 4. User's Vision vs Current State

### 4.1 User's Hypothesis

> "Every application should start with the root worker - this is the recommended way. The root worker is not an example, it's THE way to use the library. This gives developers a clear path:
> 1. Start with root worker
> 2. Add UserSpecs for your workers in the config
> 3. Register your worker types in the factory
> 4. Done."

**User's Vision**: Root supervisor as **mandatory entry point** and **prescribed workflow**.

---

### 4.2 Current State

**What EXISTS**:
1. Root supervisor in production code (`pkg/fsmv2/root/`)
2. `NewRootSupervisor()` helper function
3. Factory registration system for dynamic child types
4. YAML-based child specification
5. Automatic child creation via `reconcileChildren()`

**What's MISSING for user's vision**:
1. Documentation saying "start with root supervisor"
2. Getting-started guide showing root supervisor first
3. Main README positioning root as THE entry point
4. Production code examples using root supervisor
5. Clear statement: "Don't create supervisors directly, use root"

**Gap**: Root supervisor is **architected as production code** but **documented as one option among many**.

---

### 4.3 Current Guidance vs User's Vision

| Aspect | Current Guidance | User's Vision |
|--------|------------------|---------------|
| **Entry Point** | "Create workers and supervisors" | "Start with root supervisor" |
| **First Example** | Define worker interface | Call `root.NewRootSupervisor()` |
| **Worker Creation** | Implement Worker interface | Register factory, add UserSpec |
| **Supervisor Creation** | `supervisor.NewSupervisor()` + `AddWorker()` | Automatic via reconciliation |
| **Children** | Optional hierarchies | Standard pattern via YAML |
| **Positioning** | "Core library concept" | "THE way to use the library" |

**Conclusion**: Current documentation presents FSM v2 as a **general-purpose framework**, not "root-supervisor-based architecture."

---

## 5. Practical Implications

### 5.1 Can Developers Create Supervisors Without Root?

**YES**, and this is documented:

**From supervisor tests** (`supervisor/supervisor_action_integration_test.go`):
```go
sup := supervisor.NewSupervisor[TestObservedState, *TestDesiredState](Config{...})
sup.AddWorker(identity, worker)
```

**From root README** (Line 140-147):
```markdown
### Root Workers Need Explicit AddWorker()

Root workers must be added explicitly to the supervisor:

```go
sup := supervisor.NewSupervisor[O, D](cfg)
err := sup.AddWorker(identity, worker)  // Required for root
```

This is handled automatically by `NewRootSupervisor()`.
```

**Analysis**: Developers CAN and DO create supervisors directly. Root supervisor is presented as a HELPER, not a requirement.

---

### 5.2 What's Lost by NOT Using Root Worker?

**Without root supervisor**:
1. **Manual supervisor creation**: Must call `NewSupervisor()` + `AddWorker()` yourself
2. **Static child types**: Children must be created explicitly in worker code
3. **No YAML-driven children**: Can't configure children via external config
4. **Hardcoded types**: Parent workers know specific child types

**With root supervisor**:
1. **One-line setup**: `root.NewRootSupervisor()` creates everything
2. **Dynamic child types**: Any registered worker type can be a child
3. **YAML configuration**: Children defined in external YAML
4. **Passthrough pattern**: Root doesn't know child types (registered in factory)

**Verdict**: Root supervisor provides **dynamic child management** - critical for umh-core's protocol converter use case, but NOT universally necessary.

---

### 5.3 What's Gained by Making Root Mandatory?

**Pros** (if root becomes THE way):

1. **Consistency**: Every application follows the same pattern
2. **Simplicity**: Users don't need to understand `NewSupervisor()` + `AddWorker()`
3. **Configuration-driven**: All workers defined in YAML, not Go code
4. **Factory-centric**: Forces good practice of registering worker types
5. **Kubernetes-like**: Declarative resource specs (like k8s Deployments)
6. **Clear path**: "Register types, write YAML, done"

**Cons** (if root becomes THE way):

1. **Overkill for simple cases**: Single-worker apps don't need root infrastructure
2. **Indirection**: Extra layer between user and supervisor (harder to debug)
3. **YAML overhead**: Must learn YAML config format for trivial cases
4. **Breaking change**: Existing code creates supervisors directly
5. **Testing complexity**: Tests need factory registration boilerplate
6. **Learning curve**: Must understand factory system before creating workers

**Verdict**: Root as mandatory simplifies **complex applications** but complicates **simple use cases**.

---

## 6. Gap Analysis

### 6.1 User's Vision vs Architectural Reality

| Element | User's Vision | Current Architecture |
|---------|---------------|---------------------|
| **Code location** | Production code | ✅ `pkg/fsmv2/root/` (production) |
| **Code quality** | Production-ready | ✅ Complete implementation with tests |
| **Purpose** | Canonical entry point | ❌ Dynamic child management tool |
| **Documentation** | "THE way" | ❌ "One pattern for dynamic children" |
| **Examples** | First thing shown | ❌ Mentioned in "Where to Find Examples" |
| **Getting-started** | Step 1: Use root | ❌ Step 1: Create worker interface |
| **Production usage** | Standard practice | ❌ Not yet used in production code |

**Conclusion**: Architecture SUPPORTS user's vision (production code, quality implementation) but documentation CONTRADICTS it (presents as optional).

---

### 6.2 Documentation Changes Needed

To position root as canonical entry point:

**1. Getting-started.md rewrite**:
```markdown
# Getting Started with FSM v2

## Quick Start: The Root Supervisor Pattern

FSM v2 applications follow this pattern:

1. **Create root supervisor** with YAML config
2. **Register your worker types** in factory
3. **Define workers** in YAML UserSpecs
4. **Start the supervisor**

```go
import (
    "github.com/.../pkg/fsmv2/root"
    _ "github.com/.../myworker"  // Registers factory via init()
)

sup, err := root.NewRootSupervisor(root.SupervisorConfig{
    YAMLConfig: `
children:
  - name: "my-worker"
    workerType: "myworker"
    userSpec:
      config: |
        key: value
`,
})
done := sup.Start(ctx)
```

**2. Main README positioning**:
```markdown
## The Root Supervisor Pattern

FSM v2 applications use a **root supervisor** to manage worker lifecycles.
This pattern provides:
- Configuration-driven worker creation (YAML)
- Dynamic worker types via factory registration
- Automatic child lifecycle management
- Production-ready infrastructure

See `pkg/fsmv2/root/` for complete documentation.
```

**3. Remove "create workers directly" examples from getting-started**:
- Move direct supervisor creation to "Advanced Usage"
- Make root supervisor the default path
- Position direct creation as "for library developers"

**4. Update README table "Where to Find Examples"**:
```markdown
| Example | Location | Demonstrates |
|---------|----------|--------------|
| **Root supervisor (start here)** | `pkg/fsmv2/root/` | The recommended way to use FSM v2 |
| Custom worker types | `pkg/fsmv2/workers/example/` | How to implement worker interfaces |
| Production worker | `pkg/fsmv2/workers/communicator/` | Real-world worker implementation |
```

---

### 6.3 Code Changes Needed

**1. Production usage**: Use root supervisor in umh-core main.go

**Current** (hypothetical - no evidence found):
```go
// Create supervisors manually
benthosSupervior := supervisor.NewSupervisor[BenthosObserved, *BenthosDesired](cfg)
redpandaSupervisor := supervisor.NewSupervisor[RedpandaObserved, *RedpandaDesired](cfg)
```

**Recommended** (user's vision):
```go
// Register worker types (in init() of respective packages)
// Then use root supervisor
rootSup, err := root.NewRootSupervisor(root.SupervisorConfig{
    YAMLConfig: config.ReadRootConfig(),  // Reads from /data/config.yaml
})
```

**2. Make NewRootSupervisor the FIRST function in root/README.md**

**3. Add root supervisor example to pkg/fsmv2/README.md (before worker interface examples)**

---

## 7. Recommendations

### 7.1 Should Root Be THE Way?

**YES**, with conditions:

**Reasons to make root canonical**:

1. **UMH use case**: umh-core NEEDS dynamic children (protocol converters, data flows)
2. **Consistency**: Team confusion comes from multiple patterns (plan.md line 27-40)
3. **Simplicity**: "Register types + YAML" is clearer than "implement Worker + create Supervisor"
4. **Production-ready**: Root supervisor code is complete and tested
5. **Kubernetes analogy**: Industry-standard pattern for declarative resources

**Conditions**:

1. **Document the escape hatch**: "Advanced users can create supervisors directly"
2. **Simplify for trivial cases**: Provide helper for single-worker apps
3. **Migration guide**: Help existing code transition to root pattern
4. **Clear benefits**: Document WHY root is better (not just "do it this way")

---

### 7.2 Migration Path

**Phase 1: Documentation (1-2 days)**

1. Rewrite `docs/getting-started.md` to show root first
2. Add "Root Supervisor Pattern" section to main README
3. Update "Where to Find Examples" table
4. Add "When to create supervisors directly" advanced guide

**Phase 2: Examples (1 day)**

1. Update `pkg/fsmv2/workers/example/` to use root pattern
2. Create "single-worker" example showing simplified root usage
3. Add "Advanced: Direct Supervisor Creation" example

**Phase 3: Production Code (2-3 days)**

1. Use root supervisor in umh-core main.go
2. Refactor existing worker creation to use factory registration
3. Create YAML config for umh-core workers

**Phase 4: Deprecation (Future)**

1. Mark `supervisor.NewSupervisor()` as "advanced API"
2. Add linter rule recommending root supervisor
3. Update team coding standards

---

### 7.3 Pros/Cons Summary

**Pros of "Root as Canonical Entry Point"**:

| Benefit | Impact |
|---------|--------|
| **Consistency** | All FSM v2 apps follow same pattern |
| **Simplicity** | Users learn one path: register → YAML → start |
| **Configuration-driven** | Workers defined externally, not in Go code |
| **Dynamic types** | Add new worker types without recompiling |
| **Kubernetes-like** | Familiar pattern for cloud-native developers |
| **Team alignment** | Solves "multiple ways to do it" confusion (plan.md) |
| **Production-ready** | Root code is complete, tested, documented |

**Cons of "Root as Canonical Entry Point"**:

| Drawback | Mitigation |
|----------|------------|
| **Overkill for simple cases** | Provide `root.NewSimpleSupervisor(worker)` helper |
| **Extra indirection** | Document internals in "Advanced Usage" section |
| **YAML overhead** | Show inline YAML in examples (not files) |
| **Breaking change** | Add migration guide, deprecate gradually |
| **Testing boilerplate** | Create test helpers for factory registration |
| **Learning curve** | Update getting-started to show root FIRST |

---

### 7.4 Final Recommendation

**Make root supervisor the CANONICAL entry point** with these changes:

**1. Documentation positioning**:
- Getting-started shows root FIRST
- Main README adds "Root Supervisor Pattern" section
- Direct supervisor creation moves to "Advanced Usage"

**2. Code organization**:
- Use root supervisor in umh-core production code
- Keep direct supervisor creation API available
- Add simplified helpers for single-worker cases

**3. Communication**:
- Update team standards to use root pattern
- Create migration guide for existing code
- Document "When to use direct supervisors" (library development, testing internals)

**4. Benefits messaging**:
```
Why use root supervisor?
✅ One pattern for all applications
✅ Configuration-driven (YAML, not Go code)
✅ Dynamic worker types via factory
✅ Automatic child lifecycle management
✅ Production-ready infrastructure

When to create supervisors directly?
- Testing supervisor internals
- Building FSM library extensions
- Applications with complex supervisor customization
```

---

## 8. Conclusion

**User's hypothesis is ARCHITECTURALLY SOUND but DOCUMENTATIONALLY UNSUPPORTED.**

The root supervisor:
- ✅ **Exists in production code** (`pkg/fsmv2/root/`)
- ✅ **Is production-ready** (complete implementation, integration tests)
- ✅ **Solves real problems** (dynamic children, YAML config, factory registration)
- ✅ **Matches UMH needs** (protocol converters, data flows)
- ❌ **Not positioned as canonical** (documentation shows direct supervisor creation)
- ❌ **Not used in production** (no evidence in umh-core main.go)
- ❌ **Not the first example** (getting-started shows worker interface first)

**The gap**: Root supervisor is architected as **production infrastructure** but documented as **one option among many**.

**Recommendation**: **Elevate root supervisor to canonical entry point** via:
1. Documentation rewrite (getting-started, main README)
2. Production usage (umh-core uses root supervisor)
3. Clear positioning ("THE recommended way" with documented escape hatches)
4. Migration guide for existing code

**Timeline**: 4-6 days to complete documentation + production migration.

**Risk**: Low - root supervisor code is complete, tested, and production-ready. Main risk is team adjustment to new "blessed path."

**Benefit**: Eliminates "multiple ways to do it" confusion identified in plan.md, provides clear path for new developers, aligns with UMH's dynamic worker requirements.

---

## Appendix A: File References

| File | Location | Purpose |
|------|----------|---------|
| Getting started | `/pkg/fsmv2/docs/getting-started.md` | User's first introduction |
| Main README | `/pkg/fsmv2/README.md` | Overview and mental models |
| Root package README | `/pkg/fsmv2/root/README.md` | Root supervisor documentation |
| Root plan | `/pkg/fsmv2/workers/example/root_supervisor/plan.md` | Root supervisor design |
| FSM v2 plan | `/pkg/fsmv2/docs/plan.md` | FSM v2 architecture fixes |
| Example README | `/pkg/fsmv2/workers/example/root_supervisor/README.md` | Example usage |
| Root integration tests | `/pkg/fsmv2/root/integration_test.go` | Root supervisor tests |
| Example integration tests | `/pkg/fsmv2/workers/example/root_supervisor/integration_test.go` | Example tests |
| Supervisor integration tests | `/pkg/fsmv2/supervisor/supervisor_action_integration_test.go` | Direct supervisor tests |

---

## Appendix B: Evidence Quotes

**1. Root positioned as optional** (root/README.md, line 9-13):
> "This pattern is **ideal when** you need a root supervisor that:
> - Manages multiple child worker types
> - Configures children via YAML at runtime"

**Analysis**: "Ideal when" = optional, not mandatory.

---

**2. Getting-started shows direct worker creation** (docs/getting-started.md, line 26-51):
> "## 5-Minute Example: Create a Simple Worker
>
> ### Step 1: Define State Types
> ### Step 2: Implement States
> ### Step 3: Implement Worker Interface"

**Analysis**: First example creates workers from scratch, no mention of root.

---

**3. Root supervisor was ADDED later** (workers/example/root_supervisor/plan.md, line 11-14):
> "FSM v2 **needs** a production-ready, generic root supervisor that can dynamically manage any registered worker types based on configuration. The **current approach** of hardcoding child types in examples doesn't scale."

**Analysis**: "Needs" and "current approach" indicate root was added to solve a problem, not the original design.

---

**4. Supervisor tests create supervisors directly** (supervisor/supervisor_action_integration_test.go):
```go
sup := supervisor.NewSupervisor[TestObservedState, *TestDesiredState](Config{...})
sup.AddWorker(identity, worker)
```

**Analysis**: Production-quality tests use direct creation, not root supervisor.

---

**5. Root package describes itself as production code** (workers/example/root_supervisor/plan.md, line 15-23):
> "Lives in production code (`/pkg/fsmv2/root/`) **not examples**"

**Analysis**: Architectural intent is production infrastructure, not example.

---

## Appendix C: User's Vision vs Reality Matrix

| Aspect | User's Vision | Current Reality | Gap |
|--------|---------------|-----------------|-----|
| **Code location** | Production code | ✅ `pkg/fsmv2/root/` | None |
| **Code quality** | Production-ready | ✅ Complete + tested | None |
| **Entry point** | Start with root | ❌ Start with Worker interface | **Critical** |
| **First example** | NewRootSupervisor() | ❌ Define state types | **Critical** |
| **Documentation** | "THE way" | ❌ "Ideal when..." | **Critical** |
| **Production usage** | Standard practice | ❌ Not used yet | **Major** |
| **Getting-started** | Root supervisor first | ❌ Worker interface first | **Critical** |
| **README positioning** | Prominent section | ❌ Mentioned in examples table | **Major** |

**Summary**: 5/8 aspects have gaps, 4 are CRITICAL (documentation/examples), 1 is MAJOR (production usage).

---

## Investigation Complete

**Files Created**:
- `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/investigation-4-root-as-canonical-entry.md`

**Evidence Sources**:
- 6 documentation files
- 4 integration test files
- 2 architecture plans
- 3 README files

**Verdict**: User's hypothesis is **architecturally valid** but **not reflected in documentation or current usage**. Root supervisor should be elevated to canonical entry point.
