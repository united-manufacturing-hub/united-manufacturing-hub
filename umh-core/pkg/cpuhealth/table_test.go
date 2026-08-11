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

// S3 R1 (F8): the CPU table, throttle and steal. cpuTable builds the whole
// declaration — five signals, seven instruments, both tracks — as a function,
// not a package-level variable, because two marks and two capacities are
// denominated in the quota. A no-quota table omits limit-saturation entirely,
// since Fire{At:0}/Clear{At:0.05×0} is a pair NewEngine refuses. Throttle fires
// above a 0.05 sixty-second ratio and clears below 0.03; steal is judged on the
// p95 once the ring holds twenty entries and on the mean before that; the p95
// is never selected below its twenty-sample minimum — the whole of F8.
package cpuhealth

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// signalNamed returns the table's signal with the given name, failing the spec
// if the table does not declare it. The engine's windows are keyed by the
// signal name, so a signal taken from one cpuTable call drives an engine built
// from another.
func signalNamed(t diagnosis.Table[Sample], name string) diagnosis.Signal[Sample] {
	for _, s := range t.Signals {
		if s.Name == name {
			return s
		}
	}
	Fail("table does not declare signal " + name)
	return diagnosis.Signal[Sample]{}
}

// hasSignal reports whether the table declares a signal with the given name,
// without failing when it does not. It is the absence-asserting counterpart to
// signalNamed: F2-R3's contract is that a box with no readable core count
// carries no saturation row at all, not a row that is merely never filled.
func hasSignal(t diagnosis.Table[Sample], name string) bool {
	for _, s := range t.Signals {
		if s.Name == name {
			return true
		}
	}
	return false
}

