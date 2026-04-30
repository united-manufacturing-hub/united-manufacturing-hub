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
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// FindStateFiles discovers all state files in the workers directory.
func FindStateFiles(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() &&
			strings.HasPrefix(filepath.Base(path), "state_") &&
			!strings.HasSuffix(path, "_test.go") &&
			!strings.HasSuffix(path, "base.go") &&
			strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}

		return nil
	})

	return files
}

// FindActionFiles discovers all action files in the workers directory.
func FindActionFiles(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() &&
			strings.Contains(path, "/action/") &&
			!strings.HasSuffix(path, "_test.go") &&
			strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}

		return nil
	})

	return files
}

// FindWorkerFiles discovers all worker.go files in the workers directory.
func FindWorkerFiles(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() &&
			filepath.Base(path) == "worker.go" {
			files = append(files, path)
		}

		return nil
	})

	return files
}

// IsNewAPIWorkerFile checks if a worker.go file uses the Worker API v2
// by scanning for a struct that embeds fsmv2.WorkerBase[...].
func IsNewAPIWorkerFile(filename string) bool {
	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return false
	}

	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}

		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) != 0 {
				continue
			}

			if containsWorkerBase(field.Type) {
				found = true

				return false
			}
		}

		return true
	})

	return found
}

// containsWorkerBase checks if an AST expression refers to WorkerBase.
// WorkerBase[TConfig, TStatus] appears as IndexListExpr or IndexExpr in the AST.
func containsWorkerBase(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.IndexListExpr:
		return identifierContains(t.X, "WorkerBase")
	case *ast.IndexExpr:
		return identifierContains(t.X, "WorkerBase")
	}

	return false
}

// identifierContains checks if an expression ends with the given name.
func identifierContains(expr ast.Expr, name string) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name == name
	case *ast.Ident:
		return t.Name == name
	}

	return false
}

// IsNewAPIWorkerDir checks if a worker directory uses the Worker API v2
// by scanning the worker.go file for WorkerBase embed.
func IsNewAPIWorkerDir(workerDir string) bool {
	workerFile := filepath.Join(workerDir, "worker.go")
	if _, err := os.Stat(workerFile); err != nil {
		return false
	}

	return IsNewAPIWorkerFile(workerFile)
}

// FindSnapshotFiles discovers all snapshot.go files in the workers directory.
func FindSnapshotFiles(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() &&
			filepath.Base(path) == "snapshot.go" {
			files = append(files, path)
		}

		return nil
	})

	return files
}
