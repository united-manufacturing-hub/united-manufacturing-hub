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

package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Hierarchy Metrics (Task 3)", func() {
	var (
		ctx       context.Context
		store     *mockTriangularStore
		logger    *zap.SugaredLogger
		parentSup *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockTriangularStore()
		logger = zap.NewNop().Sugar()
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
	})

	Describe("Hierarchy Depth Metrics", func() {
		It("should record depth=0 for root supervisor", func() {
			parentWorker := &mockHierarchicalWorker{
				id:            "root",
				logger:        newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{},
				stateName:     "running",
				observed: &mockObservedState{
					ID:          "root-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "root",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "root-worker",
				Name:       "Root Worker",
				WorkerType: "root",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			store.mu.Lock()
			store.desired["root"] = map[string]persistence.Document{
				"root-worker": {
					"id":                "root-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["root"] = map[string]interface{}{
				"root-worker": persistence.Document{
					"id":          "root-worker",
					"collectedAt": time.Now(),
				},
			}
			store.mu.Unlock()

			parentSup.Start(ctx)
			time.Sleep(100 * time.Millisecond)

			metricValue := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("root"))
			Expect(metricValue).To(Equal(0.0), "root supervisor should have depth=0")
		})

		It("should record depth=1 for child supervisor", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child-config"},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			store.mu.Lock()
			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			store.mu.Unlock()
			parentSup.Start(ctx)
			time.Sleep(1500 * time.Millisecond)

			parentDepth := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("parent"))
			Expect(parentDepth).To(Equal(0.0), "parent supervisor should have depth=0")

			children := parentSup.GetChildren()
			Expect(children).To(HaveKey("child"))

			childDepth := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("child"))
			Expect(childDepth).To(Equal(1.0), "child supervisor should have depth=1")
		})

		It("should record depth=2 for grandchild supervisor (3-level hierarchy)", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child",
						WorkerType: "child",
						UserSpec:   config.UserSpec{Config: "child-config"},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			store.mu.Lock()
			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.mu.Unlock()
			parentSup.Start(ctx)
			time.Sleep(1500 * time.Millisecond)

			children := parentSup.GetChildren()
			Expect(children).To(HaveKey("child"))
			childSupInterface := children["child"]
			childSup, ok := childSupInterface.(*supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState])
			Expect(ok).To(BeTrue(), "child supervisor should use TestObservedState/TestDesiredState types")

			childIdentity := fsmv2.Identity{
				ID:         "child-worker",
				Name:       "Child Worker",
				WorkerType: "child",
			}
			childWorker := &mockHierarchicalWorker{
				id:     "child-worker",
				logger: newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "grandchild",
						WorkerType: "grandchild",
						UserSpec:   config.UserSpec{Config: "grandchild-config"},
					},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "child-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			err = childSup.AddWorker(childIdentity, childWorker)
			Expect(err).NotTo(HaveOccurred())

			childSup.TestUpdateUserSpec(config.UserSpec{Config: "child-config"})
store.mu.Lock()

			store.desired["child"] = map[string]persistence.Document{
				"child-worker": {
					"id":                "child-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child-worker": persistence.Document{
					"id":          "child-worker",
					"collectedAt": time.Now(),
				},
			}

			store.desired["grandchild"] = map[string]persistence.Document{
				"grandchild-001": {
					"id":                "grandchild-001",
					"shutdownRequested": false,
				},
			}

			store.Observed["grandchild"] = map[string]interface{}{
				"grandchild-001": persistence.Document{
					"id":          "grandchild-001",
					"collectedAt": time.Now(),
				},
			}

store.mu.Unlock()
			// Wait for first tick (1s) + metrics reporter interval (10s) + buffer
		time.Sleep(12 * time.Second)

			parentDepth := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("parent"))
			Expect(parentDepth).To(Equal(0.0), "parent supervisor should have depth=0")

			childDepth := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("child"))
			Expect(childDepth).To(Equal(1.0), "child supervisor should have depth=1")

			grandchildren := childSup.GetChildren()
			Expect(grandchildren).To(HaveKey("grandchild"))

			grandchildDepth := promtest.ToFloat64(metrics.GetHierarchyDepthGauge().WithLabelValues("grandchild"))
			Expect(grandchildDepth).To(Equal(2.0), "grandchild supervisor should have depth=2")
		})
	})

	Describe("Hierarchy Size Metrics", func() {
		It("should record size=1 for leaf supervisor with no children", func() {
			leafWorker := &mockHierarchicalWorker{
				id:            "leaf",
				logger:        newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{},
				stateName:     "running",
				observed: &mockObservedState{
					ID:          "leaf-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			leafSup := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "leaf",
				Logger:     logger,
				Store:      store,
			})
			defer leafSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "leaf-worker",
				Name:       "Leaf Worker",
				WorkerType: "leaf",
			}
			err := leafSup.AddWorker(identity, leafWorker)
			Expect(err).NotTo(HaveOccurred())

			leafSup.TestUpdateUserSpec(config.UserSpec{Config: "leaf-config"})

			store.mu.Lock()
			store.desired["leaf"] = map[string]persistence.Document{
				"leaf-worker": {
					"id":                "leaf-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["leaf"] = map[string]interface{}{
				"leaf-worker": persistence.Document{
					"id":          "leaf-worker",
					"collectedAt": time.Now(),
				},
			}

			store.mu.Unlock()
			leafSup.Start(ctx)
			time.Sleep(100 * time.Millisecond)

			metricValue := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("leaf"))
			Expect(metricValue).To(Equal(1.0), "leaf supervisor should have size=1 (self only)")
		})

		It("should record size=3 for parent with 2 children", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: config.UserSpec{}},
					{Name: "child2", WorkerType: "child", UserSpec: config.UserSpec{}},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			store.mu.Lock()
			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.desired["child"] = map[string]persistence.Document{
				"child1-001": {
					"id":                "child1-001",
					"shutdownRequested": false,
				},
				"child2-001": {
					"id":                "child2-001",
					"shutdownRequested": false,
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child1-001": persistence.Document{
					"id":          "child1-001",
					"collectedAt": time.Now(),
				},
				"child2-001": persistence.Document{
					"id":          "child2-001",
					"collectedAt": time.Now(),
				},
			}

			store.mu.Unlock()
			parentSup.Start(ctx)
			// Wait for first tick (1s) + metrics reporter interval (10s) + buffer
			time.Sleep(12 * time.Second)

			metricValue := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("parent"))
			Expect(metricValue).To(Equal(3.0), "parent supervisor should have size=3 (self + 2 children)")
		})

		It("should update size metric when children are added", func() {
			parentWorker := &mockHierarchicalWorker{
				id:            "parent",
				logger:        newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{},
				stateName:     "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			store.mu.Lock()
			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}
			store.mu.Unlock()

			parentSup.Start(ctx)
			time.Sleep(1500 * time.Millisecond)

			initialSize := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("parent"))
			Expect(initialSize).To(Equal(1.0), "parent should start with size=1")

			parentWorker.mu.Lock()
			parentWorker.childrenSpecs = []config.ChildSpec{
				{Name: "child1", WorkerType: "child", UserSpec: config.UserSpec{}},
			}
			parentWorker.mu.Unlock()

			store.mu.Lock()
			store.desired["child"] = map[string]persistence.Document{
				"child1-001": {
					"id":                "child1-001",
					"shutdownRequested": false,
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child1-001": persistence.Document{
					"id":          "child1-001",
					"collectedAt": time.Now(),
				},
			}
			store.mu.Unlock()

			// Wait for first tick (1s) + metrics reporter interval (10s) + buffer
			time.Sleep(12 * time.Second)

			updatedSize := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("parent"))
			Expect(updatedSize).To(Equal(2.0), "parent should now have size=2 after adding child")
		})

		It("should update size metric when children are removed", func() {
			parentWorker := &mockHierarchicalWorker{
				id:     "parent",
				logger: newHierarchicalTickLogger(),
				childrenSpecs: []config.ChildSpec{
					{Name: "child1", WorkerType: "child", UserSpec: config.UserSpec{}},
				},
				stateName: "running",
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
			}

			parentSup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     logger,
				Store:      store,
			})
			defer parentSup.Shutdown()

			identity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			err := parentSup.AddWorker(identity, parentWorker)
			Expect(err).NotTo(HaveOccurred())

			parentSup.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			store.mu.Lock()
			store.desired["parent"] = map[string]persistence.Document{
				"parent-worker": {
					"id":                "parent-worker",
					"shutdownRequested": false,
				},
			}

			store.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}

			store.desired["child"] = map[string]persistence.Document{
				"child1-001": {
					"id":                "child1-001",
					"shutdownRequested": false,
				},
			}

			store.Observed["child"] = map[string]interface{}{
				"child1-001": persistence.Document{
					"id":          "child1-001",
					"collectedAt": time.Now(),
				},
			}

			store.mu.Unlock()
			parentSup.Start(ctx)
			// Wait for first tick (1s) + metrics reporter interval (10s) + buffer
			time.Sleep(12 * time.Second)

			initialSize := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("parent"))
			Expect(initialSize).To(Equal(2.0), "parent should start with size=2")

			parentWorker.mu.Lock()
			parentWorker.childrenSpecs = []config.ChildSpec{}
			parentWorker.mu.Unlock()

			// Wait for first tick (1s) + metrics reporter interval (10s) + buffer
			time.Sleep(12 * time.Second)

			updatedSize := promtest.ToFloat64(metrics.GetHierarchySizeGauge().WithLabelValues("parent"))
			Expect(updatedSize).To(Equal(1.0), "parent should now have size=1 after removing child")
		})
	})
})
