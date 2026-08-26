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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

var _ = Describe("CPU host ScenarioV2", func() {
	// The configworker deps key is process-global; a run that fails midway
	// would otherwise leak it into every later spec in this process.
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})

	It("registers cpu-host in the merged listing the CLI reads", func() {
		Expect(examples.ListScenarios()).To(HaveKey("cpu-host"))
	})

	It("refuses only where the host publishes no cgroup v2 CPU files, naming the tool that provides them", func() {
		scenario, ok := examples.RegistryV2["cpu-host"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		// This spec asserts what the machine it runs on requires, so it asks
		// that machine the same question the driver asks: does the cgroup v2
		// CPU accounting file exist? cpu.stat is the sampler's primary file,
		// and a machine without it can never produce a reading, because every
		// poll fails its read. Only one arm can run per machine — macOS and
		// cgroup v1 hosts take the refusal, a cgroup v2 host the proceed — and
		// neither arm skips, so a developer always sees the arm their machine
		// exercises.
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
		// fail for its own reasons — a machine whose cpu.stat exists but is
		// unreadable produces no reading either, and ends the run on the
		// context deadline — but no failure may claim the machine publishes
		// no cgroup v2 CPU files, because this one does.
		if err != nil {
			Expect(err.Error()).NotTo(ContainSubstring("tools/cpu-host"))
			return
		}

		// A run that started must finish before the spec ends, or the
		// supervisor it left running holds the process-global deps key and
		// every later scenario run in this process refuses to start.
		Eventually(result.Done, "120s").Should(BeClosed())
	})
})
