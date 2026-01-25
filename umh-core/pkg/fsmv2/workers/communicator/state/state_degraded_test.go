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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
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
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
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
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
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
	Context("for network errors", func() {
		It("should emit ResetTransportAction at exactly 5 consecutive errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 5,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-35 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, act := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("reset_transport"))
		})

		It("should NOT emit ResetTransportAction at 6 consecutive errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 6,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("sync"))
		})

		It("should emit ResetTransportAction again at 10 consecutive errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 10,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("reset_transport"))
		})

		It("should NOT emit ResetTransportAction at 0 errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 0,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-1 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("sync"))
		})
	})

	Context("for server errors", func() {
		It("should NOT emit ResetTransportAction at 5 errors (needs 10)", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 5,
					LastErrorType:     httpTransport.ErrorTypeServerError,
					DegradedEnteredAt: time.Now().Add(-35 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("sync"))
		})

		It("should emit ResetTransportAction at 10 consecutive errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 10,
					LastErrorType:     httpTransport.ErrorTypeServerError,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("reset_transport"))
		})

		It("should emit ResetTransportAction at 20 consecutive errors", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 20,
					LastErrorType:     httpTransport.ErrorTypeServerError,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("reset_transport"))
		})
	})

	Context("for unknown/other error types", func() {
		It("should NOT emit ResetTransportAction regardless of error count", func() {
			stateObj := &state.DegradedState{}

			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 100,
					LastErrorType:     httpTransport.ErrorTypeBackendRateLimit,
					DegradedEnteredAt: time.Now().Add(-600 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			_, _, act := stateObj.Next(snap)

			Expect(act).NotTo(BeNil())
			Expect(act.Name()).To(Equal("sync"))
		})
	})
})

var _ = Describe("DegradedState Auth Transition", func() {
	It("should transition to TryingToAuthenticateState when last error is InvalidToken", func() {
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				LastErrorType:     httpTransport.ErrorTypeInvalidToken,
				ConsecutiveErrors: 1,
				DegradedEnteredAt: time.Now().Add(-65 * time.Second), // Past backoff
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		nextState, signal, action := stateObj.Next(snap)

		Expect(nextState).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
		Expect(signal).To(Equal(fsmv2.SignalNone))
		Expect(action).To(BeNil())
	})
})

var _ = Describe("DegradedState Backoff", func() {
	It("stays in degraded state during backoff period", func() {
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 3,
				DegradedEnteredAt: time.Now(), // Just entered degraded
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		nextState, signal, action := stateObj.Next(snap)

		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(signal).To(Equal(fsmv2.SignalNone))
		Expect(action).To(BeNil(), "Should NOT emit action during backoff period")
	})

	It("attempts sync after backoff period expires", func() {
		stateObj := &state.DegradedState{}

		snap := fsmv2.Snapshot{
			Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
			Observed: snapshot.CommunicatorObservedState{
				Authenticated:     false,
				ConsecutiveErrors: 3,
				DegradedEnteredAt: time.Now().Add(-10 * time.Second), // Past backoff period
			},
			Desired: &snapshot.CommunicatorDesiredState{},
		}

		nextState, _, action := stateObj.Next(snap)

		Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(action).NotTo(BeNil(), "Should emit SyncAction after backoff expires")
		Expect(action.Name()).To(Equal("sync"))
	})
})

var _ = Describe("DegradedState Transitions", func() {
	var stateObj *state.DegradedState

	BeforeEach(func() {
		stateObj = &state.DegradedState{}
	})

	Describe("Degraded -> StoppedState", func() {
		It("should transition to StoppedState when shutdown is requested", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					ConsecutiveErrors: 10,
					DegradedEnteredAt: time.Now().Add(-5 * time.Minute),
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should prioritize shutdown over recovery", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					ConsecutiveErrors: 0,
					DegradedEnteredAt: time.Time{},
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})
	})

	Describe("Degraded -> TryingToAuthenticateState", func() {
		It("should transition to TryingToAuthenticateState when last error is InvalidToken", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					LastErrorType:     httpTransport.ErrorTypeInvalidToken,
					ConsecutiveErrors: 1,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should transition to TryingToAuthenticateState on InvalidToken regardless of consecutive errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					LastErrorType:     httpTransport.ErrorTypeInvalidToken,
					ConsecutiveErrors: 100,
					DegradedEnteredAt: time.Now().Add(-5 * time.Minute),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.TryingToAuthenticateState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})
	})

	Describe("Degraded -> SyncingState", func() {
		It("should transition to SyncingState when sync is healthy and no errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 0,
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should transition to SyncingState after full recovery", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTToken:          "valid-token",
					JWTExpiry:         time.Now().Add(30 * time.Minute),
					ConsecutiveErrors: 0,
					DegradedEnteredAt: time.Time{},
				},
				Desired: &snapshot.CommunicatorDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
				},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should NOT transition to SyncingState if sync is healthy but has errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: 3,
					DegradedEnteredAt: time.Now().Add(-20 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, _, _ := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
		})
	})

	Describe("Degraded -> self (backoff/retry)", func() {
		It("should stay in DegradedState during backoff period", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 3,
					DegradedEnteredAt: time.Now(),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should stay in DegradedState and emit SyncAction after backoff", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 3,
					DegradedEnteredAt: time.Now().Add(-10 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).NotTo(BeNil())
			Expect(action.Name()).To(Equal("sync"))
		})

		It("should emit ResetTransportAction at 5 consecutive network errors after backoff", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 5,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-35 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).NotTo(BeNil())
			Expect(action.Name()).To(Equal("reset_transport"))
		})

		It("should emit ResetTransportAction at 10 consecutive network errors", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 10,
					LastErrorType:     httpTransport.ErrorTypeNetwork,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).NotTo(BeNil())
			Expect(action.Name()).To(Equal("reset_transport"))
		})

		It("should emit SyncAction at 6 consecutive errors (not a reset threshold)", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     false,
					ConsecutiveErrors: 6,
					DegradedEnteredAt: time.Now().Add(-65 * time.Second),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).NotTo(BeNil())
			Expect(action.Name()).To(Equal("sync"))
		})
	})
})
