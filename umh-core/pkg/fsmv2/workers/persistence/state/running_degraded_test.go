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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

var _ = Describe("RunningDegradedState", func() {
	var stateObj *state.RunningDegradedState

	BeforeEach(func() {
		stateObj = &state.RunningDegradedState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			It("should transition to ShuttingDownState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
						Status: persistence.PersistenceStatus{
							ConsecutiveActionErrors: 1,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State:             "running",
							ShutdownRequested: true,
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.ShuttingDownState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when healthy and last action healthy (recovered)", func() {
			It("should transition to RunningState with nil action", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
						Status: persistence.PersistenceStatus{
							ConsecutiveActionErrors: 0,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when still unhealthy and compaction is due", func() {
			It("should stay degraded and emit CompactDeltasAction", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: now,
						Status: persistence.PersistenceStatus{
							LastCompactionAt:        now.Add(-10 * time.Minute),
							ConsecutiveActionErrors: 2,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningDegradedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.CompactDeltasAction{}))
			})
		})

		Context("when still unhealthy and maintenance is due", func() {
			It("should stay degraded and emit RunMaintenanceAction", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: now,
						Status: persistence.PersistenceStatus{
							LastCompactionAt:        now.Add(-1 * time.Minute),
							LastMaintenanceAt:       now.Add(-10 * 24 * time.Hour),
							ConsecutiveActionErrors: 1,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningDegradedState{}))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when still unhealthy and both are due", func() {
			It("should emit CompactDeltasAction (higher priority)", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: now,
						Status: persistence.PersistenceStatus{
							LastCompactionAt:        now.Add(-10 * time.Minute),
							LastMaintenanceAt:       now.Add(-10 * 24 * time.Hour),
							ConsecutiveActionErrors: 3,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningDegradedState{}))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.CompactDeltasAction{}))
			})
		})

		Context("when still unhealthy and nothing is due", func() {
			It("should stay degraded with nil action (idle)", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: now,
						Status: persistence.PersistenceStatus{
							LastCompactionAt:        now.Add(-1 * time.Minute),
							LastMaintenanceAt:       now.Add(-1 * time.Hour),
							ConsecutiveActionErrors: 1,
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningDegradedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return RunningDegraded", func() {
			Expect(stateObj.String()).To(Equal("RunningDegraded"))
		})
	})
})
