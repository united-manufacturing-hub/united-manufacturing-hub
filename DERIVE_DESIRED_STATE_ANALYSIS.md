# DeriveDesiredState Transformation Analysis

## What "Templating" Means in This Context

In the FSM v2 system, "templating" refers to the **transformation of user configuration into technical configuration**. It's not about UI templates but rather a systematic conversion pipeline:

```
User Configuration (Raw)
    ↓
Parse & Validate (UserSpec)
    ↓
Variable Flattening & Enrichment
    ↓
Template Rendering (Go text/template)
    ↓
Technical Configuration (DesiredState with rendered configs)
```

### Key Insight: Two-Layer Configuration Model

- **UserSpec** (input): User-friendly configuration with variables like `{{ .IP }}`, `{{ .PORT }}`
- **DesiredState** (output): Technical configuration with all variables resolved, ready for execution
- **ChildrenSpecs** (composition): Parent workers declare desired children within DesiredState

---

## DeriveDesiredState Method Signature and Semantics

```go
DeriveDesiredState(spec interface{}) (types.DesiredState, error)
```

### Specifications Requirements

The `spec` parameter is the user-provided configuration. It should be interpreted as follows:

1. **Type**: `interface{}` - Flexible to support any configuration format
2. **Typical Conversion**: Often cast to `types.UserSpec` which contains:
   - `Config string`: Raw YAML/JSON configuration with template variables
   - `Variables VariableBundle`: Three-tier namespace structure (User, Global, Internal)

3. **Required Inputs for Rendering**:
   - **Variables to render**: Comes from `UserSpec.Variables`
   - **Location context**: Parent and child ISA-95 hierarchy
   - **Additional metadata**: Worker ID, instance metadata, etc.

### Return Type: DesiredState

```go
type DesiredState struct {
    State         string      // "running", "stopped", "shutdown", etc.
    ChildrenSpecs []ChildSpec // Declarative child workers
}
```

The DesiredState **always must implement**:
```go
func (d DesiredState) ShutdownRequested() bool {
    return d.State == "shutdown"
}
```

---

## Configuration Transformation Patterns

### Pattern 1: Simple Pass-Through (Communicator Worker)

**Use Case**: Worker doesn't need templating, just validates spec

```go
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    // MVP: No complex templating, just return default desired state
    return fsmv2types.DesiredState{
        State:         "running",
        ChildrenSpecs: nil,  // No children for communicator
    }, nil
}
```

**Characteristics**:
- No template rendering
- No variable substitution
- No child declaration
- Pure deterministic output (no spec dependency)
- Useful for stateless, self-contained workers

---

### Pattern 2: Complex Templating with Children (TemplateWorker)

**Use Case**: Worker needs to expand templates and declare children

#### Step 1: Extract Variables from UserSpec
```go
userSpec := spec.(types.UserSpec)  // Type assertion
flattened := userSpec.Variables.Flatten()  // Flatten to top-level namespace
```

#### Step 2: Compute Location Path (Enrichment)
```go
// Merge parent and child location levels
mergedLocation := location.MergeLocations(w.ParentLocation, w.ChildLocation)

// Ensure all 5 ISA-95 levels are present (fill gaps with empty strings)
filledLocation := location.FillISA95Gaps(mergedLocation)

// Join non-empty levels with dots: "ACME.Factory-1.Line-A.Cell-5"
locationPath := location.ComputeLocationPath(filledLocation)

// Add computed value to flattened variables
flattened["location_path"] = locationPath
```

#### Step 3: Render Templates Using Strict Mode
```go
// Go's text/template with missingkey=error (strict mode)
// This catches missing variables early
childConfig, err := templating.RenderTemplate(template, flattened)
if err != nil {
    return types.DesiredState{}, fmt.Errorf("render template: %w", err)
}
```

**Strict Mode Behavior**:
- Template syntax: `{{ .IP }}`, `{{ .PORT }}`, `{{ .location_path }}`
- If variable is missing: Error is returned (not silently null)
- Helps catch configuration mistakes early

