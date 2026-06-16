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

var _ = Describe("CHANGE-19 disable-mapping pass (applyDisableMapping) — D6 IsDisabled design", func() {
	var (
		ctx   context.Context
		store *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockTriangularStore()
	})

	Describe("nil specs → silent no-op", func() {
		It("TestDisableMapping_NilSpecs_LeafSupervisor_SilentlyNoOps: does not panic, does not error", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyDisableMapping(ctx, nil)
			}).NotTo(Panic())
		})

		It("empty specs slice also completes without panic", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyDisableMapping(ctx, []config.ChildSpec{})
			}).NotTo(Panic())
		})
	})

	Describe("Enabled=false → IsDisabled=true", func() {
		It("sets Disabled=true on child worker desired state", func() {
			childSpecs := []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}}
			s := newParentSupervisorWithSpecs(ctx, store, childSpecs)

			err := s.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := s.GetChildren()
			Expect(children).To(HaveKey("child1"), "child must exist before the disable-mapping pass can disable it")

			_, err = store.SaveDesired(ctx, "child", "child1-001", persistence.Document{
				"id":                "child1-001",
				"ShutdownRequested": false,
				"Disabled":          false,
			})
			Expect(err).NotTo(HaveOccurred())

			s.TestApplyDisableMapping(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    false,
			}})

			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeTrue(), "the disable-mapping pass must set Disabled=true for Enabled=false spec")
		})
	})

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

			_, err = store.SaveDesired(ctx, "child", "child1-001", persistence.Document{
				"id":                "child1-001",
				"ShutdownRequested": false,
				"Disabled":          true,
			})
			Expect(err).NotTo(HaveOccurred())

			s.TestApplyDisableMapping(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    true,
			}})

			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeFalse(), "the disable-mapping pass must clear Disabled=false on re-enable (Enabled=true)")
		})
	})

	Describe("disable-mapping pass does not touch IsShutdownRequested", func() {
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

			// The disable-mapping pass with Enabled=false must write Disabled=true but NOT clear ShutdownRequested
			s.TestApplyDisableMapping(ctx, []config.ChildSpec{{
				Name:       "child1",
				WorkerType: "child",
				Enabled:    false,
			}})

			var result supervisor.TestDesiredState
			err = store.LoadDesiredTyped(ctx, "child", "child1-001", &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Disabled).To(BeTrue(), "the disable-mapping pass must set Disabled=true")
			Expect(result.ShutdownReq).To(BeTrue(), "the disable-mapping pass must NOT clear ShutdownRequested (restart subsystem owns it)")
		})
	})

	Describe("child not found in children map", func() {
		It("Enabled=false + unknown child: no panic, no error (teardown case)", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyDisableMapping(ctx, []config.ChildSpec{{
					Name:       "nonexistent",
					WorkerType: "child",
					Enabled:    false,
				}})
			}).NotTo(Panic())
		})

		It("Enabled=true + unknown child: no panic (handled gracefully)", func() {
			s := newParentSupervisorWithSpecs(ctx, store, []config.ChildSpec{})

			Expect(func() {
				s.TestApplyDisableMapping(ctx, []config.ChildSpec{{
					Name:       "nonexistent",
					WorkerType: "child",
					Enabled:    true,
				}})
			}).NotTo(Panic())
		})
	})
})
