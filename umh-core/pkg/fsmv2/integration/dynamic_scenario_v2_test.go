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

		// The runner builds an fsmv2client over this same store, so Run reads
		// observed state from the store verifyStateFieldsAreValid inspects afterward.
		result, err := examples.Run(ctx, examples.RunConfig{
			ScenarioV2:   dynamic,
			Duration:     2 * time.Second,
			TickInterval: 100 * time.Millisecond,
			Logger:       testLogger.FSMLogger,
			Store:        store,
		})

		// The final mood is not re-read here. After Delete, nothing stops the
		// supervisor ticking the child (ENG-5107). Once Run removes its temp mood
		// files, CollectObservedState overwrites the observed mood with "".
		Expect(err).NotTo(HaveOccurred(),
			"the dynamic scenario must observe create->Running and update->changed-mood through the migration-API client, Delete the child, and still read the config worker")
		Eventually(result.Done, "55s").Should(BeClosed(),
			"the v2 runner must wait out the run and then tear down on its own")

		// ENG-5114 moves these checks into the runner. Until then they run here,
		// against the allowed-warning list every integration scenario shares.
		verifyNoErrorsOrWarnings(testLogger)
		verifyStateFieldsAreValid(store)
	})
})
