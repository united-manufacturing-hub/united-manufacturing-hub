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
	"os"
	"path/filepath"
	"strings"
)

// ValidateNextMethodTypeAssertions checks that Next() methods use the single entry-point type assertion pattern.
func ValidateNextMethodTypeAssertions(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkSingleEntryPointPattern(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkSingleEntryPointPattern checks for type assertion or ConvertSnapshot at first statement.
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
				if isConvertSnapshotCall(callExpr) {
					pos := fset.Position(callExpr.Pos())
					convertSnapshotCalls = append(convertSnapshotCalls, pos.Line)
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
				Message: "Next() method missing entry-point type conversion (should use type assertion or ConvertSnapshot at first statement)",
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

// ValidateShutdownCheckFirst checks that Next() methods check IsShutdownRequested as first conditional.
func ValidateShutdownCheckFirst(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkShutdownCheckFirst(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkShutdownCheckFirst parses a state file and checks if shutdown is checked first.
func checkShutdownCheckFirst(filename string) []Violation {
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

		isShutdownCheck := false

		ast.Inspect(firstIfStmt.Cond, func(condNode ast.Node) bool {
			// Old API: method call like snap.Desired.IsShutdownRequested()
			if callExpr, ok := condNode.(*ast.CallExpr); ok {
				if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					if selExpr.Sel.Name == "IsShutdownRequested" || selExpr.Sel.Name == "ShouldStop" {
						isShutdownCheck = true

						return false
					}
				}
			}

			// New API: field access like snap.IsShutdownRequested
			if selExpr, ok := condNode.(*ast.SelectorExpr); ok {
				if selExpr.Sel.Name == "IsShutdownRequested" {
					isShutdownCheck = true

					return false
				}
			}

			return true
		})

		if !isShutdownCheck {
			baseName := filepath.Base(filename)
			if strings.Contains(baseName, "stopped") || strings.Contains(baseName, "trying_to_stop") || strings.Contains(baseName, "stopping") {
				return true
			}

			pos := fset.Position(firstIfStmt.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "SHUTDOWN_CHECK_NOT_FIRST",
				Message: "First conditional in Next() is not IsShutdownRequested check",
			})
		}

		return true
	})

	return violations
}

// ValidateChildWorkersUseShouldStop checks that child workers use ShouldStop() not IsShutdownRequested().
func ValidateChildWorkersUseShouldStop(baseDir string) []Violation {
	var violations []Violation

	workersDir := filepath.Join(baseDir, "workers")

	entries, err := os.ReadDir(workersDir)
	if err != nil {
		return violations
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workerDir := filepath.Join(workersDir, entry.Name())

		subEntries, err := os.ReadDir(workerDir)
		if err != nil {
			continue
		}

		for _, subEntry := range subEntries {
			if !subEntry.IsDir() {
				continue
			}

			subWorkerDir := filepath.Join(workerDir, subEntry.Name())
			violations = append(violations, checkChildWorkerUseShouldStop(subWorkerDir)...)
		}

		violations = append(violations, checkChildWorkerUseShouldStop(workerDir)...)
	}

	return violations
}

// checkChildWorkerUseShouldStop checks a single worker directory.
func checkChildWorkerUseShouldStop(workerDir string) []Violation {
	var violations []Violation

	if !isChildWorker(workerDir) {
		return violations
	}

	stateDir := filepath.Join(workerDir, "state")

	stateFiles, err := filepath.Glob(filepath.Join(stateDir, "state_*.go"))
	if err != nil {
		return violations
	}

	for _, stateFile := range stateFiles {
		baseName := filepath.Base(stateFile)
		if strings.HasSuffix(baseName, "_test.go") {
			continue
		}

		if strings.Contains(baseName, "stopped") || strings.Contains(baseName, "trying_to_stop") || strings.Contains(baseName, "stopping") {
			continue
		}

		if !checkFirstConditionalUsesShouldStop(stateFile) {
			violations = append(violations, Violation{
				File:    stateFile,
				Type:    "CHILD_MUST_USE_SHOULD_STOP",
				Message: "Child worker state uses IsShutdownRequested() instead of ShouldStop()",
			})
		}
	}

	return violations
}

// isChildWorker checks if a worker is a child worker by scanning its state files
// for parent-state-driven stop logic. A worker is treated as a child when any of
// its state/*.go files reference ParentMappedState or call ShouldStop().
//
// The previous heuristic scanned snapshot/snapshot.go for the literal
// "IsStopRequired()" string, which became vacuous after the P1.5b rename and
// was already broken before that (worker observed-state methods live in
// snapshot.go but their callers live in state/*.go; the rule we care about
// is "do state files use the parent-aware stop check").
//
// Known limitations:
//
//   - Comment / string-literal false positives. The substring scan via
//     bytes.Contains matches occurrences inside Go comments and string
//     literals, not just live identifier references. A state/*.go file that
//     mentions "ParentMappedState" or "ShouldStop()" only inside an
//     explanatory comment would be classified as a child worker even if it
//     never calls the symbol. Today no such case exists in-tree (verified
//     by the meta-test in is_child_worker_test.go), but a future contributor
//     adding a long doc-comment mentioning the symbols could trip the
//     classifier. AST-based scanning would harden against this.
//
//   - Future-child false negatives. A future child worker that uses neither
//     `ParentMappedState` nor `ShouldStop()` — for example, one that reads
//     only `snap.Desired.IsShutdownRequested()` from the nested form — would
//     be misclassified as a non-child by this heuristic. The
//     ValidateChildWorkersUseShouldStop rule would then silently skip it.
//     Mitigation candidate (deferred to P2.x): scan for the
//     `SetParentMappedState` collector-contract method, which any
//     parent-aware child must declare and which cannot be satisfied via the
//     nested form alone.
func isChildWorker(workerDir string) bool {
	stateDir := filepath.Join(workerDir, "state")

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(stateDir, name))
		if err != nil {
			continue
		}

		if strings.Contains(string(content), "ParentMappedState") ||
			strings.Contains(string(content), "ShouldStop()") {
			return true
		}
	}

	return false
}

