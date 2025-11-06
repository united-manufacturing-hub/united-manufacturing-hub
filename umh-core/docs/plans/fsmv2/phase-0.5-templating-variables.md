# Phase 0.5: Templating & Variables System

**Timeline:** Weeks 3-4
**Effort:** ~325 lines of code
**Dependencies:** Phase 0 (Variables in UserSpec)
**Blocks:** Benthos worker implementation

---

## Overview

Phase 0.5 establishes the templating and variables system that enables configuration reuse and hierarchical context propagation in FSMv2. This phase allows parents to share configuration with children (e.g., Connection IP → SourceFlow + SinkFlow) and compute hierarchical location paths (ISA-95).

**Why This Phase Second:**
- Benthos workers require template rendering for config generation
- Variables enable parent→child configuration sharing
- Location computation builds ISA-95 hierarchy (enterprise.site.area.line)
- Foundation for all configuration reuse patterns

**Key Patterns:**

**Three-Tier Variable Namespaces:**
```go
type VariableBundle struct {
    // User: User-defined + parent state + computed values
    // Template access: Top-level ({{ .IP }})
    // Serialized: YES
    User map[string]any `yaml:"user,omitempty"`

    // Global: Fleet-wide settings from management loop
    // Template access: Nested ({{ .global.api_endpoint }})
    // Serialized: YES
    Global map[string]any `yaml:"global,omitempty"`

    // Internal: Runtime metadata (id, timestamps, bridged_by)
    // Template access: Nested ({{ .internal.id }})
    // Serialized: NO (runtime-only)
    Internal map[string]any `yaml:"-"`
}
```

**Template Rendering:**
```go
func RenderTemplate(tmpl string, vars VariableBundle) (string, error) {
    // Flatten User variables to top-level
    flattened := vars.Flatten()

    // Execute Go template with strict mode (missingkey=error)
    t := template.New("config").Option("missingkey=error")
    t, err := t.Parse(tmpl)

    var buf bytes.Buffer
    err = t.Execute(&buf, flattened)

    return buf.String(), err
}
```

**Location Computation:**
```go
func ComputeLocation(parent, child []LocationLevel) ([]LocationLevel, error) {
    // Merge: parent + child
    // Fill gaps: enterprise → site → area → line → cell
    // Return: complete path
}

// Example:
// Parent: [enterprise: ACME, site: Factory-1]
// Child:  [line: Line-A, cell: Cell-5]
// Result: [enterprise: ACME, site: Factory-1, area: "", line: Line-A, cell: Cell-5]
// location_path: "ACME.Factory-1.Line-A.Cell-5"
```

---

## Task Summary

### Task 0.5.1: VariableBundle Structure (2 hours, ~30 lines)
**Goal:** Define three-tier namespace structure (User/Global/Internal)

**Deliverables:**
- VariableBundle struct with User/Global/Internal fields
- YAML/JSON serialization tags (Internal not serialized)
- Namespace documentation

**Key Test:** User and Global serialize, Internal does not

---

### Task 0.5.2: Flatten() Method (2 hours, ~15 lines)
**Goal:** Promote User variables to top-level for template access

**Why:** Templates access `{{ .IP }}` not `{{ .user.IP }}`

**Implementation:**
```go
func (v VariableBundle) Flatten() map[string]any {
    result := make(map[string]any)

    // User variables promoted to top-level
    for k, v := range v.User {
        result[k] = v
    }

    // Global and Internal nested
    result["global"] = v.Global
    result["internal"] = v.Internal

    return result
}
```

**Key Test:** `{{ .IP }}` renders from User, `{{ .global.api_endpoint }}` from Global

---

### Task 0.5.3: RenderTemplate() with Strict Mode (3 hours, ~40 lines)
**Goal:** Template rendering with Go text/template engine

**Features:**
- Strict mode: `missingkey=error` (fail on undefined variables)
- Generic type-safe rendering
- Error handling for template syntax errors

**Implementation:**
```go
func RenderTemplate[T any](tmpl string, data T) (string, error) {
    t := template.New("config").Option("missingkey=error")
    t, err := t.Parse(tmpl)
    if err != nil {
        return "", fmt.Errorf("parse template: %w", err)
    }

    var buf bytes.Buffer
    err = t.Execute(&buf, data)
    if err != nil {
        return "", fmt.Errorf("execute template: %w", err)
    }

    return buf.String(), nil
}
```

**Key Tests:**
- Valid template renders correctly
- Missing variable returns error (strict mode)
- Invalid syntax returns parse error

---

### Task 0.5.4: Location Computation (5 hours, ~90 lines)
**Goal:** Merge parent and child locations, fill ISA-95 gaps

**ISA-95 Hierarchy:**
```
Enterprise → Site → Area → Line → Cell
```

**Merge Rules:**
1. Parent location comes first
2. Child location appended
3. Fill gaps with empty strings
4. Compute location_path: "ACME.Factory-1.Line-A.Cell-5"