#### Step 4: Build ChildrenSpecs with Rendered Configs
```go
return types.DesiredState{
    State: "running",
    ChildrenSpecs: []types.ChildSpec{
        {
            Name:       "mqtt_source",
            WorkerType: "benthos_dataflow",
            UserSpec: types.UserSpec{
                Config: childConfig,  // Now with all variables rendered
                Variables: types.VariableBundle{
                    User: map[string]any{
                        "name": "mqtt_source",
                    },
                },
            },
        },
    },
}, nil
```

**Key Pattern**:
- Parent renders its template into child's `UserSpec.Config`
- Child receives rendered config (no more template syntax)
- Child's UserSpec gets new Variables (different from parent's)
- Supervisor handles child creation/lifecycle

---

## Variable Flattening Semantics

### VariableBundle Structure (Three-Tier)

```go
type VariableBundle struct {
    User     map[string]any  // User variables - top-level in templates
    Global   map[string]any  // Global settings - nested as {{ .global.key }}
    Internal map[string]any  // Runtime metadata - nested as {{ .internal.key }}
}
```

### Flatten() Method: Converts to Template Data

```go
flattened := bundle.Flatten()
// Returns:
// {
//     "IP": "192.168.1.100",           // From User namespace
//     "PORT": 502,                      // From User namespace
//     "name": "test_bridge",            // From User namespace
//     "global": {...},                  // Global namespace nested
//     "internal": {...},                // Internal namespace nested
//     "location_path": "ACME.Factory-1"  // Computed/enriched value
// }
```

### Template Access Patterns

```yaml
# Template syntax:
input:
  modbus_tcp:
    address: "{{ .IP }}:{{ .PORT }}"
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .name }}"
    api_endpoint: "{{ .global.api_endpoint }}"
```

**Critical Rule**: Variables in User namespace become **top-level** (not nested).
- ✅ Correct: `{{ .IP }}` 
- ❌ Wrong: `{{ .user.IP }}`

---

## Spec Parameter Structure and Validation

### Typical UserSpec Pattern

```go
type UserSpec struct {
    Config    string         // YAML/JSON with {{ .VARIABLE }} syntax
    Variables VariableBundle // Three namespaces for substitution
}
```

### Validation Checklist (DeriveDesiredState Responsibility)

1. **Type Assertion Safety**
   ```go
   userSpec, ok := spec.(types.UserSpec)
   if !ok {
       return types.DesiredState{}, fmt.Errorf("spec must be UserSpec, got %T", spec)
   }
   ```

2. **Required Variables**
   ```go
   if userSpec.Variables.User["IP"] == nil {
       return types.DesiredState{}, fmt.Errorf("IP variable required")
   }
   ```

3. **Template Syntax Validation**
   - Handled by `templating.RenderTemplate()` with strict mode
   - Missing variables trigger errors automatically
   - Invalid syntax (e.g., `{{ .IP`) also errors

4. **Child Spec Validation**
   ```go
   // Ensure ChildSpec fields are valid
   for _, child := range desiredState.ChildrenSpecs {
       if child.Name == "" {
           return types.DesiredState{}, fmt.Errorf("child name required")
       }
       if child.WorkerType == "" {
           return types.DesiredState{}, fmt.Errorf("child workerType required")
       }
   }
   ```

---

## Default Value Handling Patterns

### Pattern: Computed Defaults (Location Path)

```go
// Compute location_path if not provided
if _, exists := flattened["location_path"]; !exists {
    flattened["location_path"] = location.ComputeLocationPath(...)
}
```

### Pattern: Merge with Defaults

```go
// Start with defaults, override with user values
defaults := map[string]any{
    "timeout": 30,
    "retries": 3,
}

for k, v := range userVariables {
    defaults[k] = v  // User overrides defaults
}

flattened := defaults  // Use merged map for rendering
```

### Pattern: Conditional Defaults

```go
// Apply defaults based on state
if userSpec.Variables.User["mode"] == "pull" {
    flattened["interval"] = 1000  // Default poll interval
} else {
    flattened["interval"] = 5000  // Different default for push
}
```

---

## Error Handling Patterns

### Pattern 1: Type Assertion with Error

