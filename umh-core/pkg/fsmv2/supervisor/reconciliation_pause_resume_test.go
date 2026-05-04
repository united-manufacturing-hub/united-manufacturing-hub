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

// newPauseResumeFixture creates a parent supervisor wired to the supplied mockStore
// with a single child spec. The returned worker pointer lets callers mutate
// childrenSpecs between ticks to simulate Enabled flips.
func newPauseResumeFixture(
	ctx context.Context,
	mockStore *mockTriangularStore,
	initialSpecs []config.ChildSpec,
) (
	parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState],
	worker *hierarchicalWorker,
) {
	worker = &hierarchicalWorker{
		id:     "parent",
		logger: newTickLogger(),
		observed: &mockObservedState{
			ID:          "parent-worker",
			CollectedAt: time.Now(),
		},
		childrenSpecs: initialSpecs,
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

	return parentSuper, worker
}

var _ = Describe("CHANGE-19 Pause/Resume", func() {
	var (
		ctx       context.Context
		mockStore *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = newMockTriangularStore()
	})

	It("TestPauseResume_EnabledFalseStopsChildSupervisorStaysResident", func() {
		// Setup: parent with one child spec, Enabled=false from the start.
		// On tick 1: reconcileChildren creates the child, then the CHANGE-19 reducer
		// immediately writes IsShutdownRequested=true (since the child now exists and
		// Enabled=false). The child remains in s.children (stopped-but-resident).
		initialSpecs := []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}
		parentSuper, _ := newPauseResumeFixture(ctx, mockStore, initialSpecs)

		// Tick 1: create child and apply reducer (Enabled=false → IsShutdownRequested=true)
		err := parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Assert 1: child is resident in s.children (stopped-but-resident, not despawned)
		children := parentSuper.GetChildren()
		Expect(children).To(HaveKey("child-a"),
			"Enabled=false child must remain resident in s.children (stopped-but-resident semantics)")

		// Assert 2: child is NOT in pendingRemoval — Enabled=false is not the same as omission
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"Enabled=false must not write pendingRemoval; only omitting the name from specs triggers despawn")

		// Assert 3: child's desired state carries IsShutdownRequested=true so the worker
		// can progress to Stopped on subsequent ticks
		var childDesired supervisor.TestDesiredState
		err = mockStore.LoadDesiredTyped(ctx, "child", "child-a-001", &childDesired)
		Expect(err).NotTo(HaveOccurred())
		Expect(childDesired.IsShutdownRequested()).To(BeTrue(),
			"reducer must write IsShutdownRequested=true on child's desired state when Enabled=false")
	})

	It("TestPauseResume_ReenableFromStopped_ChildRestartsTryingToStart", func() {
		// Setup: start with Enabled=false so the child is paused immediately on tick 1.
		initialSpecs := []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}
		parentSuper, worker := newPauseResumeFixture(ctx, mockStore, initialSpecs)

		// Tick 1: create child, reducer writes IsShutdownRequested=true
		err := parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify precondition: child is paused
		var childDesired supervisor.TestDesiredState
		err = mockStore.LoadDesiredTyped(ctx, "child", "child-a-001", &childDesired)
		Expect(err).NotTo(HaveOccurred())
		Expect(childDesired.IsShutdownRequested()).To(BeTrue(), "precondition: child must be paused before re-enable")

		// Re-enable: flip the spec to Enabled=true
		worker.childrenSpecs = []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    true,
			},
		}
		// Bust the DDS cache. The cache key is the parent's userSpec hash, not childrenSpecs.
		// hierarchicalWorker.DeriveDesiredState ignores its spec parameter and reads
		// h.childrenSpecs directly, so changing childrenSpecs alone would not invalidate
		// the cache and reconcileChildren would reuse the Enabled=false DDS from tick 1.
		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config-reenable"})

		// Tick 2: reducer calls ClearShutdownRequest → IsShutdownRequested=false;
		// the child's state machine will transition from Stopped to TryingToStart on
		// subsequent worker ticks
		err = parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Assert 1: child is still resident (never despawned during the pause/resume cycle)
		children := parentSuper.GetChildren()
		Expect(children).To(HaveKey("child-a"),
			"child must remain resident throughout the pause/resume cycle")

		// Assert 2: child is still NOT in pendingRemoval
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"re-enabling must not affect pendingRemoval — only spec omission triggers despawn")

		// Assert 3: IsShutdownRequested is now false so the worker can restart
		err = mockStore.LoadDesiredTyped(ctx, "child", "child-a-001", &childDesired)
		Expect(err).NotTo(HaveOccurred())
		Expect(childDesired.IsShutdownRequested()).To(BeFalse(),
			"reducer must write IsShutdownRequested=false when Enabled flips back to true, "+
				"allowing the child's state machine to transition from Stopped to TryingToStart")
	})

	It("TestPauseResume_RaceFreeMidCleanup_WorkerCompletesStopBeforeResume", func() {
		// This test verifies the supervisor-level invariant for the mid-cleanup re-enable
		// scenario: even when Enabled flips false→true before the child finishes stopping,
		// the supervisor never places the child in pendingRemoval and always honours the
		// last Enabled value.
		//
		// The one-way stop trajectory (TryingToStop→Stopped before accepting the new
		// IsShutdownRequested=false) is enforced by the child worker's state machine, not
		// by the supervisor. At the supervisor level, the reducer is stateless: it reads
		// the current spec's Enabled field and writes IsShutdownRequested each tick.
		initialSpecs := []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    true,
			},
		}
		parentSuper, worker := newPauseResumeFixture(ctx, mockStore, initialSpecs)

		// Tick 1: create child with Enabled=true → reducer writes IsShutdownRequested=false
		err := parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify precondition: child exists and is not shutdown-requested
		children := parentSuper.GetChildren()
		Expect(children).To(HaveKey("child-a"), "precondition: child must exist after tick 1")
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"precondition: child must not be in pendingRemoval after tick 1")

		// Disable child (simulates parent deciding to pause mid-operation)
		worker.childrenSpecs = []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    false,
			},
		}
		// Bust the DDS cache so reconcileChildren picks up the Enabled=false spec.
		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config-disable"})

		// Tick 2: reducer writes IsShutdownRequested=true (child enters TryingToStop in worker FSM)
		err = parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Child is still resident and not in pendingRemoval after the first disable tick
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"Enabled=false must never write pendingRemoval")

		// Immediately re-enable before the worker FSM has completed its stop
		worker.childrenSpecs = []config.ChildSpec{
			{
				Name:       "child-a",
				WorkerType: "child",
				UserSpec:   config.UserSpec{Config: "child-config"},
				Enabled:    true,
			},
		}
		// Bust the DDS cache again so the Enabled=true is picked up on tick 3.
		parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config-reenable"})

		// Tick 3: reducer writes IsShutdownRequested=false; child stays resident
		err = parentSuper.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Assert 1: child has been resident throughout — never entered pendingRemoval
		Expect(parentSuper.TestIsPendingRemoval("child-a")).To(BeFalse(),
			"child must never enter pendingRemoval during an Enabled flip cycle")

		children = parentSuper.GetChildren()
		Expect(children).To(HaveKey("child-a"),
			"child must remain resident after the rapid disable/re-enable cycle")

		// Assert 2: IsShutdownRequested reflects the final Enabled=true value
		var childDesired supervisor.TestDesiredState
		err = mockStore.LoadDesiredTyped(ctx, "child", "child-a-001", &childDesired)
		Expect(err).NotTo(HaveOccurred())
		Expect(childDesired.IsShutdownRequested()).To(BeFalse(),
			"after re-enable, IsShutdownRequested must be false regardless of prior mid-flight disable")
	})
})
