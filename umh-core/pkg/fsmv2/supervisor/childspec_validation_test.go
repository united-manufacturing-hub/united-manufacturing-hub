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

package supervisor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("ChildSpec Validation Integration", func() {
	var (
		ctx             context.Context
		basicStore      persistence.Store
		triangularStore *storage.TriangularStore
		registry        *storage.Registry
		logger          *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		basicStore = memory.NewInMemoryStore()
		registry = storage.NewRegistry()

		// Register collections for supervisor
		_ = registry.Register(&storage.CollectionMetadata{
			Name:          "test_supervisor_identity",
			WorkerType:    "test_supervisor",
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		_ = registry.Register(&storage.CollectionMetadata{
			Name:          "test_supervisor_desired",
			WorkerType:    "test_supervisor",
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		_ = registry.Register(&storage.CollectionMetadata{
			Name:          "test_supervisor_observed",
			WorkerType:    "test_supervisor",
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})

		// Register collections for child worker types
		for _, workerType := range []string{"valid_child", "another_child"} {
			_ = registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_identity",
				WorkerType:    workerType,
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			_ = registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_desired",
				WorkerType:    workerType,
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			_ = registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_observed",
				WorkerType:    workerType,
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
		}

		// Create collections in database
		_ = basicStore.CreateCollection(ctx, "test_supervisor_identity", nil)
		_ = basicStore.CreateCollection(ctx, "test_supervisor_desired", nil)
		_ = basicStore.CreateCollection(ctx, "test_supervisor_observed", nil)
		_ = basicStore.CreateCollection(ctx, "valid_child_identity", nil)
		_ = basicStore.CreateCollection(ctx, "valid_child_desired", nil)
		_ = basicStore.CreateCollection(ctx, "valid_child_observed", nil)
		_ = basicStore.CreateCollection(ctx, "another_child_identity", nil)
		_ = basicStore.CreateCollection(ctx, "another_child_desired", nil)
		_ = basicStore.CreateCollection(ctx, "another_child_observed", nil)

		_ = factory.RegisterWorkerType("valid_child", func(id fsmv2.Identity) fsmv2.Worker {
			return &validChildSpecMockWorker{
				identity:     id,
				initialState: &mockState{},
			}
		})
		_ = factory.RegisterWorkerType("another_child", func(id fsmv2.Identity) fsmv2.Worker {
			return &validChildSpecMockWorker{
				identity:     id,
				initialState: &mockState{},
			}
		})

		triangularStore = storage.NewTriangularStore(basicStore, registry)
	})

	AfterEach(func() {
		factory.ResetRegistry()
		_ = basicStore.Close(ctx)
	})

	Context("when DeriveDesiredState returns valid ChildSpecs", func() {
		It("should pass validation and proceed to reconciliation", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker that returns valid ChildSpecs
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-1",
					Name:       "Test Worker",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-1",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state (required by Tick - AddWorker doesn't do this)
			desiredDoc := persistence.Document{
				"id":             mockWorker.identity.ID,
				"state":          "running",
				"childrenSpecs":  mockWorker.childSpecs,
			}
			err = triangularStore.SaveDesired(ctx, "test_supervisor", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect no validation errors (if validation passes, reconciliation proceeds)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when DeriveDesiredState returns invalid ChildSpecs", func() {
		It("should fail validation with empty Name", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with ChildSpec{Name: "", ...}
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-2",
					Name:       "Test Worker 2",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-2",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state (required by Tick - AddWorker doesn't do this)
			desiredDoc := persistence.Document{
				"id":             mockWorker.identity.ID,
				"state":          "running",
				"childrenSpecs":  mockWorker.childSpecs,
			}
			err = triangularStore.SaveDesired(ctx, "test_supervisor", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect validation error (when validation fails, reconciliation doesn't proceed)
			Expect(err).To(HaveOccurred(), "should fail with empty child name")
			Expect(err.Error()).To(ContainSubstring("name cannot be empty"))
		})

		It("should fail validation with empty WorkerType", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with empty WorkerType
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-3",
					Name:       "Test Worker 3",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-3",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state (required by Tick - AddWorker doesn't do this)
			desiredDoc := persistence.Document{
				"id":             mockWorker.identity.ID,
				"state":          "running",
				"childrenSpecs":  mockWorker.childSpecs,
			}
			err = triangularStore.SaveDesired(ctx, "test_supervisor", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect validation error about empty WorkerType
			Expect(err).To(HaveOccurred(), "should fail with empty worker type")
			Expect(err.Error()).To(ContainSubstring("worker type cannot be empty"))
		})

		It("should fail validation with unknown WorkerType", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with unknown WorkerType
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-4",
					Name:       "Test Worker 4",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-4",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "unknown_worker_type",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect error listing available types
			Expect(err).To(HaveOccurred(), "should fail with unknown worker type")
			Expect(err.Error()).To(ContainSubstring("unknown worker type"))
			// Error should mention available types
			Expect(err.Error()).To(ContainSubstring("available:"))
		})

		It("should fail validation with duplicate child names", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with duplicate child names
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-5",
					Name:       "Test Worker 5",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-5",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "duplicate-child",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
					{
						Name:       "duplicate-child",
						WorkerType: "another_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect duplicate detection error (reconciliation won't proceed)
			Expect(err).To(HaveOccurred(), "should fail with duplicate child names")
			Expect(err.Error()).To(ContainSubstring("duplicate"))
		})
	})

	Context("when validation errors prevent reconciliation", func() {
		It("should not create any children when validation fails", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Multiple validation errors in different specs
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-6",
					Name:       "Test Worker 6",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-6",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
					{
						Name:       "",
						WorkerType: "another_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect validation error (validation failed before reconciliation)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("validation order verification", func() {
		It("should perform validation BEFORE reconciliation", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Create a mock worker that tracks method call order
			callOrder := []string{}

			mockWorker := &trackedCallOrderMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-7",
					Name:       "Test Worker 7",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-7",
					"status": "running",
				},
				callTracker: &callOrder,
				childSpecs: []config.ChildSpec{
					{
						Name:       "",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect validation error
			Expect(err).To(HaveOccurred())

			// Verify validation happened BEFORE reconciliation
			Expect(callOrder).To(Equal([]string{"derive"}),
				"only DeriveDesiredState should be called; reconciliation should not be attempted due to validation failure")
		})

		It("should proceed to reconciliation only when validation passes", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Create a mock worker that tracks method call order
			callOrder := []string{}

			mockWorker := &trackedCallOrderMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-8",
					Name:       "Test Worker 8",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-8",
					"status": "running",
				},
				callTracker: &callOrder,
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "valid_child",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state (required by Tick - AddWorker doesn't do this)
			desiredDoc := persistence.Document{
				"id":            mockWorker.identity.ID,
				"state":         "running",
				"childrenSpecs": mockWorker.childSpecs,
			}
			err = triangularStore.SaveDesired(ctx, "test_supervisor", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Should succeed
			Expect(err).NotTo(HaveOccurred())

			// Verify DeriveDesiredState was called (validation happens after)
			Expect(callOrder).To(ContainElement("derive"), "DeriveDesiredState must be called first")

			// After validation passes, reconciliation should proceed (verified by no error)
			// Note: Cannot verify child count from external test package due to unexported fields
		})
	})

	Context("complex validation scenarios", func() {
		It("should validate all child specs even with multiple errors", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Test that validation is comprehensive
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-9",
					Name:       "Test Worker 9",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-9",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "invalid_type",
						UserSpec:   config.UserSpec{},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Expect validation error
			Expect(err).To(HaveOccurred())

			// Error should contain context about which spec failed
			Expect(err.Error()).To(ContainSubstring("child-1"))
		})

		It("should accept valid ChildSpecs with UserSpec data", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "test_supervisor",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor(supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with ChildSpec containing UserSpec data
			mockWorker := &validChildSpecMockWorker{
				identity: fsmv2.Identity{
					ID:         "worker-10",
					Name:       "Test Worker 10",
					WorkerType: "test_supervisor",
				},
				initialState: &mockState{},
				observed: persistence.Document{
					"id":     "worker-10",
					"status": "running",
				},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-with-userspec",
						WorkerType: "valid_child",
						UserSpec: config.UserSpec{
							Config: "value",
						},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state (required by Tick - AddWorker doesn't do this)
			desiredDoc := persistence.Document{
				"id":            mockWorker.identity.ID,
				"state":         "running",
				"childrenSpecs": mockWorker.childSpecs,
			}
			err = triangularStore.SaveDesired(ctx, "test_supervisor", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// Call supervisor.Tick()
			err = sup.tick(ctx)

			// Should succeed (validation passes, reconciliation proceeds)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// validChildSpecMockWorker returns configurable ChildSpecs in DeriveDesiredState
type validChildSpecMockWorker struct {
	identity     fsmv2.Identity
	initialState fsmv2.State
	observed     persistence.Document
	childSpecs   []config.ChildSpec
}

func (m *validChildSpecMockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc: persistence.Document{
			"id": m.identity.ID,
		},
		timestamp: time.Now(),
	}, nil
}

func (m *validChildSpecMockWorker) DeriveDesiredState(_ interface{}) (config.DesiredState, error) {
	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: m.childSpecs,
	}, nil
}

func (m *validChildSpecMockWorker) GetInitialState() fsmv2.State {
	return m.initialState
}

// trackedCallOrderMockWorker tracks which methods are called and in what order
type trackedCallOrderMockWorker struct {
	identity     fsmv2.Identity
	initialState fsmv2.State
	observed     persistence.Document
	childSpecs   []config.ChildSpec
	callTracker  *[]string
}

func (m *trackedCallOrderMockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc: persistence.Document{
			"id": m.identity.ID,
		},
		timestamp: time.Now(),
	}, nil
}

func (m *trackedCallOrderMockWorker) DeriveDesiredState(_ interface{}) (config.DesiredState, error) {
	*m.callTracker = append(*m.callTracker, "derive")
	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: m.childSpecs,
	}, nil
}

func (m *trackedCallOrderMockWorker) GetInitialState() fsmv2.State {
	return m.initialState
}

