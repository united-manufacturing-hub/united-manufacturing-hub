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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow/state"
)

func TestExampleflowState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exampleflow State Suite")
}

var _ = Describe("Exampleflow states", func() {
	newSnapshot := func(started bool) fsmv2.Snapshot {
		return fsmv2.Snapshot{
			Observed: fsmv2.Observation[exampleflow.ExampleflowStatus]{
				Status: exampleflow.ExampleflowStatus{Started: started},
			},
			Desired: &fsmv2.WrappedDesiredState[exampleflow.ExampleflowConfig]{
				Config: exampleflow.ExampleflowConfig{
					Flow: `{{ .a }}`,
				},
				BaseDesiredState: config.BaseDesiredState{},
			},
		}
	}

	It("registers StoppedState as the initial state", func() {
		initial := fsmv2.LookupInitialState(exampleflow.WorkerTypeName)
		Expect(initial).NotTo(BeNil())
		Expect(initial).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("emits the start action and stays in TryingToStart while the start has not run", func() {
		trying := &state.TryingToStartState{}

		result := trying.Next(newSnapshot(false))

		Expect(result.State).To(Equal(trying))
		Expect(result.Action).NotTo(BeNil())
	})

	It("transitions to Running once the start action ran", func() {
		trying := &state.TryingToStartState{}

		result := trying.Next(newSnapshot(true))

		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
	})
})
