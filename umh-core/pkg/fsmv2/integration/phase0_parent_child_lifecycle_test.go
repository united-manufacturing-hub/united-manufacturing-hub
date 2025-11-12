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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	parentSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

var _ = Describe("Phase 0: Parent-Child Lifecycle", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		parentSup         *supervisor.Supervisor
		logger            *zap.SugaredLogger
		initialGoroutines int
		store             storage.TriangularStoreInterface
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		basicStore := memory.NewInMemoryStore()
		registry := storage.NewRegistry()
		store = storage.NewTriangularStore(basicStore, registry)

		ctx := context.Background()

		err := basicStore.CreateCollection(ctx, parent.WorkerType+"_identity", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, parent.WorkerType+"_desired", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, parent.WorkerType+"_observed", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, child.WorkerType+"_identity", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, child.WorkerType+"_desired", nil)
		if err != nil {
			panic(err)
		}
		err = basicStore.CreateCollection(ctx, child.WorkerType+"_observed", nil)
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       parent.WorkerType + "_identity",
			WorkerType: parent.WorkerType,
			Role:       storage.RoleIdentity,
		})
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       parent.WorkerType + "_desired",
			WorkerType: parent.WorkerType,
			Role:       storage.RoleDesired,
		})
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       parent.WorkerType + "_observed",
			WorkerType: parent.WorkerType,
			Role:       storage.RoleObserved,
		})
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       child.WorkerType + "_identity",
			WorkerType: child.WorkerType,
			Role:       storage.RoleIdentity,
		})
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       child.WorkerType + "_desired",
			WorkerType: child.WorkerType,
			Role:       storage.RoleDesired,
		})
		if err != nil {
			panic(err)
		}

		err = registry.Register(&storage.CollectionMetadata{
			Name:       child.WorkerType + "_observed",
			WorkerType: child.WorkerType,
			Role:       storage.RoleObserved,
		})
		if err != nil {
			panic(err)
		}

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

			err := factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConfigLoader := NewParentConfig()
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger)
			})
			Expect(err).NotTo(HaveOccurred())

			err = factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := NewConnectionPool()
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating parent supervisor")

			parentSup = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: parent.WorkerType,
				Store:      store,
				Logger:     logger,
			})
			Expect(parentSup).NotTo(BeNil())

			By("Creating and adding parent worker instance to supervisor")

			mockConfigLoader := NewParentConfig().WithChildren(1)
			parentWorker := parent.NewParentWorker("parent-001", "Test Parent", mockConfigLoader, logger)

			identity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Test Parent",
				WorkerType: parent.WorkerType,
			}
			err = parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			By("Saving initial desired state to database")

			desiredDoc := persistence.Document{
				"id":    "parent-001",
				"state": "running",
			}
			err = store.SaveDesired(ctx, parent.WorkerType, "parent-001", desiredDoc)
			Expect(err).NotTo(HaveOccurred())

			By("Saving initial observed state to database (to avoid Document vs typed struct panic)")

			initialObserved := parentSnapshot.ParentObservedState{
				ID:          "parent-001",
				CollectedAt: time.Now(),
			}
			_, err = store.SaveObserved(ctx, parent.WorkerType, "parent-001", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			By("Starting supervisor properly (collectors + tick loops)")

			done := parentSup.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for parent to reach Running state")

			Eventually(func() string {
				return GetWorkerStateName(parentSup, "parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Waiting for child supervisor to be created and child to reach Connected state")

			var childSup *supervisor.Supervisor
			Eventually(func() *supervisor.Supervisor {
				childSup = GetChildSupervisor(parentSup, "child-0")
				return childSup
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
