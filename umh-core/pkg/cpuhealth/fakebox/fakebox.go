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

// Package fakebox turns a machine condition stated in operator units into the
// cgroup and /proc files the cpuhealth sampler reads. A test states "four CPUs,
// 60% busy, throttled in 8% of periods" and drives the real sampler over it,
// instead of hand-writing kernel file text whose field positions it has to get
// right, or hand-building a Sample that skips the parsing entirely.
//
// Two rules make the numbers come back out unchanged.
//
// First, the counters and the clock move together. Every rate the sampler
// publishes is a counter delta divided by the gap between two Sample
// Timestamps, so a Box that advanced one without the other would report a
// condition nobody stated. Tick does both, and there is no way to do either
// alone.
//
// Second, the kernel writes these counters as integers, so a stated condition
// that does not land on the integer grid cannot be served. Such a condition
// panics naming the value rather than rounding to the nearest one it can write:
// rounding nr_throttled at a 100 ms period turns a stated 8% throttle into a
// served 10%, which is on the far side of the 5% fire mark, so the signal fires
// with the wrong number while the test still looks like it passed.
//
// The dependency runs one way. This package imports neither cpuhealth nor
// anything that does, and cpuhealth must not import it. Nothing in the compiler
// stops it — the two build either way — so this is a rule here rather than
// something the build enforces. What it buys is that every number a Box writes
// is stated independently of the one the sampler reads it back with, so a wrong
// constant on either side shows up as a wrong reading instead of cancelling out
// against itself.
//
// One route through the sampler is deliberately not modelled: the ARM64 DMI
// fallback, where /sys/class/dmi/id/sys_vendor carries the hypervisor identity
// that an ARM64 product_name never names. A Box always writes an x86
// /proc/cpuinfo with a flags line, so the sampler settles virtualisation before
// it gets there. That route is covered separately and is not this fixture's
// job.
//
// It ships as non-test code, like the filesystem package's MockFileSystem, so
// tests in other packages can use it.
//
// A Box is not safe for concurrent use.
package fakebox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// userHz is the jiffy rate /proc/stat counts in. The host source divides by a
// constant of the same name and the same value, stated over there separately on
// purpose — see the one-way dependency in the package comment.
const userHz = 100

// psiScale is the 0..100 figure the kernel writes cpu.pressure averages as,
// which the PSI reader divides back out into a 0..1 fraction.
const psiScale = 100

// cfsPeriodsUs are the CFS periods a Box may write, largest first. All three
// are legal cpu.max periods; the shorter ones exist so a Throttle that cannot
// be written as a whole number of throttled periods at 100 ms still can at a
// finer grid.
var cfsPeriodsUs = []int64{100_000, 10_000, 1_000}

// referenceTick is the tick length NewBox assumes when it picks the CFS period.
// The period has to be fixed before the first Tick, because cpu.max publishes
// it and nr_periods accrues against it, but NewBox is not told how long the
// caller's ticks will be. One second is the sampler's own cadence. A caller who
// ticks at some other length is not silently mis-served: Tick re-checks the
// actual tick against the chosen period and panics if it does not divide.
const referenceTick = time.Second

// fixtureEpoch is where a Box's clock starts. It is a fixed instant far from
// any plausible wall clock, so a Timestamp that came from time.Now() instead of
// this clock cannot coincidentally look right.
var fixtureEpoch = time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)

// errUnreadable is what a file a Box does not serve reads as. The sampler
// branches on the read failing at all and never on which error it was, so the
// only thing that matters here is that it is non-nil.
var errUnreadable = errors.New("no such file or directory")

// unreadable is the error for one path, wrapping errUnreadable so a caller can
// still match it with errors.Is.
func unreadable(path string) error {
	return fmt.Errorf("fakebox: %s: %w", path, errUnreadable)
}

