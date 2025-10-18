// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Data Freshness Checking", func() {
	var (
		s        *supervisor.Supervisor
		snapshot *fsmv2.Snapshot
	)

	BeforeEach(func() {
		snapshot = &fsmv2.Snapshot{
			Identity: mockIdentity(),
			Desired:  &mockDesiredState{},
		}
	})

	Context("when observation data is fresh", func() {
		It("should return true", func() {
			s = supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold: 10 * time.Second,
				},
			})

			snapshot.Observed = &mockObservedState{timestamp: time.Now()}

			Expect(s.CheckDataFreshness(snapshot)).To(BeTrue())
		})
	})

	Context("when observation data is stale", func() {
		It("should return false", func() {
			s = supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold: 10 * time.Second,
					Timeout:        20 * time.Second,
				},
			})

			snapshot.Observed = &mockObservedState{timestamp: time.Now().Add(-15 * time.Second)}

			Expect(s.CheckDataFreshness(snapshot)).To(BeFalse())
		})
	})

	Context("when observation data has timed out", func() {
		It("should return false", func() {
			s = supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					Timeout: 20 * time.Second,
				},
			})

			snapshot.Observed = &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)}

			Expect(s.CheckDataFreshness(snapshot)).To(BeFalse())
		})
	})
})
