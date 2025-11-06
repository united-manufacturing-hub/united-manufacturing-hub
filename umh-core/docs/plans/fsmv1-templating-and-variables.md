# FSMv1: Templating and Variables System

**Date:** 2025-11-02
**Status:** Analysis
**Related:**
- `fsmv2-child-specs-in-desired-state.md` - Child specifications in DesiredState
- `fsmv2-templating-vs-statemapping.md` - Comparison of templating approaches for FSMv2

## Overview

FSMv1 uses a sophisticated templating and variable system to generate runtime configurations from user-defined templates. This document analyzes how it works and what patterns FSMv2 can adopt.

**Key Components:**
- `BuildRuntimeConfig()` - Merges variables and renders templates
- `VariableBundle` - Three-tier variable structure (User, Global, Internal)
- `Flatten()` - Converts nested namespaces to template-friendly flat structure
- `RenderTemplate()` - Executes Go text/template with strict mode

---

## Variable Bundle Structure

### Three-Tier Namespace System

```go
// pkg/config/variables/variables.go
type VariableBundle struct {
    User     map[string]any `yaml:"user,omitempty"`      // User-defined variables
    Global   map[string]any `yaml:"global,omitempty"`    // Fleet-wide variables
    Internal map[string]any `yaml:"internal,omitempty"`  // Runtime-only variables
}
```

**Namespace Purposes:**

1. **User** - Configuration variables from YAML
   - Connection variables (IP, PORT, etc.)
   - Location hierarchy
   - Protocol-specific settings
   - Computed values (location_path)

2. **Global** - Fleet-wide shared variables
   - Injected by central management loop
   - Available to all protocol converters
   - Example: shared API endpoints, cluster settings

3. **Internal** - Runtime-only metadata
   - Not persisted to YAML
   - Generated during runtime
   - Example: `id`, `bridged_by` header

### Variable Flattening

**CRITICAL PATTERN:** Nested variables become top-level in templates!

```go
// pkg/config/variables/variables.go:34-50
func (vb VariableBundle) Flatten() map[string]any {
    out := map[string]any{}

    // User variables become TOP-LEVEL
    for k, v := range vb.User {
        out[k] = v
    }

    // Global and Internal stay NESTED
    if len(vb.Global) > 0 {
        out["global"] = vb.Global
    }
    if len(vb.Internal) > 0 {
        out["internal"] = vb.Internal
    }

    return out
}
```

**Example:**

```yaml
# In config.yaml:
variables:
  IP: "192.168.1.100"
  PORT: 502

# After Flatten():
{
  "IP": "192.168.1.100",           // TOP-LEVEL (not .user.IP)
  "PORT": 502,                     // TOP-LEVEL (not .user.PORT)
  "location": {...},               // TOP-LEVEL
  "location_path": "ACME.Factory", // TOP-LEVEL
  "global": {...},                 // NESTED
  "internal": {...}                // NESTED
}

# In templates:
{{ .IP }}                  # ✅ Correct
{{ .user.IP }}             # ❌ Wrong - doesn't exist
{{ .global.api_endpoint }} # ✅ Correct
{{ .internal.id }}         # ✅ Correct
```

**Why flatten User variables?**
- User-friendly YAML (no namespace nesting required)
- Shorter template syntax (`{{ .IP }}` vs `{{ .user.IP }}`)
- Matches intuitive expectation ("my variables are at the top level")

---

## Location Computation

### Location Hierarchy

**Two sources merged:**
1. Agent location (authoritative) - from `agent.location` in config
2. Protocol Converter location (extension) - from `protocolConverter.location`

**Merge algorithm:**
```go
// pkg/service/protocolconverter/runtime_config/runtime_config.go:99-111
loc := map[string]string{}

// 1. Copy agent levels (authoritative)
for k, v := range agentLocation {
    loc[k] = v
}

// 2. Extend with PC-local additions (never overwrite agent keys)
for k, v := range pcLocation {
    if agentValue, exists := loc[k]; !exists || agentValue == "" {
        loc[k] = v
    }
}

// 3. Fill gaps up to highest level with "unknown"
// ...
```

