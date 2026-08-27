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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

var _ = Describe("CPU blind ScenarioV2", func() {
	clearConfigWorkerDepsEachSpec()

	It("registers cpu-blind in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-blind"))
	})

	It("reports a machine it cannot measure as healthy, and a cgroup it cannot read as a poll error", func() {
		// On a machine the worker can read, this story costs about two seconds
		// of wall time; the ctx budget is many times that.
		store := runCPUScenario(cpuScenarioRun{
			Name:       "cpu-blind",
			CtxBudget:  300 * time.Second,
			Settle:     200 * time.Millisecond,
			DoneWithin: 300 * time.Second,
		})

		history := cpuObservationHistory(store)

		// The machine while it can still be read. The core count and the busy
		// figure are this driver's, and they are what tie the two outages below
		// to the published machine rather than to whatever the real host was
		// doing: a host that happened to say the same thing would have to be a
		// four-core box running at 1.2 cores.
		readable := messageAfter(history, -1, "The machine is using 1.2 of 4 cores")
		Expect(readable).To(BeNumerically(">=", 0),
			"the readable machine never reached the store; readings seen: %d", len(history))
		Expect(history[readable].Verdict).To(Equal("healthy"))

		// /proc/stat gone. The poll SUCCEEDS and the machine is published
		// healthy with an admission-open verdict, which is the fail-open this
		// scenario exists to make visible rather than the intended answer. The
		// message is wrong twice: it blames a cgroup read when the file that
		// went is a host file, and it announces its own default rather than
		// refusing to answer. Pinned as it stands today; the fix belongs in
		// pkg/cpuhealth.
		blind := messageAfter(history, readable,
			"CPU monitoring unavailable: cgroup read failed. Defaulting to healthy.")
		Expect(blind).To(BeNumerically(">", readable),
			"losing the host's CPU accounting changed no message; readings seen: %d", len(history))
		Expect(history[blind].Verdict).To(Equal("healthy"))
		Expect(history[blind].Degraded).To(BeFalse())

		// cpu.stat gone as well. The sampler treats the cgroup's own accounting
		// as primary, so the read fails, the poll returns an error, and the
		// framework marks the worker degraded with that error as its reason.
		// Nothing is defaulted. The path is the one this driver staged.
		failed := reasonAfter(history, blind,
			"poll error: read /sys/fs/cgroup/cpu.stat")
		Expect(failed).To(BeNumerically(">", blind),
			"losing the cgroup's own accounting did not fail the poll; readings seen: %d", len(history))
		Expect(history[failed].Degraded).To(BeTrue())

		// A failed poll publishes the zero status, so the verdict is not
		// "healthy" and not "degraded" but ABSENT. The worker's degraded state
		// lives in the framework's own fields, and a consumer reading only the
		// verdict sees neither a judgement nor a failure.
		Expect(history[failed].Verdict).To(BeEmpty())

		// The same, settled, in the worker's current observation.
		var observed fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]
		Expect(store.LoadObservedTyped(context.Background(),
			fsmv2cpu.WorkerType, config.ChildID(fsmv2cpu.InstanceName), &observed)).To(Succeed())

		Expect(observed.Status.Degraded).To(BeTrue())
		Expect(observed.Status.Reason).To(ContainSubstring("poll error: read /sys/fs/cgroup/cpu.stat"))
		Expect(observed.Status.Result.Verdict).To(BeEmpty())
	})
})
