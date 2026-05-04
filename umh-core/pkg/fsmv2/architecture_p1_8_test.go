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
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"

	// Parent worker packages — imported by name (not blank) because Test #5,
	// #7, #8 and #13 layer 2 call each parent's exported RenderChildren to
	// validate the convention introduced in P2.1.
	applicationworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	applicationsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	communicatorworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	exampleparentworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	transportworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// P1.8 — foundation-cap architecture tests. Each spec anchors a Design
// Intent invariant established earlier in the cascade. Future drift is
// caught at build time.

var _ = Describe("FSMv2 Architecture Validation — P1.8 Foundation Cap", func() {

	// =====================================================================
	// Test 2 — TestNoJsonDashFieldsOnObservation (§14)
	// =====================================================================
	Describe("Observation has no surprise json:\"-\" fields (§14)", func() {
		It("rejects json:\"-\" fields on Observation unless the type implements json.Marshaler", func() {
			// Observation[T] has json:"-" on Status but flattens it via
			// custom MarshalJSON. The check is: the enclosing type must
			// implement json.Marshaler when it carries json:"-".
			obsType := reflect.TypeOf(fsmv2.Observation[struct{}]{})

			// Verify Observation implements json.Marshaler (otherwise the
			// json:"-" Status field would be a real silent-loss bug).
			marshalerType := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
			Expect(obsType.Implements(marshalerType)).To(BeTrue(),
				"Observation[T] carries json:\"-\" Status but does not implement json.Marshaler — silent serialization loss (Design Intent §14)")

			// Walk Observation's fields. For any json:"-" field, the enclosing
			// type MUST be a Marshaler. (For Observation itself this is
			// already verified above; this loop just documents the audit
			// path for future fields.)
			for i := 0; i < obsType.NumField(); i++ {
				f := obsType.Field(i)
				tag := f.Tag.Get("json")
				if tag != "-" {
					continue
				}
				// Status is the documented exception.
				if f.Name == "Status" {
					continue
				}
				Fail(fmt.Sprintf("Observation has unexpected json:\"-\" field %s — review Design Intent §14 before adding more", f.Name))
			}
		})
	})

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
	Describe("Parent renderChildren emits non-nil []ChildSpec{}", func() {
		It("every parent worker's renderChildren returns non-nil []ChildSpec", func() {
			fixtures := parentRenderers()
			Expect(fixtures).NotTo(BeEmpty(),
				"parentRenderers() must enumerate every production parent worker; "+
					"the architecture invariant relies on the registry being exhaustive (P2.1)")

			for _, fx := range fixtures {
				specs := fx.render()
				Expect(specs).NotTo(BeNil(),
					"parent %q renderChildren returned nil; the convention is to return "+
						"an explicit []ChildSpec{} (possibly empty) so the supervisor's "+
						"NextResult.Children discriminator can distinguish 'no opinion' "+
						"(nil sentinel) from 'use this exact set' (non-nil). See the "+
						"NextResult.Children godoc in fsmv2/api.go for the discriminator "+
						"contract.",
					fx.name)
			}
		})

	})

	// =====================================================================
	// Test 7 — TestRenderChildrenIsIdempotent (un-gated at P2.1)
	// =====================================================================
	Describe("renderChildren is idempotent across ticks", func() {
		It("calling renderChildren twice with the same snapshot yields ChildSpec.Hash equality", func() {
			fixtures := parentRenderers()
			Expect(fixtures).NotTo(BeEmpty())

			for _, fx := range fixtures {
				first := fx.render()
				second := fx.render()
				Expect(len(first)).To(Equal(len(second)),
					"parent %q renderChildren returned different slice lengths across two calls — non-deterministic emitter (Design Intent §16 convergence-by-default)", fx.name)

				for i := range first {
					hashA, errA := first[i].Hash()
					hashB, errB := second[i].Hash()
					Expect(errA).NotTo(HaveOccurred(), "parent %q child[%d] Hash error on first call", fx.name, i)
					Expect(errB).NotTo(HaveOccurred(), "parent %q child[%d] Hash error on second call", fx.name, i)
					Expect(hashA).To(Equal(hashB),
						"parent %q child[%d] (Name=%q) hash differs across calls — renderChildren must be idempotent (Design Intent §16; P1.8 architecture test 7)",
						fx.name, i, first[i].Name)
				}
			}
		})
	})

	// =====================================================================
	// Test 8 — TestNoTemplatesInChildSpec (un-gated at P2.1)
	// =====================================================================
	Describe("renderChildren never emits template strings inside ChildSpec", func() {
		It("emitted ChildSpec.Name and WorkerType contain neither '{{' nor '}}'", func() {
			// UserSpec.Config legitimately contains template markers (children
			// re-render their own templates downstream). The check covers only
			// the ChildSpec identity fields whose values are addressed by the
			// supervisor before any template render happens — Name and
			// WorkerType. ChildStartStates entries are also identity-level.
			fixtures := parentRenderers()
			Expect(fixtures).NotTo(BeEmpty())

			for _, fx := range fixtures {
				for i, spec := range fx.render() {
					Expect(spec.Name).NotTo(ContainSubstring("{{"),
						"parent %q child[%d].Name contains template marker — must be resolved before emission", fx.name, i)
					Expect(spec.Name).NotTo(ContainSubstring("}}"),
						"parent %q child[%d].Name contains template marker — must be resolved before emission", fx.name, i)
					Expect(spec.WorkerType).NotTo(ContainSubstring("{{"),
						"parent %q child[%d].WorkerType contains template marker — must be resolved before emission", fx.name, i)
					Expect(spec.WorkerType).NotTo(ContainSubstring("}}"),
						"parent %q child[%d].WorkerType contains template marker — must be resolved before emission", fx.name, i)
					for j, st := range spec.ChildStartStates {
						Expect(st).NotTo(ContainSubstring("{{"),
							"parent %q child[%d].ChildStartStates[%d] contains template marker — must be resolved before emission", fx.name, i, j)
						Expect(st).NotTo(ContainSubstring("}}"),
							"parent %q child[%d].ChildStartStates[%d] contains template marker — must be resolved before emission", fx.name, i, j)
					}
				}
			}
		})
	})

	// =====================================================================
	// Test 9 — TestDDSPurity (§2.7)
	// =====================================================================
	Describe("DeriveDesiredState bodies are pure (§2.7)", func() {
		It("rejects os.*, time.Now/Since, goroutine launches, and channel ops in DDS bodies", func() {
			workersDir := filepath.Join(getFsmv2Dir(), "workers")

			var violations []string

			err := filepath.Walk(workersDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}

				fset := token.NewFileSet()
				file, perr := parser.ParseFile(fset, path, nil, 0)
				if perr != nil {
					// Don't fail the architecture test on unparseable files —
					// other validators will catch that. Just skip.
					return nil
				}

				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					if fn.Name.Name != "DeriveDesiredState" || fn.Body == nil {
						continue
					}

					ast.Inspect(fn.Body, func(n ast.Node) bool {
						switch node := n.(type) {
						case *ast.GoStmt:
							pos := fset.Position(node.Pos())
							violations = append(violations, fmt.Sprintf("%s:%d: goroutine launch in DeriveDesiredState (Design Intent §2.7)", path, pos.Line))
						case *ast.SendStmt:
							pos := fset.Position(node.Pos())
							violations = append(violations, fmt.Sprintf("%s:%d: channel send in DeriveDesiredState (Design Intent §2.7)", path, pos.Line))
						case *ast.UnaryExpr:
							if node.Op == token.ARROW {
								pos := fset.Position(node.Pos())
								violations = append(violations, fmt.Sprintf("%s:%d: channel receive in DeriveDesiredState (Design Intent §2.7)", path, pos.Line))
							}
						case *ast.SelectorExpr:
							pkgIdent, ok := node.X.(*ast.Ident)
							if !ok {
								break
							}
							pkgName := pkgIdent.Name
							sym := node.Sel.Name
							pos := fset.Position(node.Pos())

							// Resolve the identifier against the file's
							// imports so we only flag the stdlib `os` and
							// `time` packages, not user variables that
							// happen to be named `os` or `time` (e.g. a
							// helper named `os` would currently trip the
							// over-broad name check).
							if pkgName == "os" && isStdlibImport(file, "os") {
								violations = append(violations, fmt.Sprintf("%s:%d: os.%s reference in DeriveDesiredState (Design Intent §2.7)", path, pos.Line, sym))
							}
							// time package: forbid non-deterministic clock
							// reads (Now/Since/Until/After) AND
							// timer/ticker/sleep machinery that turns DDS
							// into a side-effect zone.
							if pkgName == "time" && isStdlibImport(file, "time") {
								forbidden := map[string]bool{
									"Now":       true,
									"Since":     true,
									"Until":     true,
									"After":     true,
									"AfterFunc": true,
									"Sleep":     true,
									"NewTicker": true,
									"NewTimer":  true,
									"Tick":      true,
								}
								if forbidden[sym] {
									violations = append(violations, fmt.Sprintf("%s:%d: time.%s reference in DeriveDesiredState (Design Intent §2.7)", path, pos.Line, sym))
								}
							}
							// Random sources: math/rand and crypto/rand
							// produce non-deterministic output, which breaks
							// the DDS hash-cache contract (same input must
							// produce same output).
							if (pkgName == "rand" && (isStdlibImport(file, "math/rand") || isStdlibImport(file, "crypto/rand"))) ||
								pkgName == "rand" {
								// Note: `rand` could shadow either; flag conservatively.
								violations = append(violations, fmt.Sprintf("%s:%d: rand.%s reference in DeriveDesiredState (Design Intent §2.7 hash-cache determinism)", path, pos.Line, sym))
							}
						case *ast.CallExpr:
							// make(chan ...) — flag channel construction.
							ident, ok := node.Fun.(*ast.Ident)
							if !ok || ident.Name != "make" || len(node.Args) == 0 {
								break
							}
							if _, isChan := node.Args[0].(*ast.ChanType); isChan {
								pos := fset.Position(node.Pos())
								violations = append(violations, fmt.Sprintf("%s:%d: make(chan ...) in DeriveDesiredState (Design Intent §2.7)", path, pos.Line))
							}
						}
						return true
					})
				}

				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(violations).To(BeEmpty(),
				"DDS purity violations detected (Design Intent §2.7):\n%s",
				strings.Join(violations, "\n"))
		})
	})

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
	//
	// This test has TWO complementary layers, both un-gated:
	//
	//   1. Helper meta-test (UN-GATED, ships in P1.8): exercises the
	//      validateAllEnabled() detector against synthetic ChildSpec
	//      fixtures. Catches a regression to the detector itself (e.g., a
	//      future "simplification" that no-ops the helper). This layer
	//      does NOT depend on renderChildren existing — it tests the
	//      detector directly. Failure-injection verified at P1.8 ship
	//      time: see .execution/P1.8/test_13_failure_injection.txt for
	//      the PASS-FAIL-PASS round trip on a no-op stub of the helper.
	//
	//   2. Registry walk (UN-GATED at P2.1): walks the parent worker
	//      registry, calls RenderChildren on each, feeds the emitted
	//      ChildSpec lists through validateAllEnabled. Catches forgotten-
	//      Enabled in production renderChildren bodies. Failure-injection
	//      verified at P2.1 ship time: see
	//      .execution/P2.1/test_13_layer2_failure_injection.txt for the
	//      PASS-FAIL-PASS round trip on a deliberate violation in
	//      transport/children.go (push child literal).
	//
	// The two layers are complementary: Layer 1 anchors the detector
	// mechanism, Layer 2 anchors the integration against production
	// literals. Both must keep passing across cascade evolution; any
	// drift in either is the F4⊕G1 trap re-emerging.
	//
	// Test 5 (ParentRenderChildrenEmitsNonNil), Test 7
	// (RenderChildrenIsIdempotent), and Test 8 (NoTemplatesInChildSpec)
	// also un-gated at P2.1 — they share the parentRenderers() registry
	// fixture and exercise complementary RenderChildren invariants.
	// Test 6 (RenderChildrenCalledAtTopOfStateNext) un-gated at P2.2 once
	// every parent state.Next started invoking renderChildren as the first
	// non-shutdown statement (see Describe block above).
	Describe("renderChildren emits ChildSpec with Enabled set explicitly (F4⊕G1 trap)", func() {
		It("every parent worker's RenderChildren emits Enabled:true on every spec (registry walk, layer 2)", func() {
			// Layer 2 — registry walk (un-gated at P2.1). Walks every
			// production parent worker, calls its exported RenderChildren
			// on a representative snapshot, and feeds the emitted
			// ChildSpec slice through validateAllEnabled. This is the
			// detector that catches a developer forgetting `Enabled: true`
			// in a renderChildren body — the F4⊕G1 trap proper.
			//
			// Failure-injection re-verified at P2.1 ship time:
			// .execution/P2.1/test_13_layer2_failure_injection.txt
			// records the PASS-FAIL-PASS round trip after dropping
			// `Enabled: true` from one site (transport push) and
			// restoring it.
			fixtures := parentRenderers()
			Expect(fixtures).NotTo(BeEmpty(),
				"parentRenderers() must enumerate every production parent worker; "+
					"the registry walk relies on the registry being exhaustive (P2.1)")

			for _, fx := range fixtures {
				specs := fx.render()
				Expect(specs).NotTo(BeNil(), "parent %q renderChildren returned nil — see Test 5", fx.name)
				if len(specs) == 0 {
					// Empty children-set is a valid 'I am a parent and I want
					// zero children right now' signal; nothing to validate.
					continue
				}
				err := validateAllEnabled(specs)
				Expect(err).NotTo(HaveOccurred(),
					"parent %q RenderChildren violates §4-C LOCKED (forgotten Enabled: true): %v", fx.name, err)
			}
		})
	})

	// =====================================================================
	// Bonus 1 — TestVariablesInternalSchemaStability (§4-D LOCKED)
	// =====================================================================
	//
	// Pairs with the wire-level round-trip test in
	// integration/variables_typed_wire_test.go. The two tests catch
	// different defect classes intentionally:
	//
	//   - This test (reflect-based, struct-definition level): catches
	//     EXACT tag string changes — e.g., `json:"parentID,omitempty"`
	//     accidentally becoming `json:"parentID"` (omitempty modifier
	//     dropped) would slip past the wire-level test (the key is still
	//     present in JSON output) but fails here because the tag string
	//     no longer matches the locked spelling.
	//
	//   - The integration round-trip test (wire-level): catches whether
	//     the field actually serializes / deserializes through JSON
	//     correctly — e.g., a custom MarshalJSON that drops a field, or
	//     an alias renaming field case while preserving the tag would
	//     pass this reflect check but fail the round-trip.
	//
	// Both tests are cheap (microseconds each); keep both for layered
	// coverage of the §4-D LOCKED contract.
	Describe("VariablesInternal preserves §4-D LOCKED JSON tag spelling", func() {
		It("WorkerID/ParentID/BridgedBy/CreatedAt match the locked tags", func() {
			expected := map[string]string{
				"WorkerID":  "workerID",
				"ParentID":  "parentID,omitempty",
				"BridgedBy": "bridgedBy,omitempty",
				"CreatedAt": "createdAt",
			}

			typ := reflect.TypeOf(config.VariablesInternal{})
			for name, wantTag := range expected {
				f, ok := typ.FieldByName(name)
				Expect(ok).To(BeTrue(),
					"field %s missing from VariablesInternal — §4-D LOCKED schema violation", name)
				gotTag := f.Tag.Get("json")
				Expect(gotTag).To(Equal(wantTag),
					"field %s json tag = %q; want %q (§4-D LOCKED)", name, gotTag, wantTag)
			}
		})
	})

	// =====================================================================
	// Test 14 — Mirror byte-equivalence (P2.4 / pr2_issues #10)
	// =====================================================================
	//
	// Goal: detect drift between the worker-package canonical RenderChildren
	// emitter and the state-package mirror that exists to break the parent →
	// state import cycle. Today's matrix:
	//
	//   - application       → mirror present, byte-equivalent expected
	//   - exampleparent     → mirror present, migration-window exemption
	//                         (mirror returns principled nil; canonical body
	//                         is free to differ during the window)
	//   - communicator      → no mirror needed (no import cycle)
	//   - transport         → no mirror needed (no import cycle)
	//
	// The test fails if (a) a non-exempt mirror diverges from its canonical
	// body, (b) the exampleparent mirror stops returning principled nil
	// (would re-introduce the silent-despawn risk the migration-window
	// exemption is built around), or (c) a registry entry references a
	// missing canonical/mirror file.
	//
	// AST-level approach (not text-diff): parse both files, find the
	// RenderChildren FuncDecl, render the body via go/printer with a
	// fixed config, then strip selector-package qualifiers on every
	// `pkg.Ident` (so the canonical's `config.ChildSpec` and a mirror's
	// hypothetically-aliased reference compare equal). Whitespace and
	// comments are normalized out by go/printer's TabIndent + zero
	// flags, leaving load-bearing tokens (literals, field names,
	// operators) as the only signal.
	//
	// Failure-injection: .execution/P2.4/mirror_byteequivalence_failure_injection.txt
	Describe("RenderChildren state-package mirrors are byte-equivalent to canonical worker bodies (P2.4)", func() {
		type mirrorEntry struct {
			name            string
			canonicalPath   string
			mirrorPath      string // empty when no mirror exists (no import cycle)
			migrationWindow bool   // true → mirror must return principled nil only
		}

		registry := []mirrorEntry{
			{
				name:          "application",
				canonicalPath: filepath.Join(getFsmv2Dir(), "workers", "application", "children.go"),
				mirrorPath:    filepath.Join(getFsmv2Dir(), "workers", "application", "state", "render_children.go"),
			},
			{
				name:            "exampleparent",
				canonicalPath:   filepath.Join(getFsmv2Dir(), "workers", "example", "exampleparent", "children.go"),
				mirrorPath:      filepath.Join(getFsmv2Dir(), "workers", "example", "exampleparent", "state", "render_children.go"),
				migrationWindow: true,
			},
			{
				name:          "communicator",
				canonicalPath: filepath.Join(getFsmv2Dir(), "workers", "communicator", "children.go"),
			},
			{
				name:          "transport",
				canonicalPath: filepath.Join(getFsmv2Dir(), "workers", "transport", "children.go"),
			},
		}

		It("each mirror is byte-equivalent to its canonical (or principled-nil if migration-window exempt)", func() {
			for _, entry := range registry {
				By(fmt.Sprintf("checking parent %q", entry.name))

				canonicalBody, canErr := normalizedRenderChildrenBody(entry.canonicalPath)
				Expect(canErr).NotTo(HaveOccurred(),
					"failed to extract canonical RenderChildren body from %s", entry.canonicalPath)

				if entry.mirrorPath == "" {
					// No mirror — nothing to compare. Sanity-check that the
					// canonical exists and is non-empty so a typo in
					// canonicalPath doesn't silently green-light the entry.
					Expect(canonicalBody).NotTo(BeEmpty(),
						"parent %q canonical RenderChildren body is empty — likely a path typo or missing decl",
						entry.name)
					continue
				}

				mirrorBody, mirErr := normalizedRenderChildrenBody(entry.mirrorPath)
				Expect(mirErr).NotTo(HaveOccurred(),
					"failed to extract mirror RenderChildren body from %s", entry.mirrorPath)

				if entry.migrationWindow {
					// Migration-window exemption: the mirror must return
					// principled nil only (i.e. the body must reduce to a
					// `return nil` after normalization, possibly preceded by
					// a `_ = snap` no-op). Anything else either re-introduces
					// the silent-despawn risk (`return []ChildSpec{}` would
					// authoritatively zero teaching children once the
					// supervisor reads NextResult.Children) or starts
					// emitting children that diverge from the canonical
					// body without a documented migration path.
					Expect(isPrincipledNilBody(mirrorBody)).To(BeTrue(),
						"parent %q is migration-window exempt; its mirror at %s must return principled nil "+
							"(body reducible to `_ = snap; return nil`). Observed normalized body:\n%s\n\n"+
							"If this is intentional, the exemption flag in the registry must flip to false "+
							"and the mirror must be byte-equivalent to the canonical at %s.",
						entry.name, entry.mirrorPath, mirrorBody, entry.canonicalPath)
					continue
				}

				Expect(mirrorBody).To(Equal(canonicalBody),
					"parent %q: state-package mirror at %s diverges from canonical worker body at %s.\n"+
						"This is the F4⊕G1-style trap that mirror byte-equivalence guards against — a developer "+
						"editing one of the two files (e.g. dropping `Enabled: true`) without updating the other "+
						"would silently produce divergent ChildSpec slices on the state.Next path vs the legacy "+
						"DDS path. Both files must move together.\n\n"+
						"Canonical body:\n%s\n\nMirror body:\n%s",
					entry.name, entry.mirrorPath, entry.canonicalPath, canonicalBody, mirrorBody)
			}
		})
	})
})

