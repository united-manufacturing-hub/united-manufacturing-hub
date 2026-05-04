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
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ValidateStateXORAction checks that Next() returns either state change OR action, not both.
func ValidateStateXORAction(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkStateXORAction(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStateXORAction parses a state file and checks return statements in Next().
// This now checks for fsmv2.Result(state, signal, action, reason, nil) calls.
func checkStateXORAction(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				return true
			}

			// Extract state, signal, action from fsmv2.Result() call
			stateResult, actionResult := extractResultArgs(retStmt.Results[0])
			if stateResult == nil || actionResult == nil {
				return true
			}

			stateIsChanging := false

			if unaryExpr, ok := stateResult.(*ast.UnaryExpr); ok {
				if compLit, ok := unaryExpr.X.(*ast.CompositeLit); ok {
					if ident, ok := compLit.Type.(*ast.Ident); ok {
						if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
							if starExpr, ok := funcDecl.Recv.List[0].Type.(*ast.StarExpr); ok {
								if recvIdent, ok := starExpr.X.(*ast.Ident); ok {
									if ident.Name != recvIdent.Name {
										stateIsChanging = true
									}
								}
							}
						}
					}
				}
			}

			actionIsNonNil := false

			if _, ok := actionResult.(*ast.Ident); !ok {
				if unaryExpr, ok := actionResult.(*ast.UnaryExpr); ok {
					if _, ok := unaryExpr.X.(*ast.CompositeLit); ok {
						actionIsNonNil = true
					}
				}
			} else {
				if ident, ok := actionResult.(*ast.Ident); ok && ident.Name != "nil" {
					actionIsNonNil = true
				}
			}

			if stateIsChanging && actionIsNonNil {
				pos := fset.Position(retStmt.Pos())
				violations = append(violations, Violation{
					File:    filename,
					Line:    pos.Line,
					Type:    "STATE_AND_ACTION",
					Message: "Return statement has both state change AND action (should be XOR)",
				})
			}

			return true
		})

		return true
	})

	return violations
}

// extractResultArgs extracts state and action arguments from a fsmv2.Result() call.
// Returns (state, action) or (nil, nil) if not a Result call.
func extractResultArgs(expr ast.Expr) (state, action ast.Expr) {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, nil
	}

	// Check if it's a call to fsmv2.Result or Result
	var funcName string

	switch fn := callExpr.Fun.(type) {
	case *ast.SelectorExpr:
		funcName = fn.Sel.Name
	case *ast.IndexExpr:
		// Handle generic syntax like fsmv2.Result[any, any](...)
		if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			funcName = sel.Sel.Name
		}
	case *ast.IndexListExpr:
		// Handle generic syntax like fsmv2.Result[any, any](...)
		if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			funcName = sel.Sel.Name
		}
	}

	if (funcName != "Result" && funcName != "Transition") || len(callExpr.Args) < 3 {
		return nil, nil
	}

	return callExpr.Args[0], callExpr.Args[2]
}

// extractResultArgsWithSignal extracts state, signal, and action arguments from a fsmv2.Result() call.
// Returns (state, signal, action) or (nil, nil, nil) if not a Result call.
func extractResultArgsWithSignal(expr ast.Expr) (state, signal, action ast.Expr) {
	callExpr, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, nil, nil
	}

	// Check if it's a call to fsmv2.Result or Result
	var funcName string

	switch fn := callExpr.Fun.(type) {
	case *ast.SelectorExpr:
		funcName = fn.Sel.Name
	case *ast.IndexExpr:
		// Handle generic syntax like fsmv2.Result[any, any](...)
		if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			funcName = sel.Sel.Name
		}
	case *ast.IndexListExpr:
		// Handle generic syntax like fsmv2.Result[any, any](...)
		if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			funcName = sel.Sel.Name
		}
	}

	if (funcName != "Result" && funcName != "Transition") || len(callExpr.Args) < 3 {
		return nil, nil, nil
	}

	return callExpr.Args[0], callExpr.Args[1], callExpr.Args[2]
}

// ValidateNoNilStateReturns checks that Next() never returns nil as state.
func ValidateNoNilStateReturns(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkNoNilStateReturns(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkNoNilStateReturns checks that Next() never returns nil as state.
func checkNoNilStateReturns(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				return true
			}

			// Extract state from fsmv2.Result() call
			stateResult, _ := extractResultArgs(retStmt.Results[0])
			if stateResult == nil {
				return true
			}

			if ident, ok := stateResult.(*ast.Ident); ok {
				if ident.Name == "nil" {
					pos := fset.Position(retStmt.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "NIL_STATE_RETURN",
						Message: "Next() returns nil as state in fsmv2.Result() (should return valid state)",
					})
				}
			}

			return true
		})

		return true
	})

	return violations
}

