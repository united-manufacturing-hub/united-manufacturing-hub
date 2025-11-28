# Implementation Plan: FSMv2 State Field Consistency

## Problem Statement

Two architectural invariants are not enforced in the FSMv2 codebase:

1. **ObservedState MUST embed DesiredState** - Currently Application worker uses a named field instead of embedding
2. **Standardized State field** - Workers use inconsistent field names (`ConnectionStatus`, `Authenticated`, or nothing)

## Proposed Invariants

### Invariant 1: Full Desired State Embedding

Every `ObservedState` struct MUST:
- Embed its corresponding `DesiredState` as an **anonymous field** (not a named field)
- This ensures `GetObservedDesiredState()` returns the embedded desired state

**Correct Pattern:**
```go
type ChildObservedState struct {
    ID          string    `json:"id"`
    CollectedAt time.Time `json:"collected_at"`

    ChildDesiredState  // ✅ Anonymous embedding

    // Additional observed-only fields...
}
```

**Incorrect Pattern (Application worker currently):**
```go
type ApplicationObservedState struct {
    ID          string    `json:"id"`
    CollectedAt time.Time `json:"collected_at"`

    DeployedDesiredState ApplicationDesiredState `json:"deployed_desired_state"` // ❌ Named field
}
```

### Invariant 2: Standardized State Field

Every worker MUST have a consistent `State` field:

**In DesiredState:**
```go
type ChildDesiredState struct {
    config.BaseDesiredState
    State string `json:"state"` // ✅ Standard field name
    // ... other fields
}
```

**In ObservedState:**
- The embedded DesiredState provides the `State` field
- ObservedState can access it directly via embedding

**Standard State Values:**
| Value | Meaning |
|-------|---------|
| `"running"` | Worker is actively operating |
| `"stopped"` | Worker is intentionally stopped |
| `"connecting"` | Worker is attempting to establish connection |
| `"connected"` | Worker has established connection |
| `"disconnected"` | Worker lost connection |
| `"starting"` | Worker is initializing |
| `"stopping"` | Worker is shutting down |

## Implementation Steps

### Phase 1: Add Architecture Tests (First!)

Add two new validators to enforce these invariants:

#### 1.1 ValidateObservedStateEmbedsDesired

**File:** `pkg/fsmv2/internal/validator/snapshot.go`

```go
// ValidateObservedStateEmbedsDesired checks that ObservedState types embed their
// corresponding DesiredState as an anonymous field (not a named field).
func ValidateObservedStateEmbedsDesired(baseDir string) []Violation {
    // For each ObservedState struct:
    // 1. Find if it has an embedded DesiredState (anonymous field)
    // 2. Check that it's NOT a named field with "DesiredState" in the name
    // 3. Violation if named field found instead of embedding
}
```

**Violation Type:** `OBSERVED_STATE_NOT_EMBEDDING_DESIRED`

#### 1.2 ValidateStandardStateField

**File:** `pkg/fsmv2/internal/validator/snapshot.go`

```go
// ValidateStandardStateField checks that all DesiredState types have a
// standardized State field with correct JSON tag.
func ValidateStandardStateField(baseDir string) []Violation {
    // For each DesiredState struct:
    // 1. Check for field named "State" with type "string"
    // 2. Check JSON tag is `json:"state"`
    // 3. Violation if missing or wrong type/tag
}
```

**Violation Type:** `MISSING_STANDARD_STATE_FIELD`

#### 1.3 Add to architecture_test.go

```go
Describe("ObservedState Embeds DesiredState (Invariant: Complete State Visibility)", func() {
    It("should embed DesiredState as anonymous field, not named field", func() {
        violations := validator.ValidateObservedStateEmbedsDesired(getFsmv2Dir())
        if len(violations) > 0 {
            message := validator.FormatViolationsWithPattern("Embedding Violations", violations, "OBSERVED_STATE_NOT_EMBEDDING_DESIRED")
            Fail(message)
        }
    })
})

Describe("Standard State Field (Invariant: Consistent State Naming)", func() {
    It("should have State string field in DesiredState", func() {
        violations := validator.ValidateStandardStateField(getFsmv2Dir())
        if len(violations) > 0 {
            message := validator.FormatViolationsWithPattern("State Field Violations", violations, "MISSING_STANDARD_STATE_FIELD")
            Fail(message)
        }
    })
})
```

### Phase 2: Fix Application Worker

**File:** `pkg/fsmv2/workers/application/snapshot/snapshot.go`

Change from:
```go
type ApplicationObservedState struct {
    ID          string    `json:"id"`
    CollectedAt time.Time `json:"collected_at"`
    Name        string    `json:"name"`
    DeployedDesiredState ApplicationDesiredState `json:"deployed_desired_state"`
}
```

