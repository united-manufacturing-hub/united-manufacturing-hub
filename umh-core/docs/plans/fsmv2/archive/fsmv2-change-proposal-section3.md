## 3. New Phase 0.5: Templating & Variables

This section provides complete TDD task breakdowns for Phase 0.5 (Templating & Variables System).

**Total Effort:** 2 weeks, ~325 lines of code

**Dependencies:** Phase 0 (Variables in UserSpec)

**Blocks:** Benthos worker implementation

---

### Task 0.5.1: VariableBundle Structure

**Goal:** Define three-tier namespace structure (User/Global/Internal)

**Why:** Foundation for all variable handling, namespaces separate concerns

**Location:** `pkg/fsmv2/variables/variables.go`

**Effort:** 2 hours, ~30 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/variables/variables_test.go`

```go
package variables_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/variables"
)

var _ = Describe("VariableBundle", func() {
    Describe("Structure", func() {
        It("should have User namespace for user-defined variables", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP":   "192.168.1.100",
                    "PORT": 502,
                },
            }
            
            Expect(bundle.User).To(HaveKey("IP"))
            Expect(bundle.User["IP"]).To(Equal("192.168.1.100"))
            Expect(bundle.User["PORT"]).To(Equal(502))
        })
        
        It("should have Global namespace for fleet-wide variables", func() {
            bundle := variables.VariableBundle{
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                    "tenant_id":    "acme-corp",
                },
            }
            
            Expect(bundle.Global).To(HaveKey("api_endpoint"))
            Expect(bundle.Global["api_endpoint"]).To(Equal("https://api.umh.app"))
        })
        
        It("should have Internal namespace for runtime metadata", func() {
            bundle := variables.VariableBundle{
                Internal: map[string]any{
                    "id":         "worker-123",
                    "created_at": "2025-11-03T10:00:00Z",
                },
            }
            
            Expect(bundle.Internal).To(HaveKey("id"))
            Expect(bundle.Internal["id"]).To(Equal("worker-123"))
        })
        
        It("should serialize User and Global namespaces only", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP": "192.168.1.100",
                },
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                },
                Internal: map[string]any{
                    "id": "worker-123",
                },
            }
            
            // YAML serialization should include User and Global
            yamlData, err := yaml.Marshal(bundle)
            Expect(err).NotTo(HaveOccurred())
            
            yamlStr := string(yamlData)
            Expect(yamlStr).To(ContainSubstring("user:"))
            Expect(yamlStr).To(ContainSubstring("global:"))
            Expect(yamlStr).NotTo(ContainSubstring("internal:"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# VariableBundle
#   Structure
#     should have User namespace for user-defined variables [It]
#   
#   undefined: variables.VariableBundle
```

**Verify:** Test fails with `undefined: variables.VariableBundle`

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/variables/variables.go`

```go
// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0

package variables

// VariableBundle holds variables for a worker in three namespaces
type VariableBundle struct {
    // User variables: user-defined config + parent state + computed values
    // Serialized: YES
    // Template access: Top-level ({{ .IP }})
    User map[string]any `yaml:"user,omitempty" json:"user,omitempty"`
    
    // Global variables: fleet-wide settings from management loop
    // Serialized: YES
    // Template access: Nested ({{ .global.api_endpoint }})
    Global map[string]any `yaml:"global,omitempty" json:"global,omitempty"`
    
    // Internal variables: runtime metadata (id, timestamps, bridged_by)
    // Serialized: NO (runtime-only)
    // Template access: Nested ({{ .internal.id }})
    Internal map[string]any `yaml:"-" json:"-"`
}
```

**Why this design:**
- User: Top-level access, user-facing variables
- Global: Fleet-wide config, explicit namespace
- Internal: Runtime metadata, not serialized

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# Ran 4 of 4 Specs in 0.003 seconds
# SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/variables/
git commit -m "feat(fsmv2): add VariableBundle three-tier namespace structure

Implements User/Global/Internal namespaces:
- User: user-defined + parent state + computed values
- Global: fleet-wide settings
- Internal: runtime metadata (not serialized)

User and Global serialize to YAML/JSON, Internal is runtime-only.

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Structure correctness:**
   - Three namespaces: User, Global, Internal
   - User and Global serialize (yaml/json tags)
   - Internal does NOT serialize (yaml:"-")

2. **Test coverage:**
   - Each namespace tested separately
   - Serialization behavior tested
   - No focused specs (`FIt`, `FDescribe`)

3. **Documentation:**
   - Clear comments explaining each namespace
   - Template access pattern documented
   - Serialization behavior documented

**Ask:**
- Do namespace purposes match design document?
- Is serialization behavior correct?
- Are tests comprehensive?

---

### Task 0.5.2: Flatten() Implementation

**Goal:** Implement variable flattening for template convenience

**Why:** User variables promoted to top-level ({{ .IP }} not {{ .user.IP }})

**Location:** `pkg/fsmv2/variables/variables.go`

**Effort:** 1 hour, ~15 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/variables/variables_test.go`

```go
var _ = Describe("VariableBundle", func() {
    Describe("Flatten", func() {
        It("should promote User variables to top-level", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP":            "192.168.1.100",
                    "PORT":          502,
                    "parent_state":  "active",
                    "location_path": "ACME.Factory.Line-A",
                },
            }
            
            flattened := bundle.Flatten()
            
            // User variables at top level
            Expect(flattened).To(HaveKey("IP"))
            Expect(flattened).To(HaveKey("PORT"))
            Expect(flattened).To(HaveKey("parent_state"))
            Expect(flattened).To(HaveKey("location_path"))
            
            Expect(flattened["IP"]).To(Equal("192.168.1.100"))
            Expect(flattened["PORT"]).To(Equal(502))
            Expect(flattened["parent_state"]).To(Equal("active"))
        })
        
        It("should nest Global namespace under 'global' key", func() {
            bundle := variables.VariableBundle{
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                    "tenant_id":    "acme-corp",
                },
            }
            
            flattened := bundle.Flatten()
            
            // Global nested
            Expect(flattened).To(HaveKey("global"))
            globalMap := flattened["global"].(map[string]any)
            Expect(globalMap).To(HaveKey("api_endpoint"))
            Expect(globalMap["api_endpoint"]).To(Equal("https://api.umh.app"))
        })
        
        It("should nest Internal namespace under 'internal' key", func() {
            bundle := variables.VariableBundle{
                Internal: map[string]any{
                    "id":         "worker-123",
                    "created_at": "2025-11-03T10:00:00Z",
                },
            }
            
            flattened := bundle.Flatten()
            
            // Internal nested
            Expect(flattened).To(HaveKey("internal"))
            internalMap := flattened["internal"].(map[string]any)
            Expect(internalMap).To(HaveKey("id"))
            Expect(internalMap["id"]).To(Equal("worker-123"))
        })
        
        It("should combine all namespaces correctly", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP":   "192.168.1.100",
                    "PORT": 502,
                },
                Global: map[string]any{
                    "api_endpoint": "https://api.umh.app",
                },
                Internal: map[string]any{
                    "id": "worker-123",
                },
            }
            
            flattened := bundle.Flatten()
            
            // User at top
            Expect(flattened).To(HaveKey("IP"))
            Expect(flattened).To(HaveKey("PORT"))
            
            // Global nested
            Expect(flattened).To(HaveKey("global"))
            Expect(flattened["global"].(map[string]any)).To(HaveKey("api_endpoint"))
            
            // Internal nested
            Expect(flattened).To(HaveKey("internal"))
            Expect(flattened["internal"].(map[string]any)).To(HaveKey("id"))
        })
        
        It("should handle empty namespaces gracefully", func() {
            bundle := variables.VariableBundle{
                User: map[string]any{
                    "IP": "192.168.1.100",
                },
                // Global and Internal empty
            }
            
            flattened := bundle.Flatten()
            
            // User still at top
            Expect(flattened).To(HaveKey("IP"))
            
            // Empty namespaces not included
            Expect(flattened).NotTo(HaveKey("global"))
            Expect(flattened).NotTo(HaveKey("internal"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# VariableBundle
#   Flatten
#     should promote User variables to top-level [It]
#   
#   bundle.Flatten undefined (type variables.VariableBundle has no field or method Flatten)
```

**Verify:** Test fails with `Flatten undefined`

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/variables/variables.go`

```go
// Flatten converts VariableBundle to flat map for template rendering
//
// User variables promoted to top-level:
//   {{ .IP }} instead of {{ .user.IP }}
//
// Global and Internal variables nested:
//   {{ .global.api_endpoint }}
//   {{ .internal.id }}
//
// This matches FSMv1 proven pattern and user intuition.
func (vb VariableBundle) Flatten() map[string]any {
    out := make(map[string]any)
    
    // User variables → top-level (no namespace prefix)
    for k, v := range vb.User {
        out[k] = v
    }
    
    // Global variables → nested under "global" key
    if len(vb.Global) > 0 {
        out["global"] = vb.Global
    }
    
    // Internal variables → nested under "internal" key
    if len(vb.Internal) > 0 {
        out["internal"] = vb.Internal
    }
    
    return out
}
```

**Why this design:**
- User variables top-level: Matches user expectation
- Global/Internal nested: Explicit namespace, avoids conflicts
- Empty check: Don't add empty maps

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# Ran 9 of 9 Specs in 0.003 seconds
# SUCCESS! -- 9 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/variables/
git commit -m "feat(fsmv2): implement VariableBundle.Flatten() for template convenience

Flattens variable namespaces for template rendering:
- User variables → top-level ({{ .IP }})
- Global/Internal → nested ({{ .global.api_endpoint }})

Matches FSMv1 proven pattern and user intuition.

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Flattening correctness:**
   - User variables at top level
   - Global nested under "global"
   - Internal nested under "internal"
   - Empty namespaces omitted

2. **Test coverage:**
   - Each namespace tested separately
   - Combined namespaces tested
   - Empty namespace handling tested
   - No focused specs

3. **Documentation:**
   - Clear examples in comment
   - Rationale explained

**Ask:**
- Does flattening match design document?
- Is empty namespace handling correct?
- Are test cases comprehensive?

---

### Task 0.5.3: RenderTemplate() with Strict Mode

**Goal:** Implement template rendering with strict mode (missingkey=error)

**Why:** Catch undefined variables early, fail fast on template errors

**Location:** `pkg/fsmv2/variables/template.go`

**Effort:** 3 hours, ~40 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/variables/template_test.go`

```go
package variables_test

import (
    "text/template"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/variables"
)

var _ = Describe("Template Rendering", func() {
    Describe("RenderTemplate", func() {
        It("should render simple string templates", func() {
            tmpl := "address: {{ .IP }}:{{ .PORT }}"
            scope := map[string]any{
                "IP":   "192.168.1.100",
                "PORT": 502,
            }
            
            result, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(Equal("address: 192.168.1.100:502"))
        })
        
        It("should render nested variables", func() {
            tmpl := "topic: umh.v1.{{ .location_path }}.{{ .name }}"
            scope := map[string]any{
                "location_path": "ACME.Factory.Line-A",
                "name":          "bridge-plc",
            }
            
            result, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(Equal("topic: umh.v1.ACME.Factory.Line-A.bridge-plc"))
        })
        
        It("should render Global variables with namespace", func() {
            tmpl := "endpoint: {{ .global.api_endpoint }}"
            scope := map[string]any{
                "global": map[string]any{
                    "api_endpoint": "https://api.umh.app",
                },
            }
            
            result, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(Equal("endpoint: https://api.umh.app"))
        })
        
        It("should fail with undefined variables in strict mode", func() {
            tmpl := "address: {{ .IP }}:{{ .PORT }}"
            scope := map[string]any{
                "IP": "192.168.1.100",
                // PORT missing
            }
            
            _, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("PORT"))
        })
        
        It("should fail if template markers remain in output", func() {
            tmpl := "address: {{ .IP }}:{{ .PORT }}"
            scope := map[string]any{
                "IP":   "192.168.1.100",
                "PORT": "{{ .DYNAMIC_PORT }}",  // Nested template
            }
            
            _, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("unexpanded template markers"))
        })
        
        It("should handle complex templates with conditionals", func() {
            tmpl := `{{ if eq .parent_state "active" }}running{{ else }}stopped{{ end }}`
            scope := map[string]any{
                "parent_state": "active",
            }
            
            result, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(Equal("running"))
        })
        
        It("should handle struct templates", func() {
            type BenthosConfig struct {
                Input  string
                Output string
            }
            
            tmpl := BenthosConfig{
                Input:  "modbus_tcp:\n  address: {{ .IP }}:{{ .PORT }}",
                Output: "kafka:\n  topic: umh.v1.{{ .location_path }}",
            }
            scope := map[string]any{
                "IP":            "192.168.1.100",
                "PORT":          502,
                "location_path": "ACME.Factory.Line-A",
            }
            
            result, err := variables.RenderTemplate(tmpl, scope)
            Expect(err).NotTo(HaveOccurred())
            Expect(result.Input).To(ContainSubstring("192.168.1.100:502"))
            Expect(result.Output).To(ContainSubstring("ACME.Factory.Line-A"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# Template Rendering
#   RenderTemplate
#     should render simple string templates [It]
#   
#   undefined: variables.RenderTemplate
```

**Verify:** Test fails with `RenderTemplate undefined`

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/variables/template.go`

```go
// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0

package variables

import (
    "bytes"
    "fmt"
    "reflect"
    "strings"
    "text/template"
)

// RenderTemplate renders a template with strict mode enabled
//
// For string templates:
//   tmpl := "address: {{ .IP }}:{{ .PORT }}"
//   result, err := RenderTemplate(tmpl, scope)
//
// For struct templates (e.g., BenthosConfig):
//   tmpl := BenthosConfig{Input: "{{ .IP }}", Output: "{{ .PORT }}"}
//   result, err := RenderTemplate(tmpl, scope)
//
// Strict mode: missingkey=error (fails on undefined variables)
// Validation: No {{ }} markers in output (catches nested templates)
func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
    var zero T
    
    // Handle string templates
    if str, ok := any(tmpl).(string); ok {
        result, err := renderString(str, scope)
        if err != nil {
            return zero, err
        }
        return any(result).(T), nil
    }
    
    // Handle struct templates (recurse through fields)
    return renderStruct(tmpl, scope)
}

// renderString renders a single string template
func renderString(tmpl string, scope map[string]any) (string, error) {
    // Create template with strict mode
    t, err := template.New("template").
        Option("missingkey=error").
        Parse(tmpl)
    if err != nil {
        return "", fmt.Errorf("template parse failed: %w", err)
    }
    
    // Render template
    var buf bytes.Buffer
    if err := t.Execute(&buf, scope); err != nil {
        return "", fmt.Errorf("template execution failed: %w", err)
    }
    
    result := buf.String()
    
    // Validate: no unexpanded markers
    if strings.Contains(result, "{{") || strings.Contains(result, "}}") {
        return "", fmt.Errorf("unexpanded template markers in output: %s", result)
    }
    
    return result, nil
}

// renderStruct renders all string fields in a struct recursively
func renderStruct[T any](tmpl T, scope map[string]any) (T, error) {
    val := reflect.ValueOf(tmpl)
    
    // Only handle structs
    if val.Kind() != reflect.Struct {
        return tmpl, fmt.Errorf("unsupported template type: %T", tmpl)
    }
    
    // Create mutable copy
    result := reflect.New(val.Type()).Elem()
    result.Set(val)
    
    // Iterate fields
    for i := 0; i < result.NumField(); i++ {
        field := result.Field(i)
        
        // Skip unexported fields
        if !field.CanSet() {
            continue
        }
        
        // Render string fields
        if field.Kind() == reflect.String {
            rendered, err := renderString(field.String(), scope)
            if err != nil {
                return tmpl, fmt.Errorf("field %s: %w", result.Type().Field(i).Name, err)
            }
            field.SetString(rendered)
        }
        
        // Recursively render nested structs
        if field.Kind() == reflect.Struct {
            rendered, err := renderStruct(field.Interface(), scope)
            if err != nil {
                return tmpl, fmt.Errorf("field %s: %w", result.Type().Field(i).Name, err)
            }
            field.Set(reflect.ValueOf(rendered))
        }
    }
    
    return result.Interface().(T), nil
}
```

**Why this design:**
- Generic: Works with strings and structs
- Strict mode: Catches undefined variables early
- Validation: No unexpanded markers in output
- Recursive: Handles nested structs

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# Ran 16 of 16 Specs in 0.005 seconds
# SUCCESS! -- 16 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/variables/
git commit -m "feat(fsmv2): implement RenderTemplate with strict mode

Adds template rendering with:
- Strict mode: missingkey=error (fail on undefined vars)
- Validation: No unexpanded {{ }} markers in output
- Generic: Works with strings and structs
- Recursive: Handles nested struct fields

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Template rendering correctness:**
   - String templates render correctly
   - Struct templates recurse through fields
   - Strict mode enabled (missingkey=error)
   - Validation catches unexpanded markers

2. **Error handling:**
   - Undefined variables fail with clear error
   - Nested templates fail validation
   - Parse errors handled

3. **Test coverage:**
   - Simple templates tested
   - Nested variables tested
   - Global namespace tested
   - Error cases tested (undefined vars, nested templates)
   - Struct templates tested
   - No focused specs

4. **Documentation:**
   - Clear examples in comments
   - Rationale for strict mode explained
   - Validation purpose documented

**Ask:**
- Does strict mode match design document?
- Is validation comprehensive?
- Are error messages clear?
- Is struct recursion correct?

---

### Task 0.5.4: Location Computation

**Goal:** Implement location merging and path computation for ISA-95 hierarchy

**Why:** Auto-compute {{ .location_path }} from parent + child locations

**Location:** `pkg/fsmv2/variables/location.go`

**Effort:** 4 hours, ~90 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/variables/location_test.go`

```go
package variables_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/variables"
)

var _ = Describe("Location Computation", func() {
    Describe("MergeLocations", func() {
        It("should merge parent and child locations", func() {
            parent := map[string]string{
                "enterprise": "ACME",
                "site":       "Factory-1",
            }
            child := map[string]string{
                "line": "Line-A",
                "cell": "Cell-5",
            }
            
            merged := variables.MergeLocations(parent, child)
            
            Expect(merged).To(HaveKeyWithValue("enterprise", "ACME"))
            Expect(merged).To(HaveKeyWithValue("site", "Factory-1"))
            Expect(merged).To(HaveKeyWithValue("line", "Line-A"))
            Expect(merged).To(HaveKeyWithValue("cell", "Cell-5"))
        })
        
        It("should override parent values with child values", func() {
            parent := map[string]string{
                "enterprise": "ACME",
                "site":       "Factory-1",
            }
            child := map[string]string{
                "site": "Factory-2",  // Override
                "line": "Line-A",
            }
            
            merged := variables.MergeLocations(parent, child)
            
            Expect(merged["site"]).To(Equal("Factory-2"))
            Expect(merged["line"]).To(Equal("Line-A"))
        })
        
        It("should handle nil parent", func() {
            child := map[string]string{
                "line": "Line-A",
            }
            
            merged := variables.MergeLocations(nil, child)
            
            Expect(merged).To(HaveKeyWithValue("line", "Line-A"))
        })
        
        It("should handle nil child", func() {
            parent := map[string]string{
                "enterprise": "ACME",
            }
            
            merged := variables.MergeLocations(parent, nil)
            
            Expect(merged).To(HaveKeyWithValue("enterprise", "ACME"))
        })
    })
    
    Describe("ComputeLocationPath", func() {
        It("should compute path for complete ISA-95 hierarchy", func() {
            location := map[string]string{
                "enterprise":     "ACME",
                "site":           "Factory-1",
                "area":           "Assembly",
                "productionLine": "Line-A",
                "workCell":       "Cell-5",
            }
            
            path := variables.ComputeLocationPath(location)
            
            Expect(path).To(Equal("ACME.Factory-1.Assembly.Line-A.Cell-5"))
        })
        
        It("should compute path for partial hierarchy", func() {
            location := map[string]string{
                "enterprise": "ACME",
                "site":       "Factory-1",
            }
            
            path := variables.ComputeLocationPath(location)
            
            Expect(path).To(Equal("ACME.Factory-1"))
        })
        
        It("should fill gaps with 'unknown'", func() {
            location := map[string]string{
                "enterprise": "ACME",
                // site missing
                "area": "Assembly",
            }
            
            path := variables.ComputeLocationPath(location)
            
            Expect(path).To(Equal("ACME.unknown.Assembly"))
        })
        
        It("should handle UMH legacy naming (line/cell)", func() {
            location := map[string]string{
                "enterprise": "ACME",
                "site":       "Factory-1",
                "line":       "Line-A",
                "cell":       "Cell-5",
            }
            
            path := variables.ComputeLocationPath(location)
            
            // line → productionLine, cell → workCell
            Expect(path).To(Equal("ACME.Factory-1.unknown.Line-A.Cell-5"))
        })
        
        It("should handle empty location", func() {
            location := map[string]string{}
            
            path := variables.ComputeLocationPath(location)
            
            Expect(path).To(Equal(""))
        })
        
        It("should handle nil location", func() {
            path := variables.ComputeLocationPath(nil)
            
            Expect(path).To(Equal(""))
        })
        
        It("should normalize ISA-95 hierarchy order", func() {
            location := map[string]string{
                "workCell":       "Cell-5",
                "productionLine": "Line-A",
                "enterprise":     "ACME",
                "site":           "Factory-1",
                "area":           "Assembly",
            }
            
            path := variables.ComputeLocationPath(location)
            
            // Always enterprise.site.area.productionLine.workCell
            Expect(path).To(Equal("ACME.Factory-1.Assembly.Line-A.Cell-5"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# Location Computation
#   MergeLocations
#     should merge parent and child locations [It]
#   
#   undefined: variables.MergeLocations
```

**Verify:** Test fails with `MergeLocations undefined`

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/variables/location.go`

```go
// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0

package variables

import (
    "strings"
)

// ISA-95 hierarchy levels in order (top to bottom)
var isa95Hierarchy = []string{
    "enterprise",
    "site",
    "area",
    "productionLine",
    "workCell",
}

// Legacy UMH naming to ISA-95 mapping
var legacyMapping = map[string]string{
    "line": "productionLine",
    "cell": "workCell",
}

// MergeLocations merges parent and child location maps
//
// Parent location + child location = merged hierarchy
// Child values override parent values for same keys
//
// Example:
//   parent: {"enterprise": "ACME", "site": "Factory-1"}
//   child:  {"line": "Line-A", "cell": "Cell-5"}
//   merged: {"enterprise": "ACME", "site": "Factory-1", "line": "Line-A", "cell": "Cell-5"}
func MergeLocations(parent, child map[string]string) map[string]string {
    merged := make(map[string]string)
    
    // Copy parent
    for k, v := range parent {
        merged[k] = v
    }
    
    // Overlay child (overrides parent)
    for k, v := range child {
        merged[k] = v
    }
    
    return merged
}

// ComputeLocationPath computes ISA-95 path from location map
//
// ISA-95 hierarchy order (top to bottom):
//   enterprise.site.area.productionLine.workCell
//
// Legacy UMH naming supported:
//   "line" → "productionLine"
//   "cell" → "workCell"
//
// Gap filling: Missing levels filled with "unknown"
//
// Examples:
//   {"enterprise": "ACME", "site": "Factory-1"}
//   → "ACME.Factory-1"
//
//   {"enterprise": "ACME", "area": "Assembly"}  // site missing
//   → "ACME.unknown.Assembly"
//
//   {"enterprise": "ACME", "site": "Factory-1", "line": "Line-A"}
//   → "ACME.Factory-1.unknown.Line-A"  // area missing, line→productionLine
func ComputeLocationPath(location map[string]string) string {
    if len(location) == 0 {
        return ""
    }
    
    // Normalize legacy naming
    normalized := make(map[string]string)
    for k, v := range location {
        if mappedKey, ok := legacyMapping[k]; ok {
            normalized[mappedKey] = v
        } else {
            normalized[k] = v
        }
    }
    
    // Build path following ISA-95 hierarchy
    var parts []string
    inGap := false
    
    for _, level := range isa95Hierarchy {
        if value, ok := normalized[level]; ok {
            // Found value, add to path
            parts = append(parts, value)
            inGap = false
        } else if len(parts) > 0 {
            // Missing level, but we've already started the path
            // Check if any deeper levels exist
            hasDeeper := false
            for _, deeperLevel := range isa95Hierarchy {
                if deeperLevel == level {
                    break
                }
                if _, ok := normalized[deeperLevel]; ok {
                    hasDeeper = true
                    break
                }
            }
            
            // Only fill gap if deeper levels exist
            if hasLevelsBeyond(normalized, level) {
                if !inGap {
                    parts = append(parts, "unknown")
                    inGap = true
                }
            }
        }
    }
    
    return strings.Join(parts, ".")
}

// hasLevelsBeyond checks if location has any levels beyond the given level
func hasLevelsBeyond(location map[string]string, currentLevel string) bool {
    foundCurrent := false
    for _, level := range isa95Hierarchy {
        if level == currentLevel {
            foundCurrent = true
            continue
        }
        if foundCurrent {
            if _, ok := location[level]; ok {
                return true
            }
        }
    }
    return false
}
```

**Why this design:**
- ISA-95 standard: enterprise → site → area → productionLine → workCell
- Legacy support: "line"/"cell" map to ISA-95 names
- Gap filling: Missing levels filled with "unknown"
- Merge: Child overrides parent (Kubernetes pattern)

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/variables
ginkgo -v

# Expected output:
# Ran 27 of 27 Specs in 0.006 seconds
# SUCCESS! -- 27 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/variables/
git commit -m "feat(fsmv2): implement location merging and path computation

Adds ISA-95 location hierarchy support:
- MergeLocations: parent + child → merged (child overrides)
- ComputeLocationPath: location map → dot-separated path
- Gap filling: missing levels → "unknown"
- Legacy mapping: line→productionLine, cell→workCell

ISA-95 order: enterprise.site.area.productionLine.workCell

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Location merging correctness:**
   - Parent and child merge correctly
   - Child overrides parent (Kubernetes pattern)
   - Nil handling correct

2. **Path computation correctness:**
   - ISA-95 hierarchy order correct
   - Gap filling with "unknown" works
   - Legacy mapping (line/cell) works
   - Empty/nil location handled

3. **Test coverage:**
   - Merge cases tested
   - Path computation tested
   - Gap filling tested
   - Legacy naming tested
   - Edge cases tested (empty, nil, out-of-order)
   - No focused specs

4. **Documentation:**
   - ISA-95 hierarchy documented
   - Examples clear
   - Gap filling behavior explained
   - Legacy mapping documented

**Ask:**
- Does ISA-95 hierarchy match standard?
- Is gap filling behavior correct?
- Are legacy mappings complete?
- Is child override behavior correct?

---

### Task 0.5.5: Variable Injection in Supervisor

**Goal:** Inject Global and Internal variables at supervisor level

**Why:** Supervisor owns lifecycle metadata, Global variables are fleet-wide

**Location:** `pkg/fsmv2/supervisor/supervisor.go`

**Effort:** 2 hours, ~50 lines

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/supervisor/supervisor_test.go`

```go
var _ = Describe("Supervisor Variable Injection", func() {
    var (
        supervisor *Supervisor
        mockWorker *MockWorker
        globalVars map[string]any
    )
    
    BeforeEach(func() {
        mockWorker = NewMockWorker()
        globalVars = map[string]any{
            "api_endpoint": "https://api.umh.app",
            "tenant_id":    "acme-corp",
        }
        
        supervisor = NewSupervisor(SupervisorConfig{
            ID:           "test-supervisor",
            Worker:       mockWorker,
            GlobalVars:   globalVars,
        })
    })
    
    Describe("Global Variable Injection", func() {
        It("should inject Global variables into UserSpec", func() {
            userSpec := UserSpec{
                Variables: VariableBundle{
                    User: map[string]any{
                        "IP": "192.168.1.100",
                    },
                },
            }
            
            supervisor.UpdateUserSpec(userSpec)
            
            // Trigger tick to call DeriveDesiredState
            supervisor.Tick(context.Background())
            
            // Verify worker received Global variables
            receivedSpec := mockWorker.LastUserSpec()
            Expect(receivedSpec.Variables.Global).To(HaveKeyWithValue("api_endpoint", "https://api.umh.app"))
            Expect(receivedSpec.Variables.Global).To(HaveKeyWithValue("tenant_id", "acme-corp"))
        })
        
        It("should preserve User variables", func() {
            userSpec := UserSpec{
                Variables: VariableBundle{
                    User: map[string]any{
                        "IP":   "192.168.1.100",
                        "PORT": 502,
                    },
                },
            }
            
            supervisor.UpdateUserSpec(userSpec)
            supervisor.Tick(context.Background())
            
            receivedSpec := mockWorker.LastUserSpec()
            Expect(receivedSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
            Expect(receivedSpec.Variables.User).To(HaveKeyWithValue("PORT", 502))
        })
    })
    
    Describe("Internal Variable Injection", func() {
        It("should inject supervisor ID", func() {
            supervisor.Tick(context.Background())
            
            receivedSpec := mockWorker.LastUserSpec()
            Expect(receivedSpec.Variables.Internal).To(HaveKeyWithValue("id", "test-supervisor"))
        })
        
        It("should inject creation timestamp", func() {
            supervisor.Tick(context.Background())
            
            receivedSpec := mockWorker.LastUserSpec()
            Expect(receivedSpec.Variables.Internal).To(HaveKey("created_at"))
            
            createdAt := receivedSpec.Variables.Internal["created_at"].(string)
            // Should be ISO-8601 format
            Expect(createdAt).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`))
        })
        
        It("should inject bridged_by if parent exists", func() {
            parentSupervisor := NewSupervisor(SupervisorConfig{
                ID: "parent-supervisor",
            })
            
            childSupervisor := NewSupervisor(SupervisorConfig{
                ID:     "child-supervisor",
                Parent: parentSupervisor,
                Worker: mockWorker,
            })
            
            childSupervisor.Tick(context.Background())
            
            receivedSpec := mockWorker.LastUserSpec()
            Expect(receivedSpec.Variables.Internal).To(HaveKeyWithValue("bridged_by", "parent-supervisor"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/supervisor
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# Supervisor Variable Injection
#   Global Variable Injection
#     should inject Global variables into UserSpec [It]
#   
#   Expected
#       <map[string]interface {} | len:0>: {}
#   to contain element matching
#       <string>: api_endpoint
```

**Verify:** Test fails because Global variables not injected

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/supervisor/supervisor.go`

```go
// Tick performs one iteration of the supervisor control loop
func (s *Supervisor) Tick(ctx context.Context) error {
    // Inject Global and Internal variables
    userSpec := s.injectVariables(s.userSpec)
    
    // Call worker's DeriveDesiredState with injected variables
    desiredState := s.worker.DeriveDesiredState(userSpec)
    
    // ... rest of tick logic
    
    return nil
}

// injectVariables injects Global and Internal variables into UserSpec
func (s *Supervisor) injectVariables(userSpec UserSpec) UserSpec {
    // Deep copy to avoid mutation
    injected := userSpec.DeepCopy()
    
    // Inject Global variables (fleet-wide settings)
    if s.globalVars != nil {
        injected.Variables.Global = s.globalVars
    }
    
    // Inject Internal variables (runtime metadata)
    injected.Variables.Internal = s.buildInternalVars()
    
    return injected
}

// buildInternalVars creates Internal variable map
func (s *Supervisor) buildInternalVars() map[string]any {
    internal := map[string]any{
        "id":         s.id,
        "created_at": s.createdAt.Format(time.RFC3339),
    }
    
    // Add bridged_by if parent exists
    if s.parent != nil {
        internal["bridged_by"] = s.parent.ID()
    }
    
    return internal
}
```

**Additional changes:**

**File:** `pkg/fsmv2/supervisor/supervisor.go` (struct)

```go
type Supervisor struct {
    id         string
    worker     Worker
    globalVars map[string]any
    parent     *Supervisor
    createdAt  time.Time
    // ... other fields
}

type SupervisorConfig struct {
    ID         string
    Worker     Worker
    GlobalVars map[string]any
    Parent     *Supervisor
}

func NewSupervisor(config SupervisorConfig) *Supervisor {
    return &Supervisor{
        id:         config.ID,
        worker:     config.Worker,
        globalVars: config.GlobalVars,
        parent:     config.Parent,
        createdAt:  time.Now().UTC(),
        // ... other fields
    }
}
```

**Why this design:**
- Global injection: Supervisor owns fleet-wide variables
- Internal injection: Supervisor owns runtime metadata
- Deep copy: Avoid mutation of original UserSpec
- Parent tracking: For bridged_by metadata

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/supervisor
ginkgo -v

# Expected output:
# Ran 30 of 30 Specs in 0.008 seconds
# SUCCESS! -- 30 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/supervisor/
git commit -m "feat(fsmv2): inject Global and Internal variables in Supervisor

Supervisor now injects variables before calling DeriveDesiredState:
- Global: fleet-wide settings (from config)
- Internal: runtime metadata (id, created_at, bridged_by)

User variables preserved, Global/Internal added at supervisor level.

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Injection correctness:**
   - Global variables injected from supervisor config
   - Internal variables built at runtime
   - User variables preserved (not overwritten)
   - Deep copy prevents mutation

2. **Internal variables:**
   - id: supervisor ID
   - created_at: ISO-8601 timestamp
   - bridged_by: parent supervisor ID (if parent exists)

3. **Test coverage:**
   - Global injection tested
   - Internal injection tested
   - User preservation tested
   - Parent tracking tested
   - No focused specs

4. **Documentation:**
   - Clear comments on injection point
   - Internal variables documented

**Ask:**
- Is injection happening at correct point in tick loop?
- Are all Internal variables documented?
- Is deep copy necessary and correct?
- Is parent tracking correct?

---

### Task 0.5.6: Integration with DeriveDesiredState

**Goal:** Update DeriveDesiredState signature to use Variables in UserSpec

**Why:** Complete integration, variables flow parent → child

**Location:** `pkg/fsmv2/worker/worker.go`, child worker implementations

**Effort:** 3 hours, ~100 lines (across multiple files)

---

#### Step 1: Write Failing Test

**File:** `pkg/fsmv2/integration/variables_integration_test.go`

```go
package integration_test

import (
    "context"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/variables"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/worker"
)

var _ = Describe("Variables Integration", func() {
    Describe("Parent to Child Variable Flow", func() {
        It("should pass variables from parent to child", func() {
            // Create parent worker
            parentWorker := &BridgeWorker{}
            
            // Create parent supervisor with Global variables
            globalVars := map[string]any{
                "api_endpoint": "https://api.umh.app",
            }
            
            parentSupervisor := supervisor.NewSupervisor(supervisor.SupervisorConfig{
                ID:         "bridge-123",
                Worker:     parentWorker,
                GlobalVars: globalVars,
            })
            
            // Update parent UserSpec with User variables
            parentUserSpec := worker.UserSpec{
                Variables: variables.VariableBundle{
                    User: map[string]any{
                        "IP":   "192.168.1.100",
                        "PORT": 502,
                    },
                },
            }
            parentSupervisor.UpdateUserSpec(parentUserSpec)
            
            // Tick parent (creates child specs)
            parentSupervisor.Tick(context.Background())
            
            // Get child specs from parent's desired state
            childSpecs := parentSupervisor.DesiredState().ChildrenSpecs
            Expect(childSpecs).To(HaveLen(1))
            
            childSpec := childSpecs[0]
            
            // Verify child UserSpec has parent variables
            Expect(childSpec.UserSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
            Expect(childSpec.UserSpec.Variables.User).To(HaveKeyWithValue("PORT", 502))
            Expect(childSpec.UserSpec.Variables.User).To(HaveKeyWithValue("parent_state", "active"))
            
            // Verify child gets Global variables from parent
            Expect(childSpec.UserSpec.Variables.Global).To(HaveKeyWithValue("api_endpoint", "https://api.umh.app"))
        })
        
        It("should compute location_path from merged locations", func() {
            parentWorker := &BridgeWorker{}
            
            parentSupervisor := supervisor.NewSupervisor(supervisor.SupervisorConfig{
                ID:     "bridge-123",
                Worker: parentWorker,
            })
            
            // Parent UserSpec with agent location
            parentUserSpec := worker.UserSpec{
                Variables: variables.VariableBundle{
                    User: map[string]any{
                        "location": map[string]string{
                            "enterprise": "ACME",
                            "site":       "Factory-1",
                        },
                    },
                },
            }
            parentSupervisor.UpdateUserSpec(parentUserSpec)
            
            parentSupervisor.Tick(context.Background())
            
            childSpec := parentSupervisor.DesiredState().ChildrenSpecs[0]
            
            // Child should have merged location
            childLocation := childSpec.UserSpec.Variables.User["location"].(map[string]string)
            Expect(childLocation).To(HaveKeyWithValue("enterprise", "ACME"))
            Expect(childLocation).To(HaveKeyWithValue("site", "Factory-1"))
            Expect(childLocation).To(HaveKeyWithValue("line", "Line-A"))
            
            // Child should have location_path computed
            locationPath := childSpec.UserSpec.Variables.User["location_path"].(string)
            Expect(locationPath).To(Equal("ACME.Factory-1.unknown.Line-A"))
        })
    })
    
    Describe("Template Rendering in Worker", func() {
        It("should render templates in child worker's DeriveDesiredState", func() {
            // Create Benthos worker with template
            benthosWorker := &BenthosWorker{
                ConfigTemplate: `input:
  modbus_tcp:
    address: "{{ .IP }}:{{ .PORT }}"
output:
  kafka:
    topic: "umh.v1.{{ .location_path }}"`,
            }
            
            benthosSupervisor := supervisor.NewSupervisor(supervisor.SupervisorConfig{
                ID:     "benthos-dfc-read",
                Worker: benthosWorker,
            })
            
            // UserSpec with flattened variables
            userSpec := worker.UserSpec{
                Variables: variables.VariableBundle{
                    User: map[string]any{
                        "IP":            "192.168.1.100",
                        "PORT":          502,
                        "location_path": "ACME.Factory-1.Line-A",
                    },
                },
            }
            benthosSupervisor.UpdateUserSpec(userSpec)
            
            benthosSupervisor.Tick(context.Background())
            
            // Verify rendered config
            desiredState := benthosSupervisor.DesiredState()
            config := desiredState.Config.(string)
            
            Expect(config).To(ContainSubstring("address: \"192.168.1.100:502\""))
            Expect(config).To(ContainSubstring("topic: \"umh.v1.ACME.Factory-1.Line-A\""))
            Expect(config).NotTo(ContainSubstring("{{"))
            Expect(config).NotTo(ContainSubstring("}}"))
        })
    })
})
```

---

#### Step 2: Run Test, Verify Failure

```bash
cd pkg/fsmv2/integration
ginkgo -v

# Expected output:
# • Failure [0.001 seconds]
# Variables Integration
#   Parent to Child Variable Flow
#     should pass variables from parent to child [It]
#   
#   Expected
#       <[]ChildSpec | len:0>: []
#   to have length 1
```

**Verify:** Test fails because BridgeWorker doesn't create child specs with variables

---

#### Step 3: Minimal Implementation

**File:** `pkg/fsmv2/worker/bridge_worker.go`

```go
type BridgeWorker struct {
    // ... existing fields
}

type BridgeUserSpec struct {
    IPAddress string
    Port      uint16
    Variables variables.VariableBundle
}

func (w *BridgeWorker) DeriveDesiredState(userSpec BridgeUserSpec) worker.DesiredState {
    // Compute parent state
    parentState := "active"
    
    // Merge locations (parent + child)
    parentLocation, _ := userSpec.Variables.User["location"].(map[string]string)
    childLocation := map[string]string{
        "line": "Line-A",
    }
    mergedLocation := variables.MergeLocations(parentLocation, childLocation)
    locationPath := variables.ComputeLocationPath(mergedLocation)
    
    // Create child spec with variables
    return worker.DesiredState{
        State: parentState,
        ChildrenSpecs: []worker.ChildSpec{
            {
                Name:       "dfc_read",
                WorkerType: worker.WorkerTypeBenthos,
                UserSpec: worker.UserSpec{
                    Variables: variables.VariableBundle{
                        User: map[string]any{
                            // Connection config
                            "IP":   userSpec.IPAddress,
                            "PORT": userSpec.Port,
                            
                            // Parent context
                            "parent_state": parentState,
                            
                            // Location hierarchy
                            "location":      mergedLocation,
                            "location_path": locationPath,
                        },
                        // Global passed through by supervisor
                    },
                },
            },
        },
    }
}
```

**File:** `pkg/fsmv2/worker/benthos_worker.go`

```go
type BenthosWorker struct {
    ConfigTemplate string
}

type BenthosUserSpec struct {
    Variables variables.VariableBundle
}

func (w *BenthosWorker) DeriveDesiredState(userSpec BenthosUserSpec) worker.DesiredState {
    // Flatten variables for template rendering
    scope := userSpec.Variables.Flatten()
    
    // Render config template
    config, err := variables.RenderTemplate(w.ConfigTemplate, scope)
    if err != nil {
        return worker.DesiredState{
            State: "error",
            Error: err,
        }
    }
    
    // Compute state from parent_state
    parentState, _ := scope["parent_state"].(string)
    state := "stopped"
    if parentState == "active" {
        state = "running"
    }
    
    return worker.DesiredState{
        State:  state,
        Config: config,
    }
}
```

**Why this design:**
- Parent merges locations and passes to child
- Parent passes parent_state variable
- Child renders templates using Flatten()
- Child computes state from parent_state

---

#### Step 4: Run Test, Verify Pass

```bash
cd pkg/fsmv2/integration
ginkgo -v

# Expected output:
# Ran 35 of 35 Specs in 0.012 seconds
# SUCCESS! -- 35 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Verify:** All tests pass

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/worker/ pkg/fsmv2/integration/
git commit -m "feat(fsmv2): integrate variables with DeriveDesiredState

Updates worker implementations to use Variables:
- BridgeWorker: merges locations, passes parent_state to child
- BenthosWorker: renders templates using Flatten()
- Child computes state from parent_state variable

Completes Phase 0.5 variable flow: parent → child → template rendering.

Part of Phase 0.5 (Templating & Variables).
Refs: fsmv2-idiomatic-templating-and-variables.md"
```

---

#### Review Checkpoint

**Code Reviewer Should Verify:**

1. **Parent-child variable flow:**
   - Parent merges locations correctly
   - Parent passes parent_state
   - Parent computes location_path
   - Child receives all variables

2. **Template rendering:**
   - Child calls Flatten() before rendering
   - Templates render correctly
   - No unexpanded markers in output
   - Error handling correct

3. **State computation:**
   - Child computes state from parent_state
   - State transitions correct

4. **Test coverage:**
   - Parent-child flow tested
   - Location merging tested
   - Template rendering tested
   - End-to-end integration tested
   - No focused specs

5. **Documentation:**
   - Variable flow documented
   - Template rendering documented
   - State computation documented

**Ask:**
- Does parent-child flow match design document?
- Is location merging correct?
- Are templates rendering correctly?
- Is state computation correct?
- Is error handling comprehensive?

---

## Phase 0.5 Summary

**Total Deliverables:**

1. **VariableBundle structure** (Task 0.5.1)
   - Three-tier namespace (User/Global/Internal)
   - Serialization behavior (User/Global yes, Internal no)

2. **Flatten() implementation** (Task 0.5.2)
   - User variables promoted to top-level
   - Global/Internal nested

3. **RenderTemplate() with strict mode** (Task 0.5.3)
   - Strict mode (missingkey=error)
   - Validation (no unexpanded markers)
   - Generic (strings and structs)

4. **Location computation** (Task 0.5.4)
   - MergeLocations (parent + child)
   - ComputeLocationPath (ISA-95 hierarchy)
   - Gap filling with "unknown"
   - Legacy mapping (line/cell)

5. **Variable injection in Supervisor** (Task 0.5.5)
   - Global variables from config
   - Internal variables at runtime
   - Parent tracking (bridged_by)

6. **Integration with DeriveDesiredState** (Task 0.5.6)
   - Parent passes variables to child
   - Child renders templates
   - Child computes state from parent_state

**Lines of Code:** ~325 lines total
- VariableBundle: 30 lines
- Flatten(): 15 lines
- RenderTemplate(): 40 lines
- Location: 90 lines
- Supervisor injection: 50 lines
- Worker integration: 100 lines

**Test Coverage:** ~150 lines of tests

**Dependencies Met:** Phase 0 (ChildSpec, UserSpec structures)

**Blocks Removed:** Benthos worker can now render configs from templates

**Integration Points:**
- Phase 0: ChildSpec.UserSpec contains Variables
- Phase 1: Infrastructure supervision works with variables
- Phase 2: Async actions work per child
- Phase 3: Integration tests validate full flow

---
