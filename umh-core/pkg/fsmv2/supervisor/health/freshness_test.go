// Copyright 2025 UMH Systems GmbH
package health_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/health"
	"go.uber.org/zap"
)

var _ = Describe("FreshnessChecker", func() {
	Context("when observation data is fresh", func() {
		It("should return true", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				zap.NewNop().Sugar(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now(),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.Check(snapshot)).To(BeTrue())
		})
	})

	Context("when observation data is stale", func() {
		It("should return false", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				zap.NewNop().Sugar(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now().Add(-15 * time.Second),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.Check(snapshot)).To(BeFalse())
		})
	})

	Context("when observation data has timed out", func() {
		It("should detect timeout", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				zap.NewNop().Sugar(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now().Add(-25 * time.Second),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.IsTimeout(snapshot)).To(BeTrue())
		})
	})
})
