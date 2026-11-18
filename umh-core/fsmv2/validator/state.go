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

// ValidateNextMethodTypeAssertions checks that Next() methods use fsmv2.ConvertWorkerSnapshot as the single entry-point.
func ValidateNextMethodTypeAssertions(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkSingleEntryPointPattern(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkSingleEntryPointPattern checks for type assertion or ConvertWorkerSnapshot at first statement.
func checkSingleEntryPointPattern(filename string) []Violation {
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

		var (
			typeAssertions       []int
			convertSnapshotCalls []int
			firstStatementLine   int
		)

		if funcDecl.Body != nil && len(funcDecl.Body.List) > 0 {
			firstStatementLine = fset.Position(funcDecl.Body.List[0].Pos()).Line
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			if typeAssert, ok := bodyNode.(*ast.TypeAssertExpr); ok {
				pos := fset.Position(typeAssert.Pos())
				typeAssertions = append(typeAssertions, pos.Line)

				return true
			}

			if callExpr, ok := bodyNode.(*ast.CallExpr); ok {
				if indexExpr, ok := callExpr.Fun.(*ast.IndexListExpr); ok {
					if selExpr, ok := indexExpr.X.(*ast.SelectorExpr); ok {
						if selExpr.Sel.Name == "ConvertWorkerSnapshot" {
							pos := fset.Position(callExpr.Pos())
							convertSnapshotCalls = append(convertSnapshotCalls, pos.Line)
						}
					}
				}
			}

			return true
		})

		totalEntryPoints := len(typeAssertions) + len(convertSnapshotCalls)

		switch {
		case totalEntryPoints == 0:
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(funcDecl.Pos()).Line,
				Type:    "MISSING_ENTRY_ASSERTION",
				Message: "Next() method missing entry-point type conversion (should use fsmv2.ConvertWorkerSnapshot at first statement)",
			})
		case totalEntryPoints > 1:
			allLines := make([]int, 0, len(typeAssertions)+len(convertSnapshotCalls))
			allLines = append(allLines, typeAssertions...)
			allLines = append(allLines, convertSnapshotCalls...)

			if len(allLines) > 1 {
				violations = append(violations, Violation{
					File:    filename,
					Line:    allLines[1],
					Type:    "MULTIPLE_ASSERTIONS",
					Message: fmt.Sprintf("Next() method has %d type conversions (should have exactly 1 at entry)", totalEntryPoints),
				})
			}
		default:
			var entryLine int
			if len(typeAssertions) == 1 {
				entryLine = typeAssertions[0]
			} else {
				entryLine = convertSnapshotCalls[0]
			}

			if entryLine != firstStatementLine {
				violations = append(violations, Violation{
					File:    filename,
					Line:    entryLine,
					Type:    "ASSERTION_NOT_AT_ENTRY",
					Message: "Type conversion should be first statement in Next() method",
				})
			}
		}

		return true
	})

	return violations
}

// ValidateShutdownCheckFirst checks that the first conditional in an active
// state's Next() reacts to both stop reasons: a shutdown and a disable.
func ValidateShutdownCheckFirst(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkShutdownCheckFirst(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// stopBasesByStateType maps each state type declared in file to the stop
// helper it embeds: "StoppedBase", "StoppingBase", or "" for an active state.
func stopBasesByStateType(file *ast.File) map[string]string {
	stopBases := map[string]string{}

	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		stopBases[typeSpec.Name.Name] = ""

		for _, field := range structType.Fields.List {
			if len(field.Names) > 0 {
				continue
			}

			embeddedType, ok := field.Type.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			packageName, ok := embeddedType.X.(*ast.Ident)
			if !ok || packageName.Name != "helpers" {
				continue
			}

			if embeddedType.Sel.Name == "StoppedBase" || embeddedType.Sel.Name == "StoppingBase" {
				stopBases[typeSpec.Name.Name] = embeddedType.Sel.Name
			}
		}

		return true
	})

	return stopBases
}

