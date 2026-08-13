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

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs pin how main() brings up the FSMv2 runtime: it must be built and
// started whatever the backend credentials say. Only the communicator may be
// gated on credentials. When the runtime is absent, adapter-driven workers are
// never upserted and report Starting forever with no error, no Sentry event and
// no exit, so the failure is invisible in production and cannot be caught by
// watching behaviour.
//
// The invariant is a property of main()'s control flow, and main() cannot be
// called from a test, so it is asserted against the parsed source. renderer_test.go
// reads main.go's source for the same reason; this is the structural form.
//
// The assertions are positional rather than name-matching, which matters:
// requiring the build to be a top-level statement of main() also rejects moving
// it into a helper that is then called behind a credentials check - an ordinary
// extract-function refactor that reproduces the defect exactly. The Run
// receiver is read from the build's own assignment rather than hardcoded, so
// renaming the local cannot quietly disarm anything.
//
// What these specs do NOT cover. They are structural: they prove no credentials
// conditional can skip the bring-up, never that the runtime works. Uncovered,
// and left uncovered on purpose:
//   - A credentials predicate reached through a helper call, a package-level
//     variable or a struct field: only main()'s own locals are tracked.
//   - A switch statement used as the gate instead of an if.
//   - A gate placed below main(), for example an early return inside
//     buildFSMv2Supervisor itself. renderer_test.go covers the renderer half.
//   - A panic on missing credentials: loud, so not this defect.
//   - main.go is read by relative path, so these pass only from the package's
//     own directory, as renderer_test.go already requires.
var _ = Describe("FSMv2 runtime bring-up invariant", func() {
	var (
		fset   *token.FileSet
		mainFn *ast.FuncDecl
	)

	BeforeEach(func() {
		fset = token.NewFileSet()

		file, err := parser.ParseFile(fset, "main.go", nil, 0)
		Expect(err).NotTo(HaveOccurred())

		mainFn = bringUpInvariantFindFunc(file, "main")
		Expect(mainFn).NotTo(BeNil(), "main() not found in main.go")
	})

	It("builds the supervisor as a top-level statement of main()", func() {
		Expect(bringUpInvariantCountCalls(mainFn.Body, bringUpInvariantBuildFn)).To(Equal(1),
			fmt.Sprintf("main() must contain exactly one call to %s", bringUpInvariantBuildFn))

		assign, _, ok := bringUpInvariantTopLevelBuild(mainFn)
		Expect(ok).To(BeTrue(), fmt.Sprintf(
			"%s must be called from a top-level statement of main(), not nested in a conditional, loop or helper: "+
				"anything that can skip that statement can skip the whole FSMv2 runtime", bringUpInvariantBuildFn))
		Expect(assign).NotTo(BeNil())
	})

	It("never lets a credentials conditional before the build end main()", func() {
		assign, _, ok := bringUpInvariantTopLevelBuild(mainFn)
		Expect(ok).To(BeTrue(), "cannot judge early exits without locating the build; see the previous spec")

		var terminated []string

		bringUpInvariantEachCredentialsBranch(fset, mainFn, func(branch ast.Node, line int, ifPos token.Pos) {
			if ifPos > assign.Pos() {
				return
			}

			if bringUpInvariantEndsFunc(branch) {
				terminated = append(terminated,
					fmt.Sprintf("main() can end at the credentials conditional on line %d, before the build", line))
			}
		})

		Expect(terminated).To(BeEmpty(),
			"missing credentials must not end main(); the FSMv2 runtime is built further down and an early exit skips it")
	})

	It("starts the supervisor, and not behind a credentials conditional", func() {
		_, recv, ok := bringUpInvariantTopLevelBuild(mainFn)
		Expect(ok).To(BeTrue(), "cannot judge the Run call without locating the build; see the first spec")

		runCall := recv + ".Run"

		Expect(bringUpInvariantCountCalls(mainFn.Body, runCall)).To(Equal(1),
			fmt.Sprintf("main() must call %s exactly once: a supervisor that is built but never run ticks no workers", runCall))

		var gated []string

		bringUpInvariantEachCredentialsBranch(fset, mainFn, func(branch ast.Node, line int, _ token.Pos) {
			if bringUpInvariantCountCalls(branch, runCall) > 0 {
				gated = append(gated, fmt.Sprintf("%s is gated by the credentials conditional on line %d", runCall, line))
			}
		})

		Expect(gated).To(BeEmpty(),
			"the supervisor must be started without credentials; only the communicator may be gated on them")
	})
})

// bringUpInvariantBuildFn is the constructor whose reachability the specs pin.
const bringUpInvariantBuildFn = "buildFSMv2Supervisor"

// bringUpInvariantFindFunc returns the named top-level function, or nil.
func bringUpInvariantFindFunc(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name != nil && fn.Name.Name == name {
			return fn
		}
	}

	return nil
}

