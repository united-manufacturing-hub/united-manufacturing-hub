# Investigation 2: Root Architecture - "root" vs "root_supervisor"

**Date**: 2025-11-20
**Investigator**: Claude Code
**Context**: User noticed both "root" and "root_supervisor" exist in FSM v2 and expected a simpler pattern

## Executive Summary

The architecture currently has TWO distinct components:
1. **`pkg/fsmv2/root/`** - Generic, reusable root supervisor (production code)
2. **`pkg/fsmv2/workers/example/root_supervisor/`** - Usage example showing how to use the root package

This is **intentional and well-designed**, not redundant. The naming is slightly confusing, but the architecture follows sound software engineering principles.

## Current Architecture

### Component 1: Generic Root Package (`pkg/fsmv2/root/`)

**Location**: `/umh-core/pkg/fsmv2/root/`

**Purpose**: Production-ready, reusable root supervisor that can manage ANY registered worker type dynamically based on YAML configuration.

**Key Files**:
- `types.go` - State types (`PassthroughObservedState`, `PassthroughDesiredState`)
- `worker.go` - `PassthroughWorker` implementation that parses YAML
- `setup.go` - `NewRootSupervisor()` helper function (the main API)
- `factory_registration.go` - Auto-registration via `init()`
- `integration_test.go` - Tests for the root package itself

**Core Pattern - The "Passthrough Pattern"**:
```go
// From worker.go:73-75
type childrenConfig struct {
    Children []config.ChildSpec `yaml:"children"`
}

// From worker.go:94-121
func (w *PassthroughWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
    // ... parse YAML ...
    var childrenCfg childrenConfig
    if userSpec.Config != "" {
        if err := yaml.Unmarshal([]byte(userSpec.Config), &childrenCfg); err != nil {
            return config.DesiredState{}, fmt.Errorf("failed to parse children config: %w", err)
        }
    }

    return config.DesiredState{
        State:         "running",
        ChildrenSpecs: childrenCfg.Children,  // Just passes through - doesn't know child types!
    }, nil
}
```

**Key Insight**: The root worker doesn't hardcode child types. It parses YAML and returns `ChildrenSpecs` that can reference ANY registered worker type.

### Component 2: Example Package (`pkg/fsmv2/workers/example/root_supervisor/`)

**Location**: `/umh-core/pkg/fsmv2/workers/example/root_supervisor/`

**Purpose**: Demonstrates HOW to use the generic root package by implementing a custom child worker type.

**Key Files**:
- `types.go` - Re-exports child types from snapshot package
- `child_worker.go` - `ChildWorker` implementation (EXAMPLE child type)
- `factory_registration.go` - Registers the example child worker
- `integration_test.go` - Shows complete usage with root supervisor

**What This Example Shows**:
```go
// From integration_test.go:123-138
rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
    ID:           "root-001",
    Name:         "Test Root Supervisor",
    Store:        mockStore,
    Logger:       logger,
    TickInterval: 100 * time.Millisecond,
})

// Example uses the GENERIC root package, not its own implementation!
```

**The Example Child Worker**:
```go
// From child_worker.go:96-117
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    // Child workers don't have children - they are leaf nodes
    return fsmv2types.DesiredState{
        State:         "connected",
        ChildrenSpecs: nil, // KEY: Leaf node returns nil ChildrenSpecs
    }, nil
}
```

## Why Both Exist: Architectural Reasoning

### Separation of Concerns

1. **Generic Root (`pkg/fsmv2/root/`)** = **Library/Framework Code**
   - Reusable across all use cases
   - No application-specific logic
   - Pure passthrough pattern
   - Production-ready

2. **Example (`pkg/fsmv2/workers/example/root_supervisor/`)** = **Documentation by Example**
   - Shows HOW to create custom child workers
   - Demonstrates factory registration
   - Provides integration test patterns
   - Educational, not production

### Historical Context

From `plan.md` (line 13-23):
```markdown
### Problem Statement
FSM v2 needs a production-ready, generic root supervisor that can
dynamically manage any registered worker types based on configuration.
The current approach of hardcoding child types in examples doesn't
scale and isn't suitable for production use.

### Architecture Decision: Option C - Passthrough Root Worker
After analysis, we're implementing Option C: a generic passthrough
root worker that:
1. Lives in production code (`/pkg/fsmv2/root/`) not examples
2. Parses YAML config to extract `children:` array
3. Passes through ChildrenSpecs without hardcoding types
```

