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
	"go/token"
	"strconv"
	"strings"
)

// Transition alias matcher.
//
// Scans worker state files for call expressions of the form
// `fsmv2.Result[T1, T2](...)` and `fsmv2.WrapAction[T](...)`, which PR2 C7
// migrates mechanically to `fsmv2.Transition(...)` and framework-wrapped
// actions. PR1 C2 adds the matcher as a standalone primitive; architecture_test
// wires it up against workers/** in PR2 C7. Keeping the matcher separate lets
// us land the detection logic (and its unit tests) without perturbing the
// existing architecture test dispatch.

// fsmv2ImportPathSuffix identifies the fsmv2 package import regardless of
// module path, so the matcher works in-tree and from downstream forks.
const fsmv2ImportPathSuffix = "/pkg/fsmv2"

// legacyResultSymbols enumerates the unqualified names that, when called with
// explicit type parameters via a SelectorExpr on the fsmv2 alias, constitute a
// legacy form to be migrated.
var legacyResultSymbols = map[string]struct{}{
	"Result":     {},
	"WrapAction": {},
}

// LegacyResultCall describes a single call expression that should migrate to
// `fsmv2.Transition` (for Result) or a framework-wrapped action helper (for
// WrapAction). Pos is the file:line:column of the call expression; Symbol is
// rendered using the alias actually used in the source file (e.g. "fsmv2.Result"
// or "v2.Result").
type LegacyResultCall struct {
	Pos    token.Position
	Symbol string
}

// FindLegacyResultCalls scans the parsed Go AST of a single file and returns
// token positions of every call expression of the form `fsmv2.Result[...](...)`
// or `fsmv2.WrapAction[...](...)`. These are the forms that PR2 C7 migrates to
// `fsmv2.Transition(...)` / framework-wrapped actions.
//
// The returned slice is empty if the file has no violations. The package alias
// is resolved from the file's import list: if the file imports the fsmv2
// package under a custom alias, that alias is what the matcher looks for.
// Files that do not import fsmv2 return nil.
//
// Only the explicit-generic form (IndexExpr / IndexListExpr) is flagged; plain
// calls like `fsmv2.Result(...)` (type-parameter inference, which should not
// occur in practice) are intentionally left alone, as are calls to
// `fsmv2.Transition(...)` (the non-generic alias that this matcher exists to
// drive workers toward).
func FindLegacyResultCalls(file *ast.File, fset *token.FileSet) []LegacyResultCall {
	if file == nil || fset == nil {
		return nil
	}

	alias, ok := resolveFsmv2Alias(file)
	if !ok {
		return nil
	}

	var hits []LegacyResultCall

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		symbol, matched := matchLegacySelector(call.Fun, alias)
		if !matched {
			return true
		}

		hits = append(hits, LegacyResultCall{
			Pos:    fset.Position(call.Pos()),
			Symbol: symbol,
		})

		return true
	})

	return hits
}

// resolveFsmv2Alias returns the identifier used to reference the fsmv2 package
// in this file. The second return value is false when the file does not import
// fsmv2 at all. Explicit aliases (e.g. `v2 "…/pkg/fsmv2"`) take precedence over
// the default last-path-segment alias.
func resolveFsmv2Alias(file *ast.File) (string, bool) {
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}

		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}

		if !isFsmv2ImportPath(path) {
			continue
		}

		if imp.Name != nil && imp.Name.Name != "" && imp.Name.Name != "_" && imp.Name.Name != "." {
			return imp.Name.Name, true
		}

		return lastPathSegment(path), true
	}

	return "", false
}

// isFsmv2ImportPath reports whether the given import path refers to the
// fsmv2 package. The comparison uses a path-component suffix match so forks
// under different module roots still validate.
func isFsmv2ImportPath(path string) bool {
	return path == "pkg/fsmv2" || strings.HasSuffix(path, fsmv2ImportPathSuffix)
}

// lastPathSegment returns the final path component of an import path, which is
// Go's default identifier for an unaliased import.
func lastPathSegment(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}

	return path
}

// matchLegacySelector inspects a CallExpr.Fun expression and reports whether
// it is `<alias>.Result[...]` or `<alias>.WrapAction[...]`. The rendered symbol
// uses the caller-supplied alias so error messages reflect the source verbatim.
func matchLegacySelector(fun ast.Expr, alias string) (string, bool) {
	var inner ast.Expr

	switch e := fun.(type) {
	case *ast.IndexExpr:
		inner = e.X
	case *ast.IndexListExpr:
		inner = e.X
	default:
		return "", false
	}

	sel, ok := inner.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}

	if _, legacy := legacyResultSymbols[sel.Sel.Name]; !legacy {
		return "", false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}

	if ident.Name != alias {
		return "", false
	}

	return fmt.Sprintf("%s.%s", alias, sel.Sel.Name), true
}
