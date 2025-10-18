// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Supervisor Configuration", func() {
	Context("when creating supervisor with default config", func() {
		It("should use default collector health thresholds", func() {
			cfg := supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
			}

			s := supervisor.NewSupervisor(cfg)

			Expect(s.GetStaleThreshold()).To(Equal(10 * time.Second))
			Expect(s.GetCollectorTimeout()).To(Equal(20 * time.Second))
			Expect(s.GetMaxRestartAttempts()).To(Equal(3))
		})
	})

	Context("when creating supervisor with custom config", func() {
		It("should use custom collector health thresholds", func() {
			cfg := supervisor.Config{
				Worker:   &mockWorker{},
				Identity: mockIdentity(),
				Store:    &mockStore{},
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold:     5 * time.Second,
					Timeout:            15 * time.Second,
					MaxRestartAttempts: 5,
				},
			}

			s := supervisor.NewSupervisor(cfg)

			Expect(s.GetStaleThreshold()).To(Equal(5 * time.Second))
			Expect(s.GetCollectorTimeout()).To(Equal(15 * time.Second))
			Expect(s.GetMaxRestartAttempts()).To(Equal(5))
		})
	})
})
