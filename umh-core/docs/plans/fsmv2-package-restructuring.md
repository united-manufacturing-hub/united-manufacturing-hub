# FSMv2 Package Restructuring & Quality Improvements Plan

**Last Updated:** 2025-01-08
**Status:** In Progress
**Version:** 2.0 (Integrated Plan)

---

## Revision History

**v2.0 (2025-01-08):**
- Integrated findings from architecture investigation (21-task analysis)
- Added confusion prevention documentation (Phase 0)
- Added critical quality gates (idempotency CI, ChildSpec validation)
- Integrated unused code completion (ActionExecutor, metrics)
- Prioritized based on actual needs vs initial assumptions

**v1.0 (2025-01-08):**
- Original package restructuring plan
- Focus: Clean package organization, rename functions, delete examples

---

## Executive Summary

This plan integrates three complementary improvement initiatives for FSMv2:

1. **Original Package Restructuring** - Clean package organization, idiomatic Go structure
2. **Architecture Investigation Findings** - Quality improvements and documentation clarifications
3. **Unused Code Integration** - Complete ActionExecutor infrastructure and metrics instrumentation

**Key Insight from Investigation:** Most external confusion stemmed from **documentation gaps**, not implementation problems. FSMv2's architecture is fundamentally sound. We need targeted documentation to prevent future misunderstandings by developers and AI analysis tools.

**Total Scope:**
- 11 invalid recommendations **rejected** (OOP patterns inappropriate for Go)
- 2 critical code changes (idempotency enforcement, ChildSpec validation)
- 6 documentation improvements (prevent confusion, improve DX)
- Package organization (original restructuring)
- ActionExecutor completion (8-phase integration)
- Metrics wiring (7 unused metrics)

**Timeline:** 12-16 weeks (3-4 engineering months)

---

## Goals

### 1. Clean Package Structure (Original Goal)
**Why:** Improve discoverability, align with developer mental model
**Measures:** Package import count, new developer onboarding time

### 2. Prevent Architectural Confusion (NEW)
**Why:** Investigation revealed documentation gaps caused confusion
**Measures:** Reduction in "how do I..." questions, clarity of patterns
**Key Principle:** Even though implementation is correct, confusion means docs are insufficient

### 3. Enforce Critical Invariants (NEW)
**Why:** Idempotency is core FSM invariant (I10) but has 0% test coverage
**Measures:** Automated CI enforcement, 100% action test coverage
**Priority:** CRITICAL for correctness

### 4. Complete Unused Infrastructure (NEW)
**Why:** ActionExecutor and metrics partially implemented but not integrated
**Measures:** Async action execution working, metrics visible in Prometheus
**Priority:** HIGH for production observability

---

## Phase 0: Quick Wins - Confusion Prevention (Week 1, ~8 hours)

**Rationale:** Low-effort documentation prevents future misunderstandings from developers and AI tools. Investigation showed most confusion came from undocumented design decisions.

### 0.1 Clarify Design Decisions in Code Comments (2 hours)

**Problem:** External analysis tools expected OOP patterns, didn't understand Go idioms

**Files to modify:**

**`pkg/fsmv2/worker.go:86-114`** (Snapshot immutability)
```go
// DESIGN DECISION: Immutability via Pass-by-Value
//
// Snapshot is passed by value (not pointer) to prevent mutation.
// This is the idiomatic Go approach (see time.Time, net.IP).
//
// We do NOT use getters because:
// 1. Pass-by-value makes mutation impossible (copies on assignment)
// 2. Getters add boilerplate without adding safety
// 3. Go convention favors simple field access over accessors
//
// Example:
//   snapshot := worker.CollectObservedState()  // Returns copy
//   snapshot.UserSpec = nil                     // Mutates local copy only
//   original := worker.CollectObservedState()  // Still unchanged
type Snapshot struct {
    Identity      Identity
    DesiredState  DesiredState
    ObservedState ObservedState
    UserSpec      interface{}
}
```

**`pkg/fsmv2/supervisor/supervisor.go:245-260`** (Defense-in-depth validation)
```go
// DESIGN DECISION: Defense-in-Depth Validation Strategy
//
// We validate ChildSpec at FOUR layers (not one):
// 1. API entry (supervisor.AddWorker) - fast fail on obvious errors
// 2. Reconciliation entry (reconcileChildren) - catch runtime edge cases
// 3. Factory (worker creation) - validate WorkerType exists
// 4. Worker constructor - validate dependencies
//
// This is intentional, not redundant:
// - Malicious actors might bypass layer 1
// - Runtime bugs might skip layer 2
// - Factory might be called from multiple places
// - Each layer has different validation concerns
//
// Security principle: Never trust data, even from internal callers.
```

**`pkg/fsmv2/types/variables.go:23-45`** (map[string]any rationale)
```go
// DESIGN DECISION: Why map[string]any Instead of Typed Structs
//
// VariableBundle uses map[string]any because:
// 1. Users define arbitrary config fields in YAML (cannot pre-type)
// 2. Template variables are user-defined ({{ .CustomField }})
// 3. Type safety enforced at template rendering (Golang templates validate)
//
// Example user YAML:
//   variables:
//     CustomIP: "192.168.1.100"      # User-defined field
//     CustomPort: 502                # User-defined field
//     MySpecialFlag: true            # User-defined field
//
// We CANNOT use structs because field names are user-controlled.
// Type safety happens when template renders (undefined vars = error).
type VariableBundle struct {
    User     map[string]any  // User-defined config (YAML)
    Global   map[string]any  // System-wide (location_path, instance_id)
    Internal map[string]any  // Supervisor-only (not exposed to templates)
}
```

**`pkg/fsmv2/types/childspec.go:45-60`** (StateMapping clarification)
```go
// StateMapping: Parent State → Child State Coordination
//
// StateMapping allows parent FSM states to trigger child FSM state transitions.
// This is NOT data passing - it's state synchronization.
//
// Example use case: When parent enters "Starting" state, force all children
// to enter their "Initializing" state.
//
// Format:
//   StateMapping: map[string]string{
//       "ParentStateName": "ChildStateName",
//   }
//
// When to use:
// - Parent lifecycle controls child lifecycle (e.g., Stopping → Cleanup)
// - Parent operational state affects child behavior (e.g., Paused → Idle)
//
// When NOT to use:
// - Passing data between states (use VariableBundle instead)
// - Triggering actions (use signals instead)
```