// bringUpInvariantTopLevelBuild finds the assignment that calls the supervisor
// constructor directly in main()'s statement list, and returns it with the name
// of the local it assigns first. A build reached only through a nested
// statement or another function is reported as absent, which is the point: such
// a build can be skipped.
func bringUpInvariantTopLevelBuild(mainFn *ast.FuncDecl) (*ast.AssignStmt, string, bool) {
	for _, stmt := range mainFn.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			continue
		}

		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || bringUpInvariantCalleeName(call) != bringUpInvariantBuildFn {
			continue
		}

		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			continue
		}

		return assign, ident.Name, true
	}

	return nil, "", false
}

// bringUpInvariantEachCredentialsBranch calls visit once per branch, body and
// else alike, of every credentials conditional in main().
func bringUpInvariantEachCredentialsBranch(fset *token.FileSet, mainFn *ast.FuncDecl, visit func(branch ast.Node, line int, ifPos token.Pos)) {
	derived := bringUpInvariantCredentialIdents(mainFn)

	ast.Inspect(mainFn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || !bringUpInvariantMentionsCredentials(ifStmt.Cond, derived) {
			return true
		}

		line := fset.Position(ifStmt.Pos()).Line

		for _, branch := range []ast.Node{ifStmt.Body, ifStmt.Else} {
			if branch == nil {
				continue
			}

			visit(branch, line, ifStmt.Pos())
		}

		return true
	})
}

// bringUpInvariantCredentialIdents returns the names of main()'s locals that
// carry a credentials predicate or a credential value, so a conditional written
// on the local is recognised as a credentials conditional.
//
// A value merely computed FROM a credential does not count: tainting the result
// of a call that consumes the API URL would make the ordinary
// "if err != nil { return }" after it fail these specs. Go requires declaration
// before use, so this single source-order pass also carries a name into the next
// assignment that reads it.
func bringUpInvariantCredentialIdents(mainFn *ast.FuncDecl) map[string]bool {
	derived := map[string]bool{}

	ast.Inspect(mainFn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}

		for i, rhs := range assign.Rhs {
			if !bringUpInvariantIsCredentialsValue(rhs, derived) {
				continue
			}

			if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
				derived[ident.Name] = true
			}
		}

		return true
	})

	return derived
}

// bringUpInvariantIsCredentialsValue reports whether an expression is a
// credentials predicate or a direct alias of a credential field.
func bringUpInvariantIsCredentialsValue(expr ast.Expr, derived map[string]bool) bool {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return bringUpInvariantIsCredentialsValue(node.X, derived)
	case *ast.UnaryExpr:
		return node.Op == token.NOT && bringUpInvariantIsCredentialsValue(node.X, derived)
	case *ast.BinaryExpr:
		switch node.Op {
		case token.EQL, token.NEQ, token.LAND, token.LOR:
			return bringUpInvariantMentionsCredentials(node, derived)
		default:
			return false
		}
	case *ast.CallExpr:
		return bringUpInvariantCalleeName(node) == "communicatorEnabled"
	case *ast.SelectorExpr:
		return bringUpInvariantIsCredentialField(node)
	case *ast.Ident:
		return derived[node.Name]
	}

	return false
}

// bringUpInvariantIsCredentialField reports whether a selector reads one of the
// backend credential fields.
func bringUpInvariantIsCredentialField(sel *ast.SelectorExpr) bool {
	return sel.Sel != nil && (sel.Sel.Name == "AuthToken" || sel.Sel.Name == "APIURL")
}

// bringUpInvariantMentionsCredentials reports whether an expression tests
// backend credentials, through the communicatorEnabled seam, the credential
// fields, their environment names, or a local already known to carry one.
func bringUpInvariantMentionsCredentials(expr ast.Expr, derived map[string]bool) bool {
	found := false

	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if node.Name == "communicatorEnabled" || derived[node.Name] {
				found = true
			}
		case *ast.SelectorExpr:
			if bringUpInvariantIsCredentialField(node) {
				found = true
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING && (node.Value == `"AUTH_TOKEN"` || node.Value == `"API_URL"`) {
				found = true
			}
		}

		return !found
	})

	return found
}

// bringUpInvariantEndsFunc reports whether the subtree can end the enclosing
// function. Function literals are pruned: a return inside a closure leaves the
// closure, not main().
func bringUpInvariantEndsFunc(subtree ast.Node) bool {
	found := false

	ast.Inspect(subtree, func(n ast.Node) bool {
		if found {
			return false
		}

		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			found = true
		case *ast.CallExpr:
			if bringUpInvariantCalleeName(node) == "os.Exit" {
				found = true
			}
		}

		return !found
	})

	return found
}

// bringUpInvariantCountCalls counts calls to the named callee, written bare or
// as a single-level selector such as appSup.Run.
func bringUpInvariantCountCalls(subtree ast.Node, target string) int {
	count := 0

	ast.Inspect(subtree, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && bringUpInvariantCalleeName(call) == target {
			count++
		}

		return true
	})

	return count
}

// bringUpInvariantCalleeName renders a call's callee as "fn" or "recv.fn".
func bringUpInvariantCalleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}

		return fn.Sel.Name
	}

	return ""
}
