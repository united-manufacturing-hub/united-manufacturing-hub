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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

var _ = Describe("StoppedState", func() {
	var stateObj *state.StoppedState

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	Describe("String", func() {
		It("returns Stopped", func() {
			Expect(stateObj.String()).To(Equal("Stopped"))
		})
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			It("signals removal so the supervisor can reap the worker", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{ShutdownRequested: true}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when disabled without shutdown", func() {
			It("stays resident in StoppedState so it keeps its registry handle", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{Disabled: true}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when shutdown is requested while also disabled", func() {
			It("signals removal because shutdown takes precedence", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{ShutdownRequested: true, Disabled: true}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when neither shutdown nor disabled", func() {
			It("resumes back to RunningState", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})
})
