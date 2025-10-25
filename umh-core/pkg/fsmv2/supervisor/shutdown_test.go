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

var _ = Describe("Shutdown Escalation", func() {
	Context("when max restart attempts exhausted", func() {
		It("should request graceful shutdown", func() {
			shutdownRequested := false
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					return &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{shutdown: shutdownRequested},
						Observed: &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)},
					}, nil
				},
				saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
					if desired.ShutdownRequested() {
						shutdownRequested = true
					}

					return nil
				},
			}

			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				StaleThreshold:     10 * time.Second,
				Timeout:            20 * time.Second,
				MaxRestartAttempts: 3,
			})

			s.SetRestartCount(3)

			err := s.Tick(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collector unresponsive"))

			Expect(shutdownRequested).To(BeTrue())
		})
	})

	Context("while restart attempts remain", func() {
		It("should NOT request shutdown", func() {
			shutdownRequested := false
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					return &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)},
					}, nil
				},
				saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
					if desired.ShutdownRequested() {
						shutdownRequested = true
					}

					return nil
				},
			}

			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				StaleThreshold:     10 * time.Second,
				Timeout:            20 * time.Second,
				MaxRestartAttempts: 3,
			})

			s.SetRestartCount(1)

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(shutdownRequested).To(BeFalse())
		})
	})
})
