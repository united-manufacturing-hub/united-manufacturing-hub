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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
)

var _ = Describe("TryingToAuthenticateState", func() {
	var stateObj *state.TryingToAuthenticateState

	BeforeEach(func() {
		stateObj = &state.TryingToAuthenticateState{}
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(stateObj.String()).To(Equal("TryingToAuthenticate"))
		})
	})
})

var _ = Describe("TryingToAuthenticateState Transitions", func() {
	var stateObj *state.TryingToAuthenticateState

	BeforeEach(func() {
		stateObj = &state.TryingToAuthenticateState{}
	})

	Describe("TryingToAuth -> StoppedState", func() {
		It("should transition to StoppedState when shutdown is requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to StoppedState on shutdown even if authenticated", func() {
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
	})

	Describe("TryingToAuth -> SyncingState", func() {
		It("should transition to SyncingState when authenticated with valid token", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "valid-jwt-token",
					JWTExpiry:     time.Now().Add(time.Hour),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition to SyncingState when authenticated and token not expired", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "comm-auth", Name: "communicator", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "jwt-token-123",
					JWTExpiry:     time.Now().Add(30 * time.Minute),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("TryingToAuth -> self (emits AuthenticateAction)", func() {
		It("should stay in TryingToAuthenticateState and emit AuthenticateAction when not authenticated", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: false,
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})

		It("should stay in TryingToAuthenticateState when authenticated but token is expired", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "expired-token",
					JWTExpiry:     time.Now().Add(-time.Hour),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})

		It("should stay in TryingToAuthenticateState when token expires within 10 minutes", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated: true,
					JWTToken:      "expiring-soon-token",
					JWTExpiry:     time.Now().Add(5 * time.Minute),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})
	})

	Describe("TryingToAuth -> self (backoff)", func() {
		It("should stay in TryingToAuthenticateState without action during backoff period", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 3,
					LastAuthAttemptAt: time.Now(),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil(), "Should not emit action during backoff period")
		})

		It("should emit AuthenticateAction after backoff period expires", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 3,
					LastAuthAttemptAt: time.Now().Add(-10 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})

		It("should emit AuthenticateAction on first attempt (no errors)", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 0,
				},
				Desired: &snapshot.CommunicatorDesiredState{
					RelayURL:     "https://relay.example.com",
					InstanceUUID: "instance-uuid-123",
					AuthToken:    "auth-token-abc",
					Timeout:      30 * time.Second,
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})
	})
})
