# FSMv2 Package Restructuring Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Reorganize FSMv2 package structure to reduce cognitive load for new developers by creating clear subsystem boundaries.

**Architecture:** Move from flat package structure to hierarchical organization with focused subdirectories for each major subsystem (health checking, action execution, data collection, metrics).

**Tech Stack:** Go 1.21+, existing FSMv2 codebase, git for version control

**Created:** 2025-11-06 13:00

**Last Updated:** 2025-11-06 13:00

---

## 1. Executive Summary

### Why We're Restructuring

The FSMv2 package has evolved from a simple proof-of-concept into a production-grade hierarchical state machine framework with multiple sophisticated subsystems. The current flat structure makes it difficult for new developers to:

- Understand which files implement which subsystem
- Navigate between related components
- Recognize clear boundaries between concerns
- Learn the architecture through directory structure

### What Problem It Solves

**Current pain points:**

1. **Cognitive overload**: 25+ files in `pkg/fsmv2/supervisor/` with no visual grouping
2. **Unclear boundaries**: Health checking, action execution, data collection, and metrics code all intermixed
3. **Navigation difficulty**: Finding "where do I add a metric?" requires grep or mental mapping
4. **Onboarding friction**: New developers must read extensive documentation to understand structure

**After restructuring:**

1. **Self-documenting structure**: Directory names explain purpose (`health/`, `execution/`, `collection/`)
2. **Clear subsystem boundaries**: Each directory owns one responsibility
3. **Easier navigation**: "Need to add a metric?" → `pkg/fsmv2/supervisor/metrics/`
4. **Reduced cognitive load**: See 5 directories instead of 25 files at top level

### High-Level Goals

1. **Zero breakage**: All 29 test files must pass after migration
2. **Clear boundaries**: Each subsystem in its own directory with clear imports
3. **Backward compatibility**: Maintain existing public APIs (no external consumer impact)
4. **Documentation**: Update examples to demonstrate new structure
5. **Maintainability**: Set foundation for future subsystem additions

---

## 2. Current State Analysis

### Current Package Structure

```
pkg/fsmv2/
├── communicator/              # Communicator worker implementation
│   ├── action/                # Actions (authenticate, sync)
│   ├── registry/              # Registry implementation
│   ├── snapshot/              # Snapshot management
│   ├── state/                 # FSM states (stopped, syncing, degraded, etc.)
│   ├── transport/             # Transport layer (HTTP)
│   │   └── http/
│   ├── integration_test.go
│   ├── mocks_test.go
│   ├── registry.go
│   ├── registry_test.go
│   ├── worker.go
│   └── worker_test.go
├── supervisor/                # Supervisor implementation (FLAT - 25 files)
│   ├── action_helpers_test.go
│   ├── action_idempotency_test.go
│   ├── collector.go           # Data collection subsystem
│   ├── collector_test.go
│   ├── collector_workertype_test.go
│   ├── config_test.go
│   ├── constants.go
│   ├── edge_cases_test.go
│   ├── freshness.go           # Health checking subsystem
│   ├── freshness_test.go
│   ├── immutability_test.go
│   ├── integration_multi_worker_test.go
│   ├── integration_test.go
│   ├── lifecycle_test.go
│   ├── multi_worker_test.go
│   ├── supervisor.go          # Core supervisor + action execution
│   ├── supervisor_suite_test.go
│   ├── supervisor_test.go
│   ├── tick_test.go
│   └── (several .go.disabled files)
├── registry.go                # Worker registry
├── registry_test.go
├── worker.go                  # Worker interface
├── worker_helpers.go
└── worker_helpers_test.go
```

### Files in Each Directory

**pkg/fsmv2/ (top-level):**
- `registry.go` (WorkerFactory pattern)
- `registry_test.go`
- `worker.go` (Worker interface, Identity, Snapshot types)
- `worker_helpers.go` (DeriveDesiredState, utility functions)
- `worker_helpers_test.go`

**pkg/fsmv2/supervisor/ (25 files):**

| File | Lines | Subsystem | Purpose |
|------|-------|-----------|---------|
| `supervisor.go` | 1,136 | Core + Execution | Main supervisor logic, action execution |
| `collector.go` | 231 | Collection | Observation data collection loop |
| `freshness.go` | 161 | Health | Data freshness validation |
| `constants.go` | 99 | Core | Shared constants |
| Various tests | ~4,600 | All | Comprehensive test coverage |

**pkg/fsmv2/communicator/ (structured):**
- Already well-organized into `action/`, `state/`, `transport/`, `registry/`, `snapshot/`
- This structure should be preserved and serves as a model

### Current Import Paths

**Internal imports within FSMv2:**
```go
import "github.com/.../umh-core/pkg/fsmv2"
import "github.com/.../umh-core/pkg/fsmv2/supervisor"
import "github.com/.../umh-core/pkg/fsmv2/communicator"
import "github.com/.../umh-core/pkg/fsmv2/communicator/state"
```

**External consumers (if any):**
- Must verify no external code imports `pkg/fsmv2/supervisor` directly
- Most usage should be through `pkg/fsmv2` top-level types

### Pain Points Identified

1. **Flat supervisor directory**: 25 files with no grouping makes navigation difficult
2. **Unclear subsystem boundaries**: Health, collection, execution logic all in same directory
3. **Cognitive load**: Must read extensive comments/docs to understand which file does what
4. **Inconsistent structure**: Communicator is well-organized, supervisor is flat
5. **Difficult to extend**: Adding new subsystem (e.g., caching) unclear where it belongs

---

## 3. Proposed Structure

