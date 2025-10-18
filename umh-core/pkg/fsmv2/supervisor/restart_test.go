// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Collector Restart Logic", func() {
	Context("when restart is successful", func() {
		It("should increment restart count", func() {
			s := supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				},
			})

			err := s.RestartCollector(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(s.GetRestartCount()).To(Equal(1))
		})
	})

	Context("when max restart attempts exceeded", func() {
		It("should return error", func() {
			s := supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				},
			})

			s.SetRestartCount(3)

			err := s.RestartCollector(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeded max attempts"))
			Expect(s.GetRestartCount()).To(Equal(3))
		})
	})

	Context("when collector recovers", func() {
		It("should reset restart counter", func() {
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					return &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					}, nil
				},
			}

			s := supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    store,
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold: 10 * time.Second,
				},
			})

			s.SetRestartCount(2)
			Expect(s.GetRestartCount()).To(Equal(2))

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(s.GetRestartCount()).To(Equal(0))
		})
	})
})
