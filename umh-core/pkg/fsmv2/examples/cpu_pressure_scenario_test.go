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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

var _ = Describe("CPU pressure ScenarioV2", func() {
	clearConfigWorkerDepsEachSpec()

	It("registers cpu-pressure in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-pressure"))
	})

	It("carries the degraded verdict into the store after the machine's pressure crosses its fire mark", func() {
		// This story needs two holds, and the ctx budget is well over what they
		// cost. The settle window is a poll's worth: enough for the store to
		// hold a reading taken after the driver stopped, and short enough not to
		// pad the suite.
		store := runCPUScenario(cpuScenarioRun{
			Name:       "cpu-pressure",
			CtxBudget:  90 * time.Second,
			Settle:     time.Second,
			DoneWithin: 90 * time.Second,
		})

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
