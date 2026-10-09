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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge/state"
)

func TestExamplebridgeState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Bridge State Suite")
}

var _ = Describe("Examplebridge states", func() {
	newSnapshot := func(childrenHealthy, childrenUnhealthy int, shutdown bool) fsmv2.Snapshot {
		return fsmv2.Snapshot{
			Identity: deps.Identity{
				ID:         "bridge-001",
				Name:       "b1",
				WorkerType: examplebridge.WorkerTypeName,
			},
			Observed: fsmv2.Observation[examplebridge.ExamplebridgeStatus]{
				ChildrenHealthy:   childrenHealthy,
				ChildrenUnhealthy: childrenUnhealthy,
				ShutdownRequested: shutdown,
			},
			Desired: &fsmv2.WrappedDesiredState[examplebridge.ExamplebridgeConfig]{
				Config: examplebridge.ExamplebridgeConfig{
					Read:  `{{ .connection.host }}`,
					Write: `{{ .IP }}`,
				},
				BaseDesiredState: config.BaseDesiredState{
					ShutdownRequested: shutdown,
				},
			},
		}
	}

	It("names each flow child after its bridge, so two bridges' children stay distinct", func() {
		running := &state.RunningState{}

		b1 := newSnapshot(2, 0, false)
		b1Result := running.Next(b1)

		b2 := newSnapshot(2, 0, false)
		b2.Identity.Name = "b2"
		b2Result := running.Next(b2)

		namesOf := func(result fsmv2.NextResult[any, any]) []string {
			names := make([]string, 0, len(result.Children))
			for _, spec := range result.Children {
				names = append(names, spec.Name)
			}

			return names
		}

		Expect(namesOf(b1Result)).To(Equal([]string{"b1-read", "b1-write"}))
		Expect(namesOf(b2Result)).To(Equal([]string{"b2-read", "b2-write"}))
	})

	It("registers StoppedState as the initial state", func() {
		initial := fsmv2.LookupInitialState(examplebridge.WorkerTypeName)
		Expect(initial).NotTo(BeNil())
		Expect(initial).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("spawns the flows from RunningState", func() {
		running := &state.RunningState{}
		snap := newSnapshot(2, 0, false)

		result := running.Next(snap)

		Expect(result.State).To(Equal(running))

		children := result.Children
		Expect(children).To(HaveLen(2))

		byName := map[string]bool{}
		for _, spec := range children {
			byName[spec.Name] = true
			Expect(spec.WorkerType).To(Equal("exampleflow"))
			Expect(spec.Enabled).To(BeTrue())
		}
		Expect(byName).To(HaveKey("b1-read"))
		Expect(byName).To(HaveKey("b1-write"))
	})

	It("transitions to TryingToStop on shutdown with no children", func() {
		running := &state.RunningState{}
		snap := newSnapshot(2, 0, true)

		result := running.Next(snap)

		Expect(result.State).To(BeAssignableToTypeOf(&state.TryingToStopState{}))
		Expect(result.Children).To(BeEmpty())
	})

	It("stops once every child is gone", func() {
		trying := &state.TryingToStopState{}
		snap := newSnapshot(0, 0, true)

		result := trying.Next(snap)

		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		Expect(result.Children).To(BeEmpty())
	})

	It("waits while children remain", func() {
		trying := &state.TryingToStopState{}
		snap := newSnapshot(1, 0, true)

		result := trying.Next(snap)

		Expect(result.State).To(Equal(trying))
		Expect(result.Children).To(BeEmpty())
	})
})
