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

package exampleflow_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow"
)

var _ = Describe("ExampleflowWorker", func() {
	var (
		logger   deps.FSMLogger
		identity deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		identity = deps.Identity{ID: "flow", Name: "Flow", WorkerType: exampleflow.WorkerTypeName}
	})

	It("renders the flow template with the names a parent hands down", func() {
		w, err := exampleflow.NewExampleflowWorker(identity, logger, nil)
		Expect(err).NotTo(HaveOccurred())

		desired, err := w.DeriveDesiredState(config.UserSpec{
			Config: `flow: "{{ .a }}-x"`,
			Names:  map[string]any{"a": "1"},
		})
		Expect(err).NotTo(HaveOccurred())

		wds, ok := desired.(*fsmv2.WrappedDesiredState[exampleflow.ExampleflowConfig])
		Expect(ok).To(BeTrue())
		Expect(string(wds.Config.Flow)).To(Equal("1-x"))
	})

	It("errors naming the flow field when a template name is missing", func() {
		w, err := exampleflow.NewExampleflowWorker(identity, logger, nil)
		Expect(err).NotTo(HaveOccurred())

		_, err = w.DeriveDesiredState(config.UserSpec{
			Config: `flow: "{{ .missing }}"`,
			Names:  map[string]any{},
		})
		Expect(err).To(HaveOccurred())

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("flow"))
	})
})
