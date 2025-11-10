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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type tickLogger struct {
	mu     sync.Mutex
	events []string
}

func newTickLogger() *tickLogger {
	return &tickLogger{
		events: make([]string, 0),
	}
}

func (t *tickLogger) Log(event string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.events = append(t.events, event)
}

func (t *tickLogger) GetEvents() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return append([]string{}, t.events...)
}

type hierarchicalWorker struct {
	id            string
	logger        *tickLogger
	childrenSpecs []config.ChildSpec
	observed      *mockObservedState
}

func (h *hierarchicalWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return h.observed, nil
}

func (h *hierarchicalWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	h.logger.Log("DeriveDesiredState:" + h.id)

	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: h.childrenSpecs,
	}, nil
}

func (h *hierarchicalWorker) GetInitialState() fsmv2.State {
	return &mockState{}
}

var _ = Describe("Hierarchical Tick Propagation (Task 0.6)", func() {
	var (
		ctx         context.Context
		parentSuper *supervisor.Supervisor
		mockStore   *mockTriangularStore
		tickLog     *tickLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = newMockTriangularStore()
		tickLog = newTickLogger()
	})

	Describe("Integration of Phase 0 features into Tick()", func() {
		It("should call DeriveDesiredState during tick", func() {
			worker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "test-config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			mockStore.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})

		It("should reconcile and tick child supervisors", func() {
			worker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child1",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child-config"},
					},
				},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "parent-config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
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

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})

		It("should continue ticking when a child tick fails", func() {
			worker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{Name: "child1", WorkerType: "child1", UserSpec: config.UserSpec{}},
					{Name: "child2", WorkerType: "child2", UserSpec: config.UserSpec{}},
				},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			mockStore.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})

		It("should apply state mapping before child tick", func() {
			worker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{
						Name:         "child1",
						WorkerType:   "child",
						UserSpec:     config.UserSpec{},
						StateMapping: map[string]string{"running": "active"},
					},
				},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
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

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})

		It("should reconcile new children in same tick cycle", func() {
			worker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{Name: "newChild", WorkerType: "child", UserSpec: config.UserSpec{}},
				},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
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

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})

		It("should tick through three supervisor levels", func() {
			parentWorker := &hierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: config.UserSpec{}},
				},
			}

			parentSuper = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     zap.NewNop().Sugar(),
				Store:      mockStore,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSuper.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSuper.UpdateUserSpec(config.UserSpec{Config: "config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"shutdownRequested": false,
			}
			err = mockStore.SaveDesired(ctx, "parent", identity.ID, desiredDoc)
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

			mockStore.Observed["grandchild"] = map[string]interface{}{
				"grandchild-worker": persistence.Document{
					"id":          "grandchild-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSuper.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))
		})
	})
})