### New Directory Layout

```
pkg/fsmv2/
├── supervisor/
│   ├── health/                # Health checking subsystem
│   │   ├── freshness.go           # FreshnessChecker implementation
│   │   ├── freshness_test.go      # Freshness validation tests
│   │   └── circuit.go             # Future: Circuit breaker logic (Phase 1)
│   ├── execution/             # Action execution subsystem
│   │   ├── executor.go            # Action execution with retry logic
│   │   ├── executor_test.go       # Action idempotency tests
│   │   ├── action_helpers_test.go # Helper function tests
│   │   └── timeouts.go            # Future: Timeout handling
│   ├── collection/            # Data collection subsystem
│   │   ├── collector.go           # Observation collector implementation
│   │   ├── collector_test.go      # Collection loop tests
│   │   ├── collector_workertype_test.go # Worker type validation
│   │   └── config.go              # CollectorConfig types
│   ├── metrics/               # Observability subsystem
│   │   ├── metrics.go             # Prometheus metrics definitions
│   │   ├── metrics_test.go        # Metrics recording tests
│   │   └── labels.go              # Metric label constants
│   ├── supervisor.go          # Core supervisor (orchestrates subsystems)
│   ├── supervisor_test.go     # Core supervisor tests
│   ├── config.go              # Supervisor configuration types
│   ├── config_test.go         # Configuration validation tests
│   ├── constants.go           # Shared constants
│   ├── integration_test.go    # Integration tests
│   ├── integration_multi_worker_test.go
│   ├── multi_worker_test.go
│   ├── lifecycle_test.go
│   ├── edge_cases_test.go
│   ├── immutability_test.go
│   ├── tick_test.go
│   └── supervisor_suite_test.go
├── workers/                   # Worker implementations
│   └── communicator/          # Move from top-level communicator/
│       ├── action/
│       ├── registry/
│       ├── snapshot/
│       ├── state/
│       ├── transport/
│       ├── worker.go
│       └── (all existing files)
├── examples/                  # Learning materials
│   ├── simple_worker/         # Minimal worker implementation
│   │   ├── main.go
│   │   └── README.md
│   ├── hierarchical/          # Parent-child example
│   │   ├── main.go
│   │   └── README.md
│   └── communicator_usage/    # How to use communicator worker
│       ├── main.go
│       └── README.md
├── registry.go                # (stays at top level - FSM-specific utility)
├── registry_test.go
├── worker.go                  # (stays at top level - core interfaces)
├── worker_helpers.go
└── worker_helpers_test.go
```

### Rationale for Each Subdirectory

