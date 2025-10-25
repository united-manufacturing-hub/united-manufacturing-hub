// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("DataFreshness Full Cycle Integration", func() {
	It("should validate complete 4-layer defense lifecycle", func() {
		shutdownRequested := false
		var snapshotTimestamp time.Time

		worker := &mockWorker{
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &mockObservedState{timestamp: snapshotTimestamp}, nil
			},
		}

		worker.initialState = &mockState{}

		store := &mockStore{
			loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
				return &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{shutdown: shutdownRequested},
					Observed: &mockObservedState{timestamp: snapshotTimestamp},
				}, nil
			},
			saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
				if desired.ShutdownRequested() {
					shutdownRequested = true
				}

				return nil
			},
		}

		s := newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{
			StaleThreshold:     5 * time.Second,  // Shortened for test
			Timeout:            10 * time.Second, // Shortened for test
			MaxRestartAttempts: 3,
		})

		ctx := context.Background()

		// Phase 1: Normal operation (fresh data)
		By("Phase 1: Normal operation with fresh data")
		snapshotTimestamp = time.Now()
		err := s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(0))

		// Phase 2: Stale data (pause FSM, no restart yet)
		By("Phase 2: Stale data (pause FSM)")
		snapshotTimestamp = time.Now().Add(-7 * time.Second) // >5s but <10s
		err = s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(0), "should not restart for stale (but not timeout) data")

		// Phase 3: Timeout (trigger restarts)
		By("Phase 3: Timeout triggers collector restarts")
		snapshotTimestamp = time.Now().Add(-15 * time.Second) // >10s timeout

		// First restart
		err = s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(1))

		// Second restart
		err = s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(2))

		// Third restart
		err = s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(3))

		// Phase 4: Max restarts exceeded (trigger shutdown)
		By("Phase 4: Max restarts exceeded triggers shutdown")
		err = s.Tick(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collector unresponsive"))
		Expect(shutdownRequested).To(BeTrue(), "shutdown should be requested after max restart attempts")

		// Phase 5: Collector recovers (reset counter)
		By("Phase 5: Collector recovery resets counter")
		s.SetRestartCount(2) // Had previous failures
		shutdownRequested = false

		snapshotTimestamp = time.Now() // Fresh data!

		err = s.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.GetRestartCount()).To(Equal(0), "restart counter should reset on recovery")
	})
})
