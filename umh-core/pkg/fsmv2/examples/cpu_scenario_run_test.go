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

// The setup the CPU scenario specs beside this file share: register the
// teardown their process-global deps key needs, run one v2 scenario to
// completion, and hand back the store it wrote into. Every assertion stays in
// the spec that makes it; only setup, run and teardown live here.

package examples_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
)

// cpuTickInterval is how often the collector runs during these scenarios. The
// CPU specs all use the same one: it sets the wall cost of a run and nothing
// they assert on.
const cpuTickInterval = 100 * time.Millisecond

// clearConfigWorkerDepsEachSpec registers the teardown each CPU scenario needs.
// Call it from the Describe body.
//
// The configworker deps key is process-global; a run that fails midway would
// otherwise leak it into every later spec in this process.
func clearConfigWorkerDepsEachSpec() {
	BeforeEach(func() {
		DeferCleanup(func() {
			register.ClearDeps(configworker.WorkerTypeName)
		})
	})
}

// cpuScenarioRun is the time budget one CPU scenario spec runs under. Each spec
// stages a different story, so every field below differs between specs and none
// of them carries a default.
type cpuScenarioRun struct {
	// Name is the key the scenario registered under in examples.RegistryV2.
	Name string

	// CtxBudget bounds the driver. The driver waits for the worker's first
	// reading of the machine before it holds any condition, and machine time
	// advances one second per reading. On a machine the worker cannot read,
	// machine time never advances, the holds never finish, and this budget is
	// what ends the run. Each spec says what its own story costs on a machine
	// that can be read.
	CtxBudget time.Duration

	// Settle is examples.RunConfig.Duration: the window after the driver
	// returns, not the length of the run. It buys readings taken after the
	// story ended rather than more of the story.
	Settle time.Duration

	// DoneWithin bounds the wait for the run to finish. A window shorter than
	// CtxBudget would report a failure the ctx was about to end anyway.
	DoneWithin time.Duration
}

// runCPUScenario runs one v2 CPU scenario to completion and returns the store it
// wrote into.
//
// A caller reads either end of the run out of that store: LoadObservedTyped for
// the observation the run settled on, cpuObservationHistory for the sequence of
// readings it passed through.
func runCPUScenario(run cpuScenarioRun) storage.TriangularStoreInterface {
	GinkgoHelper()

	scenario, ok := examples.RegistryV2[run.Name]
	Expect(ok).To(BeTrue())

	logger := deps.NewNopFSMLogger()
	store := examples.SetupStore(logger)

	// DeferCleanup rather than defer: the ctx has to outlive this function,
	// because the caller reads the store after it returns.
	ctx, cancel := context.WithTimeout(context.Background(), run.CtxBudget)
	DeferCleanup(cancel)

	result, err := examples.Run(ctx, examples.RunConfig{
		ScenarioV2:   scenario,
		Duration:     run.Settle,
		TickInterval: cpuTickInterval,
		Logger:       logger,
		Store:        store,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(result.Done, run.DoneWithin).Should(BeClosed())

	return store
}
