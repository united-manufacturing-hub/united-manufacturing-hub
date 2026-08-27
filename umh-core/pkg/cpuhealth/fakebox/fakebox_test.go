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

// What a Box promises: a Condition stated in operator units comes back out of
// the real sampler as the same numbers. The specs below drive
// cpuhealth.NewLinuxSamplerWithClock over a Box rather than checking the file
// text, because file text that parses is not the same fact as a Sample that
// reads correctly — a wrong field position produces both.
//
// Every assertion is on the SECOND read. The first read only fixes the rate
// baselines the sampler subtracts from, so it publishes no rate at all.
package fakebox_test

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// dmiProductName is the SMBIOS identity file the bare-metal path resolves
// against.
const dmiProductName = "/sys/class/dmi/id/product_name"

// countingFS counts reads per path on the way through to the box. Some facts
// the sampler settles once are invisible in the Sample — a resolved
// virtualisation fact and an unresolved one both read Virtualized false — and
// the read count is the only thing that separates them.
type countingFS struct {
	filesystem.Service

	reads map[string]int
}

func newCountingFS(inner filesystem.Service) *countingFS {
	return &countingFS{Service: inner, reads: map[string]int{}}
}

func (c *countingFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	c.reads[path]++

	return c.Service.ReadFile(ctx, path)
}

