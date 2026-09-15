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

// This file asserts the healthy shape of the wire key contract the Verdict
// doc defines: the console's CpuVerdict type (ManagementConsole
// frontend/src/lib/utils/cpu/cpuHealth.ts) is a discriminated union whose
// healthy case carries no attribution and no causes, so a healthy verdict
// marshals to the state key alone. verdict_json_test.go asserts the
// degraded shape, which requires an attribution and at least one cause.

package cpuhealth

import (
	"encoding/json"
	"maps"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("healthy verdict JSON", func() {
	It("should omit attribution and causes from a healthy verdict and keep them on a degraded one", func() {
		// The staged healthy shape is the zero value — State set, attribution
		// empty, causes empty — which is what omitempty needs to drop both keys.
		healthy := Verdict{State: StateHealthy}

		rawHealthy, err := json.Marshal(healthy)
		Expect(err).NotTo(HaveOccurred())

		var healthyDocument map[string]json.RawMessage
		Expect(json.Unmarshal(rawHealthy, &healthyDocument)).To(Succeed())
		Expect(slices.Sorted(maps.Keys(healthyDocument))).To(Equal([]string{"state"}),
			"a healthy verdict's document carries the state key alone")

		// The value string is the console's literal, never the StateHealthy
		// constant: comparing the constant would let a rename pass green.
		var healthyState string
		Expect(json.Unmarshal(healthyDocument["state"], &healthyState)).To(Succeed())
		Expect(healthyState).To(Equal("healthy"))

		// A degraded verdict in the same test: its non-empty attribution and
		// cause list prove the omission comes from emptiness, not from a tag
		// that would drop the keys from every verdict.
		degraded := Verdict{
			State:       StateDegraded,
			Attribution: AttributionHost,
			Causes: []Cause{
				{Kind: CauseKindSteal, Instrument: instrumentStealP95, Attribution: AttributionHost, Unit: unitRatio, Value: 0.18},
			},
		}

		rawDegraded, err := json.Marshal(degraded)
		Expect(err).NotTo(HaveOccurred())

		var degradedDocument map[string]json.RawMessage
		Expect(json.Unmarshal(rawDegraded, &degradedDocument)).To(Succeed())
		Expect(slices.Sorted(maps.Keys(degradedDocument))).To(Equal([]string{"attribution", "causes", "state"}),
			"a degraded verdict keeps the attribution and causes keys")
	})
})
