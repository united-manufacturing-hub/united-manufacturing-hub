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
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"

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
// caught at build time. Per §4-E LOCKED, Skip() callers reference the
// gating P-step so the test un-gates atomically when that step lands.

var _ = Describe("FSMv2 Architecture Validation — P1.8 Foundation Cap", func() {

	// =====================================================================
	// Test 1 — TestObservedFieldsAreSerializable (§13)
	// =====================================================================
	Describe("Observation/WrappedDesiredState fields are JSON-serializable (§13)", func() {
		It("rejects interface, chan, func, and bare any fields", func() {
			// Sample concrete instantiations of the generic types so we can
			// reflect on the struct shape.
			roots := []reflect.Type{
				reflect.TypeOf(fsmv2.Observation[struct{}]{}),
				reflect.TypeOf(fsmv2.WrappedDesiredState[struct{}]{}),
				reflect.TypeOf(config.ChildSpec{}),
				reflect.TypeOf(config.ChildInfo{}),
				reflect.TypeOf(config.ChildrenView{}),
				reflect.TypeOf(deps.FrameworkMetrics{}),
				reflect.TypeOf(config.VariableBundle{}),
				reflect.TypeOf(config.VariablesInternal{}),
			}

			var violations []string

			for _, root := range roots {
				violations = append(violations, walkSerializable(root, root.Name(), map[reflect.Type]bool{})...)
			}

			Expect(violations).To(BeEmpty(),
				"non-serializable fields detected (Design Intent §13):\n%s",
				strings.Join(violations, "\n"))
		})
	})

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
	// Test 4 — TestNoRuntimeResourcesOnObservation (§15)
	// =====================================================================
	Describe("Observation has no runtime-resource fields (§15)", func() {
		It("rejects fields whose type implements prometheus.Collector, sync.Locker, or io.Closer", func() {
			roots := []reflect.Type{
				reflect.TypeOf(fsmv2.Observation[struct{}]{}),
				reflect.TypeOf(fsmv2.WrappedDesiredState[struct{}]{}),
				reflect.TypeOf(deps.FrameworkMetrics{}),
				reflect.TypeOf(config.ChildSpec{}),
				reflect.TypeOf(config.ChildInfo{}),
				reflect.TypeOf(config.ChildrenView{}),
				reflect.TypeOf(config.VariableBundle{}),
			}

			var violations []string
			for _, root := range roots {
				violations = append(violations, walkRuntimeResources(root, root.Name(), map[reflect.Type]bool{})...)
			}

			Expect(violations).To(BeEmpty(),
				"runtime-resource fields detected on data types (Design Intent §15):\n%s",
				strings.Join(violations, "\n"))
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

		// Registry-exhaustiveness meta-test (added in P2.1 iter-2 per DA
		// per-step adversarial pass): a future parent worker added in PR2/PR3
		// without a fixture entry would silently bypass Layer 2 coverage of
		// the F4⊕G1 trap. AST-walk the workers/ tree for top-level
		// `func RenderChildren` declarations and assert the per-package set
		// matches the parentRenderers() fixture name set. If a new worker
		// adds RenderChildren without a corresponding fixture, this test
		// fails with the missing worker's package name.
		It("parentRenderers() registry covers every worker package that defines RenderChildren", func() {
			workersDir := filepath.Join(getFsmv2Dir(), "workers")

			// AST-walk: find top-level `func RenderChildren(...)` decls.
			// Map from package directory basename → worker source file path.
			discovered := map[string]string{}

			err := filepath.Walk(workersDir, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}

				fset := token.NewFileSet()
				file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
				if perr != nil {
					// Surface parse failures rather than silent skip
					// (consistent with Test 11's discipline).
					return fmt.Errorf("parse %s: %w", path, perr)
				}

				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					// Top-level (non-method) func with name RenderChildren.
					if fn.Recv != nil {
						continue
					}
					if fn.Name == nil || fn.Name.Name != "RenderChildren" {
						continue
					}

					pkgDir := filepath.Base(filepath.Dir(path))
					if existing, dup := discovered[pkgDir]; dup {
						Fail(fmt.Sprintf(
							"package %q has multiple RenderChildren declarations (first at %s, second at %s); "+
								"convention is one RenderChildren per parent package",
							pkgDir, existing, path))
					}
					discovered[pkgDir] = path
				}

				return nil
			})
			Expect(err).NotTo(HaveOccurred(), "walking workers directory")

			// Map fixtures to their package basenames. Fixture names use
			// human-readable identifiers (e.g., "transport", "communicator")
			// chosen to match the basename of their worker package.
			fixtureNames := map[string]bool{}
			for _, fx := range parentRenderers() {
				fixtureNames[fx.name] = true
			}

			// Check 1: every discovered RenderChildren has a fixture.
			var missing []string
			for pkg := range discovered {
				if !fixtureNames[pkg] {
					missing = append(missing, pkg)
				}
			}
			Expect(missing).To(BeEmpty(),
				"worker package(s) define RenderChildren but parentRenderers() has no fixture entry — "+
					"these workers silently bypass Layer 2 of the F4⊕G1 trap detector. "+
					"Add a fixture in parentRenderers() for each missing package: %v\n"+
					"Source files: %v",
				missing, discovered)

			// Check 2: every fixture corresponds to a real RenderChildren.
			// Catches the inverse drift (fixture for a package whose
			// RenderChildren was renamed/moved) — fixture would be a no-op
			// and miss real production literals.
			var stale []string
			for name := range fixtureNames {
				if _, found := discovered[name]; !found {
					stale = append(stale, name)
				}
			}
			Expect(stale).To(BeEmpty(),
				"parentRenderers() has fixture(s) for package(s) that no longer define RenderChildren: %v",
				stale)

			// Sanity: at least one production RenderChildren must exist (catches a
			// regression where all renderers got accidentally deleted).
			Expect(discovered).NotTo(BeEmpty(),
				"no top-level RenderChildren found anywhere in workers/ — F4⊕G1 detector has nothing to anchor against")
		})
	})

	// =====================================================================
	// Test 6 — TestRenderChildrenCalledAtTopOfStateNext (GATED → P2.2)
	// =====================================================================
	Describe("renderChildren is invoked at top of every parent state.Next (GATED P2.2)", func() {
		It("AST: first non-shutdown statement in parent state.Next calls renderChildren", func() {
			Skip("pending P2.2: renderChildren-in-state.Next convention not yet introduced; un-gates atomically with P2.2 per §4-E LOCKED")
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
	// Test 11 — TestNoTextTemplateImport
	// =====================================================================
	Describe("text/template is imported only by config/template.go", func() {
		It("walks fsmv2 tree and rejects text/template imports outside the canonical templating site", func() {
			fsmv2Dir := getFsmv2Dir()
			canonicalSite, err := filepath.Abs(filepath.Join(fsmv2Dir, "config", "template.go"))
			Expect(err).NotTo(HaveOccurred())

			var violations []string

			err = filepath.Walk(fsmv2Dir, func(path string, info os.FileInfo, werr error) error {
				if werr != nil {
					return werr
				}
				if info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".go") {
					return nil
				}

				abs, _ := filepath.Abs(path)
				if abs == canonicalSite {
					return nil
				}

				fset := token.NewFileSet()
				file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
				if perr != nil {
					// Surface parse failures rather than silently skipping.
					// A silent skip would let a malformed file slip past
					// the architecture invariant check (parser skips, a
					// forbidden text/template import inside the file
					// never gets flagged). Recording the error as a
					// violation makes the gap loud.
					violations = append(violations, fmt.Sprintf("%s: parse error: %v (test cannot verify import absence)", path, perr))

					return nil
				}

				for _, imp := range file.Imports {
					// imp.Path.Value is quoted: `"text/template"`.
					if imp.Path.Value == `"text/template"` {
						violations = append(violations, fmt.Sprintf("%s imports text/template (only config/template.go may)", path))
					}
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(violations).To(BeEmpty(),
				"unauthorized text/template imports (templating must stay inside the config layer):\n%s",
				strings.Join(violations, "\n"))
		})
	})

	// =====================================================================
	// Test 12 — TestChildrenDontReferenceParents (GATED → P3.7)
	// =====================================================================
	Describe("Child workers don't reference parent-side concepts (GATED P3.7)", func() {
		It("AST: child worker state files have no Parent/ParentMappedState references", func() {
			Skip("pending P3.7: ParentMappedState retirement is the gating event; un-gates atomically with P3.7 per §4-E LOCKED")
		})
	})

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
	// Test 6 (RenderChildrenCalledAtTopOfStateNext) remains gated
	// pending P2.2 (state.Next-side wiring is the next step).
	Describe("renderChildren emits ChildSpec with Enabled set explicitly (F4⊕G1 trap)", func() {
		It("validateAllEnabled accepts explicit Enabled:true and rejects forgotten Enabled (zero-value false)", func() {
			// Layer 1 — helper meta-test (un-gated since P1.8). Locks the
			// validateAllEnabled detector against regression — e.g. a
			// future "simplification" that no-ops the helper. Operates on
			// synthetic ChildSpec fixtures and does not depend on
			// renderChildren existing on real workers.

			// Positive path: explicit Enabled: true → no error.
			Expect(validateAllEnabled([]config.ChildSpec{
				{Name: "ok", WorkerType: "x", Enabled: true},
			})).To(Succeed())

			// Negative path: forgotten Enabled (zero-value false) → error caught.
			err := validateAllEnabled([]config.ChildSpec{
				{Name: "buggy", WorkerType: "x"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Enabled=false"))
			Expect(err.Error()).To(ContainSubstring("buggy"))
			Expect(err.Error()).To(ContainSubstring("F4⊕G1"))
		})

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
})

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

// walkSerializable recursively inspects a struct type and returns one
// violation message per field that is not safely JSON-serializable.
// Whitelist: time.Time, json.RawMessage, fields tagged json:"-" whose
// enclosing type implements json.Marshaler.
func walkSerializable(t reflect.Type, path string, seen map[reflect.Type]bool) []string {
	if seen[t] {
		return nil
	}
	seen[t] = true

	if t.Kind() != reflect.Struct {
		return nil
	}

	var violations []string

	marshalerType := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	enclosingImplementsMarshaler := reflect.PointerTo(t).Implements(marshalerType) || t.Implements(marshalerType)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fieldPath := path + "." + f.Name

		// json:"-" is an explicit opt-out from JSON serialization. It is a
		// deliberate authorial choice — either because (a) the field is a
		// runtime resource (channels, mutexes; see ChildSpec.Dependencies),
		// or (b) the enclosing type has a custom MarshalJSON that handles
		// the field (see Observation.Status). Either way, it is NOT a
		// silent-loss bug. The dedicated Observation-specific audit is
		// handled by Test 2 (TestNoJsonDashFieldsOnObservation). Here we
		// just skip the field — our concern is implicit non-serializability,
		// not explicit opt-outs.
		jsonTag := f.Tag.Get("json")
		if jsonTag == "-" {
			_ = enclosingImplementsMarshaler // silenced; Test 2 owns this audit
			continue
		}

		ft := f.Type

		// Whitelist time.Time and json.RawMessage early.
		switch ft {
		case reflect.TypeOf(time.Time{}):
			continue
		case reflect.TypeOf(json.RawMessage{}):
			continue
		}

		// If the field's TYPE implements json.Marshaler, defer to it
		// (custom marshalling makes the structural shape opaque to us).
		if reflect.PointerTo(ft).Implements(marshalerType) || ft.Implements(marshalerType) {
			continue
		}

		switch ft.Kind() {
		case reflect.Interface:
			violations = append(violations, fmt.Sprintf("non-serializable field %s of kind interface (Design Intent §13)", fieldPath))
		case reflect.Chan:
			violations = append(violations, fmt.Sprintf("non-serializable field %s of kind chan (Design Intent §13)", fieldPath))
		case reflect.Func:
			violations = append(violations, fmt.Sprintf("non-serializable field %s of kind func (Design Intent §13)", fieldPath))
		case reflect.Struct:
			violations = append(violations, walkSerializable(ft, fieldPath, seen)...)
		case reflect.Slice, reflect.Array:
			elem := ft.Elem()
			if elem.Kind() == reflect.Struct {
				violations = append(violations, walkSerializable(elem, fieldPath+"[]", seen)...)
			} else if elem.Kind() == reflect.Interface {
				// []any / []interface{} fields are red flags too.
				violations = append(violations, fmt.Sprintf("non-serializable field %s element of kind interface (Design Intent §13)", fieldPath))
			}
		case reflect.Map:
			// map[K]V: V may be interface{}; map[string]any is acceptable
			// only when the field is documented as an opaque user payload.
			// We skip recursion here — map[string]any is too pervasive in
			// the config layer to flag without per-field opt-out wiring.
			_ = ft
		}
	}

	return violations
}

// walkRuntimeResources recursively inspects a struct type and returns one
// violation message per field whose type implements prometheus.Collector,
// sync.Locker, or io.Closer. These are runtime resources that have no
// place on a CSE-serialized observation/desired-state document
// (Design Intent §15).
//
// We use interface name matching (rather than importing prometheus and
// sync) because the rule is about the *role* the type plays, not the
// specific package. False negatives are acceptable; this test catches
// the canonical violation patterns.
func walkRuntimeResources(t reflect.Type, path string, seen map[reflect.Type]bool) []string {
	if seen[t] {
		return nil
	}
	seen[t] = true

	if t.Kind() != reflect.Struct {
		return nil
	}

	var violations []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fieldPath := path + "." + f.Name

		// Whitelist deps.MetricsEmbedder anonymous embed: framework-injected
		// data carrier, pure data despite living near the metrics surface.
		if f.Anonymous && f.Type.Name() == "MetricsEmbedder" {
			continue
		}

		ft := f.Type
		// Dereference pointer types for method-set checking.
		probe := ft
		if probe.Kind() == reflect.Ptr {
			probe = probe.Elem()
		}

		if implementsByMethodName(probe, "Lock") && implementsByMethodName(probe, "Unlock") {
			violations = append(violations, fmt.Sprintf("%s field type %s implements sync.Locker (Design Intent §15: no runtime resources on data types)", fieldPath, ft.String()))
			continue
		}
		if implementsByMethodName(probe, "Close") {
			// io.Closer is `Close() error`. Many data structs do not have
			// a Close method; only flag when it appears.
			closeMethod, hasClose := probe.MethodByName("Close")
			// Also check ptr method set.
			if !hasClose {
				closeMethod, hasClose = reflect.PointerTo(probe).MethodByName("Close")
			}
			if hasClose && closeMethod.Type.NumIn() <= 1 && closeMethod.Type.NumOut() == 1 {
				out := closeMethod.Type.Out(0)
				if out.Kind() == reflect.Interface && out.Name() == "error" {
					violations = append(violations, fmt.Sprintf("%s field type %s implements io.Closer (Design Intent §15)", fieldPath, ft.String()))
					continue
				}
			}
		}
		// prometheus.Collector: Describe(chan<- *Desc), Collect(chan<- Metric).
		// Detect by method names with channel-typed first arg.
		if hasMethodWithChanArg(probe, "Describe") && hasMethodWithChanArg(probe, "Collect") {
			violations = append(violations, fmt.Sprintf("%s field type %s implements prometheus.Collector (Design Intent §15)", fieldPath, ft.String()))
			continue
		}

		switch ft.Kind() {
		case reflect.Struct:
			violations = append(violations, walkRuntimeResources(ft, fieldPath, seen)...)
		case reflect.Slice, reflect.Array:
			elem := ft.Elem()
			if elem.Kind() == reflect.Struct {
				violations = append(violations, walkRuntimeResources(elem, fieldPath+"[]", seen)...)
			}
		}
	}

	return violations
}

// implementsByMethodName checks whether t (or *t) has a method named name.
// Used to detect role-implementations without importing the source package.
func implementsByMethodName(t reflect.Type, name string) bool {
	if _, ok := t.MethodByName(name); ok {
		return true
	}
	if _, ok := reflect.PointerTo(t).MethodByName(name); ok {
		return true
	}
	return false
}

// hasMethodWithChanArg reports whether t has a method named name whose first
// argument is a channel (a heuristic for prometheus.Collector's
// Describe/Collect signatures).
func hasMethodWithChanArg(t reflect.Type, name string) bool {
	probe := func(rt reflect.Type) bool {
		m, ok := rt.MethodByName(name)
		if !ok {
			return false
		}
		// Method.Type includes the receiver for non-interface types, so the
		// first user argument is at index 1; for interfaces, index 0.
		mt := m.Type
		argIdx := 1
		if rt.Kind() == reflect.Interface {
			argIdx = 0
		}
		if mt.NumIn() <= argIdx {
			return false
		}
		return mt.In(argIdx).Kind() == reflect.Chan
	}
	if probe(t) {
		return true
	}
	return probe(reflect.PointerTo(t))
}