**Implementation:**
```go
func MergeLocations(parent, child []LocationLevel) []LocationLevel {
    // Start with parent levels
    merged := append([]LocationLevel{}, parent...)

    // Append child levels
    merged = append(merged, child...)

    // Fill ISA-95 gaps
    return fillGaps(merged)
}

func fillGaps(levels []LocationLevel) []LocationLevel {
    // Ensure all ISA-95 levels present (enterprise → cell)
    // Empty string for missing levels
}

func ComputeLocationPath(levels []LocationLevel) string {
    // Join non-empty levels with "."
    // Example: "ACME.Factory-1.Line-A.Cell-5"
}
```

**Key Tests:**
- Parent only: [enterprise, site] → fills area/line/cell
- Child only: [line, cell] → fills enterprise/site/area
- Parent + child: merges correctly
- location_path: joins non-empty levels

---

### Task 0.5.5: Variable Injection in Supervisor (2 hours, ~20 lines)
**Goal:** Inject Global and Internal variables before DeriveDesiredState()

**Integration Point:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // Get user spec from store
    userSpec := s.store.GetUserSpec()

    // Inject Global variables (from management loop)
    userSpec.Variables.Global = s.globalVars

    // Inject Internal variables (runtime metadata)
    userSpec.Variables.Internal = map[string]any{
        "id":         s.supervisorID,
        "created_at": s.createdAt,
        "bridged_by": s.parentID, // If child
    }

    // Derive desired state (worker gets complete VariableBundle)
    desiredState := s.worker.DeriveDesiredState(userSpec)

    // ...
}
```

**Key Tests:**
- Global variables injected correctly
- Internal variables computed correctly
- Worker receives complete VariableBundle

---

### Task 0.5.6: Integration with DeriveDesiredState (2 hours, ~25 lines)
**Goal:** Workers use VariableBundle for child ChildSpec creation

**Example (ProtocolConverter):**
```go
func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Template for child config
    childConfigTmpl := `
input:
  mqtt:
    urls: ["{{ .connection_url }}"]
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"
`

    // Render child config with variables
    childConfig, err := RenderTemplate(childConfigTmpl, userSpec.Variables.Flatten())

    return DesiredState{
        State: "running",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "source_flow",
                WorkerType: "benthos_dataflow",
                UserSpec: UserSpec{
                    Config: childConfig,
                    Variables: VariableBundle{
                        User: map[string]any{
                            "name": "source_flow",
                        },
                    },
                },
            },
        },
    }
}
```

**Key Tests:**
- Template renders with parent variables
- Child inherits location_path
- Child gets own variables

---

### Task 0.5.7: Integration Tests (4 hours, ~105 lines)
**Scenarios:**
1. **Variable flattening** - User variables accessible at top-level
2. **Template rendering** - Config generated from template + variables
3. **Location computation** - Parent + child → complete ISA-95 path
4. **Variable propagation** - Parent variables available to child workers
5. **Strict mode enforcement** - Missing variable → error
6. **Nested variables** - Global and Internal accessible via namespaces
7. **Empty locations** - Missing levels filled with empty strings

---

## Detailed TDD Task Breakdowns

**For complete RED→GREEN→REFACTOR cycles for each task**, see:
**[archive/fsmv2-change-proposal-section3.md](archive/fsmv2-change-proposal-section3.md)** (1,934 lines)

That document includes:
- Step-by-step test writing (RED phase)
- Expected test failures with exact output
- Minimal implementation (GREEN phase)
- Refactoring guidelines (REFACTOR phase)
- Commit message templates
- Code review checkpoints

---

## Design Documents

See [design/](design/) folder for detailed rationale:

- **[fsmv2-idiomatic-templating-and-variables.md](design/fsmv2-idiomatic-templating-and-variables.md)** - Complete variable system design
- **[fsmv2-templating-vs-statemapping.md](design/fsmv2-templating-vs-statemapping.md)** - When to use templates vs StateMapping
- **[fsmv2-derive-desired-state-complete-definition.md](design/fsmv2-derive-desired-state-complete-definition.md)** - Integration with DeriveDesiredState()

---

## Acceptance Criteria

### Functionality
- [ ] VariableBundle has User/Global/Internal namespaces
- [ ] Flatten() promotes User variables to top-level
- [ ] RenderTemplate() renders Go templates with strict mode
- [ ] Location merging combines parent + child
- [ ] Location path computed correctly (ISA-95 hierarchy)
- [ ] Global variables injected by supervisor
- [ ] Internal variables computed by supervisor
- [ ] Workers can use VariableBundle in DeriveDesiredState()

### Quality
- [ ] Unit test coverage >85% (template rendering critical)
- [ ] All integration tests pass (7 scenarios)
- [ ] No focused specs (`FIt`, `FDescribe`)
- [ ] Ginkgo tests use BDD style

### Performance
- [ ] Template rendering <5ms per template
- [ ] Location computation <1ms per merge
- [ ] Variable flattening <1ms

### Documentation
- [ ] VariableBundle fields documented
- [ ] RenderTemplate() usage examples
- [ ] Location computation rules documented
- [ ] Design docs updated

---

## Review Checkpoint

**After Phase 0.5 completion:**
- Code review focuses on template rendering security (no code injection)
- Verify variable propagation works parent → child → grandchild
- Confirm location computation handles all ISA-95 levels
- Check template strict mode prevents typos

**Next Phase:** [Phase 1: Infrastructure Supervision](phase-1-infrastructure-supervision.md)
