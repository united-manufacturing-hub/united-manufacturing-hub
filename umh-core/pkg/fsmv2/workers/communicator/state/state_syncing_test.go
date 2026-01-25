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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

func TestSyncingState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SyncingState Suite")
}

var _ = Describe("SyncingState", func() {
	var stateObj *state.SyncingState

	BeforeEach(func() {
		stateObj = &state.SyncingState{}
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("Syncing"))
		})
	})
})

var _ = Describe("SyncingState Circuit Breaker", func() {
	It("applies backoff delay based on ConsecutiveErrors", func() {
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 5,
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}
		delay := syncingState.GetBackoffDelay(observed)

		// 5 errors = 2^5 = 32 seconds (exponential backoff capped at 60s)
		Expect(delay).To(BeNumerically(">=", 10*time.Second))
		Expect(delay).To(BeNumerically("<=", 60*time.Second))
	})

	It("returns zero delay when no errors", func() {
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 0,
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}
		delay := syncingState.GetBackoffDelay(observed)

		Expect(delay).To(Equal(time.Duration(0)))
	})

	It("caps backoff at 60 seconds for many errors", func() {
		observed := snapshot.CommunicatorObservedState{
			ConsecutiveErrors: 100, // Many errors
			Authenticated:     true,
		}

		syncingState := &state.SyncingState{}
		delay := syncingState.GetBackoffDelay(observed)

		// 2^100 seconds would be huge, but should be capped at 60s
		Expect(delay).To(Equal(60 * time.Second))
	})
})

var _ = Describe("SyncingState Transitions", func() {
	var stateObj *state.SyncingState

	BeforeEach(func() {
		stateObj = &state.SyncingState{}
	})

	Describe("Syncing -> StoppedState", func() {
		It("should transition to StoppedState when shutdown is requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "valid-token",
					JWTExpiry:     time.Now().Add(time.Hour),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should prioritize shutdown over other conditions", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 10,
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Syncing -> TryingToAuthenticateState", func() {
		It("should transition to TryingToAuthenticateState when token is expired", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "expired-token",
					JWTExpiry:     time.Now().Add(-time.Hour),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to TryingToAuthenticateState when token expires within 10 minutes", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "expiring-token",
					JWTExpiry:     time.Now().Add(5 * time.Minute),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to TryingToAuthenticateState when not authenticated", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: false,
					JWTToken:      "",
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to TryingToAuthenticateState when authenticated but token empty", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: false,
					JWTToken:      "some-token",
					JWTExpiry:     time.Now().Add(time.Hour),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Syncing -> DegradedState", func() {
		It("should transition to DegradedState when sync is not healthy", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 5,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to DegradedState at exactly 5 consecutive errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 5,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to DegradedState with high consecutive errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 100,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Syncing -> self (continuous sync)", func() {
		It("should stay in SyncingState and emit SyncAction when healthy", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 0,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("sync"))
		})

		It("should stay in SyncingState with 4 consecutive errors (below threshold)", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 4,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("sync"))
		})

		It("should stay in SyncingState and emit SyncAction after successful recovery", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(30 * time.Minute),
					ConsecutiveErrors: 0,
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("sync"))
		})
	})
})
