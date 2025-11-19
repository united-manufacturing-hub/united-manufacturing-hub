# Plan: Generic Root Supervisor Implementation

## Metadata
- **Version**: 2.1.0
- **Created**: 2025-11-19
- **Author**: Claude Code (Coordinator)
- **Status**: Ready for Implementation
- **Last Updated**: 2025-11-19 (Simplified - use existing examples, skip Erlang patterns)
- **Estimated Time**: 8-12 hours

## Overview

### Problem Statement
FSM v2 needs a production-ready, generic root supervisor that can dynamically manage any registered worker types based on configuration. The current approach of hardcoding child types in examples doesn't scale and isn't suitable for production use. This implementation will provide a reusable root supervisor that parses YAML configuration to determine its children dynamically.

### Architecture Decision: Option C - Passthrough Root Worker
After analysis, we're implementing Option C: a generic passthrough root worker that:
1. Lives in production code (`/pkg/fsmv2/root/`) not examples
2. Parses YAML config to extract `children:` array
3. Passes through ChildrenSpecs without hardcoding types
4. Allows any registered worker type to be a child
5. Supports Erlang-inspired supervision patterns

### Key Insight - The Passthrough Pattern
The root worker doesn't need to know about child types. It simply:
1. Receives YAML config containing a `children:` array
2. Parses this into ChildSpec objects
3. Returns them from GetChildrenSpecs()
4. The supervisor framework handles the rest

### Target Structure
```
pkg/fsmv2/root/                              # Production code (NEW)
├── worker.go                                # Generic passthrough root worker
├── types.go                                 # Minimal state types
├── setup.go                                 # NewRootSupervisor() helper
├── factory_registration.go                  # Auto-registration
├── config.go                                # Config parsing utilities
└── root_test.go                             # Unit tests

pkg/fsmv2/workers/example/root_supervisor/   # Usage example only (SIMPLIFIED)
├── README.md                                # How to use the generic root
├── setup.go                                 # Example setup showing configuration
└── example_test.go                          # Example integration test

pkg/fsmv2/examples/parent_child/             # TO BE DELETED
```

---

## Note: Erlang-Inspired Patterns (Future Work)

The following Erlang/OTP patterns are documented for future implementation but **NOT included in this plan**:
- Restart strategies (one_for_one, one_for_all, rest_for_one)
- Child restart types (permanent, temporary, transient)
- Intensity/period for restart limiting

These can be added later once the basic generic root supervisor is working.

---

## Passthrough Root Worker Pattern

The core implementation pattern:

```go
func (w *PassthroughWorker) DeriveDesiredState(userSpec interface{}) (fsmv2.DesiredState, error) {
    spec := userSpec.(config.UserSpec)

    var children struct {
        Children []config.ChildSpec `yaml:"children"`
    }

    if err := yaml.Unmarshal([]byte(spec.Config), &children); err != nil {
        return nil, fmt.Errorf("failed to parse children config: %w", err)
    }

    return &PassthroughDesiredState{
        DesiredState: config.DesiredState{
            State:         "running",
            ChildrenSpecs: children.Children,
        },
    }, nil
}
```

---

## Phase 1: Production Root Package (pkg/fsmv2/root/)

### Task 1: Create Root Package Structure with Types

**Objective**: Create the production root package with minimal state types for the passthrough pattern.

**Subagent Prompt**:
```
You are implementing Task 1 from this plan.

Read this task carefully. Your job is to:
1. Create pkg/fsmv2/root/ directory structure
2. Create types.go with PassthroughObservedState, PassthroughDesiredState
3. These are MINIMAL types - just enough to satisfy interfaces
4. Include WorkerType() returning "root" consistently
5. Write compile-time verification tests
6. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Files to create:
- pkg/fsmv2/root/types.go
- pkg/fsmv2/root/types_test.go

Key insight: PassthroughDesiredState embeds config.DesiredState to get ChildrenSpecs:

type PassthroughDesiredState struct {
    config.DesiredState
    Name string
}

func (s *PassthroughDesiredState) GetChildrenSpecs() []config.ChildSpec {
    return s.ChildrenSpecs
}

Test should verify:
- var _ fsmv2.ObservedState = (*PassthroughObservedState)(nil)
- var _ fsmv2.DesiredState = (*PassthroughDesiredState)(nil)

Report: What you implemented, test results (should FAIL initially), files changed
```