// receiverTypeName returns the type name of a method receiver, without the
// pointer, or "" for a plain function.
func receiverTypeName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return ""
	}

	receiverType := funcDecl.Recv.List[0].Type
	if pointer, ok := receiverType.(*ast.StarExpr); ok {
		receiverType = pointer.X
	}

	typeName, ok := receiverType.(*ast.Ident)
	if !ok {
		return ""
	}

	return typeName.Name
}

// receiverName returns the variable name of a method receiver, or "" when the
// receiver is unnamed.
func receiverName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 || len(funcDecl.Recv.List[0].Names) == 0 {
		return ""
	}

	return funcDecl.Recv.List[0].Names[0].Name
}

// selectorName returns the field or method name of expr when expr is a plain
// selector such as snap.IsDisabled, and "" otherwise.
func selectorName(expr ast.Expr) string {
	selector, ok := ast.Unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	return selector.Sel.Name
}

// isStopCondition reports whether cond is snap.ShouldStop() or
// snap.IsShutdownRequested || snap.IsDisabled, in either operand order.
func isStopCondition(cond ast.Expr) bool {
	cond = ast.Unparen(cond)

	if call, ok := cond.(*ast.CallExpr); ok {
		return len(call.Args) == 0 && selectorName(call.Fun) == "ShouldStop"
	}

	binary, ok := cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LOR {
		return false
	}

	left := selectorName(binary.X)
	right := selectorName(binary.Y)

	if left == "IsShutdownRequested" && right == "IsDisabled" {
		return true
	}

	return left == "IsDisabled" && right == "IsShutdownRequested"
}

// checkShutdownCheckFirst parses a state file and checks that the first
// conditional of every active state's Next() is a stop condition.
func checkShutdownCheckFirst(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	stopBases := stopBasesByStateType(node)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		if stopBases[receiverTypeName(funcDecl)] != "" {
			return true
		}

		if funcDecl.Body == nil || len(funcDecl.Body.List) < 2 {
			return true
		}

		var firstIfStmt *ast.IfStmt
		for _, stmt := range funcDecl.Body.List[1:] {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				firstIfStmt = ifStmt

				break
			}
		}

		if firstIfStmt == nil {
			return true
		}

		if isStopCondition(firstIfStmt.Cond) {
			return true
		}

		pos := fset.Position(firstIfStmt.Pos())
		violations = append(violations, Violation{
			File:    filename,
			Line:    pos.Line,
			Type:    "SHUTDOWN_CHECK_NOT_FIRST",
			Message: "First conditional in Next() is not snap.ShouldStop()",
		})

		return true
	})

	return violations
}

// ValidateStoppedStateHandlesDisable checks that every Stopped state's Next()
// keeps a disabled worker in Stopped instead of starting or removing it.
func ValidateStoppedStateHandlesDisable(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkStoppedStateHandlesDisable(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// returnStatementsIn lists the return statements in node, leaving out those
// inside function literals, which return from the literal and not from Next().
func returnStatementsIn(node ast.Node) []*ast.ReturnStmt {
	var returns []*ast.ReturnStmt

	ast.Inspect(node, func(child ast.Node) bool {
		if _, ok := child.(*ast.FuncLit); ok {
			return false
		}

		if returnStmt, ok := child.(*ast.ReturnStmt); ok {
			returns = append(returns, returnStmt)
		}

		return true
	})

	return returns
}

// returnStaysInState reports whether returnStmt returns a transition whose
// target state is the receiver itself, such as fsmv2.Transition(s, ...).
func returnStaysInState(returnStmt *ast.ReturnStmt, receiver string) bool {
	if receiver == "" || len(returnStmt.Results) != 1 {
		return false
	}

	call, ok := returnStmt.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}

	target, ok := call.Args[0].(*ast.Ident)

	return ok && target.Name == receiver
}

// returnSignalsRemoval reports whether returnStmt passes SignalNeedsRemoval.
func returnSignalsRemoval(returnStmt *ast.ReturnStmt) bool {
	signalsRemoval := false

	ast.Inspect(returnStmt, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == "SignalNeedsRemoval" {
			signalsRemoval = true
		}

		return true
	})

	return signalsRemoval
}

