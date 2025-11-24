# FSM v2 Developer Experience Investigation

**Date**: 2025-11-20
**Investigator**: Claude Code
**Working Directory**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806`

## Executive Summary

The FSM v2 package has **excellent conceptual documentation** but **lacks practical onboarding materials**. A new developer faces a **top-down only approach** with no quick-start path. The documentation is comprehensive but requires reading 2,000+ lines before writing any code.

**Key Gap**: No "copy this directory and modify" template worker that demonstrates all state transitions in a working example.

---

## Current State Analysis

### Documentation Quality ✅

The package has **strong documentation**:

| File | Lines | Purpose | Quality |
|------|-------|---------|---------|
| `README.md` | 532 | Mental models, architecture, testing requirements | **Excellent** - explains WHY |
| `docs/getting-started.md` | 161 | 5-minute introduction, Triangle Model | **Good** - has code examples |
| `docs/common-patterns.md` | 227 | Pattern catalog with examples | **Excellent** - practical guidance |
| `PATTERNS.md` | 1,041 | Design rationale, established patterns | **Excellent** - deep explanations |
| `doc.go` | ~500+ | API reference with code examples | **Good** - standard Go package docs |

**Total documentation**: ~2,000 lines covering mental models, patterns, and architecture.

### Example Code ✅

Examples exist but are **not prominently featured**:

**Location**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/`

**Structure**:
```
example-parent/
├── worker.go              # Worker implementation (131 LOC)
├── state/
│   ├── state_stopped.go          # Initial state
│   ├── state_trying_to_start.go  # Active transition state
│   ├── state_running.go          # Healthy operational state
│   ├── state_degraded.go         # Partial failure state
│   └── state_trying_to_stop.go   # Cleanup state
├── action/
│   └── start_action.go    # Example idempotent action
└── snapshot/
    └── snapshot.go        # Observed/Desired state types
```

**What's Good**:
- Demonstrates all 5 base states: Stopped → TryingToStart → Running → Degraded → TryingToStop
- Shows proper shutdown handling (checks `IsShutdownRequested()` in every state)
- Includes parent-child coordination
- Has action example with idempotency test

**What's Missing**:
- **Not discoverable** - README mentions examples but doesn't say "start here"
- **No standalone template** - Mixed with parent-child complexity
- **No cycle demonstration** - Doesn't show automatic state cycling for testing
- **No "three commands to start"** quick path

### Onboarding Flow Analysis

**Current Flow** (Top-down only):

```
1. Read README.md (532 lines) - Mental models
   ↓
2. Read getting-started.md (161 lines) - Triangle Model
   ↓
3. Read common-patterns.md (227 lines) - Essential patterns
   ↓
4. Maybe find examples? (not clearly directed)
   ↓
5. Start coding (after ~920 lines of reading minimum)
```

**Time to first working code**: 30-60 minutes of reading

**What Works**:
- Documentation is comprehensive and well-written
- Mental models are clearly explained
- Design rationale is accessible

**What's Confusing**:
- No clear "start here for beginners" path
- Examples buried in `workers/example/` - not mentioned in top-level README
- No bottom-up "get running fast" option
- Template concept exists but not packaged as copyable scaffold

---

## What's Missing

### 1. Bottom-Up Quick Start ❌

**Problem**: No "three commands to get running" path exists.

**User's Vision**:
```bash
# Three commands to start
cd pkg/fsmv2/template-worker
go test ./...  # See all states cycle
# Modify states.go to fit your use case
```

**Current Reality**: Must read docs, understand Triangle Model, implement from scratch.

### 2. Template Worker ❌

**Problem**: No copyable "start from this" worker that:
- Demonstrates all base states in simplest possible form
- Cycles through states automatically for testing
- Has clear TODO comments for customization
- Works standalone without parent-child complexity

**User's Vision**:
```
template-worker/
├── README.md              # "Copy this directory and modify"
├── worker.go              # Minimal implementation with TODOs
├── states.go              # All 5 states with cycle logic
├── worker_test.go         # Shows states cycling every 5 seconds
└── CUSTOMIZATION.md       # What to change for your use case
```

### 3. State Cycle Example ❌

**Problem**: No worker that demonstrates cycling through all states for visual learning.

