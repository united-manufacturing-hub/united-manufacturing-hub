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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/state"
)

var _ = Describe("StoppedState", func() {
	var (
		stateObj *state.StoppedState
		snap     snapshot.CommunicatorSnapshot
	)

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	Describe("Next", func() {
		XContext("when shutdown is requested", func() {
			// Skip: Cannot test private shutdownRequested field from external package
		})

		Context("when shutdown is not requested", func() {
			BeforeEach(func() {
				snap = snapshot.CommunicatorSnapshot{
					Desired:  snapshot.CommunicatorDesiredState{},
					Observed: snapshot.CommunicatorObservedState{},
				}
			})

			It("should transition to TryingToAuthenticateState", func() {
				nextState, _, _ := stateObj.Next(snap)
				Expect(nextState).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := stateObj.Next(snap)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := stateObj.Next(snap)
				Expect(action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Stopped"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(stateObj.Reason()).To(Equal("Communicator is stopped"))
		})
	})
})
