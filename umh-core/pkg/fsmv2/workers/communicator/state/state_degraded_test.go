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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

var _ = Describe("DegradedState", func() {
	var (
		stateObj *state.DegradedState
		logger   *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		_ = logger
		stateObj = state.NewDegradedState()
	})

	Describe("Next", func() {
		Context("when sync recovers", func() {
			var snap snapshot.CommunicatorSnapshot

			BeforeEach(func() {
				snap = snapshot.CommunicatorSnapshot{
					Desired: snapshot.CommunicatorDesiredState{},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: true,
					},
				}
			})

			It("should transition to SyncingState", func() {
				nextState, _, _ := stateObj.Next(snap)
				Expect(nextState).To(BeAssignableToTypeOf(&state.SyncingState{}))
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

		Context("when sync is still unhealthy", func() {
			var snap snapshot.CommunicatorSnapshot

			BeforeEach(func() {
				snap = snapshot.CommunicatorSnapshot{
					Desired: snapshot.CommunicatorDesiredState{},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: false,
					},
				}
			})

			It("should stay in DegradedState", func() {
				nextState, _, _ := stateObj.Next(snap)
				Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			})

			It("should emit SyncAction to retry", func() {
				_, _, action := stateObj.Next(snap)
				Expect(action).NotTo(BeNil())
				Expect(action.Name()).To(Equal("sync"))
			})

			It("should not signal anything", func() {
				_, signal, _ := stateObj.Next(snap)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Degraded"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(stateObj.Reason()).To(Equal("Sync is experiencing errors"))
		})
	})
})

var _ = Describe("DegradedState Backoff", func() {
	It("stays in degraded state during backoff period", func() {
		// Arrange: 5 consecutive errors = 32s backoff (2^5 = 32)
		// Create DegradedState with enteredAt = now
		stateObj := state.NewDegradedState()

		snap := snapshot.CommunicatorSnapshot{
			Desired: snapshot.CommunicatorDesiredState{},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 5,
			},
		}

		// Act: Call Next() immediately (within backoff)
		nextState, signal, action := stateObj.Next(snap)

		// Assert: Still in degraded, no action (waiting for backoff)
		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(signal).To(Equal(fsmv2.SignalNone))
		Expect(action).To(BeNil(), "Should NOT emit action during backoff period")
	})

	It("attempts sync after backoff period expires", func() {
		// Arrange: Create state that entered 40s ago (backoff expired for 5 errors)
		// 5 errors = 32s backoff, so 40s should be past the backoff
		stateObj := state.NewDegradedStateWithEnteredAt(time.Now().Add(-40 * time.Second))

		snap := snapshot.CommunicatorSnapshot{
			Desired: snapshot.CommunicatorDesiredState{},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 5,
			},
		}

		// Act: Call Next()
		nextState, _, action := stateObj.Next(snap)

		// Assert: Action returned (attempting sync)
		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(action).NotTo(BeNil(), "Should emit SyncAction after backoff expires")
		Expect(action.Name()).To(Equal("sync"))
	})
})