// normalizedRenderChildrenBody parses the Go file at path, finds the
// top-level `func RenderChildren(...)` declaration, and returns the
// canonical-form text of its body suitable for byte-equality comparison
// against another mirror/canonical.
//
// Normalization steps:
//  1. go/printer with TabIndent flag — same whitespace regardless of
//     gofmt history.
//  2. Strip selector-package qualifiers on every `pkg.Ident` reference
//     (e.g. `config.ChildSpec` becomes `ChildSpec`). This lets the test
//     accept the case where the canonical and mirror use different import
//     aliases for the same package — only the bare identifier matters
//     for "do these two functions do the same thing".
//
// Returns an error if the file cannot be parsed or if RenderChildren is
// missing.
func normalizedRenderChildrenBody(path string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Recv != nil {
			continue
		}
		if fd.Name == nil || fd.Name.Name != "RenderChildren" {
			continue
		}
		fn = fd
		break
	}
	if fn == nil || fn.Body == nil {
		return "", fmt.Errorf("no top-level func RenderChildren found in %s", path)
	}

	// Strip package qualifiers on every selector. We mutate a copy via
	// ast.Inspect; rewriting `pkg.Ident` to `Ident` makes the printed
	// form independent of the alias the importing file chose.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Only strip when the LHS is a bare identifier (likely a
		// package alias). Don't touch chained selectors (`x.Y.Z`).
		if _, ok := sel.X.(*ast.Ident); !ok {
			return true
		}
		// Replace `pkg.Ident` with `Ident` in-place: replacing the
		// selector node would require parent-walking, so we instead
		// blank out the X side. The printer renders `.Ident` for an
		// empty-name X — workaround: we point X at an Ident whose
		// Name is the empty string AND we remove the leading dot by
		// returning the Sel as a plain Ident. Easiest: rename X to
		// the empty string and post-process the printed form.
		if id, ok := sel.X.(*ast.Ident); ok {
			id.Name = ""
		}
		return true
	})

	var buf bytes.Buffer
	pcfg := printer.Config{Mode: printer.TabIndent, Tabwidth: 8}
	if err := pcfg.Fprint(&buf, fset, fn.Body); err != nil {
		return "", fmt.Errorf("print %s body: %w", path, err)
	}

	out := buf.String()
	// Post-process: with X.Name="" the printer emits `.Ident` — collapse
	// `.Ident` to `Ident` everywhere so e.g. `config.ChildSpec` (post-
	// X-blanking ".ChildSpec") and an alias-renamed reference both
	// reduce to `ChildSpec`. Also collapse runs of whitespace introduced
	// by the empty-X expression to keep formatting stable.
	out = strings.ReplaceAll(out, " .", " ")
	out = strings.ReplaceAll(out, "\t.", "\t")
	out = strings.ReplaceAll(out, "(.", "(")
	out = strings.ReplaceAll(out, "[.", "[")
	out = strings.ReplaceAll(out, ",.", ",")
	out = strings.ReplaceAll(out, "{.", "{")

	return out, nil
}

