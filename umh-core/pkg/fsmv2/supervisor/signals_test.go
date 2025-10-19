package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Signal Handling", func() {
	Context("when worker returns SignalNeedsRemoval", func() {
		It("should remove worker from registry", func() {
			removalState := &mockState{
				signal: fsmv2.SignalNeedsRemoval,
			}
			removalState.nextState = removalState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: removalState}, store, supervisor.CollectorHealthConfig{})

			workersBefore := s.ListWorkers()
			Expect(workersBefore).To(HaveLen(1))

			ctx := context.Background()
			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			workersAfter := s.ListWorkers()
			Expect(workersAfter).To(BeEmpty())
		})

		It("should stop collector when removing worker", func() {
			removalState := &mockState{
				signal: fsmv2.SignalNeedsRemoval,
			}
			removalState.nextState = removalState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: removalState}, store, supervisor.CollectorHealthConfig{})

			ctx := context.Background()
			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(100 * time.Millisecond)

			workerID := s.ListWorkers()
			Expect(workerID).To(BeEmpty())
		})

		It("should handle removal after state transition and action execution", func() {
			executedAction := false
			mockAction := &mockAction{
				executeFunc: func(ctx context.Context) error {
					executedAction = true
					return nil
				},
			}

			removalState := &mockState{
				signal: fsmv2.SignalNeedsRemoval,
				action: mockAction,
			}
			removalState.nextState = removalState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: removalState}, store, supervisor.CollectorHealthConfig{})

			ctx := context.Background()
			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(executedAction).To(BeTrue())

			workersAfter := s.ListWorkers()
			Expect(workersAfter).To(BeEmpty())
		})
	})
})
