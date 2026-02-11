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

var _ = Describe("RunningState", func() {
	var stateObj *state.RunningState

	BeforeEach(func() {
		stateObj = &state.RunningState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			It("should transition to StoppedState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt: time.Now(),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State:             "running",
							ShutdownRequested: true,
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when not healthy (consecutive action errors > 0)", func() {
			It("should transition to RunningDegradedState with nil action", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt:             time.Now(),
						ConsecutiveActionErrors: 1,
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningDegradedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when compaction is due", func() {
			It("should emit CompactDeltasAction", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt:      now,
						LastCompactionAt: now.Add(-10 * time.Minute),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.CompactDeltasAction{}))
				compactAction := result.Action.(*action.CompactDeltasAction)
				Expect(compactAction.RetentionWindow).To(Equal(24 * time.Hour))
			})
		})

		Context("when maintenance is due but compaction is not", func() {
			It("should emit RunMaintenanceAction", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt:       now,
						LastCompactionAt:  now.Add(-1 * time.Minute),
						LastMaintenanceAt: now.Add(-8 * 24 * time.Hour),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when both compaction and maintenance are due", func() {
			It("should emit CompactDeltasAction (higher priority)", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt:       now,
						LastCompactionAt:  now.Add(-10 * time.Minute),
						LastMaintenanceAt: now.Add(-8 * 24 * time.Hour),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.CompactDeltasAction{}))
			})
		})

		Context("when nothing is due", func() {
			It("should return same state with nil action (idle)", func() {
				now := time.Now()
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt:       now,
						LastCompactionAt:  now.Add(-1 * time.Minute),
						LastMaintenanceAt: now.Add(-1 * time.Hour),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when LastCompactionAt is zero (first run)", func() {
			It("should emit CompactDeltasAction immediately", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt: time.Now(),
					},
					Desired: &snapshot.PersistenceDesiredState{
						BaseDesiredState: fsmv2config.BaseDesiredState{
							State: "running",
						},
						CompactionInterval:  5 * time.Minute,
						RetentionWindow:     24 * time.Hour,
						MaintenanceInterval: 7 * 24 * time.Hour,
					},
				}

				result := stateObj.Next(snap)
				Expect(result.Action).To(BeAssignableToTypeOf(&action.CompactDeltasAction{}))
			})
		})
	})

	Describe("String", func() {
		It("should return Running", func() {
			Expect(stateObj.String()).To(Equal("Running"))
		})
	})
})
