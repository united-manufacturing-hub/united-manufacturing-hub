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
// panics naming the value rather than rounding it; chooseCfsPeriodUs shows what
// rounding would cost.
//
// The dependency runs one way. This package imports neither cpuhealth nor
// anything that does, and cpuhealth must not import it. So every number a Box
// writes is stated independently of the constant the sampler reads it back
// with, and a wrong constant on either side shows up as a wrong reading.
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

// cfsPeriodsUs are the cpu.max periods a Box may write, largest first.
var cfsPeriodsUs = []int64{100_000, 10_000, 1_000}

// referenceTick is the tick length NewBox assumes when it picks the CFS period.
// The period is fixed before the first Tick (periodUs says why), and NewBox is
// not told how long the caller's ticks will be. One second is the CPU worker's
// poll interval (pkg/fsmv2/cpu.PollInterval). A caller who ticks at some other length is not silently
// mis-served: Tick re-checks the actual tick against the chosen period and
// panics if it does not divide.
const referenceTick = time.Second

// fixtureEpoch is where a Box's clock starts. It is a fixed instant far from
// any plausible wall clock, so a Timestamp that came from time.Now() instead of
// this clock cannot coincidentally look right.
var fixtureEpoch = time.Date(2020, time.March, 14, 15, 9, 26, 0, time.UTC)

// errUnreadable is what a file a Box does not serve reads as. It is not
// fs.ErrNotExist, so cpuhealth's classifyRead records such a read as ReadError,
// not ReadMissing.
var errUnreadable = errors.New("file not readable")

func unreadable(path string) error {
	return fmt.Errorf("fakebox: %s: %w", path, errUnreadable)
}

// Condition is one tick's steady state of a machine, in the units an operator
// would say out loud.
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

	// CgroupV1 true serves the cgroup v1 layout instead of v2: the cpu and
	// cpuacct controllers in one cpu,cpuacct directory under the base, the way
	// systemd mounts them, and the cpuset controller in a cpuset directory. v1
	// has no per-cgroup cpu.pressure, so PsiPresent must be false. A v1 box and a
	// v2 box stating the same Condition otherwise describe the same machine.
	CgroupV1 bool

	// Unreadable lists absolute paths this machine cannot read, whatever the
	// rest of the condition says. Each entry is matched whole against the path
	// the sampler asks for: "/sys/fs/cgroup/cpu.stat", not "cpu.stat". An entry
	// that is relative, or names no file this box serves, panics: ignored, it
	// would leave a spec asserting against a readable machine.
	Unreadable []string
}

// Box serves one machine's cgroup and /proc files from a Condition, and owns
// the clock the sampler stamps its samples from.
type Box struct {
	clk *clock.Mock

	base string
	cond Condition

	servers map[string]func() string

	// periodUs is the CFS period, chosen once in NewBox. It is fixed for the
	// box's lifetime because cpu.max has already published it and nr_periods
	// has been accruing against it, so changing it mid-run would make the
	// counter's own history inconsistent.
	periodUs int64

	// Cumulative counters, in the units their files carry.
	usageUsec    int64
	nrPeriods    int64
	nrThrottled  int64
	psiTotalUsec int64

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
// across later Set and Tick calls.
//
// A file exists when the box serves it, readable or not. cpuhealth tells v1
// from v2 by which files exist.
func (b *Box) FS() filesystem.Service {
	fs := filesystem.NewMockFileSystem()
	fs.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		return b.readFile(path)
	}
	fs.FileExistsFunc = func(ctx context.Context, path string) (bool, error) {
		_, served := b.servers[path]

		return served, nil
	}

	return fs
}

// shieldedClock hides the mock, so a caller cannot type-assert the clock back
// to *clock.Mock and call Set. A backwards step leaves the tick it lands on
// with no rate: advanceUsageRate and advanceHostRates publish only over a
// positive gap.
type shieldedClock struct{ clock.Clock }

// Clock returns the clock to hand to cpuhealth.NewLinuxSamplerWithClock. Tick
// is the only thing that moves it, and Tick only ever moves it forwards.
func (b *Box) Clock() clock.Clock { return shieldedClock{b.clk} }

