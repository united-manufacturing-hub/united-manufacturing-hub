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

package examples_test

import (
	"context"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

var _ = Describe("Registry scenarios", func() {
	BeforeEach(func() {
		DeferCleanup(register.ClearGlobalDeps, configworker.WorkerTypeName)
	})

	It("registers at least one scenario", func() {
		Expect(examples.Registry).NotTo(BeEmpty(),
			"the per-scenario specs below run nothing when the registry is empty")
	})

	names := make([]string, 0, len(examples.Registry))
	for name := range examples.Registry {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		It("runs the registered scenario "+name+" from start to a clean end", func() {
			logger := deps.NewNopFSMLogger()
			store := examples.SetupStore(logger)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := examples.Run(ctx, examples.RunConfig{
				Scenario:     examples.Registry[name],
				Duration:     time.Second,
				TickInterval: 50 * time.Millisecond,
				Logger:       logger,
				Store:        store,
			})
			Expect(err).NotTo(HaveOccurred(),
				"a registered scenario must run without error")

			Eventually(result.Done, "55s").Should(BeClosed(),
				"a registered scenario must tear down once its settle window ends")

			Expect(result.Err).NotTo(HaveOccurred(),
				"a registered scenario must end with a clean RunResult")
		})
	}
})
