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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

var _ = Describe("CPU host ScenarioV2", func() {
	It("registers cpu-host in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-host"))
	})

	// Label("live") keeps this spec out of CI, because make unit-test filters
	// live specs out: what this spec asserts depends on the machine it runs
	// on, and nobody picks the CI machine. Plain go test applies no filter,
	// so the spec still runs on every developer machine.
	It("refuses only where the host publishes no cgroup v2 CPU files, naming the tool that provides them", Label("live"), func() {
		scenario, ok := examples.LiveRegistryV2["cpu-host"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// This spec asserts what the machine it runs on requires, so it asks
		// that machine the same question the driver asks: do the cgroup v2
		// CPU accounting files exist? Without them every reading says CPU
		// monitoring is unavailable, so there is nothing to watch and the
		// driver refuses. Only one arm can run per machine: macOS and
		// cgroup v1 hosts take the refusal, a cgroup v2 host the proceed.
		// Neither arm skips, so a developer always sees the arm their
		// machine exercises.
		_, statErr := os.Stat("/sys/fs/cgroup/cpu.stat")

		// The refusal arm needs almost none of this budget: the driver checks
		// the file before it upserts anything. The proceed arm needs the
		// worker to take and publish one reading, which is a handful of its
		// one-second polls, plus the settle window.
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   scenario,
			Duration:     time.Second,
			TickInterval: 100 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})

		if statErr != nil {
			// The driver's error returns synchronously and teardown has
			// already finished by the time Run reports it, so there is no
			// Done channel to wait for. The message has to name the tool,
			// because a developer on a Mac has no other way to learn that
			// the readings exist only inside a Linux container.
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tools/cpu-host"))

			return
		}

		// The readable arm: the refusal must not fire here. The run may still
		// fail for its own reasons: a machine whose cpu.stat exists but is
		// unreadable serves the worker nothing but unavailable readings, and
		// ends the run on the context deadline.
		if err != nil {
			Expect(err.Error()).NotTo(ContainSubstring("tools/cpu-host"))

			return
		}

		// A run that started must finish before the spec ends, or the
		// supervisor it left running outlives the spec.
		Eventually(result.Done, "120s").Should(BeClosed())
	})
})
