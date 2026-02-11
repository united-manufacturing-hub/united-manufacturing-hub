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

var _ = Describe("ShuttingDownState", func() {
	var stateObj *state.ShuttingDownState

	BeforeEach(func() {
		stateObj = &state.ShuttingDownState{}
	})

	Describe("Next", func() {
		Context("when maintenance action succeeded", func() {
			It("should transition to StoppedState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: true},
						},
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

		Context("when no action results yet (first tick)", func() {
			It("should emit RunMaintenanceAction and stay", func() {
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
				Expect(result.State).To(BeAssignableToTypeOf(&state.ShuttingDownState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when maintenance action failed", func() {
			It("should re-emit RunMaintenanceAction and stay", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: false},
						},
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
				Expect(result.State).To(BeAssignableToTypeOf(&state.ShuttingDownState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when multiple ticks without success", func() {
			It("should keep emitting RunMaintenanceAction on each tick", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: snapshot.PersistenceObservedState{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: false},
						},
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

				result1 := stateObj.Next(snap)
				Expect(result1.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))

				result2 := stateObj.Next(snap)
				Expect(result2.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})
	})

	Describe("String", func() {
		It("should return ShuttingDown", func() {
			Expect(stateObj.String()).To(Equal("ShuttingDown"))
		})
	})
})
