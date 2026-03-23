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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// healthObservedState implements SetChildrenCounts so the collector's type assertion
// succeeds and counts are actually injected. Without this, the collector silently
// discards the counts (the standard mockObservedState doesn't implement it).
type healthObservedState struct {
	mu                sync.RWMutex
	ID                string             `json:"id"`
	CollectedAt       time.Time          `json:"collectedAt"`
	Desired           fsmv2.DesiredState `json:"-"`
	ChildrenHealthy   int                `json:"children_healthy"`
	ChildrenUnhealthy int                `json:"children_unhealthy"`
}

func (h *healthObservedState) GetObservedDesiredState() fsmv2.DesiredState { return h.Desired }
func (h *healthObservedState) GetTimestamp() time.Time                     { return h.CollectedAt }

func (h *healthObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ChildrenHealthy = healthy
	h.ChildrenUnhealthy = unhealthy
	return h
}

func (h *healthObservedState) GetChildrenCounts() (int, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ChildrenHealthy, h.ChildrenUnhealthy
}

// healthWorker returns a healthObservedState from CollectObservedState.
type healthWorker struct {
	observed *healthObservedState
}

func (w *healthWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return w.observed, nil
}

func (w *healthWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		BaseDesiredState: config.BaseDesiredState{State: "running"},
	}, nil
}

func (w *healthWorker) GetInitialState() fsmv2.State[any, any] {
	return &mockState{}
}

// phasedState is a configurable mock state that returns any LifecyclePhase.
type phasedState struct {
	phase config.LifecyclePhase
	name  string
}

func (s *phasedState) Next(snapshot any) fsmv2.NextResult[any, any] {
	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "staying in "+s.name)
}

func (s *phasedState) String() string                        { return s.name }
func (s *phasedState) LifecyclePhase() config.LifecyclePhase { return s.phase }

