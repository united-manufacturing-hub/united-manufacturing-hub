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

// The wrong subtraction. host-headroom's Extract carries the scope
// guard — Unknown unless CpuScope == ScopeHost — in the extractor, not in
// Decide, so nothing enters the window, the reduction is StateAbsent, and the
// host-cpu-full latch has nothing to judge. A box whose core count was never
// readable declares no host-cpu-full row at all (see cpuTable). The three Details
// fields carry the withholding to the message layer: HostHeadroomAvailable
// (dispatched on the scope, not the window state) plus the two core counts.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the missing guard, and the wrong subtraction", func() {
	// headroomSample builds a sample for the host-headroom guards, with every
	// other signal parked below its mark so only host-cpu-full is exercised.
	headroomSample := func(base time.Time, i int, scope Scope, logical, host float64) Sample {
		return Sample{
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			CpuScope:    scope,
			HostBusy:    diagnosis.Known(0.5),
			LogicalCpus: diagnosis.Known(logical),
			HostCpus:    diagnosis.Known(host),
			Pressure:    diagnosis.Known(0),
			Steal:       diagnosis.Known(0),
			NrPeriods:   diagnosis.Known(0),
			NrThrottled: diagnosis.Known(0),
			UsageCores:  diagnosis.Known(0),
		}
	}

	It("should declare no host-cpu-full signal on a box whose core count was never readable, and stay healthy", func() {
		// The omission is the gate, so assert it directly: a cores<=0 box declares
		// no host-cpu-full row at all. The message-field checks that used to sit in
		// this loop are gone because they do not discriminate — engine.Reduction on
		// a missing signal returns the same (0.0, StateAbsent) as a
		// present-but-withholding row, so they passed under both designs.
		Expect(hasSignal(cpuTable(0, 2.0), sigHostCpuFull)).To(BeFalse(),
			"a box with no readable core count must declare no host-cpu-full signal")

		// Never-readable is not capability: the box still builds and reads healthy
		// on every tick, because there is no window to fill and no latch to fire.
		engine, err := NewEngine(0, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		for i := 0; i < 5; i++ {
			verdict, _ := Decide(engine, headroomSample(base, i, ScopeHost, 0, 8), env)
			Expect(verdict.State).To(Equal(StateHealthy), "a box with no declared host-cpu-full signal must not fire the latch")
		}
	})

	It("should withhold host headroom entirely unless the sample's scope says the core count covers the whole machine, which the busy figure always does", func() {
		// An idle container pinned to 2 of 8 reads ScopeAffinity: subtracting
		// the host's busy figure from an affinity-scoped count is invalid, and
		// the invalid subtraction must never reach the window. ScopeUnknown is
		// the same withholding — a scope we failed to establish is not a scope
		// we established as the host.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		for i := 0; i < 5; i++ {
			smp := headroomSample(base, i, ScopeAffinity, 2, 8)
			verdict, _ := Decide(engine, smp, env)
			_, st := engine.Reduction(sigHostCpuFull, instHostHeadroom).Get()
			Expect(st).To(Equal(diagnosis.StateAbsent), "an affinity-scoped sample must append nothing to host-headroom")
			Expect(verdict.State).To(Equal(StateHealthy), "the host-cpu-full latch must not fire on an invalid subtraction")
		}

		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 5; i++ {
			smp := headroomSample(base, i, ScopeUnknown, 2, 8)
			verdict, _ := Decide(engine2, smp, env)
			_, st := engine2.Reduction(sigHostCpuFull, instHostHeadroom).Get()
			Expect(st).To(Equal(diagnosis.StateAbsent), "an unestablished scope must withhold host headroom too")
			Expect(verdict.State).To(Equal(StateHealthy))
		}
	})

	It("should report host headroom as unavailable with the two core counts, rather than as a reading that failed", func() {
		// The customer sentence is "host headroom unavailable: this container is
		// pinned to 2 of 8 CPUs". The three Details fields carry the withholding
		// and both counts; a verdict built from the scope alone cannot render the
		// sentence and re-reading the count in Decide would break zero-I/O.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		smp := headroomSample(base, 0, ScopeAffinity, 2, 8)
		_, sig := Decide(engine, smp, env)
		Expect(sig.HostHeadroomAvailable).To(BeFalse(), "an affinity box withholds headroom: withheld, not failed")
		Expect(sig.LogicalCpus).To(Equal(2.0))
		Expect(sig.HostCpus).To(Equal(8.0))

		// ScopeHost keeps the bit set even when the window is absent on a read
		// failure — the bit dispatches on the scope, so a plain /proc/stat
		// outage is a failed read, not a withholding (the conflation inverted).
		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		smp2 := headroomSample(base, 0, ScopeHost, 4, 8)
		smp2.HostBusy = diagnosis.Unknown()
		_, sig2 := Decide(engine2, smp2, env)
		Expect(sig2.HostHeadroomAvailable).To(BeTrue(), "a host-scoped sample is available even when the busy read failed")
		_, hhst := engine2.Reduction(sigHostCpuFull, instHostHeadroom).Get()
		Expect(hhst).To(Equal(diagnosis.StateAbsent), "the failed read leaves the window absent")
	})
})