**Success Criteria**:
- [ ] types.go compiles
- [ ] types_test.go initially fails (interface not satisfied)
- [ ] PassthroughDesiredState embeds config.DesiredState

---

### Task 2: Implement Passthrough Worker

**Objective**: Create the generic passthrough worker that parses YAML config for children.

**Subagent Prompt**:
```
You are implementing Task 2 from this plan.

Read this task carefully. Your job is to:
1. Create worker.go with PassthroughWorker struct
2. Implement DeriveDesiredState() that parses YAML for children array
3. Implement GetObservedState() and ReconcileState()
4. The key is: it doesn't know child types, just passes through ChildSpec
5. Add worker interface verification tests
6. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Files to create:
- pkg/fsmv2/root/worker.go
- pkg/fsmv2/root/worker_test.go

Core pattern (the heart of the passthrough design):

func (w *PassthroughWorker) DeriveDesiredState(userSpec interface{}) (fsmv2.DesiredState, error) {
    spec := userSpec.(config.UserSpec)

    var children struct {
        Children []config.ChildSpec `yaml:"children"`
    }

    if err := yaml.Unmarshal([]byte(spec.Config), &children); err != nil {
        return nil, fmt.Errorf("failed to parse children config: %w", err)
    }

    return &PassthroughDesiredState{
        DesiredState: config.DesiredState{
            State:         "running",
            ChildrenSpecs: children.Children,
        },
    }, nil
}

Tests should verify:
- var _ fsmv2.Worker = (*PassthroughWorker)(nil)
- YAML parsing works correctly
- Empty children array is valid
- Invalid YAML returns error

Report: What you implemented, test results (should FAIL initially), files changed
```

**Success Criteria**:
- [ ] worker.go compiles
- [ ] YAML parsing logic implemented
- [ ] Tests verify passthrough behavior

---

### Task 3: Create Setup Helper and Factory Registration

**Objective**: Create the public API for creating root supervisors and register the passthrough worker type.

**Subagent Prompt**:
```
You are implementing Task 3 from this plan.

Read this task carefully. Your job is to:
1. Create setup.go with NewRootSupervisor() helper function
2. Create factory_registration.go with init() for auto-registration
3. NewRootSupervisor should take config and return ready-to-use supervisor
4. Document the pattern clearly in GoDoc
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Files to create:
- pkg/fsmv2/root/setup.go
- pkg/fsmv2/root/factory_registration.go

NewRootSupervisor pattern:

// NewRootSupervisor creates a supervisor with a passthrough root worker.
// The root worker parses YAML config to dynamically discover child workers.
// Child workers are created automatically via reconcileChildren() based on
// the ChildrenSpecs in the config.
//
// Example config:
//   children:
//     - name: "worker-1"
//       workerType: "counter"
//       config: |
//         initialValue: 0
//     - name: "worker-2"
//       workerType: "timer"
//       config: |
//         interval: 1s
func NewRootSupervisor(cfg RootSupervisorConfig) (*supervisor.Supervisor[...], error) {
    // Create supervisor
    // Create passthrough root worker
    // Call AddWorker() for root
    // Return ready supervisor
}

Report: What you implemented, test results, files changed
```

**Success Criteria**:
- [ ] NewRootSupervisor() function works
- [ ] Factory registration auto-registers on import
- [ ] GoDoc clearly explains the pattern

---

### Task 4: Create Integration Tests for Root Package

**Objective**: Write comprehensive tests for the production root supervisor using existing example workers.

