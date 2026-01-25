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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("RunningState", func() {
	var (
		stateObj *state.RunningState
		snap     fsmv2.Snapshot
	)

	BeforeEach(func() {
		stateObj = &state.RunningState{}
	})

	Describe("Next", func() {
		Context("when running normally", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: true},
					Desired:  &snapshot.HelloworldDesiredState{},
				}
			})

			It("should stay in RunningState", func() {
				nextState, _, _ := stateObj.Next(snap)

				Expect(nextState).To(BeAssignableToTypeOf(&state.RunningState{}))
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

		Context("when shutdown is requested", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: true},
					Desired: &snapshot.HelloworldDesiredState{
						BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
					},
				}
			})

			It("should transition to StoppedState", func() {
				nextState, _, _ := stateObj.Next(snap)

				Expect(nextState).To(BeAssignableToTypeOf(&state.StoppedState{}))
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
		It("should return snake_case state name", func() {
			Expect(stateObj.String()).To(Equal("Running"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(stateObj.Reason()).To(Equal("Worker is running and has said hello"))
		})
	})
})

var _ = Describe("RunningState Transitions", func() {
	var stateObj *state.RunningState

	BeforeEach(func() {
		stateObj = &state.RunningState{}
	})

	Describe("Running -> StoppedState", func() {
		It("should transition on shutdown request", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: true},
				Desired: &snapshot.HelloworldDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})
	})

	Describe("Running -> Stay", func() {
		It("should stay running with default desired state", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: true},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should stay running with explicit shutdown=false", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "hello-running", Name: "helloworld", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: true},
				Desired: &snapshot.HelloworldDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: false},
				},
			}

			nextState, signal, action := stateObj.Next(snap)

			Expect(nextState).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})

		It("should stay running even if HelloSaid becomes false", func() {
			// This is an edge case - HelloSaid shouldn't normally become false,
			// but the state machine should handle it gracefully
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: false},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			nextState, signal, action := stateObj.Next(snap)

			// Running state stays running - it doesn't check HelloSaid
			// Only cares about shutdown
			Expect(nextState).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(signal).To(Equal(fsmv2.SignalNone))
			Expect(action).To(BeNil())
		})
	})
})