// Condition is one tick's steady state of a machine, in the units an operator
// would say out loud. It says what the machine IS doing, not what any file
// contains; a Box turns it into the files.
type Condition struct {
	// Cores is the machine's CPU count, the number of per-CPU lines /proc/stat
	// carries.
	Cores int

	// QuotaCores is the cgroup's CPU limit in cores, the figure docker run
	// --cpus or a Kubernetes CPU limit sets. Zero or less writes cpu.max as
	// "max", an explicit no-limit.
	QuotaCores float64

	// UsageCores is this cgroup's own CPU usage over the tick, in cores.
	UsageCores float64

	// HostBusy is how busy the whole machine is, as a fraction from 0 to 1. It
	// is a fraction here and reads back in CORES, because that is the unit the
	// sampler publishes: 0.60 on a four-CPU machine reads back as 2.4.
	HostBusy float64

	// Steal is the fraction of machine jiffies lost to steal, 0 to 1. HostBusy
	// and Steal are fractions of the same total and cannot exceed 1 together.
	Steal float64

	// Throttle is the fraction of CFS periods in which the cgroup was
	// throttled, 0 to 1.
	Throttle float64

	// Pressure is PSI "some" avg60 as a fraction from 0 to 1. It is a LEVEL the
	// kernel reports directly, not a counter, so a Box writes it rather than
	// accruing it: two ticks at the same Pressure serve the same figure.
	Pressure float64

	// PsiPresent false makes cpu.pressure unreadable, which is how a kernel
	// built without PSI, or a cgroup that does not expose it, behaves. Pressure
	// is then unreachable whatever it says.
	PsiPresent bool

	// Virtualized true makes /proc/cpuinfo carry the hypervisor flag.
	Virtualized bool

	// Affinity is how many CPUs the cgroup may run on, the size of
	// cpuset.cpus.effective. Zero means all of Cores, the unpinned case; any
	// smaller number is a pinned container and reads back as affinity scope.
	Affinity int

	// Unreadable lists absolute paths this machine cannot read, whatever the
	// rest of the condition says — an unreadable /proc/stat, a cgroup whose
	// cpu.stat the process may not open. Each entry is matched whole, against
	// the same path the sampler asks for, so a cgroup file needs the base:
	// "/sys/fs/cgroup/cpu.stat", not "cpu.stat". An entry that is relative, or
	// that names no file this box serves, panics rather than being ignored —
	// silently ignoring it would leave a spec asserting against the readable
	// machine it was written to rule out.
	//
	// This is a list rather than a bool per file on purpose. Four bools would
	// all mean the same thing and each would carry the PsiPresent inversion,
	// where false is the interesting value and the zero value reads as the
	// healthy one.
	Unreadable []string
}

// Box serves one machine's cgroup and /proc files from a Condition, and owns
// the clock the sampler stamps its samples from.
type Box struct {
	// clk is the mock the sampler stamps from. It is set once at construction
	// and only advanced after that: a clock that moved backwards produces a
	// negative elapsed time in the admission window downstream, and that window
	// then never opens.
	clk *clock.Mock

	base string
	cond Condition

	// servers is the path-to-renderer table, built once in NewBox because it
	// closes over base. See newServers.
	servers map[string]func() string

	// periodUs is the CFS period, chosen once in NewBox. It is fixed for the
	// box's lifetime because cpu.max has already published it and nr_periods
	// has been accruing against it, so changing it mid-run would make the
	// counter's own history inconsistent.
	periodUs int64

	// The cumulative counters, in the units their files carry. They start at
	// zero and only rise. Tick adds one tick's worth, except that the two
	// throttle counters stand still without a quota — see Tick.
	usageUsec    int64
	nrPeriods    int64
	nrThrottled  int64
	psiTotalUsec int64

	// The /proc/stat jiffy totals. Everything not busy and not stolen lands in
	// idle, which is what keeps the total the sampler divides steal by equal to
	// the whole machine's jiffies for the interval.
	jiffiesUser  int64
	jiffiesIdle  int64
	jiffiesSteal int64
}

// NewBox returns a Box serving the cgroup files under base, in the state
// initial describes. Its counters start at zero and its clock at fixtureEpoch.
// It panics on a condition no machine could be in, and on a Throttle no CFS
// period can express — see chooseCfsPeriodUs.
func NewBox(base string, initial Condition) *Box {
	validate(initial)

	clk := clock.NewMock()
	clk.Set(fixtureEpoch)

	b := &Box{
		clk:      clk,
		base:     base,
		cond:     initial,
		periodUs: chooseCfsPeriodUs(initial.Throttle),
	}
	b.servers = b.newServers()
	b.checkUnreadable(initial)

	return b
}