var _ = Describe("S3 R1 — the CPU table, throttle and steal", func() {
	It("declares every CPU signal in one table built by cpuTable, and omits limit-saturation when there is no positive quota", func() {
		t := cpuTable(4, 2.0)

		// Five signals, in Rank's last tie-break order, with limit-saturation
		// the fifth and last row.
		Expect(t.Signals).To(HaveLen(5))
		names := make([]string, 0, len(t.Signals))
		for _, s := range t.Signals {
			names = append(names, s.Name)
		}
		Expect(names).To(Equal([]string{"throttling", "pressure", "steal", "saturation", "limit-saturation"}))

		// Every signal declares the same cadence facts: 60s demote span, 60s
		// instrument span, release-on-absent true. CPU cannot witness the flag,
		// so a build that drops it is green across the whole recording.
		for _, s := range t.Signals {
			Expect(s.DemoteSpan).To(Equal(60 * time.Second))
			Expect(s.ReleaseOnAbsent).To(BeTrue())
			for _, inst := range s.Instruments {
				Expect(inst.Span).To(Equal(60 * time.Second))
			}
		}

		// Starvation outranks saturation: the three starvation signals carry
		// tier 0, the two saturation signals tier 1. External is true on steal
		// alone — Rank's third tie-break and nothing else.
		for _, idx := range []int{0, 1, 2} {
			Expect(t.Signals[idx].Tier).To(Equal(0), "%s must be a starvation signal", t.Signals[idx].Name)
		}
		for _, idx := range []int{3, 4} {
			Expect(t.Signals[idx].Tier).To(Equal(1), "%s must be a saturation signal", t.Signals[idx].Name)
		}
		for _, idx := range []int{0, 1, 3, 4} {
			Expect(t.Signals[idx].External).To(BeFalse(), "%s must not be external", t.Signals[idx].Name)
		}
		Expect(t.Signals[2].External).To(BeTrue(), "steal is the only external signal")

		// The interval the caller ticks at.
		Expect(t.Interval).To(Equal(time.Second))

		throttling := t.Signals[0]
		Expect(throttling.Instruments).To(HaveLen(1))
		throttleInst := throttling.Instruments[0]
		Expect(throttleInst.Name).To(Equal("throttle-ratio"))
		Expect(throttleInst.Requires).To(ConsistOf(HasLimit))
		Expect(throttleInst.Red.Name).To(Equal("deltaRatio"))
		Expect(throttleInst.Red.Min).To(Equal(2))
		Expect(throttleInst.Counter).To(BeTrue(), "throttle-ratio reads running totals")
		Expect(throttleInst.Marks.Fire.At).To(Equal(0.05))
		Expect(throttleInst.Marks.Fire.Inclusive).To(BeFalse())
		Expect(throttleInst.Marks.Clear.At).To(Equal(0.03))
		Expect(throttleInst.Marks.Clear.Inclusive).To(BeFalse())
		Expect(throttleInst.Marks.Polarity).To(Equal(diagnosis.HigherIsWorse))
		Expect(throttleInst.Marks.Unit).To(Equal("ratio"))
		Expect(throttleInst.Marks.Worst).To(Equal(1.0))
		Expect(throttleInst.Extract(Sample{NrThrottled: diagnosis.Known(7)})).To(Equal(diagnosis.Known(7)))
		Expect(throttleInst.Against(Sample{NrPeriods: diagnosis.Known(100)})).To(Equal(diagnosis.Known(100)))

		pressure := t.Signals[1]
		Expect(pressure.Instruments).To(HaveLen(1))
		pressureInst := pressure.Instruments[0]
		Expect(pressureInst.Name).To(Equal("pressure-avg60"))
		Expect(pressureInst.Red.Name).To(Equal("last"))
		Expect(pressureInst.Red.Min).To(Equal(1))
		Expect(pressureInst.Marks.Fire.At).To(Equal(0.20))
		Expect(pressureInst.Marks.Clear.At).To(Equal(0.12))
		Expect(pressureInst.Marks.Polarity).To(Equal(diagnosis.HigherIsWorse))
		Expect(pressureInst.Marks.Unit).To(Equal("ratio"))
		Expect(pressureInst.Marks.Worst).To(Equal(1.0))
		Expect(pressureInst.Extract(Sample{Pressure: diagnosis.Known(0.25)})).To(Equal(diagnosis.Known(0.25)))

		// The two steal arms answer one question. The p95 is the primary arm and
		// its minimum is the reduction's own twenty; the mean stands in for it at
		// two samples and shares the SAME mark pair — no second threshold.
		steal := t.Signals[2]
		Expect(steal.Instruments).To(HaveLen(2))
		stealP95 := steal.Instruments[0]
		Expect(stealP95.Name).To(Equal("steal-p95"))
		Expect(stealP95.Requires).To(ConsistOf(HasVirtualization))
		Expect(stealP95.Red.Name).To(Equal("p95"))
		Expect(stealP95.Red.Min).To(Equal(20))
		Expect(stealP95.Counter).To(BeFalse())
		Expect(stealP95.Marks.Fire.At).To(Equal(0.10))
		Expect(stealP95.Marks.Clear.At).To(Equal(0.06))
		Expect(stealP95.Marks.Polarity).To(Equal(diagnosis.HigherIsWorse))
		Expect(stealP95.Marks.Unit).To(Equal("ratio"))
		Expect(stealP95.Marks.Worst).To(Equal(1.0))
		stealMean := steal.Instruments[1]
		Expect(stealMean.Name).To(Equal("steal-mean"))
		Expect(stealMean.Requires).To(ConsistOf(HasVirtualization))
		Expect(stealMean.Red.Name).To(Equal("mean"))
		Expect(stealMean.Red.Min).To(Equal(2))
		Expect(stealMean.Counter).To(BeFalse())
		Expect(stealMean.Marks).To(Equal(stealP95.Marks), "the mean fallback shares the p95 bar")
		for _, inst := range steal.Instruments {
			Expect(inst.Extract(Sample{Steal: diagnosis.Known(0.9)})).To(Equal(diagnosis.Known(0.9)))
		}

		// The saturation signal holds the machine-full question twice: host
		// headroom from /proc/stat, and usage fraction from our own usage as the
		// fallback when /proc/stat is unreadable. host-headroom is listed first
		// so selection prefers it whenever its window can supply a value.
		saturation := t.Signals[3]
		Expect(saturation.Instruments).To(HaveLen(2))
		hostHeadroom := saturation.Instruments[0]
		Expect(hostHeadroom.Name).To(Equal("host-headroom"))
		Expect(hostHeadroom.Red.Name).To(Equal("mean"))
		Expect(hostHeadroom.Red.Min).To(Equal(2))
		Expect(hostHeadroom.Marks.Polarity).To(Equal(diagnosis.LowerIsWorse))
		Expect(hostHeadroom.Marks.Fire.At).To(Equal(0.0))
		Expect(hostHeadroom.Marks.Fire.Inclusive).To(BeFalse())
		Expect(hostHeadroom.Marks.Clear.At).To(Equal(0.5))
		Expect(hostHeadroom.Marks.Clear.Inclusive).To(BeFalse())
		Expect(hostHeadroom.Marks.Unit).To(Equal("cores"))
		Expect(hostHeadroom.Marks.Worst).To(Equal(-1.0), "the reserve, not the core count: severity 1 at −1.0 cores, the floor of cores−hostBusy−reserve")
		// The F6 guard: headroom is only a number on a host-scoped sample, and
		// only when both the count and the busy rate are present.
		Expect(hostHeadroom.Extract(Sample{CpuScope: ScopeHost, HostBusy: diagnosis.Known(0.5)})).To(Equal(diagnosis.Known(2.5)))
		Expect(hostHeadroom.Extract(Sample{CpuScope: ScopeAffinity, HostBusy: diagnosis.Known(0.5)})).To(Equal(diagnosis.Unknown()))
		Expect(hostHeadroom.Extract(Sample{CpuScope: ScopeHost, HostBusy: diagnosis.Unknown()})).To(Equal(diagnosis.Unknown()))
		usageFraction := saturation.Instruments[1]
		Expect(usageFraction.Name).To(Equal("usage-fraction"))
		Expect(usageFraction.Red.Name).To(Equal("mean"))
		Expect(usageFraction.Red.Min).To(Equal(2))
		Expect(usageFraction.Marks.Polarity).To(Equal(diagnosis.HigherIsWorse))
		Expect(usageFraction.Marks.Fire.At).To(Equal(0.70))
		Expect(usageFraction.Marks.Fire.Inclusive).To(BeTrue(), "0.70 fires AT the mark")
		Expect(usageFraction.Marks.Clear.At).To(Equal(0.60))
		Expect(usageFraction.Marks.Clear.Inclusive).To(BeFalse())
		Expect(usageFraction.Marks.Unit).To(Equal("fraction"))
		Expect(usageFraction.Marks.Worst).To(Equal(1.0))
		Expect(usageFraction.Extract(Sample{UsageCores: diagnosis.Known(1.4)})).To(Equal(diagnosis.Known(0.35)))
		Expect(usageFraction.Extract(Sample{UsageCores: diagnosis.Unknown()})).To(Equal(diagnosis.Unknown()))

		// The limit arm only exists when the quota is positive, and its clear
		// mark and capacity are denominated in that quota.
		limitSaturation := t.Signals[4]
		Expect(limitSaturation.Instruments).To(HaveLen(1))
		limitHeadroom := limitSaturation.Instruments[0]
		Expect(limitHeadroom.Name).To(Equal("limit-headroom"))
		Expect(limitHeadroom.Requires).To(ConsistOf(HasLimit))
		Expect(limitHeadroom.Red.Name).To(Equal("mean"))
		Expect(limitHeadroom.Red.Min).To(Equal(2))
		Expect(limitHeadroom.Marks.Polarity).To(Equal(diagnosis.LowerIsWorse))
		Expect(limitHeadroom.Marks.Fire.At).To(Equal(0.0))
		Expect(limitHeadroom.Marks.Clear.At).To(Equal(0.1))
		Expect(limitHeadroom.Marks.Unit).To(Equal("cores"))
		Expect(limitHeadroom.Marks.Worst).To(Equal(-0.2), "the reserve, not the quota: severity 1 at −0.2 cores, the floor of quota−usage−0.10×quota")
		Expect(limitHeadroom.Extract(Sample{UsageCores: diagnosis.Known(0.8)})).To(Equal(diagnosis.Known(1.0)))
		Expect(limitHeadroom.Extract(Sample{UsageCores: diagnosis.Unknown()})).To(Equal(diagnosis.Unknown()))

		// Both tracks, the folds no instrument produces: host-busy and
		// usage-cores, each a 60s mean on every box.
		Expect(t.Tracks).To(HaveLen(2))
		Expect(t.Tracks[0].Name).To(Equal("host-busy"))
		Expect(t.Tracks[0].Red.Name).To(Equal("mean"))
		Expect(t.Tracks[0].Red.Min).To(Equal(2))
		Expect(t.Tracks[0].Span).To(Equal(60 * time.Second))
		Expect(t.Tracks[0].Extract(Sample{HostBusy: diagnosis.Known(0.3)})).To(Equal(diagnosis.Known(0.3)))
		Expect(t.Tracks[1].Name).To(Equal("usage-cores"))
		Expect(t.Tracks[1].Red.Name).To(Equal("mean"))
		Expect(t.Tracks[1].Red.Min).To(Equal(2))
		Expect(t.Tracks[1].Span).To(Equal(60 * time.Second))
		Expect(t.Tracks[1].Extract(Sample{UsageCores: diagnosis.Known(0.7)})).To(Equal(diagnosis.Known(0.7)))

		// No positive quota: the limit row is omitted entirely, the first four
		// signals keep their identity and their order, throttling still declares
		// HasLimit, and the table still constructs into an engine. That
		// construction is the whole point of the omission — Fire{At:0} against
		// Clear{At:0} is a pair NewEngine refuses.
		noLimit := cpuTable(4, 0)
		Expect(noLimit.Signals).To(HaveLen(4))
		for i := 0; i < 4; i++ {
			Expect(noLimit.Signals[i].Name).To(Equal(t.Signals[i].Name))
		}
		Expect(noLimit.Signals[0].Instruments[0].Requires).To(ConsistOf(HasLimit))
		_, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred(), "a no-limit box must still construct an engine")
		_, err = NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
	})

	It("fires the throttle latch above a 0.05 sixty-second ratio and clears it below 0.03", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)

		base := time.Now()
		for i := 0; i <= 60; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				// Periods climb by 1000 every tick; throttled jumps to 200 at
				// tick 1 (ratio 0.20, firing) and then climbs by 1 (so the
				// window's first-to-last ratio falls toward 259/60000 = 0.004 by
				// tick 60, clearing once the window is full).
				NrPeriods:   diagnosis.Known(1000 * float64(i)),
				NrThrottled: diagnosis.Known(throttled(i)),
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			if i == 1 {
				Expect(firedSignalNames(fired)).To(ContainElement("throttling"), "a 0.20 ratio must fire the throttle latch at two samples")
				v, st := engine.Reduction("throttling", "throttle-ratio").Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(Equal(0.2))
			}
			if i == 60 {
				Expect(firedSignalNames(fired)).NotTo(ContainElement("throttling"), "a 0.004 ratio over a full window must clear the throttle latch")
			}
		}
	})

	It("judges steal on the sixty-second p95 once the ring holds twenty entries, and on the mean before that", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		stealSignal := signalNamed(cpuTable(4, 2.0), "steal")

		base := time.Now()
		for i := 0; i < 20; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Steal:       diagnosis.Known(stealReading(i)),
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			if i == 1 {
				Expect(firedSignalNames(fired)).To(ContainElement("steal"), "a single 0.9 spike must fire the mean arm at two samples")
			}
			sel, red, _, avail := engine.Select(stealSignal, env)
			switch {
			case i == 0:
				Expect(avail).To(Equal(diagnosis.NoneReady), "at one sample neither arm is judgeable")
			case i < 19:
				Expect(sel.Name).To(Equal("steal-mean"), "below twenty samples the mean answers the steal question")
			case i == 19:
				// Twenty entries: the p95 becomes usable and selection hands over
				// to it, judging on its OWN value — p95 of one 0.9 and nineteen
				// 0.1s is 0.1, while the mean of the same ring is 0.14.
				Expect(sel.Name).To(Equal("steal-p95"), "from twenty samples the p95 answers the steal question")
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(Equal(0.1), "the handover must judge on the p95's own value, not the mean's")
			}
		}
	})

	It("never selects the p95 instrument below its twenty-sample minimum, whatever the window holds", func() {
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		stealSignal := signalNamed(cpuTable(4, 2.0), "steal")

		for n := 1; n < 20; n++ {
			engine, err := NewEngine(4, 2.0)
			Expect(err).NotTo(HaveOccurred())
			base := time.Now()
			for i := 0; i < n; i++ {
				smp := Sample{
					Timestamp:   base.Add(time.Duration(i) * time.Second),
					CpuScope:    ScopeHost,
					Virtualized: true,
					Steal:       diagnosis.Known(0.1),
				}
				engine.Observe(smp, env, smp.Timestamp)
			}
			sel, _, _, avail := engine.Select(stealSignal, env)
			Expect(sel.Name).NotTo(Equal("steal-p95"), "at n=%d the below-minimum p95 must never be selected", n)
			if n >= 2 {
				Expect(sel.Name).To(Equal("steal-mean"), "at n=%d the mean fallback must be the selected arm", n)
			} else {
				Expect(sel.Name).To(BeEmpty())
				Expect(avail).To(Equal(diagnosis.NoneReady), "at n=1 neither arm has reached its floor")
			}
		}
	})

	It("should not declare a saturation signal on a box whose core count was never readable", func() {
		// A never-readable core count is not a "capable but never fillable"
		// machine — it is a machine that has not declared saturation at all.
		// The readable core count is a construction-time fact, so nothing
		// per-tick adds the capability back. The row must be absent, not
		// present-but-withholding: no signal, no instruments, no window.
		t := cpuTable(0, 2.0)
		Expect(hasSignal(t, sigSaturation)).To(BeFalse(),
			"a box with no readable core count must declare no saturation signal")
	})

	It("withholds both saturation arms when the core count is not positive", func() {
		// The belt-and-braces guard: even a direct call to saturationSignal with
		// a non-positive count must not divide by it — both Extract arms return
		// Unknown, never a Known(+Inf) that would latch a permanent saturation.
		sig := saturationSignal(0)
		Expect(sig.Instruments).To(HaveLen(2))
		host := sig.Instruments[0]
		Expect(host.Extract(Sample{CpuScope: ScopeHost, HostBusy: diagnosis.Known(0.5)})).To(Equal(diagnosis.Unknown()))
		usage := sig.Instruments[1]
		Expect(usage.Extract(Sample{UsageCores: diagnosis.Known(1.4)})).To(Equal(diagnosis.Unknown()))
	})
})

