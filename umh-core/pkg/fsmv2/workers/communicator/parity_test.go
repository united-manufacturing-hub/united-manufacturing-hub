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

package communicator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
)

// Differential parity (P2.6 / pr2_issues #12 expansion) — anchors that the
// legacy SetChildSpecsFactory-driven DDS path and the canonical
// communicator.RenderChildren emitter produce IDENTICAL ChildSpec slices for
// the communicator worker during the migration window. Equality is asserted
// by ChildSpec.Hash() — see the transport-side parity test for the full
// rationale.
//
// The communicator emits exactly one transport child. RenderChildren reads
// the transport child's UserSpec from snap.Desired.ChildrenSpecs[0]; we
// construct that snapshot from the legacy DDS output so both paths observe
// the same input.
//
// This test ships at P2.6 ship time and is retired in P3.x once the legacy
// SetChildSpecsFactory path is deleted.
var _ = Describe("Communicator — DDS vs RenderChildren differential parity (P2.6)", func() {
	BeforeEach(func() {
		communicator.SetChannelProvider(NewMockChannelProvider())
	})

	AfterEach(func() {
		communicator.ClearChannelProvider()
	})

	runParity := func(rawSpec config.UserSpec, label string) {
		identity := depspkg.Identity{ID: "parity-communicator-" + label, Name: "Parity Communicator " + label}
		logger := depspkg.NewNopFSMLogger()

		w := createCommunicatorWorker(identity.ID, logger, nil)

		desired, err := w.DeriveDesiredState(rawSpec)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue(),
			"communicator DesiredState must implement ChildSpecProvider during migration window")

		legacy := provider.GetChildrenSpecs()

		snap := fsmv2.WorkerSnapshot[communicator.CommunicatorConfig, communicator.CommunicatorStatus]{
			Desired: fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{
				BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
				ChildrenSpecs:    legacy,
			},
		}
		rendered := communicator.RenderChildren(snap)

		Expect(rendered).To(HaveLen(len(legacy)),
			"%s parity: DDS=%d rendered=%d", label, len(legacy), len(rendered))

		for i := range legacy {
			lhash, lerr := legacy[i].Hash()
			Expect(lerr).NotTo(HaveOccurred(), "hashing legacy[%d] for %s", i, label)
			rhash, rerr := rendered[i].Hash()
			Expect(rerr).NotTo(HaveOccurred(), "hashing rendered[%d] for %s", i, label)

			Expect(rhash).To(Equal(lhash),
				"%s parity violation on spec[%d]:\n"+
					"  legacy   = name=%q workerType=%q startStates=%v enabled=%v userSpec.Config=%q\n"+
					"  rendered = name=%q workerType=%q startStates=%v enabled=%v userSpec.Config=%q",
				label, i,
				legacy[i].Name, legacy[i].WorkerType, legacy[i].ChildStartStates,
				legacy[i].Enabled, legacy[i].UserSpec.Config,
				rendered[i].Name, rendered[i].WorkerType, rendered[i].ChildStartStates,
				rendered[i].Enabled, rendered[i].UserSpec.Config)
		}
	}

	It("Hash-equality on the steady-state running spec", func() {
		rawSpec := config.UserSpec{
			Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid-comm"
authToken: "test-token-comm"`,
			Variables: config.VariableBundle{},
		}
		runParity(rawSpec, "steady-state")
	})

	It("Hash-equality on the nil-spec startup path", func() {
		identity := depspkg.Identity{ID: "parity-communicator-nil", Name: "Parity Communicator Nil"}
		logger := depspkg.NewNopFSMLogger()

		w := createCommunicatorWorker(identity.ID, logger, nil)

		desired, err := w.DeriveDesiredState(nil)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue())

		legacy := provider.GetChildrenSpecs()

		snap := fsmv2.WorkerSnapshot[communicator.CommunicatorConfig, communicator.CommunicatorStatus]{
			Desired: fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{
				BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
				ChildrenSpecs:    legacy,
			},
		}
		rendered := communicator.RenderChildren(snap)

		Expect(rendered).To(HaveLen(len(legacy)),
			"nil-spec startup parity: DDS=%d rendered=%d", len(legacy), len(rendered))

		for i := range legacy {
			lhash, lerr := legacy[i].Hash()
			Expect(lerr).NotTo(HaveOccurred())
			rhash, rerr := rendered[i].Hash()
			Expect(rerr).NotTo(HaveOccurred())
			Expect(rhash).To(Equal(lhash),
				"nil-spec startup parity violation on spec[%d]: %s vs %s",
				i, legacy[i].Name, rendered[i].Name)
		}
	})

	It("Hash-equality on a Variables-populated spec", func() {
		rawSpec := config.UserSpec{
			Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid-comm-vars"
authToken: "test-token-comm-vars"`,
			Variables: config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": "502",
				},
			},
		}
		runParity(rawSpec, "variables-populated")
	})
})
