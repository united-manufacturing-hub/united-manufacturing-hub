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

// ValidateObservedStateTimestamp checks that ObservedState structs have CollectedAt field.
func ValidateObservedStateTimestamp(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkObservedStateTimestamp(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkObservedStateTimestamp parses a snapshot file and checks for CollectedAt field.
func checkObservedStateTimestamp(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for structs with "ObservedState" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "ObservedState") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Check for CollectedAt field
		hasCollectedAt := false

		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == "CollectedAt" {
					hasCollectedAt = true

					break
				}
			}
		}

		if !hasCollectedAt {
			pos := fset.Position(typeSpec.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_COLLECTED_AT",
				Message: fmt.Sprintf("ObservedState %s missing CollectedAt time.Time field", typeSpec.Name.Name),
			})
		}

		return true
	})

	return violations
}

// ValidateDesiredStateShutdownMethod checks that DesiredState types implement IsShutdownRequested.
func ValidateDesiredStateShutdownMethod(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkDesiredStateShutdownMethod(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// desiredStateInfo holds information about a DesiredState type for validation.
type desiredStateInfo struct {
	pos               token.Pos
	embedsBaseDesired bool
}

// checkDesiredStateShutdownMethod parses a snapshot file and checks for IsShutdownRequested method.
// Types that embed config.BaseDesiredState are considered valid since they inherit the method.
func checkDesiredStateShutdownMethod(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Collect all DesiredState types with embedding info
	desiredStateTypes := make(map[string]desiredStateInfo)

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "DesiredState") {
			return true
		}

		info := desiredStateInfo{pos: typeSpec.Pos()}

		// Check if struct embeds BaseDesiredState (directly or via config.BaseDesiredState)
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
			for _, field := range structType.Fields.List {
				// Anonymous/embedded fields have no names
				if len(field.Names) == 0 {
					embedName := getEmbeddedTypeName(field.Type)
					if strings.Contains(embedName, "BaseDesiredState") {
						info.embedsBaseDesired = true

						break
					}
				}
			}
		}

		desiredStateTypes[typeSpec.Name.Name] = info
		return true
	})

	// Collect all types that have IsShutdownRequested method declared explicitly
	typesWithMethod := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "IsShutdownRequested" {
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
			typesWithMethod[typeName] = true
		}

		return true
	})

	// Check for violations - only if NEITHER embedded NOR explicit method
	for typeName, info := range desiredStateTypes {
		if !typesWithMethod[typeName] && !info.embedsBaseDesired {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(info.pos).Line,
				Type:    "MISSING_IS_SHUTDOWN_REQUESTED",
				Message: fmt.Sprintf("DesiredState %s missing IsShutdownRequested() - embed config.BaseDesiredState or add method", typeName),
			})
		}
	}

	return violations
}

// getEmbeddedTypeName extracts the type name from an embedded field expression.
func getEmbeddedTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		// For qualified names like config.BaseDesiredState
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	}
	return ""
}

// ValidateObservedStateEmbedsDesired checks that ObservedState structs embed their DesiredState with json:",inline" tag.
func ValidateObservedStateEmbedsDesired(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkObservedStateEmbedsDesired(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkObservedStateEmbedsDesired parses a snapshot file and checks that ObservedState structs
// embed their corresponding DesiredState type anonymously with json:",inline" tag.
func checkObservedStateEmbedsDesired(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for structs with "ObservedState" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "ObservedState") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Check for embedded DesiredState field
		hasEmbeddedDesired := false
		hasInlineTag := false

		var embeddedFieldPos token.Pos

		for _, field := range structType.Fields.List {
			// Anonymous/embedded fields have no names
			if len(field.Names) == 0 {
				embedName := getEmbeddedTypeName(field.Type)
				if strings.Contains(embedName, "DesiredState") {
					hasEmbeddedDesired = true
					embeddedFieldPos = field.Pos()

					// Check for json:",inline" tag
					if field.Tag != nil {
						tagValue := field.Tag.Value
						// Tag value includes quotes, e.g., `json:"field_name"`
						if strings.Contains(tagValue, `json:`) && strings.Contains(tagValue, `,inline`) {
							hasInlineTag = true
						}
					}

					break
				}
			}

			// Also check for named DesiredState field (which is WRONG)
			for _, name := range field.Names {
				if strings.Contains(name.Name, "DesiredState") {
					pos := fset.Position(field.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "OBSERVED_STATE_NOT_EMBEDDING_DESIRED",
						Message: fmt.Sprintf("ObservedState %s has named field %s - should embed anonymously with json:\",inline\" tag", typeSpec.Name.Name, name.Name),
					})
					return true
				}
			}
		}

		if !hasEmbeddedDesired {
			pos := fset.Position(typeSpec.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "OBSERVED_STATE_NOT_EMBEDDING_DESIRED",
				Message: fmt.Sprintf("ObservedState %s missing embedded DesiredState - should embed anonymously with json:\",inline\" tag", typeSpec.Name.Name),
			})
		} else if !hasInlineTag {
			pos := fset.Position(embeddedFieldPos)
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "OBSERVED_STATE_NOT_EMBEDDING_DESIRED",
				Message: fmt.Sprintf("ObservedState %s embeds DesiredState but missing json:\",inline\" tag", typeSpec.Name.Name),
			})
		}

		return true
	})

	return violations
}