**`pkg/fsmv2/worker_base.go:23-45`** (Generics pattern prominence)
```go
// BaseWorker[D Dependencies]: Generic Worker Base Class
//
// USING GENERICS (added Nov 2, 2025)
//
// BaseWorker provides common worker functionality with type-safe dependencies:
//
// Example worker using BaseWorker:
//   type MyWorkerDeps struct {
//       Logger    *zap.Logger
//       APIClient *http.Client
//   }
//
//   type MyWorker struct {
//       fsmv2.BaseWorker[MyWorkerDeps]
//   }
//
//   func NewMyWorker(deps MyWorkerDeps) *MyWorker {
//       return &MyWorker{
//           BaseWorker: fsmv2.NewBaseWorker(deps),
//       }
//   }
//
// Benefits:
// - Type-safe dependency access (deps.Logger, not interface{})
// - Common fields (Identity, etc.) inherited automatically
// - No casting required in worker methods
type BaseWorker[D Dependencies] struct {
    // ... fields ...
}
```

**Deliverable:** Code comments preventing future confusion

---

### 0.2 Document Established Patterns (4 hours)

**New file:** `pkg/fsmv2/PATTERNS.md`

**Content:**
```markdown
# FSMv2 Established Patterns

This document explains patterns that are already implemented and working correctly,
but may cause confusion if undocumented.

## State Types: Structs, Not Strings

**Pattern:** States are typed structs implementing `fsmv2.State` interface

**Example:**
```go
// CORRECT - States are structs
type StartingState struct {
    attempt int
    timeout time.Duration
}

func (s *StartingState) Next(snap Snapshot) (State, Signal, Action) {
    // State-specific logic
}

// INCORRECT - Don't use string constants
const StateStarting = "starting"  // ❌ Not type-safe
```

**Why structs:**
- Type safety (compiler catches typos)
- State-specific data (attempt counters, timers)
- Method-based transitions (not case statements)
- Compiler enforces interface implementation

## Variable Namespaces: Three Tiers

**Pattern:** Variables organized in three distinct namespaces

**Tiers:**

1. **User Variables** - Defined in YAML config, exposed to templates
   - Example: `{{ .IP }}`, `{{ .PORT }}`, custom fields
   - Use when: User needs to configure behavior
   - Validation: Template rendering (undefined = error)

2. **Global Variables** - System-wide computed values
   - Example: `{{ .location_path }}`, `{{ .instance_id }}`
   - Use when: All workers need same value
   - Set by: Agent location + worker location merge

3. **Internal Variables** - Supervisor-only, not exposed to templates
   - Example: Retry counters, circuit breaker state
   - Use when: Supervisor needs to track state
   - Never exposed to user config or templates

**Why three tiers:**
- Security (internal state hidden from templates)
- Flexibility (users define arbitrary fields)
- Clarity (global vs user-defined distinction)

## State Naming Conventions

**Pattern:** State names follow consistent conventions

**Convention:**

1. **Lifecycle States** (temporary, transitioning)
   - Prefix: "TryingTo" (e.g., TryingToStart, TryingToStop)
   - Present continuous tense
   - Indicates action in progress

2. **Operational States** (stable, long-lived)
   - No prefix (e.g., Running, Stopped, Paused)
   - Simple noun or adjective
   - Indicates current condition

3. **Mapped States** (from parent StateMapping)
   - Variable name: `mappedParentState`
   - Read from snapshot: `snap.DesiredState.StateMapping[parentState]`
   - Applied conditionally in Next()

**Example:**
```go
type TryingToStart struct {}  // Lifecycle (transitioning)
type Running struct {}        // Operational (stable)

func (s *SomeState) Next(snap Snapshot) (State, Signal, Action) {
    // Check mapped state from parent
    if mappedState, ok := snap.DesiredState.StateMapping[s.Name()]; ok {
        return stateFactory.Create(mappedState), nil, nil
    }
    // ... normal logic ...
}
```

## Validation Layers: Defense-in-Depth

**Pattern:** Validate data at multiple layers, not one centralized validator

**Layers:**

1. **API Entry** (`supervisor.AddWorker`) - Fast fail on obvious errors
2. **Reconciliation Entry** (`reconcileChildren`) - Catch runtime edge cases
3. **Factory** (worker creation) - Validate WorkerType exists
4. **Worker Constructor** - Validate dependencies

**Why multiple layers:**
- Security (never trust data, even from internal callers)
- Debuggability (errors caught closest to source)
- Robustness (one layer failing doesn't compromise system)

**This is NOT redundant.** Each layer has different concerns:
- Layer 1: Public API validation (protect against bad calls)
- Layer 2: Runtime state validation (data evolved since layer 1)
- Layer 3: Registry validation (WorkerType registered?)
- Layer 4: Logical validation (dependencies compatible?)

## Testing Patterns

See `pkg/fsmv2/TESTING.md` for comprehensive testing patterns.

**Key helpers:**
- `TestWorker` - Mock worker for unit tests
- `CreateTestTriangularStore` - In-memory storage for tests
- `VerifyActionIdempotency` - Mandatory idempotency test helper

**Full lifecycle test example in TESTING.md**
```

**Deliverable:** Pattern documentation for established conventions

---

### 0.3 Improve Package Godoc (2 hours)

