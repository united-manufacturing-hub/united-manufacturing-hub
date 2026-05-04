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

	// =====================================================================
	// Test 2 — TestNoJsonDashFieldsOnObservation (§14)
	// =====================================================================
	// REMOVED in PR3-c16: replaced with godoc on Observation[T] (the §14
	// invariant is documented in the type's godoc; runtime reflect check
	// was redundant with the documented contract).

	// =====================================================================
	// Test 3 — TestSupervisorInjectionBeforePersistence (§14)
	// =====================================================================
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

	// =====================================================================
	// Test 5 — TestParentRenderChildrenEmitsNonNil (un-gated at P2.1)
	// =====================================================================
	// REMOVED in PR3-c9: Class C — F4⊕G1 trap structurally impossible after
	// migrating all RenderChildren callsites to config.NewChildSpec, which
	// always sets Enabled:true. The non-nil convention remains documented in
	// NextResult.Children godoc.

	// =====================================================================
	// Test 7 — TestRenderChildrenIsIdempotent (un-gated at P2.1)
	// =====================================================================
	// REMOVED in PR3-c10: idempotency is guaranteed by-construction now that
	// every RenderChildren emitter is a pure transformation over snapshot
	// state — no goroutines, no clocks, no random sources (DDS purity is
	// enforced by golangci-lint forbidigo rules). The runtime hash-equality
	// check duplicated coverage already provided by per-worker unit tests
	// and by the snapshot-package mirror migration completed in this
	// commit.

	// =====================================================================
	// Test 8 — TestNoTemplatesInChildSpec (un-gated at P2.1)
	// =====================================================================
	// REMOVED in PR3-c9: Class C — overlaps with template-resolution coverage
	// in production renderChildren bodies; ChildSpec.Name / WorkerType are
	// supplied as Go literals at every callsite (no template-marker source).

	// =====================================================================
	// Test 9 — TestDDSPurity (§2.7)
	// =====================================================================
	// REMOVED in PR3-c10: DDS purity is enforced by golangci-lint forbidigo
	// rules (no time.Now, no rand, no goroutines, no channel ops in DDS
	// bodies) which run on every commit. The duplicate AST walk added
	// no coverage and was prone to false positives on identifier shadowing.

	// =====================================================================
	// Test 10 — TestStateActionChildrenTruthTable
	// =====================================================================
	//
	// REMOVED at iter-1 per reviewer's I2: the XOR-state-vs-action invariant
	// is already covered with proper AST-level inspection by
	// `ValidateStateXORAction` in `internal/validator/state.go`, surfaced
	// via the existing arch test "should return either a new state or an
	// action, never both" (architecture_test.go:99-108). A string-only
	// match on `reconciliation.go` source would be lower-fidelity duplicate
	// coverage that could match a comment or a commented-out version of
	// the guard. The existing arch test is authoritative.
	//
	// If a future test ever proves the existing AST coverage is
	// insufficient (e.g., the supervisor's panic-message format becomes a
	// load-bearing contract), this slot is the natural home — but as of
	// P1.8 ship time, ValidateStateXORAction already exercises the
	// invariant at the right layer.

	// =====================================================================
	// Test 13 — TestRenderChildrenEmitsExplicitEnabled (F4⊕G1 trap)
	// =====================================================================
	// REMOVED in PR3-c9: Class C — F4⊕G1 trap structurally impossible after
	// migrating all RenderChildren callsites to config.NewChildSpec. The
	// constructor always sets Enabled:true; there is no longer a "forgotten
	// Enabled" failure mode for the registry walk to detect.

	// =====================================================================
	// Bonus 1 — TestVariablesInternalSchemaStability (§4-D LOCKED)
	// =====================================================================
	// REMOVED in PR3-c16: the §4-D LOCKED tag spelling is documented in
	// godoc on config.VariablesInternal (and a comment near LOCKED tag
	// constants in config/childspec.go). The wire-level round-trip test
	// in integration/variables_typed_wire_test.go retains coverage of the
	// serialization contract; the reflect-based exact-tag check is a
	// doc-invariant duplicate.

	// =====================================================================
	// Test 14 — Mirror byte-equivalence (P2.4 / pr2_issues #10)
	// =====================================================================
	// REMOVED in PR3-c10: with state-package RenderChildren mirrors moved
	// to snapshot/, there is no longer a parallel mirror to keep in sync —
	// state.Next now calls snapshot.RenderChildren directly. The byte-
	// equivalence trap (F4⊕G1 drift between two copies of the same body)
	// is structurally impossible without a second body to drift from.
})
