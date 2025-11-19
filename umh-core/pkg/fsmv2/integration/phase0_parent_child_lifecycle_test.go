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

package integration_test

import (
	"context"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	parentSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
	childSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
)

var _ = Describe("Phase 0: Parent-Child Lifecycle", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		parentSup         *supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]
		logger            *zap.SugaredLogger
		initialGoroutines int
		store             storage.TriangularStoreInterface
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		basicStore := memory.NewInMemoryStore()

		// Create collections following naming convention: {workerType}_{role}
		err := basicStore.CreateCollection(ctx, storage.DeriveWorkerType[parentSnapshot.ParentObservedState]()+"_identity", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, storage.DeriveWorkerType[parentSnapshot.ParentObservedState]()+"_desired", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, storage.DeriveWorkerType[parentSnapshot.ParentObservedState]()+"_observed", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, storage.DeriveWorkerType[childSnapshot.ChildObservedState]()+"_identity", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, storage.DeriveWorkerType[childSnapshot.ChildObservedState]()+"_desired", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, storage.DeriveWorkerType[childSnapshot.ChildObservedState]()+"_observed", nil)
		if err != nil {
			panic(err)
		}

		store = storage.NewTriangularStore(basicStore)

		factory.ResetRegistry()
	})

	AfterEach(func() {
		cancel()

		Eventually(func() int {
			runtime.GC()
			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Scenario 1: Parent and Child Both Operational", func() {
		It("should start parent and child, both reach operational states, and remain stable", func() {
			By("Registering parent and child worker types")

			// Register worker factories
			err := factory.RegisterFactory[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				return parent.NewParentWorker(identity.ID, identity.Name, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := NewConnectionPool()
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).ToNot(HaveOccurred())

			// Register supervisor factories (needed for child creation via reconcileChildren)
			err = factory.RegisterSupervisorFactory[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState](
				func(cfg interface{}) interface{} {
					supervisorCfg := cfg.(supervisor.Config)
					return supervisor.NewSupervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState](supervisorCfg)
				})
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterSupervisorFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](
				func(cfg interface{}) interface{} {
					supervisorCfg := cfg.(supervisor.Config)
					return supervisor.NewSupervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](supervisorCfg)
				})
			Expect(err).ToNot(HaveOccurred())

			By("Creating parent supervisor")

			parentSup = supervisor.NewSupervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState](supervisor.Config{
				WorkerType: storage.DeriveWorkerType[parentSnapshot.ParentObservedState](),
				Store:      store,
				Logger:     logger,
			})
			Expect(parentSup).NotTo(BeNil())

			By("Creating and adding parent worker instance to supervisor")

			parentWorker := parent.NewParentWorker("parent-001", "Test Parent", logger)

			identity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Test Parent",
				WorkerType: storage.DeriveWorkerType[parentSnapshot.ParentObservedState](),
			}
			err = parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			By("Saving initial desired state to database")

			// Set UserSpec on supervisor so DeriveDesiredState can generate ChildrenSpecs
			parentSup.TestUpdateUserSpec(config.UserSpec{
				Config: "children_count: 1",
			})

			desiredDoc := persistence.Document{
				"id":    "parent-001",
				"state": "running",
			}
			err = store.SaveDesired(ctx, storage.DeriveWorkerType[parentSnapshot.ParentObservedState](), "parent-001", desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			By("Saving initial observed state to database (to avoid Document vs typed struct panic)")

			initialObserved := parentSnapshot.ParentObservedState{
				ID:          "parent-001",
				CollectedAt: time.Now(),
			}
			_, err = store.SaveObserved(ctx, storage.DeriveWorkerType[parentSnapshot.ParentObservedState](), "parent-001", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			By("Starting supervisor properly (collectors + tick loops)")

			done := parentSup.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for parent to reach Running state")

			Eventually(func() string {
				return GetWorkerStateName(parentSup, "parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Waiting for child supervisor to be created and child to reach Connected state")

			var childSup *supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]
			Eventually(func() interface{} {
				children := parentSup.GetChildren()
				for name, child := range children {
					if name == "child-0" {
						if typedChild, ok := child.(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]); ok {
							childSup = typedChild
							return childSup
						}
					}
				}
				return nil
			}, "10s", "100ms").ShouldNot(BeNil())

			By("Verifying child supervisor has worker registered")
			Expect(childSup).ToNot(BeNil(), "child supervisor should exist")
			childWorkers := childSup.GetWorkers()
			Expect(childWorkers).To(HaveLen(1), "child supervisor should have exactly one worker")

			Eventually(func() string {
				if childSup == nil {
					return ""
				}
				return GetWorkerStateName(childSup, "child-0-001")
			}, "10s", "100ms").Should(Equal("Connected"))

			By("Verifying both parent and child remain stable for 10 seconds")

			Consistently(func() string {
				return GetWorkerStateName(parentSup, "parent-001")
			}, "10s", "500ms").Should(Equal("Running"))

			Consistently(func() string {
				if childSup == nil {
					return ""
				}
				return GetWorkerStateName(childSup, "child-0-001")
			}, "10s", "500ms").Should(Equal("Connected"))

			By("Cancelling context for clean shutdown")

			cancel()

			By("Waiting for supervisor to stop gracefully")

			Eventually(func() bool {
				select {
				case <-done:
					return true
				default:
					return false
				}
			}, "2s", "50ms").Should(BeTrue())

			By("Verifying test completes in reasonable time")

			testStart := time.Now()
			Eventually(func() bool {
				return time.Since(testStart) < 20*time.Second
			}, "20s").Should(BeTrue())
		})
	})
})
