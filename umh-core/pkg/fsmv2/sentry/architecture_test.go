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

	Context("Error Logging Patterns", func() {

		Describe("ErrorFields Usage (Invariant: Sentry Error Grouping)", func() {
			It("should use fsmv2sentry.ErrorFields{...}.ZapFields() for error logging", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateErrorFieldsUsage(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("ErrorFields Usage Violations", violations,
						`Error and warning logs must use fsmv2sentry.ErrorFields{...}.ZapFields()
to ensure proper Sentry error grouping and context capture.

CORRECT:
  logger.Errorw("action_failed",
      fsmv2sentry.ErrorFields{
          Err:     err,
          Context: "connecting to database",
      }.ZapFields()...)

WRONG:
  logger.Errorw("action_failed", "error", err)
  logger.Errorw("action_failed", "error", err.Error())`)

					Fail(message)
				}
			})
		})

		Describe("No err.Error() Calls (Invariant: Preserve Error Chain)", func() {
			It("should not call err.Error() in logging statements", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateNoErrorDotError(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("err.Error() Violations", violations,
						`Logging statements must NOT call err.Error() as this loses the error chain
and Sentry's ability to group related errors.

CORRECT:
  logger.Errorw("operation_failed",
      fsmv2sentry.ErrorFields{Err: err}.ZapFields()...)

WRONG:
  logger.Errorw("operation_failed", "error", err.Error())

The error chain is important for debugging and Sentry grouping. Always pass
the error object directly, never convert it to a string.`)

					Fail(message)
				}
			})
		})

		Describe("Non-Structured Logging (Invariant: Use Structured Logging)", func() {
			It("should not use non-structured error/warn logging methods", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateNoNonStructuredLogging(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("Non-Structured Logging Violations", violations,
						`Error and warning logs must use structured logging methods (Errorw/Warnw)
to ensure proper Sentry integration and consistent log formatting.

CORRECT:
  logger.Errorw("action_failed",
      fsmv2sentry.ErrorFields{Err: err}.ZapFields()...)
  logger.Warnw("action_slow", "duration", elapsed)

WRONG:
  logger.Error("action failed")
  logger.Errorf("action failed: %v", err)
  logger.Warn("action slow")
  logger.Warnf("action slow: %v", elapsed)

Non-structured methods (Error, Errorf, Warn, Warnf) bypass Sentry integration
and make log analysis more difficult. Always use Errorw/Warnw with key-value pairs.`)

					Fail(message)
				}
			})
		})

		Describe("All Errorw Calls Must Use ErrorFields (Invariant: Sentry Integration)", func() {
			It("should use ErrorFields for ALL Errorw calls", func() {
				var violations []Violation
				for _, dir := range getAllDirs() {
					violations = append(violations, validateAllErrorwUseErrorFields(dir)...)
				}

				if len(violations) > 0 {
					message := formatViolations("Errorw Without ErrorFields Violations", violations,
						`ALL Errorw calls must use fsmv2sentry.ErrorFields{...}.ZapFields()
for proper feature routing and Sentry grouping.

CORRECT:
  logger.Errorw("action_failed",
      fsmv2sentry.ErrorFields{
          Err:     err,
          Context: "connecting to database",
      }.ZapFields()...)

  // Even without an error object, use ErrorFields for consistency:
  logger.Errorw("invalid_state",
      fsmv2sentry.ErrorFields{
          Context: "unexpected nil value",
      }.ZapFields()...)

WRONG:
  logger.Errorw("action_failed", "key", value)
  logger.Errorw("action_failed", "error", err)
  logger.Errorw("invalid_state")

ErrorFields ensures consistent Sentry fingerprinting and context capture
for ALL error-level logs, not just those with error objects.`)

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

// validateErrorFieldsUsage scans all Go files in the fsmv2 package for
// logger.Errorw and logger.Warnw calls that do NOT use ErrorFields{...}.ZapFields().
func validateErrorFieldsUsage(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and generated code
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		// Skip the validator and sentry packages themselves
		if strings.Contains(path, "internal/validator") || strings.Contains(path, "/sentry/") {
			return nil
		}

		fileViolations := checkForMissingErrorFields(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// checkForMissingErrorFields parses a Go file and looks for logger.Errorw/Warnw calls
// that have "error" as a field key but don't use ErrorFields{...}.ZapFields().
func checkForMissingErrorFields(filename string) []Violation {
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
		// Only check Errorw and Warnw calls
		if methodName != "Errorw" && methodName != "Warnw" {
			return true
		}

		// Check if the receiver looks like a logger
		receiverName := ""

		switch x := selExpr.X.(type) {
		case *ast.Ident:
			receiverName = x.Name
		case *ast.SelectorExpr:
			receiverName = x.Sel.Name
		}

		if receiverName != "logger" && receiverName != "Logger" &&
			!strings.Contains(strings.ToLower(receiverName), "log") {
			return true
		}

		// Check if this call has an "error" key without using ErrorFields
		if hasRawErrorField(callExpr) && !usesErrorFieldsZapFields(callExpr) {
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "RAW_ERROR_FIELD",
				Message: fmt.Sprintf("Use fsmv2sentry.ErrorFields{...}.ZapFields() instead of raw \"error\" field in %s()", methodName),
			})
		}

		return true
	})

	return violations
}

// hasRawErrorField checks if a logger call has "error" as a literal string key.
func hasRawErrorField(call *ast.CallExpr) bool {
	// Skip the first argument (the message)
	if len(call.Args) < 2 {
		return false
	}

	// Check key-value pairs starting from the second argument
	for i := 1; i < len(call.Args); i++ {
		arg := call.Args[i]

		// Check for string literal "error"
		basicLit, ok := arg.(*ast.BasicLit)
		if ok && basicLit.Kind == token.STRING {
			value := strings.Trim(basicLit.Value, "\"'`")
			if value == "error" {
				return true
			}
		}
	}

	return false
}

// usesErrorFieldsZapFields checks if a logger call uses ErrorFields{...}.ZapFields().
// It recognizes two patterns:
// 1. Direct: logger.Errorw("msg", ErrorFields{}.ZapFields()...)
// 2. With append: logger.Errorw("msg", append(ErrorFields{}.ZapFields(), "key", "value")...)
func usesErrorFieldsZapFields(call *ast.CallExpr) bool {
	// Look for a spread operator (...) argument that is a call to ZapFields()
	for _, arg := range call.Args {
		// Check for spread argument (variadic expansion)
		if _, ok := arg.(*ast.Ellipsis); ok {
			continue
		}

		// Look for CallExpr with ZapFields selector
		if callArg, ok := arg.(*ast.CallExpr); ok {
			if sel, ok := callArg.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "ZapFields" {
					return true
				}
			}

			// Check for append(ErrorFields{}.ZapFields(), ...)... pattern
			if ident, ok := callArg.Fun.(*ast.Ident); ok && ident.Name == "append" {
				// Check the first argument to append for ZapFields
				if len(callArg.Args) > 0 {
					if innerCall, ok := callArg.Args[0].(*ast.CallExpr); ok {
						if sel, ok := innerCall.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "ZapFields" {
								return true
							}
						}
					}
				}
			}
		}
	}

	return false
}

// validateNoErrorDotError scans all Go files for patterns like "error", err.Error()
// in logger calls.
func validateNoErrorDotError(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and generated code
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		// Skip the validator and sentry packages themselves
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

// checkForErrorDotError parses a Go file and looks for patterns like
// "error", someErr.Error() in logger calls.
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
		// Only check Errorw and Warnw calls
		if methodName != "Errorw" && methodName != "Warnw" {
			return true
		}

		// Check if the receiver looks like a logger
		receiverName := ""

		switch x := selExpr.X.(type) {
		case *ast.Ident:
			receiverName = x.Name
		case *ast.SelectorExpr:
			receiverName = x.Sel.Name
		}

		if receiverName != "logger" && receiverName != "Logger" &&
			!strings.Contains(strings.ToLower(receiverName), "log") {
			return true
		}

		// Check for "error", *.Error() pattern
		if hasErrorDotErrorPattern(callExpr) {
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "ERROR_DOT_ERROR",
				Message: fmt.Sprintf("Do not call err.Error() in %s() - use fsmv2sentry.ErrorFields{Err: err} instead", methodName),
			})
		}

		return true
	})

	return violations
}

