// Copyright 2025 UMH Systems GmbH
package container_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// These tests enforce the architectural boundary between Supervisor and States.
//
// From README.md (lines 122-141): "Collector health is a supervisor concern, not
// a state concern. States never see stale data - they assume observations are
// always fresh and focus purely on business logic."
//
// The supervisor checks data freshness BEFORE calling state.Next(). If data is
// stale, the supervisor pauses the FSM and handles collector recovery.
//
// These tests verify states make decisions based ONLY on health metrics, never
// timestamps. If a state starts checking CollectedAt, these tests will fail,
// preventing architectural erosion.
var _ = Describe("State Timestamp Independence", func() {
	Describe("ActiveState", func() {
		Context("when observation data is very stale", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &container.ActiveState{}

				// Create snapshot with VERY old data (60s)
				// States should never check this - supervisor's job
				observed := &container.ContainerObservedState{
					OverallHealth: models.Active,
					CPUHealth:     models.Active,
					MemoryHealth:  models.Active,
					DiskHealth:    models.Active,
					CollectedAt:   time.Now().Add(-60 * time.Second), ObservedThresholds: // VERY stale
					standardThresholds(),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &container.ContainerDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				// Should stay Active (not transition to Degraded due to stale data)
				_, ok := nextState.(*container.ActiveState)
				Expect(ok).To(BeTrue(), "should remain in ActiveState with stale timestamp")
			})
		})
	})

	Describe("DegradedState", func() {
		Context("when observation data is very stale but metrics are healthy", func() {
			It("should not check timestamp - focus only on health metrics", func() {
				state := &container.DegradedState{}

				// Create snapshot with old data (60s) but healthy metrics
				observed := &container.ContainerObservedState{
					OverallHealth: models.Active, // Metrics recovered!
					CPUHealth:     models.Active,
					MemoryHealth:  models.Active,
					DiskHealth:    models.Active,
					CollectedAt:   time.Now().Add(-60 * time.Second), ObservedThresholds: // VERY stale
					standardThresholds(),
				}

				snapshot := fsmv2.Snapshot{
					Desired:  &container.ContainerDesiredState{},
					Observed: observed,
				}

				nextState, _, _ := state.Next(snapshot)

				// Should transition to Active (metrics healthy, ignore timestamp)
				_, ok := nextState.(*container.ActiveState)
				Expect(ok).To(BeTrue(), "should transition to Active on healthy metrics (ignoring timestamp)")
			})
		})
	})
})
