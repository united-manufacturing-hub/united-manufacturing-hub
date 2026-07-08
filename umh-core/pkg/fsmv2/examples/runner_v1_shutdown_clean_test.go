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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// v1ShutdownCleanYAML declares a single helloworld worker. moodFilePath is
// omitted so mood checking is skipped and the worker stays Running.
const v1ShutdownCleanYAML = `
children:
  - name: "hello-1"
    workerType: "helloworld"
    userSpec:
      config: |
        state: running
        message: "hello"
`

// runUntilHelloworldRunning starts the given v1 scenario and blocks until the
// helloworld child is observably Running, so a real worker is resident to
// drain when the caller triggers teardown.
func runUntilHelloworldRunning(ctx context.Context, cfg examples.RunConfig) *examples.RunResult {
	GinkgoHelper()

	result, err := examples.Run(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func(g Gomega) {
		dump, err := examples.DumpScenario(context.Background(), cfg.Store, 0)
		g.Expect(err).NotTo(HaveOccurred())

		observedStates := map[string]interface{}{}
		for _, w := range dump.Workers {
			observedStates[w.WorkerType] = w.Observed["state"]
		}

		g.Expect(observedStates).To(HaveKeyWithValue("helloworld", "Running"))
	}, "30s").Should(Succeed(),
		"the helloworld worker must be running before teardown")

	return result
}

var _ = Describe("v1 YAML runner ShutdownClean", func() {
	It("reports ShutdownClean=false when the graceful drain budget is exhausted", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// A 1ns budget cannot reap even one worker within a tick, so the drain
		// must warn graceful_shutdown_timeout and DrainOutcomeClean must be
		// false. This is the load-bearing assertion: ShutdownClean must reflect
		// the real drain outcome, not a hard-coded true.
		result := runUntilHelloworldRunning(ctx, examples.RunConfig{
			Scenario: examples.Scenario{
				Name:        "v1-degraded-drain",
				Description: "test-local YAML scenario for the degraded-drain ShutdownClean path",
				YAMLConfig:  v1ShutdownCleanYAML,
			},
			TickInterval:            50 * time.Millisecond,
			GracefulShutdownTimeout: time.Nanosecond,
			Logger:                  logger,
			Store:                   store,
		})

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed())

		Expect(result.ShutdownClean).To(BeFalse(),
			"a v1 run whose graceful drain budget is exhausted must report ShutdownClean=false")
	})

	It("reports ShutdownClean=true after a clean v1 drain", func() {
		logger := deps.NewNopFSMLogger()
		store := examples.SetupStore(logger)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The default budget (5s) reaps a single helloworld worker with room to
		// spare, so the drain is warn-free and ShutdownClean must be true.
		result := runUntilHelloworldRunning(ctx, examples.RunConfig{
			Scenario: examples.Scenario{
				Name:        "v1-clean-drain",
				Description: "test-local YAML scenario for the clean-drain ShutdownClean path",
				YAMLConfig:  v1ShutdownCleanYAML,
			},
			TickInterval: 50 * time.Millisecond,
			Logger:       logger,
			Store:        store,
		})

		cancel()
		Eventually(result.Done, "55s").Should(BeClosed())

		Expect(result.ShutdownClean).To(BeTrue(),
			"a clean v1 drain must report ShutdownClean=true")
	})
})
