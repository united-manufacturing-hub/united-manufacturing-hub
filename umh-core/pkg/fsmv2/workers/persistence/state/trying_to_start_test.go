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

var _ = Describe("TryingToStartState", func() {
	var stateObj *state.TryingToStartState

	BeforeEach(func() {
		stateObj = &state.TryingToStartState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			It("should transition to StoppedState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
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
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when maintenance action succeeded", func() {
			It("should transition to RunningState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: true},
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
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

		Context("when no action results yet (first tick)", func() {
			It("should emit RunMaintenanceAction and stay", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStartState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when maintenance action failed", func() {
			It("should re-emit RunMaintenanceAction and stay", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: false},
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
						},
						Config: persistence.PersistenceConfig{
							CompactionInterval:  5 * time.Minute,
							RetentionWindow:     24 * time.Hour,
							MaintenanceInterval: 7 * 24 * time.Hour,
						},
					},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStartState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeAssignableToTypeOf(&action.RunMaintenanceAction{}))
			})
		})

		Context("when shutdown is requested and action succeeded", func() {
			It("should prioritize shutdown over action result", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", WorkerType: "persistence"},
					Observed: fsmv2.Observation[persistence.PersistenceStatus]{
						CollectedAt: time.Now(),
						LastActionResults: []deps.ActionResult{
							{ActionType: "RunMaintenance", Success: true},
						},
					},
					Desired: &fsmv2.WrappedDesiredState[persistence.PersistenceConfig]{
						BaseDesiredState: fsmv2config.BaseDesiredState{
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
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return TryingToStart", func() {
			Expect(stateObj.String()).To(Equal("TryingToStart"))
		})
	})
})
