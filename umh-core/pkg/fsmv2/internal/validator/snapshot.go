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

	desiredStateTypes := make(map[string]desiredStateInfo)

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || !strings.Contains(typeSpec.Name.Name, "DesiredState") {
			return true
		}

		info := desiredStateInfo{pos: typeSpec.Pos()}

		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
			for _, field := range structType.Fields.List {
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

	typesWithMethod := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "IsShutdownRequested" {
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

		if typeName != "" {
			typesWithMethod[typeName] = true
		}

		return true
	})

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

// ValidateStateFieldExists checks that DesiredState and ObservedState have a State string field.
func ValidateStateFieldExists(baseDir string) []Violation {
	var violations []Violation

	snapshotFiles := FindSnapshotFiles(baseDir)

	for _, file := range snapshotFiles {
		fileViolations := checkStateFieldExists(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkStateFieldExists parses a snapshot file and checks for State string field.
// DesiredState inherits State from BaseDesiredState; ObservedState needs explicit field.
func checkStateFieldExists(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

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

		hasStateField := false
		embedsBaseDesiredState := false

		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == "State" {
					if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "string" {
						hasStateField = true

						break
					}
				}
			}

			if len(field.Names) == 0 {
				embedName := getEmbeddedTypeName(field.Type)
				if strings.Contains(embedName, "BaseDesiredState") {
					embedsBaseDesiredState = true
				}
			}
		}

		if isDesiredState && embedsBaseDesiredState {
			hasStateField = true
		}

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

// checkDesiredStateValues checks that DesiredState.State is only assigned "stopped" or "running".
func checkDesiredStateValues(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	desiredStateTypes := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if ok && strings.Contains(typeSpec.Name.Name, "DesiredState") {
			desiredStateTypes[typeSpec.Name.Name] = true
		}

		return true
	})

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