// A worker outside this package cannot reach cpuTable, so it walks Table for the
// Signal values it hands to Engine.Select. These specs hold Table to what
// cpuTable declares and to the engine NewEngine builds: were Table ever to
// answer from a parallel declaration, a worker would poll signals the engine
// keyed no windows under, and every Availability would read NoInstrument
// forever.
var _ = Describe("Table — the exported route to the CPU declaration", func() {
	It("declares what cpuTable declares, and omits limit-saturation when there is no positive quota", func() {
		Expect(tableFingerprint(Table(4, 2.0))).To(Equal(tableFingerprint(cpuTable(4, 2.0))),
			"Table must answer from cpuTable, not a parallel declaration")

		// The no-quota arm, which the fingerprint comparison alone would not
		// pin: both sides could omit the same wrong row and still match.
		noLimit := signalNames(Table(4, 0))
		Expect(noLimit).To(Equal([]string{"throttling", "pressure", "steal", "saturation"}))
		Expect(noLimit).NotTo(ContainElement("limit-saturation"), "a box with no positive quota declares no limit row")
	})

	It("omits the saturation signals through the exact route a worker polls", func() {
		// A worker polls Table, not cpuTable, so the cores<=0 omission has to hold
		// at the exported boundary too: the engine NewEngine builds comes from
		// this same call, and a signal Table advertised while the engine keyed no
		// window under it would read NoInstrument forever. Table(0, 2.0) must drop
		// saturation; Table(0, 0) must drop both saturation rows.
		noCores := signalNames(Table(0, 2.0))
		Expect(noCores).NotTo(ContainElement("saturation"),
			"a cores<=0 box must declare no saturation signal through Table")
		Expect(noCores).To(ContainElement("limit-saturation"),
			"a positive quota keeps the limit row even when the core count was never readable")

		noCoresNoLimit := signalNames(Table(0, 0))
		Expect(noCoresNoLimit).NotTo(ContainElement("saturation"),
			"a cores<=0 no-quota box must declare no saturation signal through Table")
		Expect(noCoresNoLimit).NotTo(ContainElement("limit-saturation"),
			"a cores<=0 no-quota box must declare no limit-saturation signal through Table")
	})

	It("returns a table the real engine accepts on a box with no quota", func() {
		// The quota case is covered where the engine is then observed through;
		// this is the arm no other spec builds an engine from Table for.
		_, err := diagnosis.NewEngine(Table(4, 0))
		Expect(err).NotTo(HaveOccurred(), "a no-limit box must still construct an engine from Table")
	})

	It("hands out signals the engine NewEngine built holds windows for", func() {
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())

		// One tick of an all-absent sample. Every instrument is capable under this
		// environment, so a signal the engine keyed windows under reduces to
		// AllAbsent; a signal it did not know reports NoInstrument, because
		// resolve saw no window to reduce. That difference is the drift guard.
		smp := Sample{Timestamp: time.Now(), CpuScope: ScopeHost, Virtualized: true}
		engine.Observe(smp, env, smp.Timestamp)

		table := Table(4, 2.0)
		Expect(table.Signals).NotTo(BeEmpty())
		for _, s := range table.Signals {
			_, _, _, avail := engine.Select(s, env)
			Expect(avail).To(Equal(diagnosis.AllAbsent),
				"%s must be a signal the engine keyed windows under", s.Name)
		}
	})
})

