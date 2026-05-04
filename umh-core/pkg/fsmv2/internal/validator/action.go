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
// A field is considered mutable only when Execute() directly assigns to it
// (via =, +=, ++, etc.). Fields that are only read in Execute are allowed,
// so immutable config captured at action construction time does not trigger
// a violation.
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

		execFn, receiverName := findExecuteMethod(node, typeSpec.Name.Name)
		var mutations map[string]token.Pos
		if execFn != nil && receiverName != "" {
			mutations = findReceiverFieldMutations(execFn, receiverName)
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				continue
			}

			fieldName := field.Names[0].Name
			if mutPos, mutated := mutations[fieldName]; mutated {
				pos := fset.Position(mutPos)
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "STATELESS_ACTION",
					Message: fmt.Sprintf("Action %s mutates field '%s' in Execute (actions should be stateless; mutate on deps instead)", typeSpec.Name.Name, fieldName),
				})
			}
		}

		return true
	})

	return violations
}

// findExecuteMethod locates the Execute method declaration for the named action
// struct within the parsed file. It returns the function declaration and the
// receiver variable name (e.g. "a" from `func (a *ConnectAction) Execute`).
// Returns nil, "" when no matching Execute method is found.
//
// Limitation: mutation detection only works when the action struct and its Execute
// method are defined in the same file. If Execute is in a separate file, the AST
// walk will not find it and mutations in that method will be silently missed.
// No current worker has this split, but future maintainers should keep struct and
// Execute co-located to keep this validator effective.
func findExecuteMethod(node *ast.File, structName string) (*ast.FuncDecl, string) {
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Execute" {
			continue
		}
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		recv := funcDecl.Recv.List[0]
		if extractReceiverTypeName(recv.Type) != structName {
			continue
		}
		if len(recv.Names) == 0 {
			// Receiver has no variable name — cannot reference fields.
			return funcDecl, ""
		}
		return funcDecl, recv.Names[0].Name
	}
	return nil, ""
}

// findReceiverFieldMutations walks the body of an Execute function and returns
// all receiver fields that are directly assigned (written) to. Read-only access
// does not produce an entry. The map value is the position of the first mutation.
func findReceiverFieldMutations(execFn *ast.FuncDecl, receiverName string) map[string]token.Pos {
	mutations := make(map[string]token.Pos)

	isReceiverField := func(expr ast.Expr) (string, bool) {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != receiverName {
			return "", false
		}
		return sel.Sel.Name, true
	}

	recordMutation := func(name string, pos token.Pos) {
		if _, already := mutations[name]; !already {
			mutations[name] = pos
		}
	}

	ast.Inspect(execFn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				if name, ok := isReceiverField(lhs); ok {
					recordMutation(name, stmt.Pos())
				}
				// Also catch `*a.ptr = v` (first-level pointer field write).
				if star, ok := lhs.(*ast.StarExpr); ok {
					if name, ok := isReceiverField(star.X); ok {
						recordMutation(name, stmt.Pos())
					}
				}
			}
		case *ast.IncDecStmt:
			if name, ok := isReceiverField(stmt.X); ok {
				recordMutation(name, stmt.Pos())
			}
		}
		return true
	})

	return mutations
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

// extractReceiverTypeName returns the base type name from a receiver type expression.
func extractReceiverTypeName(expr ast.Expr) string {
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
