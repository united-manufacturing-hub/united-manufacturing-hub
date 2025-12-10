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

package supervisor_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Phase 2: Lock Documentation Tests (RED Phase)
//
// These tests verify that lock usage is properly documented and that lock
// order violations will be detected at runtime. They are written BEFORE
// implementing the documentation and assertions (Test-Driven Development).
//
// Test Strategy:
// 1. Parse supervisor.go source code
// 2. Find all sync.RWMutex and sync.Mutex declarations
// 3. Verify godoc comments exist and describe what each lock protects
// 4. Verify lock acquisition order is documented
// 5. Verify runtime assertions for lock order violations exist
//
// Why this approach:
// - Tests verify documentation EXISTS (not just "code has comments")
// - Tests enforce specific documentation standards
// - Tests can validate lock order rules programmatically
// - Failing tests clearly show what documentation is missing

var _ = Describe("Lock Documentation", func() {
	var (
		fset       *token.FileSet
		file       *ast.File
		typesFile  *ast.File
		parseError error
	)

	BeforeEach(func() {
		// Parse supervisor.go to analyze locks
		fset = token.NewFileSet()
		file, parseError = parser.ParseFile(fset, "supervisor.go", nil, parser.ParseComments)
		Expect(parseError).NotTo(HaveOccurred(), "Failed to parse supervisor.go")
		// Parse types.go for WorkerContext
		typesFile, parseError = parser.ParseFile(fset, "types.go", nil, parser.ParseComments)
		Expect(parseError).NotTo(HaveOccurred(), "Failed to parse types.go")
	})

	Describe("Supervisor.mu Lock", func() {
		It("should have godoc comment describing what it protects", func() {
			// Find the Supervisor struct declaration
			var supervisorStruct *ast.StructType
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "Supervisor" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							supervisorStruct = structType

							return false // Stop searching
						}
					}
				}

				return true
			})

			Expect(supervisorStruct).NotTo(BeNil(), "Supervisor struct not found")

			// Find the mu field
			var muField *ast.Field
			var muFieldDoc string
			for _, field := range supervisorStruct.Fields.List {
				for _, name := range field.Names {
					if name.Name == "mu" {
						muField = field
						if field.Doc != nil {
							muFieldDoc = field.Doc.Text()
						}

						break
					}
				}
			}

			Expect(muField).NotTo(BeNil(), "Supervisor.mu field not found")

			// WILL FAIL: No documentation exists yet
			Expect(muFieldDoc).NotTo(BeEmpty(), "Supervisor.mu has no godoc comment")
			Expect(muFieldDoc).To(ContainSubstring("Protects"), "Supervisor.mu godoc should describe what it protects")

			// Should specify WHAT it protects
			protectedFields := []string{"workers", "expectedObservedTypes", "expectedDesiredTypes", "children"}
			foundProtections := 0
			for _, field := range protectedFields {
				if strings.Contains(muFieldDoc, field) {
					foundProtections++
				}
			}
			Expect(foundProtections).To(BeNumerically(">", 0),
				"Supervisor.mu godoc should specify which fields it protects (workers, expectedObservedTypes, etc.)")
		})

		It("should document that it's a read-write lock for performance", func() {
			// Find mu field and its documentation
			var muFieldDoc string
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "Supervisor" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									if name.Name == "mu" && field.Doc != nil {
										muFieldDoc = field.Doc.Text()

										return false
									}
								}
							}
						}
					}
				}

				return true
			})

			// WILL FAIL: Documentation doesn't explain RWMutex choice
			Expect(muFieldDoc).To(MatchRegexp(`(?i)(read|write|RW|concurrent|performance)`),
				"Supervisor.mu should explain why RWMutex is used (concurrent reads)")
		})
	})

	Describe("Supervisor.ctxMu Lock", func() {
		It("should have godoc comment describing what it protects", func() {
			var ctxMuFieldDoc string
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "Supervisor" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									if name.Name == "ctxMu" && field.Doc != nil {
										ctxMuFieldDoc = field.Doc.Text()

										return false
									}
								}
							}
						}
					}
				}

				return true
			})

			// WILL FAIL: No documentation exists yet
			Expect(ctxMuFieldDoc).NotTo(BeEmpty(), "Supervisor.ctxMu has no godoc comment")
			Expect(ctxMuFieldDoc).To(ContainSubstring("ctx"), "Supervisor.ctxMu should mention it protects ctx")
			Expect(ctxMuFieldDoc).To(ContainSubstring("cancel"), "Supervisor.ctxMu should mention it protects ctxCancel")
		})

		It("should document its purpose for preventing TOCTOU races", func() {
			var ctxMuFieldDoc string
			ast.Inspect(file, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "Supervisor" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									if name.Name == "ctxMu" && field.Doc != nil {
										ctxMuFieldDoc = field.Doc.Text()

										return false
									}
								}
							}
						}
					}
				}

				return true
			})

			// WILL FAIL: Documentation doesn't explain TOCTOU prevention
			Expect(ctxMuFieldDoc).To(MatchRegexp(`(?i)(TOCTOU|race|atomic|shutdown)`),
				"Supervisor.ctxMu should explain its role in preventing TOCTOU races during shutdown")
		})
	})

	Describe("WorkerContext.mu Lock", func() {
		It("should have godoc comment describing what it protects", func() {
			var workerCtxFieldDoc string
			ast.Inspect(typesFile, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "WorkerContext" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									if name.Name == "mu" && field.Doc != nil {
										workerCtxFieldDoc = field.Doc.Text()

										return false
									}
								}
							}
						}
					}
				}

				return true
			})

			// WorkerContext.mu documentation should describe what it protects
			Expect(workerCtxFieldDoc).NotTo(BeEmpty(), "WorkerContext.mu has no godoc comment")
			Expect(workerCtxFieldDoc).To(ContainSubstring("currentState"),
				"WorkerContext.mu should mention it protects currentState")
		})

		It("should document that it's independent from Supervisor.mu", func() {
			var workerCtxFieldDoc string
			ast.Inspect(typesFile, func(n ast.Node) bool {
				if typeSpec, ok := n.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == "WorkerContext" {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								for _, name := range field.Names {
									if name.Name == "mu" && field.Doc != nil {
										workerCtxFieldDoc = field.Doc.Text()

										return false
									}
								}
							}
						}
					}
				}

				return true
			})

			// WorkerContext.mu documentation should explain lock independence
			Expect(workerCtxFieldDoc).To(MatchRegexp(`(?i)(independent|per-worker|separate)`),
				"WorkerContext.mu should explain that it's independent from Supervisor.mu")
		})
	})

	Describe("Lock Acquisition Order", func() {
		It("should have a documented lock acquisition order at package level", func() {
			// Look for a package-level comment documenting lock order
			var lockOrderDoc string
			for _, commentGroup := range file.Comments {
				commentText := commentGroup.Text()
				if strings.Contains(commentText, "LOCK ORDER") ||
					strings.Contains(commentText, "Lock Order") ||
					strings.Contains(commentText, "Lock Acquisition Order") {
					lockOrderDoc = commentText

					break
				}
			}

			// WILL FAIL: No lock order documentation exists yet
			Expect(lockOrderDoc).NotTo(BeEmpty(), "Package should have a LOCK ORDER section documenting acquisition order")
		})

		It("should document the rule: Supervisor.mu before WorkerContext.mu", func() {
			// Find lock order documentation
			var lockOrderDoc string
			for _, commentGroup := range file.Comments {
				commentText := commentGroup.Text()
				if strings.Contains(commentText, "LOCK ORDER") ||
					strings.Contains(commentText, "Lock Order") {
					lockOrderDoc = commentText

					break
				}
			}

			// WILL FAIL: Lock order rules not documented
			Expect(lockOrderDoc).To(ContainSubstring("Supervisor.mu"),
				"Lock order documentation should mention Supervisor.mu")
			Expect(lockOrderDoc).To(ContainSubstring("WorkerContext.mu"),
				"Lock order documentation should mention WorkerContext.mu")

			// Check that the order is specified clearly
			supervisorMuPos := strings.Index(lockOrderDoc, "Supervisor.mu")
			workerCtxMuPos := strings.Index(lockOrderDoc, "WorkerContext.mu")

			if supervisorMuPos != -1 && workerCtxMuPos != -1 {
				Expect(supervisorMuPos).To(BeNumerically("<", workerCtxMuPos),
					"Lock order should specify Supervisor.mu is acquired BEFORE WorkerContext.mu")
			}
		})

		It("should document the rule: Supervisor.mu before Supervisor.ctxMu", func() {
			// Find lock order documentation
			var lockOrderDoc string
			for _, commentGroup := range file.Comments {
				commentText := commentGroup.Text()
				if strings.Contains(commentText, "LOCK ORDER") ||
					strings.Contains(commentText, "Lock Order") {
					lockOrderDoc = commentText

					break
				}
			}

			// WILL FAIL: ctxMu ordering not documented
			Expect(lockOrderDoc).To(ContainSubstring("ctxMu"),
				"Lock order documentation should mention Supervisor.ctxMu")

			// Check independence (ctxMu can be acquired without mu)
			Expect(lockOrderDoc).To(MatchRegexp(`(?i)ctxMu.*independent|ctxMu.*separate`),
				"Lock order should specify that ctxMu is independent and can be acquired alone")
		})

		It("should document that WorkerContext.mu locks are independent per worker", func() {
			// Find lock order documentation
			var lockOrderDoc string
			for _, commentGroup := range file.Comments {
				commentText := commentGroup.Text()
				if strings.Contains(commentText, "LOCK ORDER") ||
					strings.Contains(commentText, "Lock Order") {
					lockOrderDoc = commentText

					break
				}
			}

			// WILL FAIL: Per-worker independence not documented
			Expect(lockOrderDoc).To(MatchRegexp(`(?i)WorkerContext.*independent|per-worker.*separate`),
				"Lock order should explain that WorkerContext.mu locks are independent per worker")
		})
	})

	Describe("Lock Order Runtime Assertions", func() {
		It("should have an assertion function for verifying lock order", func() {
			// Look for a function like checkLockOrder() or assertLockOrder() in lockmanager
			lockManagerFset := token.NewFileSet()
			lockManagerFile, err := parser.ParseFile(lockManagerFset, "lockmanager/lockmanager.go", nil, parser.ParseComments)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse lockmanager/lockmanager.go")

			var hasLockOrderAssertion bool
			ast.Inspect(lockManagerFile, func(n ast.Node) bool {
				if funcDecl, ok := n.(*ast.FuncDecl); ok {
					funcName := funcDecl.Name.Name
					if strings.Contains(funcName, "LockOrder") ||
						strings.Contains(funcName, "lockOrder") {
						hasLockOrderAssertion = true

						return false
					}
				}

				return true
			})

			// WILL FAIL: No lock order assertion function exists yet
			Expect(hasLockOrderAssertion).To(BeTrue(),
				"Should have a function for asserting lock acquisition order (e.g., checkLockOrder)")
		})

		It("should panic if locks are acquired in wrong order", func() {
			Skip("This test will be implemented once runtime assertions are added")
			// This test will verify that the runtime assertion actually panics
			// when locks are acquired out of order.
			//
			// Example test structure:
			// - Create test scenario that violates lock order
			// - Verify it panics with specific error message
			// - Verify error message mentions which lock order was violated
		})
	})

	Describe("Lock Documentation Examples", func() {
		It("should provide example of good lock documentation format", func() {
			// This test documents what good lock documentation looks like
			expectedFormat := `
// mu protects access to workers map, expectedObservedTypes, expectedDesiredTypes,
// children, childDoneChans, globalVars, and mappedParentState.
//
// This is a sync.RWMutex to allow concurrent reads from multiple goroutines
// while ensuring exclusive writes when modifying worker registry state.
//
// Lock Order: Must be acquired BEFORE WorkerContext.mu when both are needed.
// Can be acquired independently when only accessing Supervisor state.
mu sync.RWMutex
`
			_ = expectedFormat // Document the expected format

			// This test intentionally "passes" to document the format
			Expect(true).To(BeTrue(),
				"Good lock documentation should follow the format above")
		})

		It("should document deadlock risk scenarios", func() {
			Skip("This test will pass once we add deadlock risk documentation")
			// Will verify that common deadlock scenarios are documented:
			// - Holding Supervisor.mu while calling child.Shutdown()
			// - Holding WorkerContext.mu while acquiring Supervisor.mu
			// - Recursive lock acquisition patterns
		})
	})
})