**User's Vision**:
```go
// Template worker cycles through all states automatically
Stopped (5s) → TryingToStart (5s) → Running (5s) → Degraded (5s) → TryingToStop (5s) → Stopped
```

This would let developers:
- **See** all state transitions in action
- **Understand** the flow visually before modifying
- **Test** state logic by watching logs

### 4. README Structure ❌

**Problem**: README is top-down only (theory → practice).

**Missing**: Bottom-up section at the top:

```markdown
# FSM v2

## Quick Start (5 minutes)

**Want to get running fast?**
1. Copy `template-worker/` directory
2. Run `go test ./...` to see state cycling
3. Modify states to fit your use case

**Want to understand the architecture first?**
Continue reading below for mental models and design patterns.

## Mental Models
(existing content)
```

---

## Specific Improvements Needed

### Improvement 1: Restructure README

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/README.md`

**Add at line 1** (before current content):

```markdown
# FSM v2

## Choose Your Path

### 🚀 Quick Start (5 minutes) - Bottom-Up

**Copy and modify approach:**
```bash
# 1. Copy template worker
cp -r pkg/fsmv2/template-worker/ pkg/fsmv2/workers/myworker/

# 2. See it run - watches states cycle
cd pkg/fsmv2/workers/myworker
go test -v ./...

# 3. Customize for your use case
# Edit states.go - change state logic
# Edit worker.go - implement CollectObservedState
```

**What you'll see**: Worker cycles through all states every 5 seconds:
```
Stopped → TryingToStart → Running → Degraded → TryingToStop → Stopped
```

### 📚 Architecture First (30 minutes) - Top-Down

**Conceptual understanding approach:**
1. Read this README - Mental models and core concepts
2. Read `docs/getting-started.md` - Triangle Model
3. Read `docs/common-patterns.md` - Essential patterns
4. Study `workers/example/example-parent/` - Production example

---

## Mental Models
(existing README content continues here)
```

### Improvement 2: Create Template Worker

**Directory**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/template-worker/`

**Structure**:

**`README.md`**:
```markdown
# Template Worker - Copy This Directory

This is a minimal FSM v2 worker that cycles through all base states.

## Quick Start

```bash
# 1. Copy this directory
cp -r template-worker/ workers/myworker/

# 2. Run tests to see state cycling
cd workers/myworker
go test -v ./...

# 3. Customize
# See CUSTOMIZATION.md for what to change
```

## What This Demonstrates

This worker cycles through 5 states every 5 seconds:

1. **Stopped** (passive) - Initial state, no work happening
2. **TryingToStart** (active) - Emits StartAction on every tick
3. **Running** (passive) - Healthy operational state
4. **Degraded** (passive) - Partial failure state
5. **TryingToStop** (active) - Cleanup before removal

**Active states** (TryingTo*) emit actions until success.
**Passive states** only transition when conditions change.

## Files

- `worker.go` - Worker implementation (TODOs marked)
- `states.go` - All 5 states with cycle logic
- `models.go` - ObservedState and DesiredState types
- `worker_test.go` - Integration test showing cycling
- `CUSTOMIZATION.md` - What to modify for your use case
```

**`worker.go`** (simplified):
```go
package template_worker

import (
    "context"
    "time"
    "go.uber.org/zap"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// TemplateWorker is a minimal FSM v2 worker for copying and customization
type TemplateWorker struct {
    *fsmv2.BaseWorker[*TemplateDependencies]
    identity fsmv2.Identity
    logger   *zap.SugaredLogger

    // TODO: Add your dependencies here
    // Example: database *sql.DB
}

// CollectObservedState gathers current system state
func (w *TemplateWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // TODO: Replace with your actual observation logic
    // Example: query database, check process status, read metrics

    observed := TemplateObservedState{
        CollectedAt: time.Now(),
        IsHealthy:   true,  // TODO: Real health check
        ErrorCount:  0,     // TODO: Real error counting
    }

    return observed, nil
}

// DeriveDesiredState transforms user config into desired state
func (w *TemplateWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    // TODO: Parse your user configuration
    // Example: YAML unmarshal, validate fields

    return fsmv2types.DesiredState{
        State: "running",  // TODO: Derive from spec
    }, nil
}

// GetInitialState returns where the FSM starts
func (w *TemplateWorker) GetInitialState() fsmv2.State[any, any] {
    return &StoppedState{}
}
```

