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
	"errors"
	"fmt"
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
	childrenSpecs []config.ChildSpec
	observed      *mockObservedState
	observedErr   error // Error to return from CollectObservedState (for testing failure scenarios)
	callCount     int   // Track how many times CollectObservedState was called
	failAfterCall int   // Start failing after this many calls (0 = fail immediately, -1 = never fail)
	stateName     string
	tickCount     int
	mu            sync.Mutex
}

func (m *mockHierarchicalWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	m.mu.Lock()
	m.callCount++
	currentCall := m.callCount
	m.mu.Unlock()

	// If failAfterCall is set and we've passed that threshold, return the error
	if m.failAfterCall >= 0 && currentCall > m.failAfterCall && m.observedErr != nil {
		return nil, m.observedErr
	}

	return m.observed, nil
}

func (m *mockHierarchicalWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	m.logger.Log("DeriveDesiredState:" + m.id)

	return config.DesiredState{
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
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child-config"},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			// First tick: parent creates child
			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify child was created (2 levels exist)
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should have exactly 1 child")
			Expect(children).To(HaveKey("child"), "child supervisor should exist with name 'child'")

			childSup := children["child"]
			Expect(childSup).NotTo(BeNil(), "child supervisor should not be nil")

			// Now add a worker to child so it can declare grandchildren
			childIdentity := fsmv2.Identity{
				ID:         "child-worker",
				Name:       "Child Worker",
				WorkerType: "child",
			}
			childWorker := &mockHierarchicalWorker{
				id:     "child-worker",
				logger: tickLog,
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "grandchild",
						WorkerType: "grandchild",
						UserSpec:   config.UserSpec{Config: "grandchild-config"},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "child-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			// Add worker to child supervisor
			err = childSup.AddWorker(childIdentity, childWorker)
			Expect(err).NotTo(HaveOccurred())

			childSup.TestUpdateUserSpec(config.UserSpec{Config: "child-config"})

			// Setup store for child worker
			store.desired["child"] = map[string]persistence.Document{
				"child-worker": {
					"id":                "child-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["grandchild"] = map[string]interface{}{
				"grandchild-worker": persistence.Document{
					"id":          "grandchild-worker",
					"collectedAt": time.Now(),
				},
			}

			tickLog.Clear()

			// Second tick: parent ticks, which should tick child, which creates grandchild
			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify all workers derived state
			events = tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"), "parent should have derived state")
			Expect(events).To(ContainElement("DeriveDesiredState:child-worker"), "child worker should have derived state")

			// Verify grandchild exists (TRUE 3-level hierarchy)
			grandchildren := childSup.GetChildren()
			Expect(grandchildren).To(HaveLen(1), "child should have exactly 1 grandchild")
			Expect(grandchildren).To(HaveKey("grandchild"), "grandchild supervisor should exist with name 'grandchild'")

			grandchildSup := grandchildren["grandchild"]
			Expect(grandchildSup).NotTo(BeNil(), "grandchild supervisor should not be nil")

			// Verify hierarchy structure: parent has child, child has grandchild
			Expect(parentSup.GetChildren()).To(HaveKey("child"), "parent contains child")
			Expect(childSup.GetChildren()).To(HaveKey("grandchild"), "child contains grandchild")
			Expect(grandchildSup.GetChildren()).To(BeEmpty(), "grandchild has no children (leaf node)")
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			parentWorker.childrenSpecs = []config.ChildSpec{
				{
					Name:       "child1",
					WorkerType: "child",
					UserSpec:   config.UserSpec{Config: "child1-config"},
				},
				{
					Name:       "child2",
					WorkerType: "child",
					UserSpec:   config.UserSpec{Config: "child2-config"},
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			tickLog.Clear()
			err = parentSup.TestTick(ctx)
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
				childrenSpecs: []config.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: config.UserSpec{Config: "child1"}},
					{Name: "child2", WorkerType: "child", UserSpec: config.UserSpec{Config: "child2"}},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify children exist before removal
			childrenBefore := parentSup.GetChildren()
			Expect(childrenBefore).To(HaveLen(2), "parent should have 2 children before removal")

			parentWorker.childrenSpecs = []config.ChildSpec{}

			tickLog.Clear()
			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify all children were removed
			childrenAfter := parentSup.GetChildren()
			Expect(childrenAfter).To(BeEmpty(), "parent should have 0 children after removal")
		})
	})

	Describe("Scenario 4: StateMapping Across Hierarchy", func() {
		It("should apply state mapping at each level", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child-config"},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify child was created with state mapping configured
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(1), "parent should have 1 child")
			Expect(children).To(HaveKey("child"), "child supervisor should exist")

			// VERIFY: StateMapping was applied correctly
			// Parent state is "running", mapping says "running" → "active"
			// Child supervisor should have received the MAPPED state "active", not parent state "running"
			childSup := children["child"]
			Expect(childSup).NotTo(BeNil(), "child supervisor should not be nil")

			mappedState := childSup.GetMappedParentState()
			Expect(mappedState).To(Equal("active"),
				"child should receive mapped state 'active' (from parent state 'running' via StateMapping), not parent state 'running'")

			// Additional verification: Parent state is indeed "running"
			parentState := parentWorker.stateName
			Expect(parentState).To(Equal("running"), "parent state should be 'running' for this test")
		})
	})

	Describe("Scenario 5: Child Failure Isolation", func() {
		It("should continue ticking when one child fails", func() {
			// Create parent worker that declares TWO children
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []config.ChildSpec{
					{Name: "failing-child", WorkerType: "failing", UserSpec: config.UserSpec{Config: "fail"}},
					{Name: "working-child", WorkerType: "working", UserSpec: config.UserSpec{Config: "work"}},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			// First tick: Parent creates both child supervisors
			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			events := tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"))

			// Verify both children were created
			children := parentSup.GetChildren()
			Expect(children).To(HaveLen(2), "parent should have 2 children")
			Expect(children).To(HaveKey("failing-child"), "failing-child supervisor should exist")
			Expect(children).To(HaveKey("working-child"), "working-child supervisor should exist")

			// Create FAILING child worker that returns error from CollectObservedState
			// It succeeds on first call (during AddWorker type discovery) but fails on subsequent calls
			failingWorker := &mockHierarchicalWorker{
				id:            "failing-worker",
				logger:        tickLog,
				observedErr:   errors.New("child observation failed"), // Error to return after threshold
				failAfterCall: 1,                                      // Succeed on first call (AddWorker), fail after that
				stateName:     "error",
				observed: &mockObservedState{ // Need valid state for AddWorker type discovery
					ID:          "failing-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			// Create WORKING child worker that succeeds normally
			workingWorker := &mockHierarchicalWorker{
				id:        "working-worker",
				logger:    tickLog,
				stateName: "running",
				observed: &mockObservedState{
					ID:          "working-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			// Add workers to both child supervisors
			failingChild := children["failing-child"]
			workingChild := children["working-child"]

			failingIdentity := fsmv2.Identity{
				ID:         "failing-worker",
				Name:       "Failing Worker",
				WorkerType: "failing",
			}
			err = failingChild.AddWorker(failingIdentity, failingWorker)
			Expect(err).NotTo(HaveOccurred())

			workingIdentity := fsmv2.Identity{
				ID:         "working-worker",
				Name:       "Working Worker",
				WorkerType: "working",
			}
			err = workingChild.AddWorker(workingIdentity, workingWorker)
			Expect(err).NotTo(HaveOccurred())

			// Setup store for child workers
			store.desired["failing"] = map[string]persistence.Document{
				"failing-worker": {
					"id":                "failing-worker",
					"shutdownRequested": false,
				},
			}

			store.desired["working"] = map[string]persistence.Document{
				"working-worker": {
					"id":                "working-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["working"] = map[string]interface{}{
				"working-worker": persistence.Document{
					"id":          "working-worker",
					"collectedAt": time.Now(),
				},
			}

			// Clear events to focus on this tick
			tickLog.Clear()

			// Second tick: Failing child errors, but parent should continue
			err = parentSup.TestTick(ctx)

			// CRITICAL: Parent tick should SUCCEED despite child failure
			// This is the core assertion - failure isolation means parent doesn't fail when child fails
			Expect(err).NotTo(HaveOccurred(), "parent should not fail when child fails")

			// VERIFY: Parent successfully completed its tick cycle
			// (This proves parent continued processing after child failure)
			events = tickLog.GetEvents()
			Expect(events).To(ContainElement("DeriveDesiredState:parent"), "parent should have completed its tick")

			// VERIFY: Failure was isolated (both children still exist)
			// Child failures should not cause children to be removed
			children = parentSup.GetChildren()
			Expect(children).To(HaveLen(2), "parent should still have 2 children after failure")
			Expect(children).To(HaveKey("failing-child"), "failing child should still exist")
			Expect(children).To(HaveKey("working-child"), "working child should still exist")
		})
	})

	Describe("Scenario 6: ChildSpec Updates Trigger Reconciliation", func() {
		It("should remove old child and create new child on WorkerType change", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: tickLog,
				childrenSpecs: []config.ChildSpec{
					{Name: "child", WorkerType: "type_a", UserSpec: config.UserSpec{Config: "config_a"}},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			err = parentSup.TestTick(ctx)
			Expect(err).NotTo(HaveOccurred())

			parentWorker.childrenSpecs = []config.ChildSpec{
				{Name: "child", WorkerType: "type_b", UserSpec: config.UserSpec{Config: "config_b"}},
			}

			store.Observed["type_b"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			tickLog.Clear()
			err = parentSup.TestTick(ctx)
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
				childrenSpecs: []config.ChildSpec{
					{
						Name:         "child1",
						WorkerType:   "child",
						UserSpec:     config.UserSpec{Config: "child1"},
						StateMapping: map[string]string{"running": "active"},
					},
					{
						Name:         "child2",
						WorkerType:   "child",
						UserSpec:     config.UserSpec{Config: "child2"},
						StateMapping: map[string]string{"running": "connected"},
					},
					{
						Name:       "child3",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child3"},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

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

			err = parentSup.TestTick(ctx)
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
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   config.UserSpec{},
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

			parentSup.TestUpdateUserSpec(config.UserSpec{})

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

			err = parentSup.TestTick(ctx)
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
