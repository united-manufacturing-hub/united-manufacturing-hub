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

	Context("when worker returns SignalNeedsRestart", func() {
		It("should restart collector for that worker", func() {
			restartState := &mockState{
				signal: fsmv2.SignalNeedsRestart,
			}
			restartState.nextState = restartState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: restartState}, store, supervisor.CollectorHealthConfig{
				MaxRestartAttempts: 3,
			})

			initialRestartCount := s.GetRestartCount()
			Expect(initialRestartCount).To(Equal(0))

			ctx := context.Background()
			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			finalRestartCount := s.GetRestartCount()
			Expect(finalRestartCount).To(Equal(1))
		})

		It("should increment restart count", func() {
			restartState := &mockState{
				signal: fsmv2.SignalNeedsRestart,
			}
			restartState.nextState = restartState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: restartState}, store, supervisor.CollectorHealthConfig{
				MaxRestartAttempts: 3,
			})

			ctx := context.Background()
			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			restartCount := s.GetRestartCount()
			Expect(restartCount).To(Equal(1))
		})

		It("should call RestartCollector when signal is received", func() {
			restartState := &mockState{
				signal: fsmv2.SignalNeedsRestart,
			}
			restartState.nextState = restartState

			store := &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s := newSupervisorWithWorker(&mockWorker{initialState: restartState}, store, supervisor.CollectorHealthConfig{
				MaxRestartAttempts: 3,
			})

			ctx := context.Background()

			Expect(s.GetRestartCount()).To(Equal(0))

			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetRestartCount()).To(Equal(1))
		})
	})
})
