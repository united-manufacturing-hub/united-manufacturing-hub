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
