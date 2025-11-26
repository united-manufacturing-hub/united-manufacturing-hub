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

package application_test

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// TestChildObservedState is a minimal observed state for testing child workers.
type TestChildObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`
}

func (o TestChildObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o TestChildObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &TestChildDesiredState{}
}

// TestChildDesiredState is a minimal desired state for testing child workers.
type TestChildDesiredState struct{}

func (d *TestChildDesiredState) IsShutdownRequested() bool {
	return false
}

// TestChildWorker is a minimal worker implementation for testing.
type TestChildWorker struct {
	id   string
	name string
}

func NewTestChildWorker(id, name string) *TestChildWorker {
	return &TestChildWorker{id: id, name: name}
}

func (w *TestChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return TestChildObservedState{
		ID:          w.id,
		CollectedAt: time.Now(),
	}, nil
}

func (w *TestChildWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: nil,
	}, nil
}

func (w *TestChildWorker) GetInitialState() fsmv2.State[any, any] {
	return nil
}

// setupTestStore creates a TriangularStore with collections for the given worker types.
func setupTestStore(ctx context.Context, workerTypes ...string) *storage.TriangularStore {
	basicStore := memory.NewInMemoryStore()

	// Create collections following naming convention: {workerType}_{role}
	for _, workerType := range workerTypes {
		_ = basicStore.CreateCollection(ctx, workerType+"_identity", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
		_ = basicStore.CreateCollection(ctx, workerType+"_observed", nil)
	}

	return storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())
}

var _ = Describe("Root Package Integration", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		logger            *zap.SugaredLogger
		initialGoroutines int
		err               error
		rootSupervisor    *supervisor.Supervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState]
		mockStore         *storage.TriangularStore
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		initialGoroutines = runtime.NumGoroutine()

		// Reset factory registry before each test
		factory.ResetRegistry()

		// Register passthrough root worker factory
		err = factory.RegisterFactory[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
			func(identity fsmv2.Identity) fsmv2.Worker {
				return application.NewApplicationWorker(identity.ID, identity.Name)
			})
		Expect(err).ToNot(HaveOccurred())

		// Register test child worker factory
		err = factory.RegisterFactory[TestChildObservedState, *TestChildDesiredState](
			func(identity fsmv2.Identity) fsmv2.Worker {
				return NewTestChildWorker(identity.ID, identity.Name)
			})
		Expect(err).ToNot(HaveOccurred())

		// Register supervisor factories
		err = factory.RegisterSupervisorFactory[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
			func(cfg interface{}) interface{} {
				supervisorCfg := cfg.(supervisor.Config)

				return supervisor.NewSupervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](supervisorCfg)
			})
		Expect(err).ToNot(HaveOccurred())

		err = factory.RegisterSupervisorFactory[TestChildObservedState, *TestChildDesiredState](
			func(cfg interface{}) interface{} {
				supervisorCfg := cfg.(supervisor.Config)

				return supervisor.NewSupervisor[TestChildObservedState, *TestChildDesiredState](supervisorCfg)
			})
		Expect(err).ToNot(HaveOccurred())

		// Setup store with collections for both worker types
		rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
		childWorkerType := storage.DeriveWorkerType[TestChildObservedState]()
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

	Context("NewApplicationSupervisor Setup Helper", func() {
		It("should create supervisor with root worker using NewApplicationSupervisor", func() {
			By("Creating root supervisor using setup helper")

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-001",
				Name:         "Test Root Supervisor",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
				YAMLConfig:   "",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(rootSupervisor).NotTo(BeNil())

			By("Verifying root worker was added to supervisor")

			workerIDs := rootSupervisor.ListWorkers()
			Expect(workerIDs).To(ContainElement("root-001"))
		})

		It("should use default tick interval when not specified", func() {
			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:     "root-002",
				Name:   "Test Default Interval",
				Store:  mockStore,
				Logger: logger,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(rootSupervisor).NotTo(BeNil())
		})
	})

	Context("YAML Config Parsing with Children", func() {
		It("should parse children from YAML config and create ChildrenSpecs", func() {
			By("Creating root supervisor")

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-003",
				Name:         "Test YAML Parsing",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Setting UserSpec with children YAML config")

			// Get the child worker type for YAML config
			childWorkerType := storage.DeriveWorkerType[TestChildObservedState]()

			yamlConfig := `