**File:** `pkg/fsmv2/doc.go` (create if doesn't exist)

**Content:**
```go
// Package fsmv2 provides a type-safe finite state machine framework for worker lifecycle management.
//
// # Architecture Overview
//
// FSMv2 organizes workers into three conceptual layers:
//
// 1. API Layer (what you implement):
//    - Worker interface (CollectObservedState, DeriveDesiredState, GetInitialState)
//    - State interface (Next method for transitions)
//    - Action interface (Execute for side effects)
//
// 2. Infrastructure Layer (what runs your workers):
//    - Supervisor (tick loop, reconciliation)
//    - Collector (async ObservedState collection)
//    - ActionExecutor (async action execution with retry)
//
// 3. Utility Layer (configuration helpers):
//    - config/ package (template rendering, location path computation)
//    - types/ package (Snapshot, Identity, VariableBundle)
//
// # Quick Start
//
// Implement the Worker interface:
//
//   type MyWorker struct {
//       fsmv2.BaseWorker[MyDeps]
//   }
//
//   func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
//       // Query real system state
//   }
//
//   func (w *MyWorker) DeriveDesiredState(userSpec interface{}) (fsmv2.DesiredState, error) {
//       // Parse user config
//   }
//
//   func (w *MyWorker) GetInitialState() fsmv2.State {
//       return &states.InitialState{}
//   }
//
// Register and run:
//
//   factory.RegisterWorkerType("my-worker", NewMyWorker)
//
//   supervisor := supervisor.New(supervisorID)
//   supervisor.AddWorker(identity, worker)
//   supervisor.Start(ctx)
//
// # Key Concepts
//
// See PATTERNS.md for detailed pattern documentation:
// - State types (structs, not strings)
// - Variable namespaces (User/Global/Internal)
// - State naming conventions (TryingTo vs operational)
// - Validation layers (defense-in-depth)
//
// See TESTING.md for test patterns and examples.
//
// See supervisor/METRICS.md for observability instrumentation.
package fsmv2
```

**Deliverable:** Improved package-level documentation with architecture overview

---

## Phase 1: Foundation + Critical Quality Gates (Weeks 1-2)

### 1.1 Rename FillISA95Gaps → NormalizeHierarchyLevels (Original Plan)

**Files to modify:**
- `pkg/fsmv2/location/location.go`
- `pkg/fsmv2/location/location_test.go`
- `pkg/fsmv2/integration/integration_test.go`

**Migration:**
```bash
# Update function name in 3 files
# Add backward-compatible alias:
func FillISA95Gaps(levels []LocationLevel) []LocationLevel {
    return NormalizeHierarchyLevels(levels)
}
```

**Effort:** 2 hours
**Impact:** None (backward compatible)

---

### 1.2 Delete examples/ Folder (Original Plan)

**Rationale:** Phase 0.5 demonstrations superseded by production workers

**Command:**
```bash
rm -rf pkg/fsmv2/examples/
```

**Verification:**
```bash
# Verify no production dependencies
grep -r "fsmv2/examples" pkg/
```

**Effort:** 30 minutes
**Impact:** None (zero production dependencies)

---

### 1.3 Idempotency CI Enforcement (NEW - CRITICAL)

**Problem:** Idempotency is Invariant I10 but has 0% test coverage (0/2 actions tested)

**Evidence from investigation:**
- Helper exists: `pkg/fsmv2/supervisor/execution/action_test_helpers_test.go:42-51`
- Violations: `authenticate_test.go`, `sync_test.go` lack idempotency tests
- Risk: Non-idempotent actions will break FSM retry logic

**Implementation:**

**File 1:** `Makefile` (add check target)
```makefile
# Check that all actions have idempotency tests
.PHONY: check-idempotency-tests
check-idempotency-tests:
	@echo "Checking for idempotency test coverage..."
	@FAILED=0; \
	for action in $$(find pkg/fsmv2 -name "*action.go" -not -name "*_test.go"); do \
		test_file=$$(echo $$action | sed 's/\.go/_test\.go/'); \
		if [ -f "$$test_file" ]; then \
			if ! grep -q "VerifyActionIdempotency" "$$test_file"; then \
				echo "FAIL: $$test_file missing idempotency test"; \
				FAILED=1; \
			fi \
		else \
			echo "FAIL: $$action has no test file"; \
			FAILED=1; \
		fi \
	done; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✓ All actions have idempotency tests"; \
	else \
		exit 1; \
	fi
```

**File 2:** `.github/workflows/test.yml` (add CI step)
```yaml
- name: Check idempotency test coverage
  run: make check-idempotency-tests
```

**File 3:** Fix violations in `authenticate_test.go`
```go
var _ = Describe("Authenticate Action Idempotency", func() {
    It("should be idempotent when authentication succeeds", func() {
        deps := setupTestDeps()
        action := &AuthenticateAction{deps: deps}

        execution.VerifyActionIdempotency(action, 3)
    })

    It("should be idempotent when authentication fails", func() {
        deps := setupTestDeps()
        deps.AuthService.InjectError(errors.New("auth failed"))
        action := &AuthenticateAction{deps: deps}

        execution.VerifyActionIdempotency(action, 3)
    })
})
```

**File 4:** Fix violations in `sync_test.go` (similar pattern)

**File 5:** `pkg/fsmv2/README.md` (document requirement)
```markdown
## Testing Actions

All actions MUST include idempotency tests using the helper:

```go
import "github.com/.../pkg/fsmv2/supervisor/execution"

var _ = Describe("MyAction Idempotency", func() {
    It("should be idempotent", func() {
        action := &MyAction{}
        execution.VerifyActionIdempotency(action, 3)
    })
})
```

The CI will fail if any action lacks idempotency tests.
```

**Effort:** 7 hours
- Makefile target: 2 hours
- CI integration: 1 hour
- Fix authenticate_test.go: 2 hours
- Fix sync_test.go: 1 hour
- Documentation: 1 hour

**Priority:** CRITICAL (addresses core invariant)
**Deliverable:** Automated enforcement operational, 100% coverage

---

### 1.4 ChildSpec Validation Layer (NEW)

**Problem:** No validation for empty Name or WorkerType in ChildSpec

**Evidence from investigation:**
- No validation exists: `pkg/fsmv2/types/childspec.go` has no Validate() method
- Errors only appear at runtime (factory creation fails)
- Poor developer experience (cryptic error messages)

**Implementation:**

**File 1:** `pkg/fsmv2/types/childspec.go` (add validation)
```go
// Validate checks ChildSpec for common errors.
// Call this at reconciliation entry point to fail fast.
func (c ChildSpec) Validate() error {
    if c.Name == "" {
        return fmt.Errorf("ChildSpec.Name cannot be empty")
    }

    if c.WorkerType == "" {
        return fmt.Errorf("ChildSpec.WorkerType cannot be empty (child: %s)", c.Name)
    }

    // Check for self-mapping (parent state maps to itself)
    for parentState, childState := range c.StateMapping {
        if parentState == childState {
            return fmt.Errorf("ChildSpec %s: StateMapping[%s] maps to itself (infinite loop)",
                c.Name, parentState)
        }
    }

    // Check for circular StateMapping references (A→B, B→A)
    // This is a heuristic - full cycle detection requires runtime knowledge
    if len(c.StateMapping) > 10 {
        return fmt.Errorf("ChildSpec %s: StateMapping has %d entries (suspicious, check for cycles)",
            c.Name, len(c.StateMapping))
    }

    return nil
}
```

**File 2:** `pkg/fsmv2/types/childspec_test.go` (add tests)
```go
var _ = Describe("ChildSpec Validation", func() {
    It("should reject empty Name", func() {
        spec := ChildSpec{
            Name:       "",
            WorkerType: "communicator",
        }
        err := spec.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("Name cannot be empty"))
    })

    It("should reject empty WorkerType", func() {
        spec := ChildSpec{
            Name:       "child-1",
            WorkerType: "",
        }
        err := spec.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("WorkerType cannot be empty"))
    })

    It("should reject self-mapping states", func() {
        spec := ChildSpec{
            Name:       "child-1",
            WorkerType: "communicator",
            StateMapping: map[string]string{
                "Starting": "Starting",  // Invalid: maps to itself
            },
        }
        err := spec.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("maps to itself"))
    })

    It("should accept valid ChildSpec", func() {
        spec := ChildSpec{
            Name:       "child-1",
            WorkerType: "communicator",
            StateMapping: map[string]string{
                "Starting": "Initializing",
            },
        }
        err := spec.Validate()
        Expect(err).ToNot(HaveOccurred())
    })
})
```

**File 3:** `pkg/fsmv2/supervisor/supervisor.go:1263` (call validation)
```go
func (s *Supervisor) reconcileChildren(ctx context.Context, desired DesiredState) error {
    // NEW: Validate all ChildSpecs before processing
    for _, childSpec := range desired.ChildrenSpecs {
        if err := childSpec.Validate(); err != nil {
            s.logger.Errorf("Invalid ChildSpec: %v", err)
            return fmt.Errorf("invalid child spec: %w", err)
        }
    }

    // ... existing reconciliation logic ...
}
```

**Effort:** 3-5 days
- ChildSpec.Validate(): 1 day
- Tests: 1 day
- Integration: 1 day
- Edge case testing: 1-2 days

**Priority:** MEDIUM (developer experience)
**Deliverable:** Better error messages, faster debugging

---

## Phase 2: Documentation & Testing Infrastructure (Weeks 3-4)

**NEW phase for developer experience improvements**

### 2.1 Create TESTING.md (4 hours)

**File:** `pkg/fsmv2/TESTING.md`

**Content:**
```markdown
# FSMv2 Testing Guide

## Test Helpers

### TestWorker
Mock worker for unit tests:

```go
type TestWorker struct {
    ObservedStateFunc  func() (fsmv2.ObservedState, error)
    DesiredStateFunc   func(interface{}) (fsmv2.DesiredState, error)
    InitialState       fsmv2.State
}
```

### CreateTestTriangularStore
In-memory storage for tests:

```go
store := supervisor.CreateTestTriangularStore()
supervisor := supervisor.New(supervisorID, supervisor.WithStorage(store))
```

### VerifyActionIdempotency
Mandatory helper for action tests:

```go
import "github.com/.../pkg/fsmv2/supervisor/execution"

var _ = Describe("MyAction Idempotency", func() {
    It("should be idempotent", func() {
        action := &MyAction{deps: deps}
        execution.VerifyActionIdempotency(action, 3)
    })
})
```

## Full Lifecycle Test Example

```go
var _ = Describe("Worker Lifecycle", func() {
    var (
        supervisor *supervisor.Supervisor
        ctx        context.Context
        cancel     context.CancelFunc
    )

    BeforeEach(func() {
        ctx, cancel = context.WithCancel(context.Background())
        supervisor = setupTestSupervisor()
    })

    AfterEach(func() {
        cancel()
        supervisor.Stop(ctx)
    })

    It("should transition from Initial → Running → Stopped", func() {
        // Setup
        worker := &TestWorker{
            InitialState: &states.InitialState{},
            ObservedStateFunc: func() (fsmv2.ObservedState, error) {
                return fsmv2.ObservedState{Ready: true}, nil
            },
            DesiredStateFunc: func(spec interface{}) (fsmv2.DesiredState, error) {
                return fsmv2.DesiredState{Config: spec}, nil
            },
        }

        identity := fsmv2.Identity{ID: "test-worker"}
        supervisor.AddWorker(identity, worker)

        // Start supervisor (begins tick loop)
        supervisor.Start(ctx)

        // Wait for state progression
        Eventually(func() string {
            snap := supervisor.GetSnapshot(identity.ID)
            return snap.CurrentState.Name()
        }).Should(Equal("Running"))

        // Trigger shutdown
        supervisor.RemoveWorker(identity.ID)

        Eventually(func() string {
            snap := supervisor.GetSnapshot(identity.ID)
            return snap.CurrentState.Name()
        }).Should(Equal("Stopped"))
    })
})
```

## Action Idempotency Test Example

```go
var _ = Describe("AuthenticateAction", func() {
    Context("Idempotency", func() {
        It("should produce same result when called multiple times", func() {
            deps := setupTestDeps()
            action := &AuthenticateAction{deps: deps}

            // Helper verifies:
            // 1. First call succeeds (or fails consistently)
            // 2. Subsequent calls produce identical result
            // 3. No state corruption across calls
            execution.VerifyActionIdempotency(action, 3)
        })

        It("should be idempotent even when failing", func() {
            deps := setupTestDeps()
            deps.AuthService.InjectError(errors.New("auth failed"))
            action := &AuthenticateAction{deps: deps}

            // Idempotency applies to failures too
            execution.VerifyActionIdempotency(action, 3)
        })
    })
})
```

## Integration Test Patterns

### Testing Hierarchies

```go
It("should propagate signals from parent to children", func() {
    parentWorker := createTestWorker("parent")
    childWorker1 := createTestWorker("child-1")
    childWorker2 := createTestWorker("child-2")

    supervisor.AddWorker(parentID, parentWorker)
    supervisor.AddWorker(child1ID, childWorker1)
    supervisor.AddWorker(child2ID, childWorker2)

    // Configure hierarchy
    parentWorker.DesiredStateFunc = func(spec interface{}) (fsmv2.DesiredState, error) {
        return fsmv2.DesiredState{
            ChildrenSpecs: []ChildSpec{
                {Name: "child-1", WorkerType: "test-worker"},
                {Name: "child-2", WorkerType: "test-worker"},
            },
        }, nil
    }

    supervisor.Start(ctx)

    // Emit signal from parent
    parentWorker.EmitSignal(fsmv2.Signal{Type: "restart"})

    // Verify children received signal
    Eventually(func() bool {
        snap1 := supervisor.GetSnapshot(child1ID)
        snap2 := supervisor.GetSnapshot(child2ID)
        return snap1.ReceivedSignal("restart") && snap2.ReceivedSignal("restart")
    }).Should(BeTrue())
})
```

### Testing Long-Running Actions

```go
It("should block worker during long-running action", func() {
    worker := &TestWorker{
        InitialState: &StateEmitsSlowAction{},
    }

    supervisor.AddWorker(identity, worker)
    supervisor.Start(ctx)

    // Wait for action to start
    Eventually(func() bool {
        workerCtx := supervisor.GetWorkerContext(identity.ID)
        return workerCtx.executor.HasActionInProgress()
    }).Should(BeTrue())

    // Verify state.Next() not called during action
    nextCallsBefore := worker.nextCallCount
    time.Sleep(1 * time.Second)  // Multiple ticks occur
    nextCallsAfter := worker.nextCallCount

    Expect(nextCallsAfter).To(Equal(nextCallsBefore))
})
```

## CI Requirements

All actions MUST have idempotency tests. CI fails if any are missing:

```bash
make check-idempotency-tests
```

See `Makefile` for implementation details.
```

**Effort:** 4 hours
**Deliverable:** Comprehensive testing guide

---

### 2.2 Document Variable Namespaces (2 hours)

**File:** `pkg/fsmv2/types/variables.go`

**Add to godoc:**
```go
// VariableBundle organizes variables into three namespaces:
//
// 1. User Variables (user-defined config from YAML)
//    - Example: {{ .IP }}, {{ .PORT }}, {{ .CustomField }}
//    - Source: User's config.yaml
//    - Validation: Template rendering (undefined variables = error)
//    - Use when: User needs to configure worker behavior
//
// 2. Global Variables (system-wide computed values)
//    - Example: {{ .location_path }}, {{ .instance_id }}
//    - Source: Agent location + worker location merge
//    - Computed by: DeriveDesiredState() implementation
//    - Use when: All workers need same system-wide value
//
// 3. Internal Variables (supervisor-only state)
//    - Example: Retry counters, circuit breaker state
//    - Source: Supervisor internal state
//    - Never exposed to templates or user config
//    - Use when: Supervisor needs to track operational state
//
// Example:
//   bundle := VariableBundle{
//       User: map[string]any{
//           "IP":   "192.168.1.100",  // From user YAML
//           "PORT": 502,               // From user YAML
//       },
//       Global: map[string]any{
//           "location_path": "enterprise.site.area",  // Computed
//           "instance_id":   "abc-123",               // System-assigned
//       },
//       Internal: map[string]any{
//           "retry_count": 3,  // Supervisor tracking
//       },
//   }
//
//   flattened := bundle.Flatten()  // For template rendering
//   // Result: { "IP": "192.168.1.100", "PORT": 502, "location_path": "...", "instance_id": "..." }
//   // Note: Internal variables NOT included in flattened output
type VariableBundle struct {
    User     map[string]any
    Global   map[string]any
    Internal map[string]any
}
```

**Effort:** 2 hours

---

### 2.3 Wire Up Hierarchy Depth Metrics (3-6 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Add to Tick() method:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    tickStart := time.Now()

    // ... existing tick logic ...

    // NEW: Record tick propagation depth (if hierarchy exists)
    if len(s.children) > 0 {
        depth := s.calculateHierarchyDepth()
        metrics.RecordTickPropagationDepth(s.workerType, depth)

        // Warn if hierarchy too deep (performance concern)
        if depth > 6 {
            s.logger.Warnf("Hierarchy depth %d exceeds recommended limit of 6 levels", depth)
        }
    }

    // NEW: Record tick propagation duration
    duration := time.Since(tickStart)
    metrics.RecordTickPropagationDuration(s.workerType, duration)

    return nil
}

