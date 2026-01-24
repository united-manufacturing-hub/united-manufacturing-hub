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
	"os"
	"path/filepath"
	"strings"
)

// ValidateNoDirectDocumentManipulation checks that supervisor code uses typed interfaces
// (like ShutdownRequestable) rather than direct persistence.Document manipulation.
func ValidateNoDirectDocumentManipulation(baseDir string) []Violation {
	var violations []Violation

	supervisorDir := filepath.Join(baseDir, "supervisor")

	entries, err := os.ReadDir(supervisorDir)
	if err != nil {
		return violations
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(supervisorDir, entry.Name())
		fileViolations := checkDocumentManipulation(filePath)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDocumentManipulation scans a file for direct persistence.Document manipulation.
func checkDocumentManipulation(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		typeAssert, ok := n.(*ast.TypeAssertExpr)
		if ok {
			if selExpr, ok := typeAssert.Type.(*ast.SelectorExpr); ok {
				if ident, ok := selExpr.X.(*ast.Ident); ok {
					if ident.Name == "persistence" && selExpr.Sel.Name == "Document" {
						pos := fset.Position(typeAssert.Pos())
						violations = append(violations, Violation{
							File:    filename,
							Line:    pos.Line,
							Type:    "DIRECT_DOCUMENT_ASSERTION",
							Message: "Type assertion to persistence.Document bypasses type safety - use typed interfaces instead",
						})
					}
				}
			}
		}

		return true
	})

	return violations
}
