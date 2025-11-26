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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	appSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
	childSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
	parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	parentSnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("ApplicationWorker", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		appSup            *supervisor.Supervisor[appSnapshot.ApplicationObservedState, *appSnapshot.ApplicationDesiredState]
		logger            *zap.SugaredLogger
		initialGoroutines int
		store             storage.TriangularStoreInterface
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		basicStore := memory.NewInMemoryStore()
		store = storage.NewTriangularStore(basicStore, logger)

		factory.ResetRegistry()

		err := factory.RegisterFactory[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
			worker, err := parent.NewParentWorker(identity.ID, identity.Name, logger)
			if err != nil {
				panic(err)
			}
			return worker
		})
		Expect(err).ToNot(HaveOccurred())

		err = factory.RegisterFactory[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState](func(identity fsmv2.Identity) fsmv2.Worker {
			mockConnectionPool := NewConnectionPool()

			worker, err := child.NewChildWorker(identity.ID, identity.Name, mockConnectionPool, logger)
			if err != nil {
				panic(err)
			}
			return worker
		})
		Expect(err).ToNot(HaveOccurred())

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
	})

	AfterEach(func() {
		cancel()

		Eventually(func() int {
			runtime.GC()

			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("YAML Configuration", func() {
		It("should create parent-child hierarchy from YAML config", func() {
			By("Creating application supervisor with YAML config")

			yamlConfig := `
children:
  - name: "test-parent"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 2
`

			var err error
			appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "app-001",
				Name:         "Test Application",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   yamlConfig,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(appSup).NotTo(BeNil())

			By("Starting application supervisor")

			done := appSup.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for parent worker to be created and reach Running state")

			var parentSup *supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]
			Eventually(func() interface{} {
				children := appSup.GetChildren()
				for name, child := range children {
					if name == "test-parent" {
						if typedChild, ok := child.(*supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]); ok {
							parentSup = typedChild

							return parentSup
						}
					}
				}

				return nil
			}, "10s", "100ms").ShouldNot(BeNil())

			Eventually(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "test-parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Verifying parent has 2 child supervisors")

			Eventually(func() int {
				if parentSup == nil {
					return 0
				}

				return len(parentSup.GetChildren())
			}, "10s", "100ms").Should(Equal(2))

			By("Verifying children reach Connected state")

			Eventually(func() bool {
				if parentSup == nil {
					return false
				}

				children := parentSup.GetChildren()
				for i := 0; i < 2; i++ {
					childName := "child-0"
					if i == 1 {
						childName = "child-1"
					}

					childSup, ok := children[childName].(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState])
					if !ok || childSup == nil {
						return false
					}

					workerID := childName + "-001"
					state := GetWorkerStateName(childSup, workerID)
					if state != "Connected" {
						return false
					}
				}

				return true
			}, "15s", "200ms").Should(BeTrue())

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
			}, "5s", "50ms").Should(BeTrue())
		})

		It("should support empty children configuration", func() {
			By("Creating application supervisor with no children")

			yamlConfig := `
children: []
`

			var err error
			appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "app-002",
				Name:         "Empty Application",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   yamlConfig,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(appSup).NotTo(BeNil())

			By("Starting application supervisor")

			done := appSup.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Verifying no children are created")

			Consistently(func() int {
				return len(appSup.GetChildren())
			}, "2s", "100ms").Should(Equal(0))

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
		})
	})

	Context("Worker Lifecycle", func() {
		It("should start, connect, disconnect, and stop workers", func() {
			By("Creating application supervisor with parent and child")

			yamlConfig := `
children:
  - name: "lifecycle-parent"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 1
`

			var err error
			appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "app-003",
				Name:         "Lifecycle Test",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   yamlConfig,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Starting application supervisor")

			done := appSup.Start(ctx)

			By("Finding parent supervisor")

			var parentSup *supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]
			Eventually(func() interface{} {
				children := appSup.GetChildren()
				for name, child := range children {
					if name == "lifecycle-parent" {
						if typedChild, ok := child.(*supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]); ok {
							parentSup = typedChild

							return parentSup
						}
					}
				}

				return nil
			}, "10s", "100ms").ShouldNot(BeNil())

			By("Verifying parent reaches Running state")

			Eventually(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "lifecycle-parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Verifying child reaches Connected state")

			var childSup *supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]
			Eventually(func() interface{} {
				if parentSup == nil {
					return nil
				}

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

			Eventually(func() string {
				if childSup == nil {
					return ""
				}

				return GetWorkerStateName(childSup, "child-0-001")
			}, "10s", "100ms").Should(Equal("Connected"))

			By("Verifying parent and child remain stable")

			Consistently(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "lifecycle-parent-001")
			}, "2s", "200ms").Should(Equal("Running"))

			Consistently(func() string {
				if childSup == nil {
					return ""
				}

				return GetWorkerStateName(childSup, "child-0-001")
			}, "2s", "200ms").Should(Equal("Connected"))

			By("Cancelling context to trigger shutdown")

			cancel()

			By("Waiting for supervisor to stop gracefully")

			Eventually(func() bool {
				select {
				case <-done:
					return true
				default:
					return false
				}
			}, "5s", "50ms").Should(BeTrue())
		})
	})

	Context("State Transitions", func() {
		It("should transition workers through expected states", func() {
			By("Creating application supervisor")

			yamlConfig := `
children:
  - name: "transition-parent"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 1
`

			var err error
			appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "app-004",
				Name:         "Transition Test",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   yamlConfig,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Starting application supervisor")

			done := appSup.Start(ctx)

			By("Finding parent supervisor")

			var parentSup *supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]
			Eventually(func() interface{} {
				children := appSup.GetChildren()
				for name, child := range children {
					if name == "transition-parent" {
						if typedChild, ok := child.(*supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]); ok {
							parentSup = typedChild

							return parentSup
						}
					}
				}

				return nil
			}, "10s", "100ms").ShouldNot(BeNil())

			By("Verifying parent reaches Running state")

			Eventually(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "transition-parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Finding child supervisor")

			var childSup *supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState]
			Eventually(func() interface{} {
				if parentSup == nil {
					return nil
				}

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

			By("Verifying child reaches Connected state")

			Eventually(func() string {
				if childSup == nil {
					return ""
				}

				return GetWorkerStateName(childSup, "child-0-001")
			}, "10s", "100ms").Should(Equal("Connected"))

			By("Verifying parent and child remain stable")

			Consistently(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "transition-parent-001")
			}, "2s", "200ms").Should(Equal("Running"))

			Consistently(func() string {
				if childSup == nil {
					return ""
				}

				return GetWorkerStateName(childSup, "child-0-001")
			}, "2s", "200ms").Should(Equal("Connected"))

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
			}, "5s", "50ms").Should(BeTrue())
		})
	})

	Context("Multiple Children", func() {
		It("should manage multiple child workers concurrently", func() {
			By("Creating application supervisor with parent having 3 children")

			yamlConfig := `
children:
  - name: "multi-parent"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 3
`

			var err error
			appSup, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "app-005",
				Name:         "Multi-Child Test",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   yamlConfig,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Starting application supervisor")

			done := appSup.Start(ctx)

			By("Finding parent supervisor")

			var parentSup *supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]
			Eventually(func() interface{} {
				children := appSup.GetChildren()
				for name, child := range children {
					if name == "multi-parent" {
						if typedChild, ok := child.(*supervisor.Supervisor[parentSnapshot.ParentObservedState, *parentSnapshot.ParentDesiredState]); ok {
							parentSup = typedChild

							return parentSup
						}
					}
				}

				return nil
			}, "10s", "100ms").ShouldNot(BeNil())

			By("Waiting for parent to reach Running state")

			Eventually(func() string {
				if parentSup == nil {
					return ""
				}

				return GetWorkerStateName(parentSup, "multi-parent-001")
			}, "10s", "100ms").Should(Equal("Running"))

			By("Verifying all 3 children are created")

			Eventually(func() int {
				if parentSup == nil {
					return 0
				}

				return len(parentSup.GetChildren())
			}, "10s", "100ms").Should(Equal(3))

			By("Verifying all children reach Connected state")

			Eventually(func() bool {
				if parentSup == nil {
					return false
				}

				children := parentSup.GetChildren()
				connectedCount := 0

				for i := 0; i < 3; i++ {
					childName := "child-0"
					if i == 1 {
						childName = "child-1"
					} else if i == 2 {
						childName = "child-2"
					}

					childSup, ok := children[childName].(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState])
					if !ok || childSup == nil {
						continue
					}

					workerID := childName + "-001"
					state := GetWorkerStateName(childSup, workerID)
					if state == "Connected" {
						connectedCount++
					}
				}

				return connectedCount == 3
			}, "20s", "200ms").Should(BeTrue())

			By("Verifying all children remain stable")

			Consistently(func() bool {
				if parentSup == nil {
					return false
				}

				children := parentSup.GetChildren()
				for i := 0; i < 3; i++ {
					childName := "child-0"
					if i == 1 {
						childName = "child-1"
					} else if i == 2 {
						childName = "child-2"
					}

					childSup, ok := children[childName].(*supervisor.Supervisor[childSnapshot.ChildObservedState, *childSnapshot.ChildDesiredState])
					if !ok || childSup == nil {
						return false
					}

					workerID := childName + "-001"
					state := GetWorkerStateName(childSup, workerID)
					if state != "Connected" {
						return false
					}
				}

				return true
			}, "5s", "500ms").Should(BeTrue())

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
			}, "5s", "50ms").Should(BeTrue())
		})
	})
})
