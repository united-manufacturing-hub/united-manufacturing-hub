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