```go
userSpec, ok := spec.(types.UserSpec)
if !ok {
    return types.DesiredState{}, fmt.Errorf(
        "DeriveDesiredState expects UserSpec, got %T", 
        spec,
    )
}
```

### Pattern 2: Template Rendering Errors (Caught by Strict Mode)

```go
config, err := templating.RenderTemplate(template, flattened)
if err != nil {
    // Errors include:
    // - Missing variables: "map has no entry for key \"PORT\""
    // - Invalid syntax: "unclosed action"
    return types.DesiredState{}, fmt.Errorf(
        "failed to render template for child %s: %w", 
        childName, 
        err,
    )
}
```

### Pattern 3: Validation Errors

```go
if len(userSpec.Config) == 0 {
    return types.DesiredState{}, fmt.Errorf(
        "config cannot be empty for worker %s", 
        w.identity.Name,
    )
}
```

### Pattern 4: Child Specification Errors

```go
for _, child := range desiredState.ChildrenSpecs {
    if child.Name == "" {
        return types.DesiredState{}, fmt.Errorf(
            "child at index %d missing required name", 
            i,
        )
    }
}
```

### Error Propagation Pattern

Always wrap errors with context about what operation failed:

```go
// ❌ Bad: lose context
return types.DesiredState{}, err

// ✅ Good: preserve context
return types.DesiredState{}, fmt.Errorf(
    "derive desired state for worker %q: %w", 
    w.identity.Name, 
    err,
)
```

---

## Pure Function Requirements (No Side Effects)

### Requirements Checklist

✅ **MUST**:
- Accept spec parameter and return DesiredState (no other outputs)
- Be deterministic (same input → same output)
- Not modify any receiver fields
- Not call external APIs
- Not write to filesystem or databases
- Not read system clock (use parent context if timing needed)
- Handle nil spec gracefully

❌ **MUST NOT**:
- Mutate `w` (the worker instance)
- Maintain state between calls
- Depend on global variables
- Perform I/O operations
- Have race conditions

### Example: Pure vs Impure

```go
// ✅ PURE: Deterministic, no side effects
func (w *MyWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
    userSpec := spec.(types.UserSpec)
    flattened := userSpec.Variables.Flatten()
    config, err := templating.RenderTemplate(w.Template, flattened)
    if err != nil {
        return types.DesiredState{}, err
    }
    return types.DesiredState{State: "running", ChildrenSpecs: [...]}, nil
}

// ❌ IMPURE: Side effects and non-determinism
func (w *MyWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
    w.lastCallTime = time.Now()  // SIDE EFFECT: Mutates receiver
    
    file, _ := os.Open("/etc/config")  // SIDE EFFECT: I/O
    defer file.Close()
    
    rand.Seed(time.Now().UnixNano())  // NON-DETERMINISTIC
    
    http.Get("https://api.example.com")  // SIDE EFFECT: External API
    
    return types.DesiredState{}, nil
}
```

---

## Connection to umh-core's Benthos Template Expansion

### How TemplateWorker Relates to umh-core

1. **umh-core Agent** (legacy):
   - Reads user config from YAML
   - Has built-in template expansion logic
   - Generates benthos configs inline

2. **FSM v2 TemplateWorker** (modern):
   - Extracts template expansion into reusable pattern
   - Enables parent workers to declare children
   - Supports multi-child scenarios (source + sink)
   - Uses same template variable flattening

3. **Key Difference**:
   - Old: Agent does all template expansion centrally
   - New: Each worker responsible for its own transformation
   - Enables hierarchical composition (parent → children → grandchildren)

### Template Variable Mapping

Both systems use same variable flattening:

```yaml
# In UserSpec (FSM v2) or config.yaml (legacy):
connection:
  variables:
    IP: "192.168.1.100"
    PORT: 502

# Flattens to (both systems):
{{ .IP }}      # NOT {{ .variables.IP }}
{{ .PORT }}    # NOT {{ .variables.PORT }}
```

---

## What an Example Worker Should Demonstrate

### Minimal Worker: Simple Pass-Through
File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go`

Shows:
- Ignores spec (no templating needed)
- Returns fixed DesiredState
- No child declaration
- Pure function

### Complex Worker: Full Templating Pipeline
File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker.go`

