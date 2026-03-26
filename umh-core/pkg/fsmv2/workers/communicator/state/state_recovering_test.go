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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

var _ = Describe("RecoveringState", func() {
	var (
		stateObj *state.RecoveringState
		logger   deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		_ = logger
		stateObj = &state.RecoveringState{}
	})

	Describe("Next", func() {
		Context("when children recover", func() {
			It("should transition to SyncingState", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
					Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
						ChildrenHealthy:   1,
						ChildrenUnhealthy: 0,
					},
					Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when children are still unhealthy", func() {
			It("should stay in RecoveringState with nil action", func() {
				snap := fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
					Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
						ChildrenHealthy:   0,
						ChildrenUnhealthy: 1,
					},
					Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
				}

				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Recovering"))
		})
	})
})

var _ = Describe("RecoveringState Transitions", func() {
	var stateObj *state.RecoveringState

	BeforeEach(func() {
		stateObj = &state.RecoveringState{}
	})

	Describe("Recovering -> StoppedState", func() {
		It("should transition to StoppedState when shutdown is requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 1,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should prioritize shutdown over recovery", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   1,
					ChildrenUnhealthy: 0,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Recovering -> SyncingState", func() {
		It("should transition when all children become healthy", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   1,
					ChildrenUnhealthy: 0,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should NOT transition when some children are still unhealthy", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   1,
					ChildrenUnhealthy: 1,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
		})
	})

	Describe("Recovering -> self (waiting)", func() {
		It("should stay in RecoveringState when no children exist yet", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 0,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should stay in RecoveringState with nil action when children unhealthy", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.Observation[communicator.CommunicatorStatus]{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 2,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})
})
