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
// Pattern: ONE type assertion at the start of Next(), zero elsewhere.
// Rationale: Go doesn't support covariance, so State[any, any] is required for Worker interface.
// States must accept `any` and type-assert to concrete type once at entry point.
func ValidateNextMethodTypeAssertions(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkSingleEntryPointPattern(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkSingleEntryPointPattern parses a state file and looks for type assertions in Next().
// Valid patterns:
// 1. A type assertion at the first statement: snap := snapAny.(SomeType).
// 2. A ConvertSnapshot call at the first statement: snap := helpers.ConvertSnapshot[...](snapAny).
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

		// Count type assertions and ConvertSnapshot calls, check their location
		var (
			typeAssertions       []int
			convertSnapshotCalls []int
			firstStatementLine   int
		)

		// Get line number of first statement
		if funcDecl.Body != nil && len(funcDecl.Body.List) > 0 {
			firstStatementLine = fset.Position(funcDecl.Body.List[0].Pos()).Line
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			// Check for type assertions
			if typeAssert, ok := bodyNode.(*ast.TypeAssertExpr); ok {
				pos := fset.Position(typeAssert.Pos())
				typeAssertions = append(typeAssertions, pos.Line)

				return true
			}

			// Check for ConvertSnapshot calls
			if callExpr, ok := bodyNode.(*ast.CallExpr); ok {
				if indexExpr, ok := callExpr.Fun.(*ast.IndexListExpr); ok {
					if selExpr, ok := indexExpr.X.(*ast.SelectorExpr); ok {
						if selExpr.Sel.Name == "ConvertSnapshot" {
							pos := fset.Position(callExpr.Pos())
							convertSnapshotCalls = append(convertSnapshotCalls, pos.Line)
						}
					}
				}
			}

			return true
		})

		// Valid patterns:
		// 1. Exactly 1 type assertion at first statement, no ConvertSnapshot calls
		// 2. Exactly 1 ConvertSnapshot call at first statement, no type assertions
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
			// Find the second entry point for error reporting
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
			// Exactly one entry point - check if it's at the first statement
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

		// Skip if body is too short (e.g., stopped state just returns itself)
		if funcDecl.Body == nil || len(funcDecl.Body.List) < 2 {
			return true
		}

		// Find first if statement (after type assertion)
		var firstIfStmt *ast.IfStmt
		for _, stmt := range funcDecl.Body.List[1:] { // Skip first statement (type assertion)
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				firstIfStmt = ifStmt

				break
			}
		}

		if firstIfStmt == nil {
			// No if statements after type assertion - might be a simple state
			return true
		}

		// Check if the first if statement is checking IsShutdownRequested or IsStopRequired
		// (IsStopRequired is used by child workers and combines IsShutdownRequested + !ShouldBeRunning)
		isShutdownCheck := false

		ast.Inspect(firstIfStmt.Cond, func(condNode ast.Node) bool {
			if callExpr, ok := condNode.(*ast.CallExpr); ok {
				if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					if selExpr.Sel.Name == "IsShutdownRequested" || selExpr.Sel.Name == "IsStopRequired" {
						isShutdownCheck = true

						return false
					}
				}
			}

			return true
		})

		if !isShutdownCheck {
			// Check if the file is a "stopped" or "trying_to_stop" state - these don't need shutdown check
			baseName := filepath.Base(filename)
			if strings.Contains(baseName, "stopped") || strings.Contains(baseName, "trying_to_stop") {
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

// ValidateChildWorkersIsStopRequired checks that child workers use IsStopRequired() not just IsShutdownRequested().
// Child workers are detected by presence of IsStopRequired() method in their snapshot package.
func ValidateChildWorkersIsStopRequired(baseDir string) []Violation {
	var violations []Violation

	// Find all worker directories
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

		// Check subdirectories (e.g., example/examplechild)
		subEntries, err := os.ReadDir(workerDir)
		if err != nil {
			continue
		}

		for _, subEntry := range subEntries {
			if !subEntry.IsDir() {
				continue
			}

			subWorkerDir := filepath.Join(workerDir, subEntry.Name())
			violations = append(violations, checkChildWorkerIsStopRequired(subWorkerDir)...)
		}

		// Also check direct worker (if not nested)
		violations = append(violations, checkChildWorkerIsStopRequired(workerDir)...)
	}

	return violations
}

// checkChildWorkerIsStopRequired checks a single worker directory.
func checkChildWorkerIsStopRequired(workerDir string) []Violation {
	var violations []Violation

	// Check if this worker has IsStopRequired() in snapshot (marks it as child)
	if !isChildWorker(workerDir) {
		return violations
	}

	// Find state files in this worker
	stateDir := filepath.Join(workerDir, "state")

	stateFiles, err := filepath.Glob(filepath.Join(stateDir, "state_*.go"))
	if err != nil {
		return violations
	}

	for _, stateFile := range stateFiles {
		// Skip test files and stopped/trying_to_stop states
		baseName := filepath.Base(stateFile)
		if strings.HasSuffix(baseName, "_test.go") {
			continue
		}

		if strings.Contains(baseName, "stopped") || strings.Contains(baseName, "trying_to_stop") {
			continue
		}

		// Check if first conditional uses IsStopRequired()
		if !checkFirstConditionalUsesIsStopRequired(stateFile) {
			violations = append(violations, Violation{
				File:    stateFile,
				Type:    "CHILD_MUST_USE_IS_STOP_REQUIRED",
				Message: "Child worker state uses IsShutdownRequested() instead of IsStopRequired()",
			})
		}
	}

	return violations
}

// isChildWorker checks if a worker has IsStopRequired() method in its snapshot.
func isChildWorker(workerDir string) bool {
	snapshotFile := filepath.Join(workerDir, "snapshot", "snapshot.go")

	content, err := os.ReadFile(snapshotFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "IsStopRequired()")
}

// checkFirstConditionalUsesIsStopRequired parses state file and checks first if condition.
func checkFirstConditionalUsesIsStopRequired(filename string) bool {
	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return true // Be permissive on parse errors
	}

	usesIsStopRequired := false

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		if funcDecl.Body == nil || len(funcDecl.Body.List) < 2 {
			return true
		}

		// Find first if statement
		for _, stmt := range funcDecl.Body.List[1:] {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				// Check if condition contains IsStopRequired
				ast.Inspect(ifStmt.Cond, func(condNode ast.Node) bool {
					if callExpr, ok := condNode.(*ast.CallExpr); ok {
						if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
							if selExpr.Sel.Name == "IsStopRequired" {
								usesIsStopRequired = true

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

	return usesIsStopRequired
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
func checkStateXORAction(filename string) []Violation {
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

		// Find all return statements
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 3 {
				return true
			}

			// Results are: (State, Signal, Action)
			stateResult := retStmt.Results[0]
			actionResult := retStmt.Results[2]

			// Check if state is changing (not returning 's' or receiver)
			stateIsChanging := false

			if unaryExpr, ok := stateResult.(*ast.UnaryExpr); ok {
				// &SomeState{} - check if it's not the current state
				if compLit, ok := unaryExpr.X.(*ast.CompositeLit); ok {
					if ident, ok := compLit.Type.(*ast.Ident); ok {
						// Get the receiver type name to compare
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

			// Check if action is non-nil
			actionIsNonNil := false

			if _, ok := actionResult.(*ast.Ident); !ok {
				// Not 'nil', could be &SomeAction{}
				if unaryExpr, ok := actionResult.(*ast.UnaryExpr); ok {
					if _, ok := unaryExpr.X.(*ast.CompositeLit); ok {
						actionIsNonNil = true
					}
				}
			} else {
				// Check if it's actually nil
				if ident, ok := actionResult.(*ast.Ident); ok && ident.Name != "nil" {
					actionIsNonNil = true
				}
			}

			// Violation: both state change AND action
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

// ValidateStateStringAndReason checks that all state types have String() and Reason() methods.
func ValidateStateStringAndReason(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFiles(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkStateStringAndReason(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStateStringAndReason parses a state file and checks for String() and Reason() methods.
func checkStateStringAndReason(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Collect all state types (structs ending with "State")
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

	// Collect all types that have String() method
	typesWithString := make(map[string]bool)
	// Collect all types that have Reason() method
	typesWithReason := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		// Get receiver type name
		var typeName string

		switch recvType := funcDecl.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if ident, ok := recvType.X.(*ast.Ident); ok {
				typeName = ident.Name
			}
		case *ast.Ident:
			typeName = recvType.Name
		}

		if typeName != "" {
			if funcDecl.Name.Name == "String" {
				typesWithString[typeName] = true
			}

			if funcDecl.Name.Name == "Reason" {
				typesWithReason[typeName] = true
			}
		}

		return true
	})

	// Check for violations
	for typeName, pos := range stateTypes {
		if !typesWithString[typeName] {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(pos).Line,
				Type:    "STATE_MISSING_STRING_METHOD",
				Message: fmt.Sprintf("State %s missing String() string method", typeName),
			})
		}

		if !typesWithReason[typeName] {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(pos).Line,
				Type:    "STATE_MISSING_STRING_METHOD",
				Message: fmt.Sprintf("State %s missing Reason() string method", typeName),
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

// checkNoNilStateReturns parses a state file and checks return statements in Next().
func checkNoNilStateReturns(filename string) []Violation {
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

		// Find all return statements
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) < 1 {
				return true
			}

			// Check if first result (state) is nil
			stateResult := retStmt.Results[0]
			if ident, ok := stateResult.(*ast.Ident); ok {
				if ident.Name == "nil" {
					pos := fset.Position(retStmt.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "NIL_STATE_RETURN",
						Message: "Next() returns nil as state (should return valid state)",
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

// checkSignalStateMutualExclusion parses a state file and checks return statements in Next().
func checkSignalStateMutualExclusion(filename string) []Violation {
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

		// Find all return statements with 3 results
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 3 {
				return true
			}

			// Results are: (State, Signal, Action)
			stateResult := retStmt.Results[0]
			signalResult := retStmt.Results[1]

			// Check if signal is not SignalNone
			signalIsNone := false

			if selExpr, ok := signalResult.(*ast.SelectorExpr); ok {
				if selExpr.Sel.Name == "SignalNone" {
					signalIsNone = true
				}
			}

			// If signal is not SignalNone, state must be 's' (receiver)
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

// checkTryingToStatesReturnActions parses a state file and checks if TryingTo states return actions.
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
			if !ok || len(retStmt.Results) != 3 {
				return true
			}

			actionResult := retStmt.Results[2]
			if _, ok := actionResult.(*ast.Ident); !ok {
				if unaryExpr, ok := actionResult.(*ast.UnaryExpr); ok {
					if _, ok := unaryExpr.X.(*ast.CompositeLit); ok {
						hasActionReturn = true

						return false
					}
				}
			} else {
				if ident, ok := actionResult.(*ast.Ident); ok && ident.Name != "nil" {
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

// checkExhaustiveTransitionCoverage parses a state file and checks for catch-all return.
// TryingTo states are exempt because they must end with action returns (see ValidateTryingToStatesReturnActions).
func checkExhaustiveTransitionCoverage(filename string) []Violation {
	var violations []Violation

	// Skip TryingTo states - they have their own validation requiring action returns.
	// The two patterns conflict: TryingTo must return actions, passive states must end with nil action.
	baseName := filepath.Base(filename)
	if strings.Contains(baseName, "trying_to") {
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
		if !ok || len(retStmt.Results) != 3 {
			return true
		}

		stateResult := retStmt.Results[0]
		signalResult := retStmt.Results[1]
		actionResult := retStmt.Results[2]

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
				Message: "Next() should end with catch-all: return s, SignalNone, nil",
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

// checkBaseStateEmbedding parses a state file and checks for Base*State embedding.
func checkBaseStateEmbedding(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
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
				if ident, ok := field.Type.(*ast.Ident); ok {
					if strings.HasPrefix(ident.Name, "Base") && strings.HasSuffix(ident.Name, "State") {
						hasBaseState = true

						break
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

// GetStateFilePath converts a type name to a state file path.
// For example, "TryingToConnectState" -> "state_trying_to_connect.go".
func GetStateFilePath(fsmv2Dir, typeName string) string {
	fileName := strings.ToLower(strings.ReplaceAll(typeName, "State", ""))
	fileName = "state_" + strings.ToLower(strings.ReplaceAll(fileName, "TryingTo", "trying_to_"))
	fileName = strings.ReplaceAll(fileName, "__", "_")

	return filepath.Join(fsmv2Dir, "workers", "example", fileName+".go")
}