// FS returns a filesystem service serving this box's files. It reads the box's
// state at each call rather than a snapshot, so one service stays correct
// across later Set and Tick calls. Every path the box does not serve reads as
// an error, which is what the sampler sees on a machine lacking that file.
func (b *Box) FS() filesystem.Service {
	fs := filesystem.NewMockFileSystem()
	fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		return b.readFile(path)
	}

	return fs
}

// shieldedClock hides the mock behind a plain clock.Clock. Returning the
// interface is not on its own enough to hide anything: the dynamic type travels
// with it, so box.Clock().(*clock.Mock) succeeds and hands the caller Set,
// which moves the clock BACKWARDS. That is not a style point. The admission
// window in pkg/fsmv2/cpu subtracts two instants with no lower guard, so one
// backwards step leaves it refusing for the rest of the process. Embedding the
// interface in an unexported struct promotes every read method and leaves no
// way back to the mock.
type shieldedClock struct{ clock.Clock }

// Clock returns the clock to hand to cpuhealth.NewLinuxSamplerWithClock. Tick
// is the only thing that moves it, and Tick only ever moves it forwards.
func (b *Box) Clock() clock.Clock { return shieldedClock{b.clk} }

// Set changes the condition later ticks accrue at. It does not accrue anything
// itself, so a Set between two reads changes the next tick's rates and not the
// counters already served.
//
// Set cannot check the new Throttle against the CFS period this box fixed at
// NewBox, because whether a Throttle is servable depends on the tick length and
// Set is not told it. Tick applies that check against the tick it is actually
// given, and panics there.
func (b *Box) Set(c Condition) {
	validate(c)
	b.checkUnreadable(c)

	// A machine does not lose CPUs and keep its counters. Dropping Cores would
	// cut HostCpus at once while the jiffy totals kept rising from the
	// wider-machine era, and the tick that straddled the change would report
	// more busy cores than the machine now has.
	if c.Cores != b.cond.Cores {
		panic(fmt.Sprintf(
			"fakebox: Set Cores %d on a %d-CPU box: a machine does not gain or lose CPUs mid-run while its /proc/stat counters keep rising; construct a new Box instead",
			c.Cores, b.cond.Cores))
	}

	b.cond = c
}

// Tick accrues d worth of every counter at the current condition AND advances
// the clock by d. The two happen together because the sampler divides a counter
// delta by the gap between two Sample Timestamps: advancing one without the
// other would serve a rate nobody asked for.
//
// It panics on a non-positive d, because a clock that moves backwards produces
// a negative elapsed time downstream that no later tick recovers from.
func (b *Box) Tick(d time.Duration) {
	if d <= 0 {
		panic(fmt.Sprintf("fakebox: Tick(%s) must advance time; a clock that moves backwards is not recoverable downstream", d))
	}

	seconds := d.Seconds()

	b.usageUsec += whole("usage_usec over the tick", b.cond.UsageCores*1e6*seconds)

	// The throttle counters only move while CFS bandwidth control is on, which
	// is what a positive quota turns on. The kernel starts the period timer
	// that increments nr_periods only for a quota'd cgroup, so an unquota'd one
	// reports nr_periods 0 for its whole life however busy it gets.
	//
	// Holding them still here is not only fidelity. A denominator that always
	// advances cannot express a denominator that never does, and that is a real
	// suspected defect in this package: a throttle ratio taken over a stalled
	// nr_periods. A Box with no quota states that machine directly.
	if b.cond.QuotaCores > 0 {
		// Both counters are integers, so a tick producing a fractional count of
		// either cannot be served: rounding nr_throttled changes the throttle
		// RATIO the instrument reads, which is the whole point of the counter.
		periods := whole("nr_periods over the tick", float64(d.Microseconds())/float64(b.periodUs))
		throttled := whole("nr_throttled over the tick", b.cond.Throttle*float64(periods))
		b.nrPeriods += periods
		b.nrThrottled += throttled
	}

	// The machine's jiffies for the interval. Busy and steal are fractions of
	// this total and idle takes the rest, so the denominator the sampler
	// computes off the served line is exactly this total.
	total := whole("/proc/stat jiffies over the tick", float64(b.cond.Cores)*userHz*seconds)
	busy := whole("/proc/stat busy jiffies over the tick", b.cond.HostBusy*float64(total))
	steal := whole("/proc/stat steal jiffies over the tick", b.cond.Steal*float64(total))
	b.jiffiesUser += busy
	b.jiffiesSteal += steal
	b.jiffiesIdle += total - busy - steal

	// cpu.pressure's total is the one counter nothing in cpuhealth reads — only
	// avg60 is parsed — so it is rounded rather than held to the integer grid.
	// Holding it there would reject a Pressure the reader would have served
	// exactly.
	b.psiTotalUsec += int64(math.Round(b.cond.Pressure * 1e6 * seconds))

	b.clk.Add(d)
}