// calculateHierarchyDepth recursively computes max depth
func (s *Supervisor) calculateHierarchyDepth() int {
    if len(s.children) == 0 {
        return 1
    }

    maxChildDepth := 0
    for _, child := range s.children {
        childDepth := child.calculateHierarchyDepth()
        if childDepth > maxChildDepth {
            maxChildDepth = childDepth
        }
    }

    return 1 + maxChildDepth
}
```

**Add to supervisor godoc:**
```go
// Hierarchy Depth Guidelines:
//
// - 1-2 levels: Typical (parent + direct children)
// - 3-4 levels: Advanced (multi-tier orchestration)
// - 5-6 levels: Complex (carefully consider necessity)
// - 7+ levels: Excessive (performance impact, hard to debug)
//
// Deep hierarchies increase tick propagation latency and make
// signal routing more complex. Consider flattening if possible.
```

**Effort:** 3-6 hours
- Implementation: 2-3 hours
- Testing: 1-2 hours
- Documentation: 1 hour

**Deliverable:** Depth metrics active, guidelines documented

---

## Phase 3: Package Consolidation (Weeks 5-8)

**Original restructuring plan preserved**

### 3.1 Create Unified Config Package

**Migration:**
```bash
mkdir -p pkg/fsmv2/config

mv pkg/fsmv2/templating/template.go pkg/fsmv2/config/template.go
mv pkg/fsmv2/templating/template_test.go pkg/fsmv2/config/template_test.go
mv pkg/fsmv2/location/location.go pkg/fsmv2/config/location.go
mv pkg/fsmv2/location/location_test.go pkg/fsmv2/config/location_test.go
mv pkg/fsmv2/types/variables.go pkg/fsmv2/config/variables.go
mv pkg/fsmv2/types/childspec.go pkg/fsmv2/config/childspec.go