// ValidateStateFieldExists checks that both DesiredState and ObservedState have a State string field.
func ValidateStateFieldExists(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkStateFieldExists(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStateFieldExists parses a snapshot file and checks for State string field in both
// DesiredState and ObservedState structs.
// For DesiredState structs, State is inherited from embedded BaseDesiredState.
// For ObservedState structs, State must be an explicit field (not inherited from DesiredState).
func checkStateFieldExists(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for structs with "DesiredState" or "ObservedState" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		isDesiredState := strings.Contains(typeSpec.Name.Name, "DesiredState")
		isObservedState := strings.Contains(typeSpec.Name.Name, "ObservedState")

		if !isDesiredState && !isObservedState {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Check for State string field (explicit or inherited)
		hasStateField := false
		embedsBaseDesiredState := false

		for _, field := range structType.Fields.List {
			// Check for explicit State field
			for _, name := range field.Names {
				if name.Name == "State" {
					// Verify it's a string type
					if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "string" {
						hasStateField = true

						break
					}
				}
			}

			// Check for embedded BaseDesiredState (anonymous field)
			if len(field.Names) == 0 {
				embedName := getEmbeddedTypeName(field.Type)
				if strings.Contains(embedName, "BaseDesiredState") {
					embedsBaseDesiredState = true
				}
			}
		}

		// For DesiredState structs, State is inherited from BaseDesiredState
		if isDesiredState && embedsBaseDesiredState {
			hasStateField = true
		}

		// For ObservedState, State must be explicit (not inherited from embedded DesiredState)
		// because the ObservedState.State has different semantics (lifecycle state)

		if !hasStateField {
			pos := fset.Position(typeSpec.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_STATE_FIELD",
				Message: typeSpec.Name.Name + " missing State string field",
			})
		}

		return true
	})

	return violations
}

// ValidateDesiredStateValues checks that DesiredState.State field only contains "stopped" or "running".
func ValidateDesiredStateValues(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkDesiredStateValues(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDesiredStateValues parses a snapshot file and checks that DesiredState.State field
// is only assigned "stopped" or "running" values. This is a simpler check that looks for
// string literal assignments to the State field.
func checkDesiredStateValues(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Track DesiredState type names
	desiredStateTypes := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if ok && strings.Contains(typeSpec.Name.Name, "DesiredState") {
			desiredStateTypes[typeSpec.Name.Name] = true
		}
		return true
	})

	// Look for assignments to .State field on DesiredState types
	ast.Inspect(node, func(n ast.Node) bool {
		assignStmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, lhs := range assignStmt.Lhs {
			selectorExpr, ok := lhs.(*ast.SelectorExpr)
			if !ok || selectorExpr.Sel.Name != "State" {
				continue
			}

			// Check if RHS is a string literal
			if i >= len(assignStmt.Rhs) {
				continue
			}

			rhs := assignStmt.Rhs[i]
			if basicLit, ok := rhs.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
				value := strings.Trim(basicLit.Value, `"`)
				if value != "stopped" && value != "running" {
					pos := fset.Position(assignStmt.Pos())
					violations = append(violations, Violation{
						File:    filename,
						Line:    pos.Line,
						Type:    "INVALID_DESIRED_STATE_VALUE",
						Message: fmt.Sprintf("DesiredState.State assigned invalid value %q - should be \"stopped\" or \"running\"", value),
					})
				}
			}
		}

		return true
	})

	return violations
}