**`states.go`**:
```go
package template_worker

import (
    "time"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// StoppedState - Initial state, no work happening
type StoppedState struct{}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[TemplateObservedState, *TemplateDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return s, fsmv2.SignalNeedsRemoval, nil
    }

    // RULE 2: Only transition if desired state says so
    if snap.Desired.ShouldBeRunning() {
        return &TryingToStartState{}, fsmv2.SignalNone, nil
    }

    return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string { return "Stopped" }
func (s *StoppedState) Reason() string { return "Worker is stopped" }

// TryingToStartState - Active state that emits actions until started
type TryingToStartState struct {
    startedAt time.Time
}

func (s *TryingToStartState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[TemplateObservedState, *TemplateDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Initialize timestamp on first tick
    if s.startedAt.IsZero() {
        s.startedAt = time.Now()
    }

    // Simulate startup taking 5 seconds (for demo cycling)
    if time.Since(s.startedAt) > 5*time.Second {
        return &RunningState{startedAt: time.Now()}, fsmv2.SignalNone, nil
    }

    // Keep emitting action until startup completes
    // TODO: Replace with your actual startup action
    return s, fsmv2.SignalNone, &StartAction{}
}

func (s *TryingToStartState) String() string { return "TryingToStart" }
func (s *TryingToStartState) Reason() string { return "Starting worker" }

// RunningState - Healthy operational state
type RunningState struct {
    startedAt time.Time
}

func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[TemplateObservedState, *TemplateDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Initialize timestamp
    if s.startedAt.IsZero() {
        s.startedAt = time.Now()
    }

    // Simulate degradation after 5 seconds (for demo cycling)
    if time.Since(s.startedAt) > 5*time.Second {
        return &DegradedState{startedAt: time.Now()}, fsmv2.SignalNone, nil
    }

    // TODO: Real health check
    // if snap.Observed.ErrorCount > 0 {
    //     return &DegradedState{}, fsmv2.SignalNone, nil
    // }

    return s, fsmv2.SignalNone, nil
}

func (s *RunningState) String() string { return "Running" }
func (s *RunningState) Reason() string { return "Worker is healthy" }

// DegradedState - Partial failure state
type DegradedState struct {
    startedAt time.Time
}

func (s *DegradedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[TemplateObservedState, *TemplateDesiredState](snapAny)

    // RULE 1: Always check shutdown first
    if snap.Desired.IsShutdownRequested() {
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // Initialize timestamp
    if s.startedAt.IsZero() {
        s.startedAt = time.Now()
    }

    // Simulate triggering stop after 5 seconds (for demo cycling)
    if time.Since(s.startedAt) > 5*time.Second {
        // Request shutdown to complete the cycle
        snap.Desired.SetShutdownRequested(true)
        return &TryingToStopState{}, fsmv2.SignalNone, nil
    }

    // TODO: Real recovery logic
    // if snap.Observed.ErrorCount == 0 {
    //     return &RunningState{}, fsmv2.SignalNone, nil
    // }

    return s, fsmv2.SignalNone, nil
}

func (s *DegradedState) String() string { return "Degraded" }
func (s *DegradedState) Reason() string { return "Worker is unhealthy" }

// TryingToStopState - Cleanup before removal
type TryingToStopState struct {
    startedAt time.Time
}

func (s *TryingToStopState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap := fsmv2.ConvertSnapshot[TemplateObservedState, *TemplateDesiredState](snapAny)

    // Initialize timestamp
    if s.startedAt.IsZero() {
        s.startedAt = time.Now()
    }

    // Simulate cleanup taking 5 seconds (for demo cycling)
    if time.Since(s.startedAt) > 5*time.Second {
        return &StoppedState{}, fsmv2.SignalNone, nil
    }

    // Keep emitting cleanup action until complete
    // TODO: Replace with your actual cleanup action
    return s, fsmv2.SignalNone, &StopAction{}
}

func (s *TryingToStopState) String() string { return "TryingToStop" }
func (s *TryingToStopState) Reason() string { return "Stopping worker" }
```

**`CUSTOMIZATION.md`**:
```markdown
# Customizing Template Worker

## Step-by-Step Guide

### 1. Replace State Cycle Logic

**Current**: States automatically cycle every 5 seconds for demonstration.

**Your code**: Remove time-based transitions, add real conditions:

**In `TryingToStartState.Next()`**:
```go
// REMOVE THIS (demo code):
if time.Since(s.startedAt) > 5*time.Second {
    return &RunningState{}, fsmv2.SignalNone, nil
}

