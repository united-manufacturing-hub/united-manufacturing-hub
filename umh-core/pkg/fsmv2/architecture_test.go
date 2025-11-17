// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fsmv2_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"

	// Import state packages to scan for violations
	childState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/state"
	parentState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/state"
)

// Violation represents an architectural violation found in code
type Violation struct {
	File    string
	Line    int
	Type    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", v.File, v.Line, v.Type, v.Message)
}

var _ = Describe("FSMv2 Architecture Validation", func() {

	Context("🔴 PHASE 1: Architectural Validation Tests (Steps 52-55)", func() {

		Describe("Empty State Structs (Invariant: States are Pure Behavior)", func() {
			It("should have no fields except embedded base types", func() {
				violations := validateStateStructs()

				if len(violations) > 0 {
					message := formatViolations("Empty State Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Stateless Actions (Invariant: Actions are Idempotent Commands)", func() {
			It("should have no mutable state fields", func() {
				violations := validateActionStructs()

				if len(violations) > 0 {
					message := formatViolations("Stateless Action Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Type Assertions in Next() Methods (Invariant: Single Entry-Point Pattern)", func() {
			It("should have exactly one type assertion at method entry", func() {
				violations := validateNextMethodTypeAssertions()

				if len(violations) > 0 {
					message := formatViolations("Type Assertion Pattern Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Pure DeriveDesiredState() Methods (Invariant: No External Dependencies)", func() {
			It("should not access dependencies directly", func() {
				violations := validateDeriveDesiredState()

				if len(violations) > 0 {
					message := formatViolations("DeriveDesiredState Violations", violations)
					Fail(message)
				}
			})
		})
	})
})

// validateStateStructs checks that state structs have no fields (except base state)
func validateStateStructs() []Violation {
	var violations []Violation

	// Get package path for file locations
	_, filename, _, _ := runtime.Caller(0)
	fsmv2Dir := filepath.Dir(filename)

	// Check example-child states
	childStates := []interface{}{
		&childState.TryingToConnectState{},
		&childState.ConnectedState{},
		&childState.DisconnectedState{},
		&childState.TryingToStopState{},
		&childState.StoppedState{},
	}

	for _, state := range childStates {
		v := reflect.ValueOf(state).Elem()
		t := v.Type()

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Skip embedded base types (they start with capital letter and are anonymous)
			if field.Anonymous {
				continue
			}

			// Found a violation: state has a field
			violations = append(violations, Violation{
				File:    getStateFilePath(fsmv2Dir, t.Name()),
				Line:    0, // Would need source parsing to get exact line
				Type:    "EMPTY_STATE",
				Message: fmt.Sprintf("State %s has field '%s %s' (should be empty)", t.Name(), field.Name, field.Type),
			})
		}
	}

	// Check example-parent states
	parentStates := []interface{}{
		&parentState.TryingToStartState{},
		&parentState.RunningState{},
		&parentState.TryingToStopState{},
		&parentState.StoppedState{},
	}

	for _, state := range parentStates {
		v := reflect.ValueOf(state).Elem()
		t := v.Type()

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			if field.Anonymous {
				continue
			}

			violations = append(violations, Violation{
				File:    getStateFilePath(fsmv2Dir, t.Name()),
				Line:    0,
				Type:    "EMPTY_STATE",
				Message: fmt.Sprintf("State %s has field '%s %s' (should be empty)", t.Name(), field.Name, field.Type),
			})
		}
	}

	return violations
}

// validateActionStructs checks that action structs have no mutable state
func validateActionStructs() []Violation {
	var violations []Violation

	// This would use reflection to check action types
	// For now, we'll use AST parsing of action files

	_, filename, _, _ := runtime.Caller(0)
	fsmv2Dir := filepath.Dir(filename)

	actionFiles := []string{
		filepath.Join(fsmv2Dir, "workers/example/example-child/action/connect.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-child/action/disconnect.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/action/start.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/action/stop.go"),
	}

	for _, file := range actionFiles {
		fileViolations := checkActionFile(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// validateNextMethodTypeAssertions checks that Next() methods use the single entry-point type assertion pattern
// Pattern: ONE type assertion at the start of Next(), zero elsewhere
// Rationale: Go doesn't support covariance, so State[any, any] is required for Worker interface.
// States must accept `any` and type-assert to concrete type once at entry point.
func validateNextMethodTypeAssertions() []Violation {
	var violations []Violation

	_, filename, _, _ := runtime.Caller(0)
	fsmv2Dir := filepath.Dir(filename)

	stateFiles := []string{
		// Child states
		filepath.Join(fsmv2Dir, "workers/example/example-child/state/state_trying_to_connect.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-child/state/state_connected.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-child/state/state_disconnected.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-child/state/state_stopped.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-child/state/state_trying_to_stop.go"),
		// Parent states
		filepath.Join(fsmv2Dir, "workers/example/example-parent/state/state_degraded.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/state/state_stopped.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/state/state_trying_to_start.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/state/state_running.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/state/state_trying_to_stop.go"),
	}

	for _, file := range stateFiles {
		fileViolations := checkSingleEntryPointPattern(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// validateDeriveDesiredState checks that DeriveDesiredState doesn't access dependencies
func validateDeriveDesiredState() []Violation {
	var violations []Violation

	_, filename, _, _ := runtime.Caller(0)
	fsmv2Dir := filepath.Dir(filename)

	workerFiles := []string{
		filepath.Join(fsmv2Dir, "workers/example/example-child/worker.go"),
		filepath.Join(fsmv2Dir, "workers/example/example-parent/worker.go"),
	}

	for _, file := range workerFiles {
		fileViolations := checkDeriveDesiredStateMethod(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkActionFile parses an action file and looks for mutable state fields
func checkActionFile(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for struct declarations with "Action" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "Action") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Check each field
		for _, field := range structType.Fields.List {
			// Skip embedded types
			if len(field.Names) == 0 {
				continue
			}

			fieldName := field.Names[0].Name

			// Check for violation patterns
			if strings.Contains(strings.ToLower(fieldName), "dependencies") ||
				strings.Contains(strings.ToLower(fieldName), "failure") ||
				strings.Contains(strings.ToLower(fieldName), "count") ||
				strings.Contains(strings.ToLower(fieldName), "max") {

				pos := fset.Position(field.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "STATELESS_ACTION",
					Message: fmt.Sprintf("Action %s has mutable field '%s' (actions should be stateless)", typeSpec.Name.Name, fieldName),
				})
			}
		}

		return true
	})

	return violations
}

// checkNextMethodTypeAssertions parses a state file and looks for type assertions in Next()
func checkSingleEntryPointPattern(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for Next() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		// Count type assertions and check their location
		var typeAssertions []int
		var firstStatementLine int

		// Get line number of first statement
		if funcDecl.Body != nil && len(funcDecl.Body.List) > 0 {
			firstStatementLine = fset.Position(funcDecl.Body.List[0].Pos()).Line
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			typeAssert, ok := bodyNode.(*ast.TypeAssertExpr)
			if !ok {
				return true
			}

			pos := fset.Position(typeAssert.Pos())
			typeAssertions = append(typeAssertions, pos.Line)
			return true
		})

		// Check pattern: exactly 1 assertion, at first statement
		if len(typeAssertions) == 0 {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(funcDecl.Pos()).Line,
				Type:    "MISSING_ENTRY_ASSERTION",
				Message: "Next() method missing entry-point type assertion (should assert parameter to concrete snapshot type)",
			})
		} else if len(typeAssertions) > 1 {
			violations = append(violations, Violation{
				File:    filename,
				Line:    typeAssertions[1],
				Type:    "MULTIPLE_ASSERTIONS",
				Message: fmt.Sprintf("Next() method has %d type assertions (should have exactly 1 at entry)", len(typeAssertions)),
			})
		} else if typeAssertions[0] != firstStatementLine {
			violations = append(violations, Violation{
				File:    filename,
				Line:    typeAssertions[0],
				Type:    "ASSERTION_NOT_AT_ENTRY",
				Message: "Type assertion should be first statement in Next() method",
			})
		}

		return true
	})

	return violations
}

// checkDeriveDesiredStateMethod parses a worker file and checks DeriveDesiredState
func checkDeriveDesiredStateMethod(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for DeriveDesiredState() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "DeriveDesiredState" {
			return true
		}

		// Look for GetDependencies() calls in the method body
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			callExpr, ok := bodyNode.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check for method calls on receiver
			if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				if selExpr.Sel.Name == "GetDependencies" {
					pos := fset.Position(callExpr.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "PURE_DERIVE",
						Message: "DeriveDesiredState() accesses dependencies (should use typed UserSpec parameter)",
					})
				}
			}

			return true
		})

		return true
	})

	return violations
}

// Helper functions

func getStateFilePath(fsmv2Dir, typeName string) string {
	// Convert type name to file name
	// e.g., "TryingToConnectState" -> "state_trying_to_connect.go"
	fileName := strings.ToLower(strings.ReplaceAll(typeName, "State", ""))
	fileName = "state_" + strings.ToLower(strings.ReplaceAll(fileName, "TryingTo", "trying_to_"))
	fileName = strings.ReplaceAll(fileName, "__", "_")
	return filepath.Join(fsmv2Dir, "workers", "example", fileName+".go")
}

func formatViolations(title string, violations []Violation) string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║  " + title + strings.Repeat(" ", 62-len(title)) + "║\n")
	sb.WriteString("╠════════════════════════════════════════════════════════════════╣\n")
	sb.WriteString("║                                                                ║\n")
	sb.WriteString("║  These tests document CURRENT architectural violations.       ║\n")
	sb.WriteString("║  Fix these violations to make the tests pass.                 ║\n")
	sb.WriteString("║                                                                ║\n")
	sb.WriteString("╚════════════════════════════════════════════════════════════════╝\n\n")

	for i, v := range violations {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, v))
	}

	sb.WriteString(fmt.Sprintf("\n📊 Total violations found: %d\n\n", len(violations)))

	return sb.String()
}
