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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

var _ = Describe("Phase 0: Happy Path Integration", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		parentSupervisor  *supervisor.Supervisor
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

		factory.ResetRegistry()
	})

	AfterEach(func() {
		cancel()

		Eventually(func() int {
			runtime.GC()
			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Basic Factory Registration", func() {
		It("should register parent worker type and create worker instances", func() {
			By("Step 1: Registering parent worker type in factory")

			err := factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger)
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Creating parent worker via factory")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentWorker).NotTo(BeNil())

			By("Step 3: Verifying worker has expected initial state")

			initialState := parentWorker.GetInitialState()
			Expect(initialState).NotTo(BeNil())
			Expect(initialState.String()).To(Equal("Stopped"))

			By("Step 4: Verifying worker can collect observed state")

			observedState, err := parentWorker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observedState).NotTo(BeNil())
		})

		It("should register child worker type and create worker instances", func() {
			By("Step 1: Registering child worker type in factory")

			err := factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) fsmv2.Worker {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Creating child worker via factory")

			childIdentity := fsmv2.Identity{
				ID:         "child-001",
				Name:       "Example Child",
				WorkerType: child.WorkerType,
			}

			childWorker, err := factory.NewWorker(child.WorkerType, childIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(childWorker).NotTo(BeNil())

			By("Step 3: Verifying worker has expected initial state")

			initialState := childWorker.GetInitialState()
			Expect(initialState).NotTo(BeNil())
			Expect(initialState.String()).To(Equal("Stopped"))

			By("Step 4: Verifying worker can collect observed state")

			observedState, err := childWorker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observedState).NotTo(BeNil())
		})

		It("should create supervisor without errors", func() {
			By("Step 1: Creating supervisor instance")

			parentSupervisor = supervisor.NewSupervisor(supervisor.Config{
				WorkerType: parent.WorkerType,
				Store:      store,
				Logger:     logger,
			})

			Expect(parentSupervisor).NotTo(BeNil())

			By("Step 2: Verifying supervisor can be created successfully")

			children := parentSupervisor.GetChildren()
			Expect(children).NotTo(BeNil())
			Expect(children).To(BeEmpty())
		})
	})
})

