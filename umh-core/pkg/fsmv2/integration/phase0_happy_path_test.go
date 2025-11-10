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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
)

var _ = Describe("Phase 0: Happy Path Integration", func() {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		parentSupervisor *supervisor.Supervisor
		logger           *zap.SugaredLogger
		initialGoroutines int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()
	})

	AfterEach(func() {
		if parentSupervisor != nil {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			parentSupervisor.Stop(stopCtx)
		}
		cancel()

		Eventually(func() int {
			runtime.GC()
			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Basic Parent-Child Integration", func() {
		It("should successfully start parent and child workers", func() {
			By("Step 1: Registering worker types in factory")

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger), nil
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger), nil
			})

			By("Step 2: Creating parent supervisor")

			parentIdentity := fsmv2.Identity{
				ID:         "parent-001",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentWorker).NotTo(BeNil())

			By("Step 3: Starting parent supervisor")

			parentSupervisor = supervisor.NewSupervisor(
				parentWorker,
				nil,
				logger,
			)

			err = parentSupervisor.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Step 4: Verifying parent transitions to Running")

			Eventually(func() string {
				snapshot, err := parentSupervisor.GetSnapshot()
				if err != nil {
					return ""
				}
				return snapshot.State.String()
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 5: Verifying child is created and started")

			Eventually(func() int {
				children, err := parentSupervisor.GetChildren()
				if err != nil {
					return 0
				}
				return len(children)
			}, "5s", "100ms").Should(Equal(1))

			By("Step 6: Verifying child transitions to Connected")

			Eventually(func() string {
				children, err := parentSupervisor.GetChildren()
				if err != nil || len(children) == 0 {
					return ""
				}
				childSnapshot, err := children[0].GetSnapshot()
				if err != nil {
					return ""
				}
				return childSnapshot.State.String()
			}, "5s", "100ms").Should(Equal("Connected"))

			By("Step 7: Both processing ticks normally")

			time.Sleep(500 * time.Millisecond)

			parentSnapshot, err := parentSupervisor.GetSnapshot()
			Expect(err).NotTo(HaveOccurred())
			Expect(parentSnapshot.State.String()).To(Equal("Running"))

			children, err := parentSupervisor.GetChildren()
			Expect(err).NotTo(HaveOccurred())
			Expect(children).To(HaveLen(1))

			childSnapshot, err := children[0].GetSnapshot()
			Expect(err).NotTo(HaveOccurred())
			Expect(childSnapshot.State.String()).To(Equal("Connected"))
		})

		It("should gracefully shutdown parent and remove child", func() {
			By("Step 1: Setting up parent and child")

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger), nil
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger), nil
			})

			parentIdentity := fsmv2.Identity{
				ID:         "parent-002",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())

			parentSupervisor = supervisor.NewSupervisor(
				parentWorker,
				nil,
				logger,
			)

			err = parentSupervisor.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				snapshot, err := parentSupervisor.GetSnapshot()
				if err != nil {
					return ""
				}
				return snapshot.State.String()
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 2: Requesting parent shutdown")

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			err = parentSupervisor.Stop(shutdownCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Step 3: Verifying parent transitions to Stopped")

			Eventually(func() string {
				snapshot, err := parentSupervisor.GetSnapshot()
				if err != nil {
					return ""
				}
				return snapshot.State.String()
			}, "5s", "100ms").Should(Equal("Stopped"))

			By("Step 4: Verifying child is removed")

			Eventually(func() int {
				children, err := parentSupervisor.GetChildren()
				if err != nil {
					return -1
				}
				return len(children)
			}, "5s", "100ms").Should(Equal(0))
		})

		It("should not leak goroutines", func() {
			By("Step 1: Recording initial goroutine count")

			beforeGoroutines := runtime.NumGoroutine()

			By("Step 2: Creating and starting parent worker")

			factory.RegisterWorkerType(parent.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConfigLoader := &MockConfigLoader{}
				return parent.NewParentWorker(identity.ID, identity.Name, mockConfigLoader, logger), nil
			})

			factory.RegisterWorkerType(child.WorkerType, func(identity fsmv2.Identity) (fsmv2.Worker, error) {
				mockConnectionPool := &MockConnectionPool{}
				return child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger), nil
			})

			parentIdentity := fsmv2.Identity{
				ID:         "parent-003",
				Name:       "Example Parent",
				WorkerType: parent.WorkerType,
			}

			parentWorker, err := factory.NewWorker(parent.WorkerType, parentIdentity)
			Expect(err).NotTo(HaveOccurred())

			parentSupervisor = supervisor.NewSupervisor(
				parentWorker,
				nil,
				logger,
			)

			err = parentSupervisor.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Step 3: Waiting for parent and child to be Running/Connected")

			Eventually(func() string {
				snapshot, err := parentSupervisor.GetSnapshot()
				if err != nil {
					return ""
				}
				return snapshot.State.String()
			}, "5s", "100ms").Should(Equal("Running"))

			By("Step 4: Stopping supervisor")

			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()

			err = parentSupervisor.Stop(stopCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Step 5: Verifying no goroutine leaks")

			Eventually(func() int {
				runtime.GC()
				return runtime.NumGoroutine()
			}, "5s", "100ms").Should(BeNumerically("<=", beforeGoroutines+5))
		})
	})
})

type MockConfigLoader struct{}

func (m *MockConfigLoader) LoadConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"example_config_key": "example_value",
	}, nil
}

type MockConnectionPool struct{}

func (m *MockConnectionPool) GetConnection(name string) (interface{}, error) {
	return &MockConnection{}, nil
}

type MockConnection struct{}

func (m *MockConnection) IsHealthy() bool {
	return true
}
