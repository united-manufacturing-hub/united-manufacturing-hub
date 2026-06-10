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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"

	// Blank-import the kernel config worker plus the dynamic worker the registry
	// declares, so their init() registrations exist before the supervisor ticks.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("Application supervisor spawns a registry-declared worker", func() {
	const (
		appKey          = "application"
		configWorkerKey = "configworker"
		kernelChildName = "config-worker"
	)

	AfterEach(func() {
		register.ClearDeps(appKey)
		register.ClearDeps(configWorkerKey)
	})

	It("spawns the helloworld child to Running and keeps the config-worker kernel present", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		// (1) One shared registry, wired under both keys before the app supervisor
		// constructs the application worker (so the COS read sees a non-nil handle).
		w := dynamicchildren.NewWriter()
		dynamicchildren.WireSharedRegistry(w.Registry(), appKey, configWorkerKey)

		// (2) Upsert a helloworld child. Empty MoodFilePath means the worker never
		// goes "sad", so it deterministically reaches Running.
		ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: "hello-1"}
		Expect(w.Upsert(ref, map[string]any{"state": "running"})).To(Succeed())

		// (3) Drive the application supervisor through the real tick loop. Mark it
		// started so a spawned child is handed the supervisor's long-lived context
		// and its action executor runs -- the same started==true path production
		// takes after Start()/StartAsChild().
		sup, _, _ := newAppSupervisorWithStore(logger)
		sup.TestMarkAsStarted()

		// (4) Eventually, GetChildren() includes the helloworld child reaching its
		// Running state, AND the config-worker kernel child is present.
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
			"the registry-declared helloworld child must spawn and reach Running, "+
				"and the config-worker kernel must be present once the registry is configured")
	})
})

// childStateName reads a child supervisor's current FSM state name.
func childStateName(child supervisor.SupervisorInterface) string {
	return child.GetCurrentStateName()
}
