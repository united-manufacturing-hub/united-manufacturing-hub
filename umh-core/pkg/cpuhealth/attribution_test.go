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

// Attribution consults its evidence. A verdict field is not asserted without
// the evidence for it. A full machine is narrowed to a side by our share of
// the machine's busy time, over the same 60 seconds as everything else; an
// internal cause (throttling, the container's own limit budget) attributes
// container whatever that share says; and where the share cannot be measured,
// or sits in the band the two refinements leave between them, it is unknown.
// The saturation family folds to one cause before ranking.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("attribution consults its evidence", func() {
	It("should fire host-full as its own check that stacks on container-limit-full, because a limit is a ceiling and not a reservation", func() {
		// host-cpu-full/host-full-AND-limit: quota 2.0, 4 cores, usage 0.2 -> 1.95
		// and host busy 0.1 -> 3.8 at tick 40. Both arms over their marks, one
		// cause: the host arm's 4 - 3.8 - 1.0 = -0.8, while the limit arm sits
		// at 2 - 1.95 - 0.2 = -0.15. The fold keeps the host arm, so the value
		// is -0.8 and the cause list holds exactly one saturation entry.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 100; i++ {
			usage, hb := 0.2, 0.1
			if i >= 40 {
				usage, hb = 1.95, 3.8
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage),
				HostBusy:    diagnosis.Known(hb),
			}
			verdict, sig := Decide(engine, smp, env)
			if i == 100 {
				Expect(sig.HostFullFired).To(BeTrue(), "the machine-full arm must fire")
				Expect(sig.LimitSaturationFired).To(BeTrue(), "the own-budget arm must fire on top of it")
				Expect(verdict.State).To(Equal(StateDegraded))
				Expect(verdict.Causes).To(HaveLen(1), "the two saturation arms fold to exactly one cause")
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", -0.8, 1e-9), "the folded value is the host arm's 4 - 3.8 - 1.0")
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("cores")))
			}
		}
	})

	It("should not attribute a full machine to the host when our own sustained usage exceeds the host's non-container share", func() {
		// The same scenario at ticks 100+: our usage 1.95 against a machine busy
		// 3.80, a share of 0.5132. That is past container-share's 0.51 fire
		// mark, so we account for most of what the machine is doing and the load
		// is ours.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 100; i++ {
			usage, hb := 0.2, 0.1
			if i >= 40 {
				usage, hb = 1.95, 3.8
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage),
				HostBusy:    diagnosis.Known(hb),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 100 {
				hbm, _ := engine.Measurement(trackHostBusy).Get()
				oum, _ := engine.Measurement(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 3.8, 1e-9))
				Expect(oum).To(BeNumerically("~", 1.95, 1e-9))
				Expect(oum/hbm).To(BeNumerically(">", 0.51), "1.95 / 3.80 = 0.5132, past container-share's fire mark")
				Expect(verdict.Attribution).To(Equal(AttributionContainer), "a machine full on our own load is the container's, not the host's")
			}
		}

		// The middle of the band: a share of exactly one half crosses neither
		// refinement's fire mark, so nothing narrows the full machine to a side
		// and the host-cpu-full signal's own blame answers, which is nobody. Drive
		// hbm 3.2 against oum 1.6 (host-headroom -0.2 fires, and 1.6 / 3.2 is
		// 0.5000) and require unknown. The latches start unfired here, so there
		// is no earlier answer for the band to hold.
		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env2 := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base2 := time.Now()
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base2.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				HostBusy:    diagnosis.Known(3.2),
				UsageCores:  diagnosis.Known(1.6),
				Pressure:    diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
			}
			verdict, sig := Decide(engine2, smp, env2)
			if i == 5 {
				Expect(sig.HostFullFired).To(BeTrue(), "host-headroom 4 - 3.2 - 1.0 = -0.2 fires")
				hbm, _ := engine2.Measurement(trackHostBusy).Get()
				oum, _ := engine2.Measurement(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 3.2, 1e-9))
				Expect(oum).To(BeNumerically("~", 1.6, 1e-9))
				Expect(oum/hbm).To(BeNumerically("~", 0.5, 1e-9), "1.6 / 3.2 is 0.5000 exactly")
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "a share inside the band fires neither refinement, so nothing narrows the blame")
			}
		}
	})

	It("should attribute an internal cause as container, never as host, whatever the split says", func() {
		// The throttling scenarios: host busy 1.00, our usage 0.20 -> the split
		// says host (1.00 > 2 x 0.20 = 0.40). The dominant cause is throttling,
		// which is internal — the kernel capping US against OUR OWN quota — so
		// attribution is container, never host.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				NrPeriods:   diagnosis.Known(1000 * float64(i)),
				NrThrottled: diagnosis.Known(100 * float64(i)),
				HostBusy:    diagnosis.Known(1.0),
				UsageCores:  diagnosis.Known(0.2),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 5 {
				hbm, _ := engine.Measurement(trackHostBusy).Get()
				oum, _ := engine.Measurement(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 1.0, 1e-9))
				Expect(oum).To(BeNumerically("~", 0.2, 1e-9))
				Expect(hbm).To(BeNumerically(">", 2*oum), "the split itself says host")
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindThrottling))
				Expect(verdict.Attribution).To(Equal(AttributionContainer), "an internal cause is the container's whatever the split says")
			}
		}
	})

	It("should report unknown attribution when the host-container split cannot be computed", func() {
		// Host stats absent: the host-busy track has nothing to fold, so the
		// split cannot run, and the host-cpu-full signal answers through the
		// usage-fraction fallback (3.0 / 4 = 0.75 fires). The quota is large
		// enough that the limit arm (8.0 - 3.0 - 0.8 = 4.2 headroom) does not
		// fire, so the fallback is the fold's only member. The dominant cause is
		// host-cpu-full, but the machine-full question has no host evidence, so
		// attribution is unknown.
		engine, err := NewEngine(4, 8.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.0),
				Pressure:    diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
			}
			verdict, sig := Decide(engine, smp, env)
			if i == 5 {
				_, hbState := engine.Measurement(trackHostBusy).Get()
				Expect(hbState).NotTo(Equal(diagnosis.StateValue), "the host-busy mean cannot run with no host stats")
				Expect(sig.NoHostStatsSaturationFired).To(BeTrue())
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("fraction")))
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "a split that cannot run attributes unknown")
			}
		}
	})
})