// readFile serves one path, or reports the error the sampler would see for a
// file this machine does not have.
func (b *Box) readFile(path string) ([]byte, error) {
	// Checked before anything else, so a path the box would otherwise serve
	// still fails when the condition says this machine cannot read it.
	for _, p := range b.cond.Unreadable {
		if p == path {
			return nil, unreadable(path)
		}
	}

	// A kernel without PSI has no cpu.pressure to open at all, which is a
	// different fact from the file being listed unreadable and reads the same
	// way to the sampler.
	if path == b.base+"/cpu.pressure" && !b.cond.PsiPresent {
		return nil, unreadable(path)
	}

	render, served := b.servers[path]
	if !served {
		return nil, fmt.Errorf("fakebox: %q is not one of the files this box serves: %w", path, errUnreadable)
	}

	return []byte(render()), nil
}

// newServers builds the table of every file this box serves, mapped to what
// renders it. It is the single source of truth for "servable": readFile
// dispatches on it and checkUnreadable rejects against it, so a path cannot be
// servable to one and unknown to the other.
func (b *Box) newServers() map[string]func() string {
	return map[string]func() string{
		b.base + "/cpu.stat":              b.cpuStat,
		b.base + "/cpu.max":               b.cpuMax,
		b.base + "/cpu.pressure":          b.cpuPressure,
		b.base + "/cpuset.cpus.effective": b.cpusetEffective,
		"/proc/stat":                      b.procStat,
		"/proc/cpuinfo":                   b.procCpuinfo,
		"/sys/class/dmi/id/product_name":  b.dmiProductName,
	}
}