// signalNames lists a table's signal names in declaration order, so a spec can
// compare two tables' identity without reaching into their func fields.
func signalNames(t diagnosis.Table[Sample]) []string {
	out := make([]string, 0, len(t.Signals))
	for _, s := range t.Signals {
		out = append(out, s.Name)
	}
	return out
}

// tableFingerprint renders every declared field of a table that is comparable —
// names, tiers, spans, instrument names, reductions and both marks — in
// declaration order.
//
// Names alone are too weak for the drift question. A second declaration that
// listed the same five signal names but hung different instruments, marks or
// reductions off them would read identical by name and behave nothing alike, so
// the fingerprint carries the instrument identity and its thresholds too. The
// func fields (Extract, Against) stay out: Go cannot compare them, and a table
// that matches on all of this while differing only in an extractor is a
// different defect than the one this guards.
func tableFingerprint(t diagnosis.Table[Sample]) string {
	var b strings.Builder
	fmt.Fprintf(&b, "interval=%s\n", t.Interval)
	for _, tr := range t.Tracks {
		fmt.Fprintf(&b, "track %s red=%s/%d span=%s\n", tr.Name, tr.Red.Name, tr.Red.Min, tr.Span)
	}
	for _, s := range t.Signals {
		fmt.Fprintf(&b, "signal %s tier=%d external=%t releaseOnAbsent=%t demote=%s\n",
			s.Name, s.Tier, s.External, s.ReleaseOnAbsent, s.DemoteSpan)
		for _, inst := range s.Instruments {
			fmt.Fprintf(&b, "  inst %s requires=%v red=%s/%d span=%s counter=%t boolean=%t hasAgainst=%t marks=%+v\n",
				inst.Name, inst.Requires, inst.Red.Name, inst.Red.Min, inst.Span,
				inst.Counter, inst.Boolean, inst.Against != nil, inst.Marks)
		}
	}
	return b.String()
}

// throttled returns the nr_throttled counter for tick i: 0 at the first sample,
// 200 at the second (a 0.20 ratio over the first two), then +1 per tick so the
// window's first-to-last ratio decays below the 0.03 clear mark.
func throttled(i int) float64 {
	if i == 0 {
		return 0
	}
	return 200 + float64(i-1)
}

// stealReading returns one 0.9 spike at the first sample and low readings
// afterwards, so the two-sample mean fires and the twenty-sample p95 is 0.1.
func stealReading(i int) float64 {
	if i == 0 {
		return 0.9
	}
	return 0.1
}

// firedSignalNames collects the signal names of a fired set, so a spec can
// assert a latch is or is not firing without depending on the set's order.
func firedSignalNames(fired []diagnosis.Fired) []string {
	out := make([]string, 0, len(fired))
	for _, f := range fired {
		out = append(out, f.Identity.Signal)
	}
	return out
}
