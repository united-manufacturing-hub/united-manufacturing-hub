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

// FindStateFilesIncludingCommunicator returns all state files including communicator.
// Unlike FindStateFiles(), this does NOT exclude communicator.
// It finds files that:
//   - Have prefix "state_" (e.g., state_connected.go)
//   - OR are in a "state" directory with names like "running.go", "stopped.go", etc.
func FindStateFilesIncludingCommunicator(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(filepath.Base(path), "base.go") ||
			!strings.HasSuffix(path, ".go") {
			return nil
		}

		// Include files with "state_" prefix
		if strings.HasPrefix(filepath.Base(path), "state_") {
			files = append(files, path)
			return nil
		}

		// Include files in "state" directories (like helloworld/state/running.go)
		parentDir := filepath.Base(filepath.Dir(path))
		if parentDir == "state" {
			// Only include files that look like state files (not doc.go, etc.)
			baseName := filepath.Base(path)
			if baseName != "doc.go" && !strings.HasPrefix(baseName, "base") {
				files = append(files, path)
			}
		}

		return nil
	})

	return files
}

// ValidateNoObservedStateMutation checks that Next() methods don't mutate snap.Observed.
// This enforces the invariant that state files are pure functions - they should only
// read from the snapshot, never write to it.
func ValidateNoObservedStateMutation(baseDir string) []Violation {
	var violations []Violation

	stateFiles := FindStateFilesIncludingCommunicator(baseDir)

	for _, file := range stateFiles {
		fileViolations := checkNoObservedStateMutation(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkNoObservedStateMutation parses a state file and checks for mutations to snap.Observed.
func checkNoObservedStateMutation(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Find Next() methods and check for mutations
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Next" {
			return true
		}

		// Only check methods (with receiver)
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		// Check for observed state mutations within the function body
		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			assignStmt, ok := bodyNode.(*ast.AssignStmt)
			if !ok {
				return true
			}

			// Check each LHS of the assignment
			for _, lhs := range assignStmt.Lhs {
				if isObservedFieldAssignment(lhs) {
					pos := fset.Position(assignStmt.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "OBSERVED_STATE_MUTATION",
						Message: fmt.Sprintf("Next() method mutates observed state: %s", formatExpr(lhs)),
					})
				}
			}

			return true
		})

		return true
	})

	return violations
}

// isObservedFieldAssignment checks if an expression represents an assignment to snap.Observed.*.
// It detects patterns like:
//   - snap.Observed.State = ...
//   - snap.Observed.SomeField = ...
//   - s.Observed.Field = ... (where s is the snapshot variable)
func isObservedFieldAssignment(expr ast.Expr) bool {
	selExpr, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check for patterns like: something.Observed.Field
	// We need to trace back through the selector chain
	return containsObservedAccess(selExpr)
}

// containsObservedAccess recursively checks if a selector expression
// contains an access to ".Observed." in its chain.
func containsObservedAccess(expr ast.Expr) bool {
	selExpr, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if the current selector is "Observed"
	if selExpr.Sel.Name == "Observed" {
		return true
	}

	// Check if this is X.Observed.Y pattern (the X part after Observed)
	if innerSel, ok := selExpr.X.(*ast.SelectorExpr); ok {
		// If the inner selector is accessing .Observed, this is an assignment to Observed.Field
		if innerSel.Sel.Name == "Observed" {
			return true
		}

		// Recurse deeper
		return containsObservedAccess(innerSel)
	}

	return false
}

// formatExpr returns a string representation of an expression for error messages.
func formatExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return formatExpr(e.X) + "." + e.Sel.Name
	default:
		return "<expr>"
	}
}