var _ = Describe("Lock Analysis Report", func() {
	It("should generate a comprehensive lock analysis for documentation", func() {
		// This test generates a report of all locks found in the codebase
		// to aid in documentation efforts.

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "supervisor.go", nil, parser.ParseComments)
		Expect(err).NotTo(HaveOccurred())

		locks := []string{}

		ast.Inspect(file, func(n ast.Node) bool {
			if typeSpec, ok := n.(*ast.TypeSpec); ok {
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					structName := typeSpec.Name.Name
					for _, field := range structType.Fields.List {
						// Check for *lockmanager.Lock
						if starExpr, ok := field.Type.(*ast.StarExpr); ok {
							if selectorExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
								if pkg, ok := selectorExpr.X.(*ast.Ident); ok {
									if pkg.Name == "lockmanager" && selectorExpr.Sel.Name == "Lock" {
										for _, name := range field.Names {
											lockInfo := structName + "." + name.Name + " (*lockmanager.Lock)"
											locks = append(locks, lockInfo)
										}
									}
								}
							}
						}
						// Check for sync.RWMutex or sync.Mutex (fallback)
						if fieldType, ok := field.Type.(*ast.SelectorExpr); ok {
							if pkg, ok := fieldType.X.(*ast.Ident); ok {
								if pkg.Name == "sync" {
									typeName := fieldType.Sel.Name
									if typeName == "RWMutex" || typeName == "Mutex" {
										for _, name := range field.Names {
											lockInfo := structName + "." + name.Name + " (sync." + typeName + ")"
											locks = append(locks, lockInfo)
										}
									}
								}
							}
						}
					}
				}
			}

			return true
		})

		// Generate report (will be visible in test output)
		GinkgoWriter.Printf("\n=== Lock Analysis Report ===\n")
		GinkgoWriter.Printf("Found %d locks in supervisor.go:\n", len(locks))
		for i, lock := range locks {
			GinkgoWriter.Printf("%d. %s\n", i+1, lock)
		}
		GinkgoWriter.Printf("============================\n\n")

		// This is an informational test - always passes
		Expect(locks).ToNot(BeEmpty(), "Should find at least one lock")
	})
})

// NOTE: Runtime lock order assertion tests are in lock_order_test.go
// (internal package) because they need direct access to Supervisor
// and WorkerContext types.