// ServablePaths returns every path this box serves, sorted. Exported so a test
// can walk the whole set rather than repeating it and drifting from it.
func (b *Box) ServablePaths() []string {
	paths := make([]string, 0, len(b.servers))
	for path := range b.servers {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

// checkUnreadable rejects an Unreadable entry that names no file this box could
// ever serve. Without this the entry is silently ignored, so a spec written to
// prove behaviour under an unreadable cpu.stat would run against a perfectly
// readable one and assert nothing — the failure this whole package exists to
// prevent, reached through its own newest field.
func (b *Box) checkUnreadable(c Condition) {
	for _, path := range c.Unreadable {
		if !strings.HasPrefix(path, "/") {
			panic(fmt.Sprintf(
				"fakebox: Unreadable %q is not an absolute path; entries are matched whole against the path the sampler asks for, so a cgroup file needs the base — %q, not %q",
				path, b.base+"/"+path, path))
		}

		if _, served := b.servers[path]; !served {
			panic(fmt.Sprintf(
				"fakebox: Unreadable %q names no file this box serves, so listing it would change nothing; this box serves %v",
				path, b.ServablePaths()))
		}
	}
}

// cpuStat writes the cgroup's CPU accounting. Only usage_usec, nr_periods and
// nr_throttled are read by cpuhealth; the rest are here because a real cpu.stat
// carries them and a fixture missing them would not be one.
func (b *Box) cpuStat() string {
	return fmt.Sprintf(
		"usage_usec %d\nuser_usec %d\nsystem_usec 0\nnr_periods %d\nnr_throttled %d\nthrottled_usec %d\n",
		b.usageUsec, b.usageUsec, b.nrPeriods, b.nrThrottled, b.nrThrottled*b.periodUs)
}

// cpuMax writes the cgroup's CPU limit as the kernel's quota-and-period pair.
// The quota scales with whichever period this box picked, so the cores the
// sampler divides back out are the stated ones at any period.
func (b *Box) cpuMax() string {
	if b.cond.QuotaCores <= 0 {
		return fmt.Sprintf("max %d\n", b.periodUs)
	}

	quota := whole("cpu.max quota", b.cond.QuotaCores*float64(b.periodUs))

	return fmt.Sprintf("%d %d\n", quota, b.periodUs)
}

// cpuPressure writes the PSI averages at the two decimals the kernel writes,
// matching the hand-written fixture in read_psi_test.go. Writing more decimals
// would let a test state a pressure no real box could report, and a test that
// passes has to describe a machine that could exist. validate rejects a
// Pressure this precision cannot carry, so nothing is rounded away here.
func (b *Box) cpuPressure() string {
	avg := b.cond.Pressure * psiScale

	return fmt.Sprintf(
		"some avg10=%.2f avg60=%.2f avg300=%.2f total=%d\nfull avg10=%.2f avg60=%.2f avg300=%.2f total=%d\n",
		avg, avg, avg, b.psiTotalUsec, avg, avg, avg, b.psiTotalUsec)
}

// cpusetEffective writes the CPUs the cgroup may run on, as the contiguous
// range starting at 0 that the kernel writes for a run of CPUs.
func (b *Box) cpusetEffective() string {
	allowed := b.cond.Affinity
	if allowed == 0 {
		allowed = b.cond.Cores
	}

	if allowed == 1 {
		return "0\n"
	}

	return fmt.Sprintf("0-%d\n", allowed-1)
}

// procStat writes the machine's CPU time. The aggregate "cpu " line carries the
// jiffy totals, in the kernel's field order: user, nice, system, idle, iowait,
// irq, softirq, steal, guest, guest_nice. Everything a Condition does not name
// stays zero, so the busy total the sampler sums is exactly the user field.
//
// The per-CPU lines below it are all zeros: nothing parses them, and their
// COUNT is the machine's CPU count.
func (b *Box) procStat() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "cpu  %d 0 0 %d 0 0 0 %d 0 0\n", b.jiffiesUser, b.jiffiesIdle, b.jiffiesSteal)

	for i := 0; i < b.cond.Cores; i++ {
		fmt.Fprintf(&sb, "cpu%d 0 0 0 0 0 0 0 0 0 0\n", i)
	}

	return sb.String()
}

// procCpuinfo writes an x86 cpuinfo. The flags line is what makes this machine
// answerable either way: "hypervisor" among the flags proves a guest, and the
// line's mere presence is what lets a bare-metal verdict be cached instead of
// re-read every tick.
func (b *Box) procCpuinfo() string {
	flags := "fpu vme de pse tsc msr pae mce cx8 apic lm"
	if b.cond.Virtualized {
		flags += " hypervisor"
	}

	return "processor\t: 0\nvendor_id\t: GenuineIntel\nflags\t\t: " + flags + "\n"
}

// dmiProductName writes the SMBIOS product name, and always a bare-metal one.
// There is no hypervisor branch because nothing could reach it: a virtualized
// Box writes the hypervisor flag into /proc/cpuinfo, and the sampler settles
// the fact there and never opens DMI.
//
// A bare-metal box has to serve this file. With no readable DMI source the
// sampler leaves virtualisation unresolved and re-reads it every tick, so a Box
// that errored here would never let the fact settle.
func (b *Box) dmiProductName() string { return "PowerEdge R640\n" }

