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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("TryingToStartState", func() {
	var (
		stateObj *state.TryingToStartState
		snap     fsmv2.Snapshot
	)

	BeforeEach(func() {
		stateObj = &state.TryingToStartState{}
	})

	Describe("Next", func() {
		Context("when HelloSaid is false", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: false},
					Desired:  &snapshot.HelloworldDesiredState{},
				}
			})

			It("should stay in TryingToStartState", func() {
				result := stateObj.Next(snap)

				Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStartState{}))
			})

			It("should not signal anything", func() {
				result := stateObj.Next(snap)

				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			})

			It("should emit SayHelloAction", func() {
				result := stateObj.Next(snap)

				Expect(result.Action).To(BeAssignableToTypeOf(&action.SayHelloAction{}))
			})
		})

		Context("when HelloSaid is true", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: true},
					Desired:  &snapshot.HelloworldDesiredState{},
				}
			})

			It("should transition to RunningState", func() {
				result := stateObj.Next(snap)

				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			})

			It("should not signal anything", func() {
				result := stateObj.Next(snap)

				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				result := stateObj.Next(snap)

				Expect(result.Action).To(BeNil())
			})
		})

		Context("when shutdown is requested", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: false},
					Desired: &snapshot.HelloworldDesiredState{
						BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
					},
				}
			})

			It("should transition to StoppedState", func() {
				result := stateObj.Next(snap)

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			})

			It("should not signal anything", func() {
				result := stateObj.Next(snap)

				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				result := stateObj.Next(snap)

				Expect(result.Action).To(BeNil())
			})
		})

		Context("shutdown takes priority over HelloSaid", func() {
			It("should shutdown even if HelloSaid is true", func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{HelloSaid: true},
					Desired: &snapshot.HelloworldDesiredState{
						BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
					},
				}

				result := stateObj.Next(snap)

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})

	Describe("String", func() {
		It("should return the state name", func() {
			Expect(stateObj.String()).To(Equal("TryingToStart"))
		})
	})
})

var _ = Describe("TryingToStartState Transitions", func() {
	var stateObj *state.TryingToStartState

	BeforeEach(func() {
		stateObj = &state.TryingToStartState{}
	})

	Describe("TryingToStart -> RunningState", func() {
		It("should transition when HelloSaid is observed", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: true},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("TryingToStart -> StoppedState", func() {
		It("should transition on shutdown request", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: false},
				Desired: &snapshot.HelloworldDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("TryingToStart -> Stay (emit action)", func() {
		It("should emit SayHelloAction while waiting", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{HelloSaid: false},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStartState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeAssignableToTypeOf(&action.SayHelloAction{}))
		})
	})
})
