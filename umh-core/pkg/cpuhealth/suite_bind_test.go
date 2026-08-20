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

// Bind the generated suite to the real CPU table. The six-scenario
// suite is generated from cpuTable itself, not from a fixture, so a signal
// added to the CPU table without going through the readability path has nowhere
// to hide. Readable advances the cumulative throttle counters so the ratio
// stays below its mark; Unreadable leaves every Reading absent while keeping the
// startup facts (Virtualized, CpuScope) fixed.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("bind the generated suite to the real CPU table", func() {
	It("should generate the six-scenario suite from the CPU signal table itself, not from a fixture", func() {
		// 30 scenarios on a box with a positive quota, 24 on one without:
		// 6 x 5 and 6 x 4, because cpuTable omits container-limit-full entirely at
		// quota 0. A suite built from a fixture cannot tell the two apart, and
		// one that generated scenarios for the tracks would read 42 and 36.
		out30 := RunSuite(4, 2.0)
		Expect(out30).To(HaveLen(30), "6 x 5 signals with a quota")
		out24 := RunSuite(4, 0)
		Expect(out24).To(HaveLen(24), "6 x 4 signals without a quota")

		// The readability path lands where it should: a live, fully-readable
		// capable signal reaches Ready; a Requires-gated signal reaches
		// NoInstrument on the unsupported case; a no-Requires signal stays Ready
		// there.
		liveThrottle := outcome(out30, "throttling", diagnosis.CaseLive)
		Expect(liveThrottle).To(Equal(diagnosis.Ready), "throttle-ratio with a rising denominator must reach Ready on live")
		unsupportedThrottle := outcome(out30, "throttling", diagnosis.CaseUnsupported)
		Expect(unsupportedThrottle).To(Equal(diagnosis.NoInstrument), "throttling requires HasLimit, absent in the unsupported case")
		unsupportedHostCpuFull := outcome(out30, "host-cpu-full", diagnosis.CaseUnsupported)
		Expect(unsupportedHostCpuFull).To(Equal(diagnosis.Ready), "host-cpu-full requires nothing, so it stays Ready unsupported")
		longThrottle := outcome(out30, "throttling", diagnosis.CaseLongOutage)
		Expect(longThrottle).To(Equal(diagnosis.AllAbsent), "a long outage demotes and empties the throttle window")
	})

	It("should fail when a signal is added to the CPU table without going through the readability path", func() {
		// zeroForAbsent: a sixth CPU row whose Extract returns Known(0) on an
		// absent reading instead of Unknown() — the whole readability contract
		// violated in one line. It reaches Ready wherever the reader is required
		// to say the window is unusable: CaseBriefOutage (NoneReady required),
		// CaseLongOutage (AllAbsent required) and CasePostOutageDip (NoneReady
		// required). CaseBelowFloor stays green because it never drives the
		// absent branch. Three failing outcomes, and the only way to green them
		// is to give the row a real absence.
		t := cpuTable(4, 2.0)
		t.Signals = append(t.Signals, badReadSignal())
		out := diagnosis.Run(t, diagnosis.NewEnvironment(HasLimit, HasVirtualization), cpuFeed{cores: 4, quota: 2.0})

		Expect(outcome(out, "bad-read", diagnosis.CaseBriefOutage)).To(Equal(diagnosis.Ready),
			"zeroForAbsent reaches Ready on the brief outage, where NoneReady is required")
		Expect(outcome(out, "bad-read", diagnosis.CaseLongOutage)).To(Equal(diagnosis.Ready),
			"zeroForAbsent reaches Ready on the long outage, where AllAbsent is required")
		Expect(outcome(out, "bad-read", diagnosis.CasePostOutageDip)).To(Equal(diagnosis.Ready),
			"zeroForAbsent reaches Ready on the post-outage dip, where NoneReady is required")
		Expect(outcome(out, "bad-read", diagnosis.CaseBelowFloor)).To(Equal(diagnosis.NoneReady),
			"CaseBelowFloor never reaches the absent branch, so it stays green under zeroForAbsent")

		// And the good rows in the SAME run still behave: the real table rows
		// reach the correct availability, so it is the zeroForAbsent row and
		// only the zeroForAbsent row that the suite exposes.
		Expect(outcome(out, "throttling", diagnosis.CaseLongOutage)).To(Equal(diagnosis.AllAbsent))
	})
})

// outcome returns the availability one scenario concluded from a Run output.
func outcome(out []diagnosis.Outcome, signal string, c diagnosis.Case) diagnosis.Availability {
	for _, o := range out {
		if o.Signal == signal && o.Case == c {
			return o.Availability
		}
	}
	return diagnosis.NoInstrument
}

// badReadSignal builds the suite's zeroForAbsent row: a CPU row whose Extract
// returns Known(0) even on an absent reading, skipping the readability path. It
// is only used to prove the suite catches such a row, never part of the live
// table.
func badReadSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            "bad-read",
		Tier:            tierStarvation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name: "bad-read-inst",
				Extract: func(s Sample) diagnosis.Reading {
					return diagnosis.Known(0) // absent treated as a real zero: the defect
				},
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean,
			},
			Marks: diagnosis.Marks{
				Fire:     diagnosis.Mark{At: 0.5},
				Clear:    diagnosis.Mark{At: 0.4},
				Polarity: diagnosis.HigherIsWorse,
				Unit:     "ratio",
				Worst:    1.0,
			},
		}},
	}
}
