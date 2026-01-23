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

// Package supervisor internal tests - these tests need access to unexported methods like tick().
// Tests use package supervisor (internal) while external tests use package supervisor_test.
package supervisor

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func registerInternalTestWorkerFactories() {
	workerTypes := []string{
		"child",
		"parent",
		"mqtt_client",
		"mqtt_broker",
		"opcua_client",
		"opcua_server",
		"s7comm_client",
		"modbus_client",
		"http_client",
		"child1",
		"child2",
		"grandchild",
		"valid_child",
		"another_child",
		"working-child",
		"failing-child",
		"working",
		"failing",
		"container",
		"s6_service",
		"type_a",
		"type_b",
	}

	for _, workerType := range workerTypes {
		wt := workerType
		// Register worker factory
		_ = factory.RegisterFactoryByType(wt, func(identity deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &TestWorkerWithType{
				WorkerType: wt,
			}
		})

		// Register supervisor factory for hierarchical composition
		_ = factory.RegisterSupervisorFactoryByType(wt, func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(Config)

			return NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
		})
	}
}

// internalMockWorker is a mock worker for internal supervisor tests.
// NOTE: This is different from mockWorker in supervisor_suite_test.go (external tests).
type internalMockWorker struct {
	identity     deps.Identity
	initialState fsmv2.State[any, any]
	observed     persistence.Document
}

func (m *internalMockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *internalMockWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{State: "running"}, nil
}

func (m *internalMockWorker) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}

// mockObservedState is shared across internal supervisor tests (package supervisor).
// NOTE: There is a different mockObservedState in supervisor_suite_test.go (package supervisor_test).
type mockObservedState struct {
	doc       persistence.Document
	timestamp time.Time
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.timestamp
}

func (m *mockObservedState) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.doc)
}

// mockDesiredState is shared across internal supervisor tests (package supervisor).
type mockDesiredState struct {
	State string
}

func (m *mockDesiredState) IsShutdownRequested() bool {
	return false
}

func (m *mockDesiredState) SetShutdownRequested(_ bool) {
}

func (m *mockDesiredState) GetState() string {
	if m.State == "" {
		return "running"
	}

	return m.State
}

// mockState is shared across internal supervisor tests (package supervisor).
// NOTE: There is a different mockState in supervisor_suite_test.go (package supervisor_test).
type mockState struct {
	name   string
	reason string
}

func (m *mockState) String() string {
	return m.name
}

func (m *mockState) Reason() string {
	return m.reason
}

func (m *mockState) Next(_ any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	return m, fsmv2.SignalNone, nil
}

// internalMockWorkerWithChildren is a mock worker that returns configurable ChildSpecs.
type internalMockWorkerWithChildren struct {
	identity      deps.Identity
	initialState  fsmv2.State[any, any]
	observed      persistence.Document
	childrenSpecs []config.ChildSpec
}

func (m *internalMockWorkerWithChildren) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *internalMockWorkerWithChildren) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		State:         "running",
		ChildrenSpecs: m.childrenSpecs,
	}, nil
}

func (m *internalMockWorkerWithChildren) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}