// createTickedChildSupervisor creates a child supervisor with one worker in the given
// phase, ticks it so lastLifecyclePhase is set, and returns the supervisor.
func createTickedChildSupervisor(ctx context.Context, store *mockTriangularStore, phase config.LifecyclePhase, name string) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	childSup := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "child",
		Logger:     deps.NewNopFSMLogger(),
		Store:      store,
	})

	childWorker := &mockWorker{
		initialState: &phasedState{phase: phase, name: name},
		observed: &mockObservedState{
			ID:          "child-001",
			CollectedAt: time.Now(),
			Desired:     &mockDesiredState{},
		},
	}
	childID := deps.Identity{ID: "child-001", Name: "Child", WorkerType: "child"}
	ExpectWithOffset(1, childSup.AddWorker(childID, childWorker)).To(Succeed())

	// Store entries are required so TestTick() can load the snapshot via
	// LoadObservedTyped/LoadDesired — they don't influence the lifecycle phase.
	_, err := store.SaveDesired(ctx, "child", childID.ID, persistence.Document{
		"id": childID.ID, "ShutdownRequested": false,
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	store.Observed["child"] = map[string]interface{}{
		childID.ID: persistence.Document{"id": childID.ID, "collectedAt": time.Now()},
	}

	// Tick child so lastLifecyclePhase is set from the worker's current state.
	ExpectWithOffset(1, childSup.TestTick(ctx)).To(Succeed())

	return childSup
}

var _ = Describe("HealthAccounting", func() {
	// Tests the counting logic used by ChildrenCountsProvider.
	//
	// Architecture: The ChildrenCountsProvider lambda (api.go:194-211) and
	// ChildrenManager.Counts() (children_manager.go:62-77) use identical logic:
	// iterate s.children, call GetLifecyclePhase() on each, classify by phase.
	// Both call GetLifecyclePhase() on real SupervisorInterface instances.
	//
	// These tests verify the counting logic through ChildrenManager.Counts(),
	// which is the public API equivalent. The children are real Supervisor
	// instances (not mocks) injected via TestSetChild, so GetLifecyclePhase()
	// reads the actual lastLifecyclePhase cache populated by TestTick().
	//
	// Production bug context: Container experiment showed communicator-001
	// reporting healthy=0, unhealthy=0 when transport-001 was in AuthFailed
	// (PhaseStarting). If these tests pass, the counting logic is correct
	// and the bug is in the collector pipeline wiring (collector not calling
	// ChildrenCountsProvider, or SetChildrenCounts type assertion failing).

	Describe("ChildrenManager.Counts with real child supervisors", func() {
		var (
			ctx   context.Context
			store *mockTriangularStore
		)

		BeforeEach(func() {
			ctx = context.Background()
			store = newMockTriangularStore()
		})

		It("should count a PhaseStarting child as unhealthy=1", func() {
			childSup := createTickedChildSupervisor(ctx, store, config.PhaseStarting, "AuthFailed")

			// Verify child reports the expected phase via GetLifecyclePhase().
			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseStarting))

			// Build ChildrenManager with the real child supervisor — same path
			// as ChildrenCountsProvider which iterates s.children.
			children := map[string]supervisor.SupervisorInterface{
				"transport-001": childSup,
			}
			mgr := supervisor.NewChildrenManager(children)
			healthy, unhealthy := mgr.Counts()

			Expect(unhealthy).To(Equal(1), "PhaseStarting child must be unhealthy=1")
			Expect(healthy).To(Equal(0))
		})

		It("should count a PhaseRunningHealthy child as healthy=1", func() {
			childSup := createTickedChildSupervisor(ctx, store, config.PhaseRunningHealthy, "Running")

			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseRunningHealthy))

			children := map[string]supervisor.SupervisorInterface{
				"transport-001": childSup,
			}
			mgr := supervisor.NewChildrenManager(children)
			healthy, unhealthy := mgr.Counts()

			Expect(healthy).To(Equal(1), "PhaseRunningHealthy child must be healthy=1")
			Expect(unhealthy).To(Equal(0))
		})

		It("should count a PhaseStopped child as neither healthy nor unhealthy", func() {
			childSup := createTickedChildSupervisor(ctx, store, config.PhaseStopped, "Stopped")

			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseStopped))

			children := map[string]supervisor.SupervisorInterface{
				"transport-001": childSup,
			}
			mgr := supervisor.NewChildrenManager(children)
			healthy, unhealthy := mgr.Counts()

			Expect(healthy).To(Equal(0))
			Expect(unhealthy).To(Equal(0), "PhaseStopped child must be neither healthy nor unhealthy")
		})

		It("should count a PhaseRunningDegraded child as unhealthy=1", func() {
			childSup := createTickedChildSupervisor(ctx, store, config.PhaseRunningDegraded, "Degraded")

			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseRunningDegraded))

			children := map[string]supervisor.SupervisorInterface{
				"transport-001": childSup,
			}
			mgr := supervisor.NewChildrenManager(children)
			healthy, unhealthy := mgr.Counts()

			Expect(unhealthy).To(Equal(1), "PhaseRunningDegraded child must be unhealthy=1")
			Expect(healthy).To(Equal(0))
		})

		It("should seed lastLifecyclePhase from GetInitialState at AddWorker time", func() {
			// Child supervisor without any ticks — lastLifecyclePhase is seeded
			// from GetInitialState().LifecyclePhase() at AddWorker time.
			// mockWorker's default GetInitialState() returns mockState, which
			// reports PhaseRunningHealthy. This verifies the AddWorker seeding.
			childSup := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "child",
				Logger:     deps.NewNopFSMLogger(),
				Store:      store,
			})

			childWorker := &mockWorker{
				observed: &mockObservedState{
					ID:          "child-001",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}
			childID := deps.Identity{ID: "child-001", Name: "Child", WorkerType: "child"}
			Expect(childSup.AddWorker(childID, childWorker)).To(Succeed())

			// Do NOT tick — verify the phase seeded at AddWorker time.
			// mockState.LifecyclePhase() returns PhaseRunningHealthy.
			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseRunningHealthy),
				"AddWorker should seed lastLifecyclePhase from GetInitialState()")

			children := map[string]supervisor.SupervisorInterface{
				"transport-001": childSup,
			}
			mgr := supervisor.NewChildrenManager(children)
			healthy, unhealthy := mgr.Counts()

			Expect(healthy).To(Equal(1), "child seeded with PhaseRunningHealthy must be counted as healthy=1")
			Expect(unhealthy).To(Equal(0))
		})
	})

	Describe("FAILING: Full collector pipeline injects children counts into parent observed state", func() {
		// This test reproduces the production bug observed in the ENG-4576
		// container experiment: communicator-001 reports healthy=0, unhealthy=0
		// permanently while transport-001 is in AuthFailed (PhaseStarting).
		//
		// The counting logic (tested above) is correct. The bug is in the
		// collector pipeline: TestTick() does not start the collector goroutine,
		// so ChildrenCountsProvider is never called and SetChildrenCounts never
		// injects the counts into the parent's observed state.
		//
		// This test will PASS once the pipeline bug is fixed (either by making
		// TestTick trigger collection, or by fixing the collector wiring).

		It("should inject unhealthy=1 into parent observed state when child is in PhaseStarting", func() {
			ctx := context.Background()
			store := newMockTriangularStore()

			// Child supervisor with PhaseStarting worker (simulates AuthFailedState).
			childSup := createTickedChildSupervisor(ctx, store, config.PhaseStarting, "AuthFailed")
			Expect(childSup.GetLifecyclePhase()).To(Equal(config.PhaseStarting))

			// Parent supervisor with an observed state that implements SetChildrenCounts.
			parentObs := &healthObservedState{
				ID:          "communicator-001",
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			}

			parentSup := supervisor.NewSupervisor[*healthObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "communicator",
				Logger:     deps.NewNopFSMLogger(),
				Store:      store,
			})

			parentWorker := &healthWorker{observed: parentObs}
			parentID := deps.Identity{ID: "communicator-001", Name: "Communicator", WorkerType: "communicator"}
			Expect(parentSup.AddWorker(parentID, parentWorker)).To(Succeed())

			// Store entries for parent tick.
			_, err := store.SaveDesired(ctx, "communicator", parentID.ID, persistence.Document{
				"id": parentID.ID, "ShutdownRequested": false,
			})
			Expect(err).NotTo(HaveOccurred())
			store.Observed["communicator"] = map[string]interface{}{
				parentID.ID: persistence.Document{"id": parentID.ID, "collectedAt": time.Now()},
			}

			// Inject child into parent.
			childDone := make(chan struct{})
			defer close(childDone)
			parentSup.TestSetChild("transport-001", childSup, childDone)

			// Tick the parent — this exercises the production code path:
			// reconcileSingleWorker → collector.CollectObservedState →
			// ChildrenCountsProvider → SetChildrenCounts on observed state.
			//
			// BUG: TestTick() does not start the collector goroutine, so
			// ChildrenCountsProvider is never called. SetChildrenCounts
			// is never invoked. The counts stay at (0, 0).
			Expect(parentSup.TestTick(ctx)).To(Succeed())

			healthy, unhealthy := parentObs.GetChildrenCounts()
			Expect(unhealthy).To(Equal(1),
				"REPRODUCTION: parent should see unhealthy=1 for PhaseStarting child via collector pipeline (got healthy=%d unhealthy=%d)", healthy, unhealthy)
		})
	})
})