// hasErrorDotErrorPattern checks if a logger call has the pattern "error", x.Error().
func hasErrorDotErrorPattern(call *ast.CallExpr) bool {
	// Skip the first argument (the message)
	if len(call.Args) < 3 {
		return false
	}

	// Check key-value pairs starting from the second argument
	for i := 1; i < len(call.Args)-1; i++ {
		// Check if current arg is "error" string
		keyArg := call.Args[i]

		basicLit, ok := keyArg.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			continue
		}

		value := strings.Trim(basicLit.Value, "\"'`")
		if value != "error" {
			continue
		}

		// Check if next arg is a call to .Error()
		valueArg := call.Args[i+1]

		callValue, ok := valueArg.(*ast.CallExpr)
		if !ok {
			continue
		}

		selValue, ok := callValue.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if selValue.Sel.Name == "Error" {
			return true
		}
	}

	return false
}

// validateNoNonStructuredLogging scans all Go files in the fsmv2 package for
// non-structured logging calls (Error, Errorf, Warn, Warnf) that should be
// migrated to structured methods (Errorw, Warnw).
func validateNoNonStructuredLogging(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and generated code
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		// Skip the validator and sentry packages themselves
		if strings.Contains(path, "internal/validator") || strings.Contains(path, "/sentry/") {
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

// checkForNonStructuredLogging parses a Go file and looks for logger.Error(),
// logger.Errorf(), logger.Warn(), logger.Warnf() calls.
func checkForNonStructuredLogging(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Non-structured methods that should be migrated
	nonStructuredMethods := map[string]bool{
		"Error":  true,
		"Errorf": true,
		"Warn":   true,
		"Warnf":  true,
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
		// Only check non-structured error/warn methods
		if !nonStructuredMethods[methodName] {
			return true
		}

		// Check if the receiver looks like a logger
		receiverName := ""

		switch x := selExpr.X.(type) {
		case *ast.Ident:
			receiverName = x.Name
		case *ast.SelectorExpr:
			receiverName = x.Sel.Name
		}

		if receiverName != "logger" && receiverName != "Logger" &&
			!strings.Contains(strings.ToLower(receiverName), "log") {
			return true
		}

		pos := fset.Position(callExpr.Pos())
		violations = append(violations, Violation{
			File:    filename,
			Line:    pos.Line,
			Type:    "NON_STRUCTURED_LOGGING",
			Message: fmt.Sprintf("Use %sw() with structured fields instead of %s()", strings.TrimSuffix(methodName, "f"), methodName),
		})

		return true
	})

	return violations
}

// validateAllErrorwUseErrorFields scans all Go files in the fsmv2 package for
// Errorw calls that do NOT use ErrorFields{...}.ZapFields().
func validateAllErrorwUseErrorFields(baseDir string) []Violation {
	var violations []Violation

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files and generated code
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") {
			return nil
		}

		// Skip the validator and sentry packages themselves
		if strings.Contains(path, "internal/validator") || strings.Contains(path, "/sentry/") {
			return nil
		}

		fileViolations := checkAllErrorwUseErrorFields(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		return violations
	}

	return violations
}

// checkAllErrorwUseErrorFields parses a Go file and looks for ALL logger.Errorw
// calls that do NOT use ErrorFields{...}.ZapFields().
func checkAllErrorwUseErrorFields(filename string) []Violation {
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
		// Only check Errorw calls
		if methodName != "Errorw" {
			return true
		}

		// Check if the receiver looks like a logger
		receiverName := ""

		switch x := selExpr.X.(type) {
		case *ast.Ident:
			receiverName = x.Name
		case *ast.SelectorExpr:
			receiverName = x.Sel.Name
		}

		if receiverName != "logger" && receiverName != "Logger" &&
			!strings.Contains(strings.ToLower(receiverName), "log") {
			return true
		}

		// Check if this call uses ErrorFields{...}.ZapFields()
		if !usesErrorFieldsZapFields(callExpr) {
			pos := fset.Position(callExpr.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "ERRORW_WITHOUT_ERROR_FIELDS",
				Message: "All Errorw() calls must use fsmv2sentry.ErrorFields{...}.ZapFields() for Sentry integration",
			})
		}

		return true
	})

	return violations
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
