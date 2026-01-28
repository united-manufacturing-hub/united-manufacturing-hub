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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Incremental Spec Validation", func() {
	var (
		ctx             context.Context
		basicStore      persistence.Store
		triangularStore *storage.TriangularStore
		logger          *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		basicStore = memory.NewInMemoryStore()

		_ = factory.RegisterFactoryByType("incremental_child", func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return &incrementalValidationMockWorker{
				identity:     id,
				initialState: &mockState{},
			}
		})

		triangularStore = storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())
	})

	AfterEach(func() {
		factory.ResetRegistry()
		_ = basicStore.Close(ctx)
	})

	Context("when specs have not changed between ticks", func() {
		It("should skip validation for unchanged specs on subsequent ticks", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "incremental_test",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with valid ChildSpecs
			mockWorker := &incrementalValidationMockWorker{
				identity: deps.Identity{
					ID:         "worker-inc-1",
					Name:       "Incremental Test Worker",
					WorkerType: "incremental_test",
				},
				initialState: &mockState{},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "incremental_child",
						UserSpec:   config.UserSpec{Config: "test-config"},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// Initialize desired state
			desiredDoc := persistence.Document{
				"id":            mockWorker.identity.ID,
				"state":         "running",
				"childrenSpecs": mockWorker.childSpecs,
			}
			_, err = triangularStore.SaveDesired(ctx, "incremental_test", mockWorker.identity.ID, desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			// First tick - all specs validated
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify the validated spec hashes cache is populated
			Expect(sup.validatedSpecHashes).NotTo(BeNil())
			Expect(sup.validatedSpecHashes).To(HaveLen(1))
			Expect(sup.validatedSpecHashes).To(HaveKey("child-1"))

			// Second tick with same specs - should skip validation
			// (verified by cache being unchanged and no errors)
			firstHash := sup.validatedSpecHashes["child-1"]
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Hash should be unchanged (same spec)
			Expect(sup.validatedSpecHashes["child-1"]).To(Equal(firstHash))
		})
	})

	Context("when specs change between ticks", func() {
		It("should re-validate only changed specs", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "incremental_test",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with valid ChildSpecs
			mockWorker := &dynamicChildSpecMockWorker{
				identity: deps.Identity{
					ID:         "worker-inc-2",
					Name:       "Dynamic Test Worker",
					WorkerType: "incremental_test",
				},
				initialState: &mockState{},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "incremental_child",
						UserSpec:   config.UserSpec{Config: "original-config"},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// First tick
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Record the original hash
			originalHash := sup.validatedSpecHashes["child-1"]
			Expect(originalHash).NotTo(BeEmpty())

			// Change the spec (by modifying the mock's childSpecs)
			mockWorker.childSpecs = []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "incremental_child",
					UserSpec:   config.UserSpec{Config: "modified-config"},
				},
			}

			// Update userSpec to force DeriveDesiredState to be called again
			// (supervisor caches desired state by userSpec hash)
			sup.updateUserSpec(config.UserSpec{Config: "changed-config"})

			// Second tick - should detect change and re-validate
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Hash should be different (spec changed)
			Expect(sup.validatedSpecHashes["child-1"]).NotTo(Equal(originalHash))
		})
	})

	Context("when specs are removed", func() {
		It("should clean up cache entries for removed specs", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "incremental_test",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with two ChildSpecs
			mockWorker := &dynamicChildSpecMockWorker{
				identity: deps.Identity{
					ID:         "worker-inc-3",
					Name:       "Cleanup Test Worker",
					WorkerType: "incremental_test",
				},
				initialState: &mockState{},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "incremental_child",
						UserSpec:   config.UserSpec{Config: "config-1"},
					},
					{
						Name:       "child-2",
						WorkerType: "incremental_child",
						UserSpec:   config.UserSpec{Config: "config-2"},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// First tick - both specs validated
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(sup.validatedSpecHashes).To(HaveLen(2))
			Expect(sup.validatedSpecHashes).To(HaveKey("child-1"))
			Expect(sup.validatedSpecHashes).To(HaveKey("child-2"))

			// Remove one child spec
			mockWorker.childSpecs = []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "incremental_child",
					UserSpec:   config.UserSpec{Config: "config-1"},
				},
			}

			// Update userSpec to force DeriveDesiredState to be called again
			sup.updateUserSpec(config.UserSpec{Config: "changed-config"})

			// Second tick - removed spec should be cleaned from cache
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(sup.validatedSpecHashes).To(HaveLen(1))
			Expect(sup.validatedSpecHashes).To(HaveKey("child-1"))
			Expect(sup.validatedSpecHashes).NotTo(HaveKey("child-2"))
		})
	})

	Context("when new specs are added", func() {
		It("should validate only new specs", func() {
			// Create supervisor
			supervisorCfg := Config{
				WorkerType: "incremental_test",
				Store:      triangularStore,
				Logger:     logger,
			}

			sup := NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
			Expect(sup).NotTo(BeNil())

			// Setup worker with one ChildSpec
			mockWorker := &dynamicChildSpecMockWorker{
				identity: deps.Identity{
					ID:         "worker-inc-4",
					Name:       "Add Test Worker",
					WorkerType: "incremental_test",
				},
				initialState: &mockState{},
				childSpecs: []config.ChildSpec{
					{
						Name:       "child-1",
						WorkerType: "incremental_child",
						UserSpec:   config.UserSpec{Config: "config-1"},
					},
				},
			}

			// Add worker to supervisor
			err := sup.AddWorker(mockWorker.identity, mockWorker)
			Expect(err).NotTo(HaveOccurred())

			// First tick
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(sup.validatedSpecHashes).To(HaveLen(1))
			originalHash := sup.validatedSpecHashes["child-1"]

			// Add a new child spec
			mockWorker.childSpecs = []config.ChildSpec{
				{
					Name:       "child-1",
					WorkerType: "incremental_child",
					UserSpec:   config.UserSpec{Config: "config-1"},
				},
				{
					Name:       "child-2",
					WorkerType: "incremental_child",
					UserSpec:   config.UserSpec{Config: "config-2"},
				},
			}

			// Update userSpec to force DeriveDesiredState to be called again
			sup.updateUserSpec(config.UserSpec{Config: "changed-config"})

			// Second tick - should validate only new spec (child-2)
			err = sup.tick(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(sup.validatedSpecHashes).To(HaveLen(2))
			// Original hash should be unchanged (spec not re-validated)
			Expect(sup.validatedSpecHashes["child-1"]).To(Equal(originalHash))
			// New spec should have a hash
			Expect(sup.validatedSpecHashes["child-2"]).NotTo(BeEmpty())
		})
	})
})

