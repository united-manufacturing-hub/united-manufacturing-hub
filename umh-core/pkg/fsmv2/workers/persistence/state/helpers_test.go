// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package state_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

var _ = Describe("Preferential Maintenance Scheduling", func() {
	// All tests use fixed times to avoid CI flakiness from day/time dependence.
	// Saturday 03:00 UTC = preferred window (weekend + low-traffic)
	// Monday 12:00 UTC = no window (weekday + business hours)
	// Monday 03:00 UTC = acceptable window (low-traffic, not weekend)

	var stateObj *state.RunningState
	maintenanceInterval := 7 * 24 * time.Hour

	BeforeEach(func() {
		stateObj = &state.RunningState{}
	})

	makeSnap := func(collectedAt time.Time, lastMaintenanceAt time.Time, isPreferred, isAcceptable bool) fsmv2.Snapshot {
		return fsmv2.Snapshot{
			Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
			Observed: snapshot.PersistenceObservedState{
				CollectedAt:                   collectedAt,
				LastCompactionAt:              collectedAt.Add(-1 * time.Minute),
				LastMaintenanceAt:             lastMaintenanceAt,
				IsPreferredMaintenanceWindow:  isPreferred,
				IsAcceptableMaintenanceWindow: isAcceptable,
			},
			Desired: &snapshot.PersistenceDesiredState{
				BaseDesiredState: fsmv2config.BaseDesiredState{
					State: "running",
				},
				CompactionInterval:  5 * time.Minute,
				RetentionWindow:     24 * time.Hour,
				MaintenanceInterval: maintenanceInterval,
			},
		}
	}

	Context("short intervals (< 3 days)", func() {
		It("should fire when elapsed >= interval", func() {
			now := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC) // Monday noon
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
				Observed: snapshot.PersistenceObservedState{
					CollectedAt:      now,
					LastCompactionAt: now.Add(-1 * time.Minute),
					LastMaintenanceAt: now.Add(-25 * time.Hour),
				},
				Desired: &snapshot.PersistenceDesiredState{
					BaseDesiredState: fsmv2config.BaseDesiredState{
						State: "running",
					},
					CompactionInterval:  5 * time.Minute,
					RetentionWindow:     24 * time.Hour,
					MaintenanceInterval: 24 * time.Hour,
				},
			}

			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})

		It("should NOT fire when elapsed < interval", func() {
			now := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC) // Monday noon
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
				Observed: snapshot.PersistenceObservedState{
					CollectedAt:       now,
					LastCompactionAt:  now.Add(-1 * time.Minute),
					LastMaintenanceAt: now.Add(-23 * time.Hour),
				},
				Desired: &snapshot.PersistenceDesiredState{
					BaseDesiredState: fsmv2config.BaseDesiredState{
						State: "running",
					},
					CompactionInterval:  5 * time.Minute,
					RetentionWindow:     24 * time.Hour,
					MaintenanceInterval: 24 * time.Hour,
				},
			}

			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})
	})

	Context("long intervals: preferred window (early start)", func() {
		It("should fire on weekend night when in early window (day 5-6)", func() {
			// Saturday 03:00, maintenance ran 5.5 days ago (earlyStart = 5 days)
			sat3am := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC) // Saturday
			lastMaint := sat3am.Add(-5*24*time.Hour - 12*time.Hour)

			snap := makeSnap(sat3am, lastMaint, true, true)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})

		It("should NOT fire on weekday daytime when in early window", func() {
			// Monday 12:00, maintenance ran 5.5 days ago
			mon12pm := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC) // Monday
			lastMaint := mon12pm.Add(-5*24*time.Hour - 12*time.Hour)

			snap := makeSnap(mon12pm, lastMaint, false, false)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})
	})

	Context("long intervals: acceptable window (past target)", func() {
		It("should fire on weekday night when past interval", func() {
			// Monday 03:00, maintenance ran 7.5 days ago
			mon3am := time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC) // Monday
			lastMaint := mon3am.Add(-7*24*time.Hour - 12*time.Hour)

			snap := makeSnap(mon3am, lastMaint, false, true)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})

		It("should fire on weekend daytime when past interval", func() {
			// Saturday 12:00, maintenance ran 7.5 days ago
			sat12pm := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
			lastMaint := sat12pm.Add(-7*24*time.Hour - 12*time.Hour)

			snap := makeSnap(sat12pm, lastMaint, false, true)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})

		It("should NOT fire on weekday noon when past interval but no window", func() {
			// Monday 12:00, maintenance ran 7.5 days ago
			mon12pm := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			lastMaint := mon12pm.Add(-7*24*time.Hour - 12*time.Hour)

			snap := makeSnap(mon12pm, lastMaint, false, false)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})
	})

	Context("long intervals: overdue (past late deadline)", func() {
		It("should fire regardless of day/time when overdue", func() {
			// Monday 12:00, maintenance ran 10 days ago (lateDeadline = 9 days)
			mon12pm := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			lastMaint := mon12pm.Add(-10 * 24 * time.Hour)

			snap := makeSnap(mon12pm, lastMaint, false, false)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})
	})

	Context("loop prevention", func() {
		It("should NOT fire when maintenance just ran (timeSince resets)", func() {
			// Saturday 03:00, maintenance ran 30 minutes ago
			sat3am := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			lastMaint := sat3am.Add(-30 * time.Minute)

			snap := makeSnap(sat3am, lastMaint, true, true)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})

		It("should NOT fire when still early even on preferred window", func() {
			// Saturday 03:00, maintenance ran 30 hours ago (< earlyStart of 5 days)
			sat3am := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			lastMaint := sat3am.Add(-30 * time.Hour)

			snap := makeSnap(sat3am, lastMaint, true, true)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})
	})

	Context("boundary: interval exactly at threshold (3 days)", func() {
		It("should use preferential scheduling at threshold", func() {
			// 3-day interval, 3.5 days elapsed, no window
			mon12pm := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			lastMaint := mon12pm.Add(-3*24*time.Hour - 12*time.Hour)

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
				Observed: snapshot.PersistenceObservedState{
					CollectedAt:                   mon12pm,
					LastCompactionAt:              mon12pm.Add(-1 * time.Minute),
					LastMaintenanceAt:             lastMaint,
					IsPreferredMaintenanceWindow:  false,
					IsAcceptableMaintenanceWindow: false,
				},
				Desired: &snapshot.PersistenceDesiredState{
					BaseDesiredState: fsmv2config.BaseDesiredState{
						State: "running",
					},
					CompactionInterval:  5 * time.Minute,
					RetentionWindow:     24 * time.Hour,
					MaintenanceInterval: 3 * 24 * time.Hour,
				},
			}

			result := stateObj.Next(snap)
			Expect(result.Action).To(BeNil())
		})
	})

	Context("first run (zero timestamps)", func() {
		It("should fire maintenance when LastMaintenanceAt is zero (overdue)", func() {
			now := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			snap := makeSnap(now, time.Time{}, false, false)
			result := stateObj.Next(snap)
			Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
		})
	})
})
