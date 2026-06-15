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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"
)

// workerSrc is a minimal parent worker.go with a DeriveDesiredState that
// contains no children — neither a range-over-Children loop nor make([]ChildSpec).
const workerSrc = `package exampleparent

type ParentWorker struct{}

func (w *ParentWorker) DeriveDesiredState(spec interface{}) (interface{}, error) {
	return nil, nil
}
`

// childrenSrc is a minimal children.go that declares a top-level RenderChildren
// function — the new declarative-children pattern.
const childrenSrc = `package exampleparent

func RenderChildren(cfg interface{}, enabled bool) ([]interface{}, error) {
	return nil, nil
}
`

var _ = Describe("ValidateChildSpecValidation — MISSING_CHILDSPEC_VALIDATION", func() {

	var baseDir string

	BeforeEach(func() {
		var err error
		baseDir, err = os.MkdirTemp("", "validator_childspec_test_*")
		Expect(err).NotTo(HaveOccurred())

		// FindWorkerFiles walks baseDir/workers/, so create the expected layout.
		workerDir := filepath.Join(baseDir, "workers", "exampleparent")
		Expect(os.MkdirAll(workerDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(workerDir, "worker.go"), []byte(workerSrc), 0o644)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(baseDir)).To(Succeed())
	})

	It("reports no violation when RenderChildren is present in the worker directory", func() {
		workerDir := filepath.Join(baseDir, "workers", "exampleparent")
		Expect(os.WriteFile(filepath.Join(workerDir, "children.go"), []byte(childrenSrc), 0o644)).To(Succeed())

		violations := validator.ValidateChildSpecValidation(baseDir)
		Expect(violations).To(BeEmpty(), "expected no MISSING_CHILDSPEC_VALIDATION when RenderChildren is declared")
	})

	It("reports a violation when neither DeriveDesiredState children nor RenderChildren exist", func() {
		violations := validator.ValidateChildSpecValidation(baseDir)
		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Type).To(Equal("MISSING_CHILDSPEC_VALIDATION"))
	})
})