// isDisableBranch reports whether ifStmt is if snap.IsDisabled { ... }.
func isDisableBranch(ifStmt *ast.IfStmt) bool {
	return ifStmt.Init == nil && selectorName(ifStmt.Cond) == "IsDisabled"
}

// checkStoppedStateHandlesDisable checks that a Stopped state's Next() reaches
// an if snap.IsDisabled branch before any return that leaves Stopped, and that
// the branch stays in Stopped without signalling removal.
func checkStoppedStateHandlesDisable(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	stopBases := stopBasesByStateType(node)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" || funcDecl.Body == nil {
			return true
		}

		if stopBases[receiverTypeName(funcDecl)] != "StoppedBase" {
			return true
		}

		message := stoppedDisableProblem(funcDecl.Body.List, receiverName(funcDecl))
		if message == "" {
			return true
		}

		pos := fset.Position(funcDecl.Pos())
		violations = append(violations, Violation{
			File:    filename,
			Line:    pos.Line,
			Type:    "STOPPED_IGNORES_DISABLE",
			Message: message,
		})

		return true
	})

	return violations
}

// stoppedDisableProblem describes why the statements of a Stopped state's
// Next() do not keep a disabled worker in Stopped, or returns "" when they do.
func stoppedDisableProblem(statements []ast.Stmt, receiver string) string {
	for _, stmt := range statements {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if ok && isDisableBranch(ifStmt) {
			return disableBranchProblem(ifStmt, receiver)
		}

		for _, returnStmt := range returnStatementsIn(stmt) {
			if !returnStaysInState(returnStmt, receiver) {
				return "Stopped state's Next() can leave Stopped before it checks snap.IsDisabled"
			}
		}
	}

	return "Stopped state's Next() has no if snap.IsDisabled branch"
}

// disableBranchProblem describes why an if snap.IsDisabled branch does not
// keep the worker in Stopped, or returns "" when it does.
func disableBranchProblem(disableBranch *ast.IfStmt, receiver string) string {
	returns := returnStatementsIn(disableBranch.Body)
	if len(returns) == 0 {
		return "the if snap.IsDisabled branch in a Stopped state's Next() does not return"
	}

	for _, returnStmt := range returns {
		if !returnStaysInState(returnStmt, receiver) {
			return "the if snap.IsDisabled branch in a Stopped state's Next() leaves Stopped"
		}

		if returnSignalsRemoval(returnStmt) {
			return "the if snap.IsDisabled branch in a Stopped state's Next() signals removal"
		}
	}

	return ""
}

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
// This now checks for fsmv2.Result(state, signal, action, reason) calls.
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

// ValidateStateStringAndReason checks that all state types have String() methods.
// Note: Reason() method was removed from the State interface - reason now comes from NextResult.Reason.
func ValidateStateStringAndReason(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkStateStringMethod(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStateStringMethod parses a state file and checks for String() method.
func checkStateStringMethod(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	stateTypes := make(map[string]token.Pos)

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasSuffix(typeSpec.Name.Name, "State") {
			return true
		}

		if _, ok := typeSpec.Type.(*ast.StructType); ok {
			stateTypes[typeSpec.Name.Name] = typeSpec.Pos()
		}

		return true
	})

	typesWithString := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		var typeName string

		switch recvType := funcDecl.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if ident, ok := recvType.X.(*ast.Ident); ok {
				typeName = ident.Name
			}
		case *ast.Ident:
			typeName = recvType.Name
		}

		if typeName != "" && funcDecl.Name.Name == "String" {
			typesWithString[typeName] = true
		}

		return true
	})

	for typeName, pos := range stateTypes {
		if !typesWithString[typeName] {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(pos).Line,
				Type:    "STATE_MISSING_STRING_METHOD",
				Message: fmt.Sprintf("State %s missing String() string method", typeName),
			})
		}
	}

	return violations
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
// This now checks for fsmv2.Result(state, signal, action, reason) calls.
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