# Update package declarations
sed -i '' 's/package templating/package config/' pkg/fsmv2/config/template*.go
sed -i '' 's/package location/package config/' pkg/fsmv2/config/location*.go
sed -i '' 's/package types/package config/' pkg/fsmv2/config/{variables,childspec}.go

rmdir pkg/fsmv2/templating/
rmdir pkg/fsmv2/location/
```

**Effort:** 4-6 hours
**Breaking Change:** Import paths change

---

### 3.2 Update Import Paths

**Find all imports:**
```bash
grep -r "fsmv2/templating" --include="*.go" pkg/
grep -r "fsmv2/location" --include="*.go" pkg/
grep -r "fsmv2/types" --include="*.go" pkg/ | grep -E "(variables|childspec)"
```

**Update:**
```bash
find pkg/ -name "*.go" -exec sed -i '' 's|fsmv2/templating|fsmv2/config|g' {} \;
find pkg/ -name "*.go" -exec sed -i '' 's|fsmv2/location|fsmv2/config|g' {} \;
```

**Manual review for types/ imports** (some files import Snapshot/Identity, keep those)

**Effort:** 6-8 hours
**Breaking Change:** External consumers must update imports

---

### 3.3 Rename api.go

**Migration:**
```bash
git mv pkg/fsmv2/worker.go pkg/fsmv2/api.go
```

**Effort:** 30 minutes
**Impact:** Internal only (file name, not package)

---

### 3.4 Run Full Test Suite

**Commands:** (run from project root)
```bash
make test
go test ./pkg/fsmv2/config/...
go test ./pkg/fsmv2/supervisor/...
go test ./pkg/fsmv2/workers/...
go test ./pkg/fsmv2/integration/...