// ValidateObservedStateHasSetState checks that ObservedState types have a SetState(string) method.
// This is required for the StateProvider callback mechanism: the Collector calls StateProvider
// to get the current FSM state name, then calls SetState on the observed state to inject it.
// Without this method, the State field in ObservedState would remain empty.
func ValidateObservedStateHasSetState(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkObservedStateHasSetState(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkObservedStateHasSetState parses a snapshot file and checks that ObservedState types
// have a SetState(string) method defined.
func checkObservedStateHasSetState(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Collect all ObservedState type names
	observedStateTypes := make(map[string]token.Pos)
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "ObservedState") {
			return true
		}
		observedStateTypes[typeSpec.Name.Name] = typeSpec.Pos()
		return true
	})

	// Collect all types that have SetState method defined
	typesWithSetState := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "SetState" {
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
			typesWithSetState[typeName] = true
		}

		return true
	})

	// Check for violations: ObservedState types that don't have SetState method
	for typeName, pos := range observedStateTypes {
		if !typesWithSetState[typeName] {
			violations = append(violations, Violation{
				File:    filename,
				Line:    fset.Position(pos).Line,
				Type:    "MISSING_SET_STATE_METHOD",
				Message: fmt.Sprintf("ObservedState %s missing SetState(string) method - required for StateProvider callback", typeName),
			})
		}
	}

	return violations
}

// ValidateDesiredStateHasNoDependencies checks that DesiredState structs do NOT have a Dependencies field.
// This is an architectural invariant: DesiredState is pure configuration that can be serialized.
// Dependencies are runtime interfaces that belong in Worker, not in DesiredState.
// See fsmv2.DesiredState documentation for the complete rationale.
func ValidateDesiredStateHasNoDependencies(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkDesiredStateHasNoDependencies(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// ValidateFolderMatchesWorkerType checks that worker folder names match their derived worker types.
// This ensures consistency between folder names and type names, preventing registration mismatches.
func ValidateFolderMatchesWorkerType(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkFolderMatchesWorkerType(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkFolderMatchesWorkerType parses a snapshot file, derives the worker type from the
// ObservedState type name, and compares it to the folder name.
// The derivation follows the same logic as storage.DeriveWorkerType:
// strip "ObservedState" suffix and lowercase.
func checkFolderMatchesWorkerType(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Get the worker folder name (parent of snapshot/ directory)
	// Path structure: .../workers/example/examplechild/snapshot/snapshot.go
	// We want "examplechild" (the parent of "snapshot")
	dir := filepath.Dir(filename)            // .../snapshot
	workerDir := filepath.Dir(dir)           // .../examplechild
	folderName := filepath.Base(workerDir)   // examplechild

	// Look for structs with "ObservedState" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasSuffix(typeSpec.Name.Name, "ObservedState") {
			return true
		}

		// Derive worker type using same logic as storage.DeriveWorkerType
		typeName := typeSpec.Name.Name
		workerType := strings.TrimSuffix(typeName, "ObservedState")
		workerType = strings.ToLower(workerType)

		// Compare to folder name
		if folderName != workerType {
			pos := fset.Position(typeSpec.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "FOLDER_WORKER_TYPE_MISMATCH",
				Message: fmt.Sprintf("Folder name %q does not match derived worker type %q (from %s). Rename folder to %q to match.", folderName, workerType, typeName, workerType),
			})
		}

		return true
	})

	return violations
}

// checkDesiredStateHasNoDependencies parses a snapshot file and checks that DesiredState structs
// do not have any field with "Dependencies" in the name.
func checkDesiredStateHasNoDependencies(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Look for structs with "DesiredState" in the name
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "DesiredState") {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Check for Dependencies field (any field with "Dependencies" in name)
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if strings.Contains(name.Name, "Dependencies") {
					pos := fset.Position(field.Pos())
					violations = append(violations, Violation{
						File: filename,
						Line: pos.Line,
						Type: "DEPENDENCIES_IN_DESIRED_STATE",
						Message: fmt.Sprintf("DesiredState %s has forbidden field '%s' - "+
							"dependencies belong in Worker, not DesiredState. "+
							"See fsmv2.DesiredState documentation for the architectural invariant.",
							typeSpec.Name.Name, name.Name),
					})
				}
			}
		}

		return true
	})

	return violations
}
