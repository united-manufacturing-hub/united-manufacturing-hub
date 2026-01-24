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
	"regexp"
	"strings"
)

// formatVerbWithVariable represents a format verb that contains dynamic content.
type formatVerbWithVariable struct {
	verb     string
	argIndex int
	argName  string
}

// dynamicFormatVerbs are format verbs that typically contain dynamic content.
// %w is allowed because it's for error wrapping (the wrapped error itself is OK).
// %T is allowed because it's the type name (static for a given code path).
var dynamicFormatVerbs = regexp.MustCompile(`%[+#]?[0-9]*\.?[0-9]*[sdvqxXoObBfFeEgGpUT]`)

// ValidateStaticErrorMessages scans all Go files in the fsmv2 package for
// fmt.Errorf calls that contain dynamic content in the format string.
//
// Rationale: Error messages with dynamic content (worker IDs, timestamps, counts)
// create unique error messages that Sentry cannot group. This causes:
// - Sentry spam (thousands of "unique" issues that are actually the same error)
// - Impossible to track error frequency
// - Alert fatigue for on-call engineers
//
// Static messages like "worker failed to start" group properly.
// Dynamic context should go in Sentry's extra data, not the message.
func ValidateStaticErrorMessages(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		// Skip the validator package itself
		if strings.Contains(path, "internal/validator") {
			return nil
		}

		fileViolations := checkForDynamicErrorMessages(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// checkForDynamicErrorMessages parses a Go file and looks for fmt.Errorf calls
// with dynamic content in format strings.
func checkForDynamicErrorMessages(filename string) []Violation {
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

		// Check for fmt.Errorf
		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkgIdent, ok := selExpr.X.(*ast.Ident)
		if !ok {
			return true
		}

		// Only check fmt.Errorf (not errors.New, not other packages)
		if pkgIdent.Name != "fmt" || selExpr.Sel.Name != "Errorf" {
			return true
		}

		// Need at least a format string
		if len(callExpr.Args) < 1 {
			return true
		}

		// Get the format string
		formatArg, ok := callExpr.Args[0].(*ast.BasicLit)
		if !ok {
			// Format string is not a literal (could be a variable) - that's also a violation
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "DYNAMIC_ERROR_MESSAGE",
				Message: "fmt.Errorf format string is not a literal (cannot verify static content)",
			})

			return true
		}

		if formatArg.Kind != token.STRING {
			return true
		}

		formatString := formatArg.Value

		// Find all format verbs in the string
		verbs := dynamicFormatVerbs.FindAllStringIndex(formatString, -1)
		if len(verbs) == 0 {
			return true // No format verbs, this is static
		}

		// Check each argument after the format string
		// Skip %w verbs as error wrapping is OK
		dynamicVerbsFound := findDynamicVerbs(formatString, callExpr.Args[1:])

		for _, dv := range dynamicVerbsFound {
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File: filename,
				Line: pos.Line,
				Type: "DYNAMIC_ERROR_MESSAGE",
				Message: "Error message contains dynamic content (" + dv.verb +
					" with " + dv.argName + "). Use static messages for Sentry grouping.",
			})
		}

		return true
	})

	return violations
}

// findDynamicVerbs analyzes a format string and its arguments to find verbs
// that would inject dynamic content into the error message.
func findDynamicVerbs(formatString string, args []ast.Expr) []formatVerbWithVariable {
	var result []formatVerbWithVariable

	// Parse the format string to find all verbs
	verbs := parseFormatVerbs(formatString)

	for i, verb := range verbs {
		// %w is always OK (error wrapping)
		if verb == "%w" {
			continue
		}

		// If there's a corresponding argument, check if it's dynamic
		if i < len(args) {
			argName := getExpressionName(args[i])

			// Allow "err" as it's typically wrapped errors
			if argName == "err" && (verb == "%w" || verb == "%v" || verb == "%s") {
				continue
			}

			// Everything else is considered dynamic content
			result = append(result, formatVerbWithVariable{
				verb:     verb,
				argIndex: i,
				argName:  argName,
			})
		}
	}

	return result
}

// parseFormatVerbs extracts format verbs from a format string.
func parseFormatVerbs(formatString string) []string {
	var verbs []string

	// Match all format verbs including %w
	re := regexp.MustCompile(`%[+#]?[0-9]*\.?[0-9]*[sdvqxXoObBfFeEgGpUTw]`)
	matches := re.FindAllString(formatString, -1)

	verbs = append(verbs, matches...)

	return verbs
}

// getExpressionName returns a human-readable name for an expression.
func getExpressionName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return getExpressionName(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		// For function calls like foo.Bar(), return the function name
		return getExpressionName(e.Fun) + "()"
	case *ast.BasicLit:
		// Literal values are static
		return "<literal>"
	case *ast.IndexExpr:
		return getExpressionName(e.X) + "[...]"
	case *ast.StarExpr:
		return "*" + getExpressionName(e.X)
	case *ast.UnaryExpr:
		return getExpressionName(e.X)
	default:
		return "<expression>"
	}
}
