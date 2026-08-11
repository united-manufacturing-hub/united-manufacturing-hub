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

package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The two on-disk sections are what was last read from the file. Contracts is the
// live view. Reading the sections outside this package therefore works right up
// until something mutates a contract, and then quietly serves a stale answer -- a
// class of bug that no test would catch because every fixture is read-only.
//
// This walks the module and fails on any such read, which is cheaper than
// remembering the rule. Consumers wanting the pre-merge shape call ToLegacyConfig
// or LegacyDataModels.
var _ = Describe("the pre-merge sections stay inside pkg/config", func() {
	// Matches a read through any receiver -- cfg.DataModels, snapshot.CurrentConfig
	// .DataContracts, fullConfig.DataModels -- but not the struct tag or a field
	// declaration, which have no dot.
	leak := regexp.MustCompile(`\.(DataModels|DataContracts)\b`)

	// Files allowed to read them, and why.
	permitted := map[string]string{
		// Builds the merged view in the first place.
		"pkg/config/manager.go": "absorbs the sections into contracts",
		// Projects contracts back into the sections, and copies them.
		"pkg/config/datacontract_selfcheck.go": "projects contracts into the sections",
		"pkg/config/config.go": "declares and deep-copies them",
		// Absorbs a seeded config so a mock behaves like a parsed file.
		"pkg/config/mock.go": "absorbs a test-seeded config",
		// Deliberately emits the pre-merge shape; that is the whole command.
		"pkg/config/downgrade.go": "writes the pre-merge sections on purpose",
	}

	It("is not read outside this package", func() {
		root := ".."

		var leaks []string

		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if info.Name() == "vendor" || info.Name() == "testdata" {
					return filepath.SkipDir
				}

				return nil
			}

			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			// Normalise to a module-relative path so the permitted list reads clearly.
			relative := strings.TrimPrefix(filepath.ToSlash(path), "../")
			relative = "pkg/" + strings.TrimPrefix(relative, "")

			if _, ok := permitted[relative]; ok {
				return nil
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			for i, line := range strings.Split(string(data), "\n") {
				if !leak.MatchString(line) {
					continue
				}

				// A yaml tag or a doc comment is not a read.
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || strings.Contains(line, "yaml:\"") {
					continue
				}

				leaks = append(leaks, relative+":"+itoa(i+1)+": "+trimmed)
			}

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(leaks).To(BeEmpty(),
			"these read a pre-merge section directly; use ToLegacyConfig or "+
				"LegacyDataModels, which derive from the live contracts:\n%s",
			strings.Join(leaks, "\n"))
	})
})

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}