var _ = Describe("a machine condition served as cgroup and proc files", func() {
	const base = "/sys/fs/cgroup"

	// tol is tight on purpose. The numbers below are exact on the integer grid
	// the kernel files are written on, so the only slack any of them needs is
	// float64 representation. A loose tolerance here would accept a fixture
	// that rounds a counter and reports a neighbouring value instead.
	const tol = 1e-9

	// known returns a Reading's value and fails the spec when it is absent, so
	// a missing reading reports as itself rather than as a zero value.
	known := func(r diagnosis.Reading, what string) float64 {
		v, ok := r.Get()
		ExpectWithOffset(1, ok).To(BeTrue(), what+" must be a present reading")

		return v
	}

	// readTwice drives the real sampler across one tick of the box and returns
	// both samples: the baseline read, then the read after the tick.
	readTwice := func(box *fakebox.Box, d time.Duration) (cpuhealth.Sample, cpuhealth.Sample) {
		ctx := context.Background()
		sampler := cpuhealth.NewLinuxSamplerWithClock(box.FS(), base, box.Clock())

		s1, err := sampler.Read(ctx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "the baseline read must succeed")

		box.Tick(d)

		s2, err := sampler.Read(ctx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "the read after the tick must succeed")

		return s1, s2
	}

	It("reads back every number of a busy, throttled, virtualized machine", func() {
		// A four-CPU VM capped at two cores, using 1.2 of them, on a machine
		// that is 60% busy and losing 5% to steal, throttled in 8% of its CFS
		// periods, with PSI reporting a quarter of the time stalled.
		box := fakebox.NewBox(base, fakebox.Condition{
			Cores:       4,
			QuotaCores:  2,
			UsageCores:  1.2,
			HostBusy:    0.60,
			Steal:       0.05,
			Throttle:    0.08,
			Pressure:    0.25,
			PsiPresent:  true,
			Virtualized: true,
		})

		s1, s2 := readTwice(box, time.Second)

		// Tick must move the counters and the clock by the same amount. Every
		// rate below is a counter delta divided by this elapsed time, so a
		// clock that moved differently would scale all of them together and
		// none of the assertions could tell.
		Expect(s2.Timestamp.Sub(s1.Timestamp)).To(Equal(time.Second),
			"one Tick(1s) must advance the sample's Timestamp by exactly one second")

		Expect(known(s2.UsageCores, "UsageCores")).To(BeNumerically("~", 1.2, tol),
			"a cgroup using 1.2 cores must read back as 1.2 cores")

		// HostBusy is in CORES, not a fraction: 60% of a four-CPU machine.
		Expect(known(s2.HostBusy, "HostBusy")).To(BeNumerically("~", 2.4, tol),
			"a machine 60 percent busy across 4 CPUs must read back as 2.4 busy cores")

		Expect(known(s2.Steal, "Steal")).To(BeNumerically("~", 0.05, tol),
			"5 percent steal must read back as the fraction 0.05")

		// The throttle ratio is the two counters' deltas across the window,
		// which is what the throttling instrument's DeltaRatio reduction takes.
		// Asserting the ratio rather than either counter is what catches a
		// fixture that rounds nr_throttled to a neighbouring integer.
		dPeriods := known(s2.NrPeriods, "NrPeriods") - known(s1.NrPeriods, "NrPeriods")
		dThrottled := known(s2.NrThrottled, "NrThrottled") - known(s1.NrThrottled, "NrThrottled")
		Expect(dPeriods).To(BeNumerically(">", 0), "the tick must advance nr_periods")
		Expect(dThrottled/dPeriods).To(BeNumerically("~", 0.08, tol),
			"throttling in 8 percent of CFS periods must read back as the ratio 0.08")

		Expect(known(s2.Pressure, "Pressure")).To(BeNumerically("~", 0.25, tol),
			"PSI some avg60 of a quarter must read back as 0.25")
		Expect(s2.PsiAvailable).To(BeTrue(), "a readable cpu.pressure must set PsiAvailable")

		Expect(known(s2.HostCpus, "HostCpus")).To(BeNumerically("~", 4, tol),
			"a four-CPU machine must read back four machine CPUs")
		Expect(known(s2.LogicalCpus, "LogicalCpus")).To(BeNumerically("~", 4, tol),
			"an unpinned container on a four-CPU machine may use all four")
		Expect(s2.CpuScope).To(Equal(cpuhealth.ScopeHost),
			"a cpuset covering every machine CPU is host scope, not affinity scope")

		Expect(s2.Virtualized).To(BeTrue(),
			"a guest must read back virtualized")

		Expect(known(s2.Quota, "Quota")).To(BeNumerically("~", 2, tol),
			"a two-core cap must read back as a quota of 2 cores")
	})

	It("reads back a bare-metal machine with no PSI, pinned to a subset of its CPUs", func() {
		// The same four-CPU machine, but the kernel publishes no PSI, the host
		// is bare metal, and the container is pinned to two of the four CPUs.
		box := fakebox.NewBox(base, fakebox.Condition{
			Cores:       4,
			QuotaCores:  2,
			UsageCores:  1.2,
			HostBusy:    0.60,
			Steal:       0.05,
			Throttle:    0.08,
			Pressure:    0.25,
			PsiPresent:  false,
			Virtualized: false,
			Affinity:    2,
		})

		_, s2 := readTwice(box, time.Second)

		// Pressure is stated but unreachable: PsiPresent false makes
		// cpu.pressure unreadable, so the stated level must not surface.
		_, ok := s2.Pressure.Get()
		Expect(ok).To(BeFalse(),
			"an unreadable cpu.pressure must leave Pressure absent, never a confident zero")
		Expect(s2.PsiAvailable).To(BeFalse(),
			"a kernel that never published PSI must leave PsiAvailable false")

		Expect(known(s2.LogicalCpus, "LogicalCpus")).To(BeNumerically("~", 2, tol),
			"a container pinned to two CPUs must read back two logical CPUs")
		Expect(known(s2.HostCpus, "HostCpus")).To(BeNumerically("~", 4, tol),
			"pinning does not change the machine's CPU count")
		Expect(s2.CpuScope).To(Equal(cpuhealth.ScopeAffinity),
			"a cpuset smaller than the machine is affinity scope")

		Expect(s2.Virtualized).To(BeFalse(),
			"a bare-metal host must read back not virtualized")
	})

	It("derives the same rates from any servable tick length", func() {
		// The headline property: Tick moves the counters and the clock
		// together. A Box that advanced the clock by d but accrued counters for
		// a hard-coded one second would agree with the 1s case above and be
		// wrong by 2x either side of it.
		//
		// Not every length is servable — this box picks the 10ms CFS period for
		// Throttle 0.08, and 100ms of that is 8/10ths of a throttled period,
		// which panics. The last case of the panic spec below pins that.
		//
		// 250ms and 1.5s are here because 500ms, 1s and 2s are all whole
		// multiples of 100ms and cannot tell a box that quietly rounded ticks
		// to a tenth of a second from one that did not.
		cond := fakebox.Condition{
			Cores:       4,
			QuotaCores:  2,
			UsageCores:  1.2,
			HostBusy:    0.60,
			Steal:       0.05,
			Throttle:    0.08,
			Pressure:    0.25,
			PsiPresent:  true,
			Virtualized: true,
		}

		for _, d := range []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 1500 * time.Millisecond, 2 * time.Second} {
			at := " at a tick of " + d.String()

			s1, s2 := readTwice(fakebox.NewBox(base, cond), d)

			Expect(s2.Timestamp.Sub(s1.Timestamp)).To(Equal(d),
				"the stamp must move by exactly the tick"+at)
			Expect(known(s2.UsageCores, "UsageCores")).To(BeNumerically("~", 1.2, tol),
				"the same condition must read back 1.2 usage cores"+at)
			Expect(known(s2.HostBusy, "HostBusy")).To(BeNumerically("~", 2.4, tol),
				"the same condition must read back 2.4 busy cores"+at)
			Expect(known(s2.Steal, "Steal")).To(BeNumerically("~", 0.05, tol),
				"the same condition must read back 0.05 steal"+at)

			dPeriods := known(s2.NrPeriods, "NrPeriods") - known(s1.NrPeriods, "NrPeriods")
			dThrottled := known(s2.NrThrottled, "NrThrottled") - known(s1.NrThrottled, "NrThrottled")
			Expect(dPeriods).To(BeNumerically(">", 0), "the tick must advance nr_periods"+at)
			Expect(dThrottled/dPeriods).To(BeNumerically("~", 0.08, tol),
				"the same condition must read back a throttle ratio of 0.08"+at)
		}
	})

	It("holds the throttle counters still on a cgroup with no quota", func() {
		// The kernel only runs the CFS period timer for a quota'd cgroup, so an
		// unquota'd one reports nr_periods 0 for its whole life however busy it
		// gets. A fixture that advanced the denominator anyway could not state
		// this machine at all.
		box := fakebox.NewBox(base, fakebox.Condition{
			Cores:      4,
			QuotaCores: 0,
			UsageCores: 1.2,
			HostBusy:   0.60,
			Steal:      0.05,
			Pressure:   0.25,
			PsiPresent: true,
		})

		s1, s2 := readTwice(box, time.Second)

		Expect(known(s1.NrPeriods, "NrPeriods")).To(Equal(0.0),
			"an unquota'd cgroup starts with nr_periods 0")
		Expect(known(s2.NrPeriods, "NrPeriods")).To(Equal(0.0),
			"nr_periods must not advance without a quota, however long the box ticks")
		Expect(known(s2.NrThrottled, "NrThrottled")).To(Equal(0.0),
			"a cgroup with no bandwidth control is never throttled")

		// Everything not gated on the quota still moves.
		Expect(known(s2.Quota, "Quota")).To(Equal(0.0),
			"cpu.max \"max\" is a present no-limit, not an absent reading")
		Expect(known(s2.UsageCores, "UsageCores")).To(BeNumerically("~", 1.2, tol),
			"usage accrues whether or not the cgroup has a quota")
	})

	It("resolves the bare-metal identity from a readable product_name, once", func() {
		bareMetal := fakebox.Condition{
			Cores:      4,
			QuotaCores: 2,
			UsageCores: 1.2,
			HostBusy:   0.60,
			Steal:      0.05,
			Throttle:   0.08,
			Pressure:   0.25,
			PsiPresent: true,
		}
		ctx := context.Background()

		box := fakebox.NewBox(base, bareMetal)
		counted := newCountingFS(box.FS())
		sampler := cpuhealth.NewLinuxSamplerWithClock(counted, base, box.Clock())

		s1, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s1.Virtualized).To(BeFalse())
		Expect(counted.reads[dmiProductName]).To(Equal(1),
			"the bare-metal path must find product_name READABLE; a Sample cannot say so on its own, since an unreadable one also reads Virtualized false")

		// A settled fact is not re-read. This is the property the fixture's
		// comment claims and the reason a bare-metal box has to serve DMI.
		settled := counted.reads[dmiProductName] + counted.reads["/proc/cpuinfo"]
		box.Tick(time.Second)
		_, err = sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(counted.reads[dmiProductName]+counted.reads["/proc/cpuinfo"]).To(Equal(settled),
			"a resolved virtualisation fact must not be read again on the next tick")

		// The control. With product_name unreadable the fact cannot settle, so
		// the sampler retries every tick. Virtualized reads false either way,
		// which is exactly why the count is what has to be measured.
		unresolvable := bareMetal
		unresolvable.Unreadable = []string{dmiProductName}

		box2 := fakebox.NewBox(base, unresolvable)
		counted2 := newCountingFS(box2.FS())
		sampler2 := cpuhealth.NewLinuxSamplerWithClock(counted2, base, box2.Clock())

		_, err = sampler2.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		box2.Tick(time.Second)
		s, err := sampler2.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Virtualized).To(BeFalse(),
			"an unresolvable identity still reads not virtualized, same as the resolved one")
		Expect(counted2.reads["/proc/cpuinfo"]).To(Equal(2),
			"an unresolved fact must be retried every tick, which is what makes the settled case above a real assertion")
	})

	It("makes a listed path unreadable", func() {
		ctx := context.Background()
		readable := fakebox.Condition{
			Cores:      4,
			QuotaCores: 2,
			UsageCores: 1.2,
			HostBusy:   0.60,
			Steal:      0.05,
			Throttle:   0.08,
			Pressure:   0.25,
			PsiPresent: true,
		}

		// cpu.stat is the primary file: unreadable, the whole sample fails.
		noStat := readable
		noStat.Unreadable = []string{base + "/cpu.stat"}
		box := fakebox.NewBox(base, noStat)

		_, err := cpuhealth.NewLinuxSamplerWithClock(box.FS(), base, box.Clock()).Read(ctx)
		Expect(err).To(HaveOccurred(),
			"an unreadable cpu.stat must fail the whole sample, not drop a field")

		// /proc/stat is not primary. The sample still succeeds and loses the
		// machine's CPU count, and with it the scope that needs it.
		noProcStat := readable
		noProcStat.Unreadable = []string{"/proc/stat"}
		box2 := fakebox.NewBox(base, noProcStat)

		s, err := cpuhealth.NewLinuxSamplerWithClock(box2.FS(), base, box2.Clock()).Read(ctx)
		Expect(err).NotTo(HaveOccurred(),
			"an unreadable /proc/stat must not fail the sample; only cpu.stat is primary")
		_, ok := s.HostCpus.Get()
		Expect(ok).To(BeFalse(),
			"an unreadable /proc/stat must leave the machine CPU count absent")
		Expect(s.CpuScope).To(Equal(cpuhealth.ScopeUnknown),
			"no machine count means no scope, never a silent host scope")
	})

	It("carries both decimals of the pressure it was given", func() {
		// 0.0625 is 6.25 percent — a pressure that needs both of
		// cpu.pressure's decimals. The 0.25 the other specs state is 25.00
		// percent and would survive being written with none, so none of them
		// can tell this precision from a coarser one.
		box := fakebox.NewBox(base, fakebox.Condition{
			Cores:      4,
			QuotaCores: 2,
			Pressure:   0.0625,
			PsiPresent: true,
		})

		_, s2 := readTwice(box, time.Second)

		Expect(known(s2.Pressure, "Pressure")).To(BeNumerically("~", 0.0625, tol),
			"a pressure of 6.25 percent must survive the file at full precision, not be rounded to 6.2 or 6")
	})

	It("refuses an Unreadable path it would never be asked for", func() {
		// A silently ignored entry is the worst outcome available here: a spec
		// written to prove behaviour under an unreadable cpu.stat would run
		// against a readable one and assert nothing.
		base10 := fakebox.Condition{Cores: 4, QuotaCores: 2, PsiPresent: true}

		with := func(paths ...string) fakebox.Condition {
			c := base10
			c.Unreadable = paths

			return c
		}

		Expect(func() { fakebox.NewBox(base, with("cpu.stat")) }).
			To(PanicWith(ContainSubstring("is not an absolute path")),
				"a cgroup path missing its base is the likely typo, and it must not be ignored")

		Expect(func() { fakebox.NewBox(base, with("/proc/meminfo")) }).
			To(PanicWith(ContainSubstring("names no file this box serves")),
				"listing a file the box never serves would change nothing, so it is a mistake, not a no-op")

		Expect(func() { fakebox.NewBox(base, base10).Set(with("cpu.stat")) }).
			To(PanicWith(ContainSubstring("is not an absolute path")),
				"Set must reject what NewBox rejects")

		// Every path the box claims to serve must actually read, or the table
		// the rejection above is checked against is itself wrong.
		box := fakebox.NewBox(base, base10)
		Expect(box.ServablePaths()).To(HaveLen(7))

		for _, path := range box.ServablePaths() {
			data, err := box.FS().ReadFile(context.Background(), path)
			Expect(err).NotTo(HaveOccurred(), "the box claims to serve "+path+", so it must read")
			Expect(data).NotTo(BeEmpty(), path+" must not read as empty")

			// And each one must then be accepted as an Unreadable entry.
			Expect(func() { fakebox.NewBox(base, with(path)) }).NotTo(Panic(),
				"every servable path must be listable as unreadable: "+path)
		}
	})

	It("hands out a clock the caller cannot move backwards", func() {
		// Returning a clock.Clock is not on its own enough to hide the mock:
		// the dynamic type travels with the interface, so an unwrapped mock
		// would come straight back out of a type assertion, bringing Set with
		// it. fakebox.go's shieldedClock says what a backwards step costs.
		box := fakebox.NewBox(base, fakebox.Condition{Cores: 4, QuotaCores: 2, PsiPresent: true})

		_, recovered := box.Clock().(*clock.Mock)
		Expect(recovered).To(BeFalse(),
			"the mock must not be recoverable from the clock a Box hands out, or the caller can move time backwards")

		// The clock still has to work as a clock, and still has to move under
		// Tick — hiding the mock must not have hidden the time.
		before := box.Clock().Now()
		box.Tick(time.Second)
		Expect(box.Clock().Now().Sub(before)).To(Equal(time.Second),
			"the shielded clock must still advance by exactly the tick")
	})

	It("panics on a machine it cannot serve, naming what it could not serve", func() {
		// Every guard below is otherwise unexercised, which means any of them
		// could be deleted without a spec going red.
		ok := fakebox.Condition{Cores: 4, QuotaCores: 2, UsageCores: 1.2, PsiPresent: true}

		with := func(f func(c *fakebox.Condition)) fakebox.Condition {
			c := ok
			f(&c)

			return c
		}

		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.HostBusy, c.Steal = 0.8, 0.5 }))
		}).To(PanicWith(ContainSubstring("cannot exceed 1 together")),
			"busy and stolen time are fractions of the same machine")

		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.Affinity = 8 }))
		}).To(PanicWith(ContainSubstring("Affinity 8 on a 4-CPU machine")),
			"a cgroup cannot be pinned to CPUs the machine does not have")

		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.Pressure = 0.000005 }))
		}).To(PanicWith(ContainSubstring("two decimals of a percentage")),
			"a pressure finer than cpu.pressure can carry must be refused, not rounded")

		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.Throttle, c.QuotaCores = 0.08, 0 }))
		}).To(PanicWith(ContainSubstring("no CFS bandwidth control")),
			"a cgroup with no quota is never throttled")

		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.Throttle = 0.0001 }))
		}).To(PanicWith(ContainSubstring("not a whole number of throttled periods at any CFS period")),
			"a throttle no CFS period can express must name itself rather than be rounded")

		Expect(func() {
			fakebox.NewBox(base, ok).Set(with(func(c *fakebox.Condition) { c.Cores = 2 }))
		}).To(PanicWith(ContainSubstring("does not gain or lose CPUs mid-run")),
			"dropping Cores mid-run would cut HostCpus while the jiffy totals kept rising")

		Expect(func() {
			fakebox.NewBox(base, ok).Tick(0)
		}).To(PanicWith(ContainSubstring("must advance time")),
			"a clock that does not move forwards is not recoverable downstream")

		// The tick length the CFS period cannot divide. This is the case Set
		// cannot check, because whether a Throttle is servable depends on a
		// tick Set is never told: 0.08 needs the 10ms period, and 100ms of it
		// is 8/10ths of a throttled period.
		Expect(func() {
			fakebox.NewBox(base, with(func(c *fakebox.Condition) { c.Throttle = 0.08 })).Tick(100 * time.Millisecond)
		}).To(PanicWith(ContainSubstring("nr_throttled over the tick")),
			"a tick that would need a fractional nr_throttled must panic rather than round it")
	})
})
