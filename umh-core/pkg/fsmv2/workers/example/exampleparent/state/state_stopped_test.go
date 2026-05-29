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
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

var _ = Describe("StoppedState IsDisabled discriminator", func() {
	var stateObj *state.StoppedState

	BeforeEach(func() {
		stateObj = &state.StoppedState{}
	})

	It("disabled-only (IsDisabled=true, IsShutdownRequested=false) → SignalNone, stays Stopped", func() {
		snap := fsmv2.Snapshot{
			Observed: fsmv2.Observation[exampleparent.ExampleparentStatus]{},
			Desired: &fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
				BaseDesiredState: config.BaseDesiredState{Disabled: true, ShutdownRequested: false},
			},
		}
		result := stateObj.Next(snap)
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("disabled+shutdown (both true) → SignalNeedsRemoval (shutdown wins)", func() {
		snap := fsmv2.Snapshot{
			Observed: fsmv2.Observation[exampleparent.ExampleparentStatus]{},
			Desired: &fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
				BaseDesiredState: config.BaseDesiredState{Disabled: true, ShutdownRequested: true},
			},
		}
		result := stateObj.Next(snap)
		Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})
})