// shareRun drives two engines over one sequence of samples: one through Decide,
// which is what a caller sees, and one through Engine.Observe, which is the
// only way to read the refinements nested under a fired signal. Decide's own
// state change IS that Observe call, so the two engines see the same history.
type shareRun struct {
	verdicts *diagnosis.Engine[Sample]
	tree     *diagnosis.Engine[Sample]
	at       time.Time
	env      diagnosis.Environment
	// seen holds one entry per tick: the refinements fired under host-cpu-full on
	// that tick, so a spec can assert over a whole run rather than its end.
	seen  [][]string
	scope Scope
}

// newShareRun builds a run on a box with no quota, so the only signal that can
// fire is host-cpu-full and the only thing that can narrow it is a share.
func newShareRun(cores float64, scope Scope) *shareRun {
	verdicts, err := NewEngine(cores, 0)
	Expect(err).NotTo(HaveOccurred())
	tree, err := NewEngine(cores, 0)
	Expect(err).NotTo(HaveOccurred())

	return &shareRun{
		verdicts: verdicts,
		tree:     tree,
		env:      diagnosis.NewEnvironment(),
		at:       time.Now(),
		scope:    scope,
	}
}

// advance runs n one-second ticks at a fixed machine busy time and usage, and
// returns the last tick's verdict with the refinements fired on that tick. Sixty
// ticks flush a 60-second window, so a phase longer than that ends on the new
// share and not on a mean still carrying the old one.
func (r *shareRun) advance(n int, hostBusy, usage float64) (Verdict, []string) {
	var verdict Verdict

	for i := 0; i < n; i++ {
		smp := Sample{
			Timestamp:  r.at,
			CpuScope:   r.scope,
			HostBusy:   diagnosis.Known(hostBusy),
			UsageCores: diagnosis.Known(usage),
		}
		verdict, _ = Decide(r.verdicts, smp, r.env)
		fired, _ := r.tree.Observe(smp, r.env, r.at)
		r.seen = append(r.seen, firedShares(fired))
		r.at = r.at.Add(time.Second)
	}

	return verdict, r.seen[len(r.seen)-1]
}

