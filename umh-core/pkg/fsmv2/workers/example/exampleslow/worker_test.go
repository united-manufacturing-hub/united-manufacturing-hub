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

package example_slow_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	example_slow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

func TestExampleslow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exampleslow Suite")
}

var _ = Describe("ExampleslowWorker", func() {
	It("rejects a placeholder in a plain field", func() {
		identity := deps.Identity{ID: "test-id", Name: "test-slow", WorkerType: "exampleslow"}
		logger := deps.NewNopFSMLogger()

		worker, err := example_slow.NewExampleslowWorker(identity, &example_slow.DefaultConnectionPool{}, logger, nil)
		Expect(err).NotTo(HaveOccurred())

		spec := config.UserSpec{
			Config:    "state: \"{{ .x }}\"",
			Variables: config.VariableBundle{},
		}

		_, err = worker.DeriveDesiredState(spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("state"))
	})
})