// ADD THIS (real logic):
if snap.Observed.ProcessIsRunning {
    return &RunningState{}, fsmv2.SignalNone, nil
}
```

**In `RunningState.Next()`**:
```go
// REMOVE THIS (demo code):
if time.Since(s.startedAt) > 5*time.Second {
    return &DegradedState{}, fsmv2.SignalNone, nil
}

// ADD THIS (real health check):
if snap.Observed.ErrorCount > 5 {
    return &DegradedState{}, fsmv2.SignalNone, nil
}
```

### 2. Implement Real Observations

**In `worker.go` → `CollectObservedState()`**:

```go
// REMOVE THIS (placeholder):
observed := TemplateObservedState{
    CollectedAt: time.Now(),
    IsHealthy:   true,
    ErrorCount:  0,
}

// ADD THIS (real monitoring):
observed := TemplateObservedState{
    CollectedAt:       time.Now(),
    ProcessIsRunning:  w.checkProcessStatus(),
    ErrorCount:        w.countRecentErrors(),
    LastHealthCheck:   w.performHealthCheck(),
}
```

### 3. Add Real Actions

**In `actions.go`**:

```go
// StartAction - Idempotent startup
type StartAction struct{}

func (a *StartAction) Execute(ctx context.Context, deps any) error {
    // Check if already started (idempotency)
    if processIsRunning() {
        return nil
    }

    // Start process
    return startProcess(ctx)
}

// StopAction - Idempotent cleanup
type StopAction struct{}

func (a *StopAction) Execute(ctx context.Context, deps any) error {
    // Check if already stopped (idempotency)
    if !processIsRunning() {
        return nil
    }

    // Stop process
    return stopProcess(ctx)
}
```

### 4. Parse User Configuration

**In `worker.go` → `DeriveDesiredState()`**:

```go
// Parse your YAML config
var config MyWorkerConfig
if err := yaml.Unmarshal([]byte(userSpec.Config), &config); err != nil {
    return fsmv2types.DesiredState{}, err
}

return fsmv2types.DesiredState{
    State: config.Enabled ? "running" : "stopped",
}, nil
```

## What NOT to Change

**Keep these patterns**:
- Always check `IsShutdownRequested()` first in every state
- Use TryingTo* prefix for active states that emit actions
- Keep states passive (descriptive nouns) when they only observe
- Return same state when conditions aren't met (stay stable)

## Testing Your Changes

```bash
# Run tests to verify state transitions
go test -v ./...

# Check that actions are idempotent
go test -run TestIdempotency
```
```

### Improvement 3: Update Getting Started Guide

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/docs/getting-started.md`

**Add at line 1** (before existing content):

```markdown
# Getting Started with FSM v2

## Two Learning Paths

### Path 1: Quick Start - Copy Template (5 minutes)

**Best for**: Learning by doing, getting code running fast.

```bash
# 1. Copy template worker
cp -r pkg/fsmv2/template-worker/ pkg/fsmv2/workers/myworker/

# 2. Run tests - watch states cycle
cd pkg/fsmv2/workers/myworker
go test -v ./...

# 3. Customize for your use case
# See template-worker/CUSTOMIZATION.md
```

**You'll see**: Worker cycling through all states every 5 seconds:
```
Stopped → TryingToStart → Running → Degraded → TryingToStop → Stopped (repeat)
```

### Path 2: Conceptual Understanding (30 minutes)

**Best for**: Understanding architecture before coding.

Continue reading below for Triangle Model and mental models.

---

(existing getting-started.md content continues)
```

### Improvement 4: Examples Discoverability

**File**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/README.md`

**Current line ~496**: "Where to Find Examples" section exists but not prominent.

**Change to** (add at line ~30, right after "Choose Your Path"):

```markdown
## Examples at a Glance

| Example | Location | Use Case | Complexity |
|---------|----------|----------|------------|
| **Template Worker** | `template-worker/` | Copy and customize | ⭐ Beginner |
| **Example Parent** | `workers/example/example-parent/` | Parent-child coordination | ⭐⭐ Intermediate |
| **Communicator** | `workers/communicator/` | Production worker | ⭐⭐⭐ Advanced |

**Start with**: `template-worker/` for fastest onboarding.
```

