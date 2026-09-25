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

package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/integration"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

	// Blank-import the state packages so their init() registrations exist before
	// the supervisor ticks.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("Dynamic ScenarioV2: migration-API lifecycle real proof", func() {
	const configWorkerKey = "configworker"

	AfterEach(func() {
		// The configworker deps key is process-global; clear it so a failed run
		// does not leak the registry into later integration specs.
		register.ClearGlobalDeps(configWorkerKey)
	})

	It("drives one helloworld child through create->Running, update->observed-change, then Delete while the kernel survives", func() {
		// The dynamic scenario must be registered beside noop and reachable
		// through the same merged listing the CLI reads. A missing entry here is
		// the first thing this rung adds.
		listing := examples.ListScenarios()
		Expect(listing).To(HaveKey("dynamic"),
			"merged ListScenarios must contain the v2 dynamic scenario")

		dynamic, ok := examples.RegistryV2["dynamic"]
		Expect(ok).To(BeTrue(),
			"RegistryV2 must register the dynamic scenario beside noop")
		Expect(dynamic.Run).NotTo(BeNil(),
			"the dynamic scenario must carry a Run that exercises the migration-API client")

		testLogger := integration.NewTestLogger()
		defer testLogger.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		store := setupTestStoreForScenario(testLogger.FSMLogger)

		// Run the real dynamic scenario against a live kernel-only supervisor. The
		// runner builds an fsmv2client over this same store, so Run reads
		// observed state through the same store this battery inspects afterward.
		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   dynamic,
			Duration:     2 * time.Second,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.FSMLogger,
			Store:        store,
		})

		// CREATE + UPDATE PROOF (load-bearing): Run returned nil. The Run
		// returns nil only after it has read, from the run's store through
		// fsmv2client.Get against the live child, BOTH the create->Running state and
		// the update's new mood. Each leg in runDynamicHello is a poll that loops
		// until the value is observed in the store, surfacing every error except
		// ErrNotObserved and honoring ctx. So this nil return is the create->update
		// migration-API proof: a runtime Upsert of a real config field (a new
		// moodFilePath) reached a live child and its new value became observable.
		//
		// We do not re-read the final persisted mood from the store here. After
		// Run's Delete, nothing reaps the child, so the supervisor keeps ticking
		// it through teardown; once Run's temp mood files are removed (the
		// leak fix cleans them on Run return), the worker's CollectObservedState
		// re-reads a now-missing file and overwrites the observed mood with "". That
		// post-despawn stale observation is the ENG-5107 signal: the store-side reap
		// (stop ticking + tombstone on Delete) is what makes a stable final-doc read
		// possible, and ENG-5107 builds it. This rung adds no storage or supervisor
		// code, so it proves the update at Run's own observation point.
		Expect(err).NotTo(HaveOccurred(),
			"the dynamic scenario must observe create->Running and update->changed-mood through the migration-API client, then Delete, without error")
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the v2 runner must wait out the run and then tear down on its own")

		// DELETE: Run called Delete, exercising the despawn path without
		// error. The store-side reap proof (the deleted ref returning ErrNotObserved
		// and the worker gone from the store) is deferred to ENG-5107.

		// The config worker check happens inside Run: an error from it fails the
		// run above. The error, warning and state checks below move into the
		// runner in the next PR of ENG-5114; until then they run here, with the
		// same allowed-warning list as every other integration scenario.
		verifyNoErrorsOrWarnings(testLogger)
		verifyStateFieldsAreValid(store)
	})
})
