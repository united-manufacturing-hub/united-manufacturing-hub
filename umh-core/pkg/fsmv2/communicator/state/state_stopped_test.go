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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
)

var _ = Describe("StoppedState", func() {
	var (
		state    *communicator.StoppedState
		snapshot fsmv2.Snapshot
		desired  *communicator.CommunicatorDesiredState
	)

	BeforeEach(func() {
		state = &communicator.StoppedState{}
		desired = &communicator.CommunicatorDesiredState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(true)
				snapshot = fsmv2.Snapshot{
					Desired: desired,
				}
			})

			It("should stay in stopped state", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(Equal(state))
			})

			It("should signal removal", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNeedsRemoval))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when shutdown is not requested", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				snapshot = fsmv2.Snapshot{
					Desired: desired,
				}
			})

			It("should transition to TryingToAuthenticateState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.TryingToAuthenticateState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(state.String()).To(Equal("Stopped"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(state.Reason()).To(Equal("Communicator is stopped"))
		})
	})
})
