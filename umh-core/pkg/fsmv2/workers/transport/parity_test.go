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

package transport_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// Differential parity (P2.4 / pr2_issues #6) — anchors that the legacy
// SetChildSpecsFactory-driven DDS path and the canonical
// transport.RenderChildren emitter produce IDENTICAL ChildSpec slices for
// the transport worker during the migration window. Equality is asserted by
// ChildSpec.Hash(); see ChildSpec.Hash godoc for the canonical-form digest
// that catches every load-bearing field (Name, WorkerType, ChildStartStates,
// UserSpec.Config, UserSpec.Variables, Enabled, ...).
//
// The test ships now (P2.4 ship time) and is retired in P3.x once the legacy
// SetChildSpecsFactory path is deleted (the parity it asserts becomes
// trivially true — only one path remains). Until then, this test is the
// behavioral backstop for the supervisor-cutover discriminator: if either
// path drifts, the discriminator can no longer be retired safely.
//
// Failure-injection ledger: .execution/P2.4/differential_parity_failure_injection.txt
// records the PASS-FAIL-PASS round trip (mutate one path, confirm hash
// mismatch is reported with field-level diagnostics, revert).
var _ = Describe("Transport — DDS vs RenderChildren differential parity (P2.4)", func() {
	BeforeEach(func() {
		transport.SetChannelProvider(newTestChannelProvider())
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	// Realistic spec mirrors what the production templating layer feeds the
	// worker: relayURL/instanceUUID/authToken populated, Variables empty.
	// The canonical RenderChildren reads UserSpec from
	// snap.Desired.ChildrenSpecs[0] (its snapshotUserSpec helper); we
	// construct that snapshot from the legacy-DDS output so both paths
	// observe the same input.
	rawSpec := config.UserSpec{
		Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid-parity"
authToken: "test-token-parity"`,
		Variables: config.VariableBundle{},
	}

	It("Hash-equality on every emitted ChildSpec for the steady-state running spec", func() {
		identity := deps.Identity{ID: "parity-transport", Name: "Parity Transport"}
		logger := deps.NewNopFSMLogger()

		w, err := transport.NewTransportWorker(identity, logger, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// Path A — legacy DDS-derived children via ChildSpecProvider.
		desired, err := w.DeriveDesiredState(rawSpec)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue(),
			"transport DesiredState must implement ChildSpecProvider during migration window")

		legacy := provider.GetChildrenSpecs()

		// Path B — canonical RenderChildren emitter.
		// Build the snapshot the way the supervisor will at runtime: the
		// parent's typed Desired carries ChildrenSpecs (just emitted by DDS
		// in this same tick), so RenderChildren observes the same UserSpec
		// the legacy factory observed.
		snap := fsmv2.WorkerSnapshot[transport.TransportConfig, transport.TransportStatus]{
			Desired: fsmv2.WrappedDesiredState[transport.TransportConfig]{
				BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
				ChildrenSpecs:    legacy,
			},
		}
		rendered := transport.RenderChildren(snap)

		// Length parity first — easier failure mode to interpret.
		Expect(rendered).To(HaveLen(len(legacy)),
			"DDS and RenderChildren disagree on number of children: legacy=%d, rendered=%d",
			len(legacy), len(rendered))

		// Per-spec Hash equality. Hash() is the canonical-form digest; if it
		// matches, every field that the supervisor key-derives or
		// reconciles against is identical.
		for i := range legacy {
			lhash, lerr := legacy[i].Hash()
			Expect(lerr).NotTo(HaveOccurred(), "hashing legacy[%d]", i)
			rhash, rerr := rendered[i].Hash()
			Expect(rerr).NotTo(HaveOccurred(), "hashing rendered[%d]", i)

			Expect(rhash).To(Equal(lhash),
				"DDS vs RenderChildren parity violation on spec[%d]:\n"+
					"  legacy   = name=%q workerType=%q startStates=%v enabled=%v userSpec.Config=%q\n"+
					"  rendered = name=%q workerType=%q startStates=%v enabled=%v userSpec.Config=%q\n"+
					"This breaks the P2.4 supervisor-cutover discriminator: the legacy "+
					"DDS-derived path and the new NextResult.Children path MUST agree "+
					"during the migration window. If a divergence is intentional, the "+
					"legacy path must be retired in the same change (P3.x).",
				i,
				legacy[i].Name, legacy[i].WorkerType, legacy[i].ChildStartStates,
				legacy[i].Enabled, legacy[i].UserSpec.Config,
				rendered[i].Name, rendered[i].WorkerType, rendered[i].ChildStartStates,
				rendered[i].Enabled, rendered[i].UserSpec.Config)
		}
	})

	It("Hash-equality on the nil-spec startup path (both emit identical 2 specs)", func() {
		identity := deps.Identity{ID: "parity-transport-nil", Name: "Parity Transport Nil"}
		logger := deps.NewNopFSMLogger()

		w, err := transport.NewTransportWorker(identity, logger, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// Nil spec → DDS produces children with zero-value UserSpec.
		desired, err := w.DeriveDesiredState(nil)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue())

		legacy := provider.GetChildrenSpecs()

		snap := fsmv2.WorkerSnapshot[transport.TransportConfig, transport.TransportStatus]{
			Desired: fsmv2.WrappedDesiredState[transport.TransportConfig]{
				BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
				ChildrenSpecs:    legacy,
			},
		}
		rendered := transport.RenderChildren(snap)

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

	// Variables-populated case (P2.6 / pr2_issues #12). The supervisor-cutover
	// discriminator must remain bit-equal under realistic templating shapes:
	// production specs ship with both Config (a YAML body) AND Variables (the
	// per-tag substitution bundle). Hashing both legacy and rendered ChildSpec
	// must therefore round-trip through ChildSpec.Hash with the Variables
	// bundle intact. If RenderChildren ever drops Variables silently, this
	// test fails before P3.x retires the discriminator.
	It("Hash-equality on a Variables-populated spec (P2.6 expansion)", func() {
		identity := deps.Identity{ID: "parity-transport-vars", Name: "Parity Transport Vars"}
		logger := deps.NewNopFSMLogger()

		w, err := transport.NewTransportWorker(identity, logger, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		varsSpec := config.UserSpec{
			Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid-vars"
authToken: "test-token-vars"`,
			Variables: config.VariableBundle{
				User: map[string]any{
					"IP":   "192.168.1.100",
					"PORT": "502",
				},
			},
		}

		desired, err := w.DeriveDesiredState(varsSpec)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue())

		legacy := provider.GetChildrenSpecs()

		snap := fsmv2.WorkerSnapshot[transport.TransportConfig, transport.TransportStatus]{
			Desired: fsmv2.WrappedDesiredState[transport.TransportConfig]{
				BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
				ChildrenSpecs:    legacy,
			},
		}
		rendered := transport.RenderChildren(snap)

		Expect(rendered).To(HaveLen(len(legacy)),
			"Variables-populated parity: DDS=%d rendered=%d", len(legacy), len(rendered))

		for i := range legacy {
			lhash, lerr := legacy[i].Hash()
			Expect(lerr).NotTo(HaveOccurred(), "hashing legacy[%d] with Variables", i)
			rhash, rerr := rendered[i].Hash()
			Expect(rerr).NotTo(HaveOccurred(), "hashing rendered[%d] with Variables", i)

			Expect(rhash).To(Equal(lhash),
				"Variables-populated parity violation on spec[%d] (Variables=%v vs %v)",
				i, legacy[i].UserSpec.Variables, rendered[i].UserSpec.Variables)
		}
	})
})
