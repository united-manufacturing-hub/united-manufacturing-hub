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

package validator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ValidateDeriveDesiredState checks that DeriveDesiredState doesn't access dependencies.
func ValidateDeriveDesiredState(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkDeriveDesiredStateMethod(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDeriveDesiredStateMethod parses a worker file and checks DeriveDesiredState.
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

// ValidateContextCancellationInCollect checks CollectObservedState handles ctx.Done().
func ValidateContextCancellationInCollect(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkContextCancellationInCollect(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkContextCancellationInCollect parses a worker file and checks CollectObservedState.
func checkContextCancellationInCollect(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for CollectObservedState() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "CollectObservedState" {
			return true
		}

		// Look for select statement with ctx.Done() case
		hasContextCancellation := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			selectStmt, ok := bodyNode.(*ast.SelectStmt)
			if !ok {
				return true
			}

			// Check each case in select
			for _, stmt := range selectStmt.Body.List {
				commClause, ok := stmt.(*ast.CommClause)
				if !ok || commClause.Comm == nil {
					continue
				}

				// Look for <-ctx.Done()
				if exprStmt, ok := commClause.Comm.(*ast.ExprStmt); ok {
					if unaryExpr, ok := exprStmt.X.(*ast.UnaryExpr); ok {
						if unaryExpr.Op == token.ARROW {
							if callExpr, ok := unaryExpr.X.(*ast.CallExpr); ok {
								if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
									if selExpr.Sel.Name == "Done" {
										if ident, ok := selExpr.X.(*ast.Ident); ok {
											if ident.Name == "ctx" {
												hasContextCancellation = true

												return false
											}
										}
									}
								}
							}
						}
					}
				}
			}

			return true
		})

		if !hasContextCancellation {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_CONTEXT_CANCELLATION_COLLECT",
				Message: "CollectObservedState() missing context cancellation handling (select with case <-ctx.Done())",
			})
		}

		return true
	})

	return violations
}

// ValidateNilSpecHandling checks that DeriveDesiredState checks for nil spec.
func ValidateNilSpecHandling(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkNilSpecHandling(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkNilSpecHandling parses a worker file and checks if DeriveDesiredState checks for nil spec.
// This validation passes if:
// 1. The function has an explicit `if spec == nil` check in the first two statements, OR
// 2. The function uses helper functions (DeriveLeafState, ParseUserSpec) that handle nil internally.
func checkNilSpecHandling(filename string) []Violation {
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

		// Check if body exists and has at least one statement
		if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
			return true
		}

		// Check for nil-safe helper functions in any statement
		// DeriveLeafState and ParseUserSpec handle nil internally
		if usesNilSafeHelper(funcDecl.Body) {
			return true
		}

		// Check first or second statement for nil check
		hasNilCheck := false

		for i := 0; i < 2 && i < len(funcDecl.Body.List); i++ {
			stmt := funcDecl.Body.List[i]

			// Look for if statement
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok {
				continue
			}

			// Check if condition is "spec == nil" or "nil == spec"
			if binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
				if binExpr.Op == token.EQL {
					// Check left side
					if ident, ok := binExpr.X.(*ast.Ident); ok && ident.Name == "spec" {
						if nilIdent, ok := binExpr.Y.(*ast.Ident); ok && nilIdent.Name == "nil" {
							hasNilCheck = true

							break
						}
					}

					// Check right side (nil == spec)
					if ident, ok := binExpr.Y.(*ast.Ident); ok && ident.Name == "spec" {
						if nilIdent, ok := binExpr.X.(*ast.Ident); ok && nilIdent.Name == "nil" {
							hasNilCheck = true

							break
						}
					}
				}
			}
		}

		if !hasNilCheck {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_NIL_SPEC_CHECK",
				Message: "DeriveDesiredState() must check if spec == nil before type assertion",
			})
		}

		return true
	})

	return violations
}

