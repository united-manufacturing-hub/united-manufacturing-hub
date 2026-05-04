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

package fsmv2_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// P1.8 — foundation-cap architecture tests. Each spec anchors a Design
// Intent invariant established earlier in the cascade. Future drift is
// caught at build time.

var _ = Describe("FSMv2 Architecture Validation — P1.8 Foundation Cap", func() {

	Describe("Supervisor injects observation setters before SaveObserved (§14)", func() {
		It("collector source orders SetState/SetChildrenView/etc. ahead of SaveObserved", func() {
			// AST walk SCOPED to the collectAndSaveObservedState function
			// body in collector.go (collector goroutine in the two-goroutine
			// architecture documented in Design Intent §14). SaveObserved
			// must be the LAST observation mutation in that function: any
			// setter call (SetState, SetChildrenView, SetShutdownRequested,
			// SetChildrenCounts, SetParentMappedState) that appears AFTER
			// SaveObserved indicates an injection-after-persistence drift.
			//
			// Scoping by function name (not file-wide) prevents false
			// positives from helper extractions or auxiliary SaveObserved
			// calls elsewhere in the file. If a future refactor renames
			// the function, the scaffolding-broken assertion below catches
			// the rename so the test fails loudly rather than silently
			// becoming vacuous.
			collectorPath := filepath.Join(getFsmv2Dir(), "supervisor", "internal", "collection", "collector.go")

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, collectorPath, nil, 0)
			Expect(err).NotTo(HaveOccurred(), "parse collector.go")

			setterNames := map[string]bool{
				"SetState":             true,
				"SetChildrenView":      true,
				"SetShutdownRequested": true,
				"SetChildrenCounts":    true,
				"SetParentMappedState": true,
			}

			// Locate the collector tick function. The function name has
			// historically been collectAndSaveObservedState; if it ever
			// changes, update both the matcher below and the corresponding
			// godoc reference. The "function not found" assertion catches
			// the rename so the invariant test cannot silently become
			// vacuous.
			const targetFn = "collectAndSaveObservedState"
			var fnBody *ast.BlockStmt
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if fn.Name.Name == targetFn && fn.Body != nil {
					fnBody = fn.Body

					break
				}
			}
			Expect(fnBody).NotTo(BeNil(),
				"%s function not found in collector.go — invariant scaffolding broken (rename? extraction?)", targetFn)

			var saveLine int
			lastSetterLine := 0
			lastSetterName := ""

			ast.Inspect(fnBody, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				name := sel.Sel.Name
				line := fset.Position(call.Pos()).Line

				if name == "SaveObserved" && saveLine == 0 {
					saveLine = line
				}
				if setterNames[name] && line > lastSetterLine {
					lastSetterLine = line
					lastSetterName = name
				}

				return true
			})

			Expect(saveLine).NotTo(BeZero(), "SaveObserved call not found in %s body — invariant scaffolding broken", targetFn)
			Expect(lastSetterLine).NotTo(BeZero(), "no setter calls found in %s body — invariant scaffolding broken", targetFn)
			Expect(lastSetterLine).To(BeNumerically("<", saveLine),
				"setter %s at line %d appears AFTER SaveObserved at line %d in %s — supervisor injection-before-persistence invariant violated (Design Intent §14)",
				lastSetterName, lastSetterLine, saveLine, targetFn)
		})
	})
})
