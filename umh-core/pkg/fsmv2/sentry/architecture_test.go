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

package sentry_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
)

// Violation represents an architectural violation found in code.
type Violation struct {
	File    string
	Line    int
	Type    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", v.File, v.Line, v.Type, v.Message)
}

var _ = Describe("FSMv2 Sentry Architecture", func() {

	Context("Logging Architecture", func() {

		Describe("No direct zap usage outside allowed files", func() {
			It("should not import zap outside of logger_impl, sentry, and test_logger", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateNoDirectZapUsage(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("Direct Zap Import Violations", violations,
						`Production code must use the FSMLogger interface (pkg/fsmv2/deps)
instead of importing zap directly. Direct zap imports are only allowed in:

  - deps/logger_impl.go (the FSMLogger implementation)
  - /sentry/ files (the Sentry hook operates at zap level)
  - integration/test_logger.go (test infrastructure)

CORRECT:
  import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
  logger.Info("something happened", deps.String("key", "value"))

WRONG:
  import "go.uber.org/zap"
  logger.Info("something happened", zap.String("key", "value"))`)

					Fail(message)
				}
			})
		})

		Describe("No err.Error() in logging calls (Invariant: Preserve Error Chain)", func() {
			It("should not call err.Error() in logging statements", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateNoErrorDotError(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("err.Error() Violations", violations,
						`Logging statements must NOT call .Error() on error variables as this
loses the error chain and Sentry's ability to group related errors.

CORRECT:
  logger.SentryError(feature, hierarchyPath, err, "operation_failed")
  logger.SentryWarn(feature, hierarchyPath, "problem detected", deps.Err(err))

WRONG:
  logger.SentryError(feature, hierarchyPath, err, "failed", deps.String("detail", err.Error()))
  logger.Info("failed", deps.String("error", someErr.Error()))

The error chain is important for debugging and Sentry grouping. Always pass
the error object directly, never convert it to a string with .Error().`)

					Fail(message)
				}
			})
		})
	})
})

// getFsmv2Dir returns the path to the fsmv2 package directory.
func getFsmv2Dir() string {
	_, filename, _, _ := runtime.Caller(0)
	// Navigate from sentry/ up to fsmv2/
	return filepath.Dir(filepath.Dir(filename))
}

// getCseDir returns the path to the cse package directory.
func getCseDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// Navigate from fsmv2/sentry/ up to pkg/, then into cse/
	pkgDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))

	return filepath.Join(pkgDir, "cse")
}

// getPersistenceDir returns the path to the persistence package directory.
func getPersistenceDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// Navigate from fsmv2/sentry/ up to pkg/, then into persistence/
	pkgDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))

	return filepath.Join(pkgDir, "persistence")
}

// getAllDirs returns all directories that should be scanned for architecture tests.
func getAllDirs() []string {
	return []string{
		getFsmv2Dir(),
		getCseDir(),
		getPersistenceDir(),
	}
}

// validateNoDirectZapUsage scans all non-test Go files for direct imports of
// "go.uber.org/zap" or "go.uber.org/zap/zapcore" outside of allowed files.
func validateNoDirectZapUsage(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		if isZapAllowedFile(path) {
			return nil
		}

		fileViolations := checkForZapImports(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// isZapAllowedFile returns true if the given file is allowed to import zap directly.
func isZapAllowedFile(path string) bool {
	if strings.HasSuffix(path, string(filepath.Separator)+"logger_impl.go") ||
		strings.HasSuffix(path, "/logger_impl.go") {
		dir := filepath.Base(filepath.Dir(path))
		if dir == "deps" {
			return true
		}
	}

	if strings.Contains(path, string(filepath.Separator)+"sentry"+string(filepath.Separator)) ||
		strings.Contains(path, "/sentry/") {
		return true
	}

	if strings.HasSuffix(path, string(filepath.Separator)+"test_logger.go") ||
		strings.HasSuffix(path, "/test_logger.go") {
		dir := filepath.Base(filepath.Dir(path))
		if dir == "integration" {
			return true
		}
	}

	// CLI entry points construct the zap logger before wrapping it in FSMLogger.
	if strings.Contains(path, string(filepath.Separator)+"cmd"+string(filepath.Separator)) ||
		strings.Contains(path, "/cmd/") {
		return true
	}

	return false
}

// checkForZapImports parses a Go file and checks if it imports zap or zap/zapcore.
func checkForZapImports(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ImportsOnly)
	if err != nil {
		return violations
	}

	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath == "go.uber.org/zap" || importPath == "go.uber.org/zap/zapcore" {
			pos := fset.Position(imp.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "DIRECT_ZAP_IMPORT",
				Message: fmt.Sprintf("Direct import of %q is not allowed; use the FSMLogger interface from pkg/fsmv2/deps instead", importPath),
			})
		}
	}

	return violations
}