**Example:**

```yaml
# Agent location:
agent:
  location:
    - enterprise: "ACME"          # Level 0
    - site: "Factory-1"           # Level 1

# Protocol Converter location:
protocolConverter:
  location:
    - line: "Line-A"              # Level 2
    - cell: "Cell-5"              # Level 3

# Merged result:
location:
  "0": "ACME"         # From agent
  "1": "Factory-1"    # From agent
  "2": "Line-A"       # From PC (extension)
  "3": "Cell-5"       # From PC (extension)

# Computed path:
location_path: "ACME.Factory-1.Line-A.Cell-5"
```

**Gap Filling with "unknown":**

```yaml
# Config with gaps:
location:
  "0": "ACME"
  "3": "Cell-5"  # Levels 1, 2 missing!

# After gap filling:
location:
  "0": "ACME"
  "1": "unknown"  # Filled
  "2": "unknown"  # Filled
  "3": "Cell-5"

# Path:
location_path: "ACME.unknown.unknown.Cell-5"
```

**Design Decision:** Fill gaps instead of erroring:
- Runs in reconciliation loop (retry wouldn't fix config)
- Validation belongs in parsing phase, not runtime
- "unknown" makes the gap observable in logs/metrics
- Users can fix and system will recover on next reconcile

---

## Template Rendering

### RenderTemplate() Process

**Type-safe generic template execution:**

```go
// pkg/config/templating.go:216-250
func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
    // A. Serialize to YAML (preserves structure)
    raw, err := yaml.Marshal(tmpl)

    // B. Parse + execute template with strict mode
    tpl, err := template.New("pc").Option("missingkey=error").Parse(string(raw))

    var buf bytes.Buffer
    err := tpl.Execute(&buf, scope)

    // C. Unmarshal back to same Go type
    var out T
    err := yaml.Unmarshal(buf.Bytes(), &out)

    // D. Sanity check - no {{ left over
    if bytes.Contains(buf.Bytes(), []byte("{{")) {
        return *new(T), fmt.Errorf("unresolved template markers")
    }

    return out, nil
}
```

**Key features:**
- **Generic:** Works with any struct type
- **Strict mode:** `missingkey=error` fails on undefined variables
- **Type-preserving:** Input type T → Output type T
- **Sandboxed:** No custom template functions
- **Validated:** Ensures no template directives remain

### BuildRuntimeConfig() Workflow

**Complete flow from spec to runtime config:**

```go
// pkg/service/protocolconverter/runtime_config/runtime_config.go:74-204
func BuildRuntimeConfig(
    spec ProtocolConverterServiceConfigSpec,
    agentLocation map[string]string,
    globalVars map[string]any,
    nodeName string,
    pcName string,
) (ProtocolConverterServiceConfigRuntime, error) {

    // 1. Merge location maps
    loc := mergeLocations(agentLocation, spec.Location)
    locationPath := computePath(loc)

    // 2. Assemble variable bundle
    vb := spec.Variables
    vb.User["location"] = loc
    vb.User["location_path"] = locationPath
    vb.Global = globalVars
    vb.Internal = map[string]any{
        "id": pcName,
        "bridged_by": GenerateBridgedBy(...),
    }

    // 3. Flatten variables for templates
    scope := vb.Flatten()

    // 4. Render all three sub-templates
    conn, _ := RenderTemplate(spec.GetConnectionServiceConfig(), scope)
    read, _ := RenderTemplate(spec.GetDFCReadServiceConfig(), scope)
    write, _ := RenderTemplate(spec.GetDFCWriteServiceConfig(), scope)

    return ProtocolConverterServiceConfigRuntime{
        ConnectionServiceConfig: conn,
        DataflowComponentReadServiceConfig: read,
        DataflowComponentWriteServiceConfig: write,
    }, nil
}
```

**Template execution example:**

```yaml
# Template (before):
input:
  modbus_tcp:
    address: "{{ .IP }}:{{ .PORT }}"

output:
  kafka:
    topic: "umh.v1.{{ .location_path }}.{{ .internal.id }}"

# Variables:
{
  "IP": "192.168.1.100",
  "PORT": 502,
  "location_path": "ACME.Factory.Line-A",
  "internal": {"id": "bridge-123"}
}

# Rendered (after):
input:
  modbus_tcp:
    address: "192.168.1.100:502"

output:
  kafka:
    topic: "umh.v1.ACME.Factory.Line-A.bridge-123"
```

---

## Type Conversion Pattern

### Template Types vs Runtime Types

**Problem:** YAML templates use strings, but runtime needs proper types (uint16, etc.)

**Solution:** Two-phase conversion:

```go
// Phase 1: Render template (strings)
connTemplate, _ := RenderTemplate(spec.GetConnectionServiceConfig(), scope)

// Phase 2: Convert to runtime types
connRuntime, _ := ConvertTemplateToRuntime(connTemplate)
```

**Example - Port conversion:**

```go
// Template type (before rendering):
type ConnectionServiceConfigTemplate struct {
    Port string `yaml:"port"`  // "{{ .PORT }}" or "443"
}

// After template rendering:
ConnectionServiceConfigTemplate{Port: "443"}

// Runtime type (after conversion):
type ConnectionServiceConfigRuntime struct {
    Port uint16 `yaml:"port"`  // 443
}
```

**Why two types?**
- Templates need strings (YAML can't template uint16)
- Runtime needs proper types (type safety, validation)
- Conversion handles parsing errors explicitly

---

## Parent-Child Variable Flow

### How Parents Pass Variables to Children

**FSMv1 Pattern:** Parent computes merged variables, passes to child config generation

**Current FSMv1 (no hierarchical composition in v1):**
- Agent computes location + global variables
- Protocol Converter gets variables from Agent
- Template rendering happens in BuildRuntimeConfig

**Hypothetical Parent-Child Flow (if FSMv1 had it):**

```go
// Parent worker
func (w *ParentWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Compute child variables
    childVars := VariableBundle{
        User: map[string]any{
            "IP": userSpec.IPAddress,
            "PORT": userSpec.Port,
            "parent_state": "active",  // Pass parent state
        },
    }

    // Create child spec with variables
    childSpec := ChildSpec{
        Name: "connection",
        WorkerType: "ConnectionWorker",
        UserSpec: UserSpec{
            Variables: childVars,
        },
    }

    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{childSpec},
    }
}

// Child worker
func (w *ChildWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Variables already in userSpec.Variables
    scope := userSpec.Variables.Flatten()

    // Render child's own config with variables
    config, _ := RenderTemplate(w.configTemplate, scope)

    // Use parent_state variable
    state := "down"
    if scope["parent_state"] == "active" {
        state = "up"
    }

    return DesiredState{State: state}
}
```

---

## Patterns for FSMv2 Adoption

### What FSMv2 Should Adopt

**1. Variable Bundle Structure**
```go
// ADOPT: Three-tier namespace
type VariableBundle struct {
    User     map[string]any  // User-defined + location
    Global   map[string]any  // Fleet-wide shared
    Internal map[string]any  // Runtime metadata
}
```

**2. Flatten() Method**
```go
// ADOPT: User variables at top level for template convenience
func (vb VariableBundle) Flatten() map[string]any {
    out := map[string]any{}
    for k, v := range vb.User {
        out[k] = v  // Top-level access
    }
    out["global"] = vb.Global    // Nested
    out["internal"] = vb.Internal // Nested
    return out
}
```

**3. RenderTemplate() with Strict Mode**
```go
// ADOPT: Generic, type-safe, strict template execution
func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
    tpl, _ := template.New("").Option("missingkey=error").Parse(raw)
    // ... execute and validate
}
```

**4. Location Computation**
```go
// ADOPT: Parent location + child location = merged hierarchy
loc := mergeLocations(parentLocation, childLocation)
locationPath := strings.Join(pathParts, ".")
```

### What FSMv2 Should Adapt

**Parent State in Variables:**

FSMv1 doesn't have hierarchical composition, but FSMv2 does. Extend the pattern:

```go
// Parent passes its state to child
childVars := VariableBundle{
    User: map[string]any{
        "parent_state": parentDesiredState,      // NEW: Parent's desired state
        "parent_observed": parentObservedState,  // NEW: Parent's observed state
        // ... existing variables
    },
}
```

**Child Uses Parent State:**

```go
// Child DeriveDesiredState gets parent state from variables
func (w *ChildWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    // Access parent state
    parentState := scope["parent_state"].(string)

    // Compute child state based on parent
    childState := "down"
    if parentState == "active" || parentState == "idle" {
        childState = "up"
    }

    return DesiredState{State: childState}
}
```

### What FSMv2 Should Skip

**Multi-phase conversion (Template → Runtime types):**

FSMv1 needs this because:
- UserSpec contains YAML templates (strings)
- Runtime needs proper types (uint16, etc.)

FSMv2 doesn't need this because:
- UserSpec already has proper types (Go structs)
- Templates only in ChildSpec if we use templating approach
- Can template directly in child's DeriveDesiredState

---

## FSMv1 vs FSMv2 Comparison

| Aspect | FSMv1 | FSMv2 (Proposed) |
|--------|-------|------------------|
| **Variable Source** | Agent + Global + Internal | Parent + Agent + Global + Internal |
| **Location Merge** | Agent + PC | Parent + Child |
| **Template Execution** | BuildRuntimeConfig (centralized) | Child's DeriveDesiredState (distributed) |
| **Variable Flattening** | User → top-level | Same |
| **Namespace Structure** | User/Global/Internal | Same + Parent state |
| **Parent-Child Flow** | N/A (no hierarchy) | Parent passes variables in ChildSpec |
| **Type Conversion** | Template → Runtime (2-phase) | Direct (1-phase, structs already typed) |

---

## Key Takeaways

1. **Variable Flattening is Critical**
   - User variables at top level: `{{ .IP }}` not `{{ .user.IP }}`
   - Global/Internal stay nested: `{{ .global.api }}`, `{{ .internal.id }}`

2. **Location Computation is Hierarchical**
   - Parent location + child location = merged path
   - Gap filling with "unknown" for graceful degradation

3. **Template Rendering is Strict**
   - `missingkey=error` catches undefined variables
   - Type-safe generic execution
   - Validates no templates remain after rendering

4. **Three-Tier Namespaces Work Well**
   - User: Config + location
   - Global: Fleet-wide shared
   - Internal: Runtime metadata

5. **FSMv2 Can Extend the Pattern**
   - Add parent state/observed to variables
   - Child computes own state from parent state
   - Eliminates need for StateMapping (replaced by logic in child)

---

## Usage Examples

### FSMv1 Current Pattern

```go
// Protocol Converter gets variables from Agent
spec := ProtocolConverterServiceConfigSpec{
    Variables: VariableBundle{
        User: map[string]any{
            "IP": "192.168.1.100",
            "PORT": 502,
        },
    },
}

// BuildRuntimeConfig merges location, global, internal
runtime, _ := BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, pcName)

// Result: All templates rendered, ready for FSM
```

### FSMv2 Proposed Pattern

```go
// Parent passes variables to child
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    childVars := VariableBundle{
        User: map[string]any{
            "IP": userSpec.IPAddress,
            "PORT": userSpec.Port,
            "parent_state": "active",  // NEW
        },
    }

    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name: "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{Variables: childVars},
            },
        },
    }
}

// Child computes state from parent state
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    scope := userSpec.Variables.Flatten()

    state := "down"
    if scope["parent_state"] == "active" {
        state = "up"
    }

    return DesiredState{State: state}
}
```

---

## References

**Code Locations:**
- `pkg/config/variables/variables.go` - VariableBundle, Flatten()
- `pkg/config/templating.go` - RenderTemplate(), strict mode
- `pkg/service/protocolconverter/runtime_config/runtime_config.go` - BuildRuntimeConfig()
- `pkg/service/streamprocessor/runtime_config/runtime_config.go` - Similar pattern for stream processors

**Related Documents:**
- `fsmv2-child-specs-in-desired-state.md` - Architecture using child specifications
- `fsmv2-templating-vs-statemapping.md` - Comparison of approaches for FSMv2
