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
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"

	// Blank-import the helloworld state subpackage so its init() registers the
	// initial state before the supervisor ticks. The configworker and its state
	// register transitively via the fsmv2bridge import.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

// This canary is PR1's cross-boundary proof: it drives a helloworld worker
// THROUGH the process-scoped fsmv2bridge.Client (the seam any FSMv1 benthos
// manager will use), exercising Ensure -> GetFresh Fresh+Running -> Remove ->
// Unregistered -> re-Ensure (respawn guard). It uses PublishForProduction so
// the production wiring is exercised end-to-end, and manual ticking so the
// respawn-guard Stale window is deterministic.
var _ = Describe("fsmv2bridge helloworld canary", func() {
	const (
		maxAge      = 10 * time.Second
		childName   = "canary-hello"
		moodContent = "happy"
	)

	AfterEach(func() {
		// The configworker deps key and the bridge singleton are process-global;
		// a spec that fails mid-run would otherwise leak them into later specs.
		fsmv2bridge.Set(nil)
		register.ClearDeps(configworker.WorkerTypeName)
	})

	It("drives a helloworld child through the bridge: Ensure, Fresh+Running, Remove, Unregistered, respawn", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		sup, store, _ := newAppSupervisorWithStore(logger)
		// PublishForProduction publishes the configworker deps key (so the
		// supervisor spawns the configworker kernel and enables dynamic
		// spawning) and the process-scoped bridge bound to this store.
		bridgeCleanup := fsmv2bridge.PublishForProduction(store)
		defer bridgeCleanup()

		sup.TestMarkAsStarted()

		bridge := fsmv2bridge.Get()
		Expect(bridge).NotTo(BeNil(), "PublishForProduction must publish the bridge")

		// A mood file whose contents the helloworld child surfaces in its status.
		moodDir := GinkgoT().TempDir()
		moodPath := filepath.Join(moodDir, "mood")
		Expect(os.WriteFile(moodPath, []byte(moodContent), 0o600)).To(Succeed())

		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: childName}
		spec := map[string]any{"state": "running", "moodFilePath": moodPath}
		Expect(bridge.Ensure(ref, spec)).To(Succeed())

		// Phase 1: Ensure -> the child spawns, reaches Running, and GetFresh
		// reports Fresh with the mood the child read from the file.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			status, fresh, err := fsmv2bridge.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
			if err != nil || fresh != fsmv2bridge.Fresh {
				return false
			}

			if status.Mood != moodContent {
				return false
			}

			child, ok := sup.GetChildren()[childName]
			return ok && child != nil && childStateName(child) == "Running"
		}, "5s", "100ms").Should(BeTrue(),
			"the helloworld child must spawn, reach Running, and be reported Fresh via the bridge")

		// Phase 2: Remove -> GetFresh reports Unregistered (the registry no
		// longer holds the ref).
		bridge.Remove(ref)

		Eventually(func() fsmv2bridge.Freshness {
			_, fresh, err := fsmv2bridge.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
			if err != nil {
				return fsmv2bridge.Fresh // sentinel: keep polling
			}

			return fresh
		}, "2s", "50ms").Should(Equal(fsmv2bridge.Unregistered),
			"after Remove, GetFresh must report Unregistered")

		// Phase 3: re-Ensure. With no tick yet, the store still holds the
		// pre-Remove observation whose CollectedAt predates the new lastEnsure,
		// so GetFresh must report Stale (the respawn guard) — not Fresh.
		Expect(bridge.Ensure(ref, spec)).To(Succeed())

		_, fresh, err := fsmv2bridge.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
		Expect(err).NotTo(HaveOccurred())
		Expect(fresh).To(Equal(fsmv2bridge.Stale),
			"immediately after re-Ensure, GetFresh must report Stale (respawn guard), not Fresh")

		// After ticking, the respawned child collects a fresh observation and
		// GetFresh reports Fresh again.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			status, fresh, err := fsmv2bridge.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
			if err != nil || fresh != fsmv2bridge.Fresh {
				return false
			}

			return status.Mood == moodContent
		}, "5s", "100ms").Should(BeTrue(),
			"after re-Ensure and ticking, the respawned child must be reported Fresh again")
	})
})