To:
```go
type ApplicationObservedState struct {
    ID          string    `json:"id"`
    CollectedAt time.Time `json:"collected_at"`
    Name        string    `json:"name"`

    ApplicationDesiredState  // Anonymous embedding
}
```

Update `GetObservedDesiredState()`:
```go
func (o ApplicationObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &o.ApplicationDesiredState
}
```

### Phase 3: Add State Field to All Workers

For each worker, add `State string` to DesiredState:

#### 3.1 BaseDesiredState Enhancement

**Option A:** Add to `config.BaseDesiredState`
```go
type BaseDesiredState struct {
    ShutdownRequested bool   `json:"shutdownRequested"`
    State             string `json:"state"`  // NEW: Standard state field
}
```

**Option B:** Add to each worker's DesiredState individually
- More explicit, but requires changes to all workers

**Recommendation:** Option A - centralized in BaseDesiredState

#### 3.2 Migrate ConnectionStatus → State

For workers using `ConnectionStatus`:
1. Add `State` to DesiredState (via BaseDesiredState)
2. Update FSM transitions to use `State` instead of `ConnectionStatus`
3. Deprecate/remove `ConnectionStatus` from ObservedState

### Phase 4: Update FSM State Transitions

Update all `Next()` methods to check `State` instead of `ConnectionStatus`:

**Before:**
```go
if snap.Observed.ConnectionStatus == "connected" {
    return &ConnectedState{}, fsmv2.SignalNone, nil
}
```

**After:**
```go
if snap.Observed.State == "connected" {
    return &ConnectedState{}, fsmv2.SignalNone, nil
}
```

### Phase 5: Update Actions to Set State

Actions that change connection status should update `State`:

```go
func (a *ConnectAction) Execute(ctx context.Context, deps any) error {
    d := deps.(snapshot.ChildDependencies)
    d.SetConnected(true)
    d.SetState("connected")  // NEW
    return nil
}
```

## Files to Modify

### New Files
- None (all modifications to existing files)

### Modified Files

| File | Changes |
|------|---------|
| `pkg/fsmv2/internal/validator/snapshot.go` | Add 2 new validators |
| `pkg/fsmv2/internal/validator/registry.go` | Add 2 new pattern registry entries |
| `pkg/fsmv2/architecture_test.go` | Add 2 new test cases |
| `pkg/fsmv2/config/childspec.go` or `internal/helpers/base_desired_state.go` | Add `State` field to BaseDesiredState |
| `pkg/fsmv2/workers/application/snapshot/snapshot.go` | Fix embedding pattern |
| `pkg/fsmv2/workers/example/example-child/snapshot/snapshot.go` | Remove `ConnectionStatus`, use embedded `State` |
| `pkg/fsmv2/workers/example/example-child/state/*.go` | Update transitions to use `State` |
| Similar changes for: example-failing, example-panic, example-slow, example-parent, communicator |

## Testing Strategy

1. **Run new architecture tests first** - They should FAIL initially
2. **Fix Application worker** - First test should pass
3. **Add State field** - Second test should start passing
4. **Run full test suite** - Ensure no regressions
5. **Run integration tests** - Verify FSM behavior unchanged

## Migration Notes

### Backward Compatibility

- JSON serialization changes when removing `ConnectionStatus` and adding `State`
- If there's persisted state in production, consider:
  - Adding migration code to read old format
  - Or: Keep `ConnectionStatus` deprecated but readable

### CLI Output Changes

After this change, your CLI output will show:
```
DESIRED:
  state: connected
OBSERVED:
  state: connected  ← Now matches!
```

## Open Questions

1. **Should State be in BaseDesiredState or per-worker?**
   - Recommendation: BaseDesiredState for consistency

2. **What about workers without connection concept (e.g., Parent)?**
   - They can use states like `"running"`, `"stopped"`

3. **Should we also standardize JSON tag casing?**
   - Currently mixed: `ShutdownRequested` vs `shutdownRequested`
   - Recommendation: Standardize to snake_case (`shutdown_requested`, `state`)

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1: Architecture Tests | 2-3 hours |
| Phase 2: Fix Application Worker | 30 min |
| Phase 3: Add State Field | 1-2 hours |
| Phase 4: Update Transitions | 2-3 hours |
| Phase 5: Update Actions | 1-2 hours |
| Testing & Verification | 1-2 hours |
| **Total** | **8-12 hours** |

## Next Steps

1. Review and approve this plan
2. Start with Phase 1 (architecture tests) - TDD approach
3. Implement fixes to make tests pass
4. Run full test suite
5. Create PR for review