// usesNilSafeHelper checks if the function body uses helper functions that handle nil internally.
// These helpers (DeriveLeafState, ParseUserSpec) check for nil spec before type assertion.
func usesNilSafeHelper(body *ast.BlockStmt) bool {
	// Helper function names that handle nil internally
	nilSafeHelpers := map[string]bool{
		"DeriveLeafState": true,
		"ParseUserSpec":   true,
	}

	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for direct function call: DeriveLeafState[T](spec)
		if indexExpr, ok := callExpr.Fun.(*ast.IndexExpr); ok {
			if sel, ok := indexExpr.X.(*ast.SelectorExpr); ok {
				if nilSafeHelpers[sel.Sel.Name] {
					found = true

					return false
				}
			}

			if ident, ok := indexExpr.X.(*ast.Ident); ok {
				if nilSafeHelpers[ident.Name] {
					found = true

					return false
				}
			}
		}

		// Check for selector call: config.DeriveLeafState[T](spec)
		if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			if nilSafeHelpers[sel.Sel.Name] {
				found = true

				return false
			}
		}

		return true
	})

	return found
}

// ValidatePointerReceivers checks that Worker methods use pointer receivers.
func ValidatePointerReceivers(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkPointerReceivers(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkPointerReceivers parses a worker file and checks for pointer receivers.
func checkPointerReceivers(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for Worker interface methods
	targetMethods := map[string]bool{
		"CollectObservedState": true,
		"DeriveDesiredState":   true,
		"GetInitialState":      true,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || !targetMethods[funcDecl.Name.Name] {
			return true
		}

		// Check if receiver exists and is a pointer
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		// Check if receiver is a pointer (*T)
		isPointer := false
		if _, ok := funcDecl.Recv.List[0].Type.(*ast.StarExpr); ok {
			isPointer = true
		}

		if !isPointer {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "VALUE_RECEIVER_ON_WORKER",
				Message: fmt.Sprintf("Method %s() uses value receiver (should use pointer receiver *T)", funcDecl.Name.Name),
			})
		}

		return true
	})

	return violations
}

// ValidateDependencyValidation checks that constructors validate dependencies.
func ValidateDependencyValidation(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkDependencyValidation(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDependencyValidation parses a worker file and checks constructor validation.
func checkDependencyValidation(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || !strings.HasPrefix(funcDecl.Name.Name, "New") || !strings.HasSuffix(funcDecl.Name.Name, "Worker") {
			return true
		}

		if funcDecl.Body == nil || len(funcDecl.Body.List) < 2 {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_DEPENDENCY_VALIDATION",
				Message: fmt.Sprintf("Constructor %s() should validate dependencies are non-nil", funcDecl.Name.Name),
			})
		}

		return true
	})

	return violations
}

// ValidateChildSpecValidation checks that DeriveDesiredState validates children.
func ValidateChildSpecValidation(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkChildSpecValidation(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkChildSpecValidation parses a worker file and checks DeriveDesiredState validation.
func checkChildSpecValidation(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	baseName := filepath.Base(filepath.Dir(filename))
	if !strings.Contains(baseName, "parent") {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "DeriveDesiredState" {
			return true
		}

		hasChildrenValidation := false
		hasProgrammaticChildren := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			// Check for user-provided children that need validation (range over .Children selector)
			if rangeStmt, ok := bodyNode.(*ast.RangeStmt); ok {
				if selExpr, ok := rangeStmt.X.(*ast.SelectorExpr); ok {
					if selExpr.Sel.Name == "Children" {
						hasChildrenValidation = true

						return false
					}
				}
			}

			// Check for programmatically generated children (make([]...ChildSpec) call)
			if callExpr, ok := bodyNode.(*ast.CallExpr); ok {
				if ident, ok := callExpr.Fun.(*ast.Ident); ok && ident.Name == "make" {
					if len(callExpr.Args) > 0 {
						// Check if making a slice of ChildSpec type
						if arrayType, ok := callExpr.Args[0].(*ast.ArrayType); ok {
							if sel, ok := arrayType.Elt.(*ast.SelectorExpr); ok {
								if sel.Sel.Name == "ChildSpec" {
									hasProgrammaticChildren = true

									return false
								}
							}
						}
					}
				}
			}

			return true
		})

		// Only require validation if using user-provided children (not programmatic generation)
		if !hasChildrenValidation && !hasProgrammaticChildren {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_CHILDSPEC_VALIDATION",
				Message: "DeriveDesiredState() should validate ChildrenSpecs before returning",
			})
		}

		return true
	})

	return violations
}

