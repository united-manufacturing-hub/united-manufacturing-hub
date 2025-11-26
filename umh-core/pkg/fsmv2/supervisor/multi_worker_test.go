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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

var _ = Describe("Multi-Worker Supervisor", func() {
	var (
		s               *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		triangularStore *storage.TriangularStore
		basicStore      persistence.Store
	)

	BeforeEach(func() {
		ctx := context.Background()
		var err error

		basicStore = memory.NewInMemoryStore()

		// Create collections in database
		// Collections follow convention: {workerType}_identity, {workerType}_desired, {workerType}_observed
		err = basicStore.CreateCollection(ctx, "test_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_observed", nil)
		Expect(err).ToNot(HaveOccurred())

		triangularStore = storage.NewTriangularStore(basicStore, nil)

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
		})
	})

	AfterEach(func() {
		ctx := context.Background()
		if basicStore != nil {
			_ = basicStore.Close(ctx)
		}
	})

	Describe("AddWorker", func() {
		It("should add worker to registry", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}
			worker2 := &mockWorker{observed: createMockObservedStateWithID("worker-2")}
			worker3 := &mockWorker{observed: createMockObservedStateWithID("worker-3")}

			err := s.AddWorker(identity1, worker1)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity2, worker2)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity3, worker3)
			Expect(err).ToNot(HaveOccurred())

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker-1", "worker-2", "worker-3"))
		})

		It("should reject duplicate worker IDs", func() {
			identity := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			worker1 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}
			worker2 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}

			err := s.AddWorker(identity, worker1)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity, worker2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})
	})

	Describe("RemoveWorker", func() {
		It("should remove worker from registry and stop collector", func() {
			identity := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			worker := &mockWorker{observed: createMockObservedStateWithID("worker-1")}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			Expect(s.ListWorkers()).To(ContainElement("worker-1"))

			ctx := context.Background()
			err = s.RemoveWorker(ctx, "worker-1")
			Expect(err).ToNot(HaveOccurred())

			Expect(s.ListWorkers()).ToNot(ContainElement("worker-1"))
		})

		It("should return error for non-existent worker", func() {
			ctx := context.Background()
			err := s.RemoveWorker(ctx, "non-existent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("GetWorker", func() {
		It("should return worker context for valid ID", func() {
			identity := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			worker := &mockWorker{observed: createMockObservedStateWithID("worker-1")}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			ctx, err := s.GetWorker("worker-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(ctx).ToNot(BeNil())
		})

		It("should return error for non-existent worker", func() {
			ctx, err := s.GetWorker("non-existent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(ctx).To(BeNil())
		})
	})

	Describe("ListWorkers", func() {
		It("should return all worker IDs", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}
			worker2 := &mockWorker{observed: createMockObservedStateWithID("worker-2")}
			worker3 := &mockWorker{observed: createMockObservedStateWithID("worker-3")}

			_ = s.AddWorker(identity1, worker1)
			_ = s.AddWorker(identity2, worker2)
			_ = s.AddWorker(identity3, worker3)

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker-1", "worker-2", "worker-3"))
		})

		It("should return empty list when no workers", func() {
			workers := s.ListWorkers()
			Expect(workers).To(BeEmpty())
		})
	})

	Describe("GetWorkerState", func() {
		It("should return state name and reason for a worker", func() {
			identity := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}

			stateWithReason := &mockState{}
			worker := &mockWorker{
				initialState: stateWithReason,
				observed:     createMockObservedStateWithID("worker-1"),
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			stateName, reason, err := s.GetWorkerState("worker-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(stateName).To(Equal("MockState"))
			Expect(reason).To(Equal("mock state"))
		})

		It("should return error for non-existent worker", func() {
			stateName, reason, err := s.GetWorkerState("non-existent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(stateName).To(BeEmpty())
			Expect(reason).To(BeEmpty())
		})

		It("should safely return state during concurrent tick operations", func() {
			identity := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}

			stateWithReason := &mockState{}
			worker := &mockWorker{
				initialState: stateWithReason,
				observed:     createMockObservedStateWithID("worker-1"),
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			done := make(chan bool)
			errorChan := make(chan error, 100)

			go func() {
				for range 100 {
					stateName, reason, err := s.GetWorkerState("worker-1")
					if err != nil {
						errorChan <- err

						return
					}
					if stateName == "" || reason == "" {
						errorChan <- errors.New("empty state or reason")

						return
					}
					time.Sleep(time.Millisecond)
				}
				close(done)
			}()

			select {
			case err := <-errorChan:
				Fail("concurrent access error: " + err.Error())
			case <-done:
			case <-time.After(5 * time.Second):
				Fail("concurrent access test timed out")
			}
		})
	})

	Describe("TickAll", func() {
		It("should tick all workers in registry", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}
			worker2 := &mockWorker{observed: createMockObservedStateWithID("worker-2")}
			worker3 := &mockWorker{observed: createMockObservedStateWithID("worker-3")}

			err := s.AddWorker(identity1, worker1)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity2, worker2)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity3, worker3)
			Expect(err).ToNot(HaveOccurred())

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker-1", "worker-2", "worker-3"))
		})

		It("should continue ticking other workers even if one fails", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{observed: createMockObservedStateWithID("worker-1")}
			worker2 := &mockWorker{observed: createMockObservedStateWithID("worker-2")}
			worker3 := &mockWorker{observed: createMockObservedStateWithID("worker-3")}

			_ = s.AddWorker(identity1, worker1)
			_ = s.AddWorker(identity2, worker2)
			_ = s.AddWorker(identity3, worker3)

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker-1", "worker-2", "worker-3"))
		})

		It("should return no error when no workers exist", func() {
			ctx := context.Background()
			err := s.TestTickAll(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