children:
  - name: "test-child-1"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
  - name: "test-child-2"
    workerType: "` + childWorkerType + `"
    userSpec:
      config: ""
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Saving initial observed state")

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-003",
				CollectedAt: time.Now(),
				Name:        "Test YAML Parsing",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-003", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			By("Starting supervisor to process children")

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Verifying children were created automatically")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))

			By("Verifying child names match expected pattern")

			children := rootSupervisor.GetChildren()
			childNames := make([]string, 0, len(children))
			for name := range children {
				childNames = append(childNames, name)
			}
			Expect(childNames).To(ContainElements("test-child-1", "test-child-2"))
		})
	})

	Context("Dynamic Child Management", func() {
		It("should add new children when YAML config changes", func() {
			By("Creating supervisor with 1 child")

			childWorkerType := storage.DeriveWorkerType[TestChildObservedState]()

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-004",
				Name:         "Test Dynamic Children",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			yamlConfig := `
children:
  - name: "child-1"
    workerType: "` + childWorkerType + `"
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-004",
				CollectedAt: time.Now(),
				Name:        "Test Dynamic Children",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-004", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for initial child")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(1))

			By("Updating config to add second child")

			yamlConfig = `
children:
  - name: "child-1"
    workerType: "` + childWorkerType + `"
  - name: "child-2"
    workerType: "` + childWorkerType + `"
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			By("Verifying second child was created")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))
		})

		It("should remove children when YAML config changes", func() {
			By("Creating supervisor with 2 children")

			childWorkerType := storage.DeriveWorkerType[TestChildObservedState]()

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-005",
				Name:         "Test Remove Children",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			yamlConfig := `
children:
  - name: "child-1"
    workerType: "` + childWorkerType + `"
  - name: "child-2"
    workerType: "` + childWorkerType + `"
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-005",
				CollectedAt: time.Now(),
				Name:        "Test Remove Children",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-005", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for both children")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(2))

			By("Updating config to remove all children")

			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: "children: []",
			})

			By("Verifying all children were removed")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(0))
		})
	})

	Context("Empty Config Handling", func() {
		It("should handle empty children array", func() {
			By("Creating supervisor with empty children")

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-006",
				Name:         "Test Empty Children",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: "children: []",
			})

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-006",
				CollectedAt: time.Now(),
				Name:        "Test Empty Children",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-006", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			// Give supervisor time to process
			time.Sleep(500 * time.Millisecond)

			By("Verifying no children were created")

			children := rootSupervisor.GetChildren()
			Expect(children).To(BeEmpty())
		})

		It("should handle empty config string", func() {
			By("Creating supervisor with empty config")

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-007",
				Name:         "Test Empty Config",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: "",
			})

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-007",
				CollectedAt: time.Now(),
				Name:        "Test Empty Config",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-007", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			// Give supervisor time to process
			time.Sleep(500 * time.Millisecond)

			By("Verifying no children were created")

			children := rootSupervisor.GetChildren()
			Expect(children).To(BeEmpty())
		})
	})

	Context("Error Cases", func() {
		It("should handle invalid YAML config gracefully", func() {
			By("Creating supervisor with invalid YAML")

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-008",
				Name:         "Test Invalid YAML",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			// Test that DeriveDesiredState returns error for invalid YAML
			worker := application.NewApplicationWorker("test", "test")
			userSpec := config.UserSpec{
				Config: "invalid: yaml: content: [",
			}

			_, err := worker.DeriveDesiredState(userSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse children config"))
		})
	})

	Context("Supervisor Lifecycle", func() {
		It("should start and stop supervisor cleanly", func() {
			By("Creating and starting supervisor")

			childWorkerType := storage.DeriveWorkerType[TestChildObservedState]()

			rootSupervisor, err = application.NewApplicationSupervisor(application.SupervisorConfig{
				ID:           "root-009",
				Name:         "Test Lifecycle",
				Store:        mockStore,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})
			Expect(err).ToNot(HaveOccurred())

			yamlConfig := `
children:
  - name: "child-1"
    workerType: "` + childWorkerType + `"
`
			rootSupervisor.TestUpdateUserSpec(config.UserSpec{
				Config: yamlConfig,
			})

			rootWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
			initialObserved := snapshot.ApplicationObservedState{
				ID:          "root-009",
				CollectedAt: time.Now(),
				Name:        "Test Lifecycle",
			}
			_, err = mockStore.SaveObserved(ctx, rootWorkerType, "root-009", initialObserved)
			Expect(err).NotTo(HaveOccurred())

			done := rootSupervisor.Start(ctx)
			Expect(done).NotTo(BeNil())

			By("Waiting for child creation")

			Eventually(func() int {
				children := rootSupervisor.GetChildren()

				return len(children)
			}, "5s", "100ms").Should(Equal(1))

			By("Stopping supervisor via context cancellation")

			cancel()
			rootSupervisor.Shutdown()

			// Verify shutdown completed
			Eventually(done).Should(BeClosed())
		})
	})
})
