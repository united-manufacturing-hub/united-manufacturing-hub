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

var _ = Describe("DegradedState", func() {
	var (
		stateObj *state.DegradedState
		snap     fsmv2.Snapshot
	)

	BeforeEach(func() {
		stateObj = &state.DegradedState{}
	})

	Describe("Next", func() {
		Context("when mood is still sad", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{Mood: "sad", HelloSaid: true},
					Desired:  &snapshot.HelloworldDesiredState{},
				}
			})

			It("should stay in DegradedState", func() {
				result := stateObj.Next(snap)

				Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
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

		Context("when mood has recovered", func() {
			BeforeEach(func() {
				snap = fsmv2.Snapshot{
					Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
					Observed: snapshot.HelloworldObservedState{Mood: "happy", HelloSaid: true},
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
					Observed: snapshot.HelloworldObservedState{Mood: "sad", HelloSaid: true},
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
	})

	Describe("String", func() {
		It("should return the state name", func() {
			Expect(stateObj.String()).To(Equal("Degraded"))
		})
	})
})

var _ = Describe("DegradedState Transitions", func() {
	var stateObj *state.DegradedState

	BeforeEach(func() {
		stateObj = &state.DegradedState{}
	})

	Describe("Degraded -> RunningState", func() {
		It("should transition when mood is no longer sad", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{Mood: "happy", HelloSaid: true},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should transition when mood file is absent (empty mood)", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{Mood: "", HelloSaid: true},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("Degraded -> StoppedState", func() {
		It("should transition on shutdown request", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{Mood: "sad", HelloSaid: true},
				Desired: &snapshot.HelloworldDesiredState{
					BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
				},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})

		It("should shutdown even if mood has recovered", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{Mood: "happy", HelloSaid: true},
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

	Describe("Degraded -> Stay", func() {
		It("should stay degraded when mood is still sad", func() {
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "helloworld"},
				Observed: snapshot.HelloworldObservedState{Mood: "sad", HelloSaid: true},
				Desired:  &snapshot.HelloworldDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.Action).To(BeNil())
		})
	})
})