// Set changes the condition later ticks accrue at. It does not accrue anything
// itself, so a Set between two reads changes the next tick's rates and not the
// counters already served.
//
// A Throttle the fixed CFS period cannot serve panics at the next Tick, not
// here: whether it is servable depends on the tick length, which Set is not
// told.
func (b *Box) Set(c Condition) {
	validate(c)
	b.checkUnreadable(c)

	if c.CgroupV1 != b.cond.CgroupV1 {
		panic(fmt.Sprintf(
			"fakebox: Set CgroupV1 %t on a box built with CgroupV1 %t: a machine does not change its cgroup hierarchy mid-run; construct a new Box instead",
			c.CgroupV1, b.cond.CgroupV1))
	}

	if c.Cores != b.cond.Cores {
		panic(fmt.Sprintf(
			"fakebox: Set Cores %d on a %d-CPU box: a machine does not gain or lose CPUs mid-run while its /proc/stat counters keep rising; construct a new Box instead",
			c.Cores, b.cond.Cores))
	}

	b.cond = c
}

// Tick accrues d worth of every counter at the current condition and advances
// the clock by d. The package doc says why the two move together.
//
// It panics on a non-positive d. A zero d leaves the next read no elapsed time
// to divide by, and cpuhealth reads a negative one as a cgroup reset, so
// either would serve no rate without failing.
func (b *Box) Tick(d time.Duration) {
	if d <= 0 {
		panic(fmt.Sprintf("fakebox: Tick(%s) must advance time; a clock that moves backwards is not recoverable downstream", d))
	}

	seconds := d.Seconds()

	b.usageUsec += whole("usage_usec over the tick", b.cond.UsageCores*1e6*seconds)

	// Only a positive quota turns on CFS bandwidth control, and the kernel
	// counts nr_periods only while it is on.
	if b.cond.QuotaCores > 0 {
		periods := whole("nr_periods over the tick", float64(d.Microseconds())/float64(b.periodUs))
		throttled := whole("nr_throttled over the tick", b.cond.Throttle*float64(periods))
		b.nrPeriods += periods
		b.nrThrottled += throttled
	}

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

func (b *Box) readFile(path string) ([]byte, error) {
	// Checked before anything else, so a path the box would otherwise serve
	// still fails when the condition says this machine cannot read it.
	for _, p := range b.cond.Unreadable {
		if p == path {
			return nil, unreadable(path)
		}
	}

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
// renders it. It omits /sys/class/dmi/id/sys_vendor, the ARM64 DMI source: the
// x86 cpuinfo a Box serves settles virtualisation without it, and
// read_virtualized_test.go covers the ARM64 route.
func (b *Box) newServers() map[string]func() string {
	servers := map[string]func() string{
		"/proc/stat":                     b.procStat,
		"/proc/cpuinfo":                  b.procCpuinfo,
		"/sys/class/dmi/id/product_name": b.dmiProductName,
	}

	if b.cond.CgroupV1 {
		servers[b.base+"/cpu,cpuacct/cpu.cfs_quota_us"] = b.cfsQuotaUs
		servers[b.base+"/cpu,cpuacct/cpu.cfs_period_us"] = b.cfsPeriodUs
		servers[b.base+"/cpu,cpuacct/cpu.stat"] = b.v1CPUStat
		servers[b.base+"/cpu,cpuacct/cpuacct.usage"] = b.cpuacctUsage
		servers[b.base+"/cpuset/cpuset.effective_cpus"] = b.cpusetEffective

		return servers
	}

	servers[b.base+"/cpu.stat"] = b.cpuStat
	servers[b.base+"/cpu.max"] = b.cpuMax
	servers[b.base+"/cpu.pressure"] = b.cpuPressure
	servers[b.base+"/cpuset.cpus.effective"] = b.cpusetEffective

	return servers
}

// ServablePaths returns every path this box serves, sorted.
func (b *Box) ServablePaths() []string {
	paths := make([]string, 0, len(b.servers))
	for path := range b.servers {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	return paths
}

// checkUnreadable panics on an Unreadable entry that is relative or names no
// file this box serves. Condition.Unreadable says why.
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
func (b *Box) cpuMax() string {
	if b.cond.QuotaCores <= 0 {
		return fmt.Sprintf("max %d\n", b.periodUs)
	}

	quota := whole("cpu.max quota", b.cond.QuotaCores*float64(b.periodUs))

	return fmt.Sprintf("%d %d\n", quota, b.periodUs)
}

// cfsQuotaUs writes the v1 CPU limit's quota half, -1 for no limit.
func (b *Box) cfsQuotaUs() string {
	if b.cond.QuotaCores <= 0 {
		return "-1\n"
	}

	return fmt.Sprintf("%d\n", whole("cpu.cfs_quota_us", b.cond.QuotaCores*float64(b.periodUs)))
}

// cfsPeriodUs writes the v1 CPU limit's period half.
func (b *Box) cfsPeriodUs() string { return fmt.Sprintf("%d\n", b.periodUs) }

// v1CPUStat writes the v1 throttle counters. Unlike v2's cpu.stat it carries no
// usage, and it states the throttled time in nanoseconds.
func (b *Box) v1CPUStat() string {
	return fmt.Sprintf(
		"nr_periods %d\nnr_throttled %d\nthrottled_time %d\n",
		b.nrPeriods, b.nrThrottled, b.nrThrottled*b.periodUs*1000)
}

// cpuacctUsage writes the cgroup's CPU time in nanoseconds, the same counter v2
// writes as usage_usec.
func (b *Box) cpuacctUsage() string { return fmt.Sprintf("%d\n", b.usageUsec*1000) }

// cpuPressure writes the PSI averages at the two decimals the kernel writes.
// validate rejects a Pressure that needs more, so nothing is rounded away here.
func (b *Box) cpuPressure() string {
	avg := b.cond.Pressure * psiScale

	return fmt.Sprintf(
		"some avg10=%.2f avg60=%.2f avg300=%.2f total=%d\nfull avg10=%.2f avg60=%.2f avg300=%.2f total=%d\n",
		avg, avg, avg, b.psiTotalUsec, avg, avg, avg, b.psiTotalUsec)
}

// cpusetEffective writes the CPUs the cgroup may run on.
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

	for i := range b.cond.Cores {
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

// dmiProductName writes a bare-metal SMBIOS product name. A virtualized Box is
// settled by the hypervisor flag in /proc/cpuinfo first, so DMI is never read.
// A bare-metal Box must serve this file: with no readable DMI source the
// sampler re-reads virtualisation every tick and the fact never settles.
func (b *Box) dmiProductName() string { return "PowerEdge R640\n" }

// chooseCfsPeriodUs picks the largest CFS period at which Throttle is a whole
// number of throttled periods over referenceTick.
//
// The period matters because nr_throttled is an integer. At a 100 ms period a
// one-second tick has ten periods, so a Throttle of 0.08 accrues 0.8 of a
// period: served as an integer that is 1, so the throttling signal would
// report a ratio of 0.10 where 0.08 was stated.
// A 10 ms period gives that same tick a hundred periods and 0.08 accrues
// exactly 8.
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

// validate panics on a Condition no machine could be in.
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

	if c.HostBusy+c.Steal > 1 {
		panic(fmt.Sprintf(
			"fakebox: HostBusy %v + Steal %v is %v: they are fractions of the same machine and cannot exceed 1 together",
			c.HostBusy, c.Steal, c.HostBusy+c.Steal))
	}

	if !isWhole(c.Pressure * psiScale * 100) {
		panic(fmt.Sprintf(
			"fakebox: Pressure %v is finer than the two decimals of a percentage cpu.pressure carries; state a multiple of 0.0001",
			c.Pressure))
	}

	if c.Throttle > 0 && c.QuotaCores <= 0 {
		panic(fmt.Sprintf(
			"fakebox: Throttle %v with QuotaCores %v: a cgroup with no quota has no CFS bandwidth control and is never throttled",
			c.Throttle, c.QuotaCores))
	}

	if c.CgroupV1 && c.PsiPresent {
		panic("fakebox: PsiPresent with CgroupV1: a cgroup v1 hierarchy has no per-cgroup cpu.pressure")
	}

	if c.Affinity < 0 || c.Affinity > c.Cores {
		panic(fmt.Sprintf(
			"fakebox: Affinity %d on a %d-CPU machine: a cgroup runs on some of the machine's CPUs, and 0 means all of them",
			c.Affinity, c.Cores))
	}
}

func unitFraction(name string, v float64) {
	if v < 0 || v > 1 {
		panic(fmt.Sprintf("fakebox: %s %v: expected a fraction from 0 to 1", name, v))
	}
}

// wholeTolerance is the slack isWhole allows for float64 representation error,
// which for the decimal fractions a Condition holds is many orders below this.
const wholeTolerance = 1e-6

func isWhole(v float64) bool { return math.Abs(v-math.Round(v)) <= wholeTolerance }

// whole rounds v to the integer the file will carry, and panics naming what
// could not be written when v is not one.
func whole(what string, v float64) int64 {
	if !isWhole(v) {
		panic(fmt.Sprintf(
			"fakebox: %s is %v, which is not a whole number; the kernel writes this counter as an integer, so this condition cannot be served exactly",
			what, v))
	}

	return int64(math.Round(v))
}