// firedShares names the refinements fired under the host-cpu-full signal, and
// nil when that signal itself did not fire.
func firedShares(fired []diagnosis.Fired) []string {
	for _, f := range fired {
		if f.Identity.Signal != sigHostCpuFull {
			continue
		}

		out := make([]string, 0, len(f.Refinements))
		for _, ref := range f.Refinements {
			out = append(out, ref.Identity.Signal)
		}

		return out
	}

	return nil
}

var _ = Describe("the share that narrows a full machine to a side", func() {
	It("should withhold the share on a container pinned to a subset of the CPUs", func() {
		// Machine busy 10.0 against our 3.0 is a share of 0.30, well past
		// host-share's 0.49 fire mark. On a pinned container that 0.30 is an
		// artifact: the busy time covers all eight-or-more CPUs of the machine
		// while our usage covers only the ones we may run on. Four cores and
		// usage 3.0 puts usage-fraction at 0.75, so host-cpu-full still fires and
		// the tick still needs a blame.
		pinned := newShareRun(4, ScopeAffinity)
		verdict, refinements := pinned.advance(6, 10.0, 3.0)
		Expect(verdict.Causes).To(HaveLen(1), "usage-fraction 3.0 / 4 = 0.75 fires host-cpu-full")
		Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
		Expect(refinements).To(BeEmpty(), "the share is not a number on a pinned container")
		Expect(verdict.Attribution).To(Equal(AttributionUnknown), "with nothing narrowing it, the host-cpu-full signal's own blame answers")

		// The control: the same two numbers on a host-scoped sample, where the
		// share means what it says.
		whole := newShareRun(4, ScopeHost)
		verdict, refinements = whole.advance(6, 10.0, 3.0)
		Expect(refinements).To(Equal([]string{refHostShare}))
		Expect(verdict.Attribution).To(Equal(AttributionHost))
	})

	It("should hold the side already blamed while the share sits between the two fire marks", func() {
		// Machine busy 3.5 on a four-core box leaves host-headroom at
		// 4 - 3.5 - 1.0 = -0.5, so host-cpu-full is fired throughout and only the
		// share moves. Ninety ticks per phase is a full 60-second window and
		// then some, so each phase ends on its own share.
		run := newShareRun(4, ScopeHost)

		verdict, refinements := run.advance(90, 3.5, 1.40) // share 0.40
		Expect(refinements).To(Equal([]string{refHostShare}), "0.40 is past host-share's 0.49 fire mark")
		Expect(verdict.Attribution).To(Equal(AttributionHost))

		verdict, refinements = run.advance(90, 3.5, 1.75) // share 0.50
		Expect(refinements).To(Equal([]string{refHostShare}), "0.50 is short of host-share's 0.505 clear mark, so it holds")
		Expect(verdict.Attribution).To(Equal(AttributionHost))

		verdict, refinements = run.advance(90, 3.5, 2.10) // share 0.60
		Expect(refinements).To(Equal([]string{refContainerShare}), "0.60 clears host-share and fires container-share")
		Expect(verdict.Attribution).To(Equal(AttributionContainer))

		verdict, refinements = run.advance(90, 3.5, 1.75) // share 0.50 again
		Expect(refinements).To(Equal([]string{refContainerShare}), "the same 0.50 now holds the container, which is the whole point of the band")
		Expect(verdict.Attribution).To(Equal(AttributionContainer))
	})

	It("should never fire both shares on one tick, over a run that crosses the band in both directions", func() {
		// Four phases of ninety ticks each, alternating a share of 0.30 and one
		// of 0.60, so the band is crossed upward twice and downward once. A
		// refinement may not fire again until a whole window has passed since it
		// released, which is why each phase is longer than the window.
		run := newShareRun(4, ScopeHost)
		for _, usage := range []float64{1.05, 2.10, 1.05, 2.10} {
			run.advance(90, 3.5, usage)
		}

		both := make(map[string]bool)
		for tick, refinements := range run.seen {
			Expect(len(refinements)).To(BeNumerically("<=", 1),
				"tick %d fired %v: the two bands do not overlap, so firing either one clears the other", tick, refinements)
			for _, name := range refinements {
				both[name] = true
			}
		}
		Expect(both).To(HaveKey(refHostShare), "the run must reach a share that blames the host, or the sweep asserts nothing")
		Expect(both).To(HaveKey(refContainerShare), "the run must reach a share that blames the container, or the sweep asserts nothing")
	})
})
