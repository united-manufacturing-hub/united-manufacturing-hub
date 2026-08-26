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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

var _ = Describe("CPU pressure ScenarioV2", func() {
	// The configworker deps key is process-global; a run that fails midway
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("registers cpu-pressure in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-pressure"))
	})

	It("carries the degraded verdict into the store after the machine's pressure crosses its fire mark", func() {
		scenario, ok := examples.RegistryV2["cpu-pressure"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// The ctx bounds the driver, which waits for the worker to read the
		// machine before it holds either condition. On a machine the worker
		// cannot read, that wait never finishes and this ctx is what ends the
		// run, so the budget is well over the two holds the story needs.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		// Duration is the settle window after the driver returns, not the
		// length of the run. One second is a poll's worth: enough for the
		// store to hold a reading taken after the driver stopped, and short
		// enough not to pad the suite.
		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     time.Second,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "90s").Should(BeClosed())

		// Read the worker's own observation back out of CSE, which is the far
		// end of the path this scenario exists to exercise: a fake machine
		// published into the deps registry, a supervisor-spawned worker, the
		// real sampler, the real engine, the collector, and the store.
		var observed fsmv2.Observation[fsmv2cpu.CPUStatus]
		Expect(store.LoadObservedTyped(context.Background(),
			fsmv2cpu.WorkerType, config.ChildID(fsmv2cpu.InstanceName), &observed)).To(Succeed())

		Expect(observed.Status.Verdict).To(Equal("degraded"))

		// The verdict alone would be satisfied by any degraded machine: a run
		// whose capacity signal fired for its own reasons, or a real host that
		// happened to be loaded if the driver's publish were removed. The
		// percentage below is the pressure this driver stages and nothing else
		// stages, so it names the machine the verdict came from.
		//
		// This is not a check on the message's wording, which the specs in
		// pkg/cpuhealth pin. It is a check on which reading the verdict was
		// computed from.
		Expect(observed.Status.Message).To(ContainSubstring(
			"spent 25% of the last minute waiting for a free CPU core"))
	})
})
