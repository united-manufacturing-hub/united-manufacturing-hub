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

package root_supervisor_test

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/root"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	rootsup "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/root_supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// setupTestStore creates a TriangularStore with collections for the given worker types.
func setupTestStore(ctx context.Context, workerTypes ...string) *storage.TriangularStore {
	basicStore := memory.NewInMemoryStore()

	// Create collections following naming convention: {workerType}_{role}
	for _, workerType := range workerTypes {
		_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)
	}

	return storage.NewTriangularStore(basicStore)
}

var _ = Describe("Root Supervisor Integration with Generic Root Package", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		logger            *zap.SugaredLogger
		initialGoroutines int
		err               error
		rootSupervisor    *supervisor.Supervisor[root.PassthroughObservedState, *root.PassthroughDesiredState]
		mockStore         *storage.TriangularStore
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		// Reset factory registry before each test
		factory.ResetRegistry()

		// Register the generic root package's factories
		err = factory.RegisterFactory[root.PassthroughObservedState, *root.PassthroughDesiredState](
			func(identity fsmv2.Identity) fsmv2.Worker {
				return root.NewPassthroughWorker(identity.ID, identity.Name)
			})
		Expect(err).ToNot(HaveOccurred())

		err = factory.RegisterSupervisorFactory[root.PassthroughObservedState, *root.PassthroughDesiredState](
			func(cfg interface{}) interface{} {
				supervisorCfg := cfg.(supervisor.Config)

				return supervisor.NewSupervisor[root.PassthroughObservedState, *root.PassthroughDesiredState](supervisorCfg)
			})
		Expect(err).ToNot(HaveOccurred())

		// Register the example child worker factories
		err = factory.RegisterFactory[rootsup.ChildObservedState, *rootsup.ChildDesiredState](
			func(identity fsmv2.Identity) fsmv2.Worker {
				return rootsup.NewChildWorker(identity.ID, identity.Name, logger)
			})
		Expect(err).ToNot(HaveOccurred())

		err = factory.RegisterSupervisorFactory[rootsup.ChildObservedState, *rootsup.ChildDesiredState](
			func(cfg interface{}) interface{} {
				supervisorCfg := cfg.(supervisor.Config)

				return supervisor.NewSupervisor[rootsup.ChildObservedState, *rootsup.ChildDesiredState](supervisorCfg)
			})
		Expect(err).ToNot(HaveOccurred())

		// Setup store with collections for both worker types
		rootWorkerType := storage.DeriveWorkerType[root.PassthroughObservedState]()
		childWorkerType := storage.DeriveWorkerType[rootsup.ChildObservedState]()
		mockStore = setupTestStore(ctx, rootWorkerType, childWorkerType)
	})

	AfterEach(func() {
		cancel() // Cancel context FIRST so tick loop exits
		if rootSupervisor != nil {
			rootSupervisor.Shutdown() // Then wait for shutdown
		}

		// Verify no goroutine leaks
		Eventually(func() int {
			runtime.GC()

			return runtime.NumGoroutine()
		}, "5s", "100ms").Should(BeNumerically("<=", initialGoroutines+5))
	})

	Context("Using Generic Root Package with YAML Config", func() {
		It("should create root supervisor with the generic root package", func() {
			By("Step 1: Creating root supervisor using generic root package")

			rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
				ID:           "root-001",
				Name:         "Test Root Supervisor",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rootSupervisor).NotTo(BeNil())

			By("Step 2: Verifying root worker is managed by supervisor")

			workerIDs := rootSupervisor.ListWorkers()
			Expect(workerIDs).To(ContainElement("root-001"))
		})
	})

	Context("Automatic Child Creation from YAML ChildrenSpecs", func() {
		It("should automatically create child workers from YAML config", func() {
			By("Step 1: Creating root supervisor")

			rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
				ID:           "root-002",
				Name:         "Test Root Worker",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Setting UserSpec with YAML children configuration")

			// The key pattern: children are defined in YAML using workerType
			childWorkerType := storage.DeriveWorkerType[rootsup.ChildObservedState]()
			yamlConfig := `
children:
  - name: "child-0"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "child-1"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Step 3: Saving initial observed state with fresh timestamp")

			rootWorkerType := storage.DeriveWorkerType[root.PassthroughObservedState]()
			initialObserved := root.PassthroughObservedState{
				ID:          "root-002",
				CollectedAt: time.Now(),
				Name:        "Test Root Worker",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-002", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			By("Step 4: Starting supervisor to process root worker")

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 5: Verifying children were created automatically")

			// Children should appear in supervisor's managed children via reconcileChildren()
			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))

			By("Step 6: Verifying child names match expected pattern")

			children := rootSupervisor.GetChildren()
			childNames := make([]string, 0, len(children))
			for name := range children {
				childNames = append(childNames, name)
			}
			Expect(childNames).To(ContainElements("child-0", "child-1"))
		})
	})

	Context("Dynamic Child Management via YAML Config Changes", func() {
		It("should add children when YAML config is updated", func() {
			By("Step 1: Creating supervisor with root worker and 1 child")

			rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
				ID:           "root-003",
				Name:         "Test Root Worker",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).NotTo(HaveOccurred())

			childWorkerType := storage.DeriveWorkerType[rootsup.ChildObservedState]()
			yamlConfig := `
children:
  - name: "child-0"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			rootWorkerType := storage.DeriveWorkerType[root.PassthroughObservedState]()
			initialObserved := root.PassthroughObservedState{
				ID:          "root-003",
				CollectedAt: time.Now(),
				Name:        "Test Root Worker",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-003", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 2: Waiting for first child to be created")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(1))

			By("Step 3: Updating YAML config to add second child")

			yamlConfig = `
children:
  - name: "child-0"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "child-1"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Step 4: Verifying second child is created")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))
		})
	})

	Context("Removing Children via YAML Config", func() {
		It("should remove all children when YAML config is emptied", func() {
			By("Step 1: Creating supervisor with root worker and children")

			rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
				ID:           "root-004",
				Name:         "Test Root Worker",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).NotTo(HaveOccurred())

			childWorkerType := storage.DeriveWorkerType[rootsup.ChildObservedState]()
			yamlConfig := `
children:
  - name: "child-0"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "child-1"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			rootWorkerType := storage.DeriveWorkerType[root.PassthroughObservedState]()
			initialObserved := root.PassthroughObservedState{
				ID:          "root-004",
				CollectedAt: time.Now(),
				Name:        "Test Root Worker",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-004", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Step 2: Waiting for children to be created")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))

			By("Step 3: Updating YAML config to remove all children")

			yamlConfig = `
children: []
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Step 4: Verifying all children are removed")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(0))
		})
	})

	Context("Root Worker without Children", func() {
		It("should handle root worker with empty children in YAML", func() {
			By("Step 1: Creating root supervisor with empty children config")

			rootSupervisor, err = root.NewRootSupervisor(root.SupervisorConfig{
				ID:           "root-005",
				Name:         "Test Root Worker No Children",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Setting UserSpec with empty children array")

			yamlConfig := `
children: []
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Step 3: Verifying DeriveDesiredState returns nil ChildrenSpecs")

			rootWorker := root.NewPassthroughWorker("test-empty", "Test Empty Children")
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}
			desiredState, err := rootWorker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredState.ChildrenSpecs).To(BeEmpty())
		})
	})

	Context("PassthroughWorker DeriveDesiredState Validation", func() {
		It("should generate correct ChildrenSpecs from YAML config", func() {
			By("Step 1: Creating passthrough worker")

			worker := root.NewPassthroughWorker("test-derive", "Test Derive Worker")
			Expect(worker).NotTo(BeNil())

			By("Step 2: Testing DeriveDesiredState with 3 children in YAML")

			childWorkerType := storage.DeriveWorkerType[rootsup.ChildObservedState]()
			yamlConfig := `
children:
  - name: "child-0"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "child-1"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "child-2"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			userSpec := config.UserSpec{
				Config: yamlConfig,
			}

			desiredState, err := worker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredState.State).To(Equal("running"))
			Expect(desiredState.ChildrenSpecs).To(HaveLen(3))

			By("Step 3: Verifying each child spec has correct structure")

			for i, childSpec := range desiredState.ChildrenSpecs {
				Expect(childSpec.Name).To(Equal("child-" + string(rune('0'+i))))
				Expect(childSpec.WorkerType).To(Equal(childWorkerType))
			}
		})

		It("should return error for invalid spec type", func() {
			By("Step 1: Creating passthrough worker")

			worker := root.NewPassthroughWorker("test-invalid", "Test Invalid Worker")

			By("Step 2: Testing DeriveDesiredState with invalid spec type")

			invalidSpec := "not a UserSpec"
			_, err := worker.DeriveDesiredState(invalidSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})

		It("should handle nil spec gracefully", func() {
			By("Step 1: Creating passthrough worker")

			worker := root.NewPassthroughWorker("test-nil", "Test Nil Spec Worker")

			By("Step 2: Testing DeriveDesiredState with nil spec")

			desiredState, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredState.State).To(Equal("running"))
			Expect(desiredState.ChildrenSpecs).To(BeNil())
		})
	})

	Context("Child Worker as Leaf Node", func() {
		It("should return nil ChildrenSpecs for child worker", func() {
			By("Step 1: Creating child worker")

			childWorker := rootsup.NewChildWorker("child-test", "Test Child Worker", logger)
			Expect(childWorker).NotTo(BeNil())

			By("Step 2: Testing DeriveDesiredState returns nil ChildrenSpecs")

			userSpec := config.UserSpec{}
			desiredState, err := childWorker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredState.State).To(Equal("connected"))
			Expect(desiredState.ChildrenSpecs).To(BeNil(), "Child workers are leaf nodes - no children")
		})

		It("should handle nil spec for child worker", func() {
			By("Step 1: Creating child worker")

			childWorker := rootsup.NewChildWorker("child-nil", "Test Child Nil Spec", logger)

			By("Step 2: Testing DeriveDesiredState with nil spec")

			desiredState, err := childWorker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredState.State).To(Equal("connected"))
			Expect(desiredState.ChildrenSpecs).To(BeNil())
		})
	})

	Context("CollectObservedState", func() {
		It("should collect observed state for passthrough worker", func() {
			By("Step 1: Creating passthrough worker")

			worker := root.NewPassthroughWorker("root-collect", "Test Collect Worker")

			By("Step 2: Collecting observed state")

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			By("Step 3: Verifying observed state structure")

			rootObserved, ok := observed.(*root.PassthroughObservedState)
			Expect(ok).To(BeTrue())
			Expect(rootObserved.ID).To(Equal("root-collect"))
			Expect(rootObserved.CollectedAt).NotTo(BeZero())
		})

		It("should collect observed state for child worker", func() {
			By("Step 1: Creating child worker")

			childWorker := rootsup.NewChildWorker("child-collect", "Test Child Collect", logger)

			By("Step 2: Collecting observed state")

			observed, err := childWorker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			By("Step 3: Verifying observed state structure")

			childObserved, ok := observed.(rootsup.ChildObservedState)
			Expect(ok).To(BeTrue())
			Expect(childObserved.ID).To(Equal("child-collect"))
			Expect(childObserved.CollectedAt).NotTo(BeZero())
			Expect(childObserved.ConnectionStatus).To(Equal("connected"))
			Expect(childObserved.ConnectionHealth).To(Equal("healthy"))
		})
	})
})
