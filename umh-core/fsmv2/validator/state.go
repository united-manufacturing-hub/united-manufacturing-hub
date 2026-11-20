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

// ValidateStopCheckFirst checks that the first conditional in an active
// state's Next() reacts to both stop reasons: a shutdown and a disable.
func ValidateStopCheckFirst(baseDir string) []Violation {
	var violations []Violation

	for _, file := range FindStateFiles(baseDir) {
		violations = append(violations, checkStopCheckFirst(file)...)
	}

	return violations
}

// ValidateStoppedStateHandlesDisable checks that every Stopped state's Next()
// keeps a disabled worker in Stopped instead of starting or removing it.
func ValidateStoppedStateHandlesDisable(baseDir string) []Violation {
	var violations []Violation

	for _, file := range FindStateFiles(baseDir) {
		violations = append(violations, checkStoppedStateHandlesDisable(file)...)
	}

	return violations
}

// stateKind says where a state sits in the lifecycle, by the helper it embeds.
type stateKind int

const (
	activeState stateKind = iota
	stoppingState
	stoppedState
)

// nextMethod is a state's Next() method with the two names the stop rules need.
type nextMethod struct {
	decl      *ast.FuncDecl
	stateType string
	// empty when the receiver is unnamed, so no return can name it
	receiverName string
}

// checkStopCheckFirst checks that the first conditional of every active
// state's Next() in filename is a stop condition.
func checkStopCheckFirst(filename string) []Violation {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	kinds := stateKindsByType(file)

	var violations []Violation

	for _, next := range nextMethodsIn(file) {
		kind, declaredInFile := kinds[next.stateType]
		if declaredInFile && kind != activeState {
			continue
		}

		firstIf := firstIfAfterConversion(next.decl.Body)
		if firstIf == nil {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(next.decl.Pos()).Line,
				Type:    "STOP_CHECK_NOT_FIRST",
				Message: "Next() has no if statement after the conversion, so it never checks snap.ShouldStop()",
			})

			continue
		}

		if isStopCondition(firstIf.Cond) {
			continue
		}

		violations = append(violations, Violation{
			File:    filename,
			Line:    fset.Position(firstIf.Pos()).Line,
			Type:    "STOP_CHECK_NOT_FIRST",
			Message: "First conditional in Next() is not snap.ShouldStop()",
		})
	}

	return violations
}

// checkStoppedStateHandlesDisable checks that every Stopped state's Next() in
// filename keeps a disabled worker in Stopped.
func checkStoppedStateHandlesDisable(filename string) []Violation {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	kinds := stateKindsByType(file)

	var violations []Violation

	for _, next := range nextMethodsIn(file) {
		if kinds[next.stateType] != stoppedState {
			continue
		}

		problem, found := disableProblemIn(next)
		if !found {
			continue
		}

		violations = append(violations, Violation{
			File:    filename,
			Line:    fset.Position(next.decl.Pos()).Line,
			Type:    "STOPPED_IGNORES_DISABLE",
			Message: problem,
		})
	}

	return violations
}

// stateKindsByType maps each struct type declared in file to its stateKind.
func stateKindsByType(file *ast.File) map[string]stateKind {
	kinds := map[string]stateKind{}

	for _, typeSpec := range typeSpecsIn(file) {
		structType, isStruct := typeSpec.Type.(*ast.StructType)
		if !isStruct {
			continue
		}

		kinds[typeSpec.Name.Name] = embeddedStateKind(structType)
	}

	return kinds
}

func typeSpecsIn(file *ast.File) []*ast.TypeSpec {
	var typeSpecs []*ast.TypeSpec

	for _, decl := range file.Decls {
		genDecl, isGenDecl := decl.(*ast.GenDecl)
		if !isGenDecl || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpecs = append(typeSpecs, spec.(*ast.TypeSpec))
		}
	}

	return typeSpecs
}

func embeddedStateKind(structType *ast.StructType) stateKind {
	for _, field := range structType.Fields.List {
		isEmbedded := len(field.Names) == 0
		if !isEmbedded {
			continue
		}

		switch qualifiedName(field.Type) {
		case "helpers.StoppedBase":
			return stoppedState
		case "helpers.StoppingBase":
			return stoppingState
		}
	}

	return activeState
}

// qualifiedName returns "pkg.Name" for a selector such as helpers.StoppedBase.
func qualifiedName(expr ast.Expr) string {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}

	packageName, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return ""
	}

	return packageName.Name + "." + selector.Sel.Name
}

// nextMethodsIn lists the Next() methods declared in file.
func nextMethodsIn(file *ast.File) []nextMethod {
	var methods []nextMethod

	for _, decl := range file.Decls {
		funcDecl, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || funcDecl.Name.Name != "Next" || funcDecl.Recv == nil || funcDecl.Body == nil {
			continue
		}

		methods = append(methods, newNextMethod(funcDecl))
	}

	return methods
}

func newNextMethod(funcDecl *ast.FuncDecl) nextMethod {
	receiver := funcDecl.Recv.List[0]

	receiverType := receiver.Type
	if pointer, isPointer := receiverType.(*ast.StarExpr); isPointer {
		receiverType = pointer.X
	}

	method := nextMethod{decl: funcDecl}

	if typeName, isIdent := receiverType.(*ast.Ident); isIdent {
		method.stateType = typeName.Name
	}

	if len(receiver.Names) > 0 {
		method.receiverName = receiver.Names[0].Name
	}

	return method
}