go build ./pkg/fsmv2/...
```

**Effort:** 2-4 hours (fixing issues found)

---

### 3.5 Update Documentation

**Files:**
- `pkg/fsmv2/README.md` - Update import examples
- `CHANGELOG.md` - Add migration guide
- `pkg/fsmv2/doc.go` - Update package structure

**Migration guide template:**
```markdown
## Migration Guide: v1.x → v2.0

### Import Path Changes

**Before:**
```go
import (
    "fsmv2/templating"
    "fsmv2/location"
    "fsmv2/types"
)
```

**After:**
```go
import (
    "fsmv2/config"  // Merged package
)
```

### API Changes

- `location.FillISA95Gaps()` → `config.NormalizeHierarchyLevels()` (backward-compatible alias exists)
- `types.VariableBundle` → `config.VariableBundle`
- `types.ChildSpec` → `config.ChildSpec`

### Breaking Changes

None. All changes are backward-compatible or import-only.
```

**Effort:** 4-6 hours

---

## Phase 4: ActionExecutor Integration (Weeks 9-12)

**From fsmv2-unused-code-integration.md - Complete async action infrastructure**

### 4.1 Add Executor Field to WorkerContext (1 hour)

**File:** `pkg/fsmv2/supervisor/supervisor.go:198-211`

**Change:**
```go
type WorkerContext struct {
    mu             sync.RWMutex
    tickInProgress atomic.Bool
    identity       fsmv2.Identity
    worker         fsmv2.Worker
    currentState   fsmv2.State
    collector      *collection.Collector
    executor       *execution.ActionExecutor  // NEW
}
```

**Tests:**
- [ ] Verify field is accessible after creation
- [ ] No breaking changes to existing tests

---

### 4.2 Initialize Per-Worker Executors (2-3 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go:450-480` (AddWorker)

**Change:**
```go
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
    // ... existing validation ...

    collector := collection.NewCollector(...)

    // NEW: Create per-worker executor
    executor := execution.NewActionExecutor(execution.ActionExecutorConfig{
        WorkerID:      identity.ID,
        Logger:        s.logger,
        ActionTimeout: 5 * time.Minute,
        MaxRetries:    3,
    })

    s.workers[identity.ID] = &WorkerContext{
        identity:     identity,
        worker:       worker,
        currentState: worker.GetInitialState(),
        collector:    collector,
        executor:     executor,  // NEW
    }

    return nil
}
```

**Tests:**
- [ ] Verify executor created for each worker
- [ ] Verify separate instances per worker

---

### 4.3 Start Executors During Supervisor Start (1-2 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go:520-570` (Start)

**Change:**
```go
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
    // ... existing collector start ...

    for workerID, workerCtx := range s.workers {
        if err := workerCtx.collector.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start collector: %v", err)
        }

        // NEW: Start executor goroutine
        if err := workerCtx.executor.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start executor: %v", err)
        }
    }

    // ... tick loop ...
}
```

**Tests:**
- [ ] Verify executor goroutines running
- [ ] Verify clean shutdown

---

### 4.4 Add Action Blocking Check (2-3 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go:650-720` (tickWorker)

**Change:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... snapshot, freshness check ...

    // NEW: Check if action in progress - skip state.Next() if blocked
    workerCtx.mu.RLock()
    actionInProgress := workerCtx.executor.HasActionInProgress()
    workerCtx.mu.RUnlock()

    if actionInProgress {
        s.logger.Debugf("Worker %s blocked by action: %s",
            workerID, workerCtx.executor.CurrentActionName())
        return nil  // Skip this tick
    }

    // Only call state.Next() if no action blocking
    nextState, signal, action := currentState.Next(*snapshot)

    // ... rest of tick ...
}
```

**Tests:**
- [ ] Verify state.Next() NOT called when action in progress
- [ ] Verify state.Next() IS called when no action

---

### 4.5 Route Actions to Executor (2-3 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go:650-720` (tickWorker, after state.Next())

