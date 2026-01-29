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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

var _ = Describe("StoppedState", func() {
	var (
		stateObj *state.StoppedState
		snap     fsmv2.Snapshot
	)

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	Describe("Next", func() {
		Context("when shutdown is not requested", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
					Observed: snapshot.CommunicatorObservedState{},
					Desired:  &snapshot.CommunicatorDesiredState{},
				}
			})

			It("should transition to TryingToAuthenticateState", func() {
				result := stateObj.Next(snap)
				Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			})

			It("should not signal anything", func() {
				result := stateObj.Next(snap)
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				result := stateObj.Next(snap)
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Stopped"))
		})
	})
})

var _ = Describe("StoppedState Transitions", func() {
	var stateObj *state.StoppedState

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	Describe("Stopped -> TryingToAuthenticateState", func() {
		It("should transition when shutdown is not requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{},
				Desired:  &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition with empty observed state", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "comm-1", Name: "communicator", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Stopped -> SignalNeedsRemoval", func() {
		It("should signal removal when shutdown is requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			Expect(result.Action).To(BeNil())
		})

		It("should stay in StoppedState and emit SignalNeedsRemoval on shutdown", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "comm-shutdown", Name: "communicator", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "valid-token",
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			Expect(result.Action).To(BeNil())
		})
	})
})
