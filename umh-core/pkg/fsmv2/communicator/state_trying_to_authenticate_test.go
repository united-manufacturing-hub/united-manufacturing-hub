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

package communicator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

var _ = Describe("TryingToAuthenticateState", func() {
	var (
		state         *communicator.TryingToAuthenticateState
		snapshot      fsmv2.Snapshot
		desired       *communicator.CommunicatorDesiredState
		observed      *communicator.CommunicatorObservedState
		worker        *communicator.CommunicatorWorker
		mockTransport *MockTransport
		inboundChan   chan *transportpkg.UMHMessage
		outboundChan  chan *transportpkg.UMHMessage
		logger        *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransport = NewMockTransport()
		inboundChan = make(chan *transportpkg.UMHMessage, 10)
		outboundChan = make(chan *transportpkg.UMHMessage, 10)
		worker = communicator.NewCommunicatorWorker(
			"test-communicator",
			"https://relay.example.com",
			inboundChan,
			outboundChan,
			mockTransport,
			"test-uuid",
			"test-token",
			logger,
		)
		state = &communicator.TryingToAuthenticateState{Worker: worker}
		desired = &communicator.CommunicatorDesiredState{}
		observed = &communicator.CommunicatorObservedState{}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(true)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to StoppedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.StoppedState{}))
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

		Context("when authentication is successful", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetJWTToken("valid-jwt-token")
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to SyncingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.SyncingState{}))
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

		Context("when authentication is not yet successful", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(false)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in TryingToAuthenticateState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.TryingToAuthenticateState{}))
			})

			It("should emit AuthenticateAction", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeAssignableToTypeOf(&communicator.AuthenticateAction{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(state.String()).To(Equal("TryingToAuthenticate"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(state.Reason()).To(Equal("Attempting to authenticate with relay server"))
		})
	})
})