**Subagent Prompt**:
```
You are implementing Task 4 from this plan.

Read this task carefully. Your job is to:
1. Create root_test.go with Ginkgo v2 test structure
2. Test YAML parsing with various child configurations
3. Test supervisor lifecycle using existing example-child worker type
4. Test error cases (invalid YAML, unregistered worker types)
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

File to create:
- pkg/fsmv2/root/root_test.go

Test scenarios to include:
1. "should parse children from YAML config"
2. "should create children using example-child worker type"
3. "should handle empty children array"
4. "should return error for invalid YAML"
5. "should propagate children specs to supervisor"

Key test pattern - use EXISTING example-child:

import (
    // Import to trigger registration
    _ "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

config := `
children:
  - name: "child-1"
    workerType: "example-child"  # Uses existing worker type
    config: |
      someValue: 10
  - name: "child-2"
    workerType: "example-child"
    config: |
      someValue: 20
`
// Uses the existing example-child worker type
// Root doesn't know about this type - just passes it through

Report: What you implemented, test results (should FAIL), files changed
```

**Success Criteria**:
- [ ] Test file compiles and runs
- [ ] Tests use existing example-child worker type
- [ ] Error cases tested

---

### Code Review Gate: Phase 1

**Dispatch code-reviewer subagent after Task 4 completion.**

Review focus:
- Passthrough pattern correctly implemented
- No hardcoded child types in root package
- Factory registration is automatic
- Tests use dynamic child types

---

## Phase 2: Cleanup and Example Creation

### Task 5: Make Root Package Tests Pass

**Objective**: Implement all interfaces and make the root package tests pass.

**Subagent Prompt**:
```
You are implementing Task 5 from this plan.

Read this task carefully. Your job is to:
1. Implement all interface methods on types
2. Complete the passthrough worker implementation
3. Make all root_test.go tests pass
4. Ensure YAML parsing works correctly
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Files to modify:
- pkg/fsmv2/root/types.go
- pkg/fsmv2/root/worker.go

Run: go test ./pkg/fsmv2/root/... -v

All tests must pass. The key test is dynamic child creation:
- Root parses YAML
- Returns ChildrenSpecs with various worker types
- Doesn't import or know about child types

Report: What you implemented, test results (should all PASS), files changed
```

**Success Criteria**:
- [ ] All root package tests pass
- [ ] No hardcoded child types
- [ ] YAML parsing works

---

### Task 6: Delete Old Example and Create Usage Example

**Objective**: Remove the old parent_child example and simplify root_supervisor to be a usage example only.

**Subagent Prompt**:
```
You are implementing Task 6 from this plan.

Read this task carefully. Your job is to:
1. DELETE pkg/fsmv2/examples/parent_child/ directory completely
2. SIMPLIFY pkg/fsmv2/workers/example/root_supervisor/ to be example only
3. Remove implementation files (they're now in pkg/fsmv2/root/)
4. Keep only: README.md, setup.go (example), example_test.go
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Actions:
1. rm -rf pkg/fsmv2/examples/parent_child/
2. In pkg/fsmv2/workers/example/root_supervisor/:
   - Delete: types.go, root_worker.go, child_worker.go, factory_registration.go
   - Keep/create: README.md, setup.go (example usage), example_test.go

The example setup.go should IMPORT from pkg/fsmv2/root:

import "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/root"

func ExampleSetup() {
    config := `
children:
  - name: "worker-1"
    workerType: "counter"
    config: |
      initialValue: 0
`
    sup, err := root.NewRootSupervisor(root.Config{
        Config: config,
    })
    // ... use supervisor
}

Report: What you deleted, what you created, files changed
```

**Success Criteria**:
- [ ] pkg/fsmv2/examples/parent_child/ deleted
- [ ] Example only imports from pkg/fsmv2/root
- [ ] Example shows HOW to use, not reimplements

---

### Task 7: Create Complete Example Test

**Objective**: Create example_test.go showing how to use the generic root supervisor with existing example-child workers.