// incrementalValidationMockWorker returns configurable ChildSpecs in DeriveDesiredState.
type incrementalValidationMockWorker struct {
	identity     deps.Identity
	initialState fsmv2.State[any, any]
	childSpecs   []config.ChildSpec
}

func (m *incrementalValidationMockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc: persistence.Document{
			"id": m.identity.ID,
		},
	}, nil
}

func (m *incrementalValidationMockWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		BaseDesiredState: config.BaseDesiredState{State: "running"},
		ChildrenSpecs:    m.childSpecs,
	}, nil
}

func (m *incrementalValidationMockWorker) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}

// dynamicChildSpecMockWorker allows changing ChildSpecs between ticks.
type dynamicChildSpecMockWorker struct {
	identity     deps.Identity
	initialState fsmv2.State[any, any]
	childSpecs   []config.ChildSpec
}

func (m *dynamicChildSpecMockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc: persistence.Document{
			"id": m.identity.ID,
		},
	}, nil
}

func (m *dynamicChildSpecMockWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{
		BaseDesiredState: config.BaseDesiredState{State: "running"},
		ChildrenSpecs:    m.childSpecs,
	}, nil
}

func (m *dynamicChildSpecMockWorker) GetInitialState() fsmv2.State[any, any] {
	return m.initialState
}
