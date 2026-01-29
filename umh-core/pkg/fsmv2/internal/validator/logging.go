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

// forbiddenLogMethods are format-based log methods that should not be used.
// Structured logging (Warnw, Errorw, Infow, Debugw) should be used instead.
var forbiddenLogMethods = map[string]bool{
	"Warnf":  true,
	"Errorf": true,
	"Infof":  true,
	"Debugf": true,
	"Fatalf": true,
}

// ValidateStructuredLogging scans all Go files in the fsmv2 package for
// format-based log calls (Warnf, Errorf, Infof, Debugf, Fatalf) and reports
// them as violations. Only structured logging (Warnw, Errorw, etc.) should be used.
//
// Rationale: Structured logging with key-value pairs enables:
// - Better log aggregation and querying
// - Consistent field names across the codebase
// - Hierarchical context via the "worker" field.
func ValidateStructuredLogging(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		if strings.Contains(path, "internal/validator") {
			return nil
		}

		fileViolations := checkForNonStructuredLogging(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// checkForNonStructuredLogging parses a Go file and looks for forbidden log method calls.
func checkForNonStructuredLogging(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := selExpr.Sel.Name
		if !forbiddenLogMethods[methodName] {
			return true
		}

		receiverName := ""

		switch x := selExpr.X.(type) {
		case *ast.Ident:
			receiverName = x.Name
		case *ast.SelectorExpr:
			receiverName = x.Sel.Name
		}

		if receiverName == "logger" || receiverName == "Logger" ||
			strings.Contains(receiverName, "log") || strings.Contains(receiverName, "Log") {
			pos := fset.Position(callExpr.Pos())

			structuredMethod := strings.TrimSuffix(methodName, "f") + "w"
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "NON_STRUCTURED_LOGGING",
				Message: "Use structured logging " + structuredMethod + "() instead of " + methodName + "()",
			})
		}

		return true
	})

	return violations
}
