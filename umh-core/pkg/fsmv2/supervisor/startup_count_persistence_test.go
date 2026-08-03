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

package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// startupCountStatus is the developer status payload of the NewObservation
// worker used here. It carries no business data in this test - the assertion
// reads framework metrics off the wrapped Observation, not this status.
type startupCountStatus struct {
	ID string `json:"id"`
}

// startupCountWorker is a minimal NewObservation worker: CollectObservedState
// returns fsmv2.NewObservation, whose CollectedAt is zero, so the collector's
// post-COS wrapping fires and injects framework metrics - including StartupCount
// taken from workerCtx. Its TObserved is Observation[startupCountStatus] (value),
// which is what the supervisor persists and reloads on AddWorker.
type startupCountWorker struct{}

func (w *startupCountWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return fsmv2.NewObservation(startupCountStatus{ID: "startup-count-worker"}), nil
}

func (w *startupCountWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}, State: "running"}, nil
}

func (w *startupCountWorker) GetInitialState() fsmv2.State[any, any] {
	return &supervisor.TestState{}
}

var _ = Describe("StartupCount persistence", func() {
	It("advances across a worker respawn instead of resetting to 1", func() {
		const workerType = "startupcount-rung4"
		const workerID = "startup-count-worker"

		// Collapse the observation cadence so the collector saves promptly; the
		// default is 1s, which would make the Eventually polls needlessly slow.
		fsmv2.RegisterObservationInterval(workerType, 20*time.Millisecond)

		store := supervisor.CreateTestTriangularStoreForWorkerType(workerType)

		sup := supervisor.NewSupervisor[fsmv2.Observation[startupCountStatus], *config.DesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     deps.NewNopFSMLogger(),
		})

		ctx := context.Background()
		identity := deps.Identity{ID: workerID, Name: "Startup Count Worker", WorkerType: workerType}

		// First spawn: AddWorker reads no prior observation, so StartupCount is 1.
		Expect(sup.AddWorker(identity, &startupCountWorker{})).To(Succeed())

		// Start the collector only (no reconcile tick loop) so it saves an
		// Observation carrying the framework metrics from workerCtx, StartupCount
		// injected as 1.
		sup.TestMarkAsStarted()
		DeferCleanup(func() {
			_ = sup.RemoveWorker(context.Background(), workerID)
		})

		Eventually(func() int64 {
			var loaded fsmv2.Observation[startupCountStatus]
			if err := store.LoadObservedTyped(ctx, workerType, workerID, &loaded); err != nil {
				return 0
			}

			return loaded.Metrics.Framework.StartupCount
		}, 3*time.Second, 25*time.Millisecond).Should(Equal(int64(1)),
			"first spawn should persist StartupCount=1")

		// Despawn and respawn the same worker ID. RemoveWorker stops worker1's
		// collector; AddWorker reads the persisted StartupCount=1 from the store
		// before writing its own initial observation, so workerCtx.startupCount
		// becomes 2 (not reset to 1).
		Expect(sup.RemoveWorker(ctx, workerID)).To(Succeed())
		Expect(sup.AddWorker(identity, &startupCountWorker{})).To(Succeed())

		// The respawned worker owns a fresh collector; start it the same way.
		sup.TestMarkAsStarted()

		// The observation the respawned worker's collector saves must carry
		// StartupCount=2, proving the count survives the despawn/respawn cycle.
		Eventually(func() int64 {
			var loaded fsmv2.Observation[startupCountStatus]
			if err := store.LoadObservedTyped(ctx, workerType, workerID, &loaded); err != nil {
				return 0
			}

			return loaded.Metrics.Framework.StartupCount
		}, 3*time.Second, 25*time.Millisecond).Should(Equal(int64(2)),
			"respawned worker should persist StartupCount=2, not reset to 1")
	})
})