---

## Mock-Up: Improved README Structure

```markdown
# FSM v2

One-sentence summary: Type-safe state machine framework for worker lifecycle management.

---

## Choose Your Path

### 🚀 Quick Start (5 minutes) - Bottom-Up

**Want to get running fast? Copy and modify.**

```bash
# Three commands to start
cp -r pkg/fsmv2/template-worker/ pkg/fsmv2/workers/myworker/
cd pkg/fsmv2/workers/myworker
go test -v ./...  # Watch states cycle: Stopped → TryingToStart → Running → Degraded → TryingToStop
```

Then customize: See `template-worker/CUSTOMIZATION.md`

### 📚 Architecture First (30 minutes) - Top-Down

**Want to understand the design before coding?**

1. Read **Mental Models** below (10 min)
2. Read `docs/getting-started.md` - Triangle Model (10 min)
3. Read `docs/common-patterns.md` - Essential patterns (10 min)
4. Study `workers/example/example-parent/` - Production example

---

## Examples at a Glance

| Example | Location | Use Case | Complexity |
|---------|----------|----------|------------|
| **Template Worker** | `template-worker/` | Copy and customize | ⭐ Beginner |
| **Example Parent** | `workers/example/example-parent/` | Parent-child coordination | ⭐⭐ Intermediate |
| **Communicator** | `workers/communicator/` | Production worker | ⭐⭐⭐ Advanced |

---

## Mental Models

(existing README content continues - mental models section)

To work effectively with FSMv2, developers need to understand these core concepts...
```

---

## Implementation Priority

### Must-Have (P0) - Blocks Onboarding

1. **Create `template-worker/` directory** with all files shown above
   - Estimated effort: 4-6 hours
   - Impact: Enables bottom-up learning path

2. **Restructure README.md** with "Choose Your Path" section
   - Estimated effort: 1 hour
   - Impact: Makes both paths visible

3. **Update `docs/getting-started.md`** with two learning paths
   - Estimated effort: 30 minutes
   - Impact: Clarifies entry points

### Nice-to-Have (P1) - Improves Experience

4. **Create `CUSTOMIZATION.md`** in template-worker
   - Estimated effort: 2 hours
   - Impact: Guides modification process

5. **Add "Examples at a Glance" table** to README
   - Estimated effort: 15 minutes
   - Impact: Improves discoverability

### Future Enhancement (P2)

6. **Video walkthrough** showing template worker cycling
   - Estimated effort: 2-3 hours
   - Impact: Visual learning aid

7. **Interactive tutorial** with step-by-step prompts
   - Estimated effort: 8-10 hours
   - Impact: Guided learning experience

---

## References to Existing Files

### Documentation Files

- Main README: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/README.md` (lines 1-532)
- Getting Started: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/docs/getting-started.md` (lines 1-161)
- Common Patterns: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/docs/common-patterns.md` (lines 1-227)
- Design Patterns: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/PATTERNS.md` (lines 1-1041)
- API Docs: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/doc.go` (lines 1-500+)

### Example Files

- Example Parent Worker: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/worker.go` (lines 1-131)
- Stopped State: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/state/state_stopped.go` (lines 27-40)
- TryingToStart State: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/state/state_trying_to_start.go` (lines 28-43)
- Running State: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/state/state_running.go` (lines 27-39)
- Degraded State: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/example/example-parent/state/state_degraded.go` (lines 27-39)

---

## Conclusion

The FSM v2 package has **excellent architectural documentation** but **lacks practical onboarding**. The gap is **not documentation quality** (which is high) but **documentation accessibility** for new developers.

**Key Recommendation**: Add a **template worker** with automatic state cycling that serves as both:
1. **Learning tool** - Visual demonstration of all states
2. **Starting point** - Copy-paste scaffold for new workers

This single addition, combined with README restructuring to show both top-down and bottom-up paths, would transform the onboarding experience from "read 2,000 lines then code" to "run code in 5 minutes, understand architecture as you go."

**Estimated Total Effort**: 8-10 hours to implement all P0 and P1 improvements.

**Impact**: Reduces time-to-first-working-code from 30-60 minutes (reading) to 5 minutes (copying template).
