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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("PendingRemoval Flag Clearing", func() {
	var (
		ctx         context.Context
		parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		mockStore   *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = newMockTriangularStore()
	})

	Describe("when child with pendingRemoval flag re-appears in specs", func() {
		It("should clear pendingRemoval flag when child re-appears", func() {
			// Create a worker that starts with one child
			childSpecs := []config.ChildSpec{
				{
					Name:       "child1",
					WorkerType: "child",
					UserSpec:   config.UserSpec{Config: "child-config"},
				},
			}

			worker := &hierarchicalWorker{
				id:     "parent",
				logger: newTickLogger(),
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: childSpecs,
			}

			parentSuper = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
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

			mockStore.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			// Tick 1: Create the child
			err = parentSuper.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify child was created
			children := parentSuper.GetChildren()
			Expect(children).To(HaveKey("child1"))

			// Manually set pendingRemoval flag to simulate a child that was marked for removal
			// (this happens when a child disappears from specs but hasn't finished shutdown yet)
			parentSuper.TestSetPendingRemovalFlag("child1", true)

			// Verify pendingRemoval flag is set
			Expect(parentSuper.TestIsPendingRemoval("child1")).To(BeTrue(),
				"pendingRemoval should be set after manual flag set")

			// Child is still in specs, so the next tick should clear pendingRemoval
			// (simulating: child was marked for removal, but then config was restored)
			err = parentSuper.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify child still exists
			children = parentSuper.GetChildren()
			Expect(children).To(HaveKey("child1"), "child should still exist after re-appearing in specs")

			// THE KEY ASSERTION: pendingRemoval flag should be cleared
			// because the child is present in the desired specs
			Expect(parentSuper.TestIsPendingRemoval("child1")).To(BeFalse(),
				"pendingRemoval flag should be cleared when child re-appears in specs")
		})
	})
})
