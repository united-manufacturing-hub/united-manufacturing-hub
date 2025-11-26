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

package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// Test state types that embed BaseState and implement String() using StateNameFromType

type RunningState struct {
	helpers.BaseState
}

func (s RunningState) String() string {
	return s.StateNameFromType(s)
}

type TryingToStartState struct {
	helpers.BaseState
}

func (s TryingToStartState) String() string {
	return s.StateNameFromType(s)
}

type StoppedState struct {
	helpers.BaseState
}

func (s StoppedState) String() string {
	return s.StateNameFromType(s)
}

type ConnectedState struct {
	helpers.BaseState
}

func (s ConnectedState) String() string {
	return s.StateNameFromType(s)
}

type TryingToConnectState struct {
	helpers.BaseState
}

func (s TryingToConnectState) String() string {
	return s.StateNameFromType(s)
}

type DegradedState struct {
	helpers.BaseState
}

func (s DegradedState) String() string {
	return s.StateNameFromType(s)
}

type InitializingState struct {
	helpers.BaseState
}

func (s InitializingState) String() string {
	return s.StateNameFromType(s)
}

// StateWithFields is a test state type with additional fields.
type StateWithFields struct {
	helpers.BaseState
	SomeField string
	Counter   int
}

func (s StateWithFields) String() string {
	return s.StateNameFromType(s)
}

var _ = Describe("BaseState", func() {
	Describe("String() method", func() {
		Context("with standard state names ending in 'State'", func() {
			It("should derive 'Running' from 'RunningState'", func() {
				state := RunningState{}
				Expect(state.String()).To(Equal("Running"))
			})

			It("should derive 'TryingToStart' from 'TryingToStartState'", func() {
				state := TryingToStartState{}
				Expect(state.String()).To(Equal("TryingToStart"))
			})

			It("should derive 'Stopped' from 'StoppedState'", func() {
				state := StoppedState{}
				Expect(state.String()).To(Equal("Stopped"))
			})

			It("should derive 'Connected' from 'ConnectedState'", func() {
				state := ConnectedState{}
				Expect(state.String()).To(Equal("Connected"))
			})

			It("should derive 'TryingToConnect' from 'TryingToConnectState'", func() {
				state := TryingToConnectState{}
				Expect(state.String()).To(Equal("TryingToConnect"))
			})

			It("should derive 'Degraded' from 'DegradedState'", func() {
				state := DegradedState{}
				Expect(state.String()).To(Equal("Degraded"))
			})

			It("should derive 'Initializing' from 'InitializingState'", func() {
				state := InitializingState{}
				Expect(state.String()).To(Equal("Initializing"))
			})
		})

		Context("with pointer receivers", func() {
			It("should work with pointer to RunningState", func() {
				state := &RunningState{}
				Expect(state.String()).To(Equal("Running"))
			})

			It("should work with pointer to TryingToStartState", func() {
				state := &TryingToStartState{}
				Expect(state.String()).To(Equal("TryingToStart"))
			})

			It("should work with pointer to StoppedState", func() {
				state := &StoppedState{}
				Expect(state.String()).To(Equal("Stopped"))
			})

			It("should work with pointer to ConnectedState", func() {
				state := &ConnectedState{}
				Expect(state.String()).To(Equal("Connected"))
			})
		})

		Context("with structs containing additional fields", func() {
			It("should derive name without State suffix", func() {
				state := StateWithFields{
					SomeField: "test",
					Counter:   42,
				}
				// StateWithFields doesn't end with "State", so full name is returned
				Expect(state.String()).To(Equal("StateWithFields"))
			})

			It("should work with pointer to struct with additional fields", func() {
				state := &StateWithFields{
					SomeField: "test",
					Counter:   42,
				}
				Expect(state.String()).To(Equal("StateWithFields"))
			})
		})

		Context("edge cases", func() {
			It("should handle direct BaseState usage", func() {
				state := helpers.BaseState{}
				Expect(state.String()).To(Equal("BaseState"))
			})

			It("should handle pointer to BaseState", func() {
				state := &helpers.BaseState{}
				Expect(state.String()).To(Equal("BaseState"))
			})
		})
	})

	Describe("Reason() method", func() {
		It("should return empty string by default", func() {
			state := RunningState{}
			Expect(state.Reason()).To(Equal(""))
		})

		It("should return empty string for pointer receiver", func() {
			state := &TryingToStartState{}
			Expect(state.Reason()).To(Equal(""))
		})

		It("should return empty string for BaseState directly", func() {
			state := helpers.BaseState{}
			Expect(state.Reason()).To(Equal(""))
		})
	})

	Describe("DeriveStateName function", func() {
		It("should derive state name from value type", func() {
			state := RunningState{}
			Expect(helpers.DeriveStateName(state)).To(Equal("Running"))
		})

		It("should derive state name from pointer type", func() {
			state := &TryingToStartState{}
			Expect(helpers.DeriveStateName(state)).To(Equal("TryingToStart"))
		})

		It("should work with BaseState directly", func() {
			state := helpers.BaseState{}
			Expect(helpers.DeriveStateName(state)).To(Equal("Base"))
		})

		It("should work with pointer to BaseState", func() {
			state := &helpers.BaseState{}
			Expect(helpers.DeriveStateName(state)).To(Equal("Base"))
		})

		It("should return full name if no State suffix", func() {
			type MyCustomType struct{}
			state := MyCustomType{}
			Expect(helpers.DeriveStateName(state)).To(Equal("MyCustomType"))
		})
	})
})