Shows:
- Type assertion on spec
- Variable flattening
- Location path computation
- Template rendering with strict mode
- Single-child and multi-child variants
- Child variable isolation

### Test Coverage
File: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker_test.go`

Demonstrates:
- Variable substitution results
- Missing variable error handling
- Invalid template syntax error handling
- Location path computation with gaps
- Multiple children with different templates
- Error cases in strict mode

---

## Summary: Key Patterns

| Aspect | Simple (Communicator) | Complex (TemplateWorker) |
|--------|----------------------|--------------------------|
| **Spec Handling** | Ignored | Type-asserted to UserSpec |
| **Variables** | None | Flattened from VariableBundle |
| **Enrichment** | None | Location path computed |
| **Templating** | None | Go text/template (strict) |
| **Children** | None | 1+ ChildSpecs declared |
| **Error Cases** | None | Missing vars, syntax errors |
| **Use Case** | Stateless workers | Config transformation workers |

---

## Distinguishing Features

### Simple Pass-Through vs Complex Templating

1. **Does spec get type-asserted?**
   - Simple: No, ignored
   - Complex: Yes, to UserSpec

2. **Are variables used in templates?**
   - Simple: No variables
   - Complex: Yes, via flattening

3. **Are children declared?**
   - Simple: No ChildrenSpecs
   - Complex: Yes, 1+ specs

4. **Is template rendering needed?**
   - Simple: No templating.RenderTemplate()
   - Complex: Yes, with strict mode

5. **Does enrichment happen?**
   - Simple: None
   - Complex: Location path, metadata, etc.

---

## Concrete Examples from Codebase

### Example 1: Communicator (Simple)
Location: `worker.go` lines 254-261

```go
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
    return fsmv2types.DesiredState{
        State:         "running",
        ChildrenSpecs: nil,
    }, nil
}
```

**Why it's simple**:
- Spec completely ignored
- No variable processing
- No template rendering
- No children
- Pure constant function

### Example 2: TemplateWorker (Complex)
Location: `examples/template_worker.go` lines 51-89

```go
func (w *TemplateWorker) DeriveDesiredState(userSpec types.UserSpec) (types.DesiredState, error) {
    flattened := userSpec.Variables.Flatten()
    
    mergedLocation := location.MergeLocations(w.ParentLocation, w.ChildLocation)
    filledLocation := location.FillISA95Gaps(mergedLocation)
    locationPath := location.ComputeLocationPath(filledLocation)
    flattened["location_path"] = locationPath
    
    if w.MultiChild {
        return w.deriveMultiChild(flattened)
    }
    return w.deriveSingleChild(flattened)
}

func (w *TemplateWorker) deriveSingleChild(flattened map[string]any) (types.DesiredState, error) {
    childConfig, err := templating.RenderTemplate(w.Template, flattened)
    if err != nil {
        return types.DesiredState{}, fmt.Errorf("render template: %w", err)
    }
    
    return types.DesiredState{
        State: "running",
        ChildrenSpecs: []types.ChildSpec{
            {
                Name:       "mqtt_source",
                WorkerType: "benthos_dataflow",
                UserSpec: types.UserSpec{
                    Config: childConfig,
                    Variables: types.VariableBundle{
                        User: map[string]any{"name": "mqtt_source"},
                    },
                },
            },
        },
    }, nil
}
```

**Why it's complex**:
- Full spec handling with type conversion
- Variable flattening from VariableBundle
- Location path computation and enrichment
- Template rendering with error handling
- Child declaration with rendered config
- Multi-variant support (single vs multi-child)

---

## Files to Reference

| File | Purpose |
|------|---------|
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/worker.go` | Worker interface definition with DeriveDesiredState documentation |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/workers/communicator/worker.go` | Simple example (pass-through) |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker.go` | Complex example (full templating pipeline) |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/examples/template_worker_test.go` | Test coverage and validation patterns |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/templating/template.go` | Template rendering with strict mode |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/location/location.go` | Location path computation |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/types/childspec.go` | ChildSpec and DesiredState definitions |
| `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/types/variables.go` | VariableBundle structure and flattening |
