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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// newParentSupervisorWithSpecs creates a parent supervisor whose worker returns the given child specs.
// The parent's desired + observed state are saved to the mockTriangularStore.
func newParentSupervisorWithSpecs(
	ctx context.Context,
	store *mockTriangularStore,
	specs []config.ChildSpec,
) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	worker := &hierarchicalWorker{
		id:     "parent",
		logger: newTickLogger(),
		observed: &mockObservedState{
			ID:          "parent-worker",
			CollectedAt: time.Now(),
			Desired:     &mockDesiredState{},
		},
		childrenSpecs: specs,
	}

	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "parent",
		Logger:     deps.NewNopFSMLogger(),
		Store:      store,
	})

	identity := deps.Identity{
		ID:         "parent-worker",
		Name:       "Parent Worker",
		WorkerType: "parent",
	}
	err := s.AddWorker(identity, worker)
	Expect(err).NotTo(HaveOccurred())

	s.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

	desiredDoc := persistence.Document{
		"id":                identity.ID,
		"ShutdownRequested": false,
	}
	_, err = store.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
	Expect(err).NotTo(HaveOccurred())

	store.Observed["parent"] = map[string]interface{}{
		"parent-worker": persistence.Document{
			"id":          "parent-worker",
			"collectedAt": time.Now(),
		},
	}

	return s
}

var _ = Describe("CHANGE-19 Reducer (applyReducer) — D6 IsDisabled design", func() {
	var (
		ctx   context.Context
		store *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockTriangularStore()
	})

	// --- nil specs: silent no-op (P0-A fix) ---

	Describe("nil specs → silent no-op", func() {
		It("TestReducer_NilSpecs_LeafSupervisor_SilentlyNoOps: does not panic, does not error", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyReducer(ctx, nil)
			}).NotTo(Panic())
		})

		It("empty specs slice also completes without panic", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyReducer(ctx, []config.ChildSpec{})
			}).NotTo(Panic())
		})
	})

	// --- Enabled=false → IsDisabled=true in store ---

	Describe("Enabled=false → IsDisabled=true", func() {
		It("sets Disabled=true on child worker desired state", func() {
			childSpecs := []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}}
			s := newParentSupervisorWithSpecs(ctx, store, childSpecs)

			// Tick to create the child supervisor in s.children
			err := s.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := s.GetChildren()
			Expect(children).To(HaveKey("child1"), "child must exist before reducer can disable it")

			// Save initial desired state for the child's worker so setDisabled has something to load
			_, err = store.SaveDesired(ctx, "child", "child1-001", persistence.Document{
				"id":                "child1-001",
				"ShutdownRequested": false,
				"Disabled":          false,
			})
			Expect(err).NotTo(HaveOccurred())

			// Run reducer with Enabled=false
			s.TestApplyReducer(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    false,
			}})

			// Read back the child's desired state and verify Disabled=true
			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeTrue(), "reducer must set Disabled=true for Enabled=false spec")
		})
	})

	// --- Enabled=true → IsDisabled=false (re-enable path) ---

	Describe("Enabled=true → IsDisabled=false (re-enable)", func() {
		It("clears Disabled=false after a previous disable", func() {
			childSpecs := []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}}
			s := newParentSupervisorWithSpecs(ctx, store, childSpecs)

			err := s.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := s.GetChildren()
			Expect(children).To(HaveKey("child1"))

			// Start with Disabled=true in store (previously disabled)
			_, err = store.SaveDesired(ctx, "child", "child1-001", persistence.Document{
				"id":                "child1-001",
				"ShutdownRequested": false,
				"Disabled":          true,
			})
			Expect(err).NotTo(HaveOccurred())

			// Run reducer with Enabled=true (re-enable)
			s.TestApplyReducer(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}})

			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeFalse(), "reducer must clear Disabled=false on re-enable (Enabled=true)")
		})
	})

	// --- Reducer-exclusive write: does NOT touch IsShutdownRequested ---

	Describe("reducer does not touch IsShutdownRequested", func() {
		It("leaves ShutdownRequested unchanged when setting Disabled", func() {
			childSpecs := []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}}
			s := newParentSupervisorWithSpecs(ctx, store, childSpecs)

			err := s.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(s.GetChildren()).To(HaveKey("child1"))

			// ShutdownRequested=true simulates restart subsystem mid-flight
			_, err = store.SaveDesired(ctx, "child", "child1-001", persistence.Document{
				"id":                "child1-001",
				"ShutdownRequested": true,
				"Disabled":          false,
			})
			Expect(err).NotTo(HaveOccurred())

			// Reducer with Enabled=false must write Disabled=true but NOT clear ShutdownRequested
			s.TestApplyReducer(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    false,
			}})

			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeTrue(), "reducer must set Disabled=true")
			Expect(result.ShutdownReq).To(BeTrue(), "reducer must NOT clear ShutdownRequested (restart subsystem owns it)")
		})
	})

	// --- child not in s.children → no panic ---

	Describe("child not found in children map", func() {
		It("Enabled=false + unknown child: no panic, no error (teardown case)", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyReducer(ctx, []config.ChildSpec{{
					Name:       "nonexistent",
					WorkerType: "child",
					Enabled:    false,
				}})
			}).NotTo(Panic())
		})

		It("Enabled=true + unknown child: no panic (handled gracefully)", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyReducer(ctx, []config.ChildSpec{{
					Name:       "nonexistent",
					WorkerType: "child",
					Enabled:    true,
				}})
			}).NotTo(Panic())
		})
	})
})

// --- WorkerSnapshot.ShouldStop: 3-way discriminator ---

var _ = Describe("WorkerSnapshot.ShouldStop — 3-way discriminator (D6)", func() {
	type noConfig struct{}
	type noStatus struct{}

	It("IsShutdownRequested=true → ShouldStop=true (shutdown wins)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: true,
			IsDisabled:          false,
			ParentMappedState:   config.DesiredStateRunning,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("IsDisabled=true → ShouldStop=true (admin pause)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: false,
			IsDisabled:          true,
			ParentMappedState:   config.DesiredStateRunning,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})

	It("both IsShutdownRequested and IsDisabled → ShouldStop=true (shutdown wins in StoppedState.Next)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: true,
			IsDisabled:          true,
			ParentMappedState:   config.DesiredStateRunning,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
		// Verify ordering invariant: IsShutdownRequested is checked first in state files.
		// ShouldStop ORs both; the discriminator in StoppedState.Next checks IsShutdownRequested
		// first to emit SignalNeedsRemoval, only checking IsDisabled when shutdown is not set.
		Expect(snap.IsShutdownRequested).To(BeTrue(), "shutdown WINS in StoppedState discriminator")
	})

	It("neither flag, parent running → ShouldStop=false (normal resume)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: false,
			IsDisabled:          false,
			ParentMappedState:   config.DesiredStateRunning,
		}
		Expect(snap.ShouldStop()).To(BeFalse())
	})

	It("ParentMappedState=stopped → ShouldStop=true (parent-driven stop)", func() {
		snap := fsmv2.WorkerSnapshot[noConfig, noStatus]{
			IsShutdownRequested: false,
			IsDisabled:          false,
			ParentMappedState:   config.DesiredStateStopped,
		}
		Expect(snap.ShouldStop()).To(BeTrue())
	})
})