**Change:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... blocking check, state.Next() ...

    // NEW: Asynchronous execution (enqueue and continue)
    if action != nil {
        if err := workerCtx.executor.EnqueueAction(action); err != nil {
            s.logger.Warnf("Failed to enqueue action: %v", err)
        } else {
            // Record metric
            metrics.RecordActionQueued(s.workerType, action.Name())
        }
    }

    // ... state transition, signal processing ...
}
```

**Tests:**
- [ ] Verify action enqueued (not executed inline)
- [ ] Verify tick continues immediately

---

### 4.6 Remove Old Execution Method (1 hour)

**File:** `pkg/fsmv2/supervisor/supervisor.go:820-880`

**Change:**
```go
// DELETE: executeActionWithRetry() method entirely
// Functionality moved to ActionExecutor
```

**Tests:**
- [ ] Verify no compilation errors
- [ ] Grep for remaining references

---

### 4.7 Add Executor Cleanup (2-3 hours)

**File:** `pkg/fsmv2/supervisor/supervisor.go` (RemoveWorker, Stop)

**Changes:**

**RemoveWorker:**
```go
func (s *Supervisor) RemoveWorker(workerID string) error {
    workerCtx := s.workers[workerID]

    workerCtx.collector.Stop(context.Background())

    // NEW: Stop executor (5s timeout)
    stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    workerCtx.executor.Stop(stopCtx)

    delete(s.workers, workerID)
    return nil
}
```

**Stop:**
```go
func (s *Supervisor) Stop(ctx context.Context) {
    for workerID, workerCtx := range s.workers {
        workerCtx.collector.Stop(ctx)
        workerCtx.executor.Stop(ctx)  // NEW
    }
}
```

**Tests:**
- [ ] Verify executor stops cleanly
- [ ] No goroutine leaks

---

### 4.8 Integration Testing (3-5 days)

**File:** `pkg/fsmv2/supervisor/supervisor_integration_test.go` (new)

**Test scenarios:**
1. Action execution flow (emit → enqueue → execute → complete)
2. Worker blocking during long-running action
3. Action retry with exponential backoff
4. Action timeout handling
5. Concurrent workers with separate executors
6. Cleanup on supervisor stop

**Reference implementation in unused code plan, lines 598-693**

**Effort:** 3-5 days
- Test implementation: 2-3 days
- Integration debugging: 1-2 days

---

### 4.9 ActionExecutor Metrics Integration (1-2 days)

**Files:**
- `pkg/fsmv2/supervisor/execution/action_executor.go`
- `pkg/fsmv2/supervisor/supervisor.go`

**Metrics to wire up:**

**1. RecordActionQueueSize** (in EnqueueAction, actionLoop)
```go
func (e *ActionExecutor) EnqueueAction(action fsmv2.Action) error {
    select {
    case e.actionQueue <- action:
        metrics.RecordActionQueueSize(e.config.WorkerID, 1)  // Queue has 1
        return nil
    default:
        return fmt.Errorf("queue full")
    }
}

// In actionLoop
case action := <-e.actionQueue:
    metrics.RecordActionQueueSize(e.config.WorkerID, 0)  // Queue empty
    e.executeWithRetry(action)
```

**2. RecordActionExecutionDuration** (in executeAction)
```go
func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    startTime := time.Now()

    err := action.Execute(ctx)

    duration := time.Since(startTime)
    status := "success"
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            status = "timeout"
        } else {
            status = "failure"
        }
    }
    metrics.RecordActionExecutionDuration(e.config.WorkerID, action.Name(), status, duration)

    return err
}
```

**3. RecordActionTimeout** (in executeAction when timeout occurs)
```go
if errors.Is(err, context.DeadlineExceeded) {
    metrics.RecordActionTimeout(e.config.WorkerID, action.Name())
    return fmt.Errorf("action timeout: %w", err)
}
```

**4. RecordActionQueued** (already added in Phase 4.5)

**Tests:**
- [ ] Verify all 4 metrics recorded correctly
- [ ] Verify labels (workerID, action name, status)
- [ ] Performance benchmark (overhead < 1μs)

**Effort:** 1-2 days

---

## Success Metrics

### Developer Velocity (Track Before/After)
- Time to implement new worker: Target -20-30%
- Lines of boilerplate per worker: Reduce with helpers
- Time to find test helper: <5 min with TESTING.md
- New developer onboarding: Measure duration

### Code Quality (Automated Enforcement)
- **Idempotency test coverage: 100% (CI enforced)**
- ChildSpec validation errors: Caught before runtime
- Linter violations: 0
- Goroutine leaks: 0 (verified by tests)
- Race conditions: 0 (verified by race detector)

### Documentation Effectiveness (Survey/Feedback)
- "How do I test..." questions: Decrease
- Confusion about patterns: Decrease
- CI failures due to missing tests: Track trend

### Package Organization (Qualitative)
- Package import clarity: Measured by new developer questions
- Dependency graph complexity: Visualize before/after
- Godoc completeness: All public APIs documented

### ActionExecutor Integration (Production Metrics)
- Action execution throughput: Monitor
- Tick latency P99: <10ms (with ActionExecutor)
- Action timeout rate: Monitor in Prometheus
- Queue overflow rate: Should be near 0 (size-1 channel)

### Metrics Integration (Prometheus)
- All 7 unused metrics visible in scrape
- Metric cardinality: Monitor for label explosion
- Recording overhead: <1μs per metric (benchmarked)

---

## Rejected Recommendations

**DO NOT implement** (11 invalid recommendations from investigation):

1. ❌ **Snapshot getters for immutability** - Pass-by-value is correct Go idiom
2. ❌ **Variadic options pattern** - Hallucinated (doesn't exist in source)
3. ❌ **Standardize error signatures** - Contextually appropriate differences
4. ❌ **Type-safe state enums** - Already using typed structs (not strings)
5. ❌ **Rename Snapshot terminology** - Clear in package context
6. ❌ **Add dependency injection** - Already implemented via constructors
7. ❌ **Create BaseWorker pattern** - Already exists (Nov 2, 2025)
8. ❌ **Centralize validation** - Defense-in-depth intentional
9. ❌ **Type VariableBundle with structs** - Breaks user-defined config
10. ❌ **Add StateMapping** - Already implemented and working
11. ❌ **Avoid embedding** - BaseWorker correctly uses embedding

**Rationale:** These recommendations stem from misunderstanding Go idioms (expecting OOP patterns) or missing existing features (BaseWorker, StateMapping already exist).

---

## Appendix A: Investigation Summary

A comprehensive 21-task investigation analyzed external AI recommendations for FSMv2 architecture improvements. Key findings:

**Results:**
- 11 INVALID (based on misunderstandings of Go idioms)
- 2 VALID (idempotency enforcement, ChildSpec validation)
- 6 PARTIALLY VALID (documentation improvements)
- 2 ALREADY IMPLEMENTED (StateMapping, BaseWorker)

**Confusion Patterns:**
- External AI expected OOP patterns (getters/setters) instead of Go idioms (pass-by-value)
- Missed existing features (BaseWorker generics, StateMapping)
- Confused package-level clarity for naming issues
- Proposed solutions before understanding problems

**What FSMv2 Does Well:**
- Pass-by-value immutability (elegant Go idiom)
- Defense-in-depth validation (intentional security)
- Dependency injection (constructor-based)
- Flexible type system (map[string]any for user config)
- Modern Go adoption (generics, go 1.24.4)

**Real Gaps:**
- Idempotency testing enforcement (CI/linting) - CRITICAL
- Documentation of established patterns - HIGH
- Integration test pattern visibility - MEDIUM
- ChildSpec validation layer - MEDIUM

**Lesson:** Confusion reveals documentation gaps, not implementation problems.

---

## Appendix B: Confusion Prevention Mapping

| Confusion Source | Documentation Added | Location |
|------------------|---------------------|----------|
| "Need getters for immutability" | Pass-by-value comment explaining WHY | worker.go:86 |
| "States should be enums" | State types documentation (structs, not strings) | PATTERNS.md |
| "Validation should be centralized" | Defense-in-depth strategy explanation | supervisor.go:245 |
| "VariableBundle should be typed" | Why map[string]any is correct (user-defined) | variables.go:23 |
| "StateMapping doesn't exist" | Improved godoc + example | childspec.go:45 |
| "BaseWorker pattern missing" | Prominent "USING GENERICS" comment | worker_base.go:23 |
| "Variable tiers unclear" | Three-tier namespace documentation | variables.go godoc |
| "State naming inconsistent" | TryingTo vs gerund convention | PATTERNS.md |
| "How to test actions?" | Complete TESTING.md guide | TESTING.md |
| "How deep can hierarchies go?" | Depth guidelines + metrics | supervisor.go godoc |

---

## Appendix C: Phase Dependencies

```
Phase 0 (Quick Wins)
  └─ No dependencies (can start immediately)