This was a **deliberate architectural decision** to move from hardcoded examples to a generic, reusable pattern.

## User's Expected Design vs Current Design

### User's Expectation
> "I expected to see only a root supervisor under workers that acts as a template,
> and then in examples just a setup.go that registers example-child and example-parent
> by adding them to the root worker."

### Current Design
- **Generic root supervisor**: Lives in `pkg/fsmv2/root/` (production code, not under workers)
- **Example**: Lives in `pkg/fsmv2/workers/example/root_supervisor/` (shows usage, not template)

### Key Difference

The user expected:
```
workers/
  root_supervisor/        # Generic template here
examples/
  setup.go                # Just registration
```

The actual design:
```
root/                     # Generic implementation (NOT under workers)
  setup.go                # NewRootSupervisor() API
workers/example/
  root_supervisor/        # Complete usage example with custom child
```

## Comparison: Current vs Expected Design

### Current Design - Pros

1. **Clear Separation**: Production code (`root/`) vs examples (`workers/example/`)
2. **Import Clarity**: Users import `pkg/fsmv2/root`, not from examples
3. **Encapsulation**: Root package is self-contained with its own state types
4. **Discoverability**: New users find examples under `workers/example/`
5. **Testing**: Each component tested independently
6. **Versioning**: Production code versioned separately from examples

### Current Design - Cons

1. **Naming Confusion**: "root" vs "root_supervisor" is confusing
2. **Two Locations**: Users must look in two places
3. **Example Complexity**: `root_supervisor` example includes full worker implementation
4. **Cognitive Load**: Understanding requires reading both packages

### Expected Design - Pros

1. **Simpler Naming**: Everything "root" is in one place
2. **Clearer Organization**: Template under workers, registration in examples
3. **Less Duplication**: Single location for root concept
4. **Easier Discovery**: One place to look for root functionality

### Expected Design - Cons

1. **Production + Example Mixed**: Template code mixed with example usage
2. **Import Path Issues**: Importing from `workers/` feels wrong for production
3. **Less Encapsulation**: State types shared between production and examples
4. **Harder to Version**: Production and example code coupled

## Detailed File Analysis

### Generic Root Package Files

| File | Purpose | Lines | Key Responsibility |
|------|---------|-------|-------------------|
| `root/types.go` | State definitions | 85 | Define `PassthroughObservedState`, `PassthroughDesiredState` |
| `root/worker.go` | Worker implementation | 131 | Parse YAML, return ChildrenSpecs |
| `root/setup.go` | Public API | 127 | `NewRootSupervisor()` helper |
| `root/factory_registration.go` | Auto-registration | 51 | Register root worker with factory |
| `root/integration_test.go` | Tests | 551 | Test root package itself |

### Example Package Files

| File | Purpose | Lines | Key Responsibility |
|------|---------|-------|-------------------|
| `root_supervisor/types.go` | Re-exports | 31 | Convenience exports of child types |
| `root_supervisor/child_worker.go` | Example child | 123 | Show how to create leaf worker |
| `root_supervisor/factory_registration.go` | Child registration | 55 | Register example child worker |
| `root_supervisor/integration_test.go` | Example tests | 523 | Show complete usage pattern |

## Usage Flow

### How a Developer Uses This

1. **Import the root package**:
   ```go
   import "github.com/.../pkg/fsmv2/root"
   import _ "github.com/.../pkg/fsmv2/workers/example/root_supervisor"  // Triggers registration
   ```

2. **Create root supervisor**:
   ```go
   sup, err := root.NewRootSupervisor(root.SupervisorConfig{
       ID:     "root-001",
       Name:   "My Root",
       Store:  store,
       Logger: logger,
       YAMLConfig: `
   children:
     - name: "child-1"
       workerType: "example/root_supervisor/snapshot.ChildObservedState"
       userSpec:
         config: |
           connection_timeout: 5s
   `,
   })
   ```

3. **Root supervisor automatically**:
   - Parses YAML config
   - Extracts children specifications
   - Creates child supervisors via factory
   - Manages child lifecycle

### The Asymmetry: Root vs Children

