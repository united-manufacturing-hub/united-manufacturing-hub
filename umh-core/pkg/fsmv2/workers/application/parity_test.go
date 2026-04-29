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

package application_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// Differential parity (P2.6 / pr2_issues #12 expansion) — anchors that the
// legacy DDS-derived ChildrenSpecs (read via GetChildrenSpecs) and the
// canonical application.RenderChildren emitter produce IDENTICAL ChildSpec
// slices for the application worker during the migration window.
//
// The application worker is the YAML-passthrough parent: it reads a list of
// children from the user's YAML config and projects them into
// snap.Desired.ChildrenSpecs with Enabled forced to true (§4-C YAML
// passthrough exception). RenderChildren reads that same slice back out and
// re-asserts Enabled: true. The parity property is "the two reads observe
// the same set of fields under Hash()" — Variables included.
//
// The test ships at P2.6 ship time and is retired in P3.x once the legacy
// DDS-via-GetChildrenSpecs path is the only path remaining.
var _ = Describe("Application — DDS vs RenderChildren differential parity (P2.6)", func() {
	logger := deps.NewNopFSMLogger()

	runParity := func(rawSpec config.UserSpec, label string) {
		w := application.NewApplicationWorker("parity-app-"+label, "ParityApp"+label, logger, nil)
		Expect(w).NotTo(BeNil())

		desired, err := w.DeriveDesiredState(rawSpec)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue(),
			"application DesiredState must implement ChildSpecProvider during migration window")

		legacy := provider.GetChildrenSpecs()

		typedDesired := desired.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
		snap := fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
			Desired: *typedDesired,
		}
		rendered := application.RenderChildren(snap)

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

	It("Hash-equality for a multi-child YAML config", func() {
		yamlConfig := `
children:
  - name: "child-1"
    workerType: "example-child"
    childStartStates: ["TryingToStart", "Running"]
    userSpec:
      config: |
        value: 10
  - name: "child-2"
    workerType: "example-child"
    childStartStates: ["TryingToStart", "Running"]
    userSpec:
      config: |
        value: 20
`
		runParity(config.UserSpec{Config: yamlConfig}, "multi-child")
	})

	It("Hash-equality for the empty-children path", func() {
		runParity(config.UserSpec{Config: `children: []`}, "empty-children")
	})

	It("Hash-equality for the nil-spec startup path", func() {
		w := application.NewApplicationWorker("parity-app-nil", "ParityAppNil", logger, nil)
		Expect(w).NotTo(BeNil())

		desired, err := w.DeriveDesiredState(nil)
		Expect(err).NotTo(HaveOccurred())

		provider, ok := desired.(config.ChildSpecProvider)
		Expect(ok).To(BeTrue())

		legacy := provider.GetChildrenSpecs()

		typedDesired := desired.(*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig])
		snap := fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]{
			Desired: *typedDesired,
		}
		rendered := application.RenderChildren(snap)

		Expect(rendered).To(HaveLen(len(legacy)),
			"nil-spec startup parity: DDS=%d rendered=%d", len(legacy), len(rendered))
	})

	It("Hash-equality for a Variables-populated child spec", func() {
		yamlConfig := `
children:
  - name: "child-vars"
    workerType: "example-child"
    childStartStates: ["TryingToStart", "Running"]
    userSpec:
      config: |
        endpoint: "{{ .IP }}:{{ .PORT }}"
      variables:
        user:
          IP: "192.168.1.100"
          PORT: "502"
`
		runParity(config.UserSpec{Config: yamlConfig}, "variables-populated")
	})
})