**Subagent Prompt**:
```
You are implementing Task 7 from this plan.

Read this task carefully. Your job is to:
1. Create example_test.go in pkg/fsmv2/workers/example/root_supervisor/
2. Import existing example-child worker (triggers registration)
3. Show complete lifecycle with multiple children
4. This is the canonical example for FSM v2 users
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

File to create:
- pkg/fsmv2/workers/example/root_supervisor/example_test.go

Test structure:

import (
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/root"
    // Import to trigger registration
    _ "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

var _ = Describe("Root Supervisor Example", func() {
    It("should manage dynamically typed children", func() {
        config := `
children:
  - name: "child-1"
    workerType: "example-child"
    config: |
      value: 10
  - name: "child-2"
    workerType: "example-child"
    config: |
      value: 20
`
        sup, err := root.NewRootSupervisor(root.Config{Config: config})
        Expect(err).ToNot(HaveOccurred())

        // Tick to process root and create children
        sup.Tick()

        // Verify children exist
        // ...
    })
})

Report: What you implemented, test results, files changed
```

**Success Criteria**:
- [ ] Example shows multiple children
- [ ] Imports trigger worker registration
- [ ] Test passes and demonstrates pattern

---

### Code Review Gate: Phase 2

**Dispatch code-reviewer subagent after Task 8 completion.**

Review focus:
- Old example deleted
- New example only imports from root package
- Example worker types are simple and clear
- Complete lifecycle demonstrated

---

## Phase 3: Documentation and Verification

### Task 8: Create README and Documentation

**Objective**: Write comprehensive documentation for both the production root package and the example.

**Subagent Prompt**:
```
You are implementing Task 8 from this plan.

Read this task carefully. Your job is to:
1. Create README.md for pkg/fsmv2/root/ (production package)
2. Create README.md for pkg/fsmv2/workers/example/root_supervisor/ (example)
3. Add GoDoc to all exported types and functions
4. Explain the passthrough pattern clearly
5. Commit your work

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Files to create:
- pkg/fsmv2/root/README.md
- pkg/fsmv2/workers/example/root_supervisor/README.md

Root package README sections:
1. Overview - Generic passthrough root supervisor
2. Key Concept - YAML config drives children dynamically
3. API Reference - NewRootSupervisor, Config types
4. Erlang Patterns - Restart strategies, types, intensity
5. Integration with umh-core

Example README sections:
1. Overview - How to use the generic root
2. Quick Start - Run the example
3. Creating Custom Workers - How to add new types
4. Testing - How to test with dynamic children

Report: What you documented, files created
```

**Success Criteria**:
- [ ] Both READMEs created
- [ ] GoDoc on all exports
- [ ] Passthrough pattern explained

---

### Task 9: Code Quality and Final Polish

**Objective**: Ensure code quality and complete final verification.

**Subagent Prompt**:
```
You are implementing Task 9 from this plan.

Read this task carefully. Your job is to:
1. Run go vet, go fmt on all new packages
2. Ensure consistent error message format
3. Add copyright headers to all files
4. Verify no focused tests
5. Run race detector
6. Commit final polish

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Quality checks:
- go vet ./pkg/fsmv2/root/...
- go vet ./pkg/fsmv2/workers/example/...
- go fmt ./pkg/fsmv2/root/...
- go fmt ./pkg/fsmv2/workers/example/...
- golangci-lint run ./pkg/fsmv2/root/...
- golangci-lint run ./pkg/fsmv2/workers/example/...

Run all tests:
- go test ./pkg/fsmv2/root/... -v -race
- go test ./pkg/fsmv2/workers/example/... -v -race

Report: Quality check results, any fixes made, final test results
```

**Success Criteria**:
- [ ] All linters pass
- [ ] Race detector passes
- [ ] No focused tests

---

### Task 10: Final Verification and Summary

**Objective**: Complete final verification against all success criteria.

**Subagent Prompt**:
```
You are implementing Task 10 from this plan.

Read this task carefully. Your job is to:
1. Verify pkg/fsmv2/examples/parent_child/ is deleted
2. Verify pkg/fsmv2/root/ has complete implementation
3. Verify example imports from root package only
4. Run full test suite
5. Create implementation summary
6. Commit any final fixes

Work from: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Verification checklist:
- [ ] pkg/fsmv2/examples/parent_child/ does NOT exist
- [ ] pkg/fsmv2/root/ exists with all files
- [ ] Example has no hardcoded types
- [ ] Dynamic child creation works
- [ ] All tests pass
- [ ] Documentation complete

Summary should include:
- Files created
- Files deleted
- Key patterns implemented
- How to use in umh-core

Report: Full verification results, implementation summary
```

