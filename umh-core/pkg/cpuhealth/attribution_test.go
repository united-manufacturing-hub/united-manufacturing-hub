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
package cpuhealth

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("attribution consults its evidence", func() {
	It("should fire host-full as its own check that stacks on container-limit-full, because a limit is a ceiling and not a reservation", func() {
		// host-cpu-full AND container-limit-full: quota 2.0, 4 cores, usage
		// 0.2 -> 1.95 and host busy 0.1 -> 3.8 at tick 40. Both signals are over
		// their marks, so both reach the verdict. The machine's headroom is
		// 4 - 3.8 - 1.0 = -0.8 against a worst of -1.0, and the limit's is
		// 2 - 1.95 - 0.2 = -0.15 against a worst of -0.2, so the machine is the
		// more severe of the two and Rank puts it first.
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
				Expect(verdict.State).To(Equal(StateDegraded))
				Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull),
					"the machine-full and own-budget signals are two causes, not one")
				machine := causeOfKind(verdict.Causes, CauseKindHostCpuFull)
				Expect(machine.Instrument).To(Equal(instrumentHostHeadroom), "the machine-full arm must fire")
				Expect(machine.Value).To(BeNumerically("~", -0.8, 1e-9), "the machine's own headroom, 4 - 3.8 - 1.0")
				Expect(machine.Unit).To(Equal(Unit("cores")))
			}
		}
	})

	It("should not attribute a full machine to the host when our own sustained usage exceeds the host's non-container share", func() {
		// The same scenario at ticks 100+: our usage 1.95 against a machine busy
		// 3.80, a share of 0.5132. That is past container-share's 0.51 fire
		// mark, so we account for most of what the machine is doing and the load
		// is ours.
		//
		// No quota, so container-limit-full is not in the table at all. It
		// blames the container by declaration, and a verdict it can rank first
		// would answer AttributionContainer whatever the refinements said.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization)
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
				hbm, _ := engine.Measurement(measurementHostBusy).Get()
				oum, _ := engine.Measurement(measurementUsageCores).Get()
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
			verdict, details := Decide(engine2, smp, env2)
			if i == 5 {
				Expect(details.HostHeadroomCores).To(BeNumerically("~", -0.2, 1e-9), "host-headroom 4 - 3.2 - 1.0 = -0.2 fires")
				Expect(verdict.Causes[0].Instrument).To(Equal(instrumentHostHeadroom))
				hbm, _ := engine2.Measurement(measurementHostBusy).Get()
				oum, _ := engine2.Measurement(measurementUsageCores).Get()
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
				hbm, _ := engine.Measurement(measurementHostBusy).Get()
				oum, _ := engine.Measurement(measurementUsageCores).Get()
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
		// Host stats absent: the host-busy measurement has nothing to reduce, so
		// the split cannot run, and the host-cpu-full signal answers through the
		// usage-fraction fallback (3.0 / 4 = 0.75 fires). No quota and no PSI,
		// which is the only box usage-fraction is allowed to answer on, so
		// host-cpu-full is the only cause. The machine-full question has no
		// host evidence, so attribution is unknown.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimitedVisibility)
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
			verdict, _ := Decide(engine, smp, env)
			if i == 5 {
				_, hbState := engine.Measurement(measurementHostBusy).Get()
				Expect(hbState).NotTo(Equal(diagnosis.StateValue), "the host-busy mean cannot run with no host stats")
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
				Expect(verdict.Causes[0].Instrument).To(Equal(instrumentUsageFraction))
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
	// seen holds one entry per tick, so a spec can assert over a whole run
	// rather than its end.
	seen  []tickShares
	scope Scope
}

// tickShares is one tick's reading of the refinements fired under
// host-cpu-full. hostCpuFull says whether that signal fired at all, which the
// refinements alone cannot: a signal that did not fire and a signal that fired
// with nothing narrowing it both leave the list empty.
type tickShares struct {
	refinements []string
	hostCpuFull bool
}

// newShareRun builds a run on a box with no quota and no PSI, so the only
// signal that can fire is host-cpu-full, the only thing that can narrow it is a
// share, and usage-fraction is allowed to answer where host-headroom cannot.
func newShareRun(cores float64, scope Scope) *shareRun {
	verdicts, err := NewEngine(cores, 0)
	Expect(err).NotTo(HaveOccurred())
	tree, err := NewEngine(cores, 0)
	Expect(err).NotTo(HaveOccurred())

	return &shareRun{
		verdicts: verdicts,
		tree:     tree,
		env:      diagnosis.NewEnvironment(HasLimitedVisibility),
		at:       time.Now(),
		scope:    scope,
	}
}

// advance runs n one-second ticks at a fixed machine busy time and usage, and
// returns the last tick's verdict with the refinements fired on that tick. Sixty
// ticks flush a 60-second window, so a phase longer than that ends on the new
// share and not on a mean still carrying the old one.
func (r *shareRun) advance(n int, hostBusy, usage float64) (Verdict, []string, bool) {
	var verdict Verdict
	var last tickShares

	for i := 0; i < n; i++ {
		smp := Sample{
			Timestamp:  r.at,
			CpuScope:   r.scope,
			HostBusy:   diagnosis.Known(hostBusy),
			UsageCores: diagnosis.Known(usage),
		}
		verdict, _ = Decide(r.verdicts, smp, r.env)
		fired, _ := r.tree.Observe(smp, r.env, r.at)
		refinements, hostCpuFull := firedShares(fired)
		last = tickShares{refinements: refinements, hostCpuFull: hostCpuFull}
		r.seen = append(r.seen, last)
		r.at = r.at.Add(time.Second)
	}

	return verdict, last.refinements, last.hostCpuFull
}

// firedShares names the refinements fired under the host-cpu-full signal, and
// says whether that signal fired at all. The two answers are separate because
// a signal that did not fire has no refinements for the same reason a signal
// narrowed by nothing has none, and the caller needs to tell them apart.
func firedShares(fired []diagnosis.Fired) ([]string, bool) {
	for _, f := range fired {
		if f.Identity.Signal != signalHostCpuFull {
			continue
		}

		out := make([]string, 0, len(f.Refinements))
		for _, ref := range f.Refinements {
			out = append(out, ref.Identity.Signal)
		}

		return out, true
	}

	return nil, false
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
		verdict, refinements, _ := pinned.advance(6, 10.0, 3.0)
		Expect(verdict.Causes).To(HaveLen(1), "usage-fraction 3.0 / 4 = 0.75 fires host-cpu-full")
		Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
		Expect(refinements).To(BeEmpty(), "the share is not a number on a pinned container")
		Expect(verdict.Attribution).To(Equal(AttributionUnknown), "with nothing narrowing it, the host-cpu-full signal's own blame answers")

		// The control: the same two numbers on a host-scoped sample, where the
		// share means what it says.
		whole := newShareRun(4, ScopeHost)
		verdict, refinements, _ = whole.advance(6, 10.0, 3.0)
		Expect(refinements).To(Equal([]string{refinementHostShare}))
		Expect(verdict.Attribution).To(Equal(AttributionHost))
	})

	It("should tell a signal that never fired from one that fired with nothing narrowing it", func() {
		// Both runs end with no refinements, and only the second one has a
		// signal to hang them under. An idle four-core box is 0.1 busy, which
		// leaves host-headroom far from its mark and usage-fraction at 0.025,
		// so nothing fires at all.
		idle := newShareRun(4, ScopeHost)
		verdict, refinements, hostCpuFull := idle.advance(6, 0.1, 0.1)
		Expect(verdict.Causes).To(BeEmpty())
		Expect(hostCpuFull).To(BeFalse(), "an idle box does not fire host-cpu-full")
		Expect(refinements).To(BeEmpty())

		// The pinned container of the spec above: usage-fraction 3.0 / 4 = 0.75
		// fires host-cpu-full, and the share that would narrow it is withheld
		// because it is not a number on a pinned container.
		pinned := newShareRun(4, ScopeAffinity)
		verdict, refinements, hostCpuFull = pinned.advance(6, 10.0, 3.0)
		Expect(verdict.Causes).To(HaveLen(1))
		Expect(hostCpuFull).To(BeTrue(), "a pinned container at 0.75 does fire host-cpu-full")
		Expect(refinements).To(BeEmpty())
	})

	It("should hold the side already blamed while the share sits between the two fire marks", func() {
		// Machine busy 3.5 on a four-core box leaves host-headroom at
		// 4 - 3.5 - 1.0 = -0.5, so host-cpu-full is fired throughout and only the
		// share moves. Ninety ticks per phase is a full 60-second window and
		// then some, so each phase ends on its own share.
		run := newShareRun(4, ScopeHost)

		verdict, refinements, _ := run.advance(90, 3.5, 1.40) // share 0.40
		Expect(refinements).To(Equal([]string{refinementHostShare}), "0.40 is past host-share's 0.49 fire mark")
		Expect(verdict.Attribution).To(Equal(AttributionHost))

		verdict, refinements, _ = run.advance(90, 3.5, 1.75) // share 0.50
		Expect(refinements).To(Equal([]string{refinementHostShare}), "0.50 is short of host-share's 0.505 clear mark, so it holds")
		Expect(verdict.Attribution).To(Equal(AttributionHost))

		verdict, refinements, _ = run.advance(90, 3.5, 2.10) // share 0.60
		Expect(refinements).To(Equal([]string{refinementContainerShare}), "0.60 clears host-share and fires container-share")
		Expect(verdict.Attribution).To(Equal(AttributionContainer))

		verdict, refinements, _ = run.advance(90, 3.5, 1.75) // share 0.50 again
		Expect(refinements).To(Equal([]string{refinementContainerShare}), "the same 0.50 now holds the container, which is the whole point of the band")
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
		for tick, shares := range run.seen {
			Expect(len(shares.refinements)).To(BeNumerically("<=", 1),
				"tick %d fired %v: the two bands do not overlap, so firing either one clears the other", tick, shares.refinements)
			for _, name := range shares.refinements {
				both[name] = true
			}
		}
		Expect(both).To(HaveKey(refinementHostShare), "the run must reach a share that blames the host, or the sweep asserts nothing")
		Expect(both).To(HaveKey(refinementContainerShare), "the run must reach a share that blames the container, or the sweep asserts nothing")
	})

	It("should divide one interval's cgroup usage by the same interval's host busy time, so the share does not depend on when either was read", func() {
		// The share is a quotient of two rates the sampler derives separately:
		// cgroupSource.advanceUsageRate from cpu.stat's usage_usec, hostSource.advanceHostRates
		// from /proc/stat's busy jiffies. Both divide by an elapsed time, and the
		// elapsed time cancels out of the quotient only while it is the same
		// number on both sides. linuxSampler.Read stamps one Timestamp and hands
		// it to both, which is what makes it the same number. A source reading
		// its own clock would leave two divisors that differ by the work done
		// between the two calls, inside a fraction meant to describe one
		// interval.
		//
		// Over the two reads below usage rises by 3.0 core-seconds and the host's
		// busy time by 400 jiffies, which USER_HZ 100 makes 4.0 core-seconds. The
		// share is 3.0 / 4.0 whatever the wall-clock gap between the reads was.
		const cgroupBase = "/sys/fs/cgroup"
		usages := []string{"5000000", "8000000"}
		procStats := []string{
			"cpu  100 0 100 5000 0 0 0 0 0 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\n",
			"cpu  300 0 300 5000 0 0 0 0 0 0\ncpu0 0 0 0 0 0 0 0 0 0 0\ncpu1 0 0 0 0 0 0 0 0 0 0\n",
		}
		read := 0

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case cgroupBase + "/cpu.stat":
				return []byte("usage_usec " + usages[read] + "\nuser_usec 0\nsystem_usec 0\nnr_periods 0\nnr_throttled 0\n"), nil
			case "/proc/stat":
				return []byte(procStats[read]), nil
			case cgroupBase + "/cpuset.cpus.effective":
				// Two CPUs allowed against the two per-CPU lines above, so the
				// sample is host-scoped and containerShare will answer at all.
				return []byte("0-1"), nil
			default:
				return nil, errors.New("unreadable")
			}
		}
		sampler := NewLinuxSampler(fs, cgroupBase)

		ctx := context.Background()
		first, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		read = 1
		second, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(second.Timestamp.After(first.Timestamp)).To(BeTrue(),
			"the two reads must span a positive interval, or neither rate is published and this spec asserts nothing")
		Expect(second.CpuScope).To(Equal(ScopeHost),
			"containerShare withholds off ScopeHost, so a non-host sample would make this spec vacuous")
		usageRate, ok := second.UsageCores.Get()
		Expect(ok).To(BeTrue(), "the second read must publish a usage rate")
		busyRate, ok := second.HostBusy.Get()
		Expect(ok).To(BeTrue(), "the second read must publish a host-busy rate")
		Expect(busyRate).To(BeNumerically(">", 0))
		Expect(usageRate/busyRate).To(BeNumerically("~", 0.75, 1e-12),
			"the two rates were derived over different intervals, so their quotient carries the ratio of two elapsed times")

		share, ok := containerShare(second).Get()
		Expect(ok).To(BeTrue(), "a host-scoped sample with both rates present must yield a share")
		Expect(share).To(BeNumerically("~", 0.75, 1e-12),
			"3.0 core-seconds of cgroup usage against 4.0 core-seconds of host busy time is 0.75, whatever the interval was")
	})
})
