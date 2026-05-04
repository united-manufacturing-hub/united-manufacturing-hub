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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	exampleslow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

var _ = Describe("ExampleslowWorker", func() {
	Describe("GetDependenciesAny", func() {
		It("returns *ExampleslowDependencies", func() {
			logger := deps.NewNopFSMLogger()
			pool := &exampleslow.DefaultConnectionPool{}
			identity := deps.Identity{ID: "test-id", Name: "test-slow"}

			worker, err := exampleslow.NewExampleslowWorker(identity, pool, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			var w fsmv2.Worker = worker
			dp, ok := w.(fsmv2.DependencyProvider)
			Expect(ok).To(BeTrue(), "worker must implement DependencyProvider")
			got := dp.GetDependenciesAny()
			_, ok = got.(*exampleslow.ExampleslowDependencies)
			Expect(ok).To(BeTrue(), "expected *ExampleslowDependencies, got %T", got)
		})
	})
})
