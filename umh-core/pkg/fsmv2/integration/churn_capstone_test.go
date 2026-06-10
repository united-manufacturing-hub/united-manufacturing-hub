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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"

	// Blank-import the kernel config worker plus the dynamic worker the registry
	// declares, so their init() registrations exist before the supervisor ticks.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("Migration-API seam capstone: example-worker churn end-to-end", func() {
	const (
		configWorkerKey = "configworker"
		kernelChildName = "config-worker"
	)

	AfterEach(func() {
		register.ClearDeps(configWorkerKey)
	})

	It("Upserts K=3 helloworld workers to Running, then Deletes all and converges to exactly the kernel", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		// One shared registry, published under the config-worker key before the
		// application supervisor constructs its worker, so the COS read sees a
		// non-nil handle and the kernel is emitted. The config-worker kernel and
		// the application read the same registry instance.
		w := dynamicchildren.NewWriter()
		register.SetDeps[*dynamicchildren.Registry](configWorkerKey, w.Registry())

		// An fsmv2Client over that Writer (writes) and the supervisor's store
		// (reads) -- built once the store exists below. HEADLESS: no Agent block,
		// so no communicator child.
		sup, store, _ := newAppSupervisorWithStore(logger)
		sup.TestMarkAsStarted()

		client := fsmv2client.NewFSMv2Client(w, store)

		// (1) Upsert K=3 helloworld workers through the client. Empty MoodFilePath
		// (state "running") means each worker never goes "sad", so it
		// deterministically reaches Running.
		refs := []dynamicchildren.Ref{
			{WorkerType: "helloworld", Name: "hello-1"},
			{WorkerType: "helloworld", Name: "hello-2"},
			{WorkerType: "helloworld", Name: "hello-3"},
		}
		for _, ref := range refs {
			Expect(client.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())
		}

		// (2) Drive the tick loop until EACH typed Get returns observed state
		// "Running" -- proves declare -> exist -> typed-read. The 20s budget
		// absorbs the real collector cadence, especially under -race.
		for _, ref := range refs {
			ref := ref
			Eventually(func() string {
				_ = sup.TestTick(ctx)

				obs, getErr := fsmv2client.Get[hello_world.HelloworldStatus](ctx, client, ref)
				if getErr != nil {
					return ""
				}

				return obs.State
			}, "20s", "100ms").Should(Equal("Running"),
				"helloworld child %q must reach Running through the real collector cadence", ref.Name)
		}

		// (3) Delete ALL three via the client. Delete is intentionally
		// fire-and-forget (no return); step 4's convergence poll is the real
		// assertion that the deletes took effect.
		for _, ref := range refs {
			client.Delete(ref)
		}

		// (4) Keep ticking until GetChildren() converges to EXACTLY the kernel:
		// it contains "config-worker" and NONE of hello-1..hello-3 -- proving
		// Delete -> converge-to-kernel and that the kernel child persists across
		// the churn.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			return childrenAreExactly(sup.GetChildren(), kernelChildName)
		}, "20s", "100ms").Should(BeTrue(),
			"after deleting all three helloworld refs the application must converge to exactly {config-worker}")

		// NOTE: Get on a Deleted ref still returns its last observed state until despawn store-cleanup lands (see ENG-5088 follow-up); the authoritative signal that a ref is gone is its absence from GetChildren(), asserted above.
	})
})

// childrenAreExactly reports whether children's key set equals want exactly:
// every wanted name is present and non-nil, and no other child remains.
func childrenAreExactly(children map[string]supervisor.SupervisorInterface, want ...string) bool {
	if len(children) != len(want) {
		return false
	}

	for _, name := range want {
		child, ok := children[name]
		if !ok || child == nil {
			return false
		}
	}

	return true
}