var _ = Describe("Supervisor Internal", func() {
	var (
		ctx    context.Context
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		registerInternalTestWorkerFactories()
	})

	AfterEach(func() {
		factory.ResetRegistry()
	})

	Describe("TriangularStore Integration", func() {
		It("should use TriangularStore instead of basic Store", func() {
			basicStore := memory.NewInMemoryStore()
			defer func() { _ = basicStore.Close(ctx) }()

			Expect(basicStore.CreateCollection(ctx, "test_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_observed", nil)).To(Succeed())

			triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

			supervisorCfg := Config{
				WorkerType: "test",
				Store:      triangularStore,
				Logger:     logger,
			}

			supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			Expect(supervisor).NotTo(BeNil())
		})

		It("should save identity to TriangularStore when adding worker", func() {
			basicStore := memory.NewInMemoryStore()
			defer func() { _ = basicStore.Close(ctx) }()

			Expect(basicStore.CreateCollection(ctx, "test_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_observed", nil)).To(Succeed())

			triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

			supervisorCfg := Config{
				WorkerType: "test",
				Store:      triangularStore,
				Logger:     logger,
			}

			supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "worker-1",
				Name:       "Test Worker",
				WorkerType: "test",
			}

			worker := &internalMockWorker{
				identity:     identity,
				initialState: &mockState{name: "initial"},
				observed: persistence.Document{
					"id":     "worker-1",
					"status": "running",
				},
			}

			err := supervisor.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			loadedIdentity, err := triangularStore.LoadIdentity(ctx, "test", "worker-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedIdentity["id"]).To(Equal("worker-1"))
			Expect(loadedIdentity["name"]).To(Equal("Test Worker"))
		})

		It("should load snapshot from TriangularStore during tick", func() {
			basicStore := memory.NewInMemoryStore()
			defer func() { _ = basicStore.Close(ctx) }()

			Expect(basicStore.CreateCollection(ctx, "test_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "test_observed", nil)).To(Succeed())

			triangularStore := storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

			supervisorCfg := Config{
				WorkerType: "test",
				Store:      triangularStore,
				Logger:     logger,
			}

			supervisor := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "worker-1",
				Name:       "Test Worker",
				WorkerType: "test",
			}

			worker := &internalMockWorker{
				identity:     identity,
				initialState: &mockState{name: "initial"},
				observed: persistence.Document{
					"id":     "worker-1",
					"status": "running",
				},
			}

			err := supervisor.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			_, err = triangularStore.SaveDesired(ctx, "test", "worker-1", persistence.Document{
				"id":     "worker-1",
				"config": "production",
			})
			Expect(err).NotTo(HaveOccurred())

			err = supervisor.tick(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ChildStartStates", func() {
		var (
			basicStore      *memory.InMemoryStore
			triangularStore *storage.TriangularStore
		)

		BeforeEach(func() {
			basicStore = memory.NewInMemoryStore()

			Expect(basicStore.CreateCollection(ctx, "parent_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "parent_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "parent_observed", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child_observed", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "mqtt_client_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "mqtt_client_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "mqtt_client_observed", nil)).To(Succeed())

			triangularStore = storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())
		})

		AfterEach(func() {
			_ = basicStore.Close(ctx)
		})

		It("should run child when parent state is in ChildStartStates list", func() {
			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: []string{"active"},
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "active"},
				observed:      persistence.Document{"id": "parent-1", "status": "active"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			Expect(children).To(HaveLen(1))

			child, exists := children["child-1"]
			Expect(exists).To(BeTrue())

			typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())

			mappedState := typedChild.GetMappedParentState()
			Expect(mappedState).To(Equal("running"))
		})

		It("should always run child when ChildStartStates is nil", func() {
			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: nil,
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "running"},
				observed:      persistence.Document{"id": "parent-1", "status": "running"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			child, exists := children["child-1"]
			Expect(exists).To(BeTrue())

			typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())

			mappedState := typedChild.GetMappedParentState()
			Expect(mappedState).To(Equal("running"))
		})

		It("should stop child when parent state is not in ChildStartStates list", func() {
			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: []string{"active"},
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "unknown"},
				observed:      persistence.Document{"id": "parent-1", "status": "unknown"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			child, exists := children["child-1"]
			Expect(exists).To(BeTrue())

			typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())

			mappedState := typedChild.GetMappedParentState()
			Expect(mappedState).To(Equal("stopped"))
		})

		It("should handle multiple children with different ChildStartStates", func() {
			// Add additional collections for child-2 and child-3
			Expect(basicStore.CreateCollection(ctx, "child-2_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child-2_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child-2_observed", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child-3_identity", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child-3_desired", nil)).To(Succeed())
			Expect(basicStore.CreateCollection(ctx, "child-3_observed", nil)).To(Succeed())

			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: []string{"active"},
				},
				{
					Name:             "child-2",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "address: 192.168.1.100:502"},
					ChildStartStates: []string{"idle"},
				},
				{
					Name:             "child-3",
					WorkerType:       "child",
					UserSpec:         config.UserSpec{Config: "endpoint: opc.tcp://localhost:4840"},
					ChildStartStates: nil,
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "active"},
				observed:      persistence.Document{"id": "parent-1", "status": "active"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			Expect(children).To(HaveLen(3))

			// child-1: ChildStartStates: ["active"], parent is "active" → runs
			child1, exists := children["child-1"]
			Expect(exists).To(BeTrue())
			typedChild1, ok := child1.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())
			Expect(typedChild1.GetMappedParentState()).To(Equal("running"))

			// child-2: ChildStartStates: ["idle"], parent is "active" → stopped
			child2, exists := children["child-2"]
			Expect(exists).To(BeTrue())
			typedChild2, ok := child2.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())
			Expect(typedChild2.GetMappedParentState()).To(Equal("stopped"))

			// child-3: ChildStartStates: nil (empty) → always runs
			child3, exists := children["child-3"]
			Expect(exists).To(BeTrue())
			typedChild3, ok := child3.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())
			Expect(typedChild3.GetMappedParentState()).To(Equal("running"))
		})

		It("should always run child when ChildStartStates is empty slice", func() {
			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "mqtt_client",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: []string{},
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "running"},
				observed:      persistence.Document{"id": "parent-1", "status": "running"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			child, exists := children["child-1"]
			Expect(exists).To(BeTrue())

			typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())

			mappedState := typedChild.GetMappedParentState()
			Expect(mappedState).To(Equal("running"))
		})

		It("should always run child when ChildStartStates is nil (starting state)", func() {
			supervisorCfg := Config{
				WorkerType: "parent",
				Store:      triangularStore,
				Logger:     logger,
			}

			parent := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)

			identity := deps.Identity{
				ID:         "parent-1",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}

			childSpecs := []config.ChildSpec{
				{
					Name:             "child-1",
					WorkerType:       "mqtt_client",
					UserSpec:         config.UserSpec{Config: "url: tcp://localhost:1883"},
					ChildStartStates: nil,
				},
			}

			worker := &internalMockWorkerWithChildren{
				identity:      identity,
				initialState:  &mockState{name: "starting"},
				observed:      persistence.Document{"id": "parent-1", "status": "starting"},
				childrenSpecs: childSpecs,
			}

			err := parent.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			err = parent.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			children := parent.GetChildren()
			child, exists := children["child-1"]
			Expect(exists).To(BeTrue())

			typedChild, ok := child.(*Supervisor[*TestObservedState, *TestDesiredState])
			Expect(ok).To(BeTrue())

			mappedState := typedChild.GetMappedParentState()
			Expect(mappedState).To(Equal("running"))
		})
	})

	Describe("Test Specs (Future Implementation)", func() {
		It("should test action behavior during circuit breaker", func() {
			Skip("Task 3.2: Test spec for action-child lifecycle behavior")
		})

		It("should test collector continues during circuit open", func() {
			Skip("Task 3.4: Test spec for collector-circuit independence")
		})

		It("should test variable propagation through 3-level hierarchy", func() {
			Skip("Task 3.6: Test spec for variable flow through supervisor hierarchy")
		})

		It("should test location hierarchy computation", func() {
			Skip("Task 3.7: Test spec for location hierarchy computation")
		})

		It("should test ProtocolConverter end-to-end tick", func() {
			Skip("Task 3.8: Test spec for ProtocolConverter end-to-end integration")
		})
	})
})
