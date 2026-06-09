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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	registry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// newAppSupervisorWithStore builds an application supervisor through the boot
// path (NewApplicationSupervisor -> NewApplicationWorker) and returns it
// together with the TriangularStore it writes observed state into and the
// worker ID, so the test can read ApplicationStatus back after a tick.
func newAppSupervisorWithStore(logger deps.FSMLogger) (
	*supervisor.Supervisor[fsmv2.Observation[snapshot.ApplicationStatus], *fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]],
	*storage.TriangularStore,
	string,
) {
	ctx := context.Background()
	const appID = "test-app-001"

	basicStore := memory.NewInMemoryStore()

	appWorkerType, err := storage.DeriveWorkerType[fsmv2.Observation[snapshot.ApplicationStatus]]()
	Expect(err).ToNot(HaveOccurred())

	_ = basicStore.CreateCollection(ctx, appWorkerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, appWorkerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, appWorkerType+"_observed", nil)

	store := storage.NewTriangularStore(basicStore, logger)

	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           appID,
		Name:         "Test Application Supervisor",
		Store:        store,
		Logger:       logger,
		TickInterval: 100 * time.Millisecond,
	})
	Expect(err).ToNot(HaveOccurred())

	return sup, store, appID
}

var _ = Describe("Application worker surfaces the shared registry into ApplicationStatus", func() {
	const appKey = "application"

	AfterEach(func() {
		register.ClearDeps(appKey)
	})

	It("carries DynamicChildren and sets RegistryConfigured=true when a registry is published", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		// Publish ONE shared registry under the application worker type and
		// record a single ref so the COS read has something to surface.
		reg := registry.NewConfigWorker()
		ref := registry.Ref{WorkerType: "example", Name: "example-child"}
		Expect(reg.Upsert(ref, map[string]any{"value": 1})).To(Succeed())
		register.SetDeps[*registry.Registry](appKey, reg.Registry())

		sup, store, appID := newAppSupervisorWithStore(logger)

		var observed fsmv2.Observation[snapshot.ApplicationStatus]
		Eventually(func() bool {
			_ = sup.TestTick(ctx)
			obs, loadErr := storage.LoadObservedTyped[fsmv2.Observation[snapshot.ApplicationStatus]](store, ctx, appID)
			if loadErr != nil {
				return false
			}
			observed = obs
			return observed.Status.RegistryConfigured
		}, "5s", "100ms").Should(BeTrue(),
			"RegistryConfigured must be true once a non-nil registry is published under the application worker type")

		Expect(observed.Status.DynamicChildren).To(HaveLen(1),
			"DynamicChildren must carry the single spec recorded in the shared registry")
		got := observed.Status.DynamicChildren[0]
		Expect(got.Name).To(Equal("example-child"))
		Expect(got.WorkerType).To(Equal("example"))
		Expect(got.Enabled).To(BeTrue(),
			"the surfaced spec must carry Enabled=true so a later step can spawn it")
		Expect(got.UserSpec.Config).ToNot(BeEmpty(),
			"the surfaced spec must carry the structured config, not just a Ref")
	})

	It("leaves RegistryConfigured=false and DynamicChildren nil when no registry is published", func() {
		ctx := context.Background()
		logger := deps.NewNopFSMLogger()

		// No SetDeps: the constructor's register.GetDeps returns a nil handle.
		register.ClearDeps(appKey)

		sup, store, appID := newAppSupervisorWithStore(logger)

		var observed fsmv2.Observation[snapshot.ApplicationStatus]
		Eventually(func() error {
			_ = sup.TestTick(ctx)
			obs, loadErr := storage.LoadObservedTyped[fsmv2.Observation[snapshot.ApplicationStatus]](store, ctx, appID)
			if loadErr != nil {
				return loadErr
			}
			observed = obs
			return nil
		}, "5s", "100ms").Should(Succeed())

		Expect(observed.Status.RegistryConfigured).To(BeFalse(),
			"RegistryConfigured must be false when no registry is published")
		Expect(observed.Status.DynamicChildren).To(BeNil(),
			"DynamicChildren must be nil when no registry is published")
	})
})
