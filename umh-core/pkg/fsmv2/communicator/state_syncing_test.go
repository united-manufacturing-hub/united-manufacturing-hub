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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
)

var _ = Describe("SyncingState", func() {
	var (
		state          *communicator.SyncingState
		snapshot       fsmv2.Snapshot
		desired        *communicator.CommunicatorDesiredState
		observed       *communicator.CommunicatorObservedState
		worker         *communicator.CommunicatorWorker
		mockOrch       *MockOrchestrator
		logger         *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockOrch = NewMockOrchestrator()
		worker = communicator.NewCommunicatorWorker(
			"test-communicator",
			"https://relay.example.com",
			mockOrch,
			"test-uuid",
			"test-token",
			logger,
		)
		state = &communicator.SyncingState{Worker: worker}
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

		Context("when sync has errors", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(false)
				observed.SetConsecutiveErrors(3)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.DegradedState{}))
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

		Context("when authentication is invalid", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(false)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
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

		Context("when sync is healthy", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(true)
				observed.SetConsecutiveErrors(0)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in SyncingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.SyncingState{}))
			})

			It("should emit SyncAction", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeAssignableToTypeOf(&communicator.SyncAction{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(state.String()).To(Equal("Syncing"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(state.Reason()).To(Equal("Syncing CSE deltas with relay server"))
		})
	})

	Describe("Token expiration handling", func() {
		Context("when token is expired", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(true)
				observed.SetTokenExpiresAt(time.Now().Add(-1 * time.Hour))
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
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

		Context("when token is expiring soon (within 10-minute buffer)", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(true)
				observed.SetTokenExpiresAt(time.Now().Add(5 * time.Minute))
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to TryingToAuthenticateState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.TryingToAuthenticateState{}))
			})
		})

		Context("when token is not expired", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(true)
				observed.SetTokenExpiresAt(time.Now().Add(15 * time.Minute))
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in SyncingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.SyncingState{}))
			})

			It("should emit SyncAction", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeAssignableToTypeOf(&communicator.SyncAction{}))
			})
		})

		Context("when token expiration is zero value", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.SetAuthenticated(true)
				observed.SetSyncHealthy(true)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in SyncingState for backward compatibility", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&communicator.SyncingState{}))
			})

			It("should emit SyncAction", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeAssignableToTypeOf(&communicator.SyncAction{}))
			})
		})
	})
})
