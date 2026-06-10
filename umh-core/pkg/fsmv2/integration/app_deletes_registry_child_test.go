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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"

	// Blank-import the kernel config worker plus the dynamic worker the registry
	// declares, so their init() registrations exist before the supervisor ticks.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("Application supervisor tears down a registry-deleted worker", func() {
	const (
		appKey          = "application"
		configWorkerKey = "configworker"
		kernelChildName = "config-worker"
	)

	AfterEach(func() {
		register.ClearDeps(appKey)
		register.ClearDeps(configWorkerKey)
	})

	It("removes the helloworld child after Delete while the config-worker kernel survives", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		// One shared registry, wired under both keys before the app supervisor
		// constructs the application worker (so the COS read sees a non-nil handle).
		w := dynamicchildren.NewWriter()
		dynamicchildren.WireSharedRegistry(w.Registry(), appKey, configWorkerKey)

		// Upsert a helloworld child and drive the supervisor on its started
		// context so the spawned child's action executor runs.
		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}
		Expect(w.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())

		sup, _, _ := newAppSupervisorWithStore(logger)
		sup.TestMarkAsStarted()

		// Phase 1: the helloworld child spawns and reaches Running, and the
		// config-worker kernel is present.
		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			children := sup.GetChildren()

			kernel, hasKernel := children[kernelChildName]
			if !hasKernel || kernel == nil {
				return false
			}

			child, hasChild := children["hello-1"]
			if !hasChild || child == nil {
				return false
			}

			return childStateName(child) == "Running"
		}, "5s", "100ms").Should(BeTrue(),
			"the registry-declared helloworld child must first spawn and reach Running")

		// After Delete, hello-1 disappears from GetChildren() within a few ticks.
		w.Delete(ref)

		Eventually(func() bool {
			_ = sup.TestTick(ctx)

			_, hasChild := sup.GetChildren()["hello-1"]

			return !hasChild
		}, "5s", "100ms").Should(BeTrue(),
			"the helloworld child must be reaped once its registry ref is deleted")

		// Phase 3: deleting a dynamic ref must NEVER remove the kernel (P7).
		// The config-worker kernel remains present after the reap.
		kernel, hasKernel := sup.GetChildren()[kernelChildName]
		Expect(hasKernel).To(BeTrue(),
			"deleting a dynamic ref must not reap the config-worker kernel (P7)")
		Expect(kernel).ToNot(BeNil())
	})
})
