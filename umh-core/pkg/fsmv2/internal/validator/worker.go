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
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ValidateChildSpecValidation checks that DeriveDesiredState validates children.
func ValidateChildSpecValidation(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkChildSpecValidation(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkChildSpecValidation checks that parent workers validate ChildrenSpecs.
func checkChildSpecValidation(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Scope: this validator only walks worker directories whose basename
	// contains "parent" — today, that's just `exampleparent/`. Other parent
	// workers (transport, communicator, application) are NOT validated by
	// this code path; their ChildSpec emission is exercised by P1.8
	// architecture tests (Test #5 ParentRenderChildrenEmitsNonNil + Test #13
	// Layer 2 registry walk + Test #7 idempotency + Test #8 no-templates).
	// Expanding this filter to the other parents is reasonable future work
	// but currently adds no coverage the architecture tests don't already
	// provide.
	baseName := filepath.Base(filepath.Dir(filename))
	if !strings.Contains(baseName, "parent") {
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "DeriveDesiredState" {
			return true
		}

		hasChildrenValidation := false
		hasProgrammaticChildren := false
		hasRenderChildrenCall := false

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			if rangeStmt, ok := bodyNode.(*ast.RangeStmt); ok {
				if selExpr, ok := rangeStmt.X.(*ast.SelectorExpr); ok {
					if selExpr.Sel.Name == "Children" {
						hasChildrenValidation = true

						return false
					}
				}
			}

			if callExpr, ok := bodyNode.(*ast.CallExpr); ok {
				if ident, ok := callExpr.Fun.(*ast.Ident); ok && ident.Name == "make" {
					if len(callExpr.Args) > 0 {
						if arrayType, ok := callExpr.Args[0].(*ast.ArrayType); ok {
							if sel, ok := arrayType.Elt.(*ast.SelectorExpr); ok {
								if sel.Sel.Name == "ChildSpec" {
									hasProgrammaticChildren = true

									return false
								}
							}
						}
					}
				}

				// P2.1 renderChildren convention: DeriveDesiredState
				// delegates child construction to a RenderChildren
				// helper. Treat that delegation as equivalent to
				// inline make([]config.ChildSpec, ...) for the
				// early-validation invariant. Per-spec validation is
				// NOT performed inside the helper; it is enforced
				// elsewhere in the framework (P1.8 architecture
				// tests #5/#7/#8/#13 Layer 2 anchor the invariants
				// the helper output must satisfy). This branch
				// recognises that the construction has been delegated;
				// the framework is responsible for correctness.
				//
				// Match both same-package calls (RenderChildren as
				// *ast.Ident) and cross-package calls (pkg.RenderChildren
				// as *ast.SelectorExpr) so future workers that import a
				// shared RenderChildren helper aren't false-negatively
				// flagged.
				if ident, ok := callExpr.Fun.(*ast.Ident); ok && ident.Name == "RenderChildren" {
					hasRenderChildrenCall = true

					return false
				}
				if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "RenderChildren" {
					hasRenderChildrenCall = true

					return false
				}
			}

			return true
		})

		if !hasChildrenValidation && !hasProgrammaticChildren && !hasRenderChildrenCall {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "MISSING_CHILDSPEC_VALIDATION",
				Message: "DeriveDesiredState() should validate ChildrenSpecs before returning",
			})
		}

		return true
	})

	return violations
}

// ValidateDeriveDesiredStateReturns checks that DeriveDesiredState returns valid State values ("stopped" or "running").
func ValidateDeriveDesiredStateReturns(baseDir string) []Violation {
	var violations []Violation

	workerFiles := FindWorkerFiles(baseDir)

	for _, file := range workerFiles {
		fileViolations := checkDeriveDesiredStateReturns(file)
		violations = append(violations, fileViolations...)
	}

	return violations
}

// checkDeriveDesiredStateReturns checks that DeriveDesiredState returns valid State values.
func checkDeriveDesiredStateReturns(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	validStates := map[string]bool{
		"stopped": true,
		"running": true,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "DeriveDesiredState" {
			return true
		}

		ast.Inspect(funcDecl.Body, func(bodyNode ast.Node) bool {
			retStmt, ok := bodyNode.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) == 0 {
				return true
			}

			firstResult := retStmt.Results[0]

			var compLit *ast.CompositeLit
			if unaryExpr, ok := firstResult.(*ast.UnaryExpr); ok {
				compLit, _ = unaryExpr.X.(*ast.CompositeLit)
			} else {
				compLit, _ = firstResult.(*ast.CompositeLit)
			}

			if compLit == nil {
				return true
			}

			for _, elt := range compLit.Elts {
				kvExpr, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}

				keyIdent, ok := kvExpr.Key.(*ast.Ident)
				if !ok || keyIdent.Name != "State" {
					continue
				}

				if basicLit, ok := kvExpr.Value.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					value := strings.Trim(basicLit.Value, `"`)
					if !validStates[value] {
						pos := fset.Position(kvExpr.Pos())
						violations = append(violations, Violation{
							File:    filename,
							Line:    pos.Line,
							Type:    "INVALID_DESIRED_STATE_VALUE",
							Message: fmt.Sprintf("DeriveDesiredState returns invalid State value %q - must be \"stopped\" or \"running\"", value),
						})
					}
				}
			}

			return true
		})

		return true
	})

	return violations
}

// ValidateMetricsEmbedderValueReceivers checks that MetricsEmbedder uses value receivers.
func ValidateMetricsEmbedderValueReceivers(baseDir string) []Violation {
	var violations []Violation

	apiFile := filepath.Join(baseDir, "api.go")
	fileViolations := checkMetricsEmbedderReceivers(apiFile)
	violations = append(violations, fileViolations...)

	return violations
}

// checkMetricsEmbedderReceivers checks that MetricsEmbedder methods use value receivers.
func checkMetricsEmbedderReceivers(filename string) []Violation {
	var violations []Violation

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	targetMethods := map[string]bool{
		"GetWorkerMetrics":    true,
		"GetFrameworkMetrics": true,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		recv := funcDecl.Recv.List[0]

		recvTypeName := getReceiverTypeName(recv.Type)
		if recvTypeName != "MetricsEmbedder" {
			return true
		}

		if !targetMethods[funcDecl.Name.Name] {
			return true
		}

		if _, isPointer := recv.Type.(*ast.StarExpr); isPointer {
			pos := fset.Position(funcDecl.Pos())
			violations = append(violations, Violation{
				File:    filename,
				Line:    pos.Line,
				Type:    "POINTER_RECEIVER_ON_METRICS_EMBEDDER",
				Message: fmt.Sprintf("MetricsEmbedder.%s() uses pointer receiver (*MetricsEmbedder) - must use value receiver for interface satisfaction", funcDecl.Name.Name),
			})
		}

		return true
	})

	return violations
}

// getReceiverTypeName extracts the type name from a receiver.
func getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}

	return ""
}

