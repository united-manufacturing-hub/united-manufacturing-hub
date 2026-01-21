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
		stateObj = &state.DegradedState{}
	})

	Describe("Next", func() {
		Context("when sync recovers", func() {
			var snap fsmv2.Snapshot

			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: true,
					},
					Desired: &snapshot.CommunicatorDesiredState{},
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
			var snap fsmv2.Snapshot

			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
					Observed: snapshot.CommunicatorObservedState{
						Authenticated: false,
					},
					Desired: &snapshot.CommunicatorDesiredState{},
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

var _ = Describe("DegradedState Transport Reset", func() {
	// TransportResetThreshold is 5 - reset at 5, 10, 15... errors
	// Backoff: 5 errors = 32s, 6+ errors = 60s (capped)
	It("should emit ResetTransportAction at exactly 5 consecutive errors", func() {
		// Arrange: 5 errors = 32s backoff, DegradedEnteredAt was 35s ago (past backoff)
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 5,
				DegradedEnteredAt: time.Now().Add(-35 * time.Second),
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act
		nextState, signal, act := stateObj.Next(snap)

		// Assert
		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(signal).To(Equal(fsmv2.SignalNone))
		Expect(act).NotTo(BeNil())
		Expect(act.Name()).To(Equal("reset_transport"))
	})

	It("should NOT emit ResetTransportAction at 6 consecutive errors", func() {
		// Arrange: 6 errors = 60s backoff (capped), DegradedEnteredAt was 65s ago (past backoff)
		// 6 % 5 != 0, so should emit SyncAction, not ResetTransportAction
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 6,
				DegradedEnteredAt: time.Now().Add(-65 * time.Second),
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act
		_, _, act := stateObj.Next(snap)

		// Assert: Should emit SyncAction, not ResetTransportAction
		Expect(act).NotTo(BeNil())
		Expect(act.Name()).To(Equal("sync"))
	})

	It("should emit ResetTransportAction again at 10 consecutive errors", func() {
		// Arrange: 10 errors = 60s backoff (capped), DegradedEnteredAt was 65s ago (past backoff)
		// 10 % 5 == 0, so should emit ResetTransportAction
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 10,
				DegradedEnteredAt: time.Now().Add(-65 * time.Second),
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act
		_, _, act := stateObj.Next(snap)

		// Assert
		Expect(act).NotTo(BeNil())
		Expect(act.Name()).To(Equal("reset_transport"))
	})

	It("should NOT emit ResetTransportAction at 0 errors", func() {
		// 0 errors = 0s backoff, DegradedEnteredAt was 1s ago (past backoff)
		// 0 % 5 == 0 but we shouldn't reset when there are no errors
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 0,
				DegradedEnteredAt: time.Now().Add(-1 * time.Second),
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act
		_, _, act := stateObj.Next(snap)

		// Assert: Should emit SyncAction, not ResetTransportAction
		Expect(act).NotTo(BeNil())
		Expect(act.Name()).To(Equal("sync"))
	})
})

var _ = Describe("DegradedState Backoff", func() {
	It("stays in degraded state during backoff period", func() {
		// Arrange: 3 consecutive errors = 8s backoff (2^3 = 8)
		// DegradedEnteredAt set to now (within backoff period)
		// Using 3 errors to avoid the reset threshold (5)
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 3,
				DegradedEnteredAt: time.Now(), // Just entered degraded
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act: Call Next() immediately (within backoff)
		nextState, signal, action := stateObj.Next(snap)

		// Assert: Still in degraded, no action (waiting for backoff)
		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(signal).To(Equal(fsmv2.SignalNone))
		Expect(action).To(BeNil(), "Should NOT emit action during backoff period")
	})

	It("attempts sync after backoff period expires", func() {
		// Arrange: 3 errors = 8s backoff, DegradedEnteredAt was 10s ago (past backoff)
		// Using 3 errors to avoid the reset threshold (5)
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: fsmv2.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 3,
				DegradedEnteredAt: time.Now().Add(-10 * time.Second), // Past backoff period
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		// Act: Call Next()
		nextState, _, action := stateObj.Next(snap)

		// Assert: Action returned (attempting sync)
		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(action).NotTo(BeNil(), "Should emit SyncAction after backoff expires")
		Expect(action.Name()).To(Equal("sync"))
	})
})