// chooseCfsPeriodUs picks the largest CFS period at which Throttle is a whole
// number of throttled periods over referenceTick.
//
// The period matters because nr_throttled is an integer. At a 100 ms period a
// one-second tick has ten periods, so a Throttle of 0.08 accrues 0.8 of a
// period: served as an integer that is 1, a ratio of 0.10, which is on the far
// side of the 5% fire mark. The signal then fires with a number nobody stated.
// A 10 ms period gives that same tick a hundred periods and 0.08 accrues
// exactly 8.
//
// A Throttle no period can express panics rather than being served
// approximately, because there is no way to tell a wrong-by-rounding ratio
// apart from a stated one once it is in the file.
func chooseCfsPeriodUs(throttle float64) int64 {
	for _, periodUs := range cfsPeriodsUs {
		periods := float64(referenceTick.Microseconds()) / float64(periodUs)
		if isWhole(throttle * periods) {
			return periodUs
		}
	}

	panic(fmt.Sprintf(
		"fakebox: Throttle %v is not a whole number of throttled periods at any CFS period (100ms, 10ms, 1ms) over a %s tick; state a Throttle that is, such as a multiple of 0.001",
		throttle, referenceTick))
}

// validate rejects a Condition no machine could be in. Each of these would
// otherwise be served as some other condition — a Steal above 1 as a negative
// idle, an Affinity above Cores as a cpuset the machine does not have — and the
// test would then be asserting against a machine it did not describe.
func validate(c Condition) {
	if c.Cores < 1 {
		panic(fmt.Sprintf("fakebox: Cores %d: a machine has at least one CPU", c.Cores))
	}

	if c.UsageCores < 0 {
		panic(fmt.Sprintf("fakebox: UsageCores %v: usage cannot be negative", c.UsageCores))
	}

	unitFraction("HostBusy", c.HostBusy)
	unitFraction("Steal", c.Steal)
	unitFraction("Throttle", c.Throttle)
	unitFraction("Pressure", c.Pressure)

	// Busy and stolen time are both fractions of the machine's jiffies, and
	// what is left over is idle. Together above 1 there is no idle left to take
	// it from.
	if c.HostBusy+c.Steal > 1 {
		panic(fmt.Sprintf(
			"fakebox: HostBusy %v + Steal %v is %v: they are fractions of the same machine and cannot exceed 1 together",
			c.HostBusy, c.Steal, c.HostBusy+c.Steal))
	}

	// cpu.pressure carries two decimals of a percentage. A finer Pressure
	// cannot be written, and this package states what it cannot serve rather
	// than rounding it into something the caller did not ask for.
	if !isWhole(c.Pressure * psiScale * 100) {
		panic(fmt.Sprintf(
			"fakebox: Pressure %v is finer than the two decimals of a percentage cpu.pressure carries; state a multiple of 0.0001",
			c.Pressure))
	}

	// Only a quota'd cgroup can be throttled, because the quota is what turns
	// CFS bandwidth control on. Serving a throttle without one would mean
	// writing nr_throttled against an nr_periods that never moves.
	if c.Throttle > 0 && c.QuotaCores <= 0 {
		panic(fmt.Sprintf(
			"fakebox: Throttle %v with QuotaCores %v: a cgroup with no quota has no CFS bandwidth control and is never throttled",
			c.Throttle, c.QuotaCores))
	}

	if c.Affinity < 0 || c.Affinity > c.Cores {
		panic(fmt.Sprintf(
			"fakebox: Affinity %d on a %d-CPU machine: a cgroup runs on some of the machine's CPUs, and 0 means all of them",
			c.Affinity, c.Cores))
	}
}

// unitFraction panics unless v is a fraction from 0 to 1.
func unitFraction(name string, v float64) {
	if v < 0 || v > 1 {
		panic(fmt.Sprintf("fakebox: %s %v: expected a fraction from 0 to 1", name, v))
	}
}

// wholeTolerance is the slack whole allows for float64 representation. The
// products it checks are computed from decimal fractions, whose error at these
// magnitudes is many orders below this, while the smallest fractional part it
// has to reject is 0.5.
const wholeTolerance = 1e-6

// isWhole reports whether v is an integer up to float64 representation.
func isWhole(v float64) bool { return math.Abs(v-math.Round(v)) <= wholeTolerance }

// whole rounds v to the integer the file will carry, and panics naming what
// could not be written when v is not one. Serving the rounded value instead
// would change the condition rather than fail to express it.
func whole(what string, v float64) int64 {
	if !isWhole(v) {
		panic(fmt.Sprintf(
			"fakebox: %s is %v, which is not a whole number; the kernel writes this counter as an integer, so this condition cannot be served exactly",
			what, v))
	}

	return int64(math.Round(v))
}