Phase 1 (Foundation + Quality Gates)
  ├─ 1.1 Rename function (independent)
  ├─ 1.2 Delete examples (independent)
  ├─ 1.3 Idempotency CI (independent, CRITICAL)
  └─ 1.4 ChildSpec validation (independent)

Phase 2 (Documentation)
  ├─ Depends on: Phase 0 (PATTERNS.md referenced)
  ├─ 2.1 TESTING.md (references Phase 1.3 CI check)
  ├─ 2.2 Variable docs (independent)
  └─ 2.3 Metrics wiring (independent)

Phase 3 (Package Consolidation)
  ├─ Depends on: Phase 1.1 (FillISA95Gaps renamed)
  ├─ Depends on: Phase 1.2 (examples/ deleted)
  ├─ 3.1-3.2 Config package (core restructuring)
  ├─ 3.3 Rename api.go (independent)
  └─ 3.4-3.5 Testing + docs (depends on 3.1-3.3)

Phase 4 (ActionExecutor)
  ├─ Depends on: Phase 3 complete (clean package structure)
  ├─ 4.1-4.7 ActionExecutor integration (sequential)
  ├─ 4.8 Integration tests (depends on 4.1-4.7)
  └─ 4.9 Metrics integration (depends on 4.1-4.7)
```

**Critical Path:** Phase 1.3 (idempotency CI) → Phase 4.1-4.8 (ActionExecutor) → Phase 4.9 (metrics)

---

## Appendix D: Effort Summary

| Phase | Tasks | Total Effort | Priority |
|-------|-------|--------------|----------|
| Phase 0 | 3 | 8 hours | MEDIUM |
| Phase 1 | 4 | 5-8 days | CRITICAL |
| Phase 2 | 3 | 9-14 hours | HIGH |
| Phase 3 | 5 | 16-24 hours | MEDIUM |
| Phase 4 | 9 | 10-18 days | HIGH |
| **TOTAL** | **24** | **18-32 days** | |

**Timeline:** 12-16 weeks (3-4 engineering months) at 2-3 days/week

---

## Appendix E: Key Files Reference

**Core Files:**
- `pkg/fsmv2/api.go` (Worker interface)
- `pkg/fsmv2/supervisor/supervisor.go` (Orchestrator)
- `pkg/fsmv2/supervisor/execution/action_executor.go` (Async actions)

**Test Infrastructure:**
- `pkg/fsmv2/supervisor/execution/action_test_helpers_test.go` (VerifyActionIdempotency)
- `pkg/fsmv2/supervisor/testing.go` (TestWorker, CreateTestTriangularStore)

**Metrics:**
- `pkg/fsmv2/supervisor/metrics/metrics.go` (10 unused metrics)
- `pkg/fsmv2/supervisor/METRICS.md` (Metrics specification)

**Design Documents:**
- `docs/design/fsmv2-async-action-executor.md` (ActionExecutor design)
- `docs/design/fsmv2-infrastructure-supervision-patterns.md` (Supervision patterns)

**Planning Documents:**
- `docs/plans/fsmv2-architecture-investigation-findings.md` (Investigation results)
- `docs/plans/fsmv2-unused-code-integration.md` (ActionExecutor integration)

---

## Next Steps

1. **Review with team** - Validate priorities and timeline
2. **Start Phase 0** - Quick documentation wins (8 hours)
3. **Implement Phase 1.3** - Idempotency CI (CRITICAL, 7 hours)
4. **Execute remaining phases** - Follow dependency graph
5. **Measure success metrics** - Track before/after improvements

**Recommendation:** Execute Phase 0 + Phase 1.3 immediately (total 15 hours) for maximum impact with minimal risk.

---

**Document Status:** Complete and actionable
**Owner:** FSMv2 Team
**Review Date:** 2025-01-08
