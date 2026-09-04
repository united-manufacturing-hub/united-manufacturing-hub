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

// The wire key contract the Verdict doc defines. The verdict will reach the
// Management Console inside the status message umh-core pushes to the console,
// and a key or a value string the console does not expect leaves the console's
// CPU-health row unfilled. This file decodes the marshalled document into
// maps, not back into the struct: a round trip through Verdict would pass
// whatever the tags say, and the key names are the behavior under test.

package cpuhealth

import (
	"encoding/json"
	"maps"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("verdict JSON", func() {
	It("should marshal Verdict and Cause under the console's key names and value strings", func() {
		// A staged degraded verdict, not one produced by Decide: the marshalled
		// document is what the tags and the constants produce, so an engine path
		// would force nothing the assertions read. One cause per kind, with the
		// attributions and units spread across them, so every vocabulary
		// constant crosses the wire. Each row carries its signal's real
		// instrument, blame and unit.
		verdict := Verdict{
			State:       StateDegraded,
			Attribution: AttributionHost,
			Causes: []Cause{
				{Kind: CauseKindSteal, Instrument: instrumentStealP95, Attribution: AttributionHost, Unit: unitRatio, Value: 0.18},
				{Kind: CauseKindPressure, Instrument: instrumentPressureAvg60, Attribution: AttributionUnknown, Unit: unitRatio, Value: 0.40},
				{Kind: CauseKindThrottling, Instrument: instrumentThrottleRatio, Attribution: AttributionContainer, Unit: unitRatio, Value: 0.08},
				{Kind: CauseKindHostCpuFull, Instrument: refinementHostShare, Attribution: AttributionHost, Unit: unitFraction, Value: 0.55},
				{Kind: CauseKindContainerLimitFull, Instrument: instrumentLimitHeadroom, Attribution: AttributionContainer, Unit: unitCores, Value: -0.2},
			},
		}

		raw, err := json.Marshal(verdict)
		Expect(err).NotTo(HaveOccurred())

		var document map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &document)).To(Succeed())
		Expect(slices.Sorted(maps.Keys(document))).To(Equal([]string{"attribution", "causes", "state"}),
			"the wire key contract on the Verdict doc fixes a verdict's key names")

		// The value strings below are the console's literals, never the Go
		// constants: comparing against the constants would let a constant's
		// value be renamed without this spec going red.
		var state string
		Expect(json.Unmarshal(document["state"], &state)).To(Succeed())
		Expect(state).To(Equal("degraded"))
		var attribution string
		Expect(json.Unmarshal(document["attribution"], &attribution)).To(Succeed())
		Expect(attribution).To(BeElementOf("host", "container", "unknown"))

		var causes []map[string]json.RawMessage
		Expect(json.Unmarshal(document["causes"], &causes)).To(Succeed())

		// instrument is the string the console labels the row with, so it is
		// decoded and asserted beside kind rather than only checked for
		// presence. Asserting the four together fixes the whole per-cause
		// mapping: a renamed instrument, a changed attribution or a changed
		// unit each fail here on their own kind.
		type wireCause struct {
			kind        string
			instrument  string
			attribution string
			unit        string
		}

		decoded := make([]wireCause, len(causes))
		for i, cause := range causes {
			Expect(slices.Sorted(maps.Keys(cause))).To(Equal([]string{"attribution", "instrument", "kind", "unit", "value"}),
				"the wire key contract on the Verdict doc fixes a cause's key names")
			Expect(json.Unmarshal(cause["kind"], &decoded[i].kind)).To(Succeed())
			Expect(json.Unmarshal(cause["instrument"], &decoded[i].instrument)).To(Succeed())
			Expect(json.Unmarshal(cause["attribution"], &decoded[i].attribution)).To(Succeed())
			Expect(json.Unmarshal(cause["unit"], &decoded[i].unit)).To(Succeed())
		}

		Expect(decoded).To(ConsistOf(
			wireCause{kind: "steal", instrument: "steal-p95", attribution: "host", unit: "ratio"},
			wireCause{kind: "pressure", instrument: "pressure-avg60", attribution: "unknown", unit: "ratio"},
			wireCause{kind: "throttling", instrument: "throttle-ratio", attribution: "container", unit: "ratio"},
			wireCause{kind: "host-cpu-full", instrument: "host-share", attribution: "host", unit: "fraction"},
			wireCause{kind: "container-limit-full", instrument: "limit-headroom", attribution: "container", unit: "cores"},
		))
	})
})
