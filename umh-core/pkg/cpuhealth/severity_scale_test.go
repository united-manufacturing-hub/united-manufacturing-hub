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

// Severity is what orders two causes inside one tier, and the two headroom arms
// are the only ones whose scale is not already a 0..1 ratio. Both are
// LowerIsWorse in cores and both bottom out below their fire mark at zero, so
// the capacity that normalises them has to be the reserve they subtract — not
// the total they subtract it from. Declaring the total instead put a wholly
// consumed container at 0.10 and a wholly consumed host at 0.25, under a merely
// busy box at 1.00 in the same tier, so the worst cause sorted last.
//
// The arms are read off the built table rather than reconstructed, so a capacity
// changed in signal_saturation.go is a capacity this measures.
package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("severity scale of the headroom arms", func() {
	const cores, quota = 4.0, 2.0

	// severityAt judges a raw value against an instrument's own marks, the same
	// path Rank takes.
	severityAt := func(m diagnosis.Marks, v float64) float64 {
		return diagnosis.Fired{Marks: m, Value: v}.Severity()
	}

	It("should reach severity 1 where each headroom arm's quantity bottoms out, so the arms rank against the ratio arms", func() {
		t := Table(cores, quota)

		hostCpuFull := t.Signals[3]
		hostHeadroom := hostCpuFull.Instruments[0].Marks
		Expect(hostCpuFull.Instruments[0].Name).To(Equal("host-headroom"))
		usageFraction := hostCpuFull.Instruments[1].Marks
		Expect(hostCpuFull.Instruments[1].Name).To(Equal("usage-fraction"))

		containerLimitFull := t.Signals[4]
		limitHeadroom := containerLimitFull.Instruments[0].Marks
		Expect(containerLimitFull.Instruments[0].Name).To(Equal("limit-headroom"))

		// Both arms live in the tier the ordering broke in.
		Expect(hostCpuFull.Tier).To(Equal(containerLimitFull.Tier))

		// host-headroom is cores − hostBusy − cpuReserveCores, and hostBusy
		// cannot exceed cores, so the floor is −cpuReserveCores.
		Expect(severityAt(hostHeadroom, 0)).To(Equal(0.0), "at the fire mark nothing is wrong yet")
		Expect(severityAt(hostHeadroom, -cpuReserveCores)).To(Equal(1.0), "a wholly consumed host is the worst this arm can see")

		// limit-headroom is quota − usage − 0.10 × quota, and the kernel
		// throttles usage at the quota, so the floor is −0.10 × quota.
		Expect(severityAt(limitHeadroom, 0)).To(Equal(0.0), "at the fire mark nothing is wrong yet")
		Expect(severityAt(limitHeadroom, -0.10*quota)).To(Equal(1.0), "a container wholly out of its budget is the worst this arm can see")

		// The comparison that was inverted: a box merely at its usage ceiling
		// must not outrank either arm at its floor.
		busyBox := severityAt(usageFraction, 1.0)
		Expect(busyBox).To(Equal(1.0))
		Expect(severityAt(limitHeadroom, -0.10*quota)).To(BeNumerically(">=", busyBox))
		Expect(severityAt(hostHeadroom, -cpuReserveCores)).To(BeNumerically(">=", busyBox))

		// Halfway to the floor reads as half severity on both, which is what
		// makes a partial reading on one arm comparable to one on the other.
		Expect(severityAt(hostHeadroom, -0.5*cpuReserveCores)).To(Equal(0.5))
		Expect(severityAt(limitHeadroom, -0.05*quota)).To(Equal(0.5))
	})
})
