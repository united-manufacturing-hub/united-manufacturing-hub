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

package validator_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/validator"
)

const deriveReadingDepsSrc = `package exampleworker

type Worker struct{}

func (w *Worker) DeriveDesiredState(spec interface{}) (interface{}, error) {
	w.%s()
	return nil, nil
}
`

const collectReadingDepsSrc = `package exampleworker

type Worker struct{}

func (w *Worker) CollectObservedState(ctx interface{}, desired interface{}) (interface{}, error) {
	w.%s()
	return nil, nil
}
`

var _ = Describe("rules that detect a read of the worker's dependencies", func() {
	var baseDir string

	writeWorker := func(srcFormat, accessor string) {
		workerDir := filepath.Join(baseDir, "workers", "exampleworker")
		Expect(os.MkdirAll(workerDir, 0o755)).To(Succeed())

		workerSrc := fmt.Sprintf(srcFormat, accessor)
		Expect(os.WriteFile(filepath.Join(workerDir, "worker.go"), []byte(workerSrc), 0o644)).To(Succeed())
	}

	BeforeEach(func() {
		baseDir = GinkgoT().TempDir()
	})

	DescribeTable("PURE_DERIVE",
		func(accessor string) {
			writeWorker(deriveReadingDepsSrc, accessor)

			violations := validator.ValidateDeriveDesiredState(baseDir)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal("PURE_DERIVE"))
		},
		Entry("Deps", "Deps"),
	)

	DescribeTable("MISSING_FRAMEWORK_METRICS_COPY",
		func(accessor string) {
			writeWorker(collectReadingDepsSrc, accessor)

			violations := validator.ValidateFrameworkMetricsCopy(baseDir)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal("MISSING_FRAMEWORK_METRICS_COPY"))
		},
		Entry("Deps", "Deps"),
	)

	DescribeTable("MISSING_ACTION_HISTORY_COPY",
		func(accessor string) {
			writeWorker(collectReadingDepsSrc, accessor)

			violations := validator.ValidateActionHistoryCopy(baseDir)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal("MISSING_ACTION_HISTORY_COPY"))
		},
		Entry("Deps", "Deps"),
	)
})