// validateNoErrorDotError scans all non-test Go files for .Error() calls on
// variables inside logging method calls.
func validateNoErrorDotError(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		if strings.Contains(path, "internal/validator") || strings.Contains(path, "/sentry/") {
			return nil
		}

		fileViolations := checkForErrorDotError(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// checkForErrorDotError parses a Go file and looks for .Error() calls on
// variables inside logging method calls (SentryError, SentryWarn, Info, Debug,
// or any method on a receiver whose name contains "log").
func checkForErrorDotError(filename string) []Violation {
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

		if !isLoggingMethod(selExpr) {
			return true
		}

		if containsErrorDotError(callExpr) {
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "ERROR_DOT_ERROR",
				Message: fmt.Sprintf("Do not call .Error() on error variables in %s() - pass the error object directly instead", methodName),
			})
		}

		return true
	})

	return violations
}

// isLoggingMethod returns true if the selector expression looks like a logging call.
// It matches:
//   - Known FSMLogger methods: SentryError, SentryWarn, Info, Debug
//   - Any method on a receiver whose name contains "log" (case-insensitive)
func isLoggingMethod(sel *ast.SelectorExpr) bool {
	methodName := sel.Sel.Name

	knownLogMethods := map[string]bool{
		"SentryError": true,
		"SentryWarn":  true,
		"Info":        true,
		"Debug":       true,
	}

	receiverName := ""

	switch x := sel.X.(type) {
	case *ast.Ident:
		receiverName = x.Name
	case *ast.SelectorExpr:
		receiverName = x.Sel.Name
	}

	receiverLooksLikeLogger := strings.Contains(strings.ToLower(receiverName), "log") ||
		receiverName == "logger" || receiverName == "Logger"

	return knownLogMethods[methodName] && receiverLooksLikeLogger
}

// containsErrorDotError recursively checks if any argument in a call expression
// contains a method call to .Error() (i.e., someVar.Error()).
func containsErrorDotError(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if hasErrorMethodCall(arg) {
			return true
		}
	}

	return false
}

// hasErrorMethodCall recursively inspects an AST node for calls to .Error()
// on any expression (e.g., err.Error(), someErr.Error()).
func hasErrorMethodCall(node ast.Node) bool {
	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}

		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if selExpr.Sel.Name == "Error" && len(callExpr.Args) == 0 {
			found = true
			return false
		}

		return true
	})

	return found
}

// formatViolations formats violations into a readable string with context.
func formatViolations(title string, violations []Violation, guidance string) string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString("================================================================================\n")
	sb.WriteString(fmt.Sprintf("  %s\n", title))
	sb.WriteString("================================================================================\n\n")

	sb.WriteString("WHY THIS MATTERS:\n")
	sb.WriteString(guidance)
	sb.WriteString("\n\n")

	sb.WriteString("VIOLATIONS FOUND:\n")
	sb.WriteString("--------------------------------------------------------------------------------\n")

	for i, v := range violations {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, v))
	}

	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("\nTotal violations: %d\n\n", len(violations)))

	return sb.String()
}
