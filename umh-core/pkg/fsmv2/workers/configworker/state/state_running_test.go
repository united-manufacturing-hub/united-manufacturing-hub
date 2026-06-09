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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

func runningSnapshot(base config.BaseDesiredState) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "configworker"},
		Observed: fsmv2.Observation[snapshot.ConfigworkerStatus]{
			Status: snapshot.ConfigworkerStatus{},
		},
		Desired: &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{
			BaseDesiredState: base,
		},
	}
}

var _ = Describe("RunningState", func() {
	var stateObj *state.RunningState

	BeforeEach(func() {
		stateObj = &state.RunningState{}
	})

	Describe("String", func() {
		It("returns Running", func() {
			Expect(stateObj.String()).To(Equal("Running"))
		})
	})

	Describe("Next", func() {
		Context("when neither shutdown nor disabled", func() {
			It("stays in RunningState with no signal and no action", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when shutdown is requested", func() {
			It("transitions to StoppedState without yet signaling removal", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{ShutdownRequested: true}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})

		Context("when disabled", func() {
			It("transitions to StoppedState without yet signaling removal", func() {
				result := stateObj.Next(runningSnapshot(config.BaseDesiredState{Disabled: true}))

				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
				Expect(result.Signal).To(Equal(fsmv2.SignalNone))
				Expect(result.Action).To(BeNil())
			})
		})
	})
})
