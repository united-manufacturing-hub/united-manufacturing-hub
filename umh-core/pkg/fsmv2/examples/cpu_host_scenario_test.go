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
	// live specs out. Which branch this spec takes depends on whether the machine
	// publishes Linux CPU files, and CI does not control that. Plain go test
	// applies no filter, so the spec still runs on every developer machine.
	It("refuses only where the host publishes no Linux CPU files, naming the tool that provides them", Label("live"), func() {
		scenario, ok := examples.LiveRegistryV2["cpu-host"]
		Expect(ok).To(BeTrue())

		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		_, statErr := os.Stat("/proc/stat")

		// The budget is for the proceed arm: a handful of one-second polls,
		// then teardown. The refusal returns before anything is upserted.
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
			// The scenario's error returns synchronously, after teardown, so
			// there is no Done channel to wait for.
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tools/cpu-host"))

			return
		}

		// The readable arm: the refusal must not fire here. The run may still
		// fail for its own reasons, so only the refusal's absence is asserted.
		if err != nil {
			Expect(err.Error()).NotTo(ContainSubstring("tools/cpu-host"))

			return
		}

		// A run that started must finish before the spec ends, or the
		// supervisor it left running outlives the spec.
		Eventually(result.Done, "120s").Should(BeClosed())
	})
})
