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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"

	// Blank-import the helloworld state subpackage so its init() registers the
	// initial state before the supervisor ticks. The configworker and its state
	// register transitively via the configworker import above.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

// This canary drives a helloworld worker THROUGH the process-scoped
// fsmv2client, the seam any FSMv1 benthos manager uses. It inlines the
// production wiring (dynamicchildren registry + SetClient) and ticks by hand,
// so the multi-tick reap (the supervisor removing the child) is deterministic.
var _ = Describe("fsmv2bridge helloworld canary", func() {
	const (
		maxAge      = 10 * time.Second
		childName   = "canary-hello"
		moodContent = "happy"
		respawnMood = "cheerful"
	)

	AfterEach(func() {
		// The configworker deps key and the client singleton are process-global;
		// a spec that fails mid-run would otherwise leak them into later specs.
		fsmv2client.SetClient(nil)
		register.ClearDeps(configworker.WorkerTypeName)
	})

	It("drives a helloworld child through the client: Upsert, Fresh+Running, Delete, reap, Deleted, respawn with a new mood", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		sup, store, _ := newAppSupervisorWithStore(logger)
		// Inline the production wiring: publish the dynamicchildren registry
		// under the configworker deps key (so the supervisor spawns the
		// configworker kernel and enables dynamic spawning) and publish the
		// process-scoped client bound to this store.
		dynWriter := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, dynWriter.Registry())
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynWriter, store))

		sup.TestMarkAsStarted()

		bridge := fsmv2client.GetClient()
		Expect(bridge).NotTo(BeNil(), "SetClient must publish the client")

		// A mood file whose contents the helloworld child surfaces in its status.
		moodDir := GinkgoT().TempDir()
		moodPath := filepath.Join(moodDir, "mood")
		Expect(os.WriteFile(moodPath, []byte(moodContent), 0o600)).To(Succeed())

		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: childName}
		spec := map[string]any{"state": "running", "moodFilePath": moodPath}
		Expect(bridge.Upsert(ref, spec)).To(Succeed())

		// Phase 1: Upsert -> the child spawns, reaches Running, and GetFresh
		// reports Fresh with the mood the child read from the file.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			obs, freshness, err := fsmv2client.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
			if err != nil || freshness != fsmv2client.Fresh {
				return false
			}

			if obs.Status.Mood != moodContent {
				return false
			}

			child, ok := sup.GetChildren()[childName]
			return ok && child != nil && childStateName(child) == "Running"
		}, "5s", "100ms").Should(BeTrue(),
			"the helloworld child must spawn, reach Running, and be reported Fresh via the client")

		// Phase 2: Delete, then reap. Despawn is multi-tick (a single TestTick
		// does NOT reap: tick1 pendingRemoval+RequestShutdown; tick2
		// Running->Stopped; tick3 Stopped->SignalNeedsRemoval->reap+collector
		// stopped). Reap the old child fully BEFORE re-Upsert so the respawn is
		// genuine (re-Upsert before reap would cancel pendingRemoval and reuse
		// the same child).
		bridge.Delete(ref)

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			_, hasChild := sup.GetChildren()[childName]
			return !hasChild
		}, "5s", "100ms").Should(BeTrue(),
			"after Delete, the child must be fully reaped from the supervisor before respawn")

		// Phase 3(a): the reaped child reads as Deleted.
		reapedObs, reapedFreshness, err := fsmv2client.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
		Expect(err).NotTo(HaveOccurred())
		Expect(reapedFreshness).To(Equal(fsmv2client.Deleted),
			"after the reap, GetFresh must report the removed child as Deleted")
		Expect(reapedObs.Status.Mood).To(BeEmpty(),
			"GetFresh must not return the removed child's last observation")

		// Phase 3(b): genuine respawn. The old child is gone, so this Upsert
		// spawns a NEW child with a changed mood file.
		moodPath2 := filepath.Join(GinkgoT().TempDir(), "mood")
		Expect(os.WriteFile(moodPath2, []byte(respawnMood), 0o600)).To(Succeed())

		spec2 := map[string]any{"state": "running", "moodFilePath": moodPath2}
		Expect(bridge.Upsert(ref, spec2)).To(Succeed())

		// Phase 3(c): before the next tick the new child does not exist yet, and
		// the store still marks the old one as removed.
		_, preTickFreshness, err := fsmv2client.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
		Expect(err).NotTo(HaveOccurred())
		Expect(preTickFreshness).To(Equal(fsmv2client.Deleted),
			"immediately after re-Upsert (pre-tick), GetFresh must still report Deleted")

		// Phase 3(d): after ticking, the respawned child reports the NEW mood.
		servedOldMood := false

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			obs, freshness, err := fsmv2client.GetFresh[hello_world.HelloworldStatus](ctx, bridge, ref, maxAge)
			if err != nil || freshness != fsmv2client.Fresh {
				return false
			}

			if obs.Status.Mood == moodContent {
				servedOldMood = true
			}

			return obs.Status.Mood == respawnMood
		}, "5s", "100ms").Should(BeTrue(),
			"after re-Upsert with a new mood, the respawned child must be reported Fresh with the new mood")

		Expect(servedOldMood).To(BeFalse(),
			"GetFresh must never serve the removed child's observation after the respawn")
	})
})