**supervisor/health/**
- **Purpose**: Health checking and infrastructure monitoring
- **Contents**: Data freshness validation, circuit breaker (future)
- **Why separate**: Health checking is a distinct concern with clear responsibility
- **Import by**: Supervisor core for data validation before state transitions

**supervisor/execution/**
- **Purpose**: Action execution with retry logic and timeout handling
- **Contents**: Action execution, retry loops, timeout management
- **Why separate**: Action execution has complex retry/timeout logic that deserves isolation
- **Import by**: Supervisor core for executing actions requested by workers

**supervisor/collection/**
- **Purpose**: Observation data collection lifecycle
- **Contents**: Collector goroutine management, observation loops, type validation
- **Why separate**: Collection is a self-contained subsystem with its own lifecycle
- **Import by**: Supervisor core for starting/stopping collectors per worker

**supervisor/metrics/**
- **Purpose**: Prometheus metrics and observability
- **Contents**: Metric definitions, recording functions, label management
- **Why separate**: Metrics are cross-cutting but should be centralized for discoverability
- **Import by**: All subsystems for recording operational metrics

**workers/communicator/**
- **Purpose**: Communicator worker implementation (example/reference)
- **Why move**: Clarifies that communicator is a worker implementation, not part of FSM framework
- **Benefit**: New developers immediately understand "workers implement the FSM interface"

**examples/**
- **Purpose**: Learning materials demonstrating FSM usage
- **Why separate**: Examples should be discoverable and self-contained
- **Benefit**: New developers can run examples to understand framework usage

### Why NOT Create More Directories

**Considered but rejected:**

1. **supervisor/core/**: Would make `supervisor/` package structure more complex without benefit
2. **supervisor/state/**: State management is intrinsic to supervisor core, not a subsystem
3. **supervisor/hierarchy/**: Hierarchical composition is part of core supervisor logic
4. **supervisor/config/**: Configuration is small enough to stay in top-level `config.go`

**Principle**: Only create directories for subsystems with clear boundaries and multiple related files.

---

## 4. Migration Plan - 9 Tasks

### Task 1: Create Subdirectories and Move Health Files

**Objective**: Establish `supervisor/health/` subdirectory and migrate freshness checking code.

**Files to create:**
- `pkg/fsmv2/supervisor/health/` (directory)

**Files to move:**
- `pkg/fsmv2/supervisor/freshness.go` → `pkg/fsmv2/supervisor/health/freshness.go`
- `pkg/fsmv2/supervisor/freshness_test.go` → `pkg/fsmv2/supervisor/health/freshness_test.go`

**Import path changes:**
- Internal: Update `supervisor.go` to import `health` subdirectory
- External: None (health package not exposed publicly)

**Test verification:**
```bash
# Run health subsystem tests
go test ./pkg/fsmv2/supervisor/health/ -v

# Run full supervisor suite
go test ./pkg/fsmv2/supervisor/... -v

# Verify no import errors
go build ./pkg/fsmv2/...
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify test coverage maintained
- Check for unintended import changes

**Commit message:**
```
refactor(fsmv2): move health checking to supervisor/health/

Part of FSMv2 package restructuring (Task 1/9).

Moves freshness checking logic into dedicated health/ subdirectory
to establish clear subsystem boundaries.

Files moved:
- freshness.go → health/freshness.go
- freshness_test.go → health/freshness_test.go

No functional changes. All tests pass.
```

---

### Task 2: Create Execution Subdirectory and Move Action Files

**Objective**: Establish `supervisor/execution/` subdirectory and migrate action execution logic.

**Files to create:**
- `pkg/fsmv2/supervisor/execution/` (directory)
- `pkg/fsmv2/supervisor/execution/executor.go` (extract from supervisor.go)

**Files to move:**
- `pkg/fsmv2/supervisor/action_helpers_test.go` → `pkg/fsmv2/supervisor/execution/action_helpers_test.go`
- `pkg/fsmv2/supervisor/action_idempotency_test.go` → `pkg/fsmv2/supervisor/execution/action_idempotency_test.go`

**Code extraction:**

Extract from `supervisor.go` into `execution/executor.go`:
```go
// Functions to extract:
// - executeActionWithRetry() and related retry logic
// - Action execution helper functions
// - Retry backoff logic
```

Keep in `supervisor.go`:
```go
// Functions to keep:
// - Tick() and TickAll() (core orchestration)
// - AddWorker() / RemoveWorker() (worker management)
// - Start() / Stop() (lifecycle)
// - State management functions
```

**Import path changes:**
- Update `supervisor.go` to import `execution` subdirectory
- Add `execution.Executor` field to Supervisor struct
- Wire executor in `NewSupervisor()`

**Test verification:**
```bash
# Run execution subsystem tests
go test ./pkg/fsmv2/supervisor/execution/ -v

# Run full supervisor suite
go test ./pkg/fsmv2/supervisor/... -v

# Verify action execution still works
go test ./pkg/fsmv2/supervisor/ -run TestActionIdempotency -v
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify action retry logic unchanged
- Check for performance regressions

**Commit message:**
```
refactor(fsmv2): move action execution to supervisor/execution/

Part of FSMv2 package restructuring (Task 2/9).

Extracts action execution logic from supervisor.go into dedicated
execution/ subdirectory for clearer subsystem boundaries.

Changes:
- Extract executeActionWithRetry() → execution/executor.go
- Move action test files to execution/
- Wire execution.Executor into Supervisor

No functional changes. All tests pass.
```

---

### Task 3: Create Collection Subdirectory and Move Collector Files

**Objective**: Establish `supervisor/collection/` subdirectory and migrate data collection code.

**Files to create:**
- `pkg/fsmv2/supervisor/collection/` (directory)

**Files to move:**
- `pkg/fsmv2/supervisor/collector.go` → `pkg/fsmv2/supervisor/collection/collector.go`
- `pkg/fsmv2/supervisor/collector_test.go` → `pkg/fsmv2/supervisor/collection/collector_test.go`
- `pkg/fsmv2/supervisor/collector_workertype_test.go` → `pkg/fsmv2/supervisor/collection/collector_workertype_test.go`

**Import path changes:**
- Update `supervisor.go` to import `collection` subdirectory
- Update `Collector` instantiation in `AddWorker()`
- Update collector restart logic to use `collection.Collector`

**Test verification:**
```bash
# Run collection subsystem tests
go test ./pkg/fsmv2/supervisor/collection/ -v

# Run collector integration tests
go test ./pkg/fsmv2/supervisor/ -run TestCollector -v

# Verify observation loops work
go test ./pkg/fsmv2/supervisor/ -run TestIntegration -v
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify collector lifecycle unchanged
- Check observation interval timing

**Commit message:**
```
refactor(fsmv2): move data collection to supervisor/collection/

Part of FSMv2 package restructuring (Task 3/9).

Moves observation collector implementation into dedicated
collection/ subdirectory for clearer subsystem boundaries.

Files moved:
- collector.go → collection/collector.go
- collector_test.go → collection/collector_test.go
- collector_workertype_test.go → collection/collector_workertype_test.go

No functional changes. All tests pass.
```

---

### Task 4: Create Metrics Subdirectory and Extract Metrics Code

**Objective**: Establish `supervisor/metrics/` subdirectory for centralized observability.

**Files to create:**
- `pkg/fsmv2/supervisor/metrics/` (directory)
- `pkg/fsmv2/supervisor/metrics/metrics.go` (extract from various files)
- `pkg/fsmv2/supervisor/metrics/metrics_test.go` (new tests)
- `pkg/fsmv2/supervisor/metrics/labels.go` (metric label constants)

**Code extraction:**

Extract from `supervisor.go` and other files:
```go
// Prometheus metric definitions
// Metric recording functions
// Label constant definitions
```

Create new `metrics/metrics.go`:
```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    // Metric definitions
    CollectorRestarts prometheus.Counter
    ActionDuration    prometheus.Histogram
    // ... all FSMv2 metrics
)

func RecordCollectorRestart(workerID string) {
    // Increment counter with labels
}

func RecordActionExecution(action string, duration time.Duration) {
    // Record histogram
}
```

**Import path changes:**
- All subsystems import `metrics` for recording
- Remove metric definitions from individual files
- Centralize metric registration in `metrics.go`

**Test verification:**
```bash
# Run metrics tests
go test ./pkg/fsmv2/supervisor/metrics/ -v

# Verify metrics still recorded
go test ./pkg/fsmv2/supervisor/... -v

# Check Prometheus scrape endpoint
go run examples/simple_worker/main.go
curl localhost:2112/metrics | grep fsmv2
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify all metrics still recorded
- Check metric naming consistency

**Commit message:**
```
refactor(fsmv2): centralize metrics in supervisor/metrics/

Part of FSMv2 package restructuring (Task 4/9).

Extracts Prometheus metrics into dedicated metrics/ subdirectory
for better discoverability and maintainability.

Changes:
- Create metrics/metrics.go with all FSMv2 metrics
- Create metrics/labels.go for label constants
- Add metrics/metrics_test.go for validation

No functional changes. All metrics still recorded.
```

---

### Task 5: Move Communicator to workers/ Subdirectory

**Objective**: Clarify that communicator is a worker implementation by moving to `workers/` namespace.

**Files to create:**
- `pkg/fsmv2/workers/` (directory)
- `pkg/fsmv2/workers/communicator/` (directory)

**Files to move:**
- `pkg/fsmv2/communicator/*` → `pkg/fsmv2/workers/communicator/*`
- All subdirectories preserved: `action/`, `registry/`, `snapshot/`, `state/`, `transport/`

**Import path changes:**

**Before:**
```go
import "github.com/.../umh-core/pkg/fsmv2/communicator"
import "github.com/.../umh-core/pkg/fsmv2/communicator/state"
```

**After:**
```go
import "github.com/.../umh-core/pkg/fsmv2/workers/communicator"
import "github.com/.../umh-core/pkg/fsmv2/workers/communicator/state"
```

**Update locations:**
- Search entire codebase for `pkg/fsmv2/communicator` imports
- Update to `pkg/fsmv2/workers/communicator`
- Verify no external packages import communicator

**Test verification:**
```bash
# Run communicator tests
go test ./pkg/fsmv2/workers/communicator/... -v

# Run integration tests
go test ./pkg/fsmv2/supervisor/ -run TestIntegration -v

# Verify no import errors
go build ./...
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify communicator functionality unchanged
- Check for broken imports in other packages

**Commit message:**
```
refactor(fsmv2): move communicator to workers/ subdirectory

Part of FSMv2 package restructuring (Task 5/9).

Moves communicator implementation from top-level to workers/
subdirectory to clarify it's a worker implementation, not
part of the core FSM framework.

Import path change:
- pkg/fsmv2/communicator → pkg/fsmv2/workers/communicator

All internal structure preserved (action/, state/, etc.).
No functional changes. All tests pass.
```

---

### Task 6: Update Import Paths Throughout Codebase

**Objective**: Systematically update all import paths to use new package structure.

**Files to modify:**

Search and replace in:
- `pkg/fsmv2/supervisor/supervisor.go` (imports health, execution, collection, metrics)
- `pkg/fsmv2/supervisor/*_test.go` (update test imports)
- Any files that import `pkg/fsmv2/communicator` → update to `workers/communicator`
- Examples (if they exist) → update imports

**Search commands:**
```bash
# Find all fsmv2 imports
grep -r "pkg/fsmv2" . --include="*.go" | grep -v "vendor/"

# Find communicator imports specifically
grep -r "pkg/fsmv2/communicator" . --include="*.go" | grep -v "vendor/"

# Find supervisor imports
grep -r "pkg/fsmv2/supervisor" . --include="*.go" | grep -v "vendor/"
```

**Update strategy:**

1. Update imports in `supervisor.go` first (core file)
2. Update imports in each subsystem (`health/`, `execution/`, etc.)
3. Update imports in test files
4. Update imports in external packages (if any)
5. Verify with `go build ./...`

**Test verification:**
```bash
# Verify no import errors
go build ./...

# Run all tests
go test ./pkg/fsmv2/... -v

# Check for import cycles
go list -f '{{.ImportPath}} {{.Imports}}' ./pkg/fsmv2/... | grep -i cycle
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Verify no import cycles introduced
- Check all tests still pass

**Commit message:**
```
refactor(fsmv2): update import paths for new structure

Part of FSMv2 package restructuring (Task 6/9).

Updates all import paths to reflect new package organization:
- supervisor/health/
- supervisor/execution/
- supervisor/collection/
- supervisor/metrics/
- workers/communicator/

No functional changes. All tests pass.
```

---

### Task 7: Create Examples for Learning Materials

**Objective**: Provide runnable examples demonstrating FSM usage with new structure.

**Files to create:**

**Example 1: Simple Worker** (`pkg/fsmv2/examples/simple_worker/`)
```go
// main.go
package main

import (
    "github.com/.../umh-core/pkg/fsmv2"
    "github.com/.../umh-core/pkg/fsmv2/supervisor"
)

// SimpleWorker demonstrates minimal FSM worker implementation
type SimpleWorker struct {
    counter int
}

func (w *SimpleWorker) CollectObservedState(ctx context.Context) (interface{}, error) {
    return map[string]interface{}{"counter": w.counter}, nil
}

func (w *SimpleWorker) DeriveDesiredState(observed interface{}) (fsmv2.DesiredState, error) {
    // No child workers, no signals
    return fsmv2.DesiredState{}, nil
}

func (w *SimpleWorker) Next(snapshot fsmv2.Snapshot) (string, error) {
    // Increment counter
    w.counter++
    return "running", nil
}

func main() {
    // Create supervisor
    sup := supervisor.NewSupervisor(supervisor.Config{
        StaleThreshold: 5 * time.Second,
        CollectorTimeout: 10 * time.Second,
    })

    // Add worker
    worker := &SimpleWorker{}
    identity := fsmv2.Identity{ID: "simple-1", Type: "simple"}
    sup.AddWorker(identity, worker)

    // Start supervisor
    ctx := context.Background()
    done := sup.Start(ctx)
    <-done
}
```

**Example 2: Hierarchical Composition** (`pkg/fsmv2/examples/hierarchical/`)
```go
// main.go - Demonstrates parent-child worker relationships
// Parent worker that spawns child workers based on config
```

**Example 3: Communicator Usage** (`pkg/fsmv2/examples/communicator_usage/`)
```go
// main.go - Shows how to use the communicator worker
// Demonstrates authentication, syncing, transport setup
```

**README files:**

Each example includes `README.md` with:
- What the example demonstrates
- How to run it
- Key concepts illustrated
- Links to relevant documentation

**Test verification:**
```bash
# Build all examples
go build ./pkg/fsmv2/examples/simple_worker/
go build ./pkg/fsmv2/examples/hierarchical/
go build ./pkg/fsmv2/examples/communicator_usage/

# Run simple example
./examples/simple_worker/simple_worker
# Should run without errors and show tick logs

# Verify examples compile
go build ./pkg/fsmv2/examples/...
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Test that examples actually run
- Verify README clarity

**Commit message:**
```
docs(fsmv2): add learning examples for new structure

Part of FSMv2 package restructuring (Task 7/9).

Adds runnable examples demonstrating FSM usage:
- simple_worker: Minimal worker implementation
- hierarchical: Parent-child relationships
- communicator_usage: Using the communicator worker

Each example includes README with usage instructions.
```

---

### Task 8: Verify All Tests Pass

**Objective**: Comprehensive test verification across entire FSMv2 package.

**Test commands:**
```bash
# Run all fsmv2 tests with verbose output
go test ./pkg/fsmv2/... -v -count=1

# Run tests with race detector
go test ./pkg/fsmv2/... -race

# Run tests with coverage
go test ./pkg/fsmv2/... -cover -coverprofile=coverage.out

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html

# Check for focused tests (should be none)
ginkgo -r ./pkg/fsmv2/... --fail-on-focused

# Run supervisor integration tests specifically
go test ./pkg/fsmv2/supervisor/ -run TestIntegration -v
```

**Verification checklist:**

- [ ] All unit tests pass (`go test ./pkg/fsmv2/...`)
- [ ] All integration tests pass
- [ ] No race conditions detected (`-race`)
- [ ] Test coverage maintained (compare before/after)
- [ ] No focused tests (`ginkgo --fail-on-focused`)
- [ ] No disabled tests accidentally re-enabled
- [ ] All examples compile and run
- [ ] No import cycles (`go list -f '{{.ImportPath}} {{.Imports}}' ./pkg/fsmv2/...`)

**Test verification:**
```bash
# Comprehensive test run
make test  # (uses project Makefile)

# Or manually:
go test ./pkg/fsmv2/... -v -race -cover
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Compare test coverage before/after restructuring
- Verify no test regressions

**Commit message:**
```
test(fsmv2): verify all tests pass after restructuring

Part of FSMv2 package restructuring (Task 8/9).

Comprehensive test verification:
- All 29 test files pass
- No race conditions detected
- Test coverage maintained
- No import cycles
- All examples compile

Verification complete. Structure change successful.
```

---

### Task 9: Create Migration Guide and Update Documentation

**Objective**: Document the restructuring for future developers and maintainers.

**Files to create/update:**

**1. Migration guide** (`docs/fsmv2/MIGRATION_GUIDE.md`):
```markdown
# FSMv2 Package Restructuring Migration Guide

## For Internal Developers

If you have an open PR that imports fsmv2 packages, update imports:

**Old imports:**
```go
import "github.com/.../pkg/fsmv2/communicator"
import "github.com/.../pkg/fsmv2/supervisor"
```

**New imports:**
```go
import "github.com/.../pkg/fsmv2/workers/communicator"
import "github.com/.../pkg/fsmv2/supervisor"           // Unchanged
import "github.com/.../pkg/fsmv2/supervisor/health"     // New
import "github.com/.../pkg/fsmv2/supervisor/execution"  // New
import "github.com/.../pkg/fsmv2/supervisor/collection" // New
import "github.com/.../pkg/fsmv2/supervisor/metrics"    // New
```

## For External Consumers

Public APIs unchanged. Only internal package structure reorganized.

If you import `pkg/fsmv2/communicator`, update to `pkg/fsmv2/workers/communicator`.

## Directory Structure Changes

- `communicator/` → `workers/communicator/` (worker implementation)
- `supervisor/` → Subdivided into health/, execution/, collection/, metrics/
- New `examples/` directory for learning materials
```

**2. Architecture documentation** (`docs/fsmv2/ARCHITECTURE.md`):

Update to reflect new structure:
- Add section on subsystem organization
- Document health/, execution/, collection/, metrics/ responsibilities
- Update diagrams to show new structure

**3. README** (`pkg/fsmv2/README.md`):

Create or update README explaining:
- Package structure and purpose of each subdirectory
- How to navigate the codebase
- Links to examples
- Quick start guide for new developers

**4. Update existing documentation references:**

Search for any documentation referencing old paths:
```bash
grep -r "pkg/fsmv2/communicator" docs/
grep -r "supervisor.go" docs/
```

Update references to reflect new structure.

**Test verification:**
```bash
# Verify documentation renders correctly
# (if using markdown tools)

# Check for broken links
# (manual or automated link checking)

# Verify examples referenced in docs actually exist
```

**Review checkpoint:**
- CodeRabbit CLI: `coderabbit review --incremental`
- Human review of documentation clarity
- Check all links and references valid

**Commit message:**
```
docs(fsmv2): add migration guide and update documentation

Part of FSMv2 package restructuring (Task 9/9).

Adds comprehensive documentation for restructuring:
- MIGRATION_GUIDE.md for developers with open PRs
- Updated ARCHITECTURE.md with subsystem organization
- New pkg/fsmv2/README.md explaining structure
- Updated references in existing docs

Completes FSMv2 package restructuring.
```

---

## 5. Lost Work Recovery

### Monitoring Runbook Commit (3bb6a374c)

**Commit details:**
- SHA: `3bb6a374cf1df0a7c73f009d1b680962599f169a`
- Date: 2025-11-05 18:27:27 +0100
- Author: Jeremy Theocharis
- File: `docs/runbooks/fsmv2-supervisor-monitoring.md` (929 lines)

**Content:**
- Comprehensive monitoring runbook for FSMv2 supervisor
- 5 common operational scenarios with troubleshooting steps
- Complete metrics reference table (17 FSMv2 metrics)
- UX integration examples (recovery feedback, error distinction)
- Prometheus query examples for common scenarios

**Recovery strategy:**

This commit is NOT lost in git history. It exists in the commit graph between base commit `7a6adf55f` and the lost commit `3bb6a374c`. However, it's not on the current branch.

**Cherry-pick command:**
```bash
# After completing restructuring tasks, cherry-pick the runbook
git cherry-pick 3bb6a374cf1df0a7c73f009d1b680962599f169a

# If conflicts, resolve manually (unlikely - runbook is new file)
# Then commit the cherry-pick
```

**Verification:**
```bash
# Verify runbook exists
ls -lh docs/runbooks/fsmv2-supervisor-monitoring.md

# Should be 929 lines
wc -l docs/runbooks/fsmv2-supervisor-monitoring.md
```

### Other Commits Between Base and Lost Commit

**Commit range:** `7a6adf55f..3bb6a374c` (48 commits)

**Analysis of commits:**

Most commits are Phase 0-4 implementation work:
- Phase 0: Hierarchical composition (reconcileChildren, StateMapping, WorkerFactory)
- Phase 1: Infrastructure recovery (ExponentialBackoff, circuit breaker)
- Phase 2: Action execution (ActionExecutor, worker pool)
- Phase 3: Integration (tick loop, timeout handling)
- Phase 4: Monitoring & observability (metrics, structured logging, runbook)

**Important commits to preserve:**

1. **f21f310c8** - Defensive nil check in CheckChildConsistency (bug fix)
2. **e9e54e788** - Verify tick loop non-blocking guarantees (important test)
3. **e395c2a9f** - Structured logging with UX enhancements (Phase 4 feature)
4. **3bb6a374c** - Monitoring runbook (929 lines of documentation)

**Cherry-pick strategy:**

**Option 1: Cherry-pick entire range (recommended if base branch is outdated)**
```bash
# Cherry-pick all 48 commits at once
git cherry-pick 7a6adf55f..3bb6a374c

# If conflicts, resolve and continue
git cherry-pick --continue
```

**Option 2: Cherry-pick specific commits (if base branch is up-to-date)**
```bash
# Only cherry-pick critical commits not in base
git cherry-pick f21f310c8  # nil check fix
git cherry-pick e395c2a9f  # structured logging
git cherry-pick 3bb6a374c  # monitoring runbook
```

**Verification after cherry-pick:**
```bash
# Verify all tests still pass
go test ./pkg/fsmv2/... -v

# Verify monitoring runbook exists
cat docs/runbooks/fsmv2-supervisor-monitoring.md | head -50

# Verify structured logging works
go test ./pkg/fsmv2/supervisor/ -run TestStructuredLogging -v
```

### Recovery Timeline

1. **Complete Tasks 1-9** (restructuring)
2. **Verify base branch status** (is it missing Phase 0-4 work?)
3. **If missing Phase 0-4:**
   - Cherry-pick entire range `7a6adf55f..3bb6a374c`
   - Resolve conflicts (import paths may need updating)
4. **If base has Phase 0-4:**
   - Cherry-pick only critical commits (f21f310c8, e395c2a9f, 3bb6a374c)
5. **Run full test suite** to verify integration
6. **Commit cherry-picks** with message:
   ```
   chore(fsmv2): restore lost work from eng-3806 branch

   Cherry-picks Phase 4 work lost during branch divergence:
   - f21f310c8: Defensive nil check in CheckChildConsistency
   - e395c2a9f: Structured logging with UX enhancements
   - 3bb6a374c: Monitoring and observability runbook (929 lines)

   All tests pass. No conflicts with restructuring.
   ```

---

## 6. Breaking Changes

### Import Path Changes

**Changed imports:**

| Old Import Path | New Import Path | Impact |
|----------------|-----------------|---------|
| `pkg/fsmv2/communicator` | `pkg/fsmv2/workers/communicator` | Medium - Internal usage |
| `pkg/fsmv2/supervisor` | `pkg/fsmv2/supervisor` | None - Unchanged |
| (none) | `pkg/fsmv2/supervisor/health` | New - Internal only |
| (none) | `pkg/fsmv2/supervisor/execution` | New - Internal only |
| (none) | `pkg/fsmv2/supervisor/collection` | New - Internal only |
| (none) | `pkg/fsmv2/supervisor/metrics` | New - Internal only |

**Public API changes:**

**None.** All public APIs remain unchanged:
- `fsmv2.Worker` interface unchanged
- `fsmv2.Identity`, `fsmv2.Snapshot` unchanged
- `supervisor.Supervisor` public methods unchanged
- `supervisor.Config` structure unchanged

**Internal API changes:**

- `supervisor.Collector` → `collection.Collector` (internal only)
- `supervisor.FreshnessChecker` → `health.FreshnessChecker` (internal only)
- Metric recording functions moved to `metrics` package (internal only)

### Impact on External Consumers

**External consumers (if any):**

Most likely consumers:
1. `pkg/agent/` (umh-core agent using FSMv2 framework)
2. `pkg/` other packages using FSMv2
3. (Unlikely) External repositories importing umh-core FSMv2

**Required changes for external consumers:**

**If importing communicator:**
```go
// Before
import "github.com/.../umh-core/pkg/fsmv2/communicator"

// After
import "github.com/.../umh-core/pkg/fsmv2/workers/communicator"
```

**If importing supervisor (no change):**
```go
// Before and After (unchanged)
import "github.com/.../umh-core/pkg/fsmv2/supervisor"
```

**If directly importing internal packages (discouraged):**
- Internal packages (`health/`, `execution/`, etc.) should not be imported externally
- If external code imports these, it violates abstraction boundaries
- Solution: Use public supervisor APIs instead

### Migration Guide for Developers

**For developers with open PRs:**

1. **Rebase onto restructured branch:**
   ```bash
   git fetch origin
   git rebase origin/eng-3806
   ```

2. **Fix import conflicts:**
   ```bash
   # Search for old imports
   grep -r "pkg/fsmv2/communicator" . --include="*.go"

   # Replace with new imports
   sed -i 's|pkg/fsmv2/communicator|pkg/fsmv2/workers/communicator|g' **/*.go
   ```

3. **Verify tests pass:**
   ```bash
   go test ./... -v
   ```

4. **If using internal supervisor APIs:**
   - Review if public APIs sufficient
   - If not, discuss with maintainers about exposing needed functionality

**For new developers:**

1. **Read package structure:** Start with `pkg/fsmv2/README.md`
2. **Run examples:** See `pkg/fsmv2/examples/` for learning materials
3. **Understand subsystems:** Read `docs/fsmv2/ARCHITECTURE.md`
4. **Use supervisor package:** Import `pkg/fsmv2/supervisor`, not internal packages

---

## 7. Success Criteria

### Functional Requirements

- [ ] All 29 test files pass after restructuring
- [ ] No race conditions detected (`go test -race`)
- [ ] Test coverage maintained or improved
- [ ] No import cycles introduced
- [ ] All examples compile and run successfully

### Structural Requirements

- [ ] Clear package boundaries between subsystems
- [ ] Each subdirectory has single, well-defined responsibility
- [ ] Import relationships form clear hierarchy (no circular deps)
- [ ] Public APIs unchanged (no breaking changes for external consumers)
- [ ] Internal APIs clearly separated from public APIs

### Developer Experience Requirements

- [ ] New developer can find relevant code within 30 seconds
  - "Where do I add a metric?" → `pkg/fsmv2/supervisor/metrics/`
  - "Where is data collection?" → `pkg/fsmv2/supervisor/collection/`
  - "Where are action retries?" → `pkg/fsmv2/supervisor/execution/`
- [ ] Examples demonstrate usage patterns for each subsystem
- [ ] Directory structure is self-documenting (names explain purpose)
- [ ] README guides new developers through structure

### Documentation Requirements

- [ ] Migration guide exists and is accurate
- [ ] Architecture documentation updated
- [ ] Examples include README with usage instructions
- [ ] All documentation links valid (no broken references)
- [ ] Package-level godoc comments updated

### Quality Requirements

- [ ] CodeRabbit CLI review passes for each task
- [ ] No linting errors (`golangci-lint run`)
- [ ] No vet errors (`go vet ./pkg/fsmv2/...`)
- [ ] No focused tests (`ginkgo --fail-on-focused`)
- [ ] All commits follow conventional commit format

---

## 8. Implementation Checklist

### Pre-Implementation

- [ ] Create worktree for restructuring work
- [ ] Back up current state (tag or branch)
- [ ] Read entire restructuring plan
- [ ] Understand subsystem boundaries

### Task Execution

- [ ] Task 1: Create supervisor/health/ and move freshness files
- [ ] Task 2: Create supervisor/execution/ and move action files
- [ ] Task 3: Create supervisor/collection/ and move collector files
- [ ] Task 4: Create supervisor/metrics/ and extract metrics code
- [ ] Task 5: Move communicator to workers/communicator/
- [ ] Task 6: Update all import paths throughout codebase
- [ ] Task 7: Create examples/ with learning materials
- [ ] Task 8: Verify all tests pass (comprehensive)
- [ ] Task 9: Create migration guide and update docs

### Post-Implementation

- [ ] Cherry-pick lost commits (monitoring runbook, etc.)
- [ ] Run full test suite one final time
- [ ] Review all changes with code-reviewer subagent
- [ ] Create PR with summary of changes
- [ ] Request human review from maintainers

---

## 9. Risk Mitigation

### Merge Conflict Risks

**Risk:** Import path changes conflict with concurrent PRs.

**Mitigation:**
1. Communicate restructuring timeline to team
2. Request PRs hold off on merging during restructuring
3. Use separate worktree for isolation
4. Keep Tasks 1-5 in separate commits for easier conflict resolution

**Resolution:**
- If conflict in imports, apply sed replacement to conflicted files
- Re-run tests after resolving conflicts
- Use `git rerere` to record conflict resolutions

### Test Failure Risks

**Risk:** Tests fail after import path changes.

**Mitigation:**
1. Run tests after each task (incremental verification)
2. Use `-count=1` to avoid test caching
3. Use `-race` to catch concurrency issues
4. Keep old structure in separate branch for comparison

**Resolution:**
- Use `go test -v` for detailed failure output
- Compare test output before/after to identify cause
- Rollback to previous task if failure unrecoverable
- Fix issue and re-run from that task

### Import Cycle Risks

**Risk:** New package structure introduces import cycles.

**Mitigation:**
1. Follow clear hierarchy: supervisor → subsystems → utilities
2. Subsystems should NOT import each other (only supervisor imports subsystems)
3. Run `go list -f '{{.ImportPath}} {{.Imports}}'` after each task
4. Design subsystems to be independent

**Resolution:**
- Use `go list` to identify cycle
- Extract shared types to top-level package
- Use dependency inversion (interfaces) to break cycle

### Lost Work Risks

**Risk:** Cherry-pick of lost commits fails or corrupts state.

**Mitigation:**
1. Complete restructuring first, verify tests pass
2. Cherry-pick lost work as separate step
3. Create checkpoint commits before cherry-pick
4. Have clean working directory before cherry-pick

**Resolution:**
- Use `git cherry-pick --abort` to abort failed cherry-pick
- Manually apply changes from commit diff
- Re-run tests after manual application
- Commit with clear message indicating manual merge

### Performance Regression Risks

**Risk:** Package structure change impacts runtime performance.

**Mitigation:**
1. Run benchmarks before restructuring (baseline)
2. Run benchmarks after restructuring (comparison)
3. Verify no significant performance changes
4. Structure changes should be compile-time only

**Resolution:**
- Use `go test -bench=. -benchmem` for comparison
- If regression found, investigate import overhead
- Consider using internal linking if performance critical

### Rollback Strategy

**If restructuring fails:**

1. **Immediate rollback:**
   ```bash
   git reset --hard <checkpoint-tag>
   git clean -fdx
   ```

2. **Partial rollback (undo specific task):**
   ```bash
   git revert <task-commit-sha>
   git commit -m "revert: undo Task N due to issue"
   ```

3. **Recovery from checkpoint:**
   ```bash
   git checkout <checkpoint-branch>
   git checkout -b restructuring-attempt-2
   # Start from last successful task
   ```

**Checkpoints to create:**
- Before Task 1: `git tag restructuring-start`
- After Task 5: `git tag restructuring-midpoint`
- After Task 8: `git tag restructuring-complete`

---

## 10. Timeline

### Estimated Effort Per Task

| Task | Description | Estimated Time | Dependencies |
|------|-------------|----------------|--------------|
| 1 | Create health/ subdirectory | 30 minutes | None |
| 2 | Create execution/ subdirectory | 1 hour | Task 1 |
| 3 | Create collection/ subdirectory | 30 minutes | Task 2 |
| 4 | Create metrics/ subdirectory | 1.5 hours | Task 3 |
| 5 | Move communicator to workers/ | 45 minutes | Task 4 |
| 6 | Update import paths | 1 hour | Task 5 |
| 7 | Create examples | 2 hours | Task 6 |
| 8 | Verify all tests | 30 minutes | Task 7 |
| 9 | Migration guide and docs | 1.5 hours | Task 8 |

**Total estimated time:** ~9 hours of focused work

### Dependencies Between Tasks

**Sequential dependencies:**
- Tasks 1-5: Can be done sequentially (must finish each before next)
- Task 6: Depends on all previous tasks (all moves must be complete)
- Task 7: Depends on Task 6 (needs correct import paths)
- Task 8: Depends on all previous tasks (final verification)
- Task 9: Depends on Task 8 (documents final state)

**Parallelization opportunities:**
- None (tasks are inherently sequential due to import dependencies)

**Critical path:**
- Tasks 1-5 (structure creation) → Task 6 (imports) → Task 8 (verification)
- Task 7 (examples) and Task 9 (docs) are less critical but still sequential

### Execution Strategy

**Option 1: Single session (9 hours)**
- Complete all tasks in one focused session
- Minimize context switching
- Easier to maintain mental model of changes
- Higher risk if interruption occurs

**Option 2: Multi-session (3 sessions of 3 hours)**
- Session 1: Tasks 1-3 (create health, execution, collection)
- Session 2: Tasks 4-6 (create metrics, move communicator, update imports)
- Session 3: Tasks 7-9 (examples, verification, docs)
- Allows breaks for review and mental refresh
- Lower risk of fatigue-induced errors

**Recommended: Option 2 (multi-session)**
- More sustainable pace
- Natural checkpoints for review
- Easier to rollback if issues found

---

## Changelog

### 2025-11-06 13:00 - Plan created

Created comprehensive FSMv2 package restructuring plan following writing-plans skill guidelines.

**Scope:**
- 9 tasks covering directory creation, file moves, import updates, examples, and documentation
- Clear subsystem boundaries (health, execution, collection, metrics)
- Lost work recovery strategy (monitoring runbook + 48 commits)
- Risk mitigation for merge conflicts, test failures, import cycles
- Estimated 9 hours total effort across 3 sessions

**Key decisions:**
- Move communicator to workers/ (clarifies it's a worker implementation)
- Create focused subdirectories (not excessive splitting)
- Maintain backward compatibility (no public API changes)
- Examples for learning (simple_worker, hierarchical, communicator_usage)

---
