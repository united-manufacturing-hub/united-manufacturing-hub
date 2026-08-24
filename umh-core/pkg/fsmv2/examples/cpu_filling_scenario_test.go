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

var _ = Describe("CPU filling ScenarioV2", func() {
	// The configworker deps key is process-global; a run that fails midway
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("registers cpu-filling in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-filling"))
	})

	It("moves the remedy from the other software on the machine to this instance's own load", func() {
		scenario, ok := examples.RegistryV2["cpu-filling"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// The ctx bounds the driver, which waits for the worker's first
		// reading and then holds three conditions for 150 seconds of machine
		// time between them. Machine time advances one second per reading, so
		// the wall cost is the collector's cadence and a healthy run is a
		// couple of seconds; the budget is many times that. On a machine the
		// worker cannot read, machine time never advances, the holds never
		// finish, and this ctx is what ends the run.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		// Duration is the settle window after the driver returns, not the
		// length of the run. The box is frozen through it, so it buys readings
		// taken after the story ended rather than more of the story.
		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     200 * time.Millisecond,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(result.Done, "300s").Should(BeClosed())

		// The three readings this scenario turns on do not coexist: each
		// replaces the one before it, so only the last survives in the
		// worker's current observation. cpuObservationHistory replays the
		// store's own delta history, and searching it forward is what makes
		// the ORDER part of the claim — a run that produced the last sentence
		// first would fail here.
		history := cpuObservationHistory(store)

		// The quiet opening, which names the CPU limit this driver staged and
		// this instance's usage against it. Nothing else in the run says "of 3
		// cores", and a host the suite happened to run on would have to be
		// under a three-core limit at half a core of usage to say it, so this
		// is the reading that ties the story to the published machine rather
		// than to whatever the real host was doing.
		quiet := messageAfter(history, -1,
			"This instance is using 0.5 of 3 cores (17% of its limit)")
		Expect(quiet).To(BeNumerically(">=", 0),
			"the quiet condition never reached the store; readings seen: %d", len(history))

		// The machine fills from outside. "Other software" is the remedy only
		// a machine filled by somebody else earns.
		host := messageAfter(history, quiet, "reduce other software running on it")
		Expect(host).To(BeNumerically(">", quiet),
			"the machine never read full with the blame outside this instance; readings seen: %d", len(history))

		// The same machine, equally full, filled by this instance. The remedy
		// has to change: sending this customer after somebody else's load
		// would send them after nothing.
		ours := messageAfter(history, host, "this instance is using most of it")
		Expect(ours).To(BeNumerically(">", host),
			"the blame never moved to this instance; readings seen: %d", len(history))

		// The box freezes when the driver returns, and a frozen box withholds
		// every rate rather than reporting zero, so the settled verdict is the
		// one the last condition earned.
		var observed fsmv2.Observation[fsmv2cpu.CPUStatus]
		Expect(store.LoadObservedTyped(context.Background(),
			fsmv2cpu.WorkerType, config.ChildID(fsmv2cpu.InstanceName), &observed)).To(Succeed())

		Expect(observed.Status.Verdict).To(Equal("degraded"))
		Expect(observed.Status.Message).To(ContainSubstring("this instance is using most of it"))
	})
})
