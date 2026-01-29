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

		if strings.Contains(path, "workers/communicator/") {
			return nil
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

		if strings.Contains(path, "workers/communicator/") {
			return nil
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

		if strings.Contains(path, "workers/communicator/") {
			return nil
		}

		if !info.IsDir() &&
			filepath.Base(path) == "worker.go" {
			files = append(files, path)
		}

		return nil
	})

	return files
}

// FindSnapshotFiles discovers all snapshot.go files in the workers directory.
func FindSnapshotFiles(baseDir string) []string {
	var files []string

	_ = filepath.Walk(filepath.Join(baseDir, "workers"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.Contains(path, "workers/communicator/") {
			return nil
		}

		if !info.IsDir() &&
			filepath.Base(path) == "snapshot.go" {
			files = append(files, path)
		}

		return nil
	})

	return files
}