// isPrincipledNilBody returns true when body (already normalized by
// normalizedRenderChildrenBody) is the canonical principled-nil form
// recognized by the migration-window exemption: a function body that
// reduces to `return nil` (with optional preceding no-op `_ = snap`).
func isPrincipledNilBody(body string) bool {
	// Tolerant of whitespace + the `_ = snap` line (keeps `snap` referenced
	// so go vet / unused-parameter linters stay happy).
	stripped := strings.Join(strings.Fields(body), " ")
	// Acceptable normalized shapes:
	//   "{ _ = snap return nil }"
	//   "{ return nil }"
	switch stripped {
	case "{ _ = snap return nil }",
		"{ return nil }",
		"{ _ = snap; return nil }",
		"{ return nil; }":
		return true
	}
	return false
}

// validateAllEnabled is the F4⊕G1 trap detector. It walks a list of ChildSpec
// values (as a parent's renderChildren would emit) and returns an error for
// any spec with Enabled=false. Per §4-C LOCKED, the zero value of Enabled is
// false; parents that want a running child MUST explicitly set Enabled: true
// in their renderChildren body.
func validateAllEnabled(specs []config.ChildSpec) error {
	for i, spec := range specs {
		if !spec.Enabled {
			return fmt.Errorf("spec[%d] (Name=%q) has Enabled=false; renderChildren must emit Enabled: true explicitly (F4⊕G1 trap: Enabled default-false × ShouldStop collapse)", i, spec.Name)
		}
	}
	return nil
}

