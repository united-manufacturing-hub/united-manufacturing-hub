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
	"strings"
)

// ValidateActionStructs checks that action structs have no mutable state.
func ValidateActionStructs(baseDir string) []Violation {
	var violations []Violation

	actionFiles := FindActionFiles(baseDir)

	for _, file := range actionFiles {
		fileViolations := checkActionFile(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkActionFile parses an action file and looks for mutable state fields.
func checkActionFile(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "Action") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				continue
			}

			fieldName := field.Names[0].Name
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

// ValidateContextCancellationInActions checks that Execute() handles ctx.Done().
func ValidateContextCancellationInActions(baseDir string) []Violation {
	var violations []Violation

	actionFiles := FindActionFiles(baseDir)

	for _, file := range actionFiles {
		fileViolations := checkContextCancellationInAction(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkContextCancellationInAction parses an action file and checks for ctx.Done() handling.
func checkContextCancellationInAction(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Execute" {
			return true
		}

		hasContextCheck := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			if selectStmt, ok := bodyNode.(*ast.SelectStmt); ok {
				for _, commClause := range selectStmt.Body.List {
					if clause, ok := commClause.(*ast.CommClause); ok {
						if clause.Comm != nil {
							if exprStmt, ok := clause.Comm.(*ast.ExprStmt); ok {
								if unaryExpr, ok := exprStmt.X.(*ast.UnaryExpr); ok && unaryExpr.Op == token.ARROW {
									if callExpr, ok := unaryExpr.X.(*ast.CallExpr); ok {
										if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
											if xIdent, ok := selExpr.X.(*ast.Ident); ok && xIdent.Name == "ctx" {
												if selExpr.Sel.Name == "Done" {
													hasContextCheck = true

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
			}

			if ifStmt, ok := bodyNode.(*ast.IfStmt); ok {
				ast.Inspect(ifStmt.Cond, func(condNode ast.Node) bool {
					if callExpr, ok := condNode.(*ast.CallExpr); ok {
						if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
							if xIdent, ok := selExpr.X.(*ast.Ident); ok && xIdent.Name == "ctx" {
								if selExpr.Sel.Name == "Done" {
									hasContextCheck = true

									return false
								}
							}
						}
					}

					return true
				})
			}

			return true
		})

		if !hasContextCheck {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_CONTEXT_CANCELLATION_ACTION",
				Message: "Execute() must handle context cancellation via ctx.Done() check",
			})
		}

		return true
	})

	return violations
}

// ValidateNoInternalRetryLoops checks that actions don't have retry loops.
func ValidateNoInternalRetryLoops(baseDir string) []Violation {
	var violations []Violation

	actionFiles := FindActionFiles(baseDir)

	for _, file := range actionFiles {
		fileViolations := checkNoInternalRetryLoops(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkNoInternalRetryLoops parses an action file and checks for retry loops in Execute.
func checkNoInternalRetryLoops(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Execute" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			forStmt, ok := bodyNode.(*ast.ForStmt)
			if !ok {
				if rangeStmt, ok := bodyNode.(*ast.RangeStmt); ok {
					if hasErrorHandling(rangeStmt.Body) {
						pos := fset.Position(rangeStmt.Pos())
						violations = append(violations, Violation{
							File:    filename,
							Line:    pos.Line,
							Type:    "INTERNAL_RETRY_LOOP",
							Message: "Execute() has internal retry loop with error handling (supervisor manages retries)",
						})
					}
				}

				return true
			}

			if hasErrorHandling(forStmt.Body) {
				pos := fset.Position(forStmt.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "INTERNAL_RETRY_LOOP",
					Message: "Execute() has internal retry loop with error handling (supervisor manages retries)",
				})
			}

			return true
		})

		return true
	})

	return violations
}

// hasErrorHandling checks if a block statement contains error handling patterns.
func hasErrorHandling(body *ast.BlockStmt) bool {
	hasError := false

	ast.Inspect(body, func(n ast.Node) bool {
		if retStmt, ok := n.(*ast.ReturnStmt); ok {
			for _, result := range retStmt.Results {
				if ident, ok := result.(*ast.Ident); ok && ident.Name == "err" {
					hasError = true

					return false
				}
			}
		}

		if ifStmt, ok := n.(*ast.IfStmt); ok {
			if binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
				if binExpr.Op == token.NEQ {
					if ident, ok := binExpr.X.(*ast.Ident); ok && ident.Name == "err" {
						if nilIdent, ok := binExpr.Y.(*ast.Ident); ok && nilIdent.Name == "nil" {
							hasError = true

							return false
						}
					}
				}
			}
		}

		return true
	})

	return hasError
}

// ValidateNoChannelOperations checks that actions don't use channels or goroutines.
func ValidateNoChannelOperations(baseDir string) []Violation {
	var violations []Violation

	actionFiles := FindActionFiles(baseDir)

	for _, file := range actionFiles {
		fileViolations := checkNoChannelOperations(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkNoChannelOperations parses an action file and checks for channel/goroutine usage.
func checkNoChannelOperations(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Execute" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			if goStmt, ok := bodyNode.(*ast.GoStmt); ok {
				pos := fset.Position(goStmt.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "CHANNEL_OPERATION_IN_ACTION",
					Message: "Execute() uses goroutine (actions should be synchronous)",
				})
			}

			if chanType, ok := bodyNode.(*ast.ChanType); ok {
				pos := fset.Position(chanType.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "CHANNEL_OPERATION_IN_ACTION",
					Message: "Execute() uses channel type (actions should be synchronous)",
				})
			}

			if sendStmt, ok := bodyNode.(*ast.SendStmt); ok {
				pos := fset.Position(sendStmt.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "CHANNEL_OPERATION_IN_ACTION",
					Message: "Execute() uses channel send (actions should be synchronous)",
				})
			}

			return true
		})

		return true
	})

	return violations
}
