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

var _ = Describe("CPU latch ScenarioV2", func() {
	// The configworker deps key is process-global; a run that fails midway
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("registers cpu-latch in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-latch"))
	})

	It("holds the degraded verdict through a reading under the fire mark and releases it under the clear mark", func() {
		scenario, ok := examples.RegistryV2["cpu-latch"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// Machine time advances one second per reading, so the wall cost is
		// the collector's cadence and a healthy run is about a second. On a
		// machine the worker cannot read, machine time never advances, the
		// holds never finish, and this ctx is what ends the run.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     200 * time.Millisecond,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "300s").Should(BeClosed())

		// Five readings that do not coexist, checked in order out of the
		// store's own delta history. Every percentage below is one this driver
		// staged and nothing else stages, so each names the machine the verdict
		// was computed from rather than only the verdict.
		history := cpuObservationHistory(store)

		// Over the fire mark: degraded, reporting the number that fired it.
		fired := messageAfter(history, -1,
			"spent 25% of the last minute waiting for a free CPU core")
		Expect(fired).To(BeNumerically(">=", 0),
			"the machine never read degraded on pressure; readings seen: %d", len(history))

		// Back under the fire mark and over the clear mark. A signal that had
		// not fired would not fire on 15%, and this one does not let go of it
		// — which is the whole point of having two marks rather than one.
		held := messageAfter(history, fired,
			"spent 15% of the last minute waiting for a free CPU core")
		Expect(held).To(BeNumerically(">", fired),
			"the verdict did not survive a reading below the fire mark; readings seen: %d", len(history))

		// Under the clear mark: released, and the budget dashboard prints the
		// reading it released on.
		released := messageAfter(history, held, "Pressure 5% (degraded above 20%)")
		Expect(released).To(BeNumerically(">", held),
			"the verdict never let go on a recovered machine; readings seen: %d", len(history))

		// Over the fire mark again, and still reported healthy: a signal that
		// has released cannot fire again for a whole window. The dashboard
		// prints 25% against a threshold of 20% while the verdict reads
		// healthy, which is the cost of the bar rather than a rendering slip.
		barred := messageAfter(history, released, "Pressure 25% (degraded above 20%)")
		Expect(barred).To(BeNumerically(">", released),
			"the machine never sat over its fire mark while still reported healthy; readings seen: %d", len(history))

		// The bar runs out and the same machine is degraded again.
		refired := messageAfter(history, barred,
			"spent 25% of the last minute waiting for a free CPU core")
		Expect(refired).To(BeNumerically(">", barred),
			"the verdict never came back; readings seen: %d", len(history))

		// The box freezes when the driver returns, and a frozen box withholds
		// every rate rather than reporting zero, so the settled verdict is the
		// one the last condition earned.
		var observed fsmv2.Observation[fsmv2cpu.CPUStatus]
		Expect(store.LoadObservedTyped(context.Background(),
			fsmv2cpu.WorkerType, config.ChildID(fsmv2cpu.InstanceName), &observed)).To(Succeed())

		Expect(observed.Status.Verdict).To(Equal("degraded"))
		Expect(observed.Status.Message).To(ContainSubstring(
			"spent 25% of the last minute waiting for a free CPU core"))
	})
})