// ValidateSignalStateMutualExclusion checks signals only with same-state returns.
func ValidateSignalStateMutualExclusion(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkSignalStateMutualExclusion(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkSignalStateMutualExclusion checks signals only with same-state returns.
// This now checks for fsmv2.Result(state, signal, action, reason, nil) calls.
func checkSignalStateMutualExclusion(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				return true
			}

			// Extract state and signal from fsmv2.Result() call
			stateResult, signalResult, _ := extractResultArgsWithSignal(retStmt.Results[0])
			if stateResult == nil || signalResult == nil {
				return true
			}

			signalIsNone := false

			if selExpr, ok := signalResult.(*ast.SelectorExpr); ok {
				if selExpr.Sel.Name == "SignalNone" {
					signalIsNone = true
				}
			}

			if !signalIsNone {
				stateIsReceiver := false

				if ident, ok := stateResult.(*ast.Ident); ok {
					if ident.Name == "s" {
						stateIsReceiver = true
					}
				}

				if !stateIsReceiver {
					pos := fset.Position(retStmt.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "SIGNAL_STATE_MISMATCH",
						Message: "Signal sent with state change (signals only allowed with same-state returns)",
					})
				}
			}

			return true
		})

		return true
	})

	return violations
}

// ValidateStoppingStateNoCatchAllSelfReturn checks that states embedding StoppingBase
// do not have a catch-all self-return with nil action (which causes deadlocks).
// Self-return WITH an action is allowed (cleanup in progress).
func ValidateStoppingStateNoCatchAllSelfReturn(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkStoppingStateNoCatchAllSelfReturn(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStoppingStateNoCatchAllSelfReturn checks a single file for the stopping deadlock pattern.
// A StoppingBase state is flagged if it has NO return path that transitions to a different state.
// States that wait for supervisor-controlled conditions (e.g., children count) are safe because
// they have at least one return that transitions to StoppedState.
func checkStoppingStateNoCatchAllSelfReturn(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// First, check if any state struct in this file embeds helpers.StoppingBase.
	embedsStoppingBase := false

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasSuffix(typeSpec.Name.Name, "State") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				if selExpr, ok := field.Type.(*ast.SelectorExpr); ok {
					if pkgIdent, ok := selExpr.X.(*ast.Ident); ok {
						if pkgIdent.Name == "helpers" && selExpr.Sel.Name == "StoppingBase" {
							embedsStoppingBase = true

							return false
						}
					}
				}
			}
		}

		return true
	})

	if !embedsStoppingBase {
		return violations
	}

	// Check that the Next() method has at least one return that transitions to a different state
	// or carries a non-nil action. If all returns are self-returns with nil action, the state
	// will deadlock (no path to progress).
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
			return true
		}

		hasProgressPath := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				return true
			}

			stateResult, _, actionResult := extractResultArgsWithSignal(retStmt.Results[0])
			if stateResult == nil {
				return true
			}

			// Check if this return transitions to a different state (not self)
			isSelfReturn := false
			if ident, ok := stateResult.(*ast.Ident); ok && ident.Name == "s" {
				isSelfReturn = true
			}

			if !isSelfReturn {
				// Transitions to a different state — this is a progress path
				hasProgressPath = true

				return false
			}

			// Self-return with non-nil action is also a progress path (cleanup in progress)
			if actionResult != nil {
				if ident, ok := actionResult.(*ast.Ident); !ok || ident.Name != "nil" {
					hasProgressPath = true

					return false
				}
			}

			return true
		})

		if !hasProgressPath {
			// Get position of the last return for the violation
			lastStmt := funcDecl.Body.List[len(funcDecl.Body.List)-1]

			pos := fset.Position(lastStmt.Pos())
			violations = append(violations, Violation{
				File: filename,
				Line: pos.Line,
				Type: "STOPPING_STATE_DEADLOCK",
				Message: "StoppingState has no path to progress (all returns are self-returns with nil action). " +
					"StoppingState must have at least one return that transitions to StoppedState or carries a cleanup action.",
			})
		}

		return true
	})

	return violations
}