**Success Criteria**:
- [ ] All verification checks pass
- [ ] Summary created
- [ ] Ready for production use

---

### Code Review Gate: Final

**Dispatch final code-reviewer subagent after Task 10 completion.**

Review entire implementation:
- Option C architecture correctly implemented
- Old examples cleaned up
- Production code in pkg/fsmv2/root/
- Example only shows usage
- Uses existing example-child workers
- Ready for umh-core integration

---

## Success Criteria

### Architecture Requirements
- [ ] Production code in pkg/fsmv2/root/ (not examples)
- [ ] Generic passthrough pattern - no hardcoded child types
- [ ] YAML config dynamically drives children
- [ ] pkg/fsmv2/examples/parent_child/ deleted
- [ ] Example only imports from production package

### Functional Requirements
- [ ] Root supervisor creates and manages ANY registered worker type
- [ ] Root worker parses YAML config for children array
- [ ] Children created automatically from ChildrenSpecs
- [ ] Factory registration is automatic on import
- [ ] Proper cleanup on shutdown

### Documentation Requirements
- [ ] README for pkg/fsmv2/root/ (production)
- [ ] README for example (usage demonstration)
- [ ] GoDoc on all exported items
- [ ] Passthrough pattern clearly explained

### Test Requirements
- [ ] All tests pass
- [ ] Race detector passes
- [ ] Tests use existing example-child worker type
- [ ] Error cases covered (invalid YAML, unregistered types)

---

## Risk Mitigation

### Risk 1: Passthrough Pattern Breaks Type Safety
**Mitigation**: Use YAML parsing with clear error messages. Test invalid configs extensively. Factory validates worker types at creation time.

### Risk 2: Erlang Patterns Require Supervisor Changes
**Mitigation**: Implement patterns in root worker where possible. Document supervisor limitations. Mark as "future enhancement" where needed.

### Risk 3: Example Imports Create Dependency Issues
**Mitigation**: Example only imports from pkg/fsmv2/root/. Test worker types (counter, timer) are separate packages with their own factory registration.

### Risk 4: YAML Parsing Performance
**Mitigation**: Parse YAML once in DeriveDesiredState. Cache results in desired state. Only re-parse on config changes.

### Risk 5: Breaking Existing Users
**Mitigation**: This is new code, no existing users. Ensure backward compatibility for any Erlang patterns via sensible defaults.

---

## Time Estimates

| Phase | Tasks | Estimated Time |
|-------|-------|----------------|
| Phase 1 (Production Root) | 1-4 | 4-5 hours |
| Phase 2 (Cleanup & Example) | 5-7 | 2-3 hours |
| Phase 3 (Docs & Verification) | 8-10 | 2-3 hours |
| **Total** | 10 | **8-12 hours** |

---

## Changelog

### v2.1.0 (2025-11-19)
- **Simplified**: Remove counter/timer examples - use existing example-child workers
- **Simplified**: Remove Erlang patterns (tasks 9-11) - future work
- Reduced from 14 to 10 tasks across 3 phases
- Estimated time reduced to 8-12 hours

### v2.0.0 (2025-11-19)
- **Architecture redesign**: Option C - Passthrough Root Worker
- Production code moved to pkg/fsmv2/root/
- Delete pkg/fsmv2/examples/parent_child/
- Example simplified to usage demonstration only
- Added Erlang-inspired supervision patterns
- 14 tasks across 4 phases
- Estimated time increased to 15-20 hours

### v1.0.0 (2025-11-19)
- Initial plan creation
- Hardcoded child types (now obsolete)
- 15 tasks across 4 phases
- Based on FSM v2 test fix insights