// ValidateDeriveDesiredStateReturns checks that DeriveDesiredState returns valid State values.
// The State field in DesiredState must be "stopped" or "running" (config.DesiredState* constants).
// This is validated at test-time via AST parsing to catch hardcoded invalid values.
func ValidateDeriveDesiredStateReturns(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkDeriveDesiredStateReturns(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDeriveDesiredStateReturns parses a worker file and checks that DeriveDesiredState
// returns valid State values ("stopped" or "running").
func checkDeriveDesiredStateReturns(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Valid desired state values
	validStates := map[string]bool{
		"stopped": true,
		"running": true,
	}

	// Look for DeriveDesiredState() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "DeriveDesiredState" {
			return true
		}

		// Find all return statements in the method
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) == 0 {
				return true
			}

			// Check first return value (DesiredState)
			firstResult := retStmt.Results[0]

			// Handle &Type{} or Type{} composite literals
			var compLit *ast.CompositeLit
			if unaryExpr, ok := firstResult.(*ast.UnaryExpr); ok {
				compLit, _ = unaryExpr.X.(*ast.CompositeLit)
			} else {
				compLit, _ = firstResult.(*ast.CompositeLit)
			}

			if compLit == nil {
				return true
			}

			// Find State field in composite literal
			for _, elt := range compLit.Elts {
				kvExpr, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}

				keyIdent, ok := kvExpr.Key.(*ast.Ident)
				if !ok || keyIdent.Name != "State" {
					continue
				}

				// Check if value is a string literal
				if basicLit, ok := kvExpr.Value.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					value := strings.Trim(basicLit.Value, `"`)
					if !validStates[value] {
						pos := fset.Position(kvExpr.Pos())
						violations = append(violations, Violation{
							File:    filename,
							Line:    pos.Line,
							Type:    "INVALID_DESIRED_STATE_VALUE",
							Message: fmt.Sprintf("DeriveDesiredState returns invalid State value %q - must be \"stopped\" or \"running\"", value),
						})
					}
				}
				// Note: We don't flag config.DesiredState* constants as they're valid
			}

			return true
		})

		return true
	})

	return violations
}

// ValidateFrameworkMetricsCopy checks that workers with dependencies copy framework metrics.
// Workers that call GetDependencies() in CollectObservedState and have MetricsEmbedder
// must also call GetFrameworkState() to copy framework metrics.
func ValidateFrameworkMetricsCopy(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkFrameworkMetricsCopy(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkFrameworkMetricsCopy verifies that CollectObservedState copies framework metrics.
// The pattern is: if a worker has dependencies (calls GetDependencies), it should
// call GetFrameworkState() to copy framework metrics to the observed state.
//
// Workers without dependencies (like ApplicationWorker) are exempt - they intentionally
// don't have access to framework metrics.
func checkFrameworkMetricsCopy(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for CollectObservedState() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "CollectObservedState" {
			return true
		}

		hasGetDependencies := false
		hasGetFrameworkState := false

		// Check for GetDependencies() and GetFrameworkState() calls
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			callExpr, ok := bodyNode.(*ast.CallExpr)
			if !ok {
				return true
			}

			if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				switch selExpr.Sel.Name {
				case "GetDependencies":
					hasGetDependencies = true
				case "GetFrameworkState":
					hasGetFrameworkState = true
				}
			}

			return true
		})

		// If worker has dependencies but doesn't copy framework metrics, flag it
		if hasGetDependencies && !hasGetFrameworkState {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_FRAMEWORK_METRICS_COPY",
				Message: "CollectObservedState() calls GetDependencies() but not GetFrameworkState() - must copy framework metrics from deps",
			})
		}

		return true
	})

	return violations
}

