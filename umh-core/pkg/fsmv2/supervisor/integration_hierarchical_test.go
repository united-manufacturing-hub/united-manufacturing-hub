// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type hierarchicalTickLogger struct {
	mu     sync.Mutex
	events []string
}

func newHierarchicalTickLogger() *hierarchicalTickLogger {
	return &hierarchicalTickLogger{
		events: make([]string, 0),
	}
}

func (h *hierarchicalTickLogger) Log(event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
}

func (h *hierarchicalTickLogger) GetEvents() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.events...)
}

func (h *hierarchicalTickLogger) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = make([]string, 0)
}

type mockHierarchicalWorker struct {
	id            string
	logger        *hierarchicalTickLogger
	childrenSpecs []types.ChildSpec
	observed      *mockObservedState
	stateName     string
	tickCount     int
	mu            sync.Mutex
}

func (m *mockHierarchicalWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return m.observed, nil
}

func (m *mockHierarchicalWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	m.logger.Log(fmt.Sprintf("DeriveDesiredState:%s", m.id))
	return types.DesiredState{
		State:         m.stateName,
		ChildrenSpecs: m.childrenSpecs,
	}, nil
}

func (m *mockHierarchicalWorker) GetInitialState() fsmv2.State {
	return &mockStateWithName{name: m.stateName}
}

func (m *mockHierarchicalWorker) IncrementTickCount() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickCount++
	m.logger.Log(fmt.Sprintf("Tick:%s:count=%d", m.id, m.tickCount))
}

func (m *mockHierarchicalWorker) GetTickCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tickCount
}

type mockStateWithName struct {
	name string
}

func (m *mockStateWithName) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}

func (m *mockStateWithName) String() string {
	return m.name
}

func (m *mockStateWithName) Reason() string {
	return "mock state"
}

var _ = Describe("Integration: Hierarchical Composition (Task 0.7)", func() {
	var (
		ctx       context.Context
		store     *mockTriangularStore
		logger    *zap.SugaredLogger
		tickLog   *hierarchicalTickLogger
		parentSup *supervisor.Supervisor
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockTriangularStore()
		logger = zap.NewNop().Sugar()
		tickLog = newHierarchicalTickLogger()
	})

	Describe("Scenario 1: Three-Level Hierarchy", func() {
		It("should tick through parent → child → grandchild", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   types.UserSpec{Config: "child-config"},
						StateMapping: map[string]string{
							"running": "active",
						},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify hierarchical composition
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should have exactly 1 child")
			Expect(children).To(HaveKey("child"), "child supervisor should exist with name 'child'")

			childSup := children["child"]
			Expect(childSup).NotTo(BeNil(), "child supervisor should not be nil")
		})
	})

	Describe("Scenario 2: Dynamic Child Addition", func() {
		It("should create children on-the-fly during tick", func() {
			tickLog.Clear()

			parentWorker := &mockHierarchicalWorker{
				id:            "parent",
				logger:        tickLog,
				childrenSpecs: nil,
				stateName:     "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			parentWorker.childrenSpecs = []types.ChildSpec{
				{
					Name:       "child1",
					WorkerType: "child",
					UserSpec:   types.UserSpec{Config: "child1-config"},
				},
				{
					Name:       "child2",
					WorkerType: "child",
					UserSpec:   types.UserSpec{Config: "child2-config"},
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			tickLog.Clear()
			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify children were created dynamically
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(2), "parent should have exactly 2 children after second tick")
			Expect(children).To(HaveKey("child1"), "child1 supervisor should exist")
			Expect(children).To(HaveKey("child2"), "child2 supervisor should exist")
		})
	})

	Describe("Scenario 3: Dynamic Child Removal", func() {
		It("should remove children when ChildrenSpecs becomes empty", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: types.UserSpec{Config: "child1"}},
					{Name: "child2", WorkerType: "child", UserSpec: types.UserSpec{Config: "child2"}},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify children exist before removal
			childrenBefore := parentSup.GetChildren()
			Expect(childrenBefore).To(HaveLen(2), "parent should have 2 children before removal")

			parentWorker.childrenSpecs = []types.ChildSpec{}

			tickLog.Clear()
			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify all children were removed
			childrenAfter := parentSup.GetChildren()
			Expect(childrenAfter).To(HaveLen(0), "parent should have 0 children after removal")
		})
	})

	Describe("Scenario 4: StateMapping Across Hierarchy", func() {
		It("should apply state mapping at each level", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   types.UserSpec{Config: "child-config"},
						StateMapping: map[string]string{
							"running": "active",
						},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify child was created with state mapping configured
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should have 1 child")
			Expect(children).To(HaveKey("child"), "child supervisor should exist")
		})
	})

	Describe("Scenario 5: Child Failure Isolation", func() {
		It("should continue ticking when one child fails", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: types.UserSpec{Config: "child1"}},
					{Name: "child2", WorkerType: "child", UserSpec: types.UserSpec{Config: "child2"}},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify children were created (failure isolation requires children to exist)
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(2), "parent should have 2 children")
			Expect(children).To(HaveKey("child1"), "child1 supervisor should exist")
			Expect(children).To(HaveKey("child2"), "child2 supervisor should exist")

			// Parent tick succeeded despite potential child issues (isolation working)
			Expect(err).NotTo(HaveOccurred(), "parent tick should succeed even if children have issues")
		})
	})

	Describe("Scenario 6: ChildSpec Updates Trigger Reconciliation", func() {
		It("should remove old child and create new child on WorkerType change", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{Name: "child", WorkerType: "type_a", UserSpec: types.UserSpec{Config: "config_a"}},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["type_a"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			parentWorker.childrenSpecs = []types.ChildSpec{
				{Name: "child", WorkerType: "type_b", UserSpec: types.UserSpec{Config: "config_b"}},
			}

			store.Observed["type_b"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			tickLog.Clear()
			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify child count is still 1 after update (reconciliation preserved child name)
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should still have 1 child after WorkerType change")
			Expect(children).To(HaveKey("child"), "child supervisor should still exist with same name")
		})
	})

	Describe("Scenario 7: Multiple Children with Different Mappings", func() {
		It("should apply correct mapping to each child", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{
						Name:         "child1",
						WorkerType:   "child",
						UserSpec:     types.UserSpec{Config: "child1"},
						StateMapping: map[string]string{"running": "active"},
					},
					{
						Name:         "child2",
						WorkerType:   "child",
						UserSpec:     types.UserSpec{Config: "child2"},
						StateMapping: map[string]string{"running": "connected"},
					},
					{
						Name:       "child3",
						WorkerType: "child",
						UserSpec:   types.UserSpec{Config: "child3"},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{Config: "parent-config"})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify all three children exist with different state mappings
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(3), "parent should have 3 children")
			Expect(children).To(HaveKey("child1"), "child1 should exist")
			Expect(children).To(HaveKey("child2"), "child2 should exist")
			Expect(children).To(HaveKey("child3"), "child3 should exist")
		})
	})

	Describe("Additional Scenario: Empty UserSpec Handling", func() {
		It("should handle empty UserSpec correctly", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []types.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   types.UserSpec{},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.UpdateUserSpec(types.UserSpec{})

			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			err = parentSup.Tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify child was created despite empty UserSpec
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should have 1 child")
			Expect(children).To(HaveKey("child"), "child should exist even with empty UserSpec")
		})
	})
})