// checkFirstConditionalUsesShouldStop parses state file and checks first if condition.
func checkFirstConditionalUsesShouldStop(filename string) bool {
	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return true // Be permissive on parse errors
	}

	usesShouldStop := false

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		if funcDecl.Body == nil || len(funcDecl.Body.List) < 2 {
			return true
		}

		for _, stmt := range funcDecl.Body.List[1:] {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				ast.Inspect(ifStmt.Cond, func(condNode ast.Node) bool {
					if callExpr, ok := condNode.(*ast.CallExpr); ok {
						if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
							if selExpr.Sel.Name == "ShouldStop" {
								usesShouldStop = true

								return false
							}
						}
					}

					return true
				})

				break
			}
		}

		return true
	})

	return usesShouldStop
}

// isConvertSnapshotCall checks if a call expression is ConvertSnapshot or ConvertWorkerSnapshot.
// Both are valid entry-point type conversions: ConvertSnapshot for old-API, ConvertWorkerSnapshot for new-API.
func isConvertSnapshotCall(callExpr *ast.CallExpr) bool {
	validNames := map[string]bool{
		"ConvertSnapshot":       true,
		"ConvertWorkerSnapshot": true,
	}

	if indexExpr, ok := callExpr.Fun.(*ast.IndexListExpr); ok {
		if selExpr, ok := indexExpr.X.(*ast.SelectorExpr); ok {
			return validNames[selExpr.Sel.Name]
		}

		if ident, ok := indexExpr.X.(*ast.Ident); ok {
			return validNames[ident.Name]
		}
	}

	if indexExpr, ok := callExpr.Fun.(*ast.IndexExpr); ok {
		if selExpr, ok := indexExpr.X.(*ast.SelectorExpr); ok {
			return validNames[selExpr.Sel.Name]
		}

		if ident, ok := indexExpr.X.(*ast.Ident); ok {
			return validNames[ident.Name]
		}
	}

	return false
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

	if funcName != "Result" || len(callExpr.Args) < 3 {
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

	if funcName != "Result" || len(callExpr.Args) < 3 {
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

			if funcName != "Result" {
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

// ValidateExhaustiveTransitionCoverage checks that Next() methods end with catch-all return.
func ValidateExhaustiveTransitionCoverage(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkExhaustiveTransitionCoverage(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkExhaustiveTransitionCoverage checks for catch-all return (TryingTo states exempt).
// This now checks for fsmv2.Result(s, fsmv2.SignalNone, nil, "reason") pattern.
func checkExhaustiveTransitionCoverage(filename string) []Violation {
	var violations []Violation

	baseName := filepath.Base(filename)
	if strings.Contains(baseName, "trying_to") || strings.Contains(baseName, "stopping") {
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

		if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
			return true
		}

		lastStmt := funcDecl.Body.List[len(funcDecl.Body.List)-1]

		retStmt, ok := lastStmt.(*ast.ReturnStmt)
		if !ok || len(retStmt.Results) != 1 {
			return true
		}

		// Extract state, signal, action from fsmv2.Result() call
		stateResult, signalResult, actionResult := extractResultArgsWithSignal(retStmt.Results[0])
		if stateResult == nil || signalResult == nil || actionResult == nil {
			return true
		}

		isCatchAll := false

		if ident, ok := stateResult.(*ast.Ident); ok && ident.Name == "s" {
			if selExpr, ok := signalResult.(*ast.SelectorExpr); ok {
				if selExpr.Sel.Name == "SignalNone" {
					if nilIdent, ok := actionResult.(*ast.Ident); ok && nilIdent.Name == "nil" {
						isCatchAll = true
					}
				}
			}
		}

		if !isCatchAll {
			pos := fset.Position(retStmt.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_CATCHALL_RETURN",
				Message: "Next() should end with catch-all: fsmv2.Result(s, fsmv2.SignalNone, nil, reason, nil)",
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
		"StartingBase":       true,
		"RunningHealthyBase": true,
		"RunningDegradedBase": true,
		"StoppingBase":       true,
		"StoppedBase":        true,
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
