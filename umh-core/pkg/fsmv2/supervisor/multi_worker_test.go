package supervisor_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Multi-Worker Supervisor", func() {
	var (
		s     *supervisor.Supervisor
		store *mockStore
	)

	BeforeEach(func() {
		store = &mockStore{}

		s = supervisor.NewSupervisor(supervisor.Config{
			WorkerType: "container",
			Store:      store,
			Logger:     zap.NewNop().Sugar(),
		})
	})

	Describe("AddWorker", func() {
		It("should add worker to registry", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

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
			worker1 := &mockWorker{}
			worker2 := &mockWorker{}

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
			worker := &mockWorker{}

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
			worker := &mockWorker{}

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

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

			s.AddWorker(identity1, worker1)
			s.AddWorker(identity2, worker2)
			s.AddWorker(identity3, worker3)

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker-1", "worker-2", "worker-3"))
		})

		It("should return empty list when no workers", func() {
			workers := s.ListWorkers()
			Expect(workers).To(BeEmpty())
		})
	})

	Describe("TickAll", func() {
		It("should tick all workers in registry", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

			err := s.AddWorker(identity1, worker1)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity2, worker2)
			Expect(err).ToNot(HaveOccurred())

			err = s.AddWorker(identity3, worker3)
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			err = s.TickAll(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should continue ticking other workers even if one fails", func() {
			identity1 := fsmv2.Identity{ID: "worker-1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker-2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker-3", Name: "Worker 3"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

			s.AddWorker(identity1, worker1)
			s.AddWorker(identity2, worker2)
			s.AddWorker(identity3, worker3)

			failingStore := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					if id == "worker-2" {
						return nil, errors.New("simulated failure for worker-2")
					}
					return &fsmv2.Snapshot{
						Identity: fsmv2.Identity{ID: id, Name: "Test"},
						Observed: &mockObservedState{timestamp: time.Now()},
						Desired:  &mockDesiredState{},
					}, nil
				},
			}

			s2 := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "container",
				Store:      failingStore,
				Logger:     zap.NewNop().Sugar(),
			})

			s2.AddWorker(identity1, worker1)
			s2.AddWorker(identity2, worker2)
			s2.AddWorker(identity3, worker3)

			ctx := context.Background()
			err := s2.TickAll(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("worker-2"))
		})

		It("should return no error when no workers exist", func() {
			ctx := context.Background()
			err := s.TickAll(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