// ValidateMetricsEmbedderValueReceivers checks that MetricsEmbedder uses value receivers.
// This ensures type assertions work when ObservedState is passed as interface{} value.
// Pointer receivers cause type assertion failures because when a struct is passed by value,
// pointer receiver methods are not in the method set.
func ValidateMetricsEmbedderValueReceivers(baseDir string) []Violation {
	var violations []Violation

	// Check api.go for MetricsEmbedder methods
	apiFile := filepath.Join(baseDir, "api.go")
	fileViolations := checkMetricsEmbedderReceivers(apiFile)
	violations = append(violations, fileViolations...)

	return violations
}

// checkMetricsEmbedderReceivers parses api.go and checks that MetricsEmbedder methods
// use value receivers (not pointer receivers).
func checkMetricsEmbedderReceivers(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Methods that must use value receivers for MetricsHolder interface
	targetMethods := map[string]bool{
		"GetWorkerMetrics":    true,
		"GetFrameworkMetrics": true,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		// Check if this is a MetricsEmbedder method
		recv := funcDecl.Recv.List[0]

		recvTypeName := getReceiverTypeName(recv.Type)
		if recvTypeName != "MetricsEmbedder" {
			return true
		}

		// Check if this is a target method
		if !targetMethods[funcDecl.Name.Name] {
			return true
		}

		// Check if receiver is a pointer (violation)
		if _, isPointer := recv.Type.(*ast.StarExpr); isPointer {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "POINTER_RECEIVER_ON_METRICS_EMBEDDER",
				Message: fmt.Sprintf("MetricsEmbedder.%s() uses pointer receiver (*MetricsEmbedder) - must use value receiver for interface satisfaction", funcDecl.Name.Name),
			})
		}

		return true
	})

	return violations
}

// getReceiverTypeName extracts the type name from a receiver (handles both T and *T).
func getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}

	return ""
}

// ValidateActionHistoryCopy checks that workers with dependencies copy action history.
// Workers that call GetDependencies() in CollectObservedState must also call
// GetActionHistory() to copy action history to the observed state.
// This follows the same pattern as FrameworkMetrics - supervisor sets on deps, worker reads and assigns.
func ValidateActionHistoryCopy(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkActionHistoryCopy(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkActionHistoryCopy verifies that CollectObservedState copies action history.
// The pattern is: if a worker has dependencies (calls GetDependencies), it should
// call GetActionHistory() to copy action history to the observed state.
//
// Workers without dependencies (like ApplicationWorker) are exempt - they intentionally
// don't have access to action history.
func checkActionHistoryCopy(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for CollectObservedState() method
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "CollectObservedState" {
			return true
		}

		hasGetDependencies := false
		hasGetActionHistory := false

		// Check for GetDependencies() and GetActionHistory() calls
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			callExpr, ok := bodyNode.(*ast.CallExpr)
			if !ok {
				return true
			}

			if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				switch selExpr.Sel.Name {
				case "GetDependencies":
					hasGetDependencies = true
				case "GetActionHistory":
					hasGetActionHistory = true
				}
			}

			return true
		})

		// If worker has dependencies but doesn't copy action history, flag it
		if hasGetDependencies && !hasGetActionHistory {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_ACTION_HISTORY_COPY",
				Message: "CollectObservedState() calls GetDependencies() but not GetActionHistory() - must copy action history from deps",
			})
		}

		return true
	})

	return violations
}
