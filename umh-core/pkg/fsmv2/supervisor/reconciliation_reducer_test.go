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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("CHANGE-19 Reducer", func() {
	var (
		ctx         context.Context
		parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		mockStore   *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = newMockTriangularStore()
	})

	It("TestReconcileChildren_EnabledFalse_SetsIsShutdownRequestedWithoutPendingRemoval", func() {
		childSpecs := []config.ChildSpec{
			{
				Name:       "child1",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}

		worker := &hierarchicalWorker{
			id:     "parent",
			logger: newTickLogger(),
			observed: &mockObservedState{
				ID:          "parent-worker",
				CollectedAt: time.Now(),
			},
			childrenSpecs: childSpecs,
		}

		parentSuper = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "parent",
			Logger:     deps.NewNopFSMLogger(),
			Store:      mockStore,
		})

		identity := deps.Identity{
			ID:         "parent-worker",
			Name:       "Parent Worker",
			WorkerType: "parent",
		}
		err := parentSuper.AddWorker(identity, worker)
		Expect(err).NotTo(HaveOccurred())

		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

		desiredDoc := persistence.Document{
			"id":                identity.ID,
			"ShutdownRequested": false,
		}
		_, err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
		Expect(err).NotTo(HaveOccurred())

		mockStore.Observed["parent"] = map[string]interface{}{
			"parent-worker": persistence.Document{
				"id":          "parent-worker",
				"collectedAt": time.Now(),
			},
		}

		// Tick 1: create child and apply reducer (Enabled=false → IsShutdownRequested=true)
		err = parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Assert 1: child is resident in s.children (not despawned)
		children := parentSuper.GetChildren()
		Expect(children).To(HaveKey("child1"), "Enabled=false child must stay resident in s.children")

		// Assert 2: child is NOT in pendingRemoval — Enabled=false ≠ omission from specs
		Expect(parentSuper.TestIsPendingRemoval("child1")).To(BeFalse(),
			"Enabled=false must not write pendingRemoval; only omission from the spec list does")

		// Assert 3: child's worker desired state carries IsShutdownRequested=true
		var childDesired supervisor.TestDesiredState
		err = mockStore.LoadDesiredTyped(ctx, "child", "child1-001", &childDesired)
		Expect(err).NotTo(HaveOccurred())
		Expect(childDesired.IsShutdownRequested()).To(BeTrue(),
			"reducer must write IsShutdownRequested=true on child's worker desired state when Enabled=false")
	})
})