From `setup.go:117-119`:
```go
// KEY PATTERN: Root workers need explicit AddWorker().
// Child workers are created automatically via reconcileChildren().
// This is the fundamental asymmetry that this setup helper encapsulates.
```

**Why this asymmetry exists**:
- **Root worker**: Must be added explicitly via `AddWorker()` because there's no parent to create it
- **Child workers**: Created automatically by supervisor's `reconcileChildren()` based on `ChildrenSpecs`

This is a **fundamental pattern** in FSM v2, not a bug.

## Recommendations

### Option A: Keep Current Design, Improve Naming

**Change**:
- Rename `pkg/fsmv2/workers/example/root_supervisor/` → `pkg/fsmv2/workers/example/using_root/`
- Update README to clarify: "This example shows how to USE the generic root package"

**Pros**:
- Minimal code changes
- Preserves clean separation
- Makes purpose clearer

**Cons**:
- Still two locations
- Doesn't address complexity

### Option B: Simplify Example Package

**Change**:
- Keep `pkg/fsmv2/root/` as-is
- Simplify `pkg/fsmv2/workers/example/root_supervisor/` to ONLY:
  - `README.md` - How to use root package
  - `setup.go` - Example setup code (not full worker)
  - No custom child worker (use existing example-child or example-parent)

**Pros**:
- Reduces duplication
- Makes example truly minimal
- Leverages existing example workers

**Cons**:
- Less comprehensive demonstration
- Developers need to look at other examples for child patterns

### Option C: User's Suggested Pattern (Not Recommended)

**Change**:
- Move `pkg/fsmv2/root/` → `pkg/fsmv2/workers/root_supervisor/`
- Remove state types from root, use generic types
- Add `examples/setup.go` with just registration

**Pros**:
- Matches user expectation
- Single "root" concept location

**Cons**:
- Production code under `workers/` (conceptually wrong)
- Import path `pkg/fsmv2/workers/root_supervisor` for production code
- Less encapsulation
- Harder to version separately
- Breaks existing import paths

## Conclusion

### Is This Intentional?
**YES**. This is a deliberate architectural decision documented in `plan.md`.

### Is This Good Design?
**MOSTLY YES**, with room for improvement:

**Good**:
- Clear separation of concerns (production vs example)
- Reusable generic pattern
- Well-documented
- Properly tested
- Factory registration pattern

**Could Be Better**:
- Naming is confusing (`root` vs `root_supervisor`)
- Example could be simpler
- Documentation could explain the relationship better

### Should It Be Simplified?
**Recommendation: Option B** - Simplify the example, keep the generic root package as-is.

**Rationale**:
1. The generic root package (`pkg/fsmv2/root/`) is well-designed and production-ready
2. The example is doing too much - it should just show HOW to use root, not implement a full child worker
3. The asymmetry (root needs `AddWorker()`, children auto-created) is fundamental and correct
4. Moving production code under `workers/` would be architecturally wrong

### Next Steps

If pursuing Option B (simplify example):
1. Remove custom child worker from `root_supervisor/` example
2. Update example to use existing `example-child` or `example-parent` workers
3. Rename to `using_root/` or keep name but update README to clarify purpose
4. Add cross-references between root package and example
5. Update documentation to explain the two-package pattern

## References

### Key Files
- `/umh-core/pkg/fsmv2/root/README.md` - Generic root package documentation
- `/umh-core/pkg/fsmv2/workers/example/root_supervisor/README.md` - Example documentation
- `/umh-core/pkg/fsmv2/workers/example/root_supervisor/plan.md` - Architectural decision log
- `/umh-core/pkg/fsmv2/root/setup.go:117-119` - Explains root vs children asymmetry
- `/umh-core/pkg/fsmv2/root/worker.go:94-121` - Passthrough pattern implementation

### Key Insights
1. **Passthrough Pattern**: Root doesn't know child types, just passes through specs (line 119 in worker.go)
2. **Fundamental Asymmetry**: Root needs explicit `AddWorker()`, children auto-created (setup.go:117-119)
3. **Architecture Decision**: Deliberate move from hardcoded to generic (plan.md:13-23)
4. **Factory Registry**: Worker types registered via `init()` and referenced by string in YAML
5. **Separation of Concerns**: Production code (`root/`) vs usage example (`root_supervisor/`)