// parentRendererFixture pairs a parent worker name with a closure that
// invokes that worker's exported RenderChildren on a representative
// snapshot. Test #5/7/8/13-layer-2 walk this list to exercise the
// renderChildren convention against every production parent.
type parentRendererFixture struct {
	name   string
	render func() []config.ChildSpec
}

// parentRenderers returns the registry of parent-worker renderChildren
// fixtures. Each fixture builds a typed snapshot the parent's RenderChildren
// expects and invokes it. Adding a new parent worker means adding a fixture
// here; the test #5/7/8/13-layer-2 walks pick it up automatically.
//
// The fixtures intentionally exercise the non-empty-children path so
// validateAllEnabled has at least one ChildSpec to inspect (catches the
// forgotten-Enabled trap). The empty-input branches (nil-spec startup) are
// covered separately by each parent's own unit tests.
func parentRenderers() []parentRendererFixture {
	return []parentRendererFixture{
		{
			name: "communicator",
			render: func() []config.ChildSpec {
				return communicatorworker.RenderChildren(fsmv2.WorkerSnapshot[communicatorworker.CommunicatorConfig, communicatorworker.CommunicatorStatus]{
					Desired: fsmv2.WrappedDesiredState[communicatorworker.CommunicatorConfig]{
						BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
						ChildrenSpecs: []config.ChildSpec{{
							Name:       "transport",
							WorkerType: "transport",
							UserSpec:   config.UserSpec{Config: "relayURL: https://example.com"},
						}},
					},
				})
			},
		},
		{
			name: "transport",
			render: func() []config.ChildSpec {
				return transportworker.RenderChildren(fsmv2.WorkerSnapshot[transportworker.TransportConfig, transportworker.TransportStatus]{
					Desired: fsmv2.WrappedDesiredState[transportworker.TransportConfig]{
						BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
						ChildrenSpecs: []config.ChildSpec{{
							Name:       "push",
							WorkerType: "push",
							UserSpec:   config.UserSpec{Config: "relayURL: https://example.com"},
						}},
					},
				})
			},
		},
		{
			name: "exampleparent",
			render: func() []config.ChildSpec {
				return exampleparentworker.RenderChildren(&exampleparentworker.ParentUserSpec{
					ChildrenCount:   2,
					ChildWorkerType: "examplechild",
				})
			},
		},
		{
			name: "application",
			render: func() []config.ChildSpec {
				return applicationworker.RenderChildren(fsmv2.WorkerSnapshot[applicationsnapshot.ApplicationConfig, applicationsnapshot.ApplicationStatus]{
					Desired: fsmv2.WrappedDesiredState[applicationsnapshot.ApplicationConfig]{
						BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
						ChildrenSpecs: []config.ChildSpec{{
							Name:       "child-a",
							WorkerType: "examplechild",
							UserSpec:   config.UserSpec{Config: "device: x"},
						}},
					},
				})
			},
		},
	}
}

// isStdlibImport returns true when the given file imports the stdlib package
// at the given path under its default name (i.e. without a rename alias).
// Used by Test 9 to disambiguate `os.X` references that target the stdlib
// `os` package from references that target a user variable or shadowed
// identifier named `os`.
func isStdlibImport(file *ast.File, pkgPath string) bool {
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		// imp.Path.Value includes the surrounding quotes.
		if imp.Path.Value != `"`+pkgPath+`"` {
			continue
		}
		// Renamed imports (`alias "pkg"`) don't apply — Test 9 only
		// guards the default-name reference.
		if imp.Name == nil {
			return true
		}
	}

	return false
}