// ValidateTryingToStatesReturnActions checks that states named "TryingTo*" return actions.
func ValidateTryingToStatesReturnActions(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkTryingToStatesReturnActions(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkTryingToStatesReturnActions checks if TryingTo states return actions.
// This now checks for fsmv2.Result(state, signal, action, reason) calls where action is non-nil.
func checkTryingToStatesReturnActions(filename string) []Violation {
	var violations []Violation

	baseName := filepath.Base(filename)
	if !strings.Contains(baseName, "trying_to") {
		return violations
	}

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

		hasActionReturn := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				return true
			}

			// Check for fsmv2.Result(...) call
			callExpr, ok := retStmt.Results[0].(*ast.CallExpr)
			if !ok {
				return true
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

			if funcName != "Result" && funcName != "Transition" {
				return true
			}

			// fsmv2.Result(state, signal, action, reason) - action is the 3rd argument (index 2)
			if len(callExpr.Args) >= 3 {
				actionArg := callExpr.Args[2]

				// Check if action is not nil
				if ident, ok := actionArg.(*ast.Ident); ok {
					if ident.Name != "nil" {
						hasActionReturn = true

						return false
					}
				} else if unaryExpr, ok := actionArg.(*ast.UnaryExpr); ok {
					// Handle &SomeAction{}
					if _, ok := unaryExpr.X.(*ast.CompositeLit); ok {
						hasActionReturn = true

						return false
					}
				} else {
					// Any other expression is likely a non-nil action
					hasActionReturn = true

					return false
				}
			}

			return true
		})

		if !hasActionReturn {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "TRYINGTO_NO_ACTION",
				Message: "TryingTo state never returns an action (should return action or be renamed)",
			})
		}

		return true
	})

	return violations
}

// ValidateBaseStateEmbedding checks that state structs embed Base*State.
func ValidateBaseStateEmbedding(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkBaseStateEmbedding(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkBaseStateEmbedding checks for Base*State or phase-specific base type embedding.
func checkBaseStateEmbedding(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Valid phase-specific base types from helpers package
	validPhaseBaseTypes := map[string]bool{
		"StartingBase":        true,
		"RunningHealthyBase":  true,
		"RunningDegradedBase": true,
		"StoppingBase":        true,
		"StoppedBase":         true,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasSuffix(typeSpec.Name.Name, "State") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		hasBaseState := false

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				// Check for old pattern: BaseXxxState (simple identifier)
				if ident, ok := field.Type.(*ast.Ident); ok {
					if strings.HasPrefix(ident.Name, "Base") && strings.HasSuffix(ident.Name, "State") {
						hasBaseState = true

						break
					}
				}
				// Check for new pattern: helpers.XxxBase (selector expression)
				if selExpr, ok := field.Type.(*ast.SelectorExpr); ok {
					if pkgIdent, ok := selExpr.X.(*ast.Ident); ok {
						if pkgIdent.Name == "helpers" && validPhaseBaseTypes[selExpr.Sel.Name] {
							hasBaseState = true

							break
						}
					}
				}
			}
		}

		if !hasBaseState {
			pos := fset.Position(typeSpec.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_BASE_STATE",
				Message: fmt.Sprintf("State %s does not embed a Base*State type", typeSpec.Name.Name),
			})
		}

		return true
	})

	return violations
}

// GetStateFilePath converts a type name to a state file path (e.g., "TryingToConnectState" -> "state_trying_to_connect.go").
func GetStateFilePath(fsmv2Dir, typeName string) string {
	fileName := strings.ToLower(strings.ReplaceAll(typeName, "State", ""))
	fileName = "state_" + strings.ToLower(strings.ReplaceAll(fileName, "TryingTo", "trying_to_"))
	fileName = strings.ReplaceAll(fileName, "__", "_")

	return filepath.Join(fsmv2Dir, "workers", "example", fileName+".go")
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
				// Transitions to a different state  -  this is a progress path
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
