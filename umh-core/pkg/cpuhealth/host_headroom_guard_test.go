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

// S3 R5 (F2, F6): the missing guard and the wrong subtraction. host-headroom's
// Extract carries BOTH guards — Unknown on cores <= 0 (F2) and Unknown unless
// CpuScope == ScopeHost (F6) — and both live in the extractor, not in Decide,
// so nothing enters the window, the reduction is StateAbsent, and the saturation
// latch has nothing to judge. The three Signals fields carry the withholding to
// the message layer: HostHeadroomAvailable (dispatched on the scope, not the
// window state) plus the two core counts.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("S3 R5 — the missing guard, and the wrong subtraction", func() {
	// headroomSample builds a sample for the host-headroom guards, with every
	// other signal parked below its mark so only saturation is exercised.
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

	It("should treat a non-positive logical CPU count as no signal on the host-headroom arm, as usage-fraction already does", func() {
		// cores = 0 in the table: host-headroom's subtraction would be from
		// nothing and usage-fraction's division would be by zero. Both Extract
		// arms return Unknown, so the saturation window receives nothing and the
		// signal reaches AllAbsent — the latch cannot fire. F2 has three sites
		// and a guard in the extractor closes all three at once.
		engine, err := NewEngine(0, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		for i := 0; i < 5; i++ {
			verdict, _ := Decide(engine, headroomSample(base, i, ScopeHost, 0, 8), env)
			red, st := engine.Reduction(sigSaturation, instHostHeadroom).Get()
			Expect(st).To(Equal(diagnosis.StateAbsent), "a non-positive core count must append nothing to host-headroom")
			Expect(red).To(Equal(0.0))
			_, ufst := engine.Reduction(sigSaturation, instUsageFraction).Get()
			Expect(ufst).To(Equal(diagnosis.StateAbsent), "usage-fraction must refuse the same count")
			Expect(verdict.State).To(Equal(StateHealthy), "an unjudgeable saturation signal must not fire the latch")
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
			_, st := engine.Reduction(sigSaturation, instHostHeadroom).Get()
			Expect(st).To(Equal(diagnosis.StateAbsent), "an affinity-scoped sample must append nothing to host-headroom")
			Expect(verdict.State).To(Equal(StateHealthy), "the saturation latch must not fire on an invalid subtraction")
		}

		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 5; i++ {
			smp := headroomSample(base, i, ScopeUnknown, 2, 8)
			verdict, _ := Decide(engine2, smp, env)
			_, st := engine2.Reduction(sigSaturation, instHostHeadroom).Get()
			Expect(st).To(Equal(diagnosis.StateAbsent), "an unestablished scope must withhold host headroom too")
			Expect(verdict.State).To(Equal(StateHealthy))
		}
	})

	It("should report host headroom as unavailable with the two core counts, rather than as a reading that failed", func() {
		// The customer sentence is "host headroom unavailable: this container is
		// pinned to 2 of 8 CPUs". The three Signals fields carry the withholding
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
		// outage is a failed read, not a withholding (the F1 conflation inverted).
		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		smp2 := headroomSample(base, 0, ScopeHost, 4, 8)
		smp2.HostBusy = diagnosis.Unknown()
		_, sig2 := Decide(engine2, smp2, env)
		Expect(sig2.HostHeadroomAvailable).To(BeTrue(), "a host-scoped sample is available even when the busy read failed")
		_, hhst := engine2.Reduction(sigSaturation, instHostHeadroom).Get()
		Expect(hhst).To(Equal(diagnosis.StateAbsent), "the failed read leaves the window absent")
	})
})