// firstIfAfterConversion returns the first if statement after the snapshot
// conversion on the first line of Next(), or nil when there is none.
func firstIfAfterConversion(body *ast.BlockStmt) *ast.IfStmt {
	if len(body.List) < 2 {
		return nil
	}

	for _, stmt := range body.List[1:] {
		if ifStmt, isIf := stmt.(*ast.IfStmt); isIf {
			return ifStmt
		}
	}

	return nil
}

// selectorName returns the field or method name of expr when expr is a plain
// selector such as snap.IsDisabled, and "" otherwise.
func selectorName(expr ast.Expr) string {
	selector, isSelector := ast.Unparen(expr).(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}

	return selector.Sel.Name
}

// isStopCondition reports whether cond is snap.ShouldStop() or
// snap.IsShutdownRequested || snap.IsDisabled, in either operand order.
func isStopCondition(cond ast.Expr) bool {
	cond = ast.Unparen(cond)

	if call, isCall := cond.(*ast.CallExpr); isCall {
		return len(call.Args) == 0 && selectorName(call.Fun) == "ShouldStop"
	}

	binary, isBinary := cond.(*ast.BinaryExpr)
	if !isBinary || binary.Op != token.LOR {
		return false
	}

	left := selectorName(binary.X)
	right := selectorName(binary.Y)

	if left == "IsShutdownRequested" && right == "IsDisabled" {
		return true
	}

	return left == "IsDisabled" && right == "IsShutdownRequested"
}

// disableProblemIn describes why a Stopped state's Next() does not keep a
// disabled worker in Stopped. found is false when it does.
func disableProblemIn(next nextMethod) (problem string, found bool) {
	for _, stmt := range next.decl.Body.List {
		ifStmt, isIf := stmt.(*ast.IfStmt)
		if isIf && isDisableBranch(ifStmt) {
			return disableBranchProblem(ifStmt, next.receiverName)
		}

		if leavesState(stmt, next.receiverName) {
			return "Stopped state's Next() can leave Stopped before it checks snap.IsDisabled", true
		}

		if mentionsRemoval(stmt) && !isShutdownBranch(stmt) {
			return "Stopped state's Next() can signal removal outside an if snap.IsShutdownRequested branch before it checks snap.IsDisabled", true
		}
	}

	return "Stopped state's Next() has no if snap.IsDisabled branch", true
}

// disableBranchProblem describes why an if snap.IsDisabled branch does not
// keep the worker in Stopped. found is false when it does.
func disableBranchProblem(disableBranch *ast.IfStmt, receiverName string) (problem string, found bool) {
	if !endsWithReturn(disableBranch.Body) {
		return "the if snap.IsDisabled branch in a Stopped state's Next() does not end with a return", true
	}

	if leavesState(disableBranch.Body, receiverName) {
		return "the if snap.IsDisabled branch in a Stopped state's Next() leaves Stopped", true
	}

	if mentionsRemoval(disableBranch.Body) {
		return "the if snap.IsDisabled branch in a Stopped state's Next() signals removal", true
	}

	return "", false
}

// endsWithReturn reports whether the last statement of block is a return, so
// that every path through block returns.
func endsWithReturn(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}

	_, isReturn := block.List[len(block.List)-1].(*ast.ReturnStmt)

	return isReturn
}

// isShutdownBranch reports whether stmt is if snap.IsShutdownRequested { ... }
// without an else branch.
func isShutdownBranch(stmt ast.Stmt) bool {
	ifStmt, isIf := stmt.(*ast.IfStmt)

	return isIf && ifStmt.Init == nil && ifStmt.Else == nil &&
		selectorName(ifStmt.Cond) == "IsShutdownRequested"
}

// isDisableBranch reports whether ifStmt is if snap.IsDisabled { ... }.
func isDisableBranch(ifStmt *ast.IfStmt) bool {
	return ifStmt.Init == nil && selectorName(ifStmt.Cond) == "IsDisabled"
}

// leavesState reports whether node holds a return that does not transition to
// the receiver itself, as fsmv2.Transition(s, ...) does.
func leavesState(node ast.Node, receiverName string) bool {
	for _, returnStmt := range returnStatementsIn(node) {
		if !returnsReceiver(returnStmt, receiverName) {
			return true
		}
	}

	return false
}

func returnsReceiver(returnStmt *ast.ReturnStmt, receiverName string) bool {
	if len(returnStmt.Results) != 1 {
		return false
	}

	call, isCall := returnStmt.Results[0].(*ast.CallExpr)
	if !isCall || len(call.Args) == 0 {
		return false
	}

	target, isIdent := call.Args[0].(*ast.Ident)

	return isIdent && target.Name == receiverName
}

// returnStatementsIn lists the return statements in node, leaving out those
// inside function literals, which return from the literal and not from Next().
func returnStatementsIn(node ast.Node) []*ast.ReturnStmt {
	var returns []*ast.ReturnStmt

	ast.Inspect(node, func(child ast.Node) bool {
		if _, isFuncLit := child.(*ast.FuncLit); isFuncLit {
			return false
		}

		if returnStmt, isReturn := child.(*ast.ReturnStmt); isReturn {
			returns = append(returns, returnStmt)
		}

		return true
	})

	return returns
}

// mentionsRemoval reports whether node names SignalNeedsRemoval, qualified or not.
func mentionsRemoval(node ast.Node) bool {
	found := false

	ast.Inspect(node, func(child ast.Node) bool {
		identifier, isIdent := child.(*ast.Ident)
		if isIdent && identifier.Name == "SignalNeedsRemoval" {
			found = true
		}

		return !found
	})

	return found
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
