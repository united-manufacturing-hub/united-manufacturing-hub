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

package cpuhealth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Scope reports whether a sampler's logical CPU count describes every CPU on
// the machine or only the ones its container may run on.
type Scope int

const (
	// ScopeUnknown means the machine's CPU count could not be read, so the
	// scope cannot be known. It is never a silent ScopeHost on a failing read.
	ScopeUnknown Scope = iota
	// ScopeHost means the container's allowed set covers every machine CPU.
	ScopeHost
	// ScopeAffinity means the container is pinned to a strict subset of the
	// machine's CPUs, so the logical count describes only those.
	ScopeAffinity
)

// Environment capabilities. They are startup facts about this host, distinct
// from per-tick readability, and are folded into the Environment the engine
// selects instruments with.
const (
	// HasVirtualization means the host is a virtual machine.
	HasVirtualization diagnosis.Capability = "cpuhealth.HasVirtualization"
	// HasLimit means the cgroup names a positive CPU quota.
	HasLimit diagnosis.Capability = "cpuhealth.HasLimit"
)

// DeriveEnvironment reads exactly two facts off Sample — whether the host is
// Virtualized, and whether Quota names a POSITIVE number — and builds the
// Environment the engine selects instruments with.
func DeriveEnvironment(s Sample) diagnosis.Environment {
	caps := make([]diagnosis.Capability, 0, 2)
	if s.Virtualized {
		caps = append(caps, HasVirtualization)
	}
	if q, ok := s.Quota.Get(); ok && q > 0 {
		caps = append(caps, HasLimit)
	}
	return diagnosis.NewEnvironment(caps...)
}

// Sampler reads a cgroup's CPU health signals.
type Sampler interface {
	Read(ctx context.Context) (Sample, error)
}

// Sample holds the CPU health readings for one cgroup.
type Sample struct {
	// Quota is present when cpu.max names a positive limit (the capacity in
	// cores), the literal "max" (a present 0.0, a definite no-limit), or a
	// non-positive numeric limit (a present 0.0, never a positive capacity).
	// It is absent no-signal when cpu.max is unreadable or unparsable.
	Quota diagnosis.Reading

	// Pressure is present when cpu.pressure's "some" line yields a readable
	// avg60 this tick (the kernel's 0..100 figure divided by 100 into the 0..1
	// fraction the marks are denominated in). It is absent when that read fails
	// this tick.
	Pressure diagnosis.Reading

	// PsiAvailable is sticky: it is set true on the first successful
	// cpu.pressure read and never cleared, even when a later read fails.
	PsiAvailable bool

	// NrPeriods and NrThrottled come from the SAME cpu.stat read that carries
	// usage. Each is present when its key is in cpu.stat and parses, and
	// unavailable (never a trusted 0) when the key is absent or unparsable.
	NrPeriods   diagnosis.Reading
	NrThrottled diagnosis.Reading

	// UsageUsec is the raw cumulative usage_usec counter from cpu.stat. It is
	// present when the key is in cpu.stat and parses, and unavailable when it
	// is absent or unparsable. The raw total is kept beside the rate so a later
	// throttle-ratio reduction still has the totals.
	UsageUsec diagnosis.Reading

	// UsageCores is the instantaneous usage rate in cores: the delta of
	// cumulative usage_usec across two consecutive reads divided by 1e6 (the
	// microsecond divisor) and by the elapsed seconds between the reads'
	// Timestamps. It is Unknown on the first read (no previous edge to subtract
	// from) and when usage_usec falls (a cumulative counter that falls has been
	// reset) — never a confident zero from no measurement.
	UsageCores diagnosis.Reading

	// HostBusy is the host's busy cores as an instantaneous rate: the delta of
	// the first aggregate "cpu " line's busy jiffies (user+nice+sys+irq+softirq)
	// between this read and the previous one, divided by USER_HZ into seconds
	// and by the interval's elapsed seconds. It is absent on the first read
	// (that read only fixes a baseline), when /proc/stat is unreadable or
	// unparsable, and when the busy counter falls (a host restart has reset it).
	HostBusy diagnosis.Reading

	// Steal is the host's CPU-steal fraction over the last poll interval: the
	// interval's steal-jiffy delta over the interval's total-jiffy (fields 0..7)
	// delta, read off the same first aggregate "cpu " line as HostBusy. It is
	// absent on the baseline read, when that line is unavailable like HostBusy,
	// when the steal counter falls (a reset), and when the denominator did not
	// advance (nothing measured, not a NaN/Inf reading).
	Steal diagnosis.Reading

	// HostCpus is the machine's CPU count: the number of per-CPU (cpu0, cpu1, …)
	// lines in /proc/stat. It is present when that file is readable, and unknown
	// when it is not.
	HostCpus diagnosis.Reading

	// LogicalCpus is the CPUs this process may use: the size of the container's
	// allowed cpuset, which under --cpuset-cpus is a strict subset of the
	// machine's count. It is F6's "2" in "pinned to 2 of 8 CPUs". It is present
	// when the cpuset is readable, and unknown when it is not.
	LogicalCpus diagnosis.Reading

	// CpuScope reports whether the sampler's logical CPU count describes every
	// CPU on the machine (ScopeHost), only the ones its container may run on
	// (ScopeAffinity), or ScopeUnknown when the machine's CPU count cannot be
	// read. It is never ScopeHost on an unreadable count.
	CpuScope Scope

	// Virtualized reports whether this host is a virtual machine. It is resolved
	// once and cached across ticks, not re-read every tick. On x86 the evidence
	// is the "hypervisor" flag in /proc/cpuinfo's flags line; an ARM64 cpuinfo
	// has no flags line at all, so the distinct ARM64 route is the DMI fallback
	// of /sys/class/dmi/id/product_name naming a known hypervisor. An unreadable
	// cpuinfo is no evidence and reads false.
	Virtualized bool

	// Timestamp is the time of this read. Every field off the same read carries
	// the same Timestamp.
	Timestamp time.Time
}

// NewCgroupSampler returns a Sampler reading via fs from base.
func NewCgroupSampler(fs filesystem.Service, base string) Sampler {
	return &cgroupSampler{fs: fs, base: base}
}

type cgroupSampler struct {
	fs           filesystem.Service
	base         string
	psiAvailable bool

	// prevUsage and prevTime are the usage_usec edge and its read timestamp from
	// the previous Read, used to derive the instantaneous usage rate. haveUsage
	// reports whether the previous edge exists at all (false before the first
	// successful usage read).
	prevUsage float64
	prevTime  time.Time
	haveUsage bool

	// prevHostBusy, prevHostSteal and prevHostDenom are the raw jiffy totals of
	// the previous /proc/stat read, and prevHostTime its timestamp. These give
	// the interval edges from which the instantaneous host-busy rate and the
	// per-interval steal fraction are derived. haveHost reports whether a
	// previous Read has fixed the /proc/stat baseline; only a read after that
	// publishes host signals, and a falling counter (a host restart) re-baselines
	// instead of publishing a nonsense value.
	prevHostBusy  float64
	prevHostSteal float64
	prevHostDenom float64
	prevHostTime  time.Time
	haveHost      bool

	// virtualized is the sticky virtualisation fact, resolved on the first
	// successful /proc/cpuinfo read and re-published without re-reading. An
	// unreadable cpuinfo resolves false but is not cached, so a later readable
	// one is still considered.
	virtualized  bool
	virtResolved bool
}

// userHz matches the kernel's USER_HZ: the tick rate dividing /proc/stat jiffies
// into seconds, hence cores.
const userHz = 100.0

// readHost reads the first aggregate "cpu " line of /proc/stat and yields the
// raw busy, steal and denominator jiffy totals. busy counts user, nice, system,
// irq and softirq jiffies (idle, iowait, steal, guest and guest_nice excluded).
// The steal denominator is the sum of fields 0..7 only, since the kernel folds
// guest and guest_nice into user and nice. Both totals are kept raw so the
// caller can derive interval deltas. machine is the number of per-CPU (cpu0,
// cpu1, …) lines in the same file, the machine's CPU count. The trailing space
// in "cpu " keeps the aggregate line from matching cpu0/cpu1.
func (s *cgroupSampler) readHost(ctx context.Context) (busy, steal, denom, machine float64, ok bool) {
	data, err := s.fs.ReadFile(ctx, "/proc/stat")
	if err != nil {
		return 0, 0, 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		// A per-CPU line is "cpu" followed by a digit; the aggregate "cpu " line
		// (space, not digit) is not one of them.
		if len(line) > 3 && strings.HasPrefix(line, "cpu") && line[3] >= '0' && line[3] <= '9' {
			machine++
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line) // fields[0] == "cpu"
		if len(fields) < 9 {
			return 0, 0, 0, machine, false
		}
		vals := make([]float64, len(fields))
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return 0, 0, 0, machine, false
			}
			vals[i] = v
		}
		busy := vals[1] + vals[2] + vals[3] + vals[6] + vals[7]
		denom := vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7] + vals[8]
		return busy, vals[8], denom, machine, true
	}
	return 0, 0, 0, machine, false
}

// readCpuset counts the CPUs in the cgroup's effective cpuset when it is a
// single inclusive range such as "0-3". The second value reports whether the
// set was readable and parsed as such a range.
func (s *cgroupSampler) readCpuset(ctx context.Context) (int, bool) {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpuset.cpus.effective")
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, false
	}
	// The cpuset is a comma-separated list of ranges and single ids — "0-3",
	// "0,2,4", "0-1,4-5" — the shapes the scheduler emits when it pins a pod
	// to non-contiguous CPUs, which is F6's primary target. Count every id it
	// names, so any shape collapses to the size of the allowed set.
	var count int
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, false
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || hi < lo {
				return 0, false
			}
			count += hi - lo + 1
		} else {
			if _, err := strconv.Atoi(part); err != nil {
				return 0, false
			}
			count++
		}
	}
	return count, true
}

// readPSI reads cpu.pressure's "some" avg60 as a 0..1 fraction. The second
// value reports whether a present Pressure Reading was produced this tick.
func (s *cgroupSampler) readPSI(ctx context.Context) (float64, bool) {
	data, err := s.fs.ReadFile(ctx, s.base+"/cpu.pressure")
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg60=") {
				v, err := strconv.ParseFloat(strings.TrimPrefix(field, "avg60="), 64)
				if err != nil {
					// An unparsable avg60 is no pressure this tick, matching the
					// unparsable cpu.max no-signal handling: never a present 0.0.
					return 0, false
				}
				return v / 100.0, true
			}
		}
	}
	return 0, false
}

// resolveVirtualized returns the sticky virtualisation fact. On the first call
// it reads /proc/cpuinfo and, when the flags line carries "hypervisor", caches
// and returns true for every later read. The distinct ARM64 route is the DMI
// fallback: an ARM64 cpuinfo has a Features line and no flags line, so the
// primary path can never succeed there and only the DMI product_name match can
// mark it a guest. The DMI route is consulted whenever the cpuinfo route did
// not already prove virtualization — including when cpuinfo is unreadable — and
// a read failure on either source leaves the fact unresolved so the next tick
// retries rather than permanently caching Virtualized=false.
func (s *cgroupSampler) resolveVirtualized(ctx context.Context) bool {
	if s.virtResolved {
		return s.virtualized
	}
	data, err := s.fs.ReadFile(ctx, "/proc/cpuinfo")
	if err == nil && cpuinfoHasHypervisorFlag(data) {
		s.virtualized = true
		s.virtResolved = true
		return true
	}
	// ARM64 route. A successful DMI read resolves the fact either way; a failed
	// DMI read leaves it unresolved so the next tick retries. The DMI identity
	// spans two independent sources: product_name and sys_vendor (the cloud
	// hypervisor an ARM64 product_name like "m6g.medium" never names), each read
	// separately so a failure or absence on one never breaks the other.
	pv, pok := s.dmiProductVirtualized(ctx)
	vv, vok := s.dmiVendorVirtualized(ctx)
	if (pok && pv) || (vok && vv) {
		s.virtualized = true
		s.virtResolved = true
		return true
	}
	// product_name read resolved the fact. On a platform whose /proc/cpuinfo
	// has a flags line (x86) product_name alone is authoritative and the result
	// is cached. ARM64 has no flags line and sys_vendor is part of its identity,
	// so a still-unresolved sys_vendor keeps the fact open on the next tick
	// rather than permanently caching false.
	if err == nil && !cpuinfoHasFlagsLine(data) && !vok {
		return false
	}
	s.virtualized = false
	s.virtResolved = true
	return false
}

// cpuinfoHasFlagsLine reports whether /proc/cpuinfo carries a "flags" line at
// all. Its presence marks an x86 kernel, where the DMI product_name alone is an
// authoritative bare-metal identity; an ARM64 cpuinfo has a Features line and
// no flags line, and there sys_vendor is part of the DMI identity.
func cpuinfoHasFlagsLine(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ':'); i >= 0 {
			if strings.TrimSpace(line[:i]) == "flags" {
				return true
			}
		}
	}
	return false
}

// cpuinfoHasHypervisorFlag reports whether /proc/cpuinfo's flags line carries
// the "hypervisor" flag, the x86 evidence of a guest.
func cpuinfoHasHypervisorFlag(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ':'); i >= 0 {
			if strings.TrimSpace(line[:i]) == "flags" {
				for _, f := range strings.Fields(line[i+1:]) {
					if f == "hypervisor" {
						return true
					}
				}
			}
		}
	}
	return false
}

// dmiHypervisorTokens are the vendor substrings (lowercased) that indicate a
// hypervisor in /sys/class/dmi/id/product_name. Matched case-insensitively,
// because SMBIOS product_name casing is firmware-controlled and not reliably
// Title Case. The set matches the range reported by the major hypervisors — an
// ARM64 VM under any of them would otherwise never be marked Virtualized and
// the steal instrument's HasVirtualization capability would stay shut.
var dmiHypervisorTokens = []string{
	"vmware",
	"kvm",
	"xen",
	"hyper-v",
	"virtualbox",
	"qemu",
}

// dmiProductVirtualized is the first ARM64 DMI source. It reports whether
// /sys/class/dmi/id/product_name names a known hypervisor. The second return is
// false when the read failed: an unreadable product_name is no evidence, and
// leaving it unresolved lets the caller (and a later tick) retry rather than
// caching Virtualized=false forever.
func (s *cgroupSampler) dmiProductVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := s.fs.ReadFile(ctx, "/sys/class/dmi/id/product_name")
	return dmiTokenMatch(data, err, dmiHypervisorTokens)
}

// dmiVendorHypervisorTokens are the cloud-vendor substrings (lowercased) that
// indicate a hypervisor in /sys/class/dmi/id/sys_vendor. AWS Graviton/Nitro
// puts its instance type in product_name ("m6g.medium") and the hypervisor
// identity in sys_vendor ("Amazon EC2"); GCP Tau T2A likewise, and QEMU/KVM
// guests report sys_vendor "QEMU" with a "Standard PC" product_name that the
// product tokens never catch. sys_vendor is the second DMI source, read and
// cached independently of product_name.
//
// "microsoft" is deliberately NOT here: sys_vendor "Microsoft Corporation" is
// ambiguous between Azure ARM guests (a VM) and Microsoft Surface machines
// (bare-metal OEM hardware), and a bare-metal Surface misread as a VM is the
// worse error. Azure ARM therefore stays an undetected known-limitation.
var dmiVendorHypervisorTokens = []string{
	"amazon",
	"google compute engine",
	"qemu",
}

// dmiVendorVirtualized is the second ARM64 DMI source. It reports whether
// /sys/class/dmi/id/sys_vendor names a cloud hypervisor, with the same
// read-failure contract as dmiProductVirtualized.
func (s *cgroupSampler) dmiVendorVirtualized(ctx context.Context) (virtualized, resolved bool) {
	data, err := s.fs.ReadFile(ctx, "/sys/class/dmi/id/sys_vendor")
	return dmiTokenMatch(data, err, dmiVendorHypervisorTokens)
}

// dmiTokenMatch reports whether a (possibly failed) DMI source read names a
// known hypervisor token. The second return is false when the read itself
// failed, so the caller leaves the fact unresolved and retries next tick.
func dmiTokenMatch(data []byte, err error, tokens []string) (virtualized, resolved bool) {
	if err != nil {
		return false, false
	}
	lower := strings.ToLower(strings.TrimSpace(string(data)))
	for _, tok := range tokens {
		if strings.Contains(lower, tok) {
			return true, true
		}
	}
	return false, true
}

// parseCounter reads one key's numeric value out of cpu.stat bytes. An absent
// key yields an unavailable Reading — never a trusted 0 — while an unparsable
// value for a present key returns a non-nil error, which fails the whole sample.
func parseCounter(data []byte, key string) (diagnosis.Reading, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			v, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return diagnosis.Unknown(), fmt.Errorf("unparsable %s value %q: %w", key, fields[1], err)
			}
			return diagnosis.Known(v), nil
		}
	}
	return diagnosis.Unknown(), nil
}

// readStat reads cpu.stat once and yields the raw usage total and both throttle
// counters. A non-nil error reports a read OR parse failure of cpu.stat, either
// of which fails the whole sample; each value's Reading is independently present
// or unavailable on success.
func (s *cgroupSampler) readStat(ctx context.Context) (usage, periods, throttled diagnosis.Reading, err error) {
	var data []byte
	data, err = s.fs.ReadFile(ctx, s.base+"/cpu.stat")
	if err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if usage, err = parseCounter(data, "usage_usec"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if periods, err = parseCounter(data, "nr_periods"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	if throttled, err = parseCounter(data, "nr_throttled"); err != nil {
		return diagnosis.Unknown(), diagnosis.Unknown(), diagnosis.Unknown(), err
	}
	return usage, periods, throttled, nil
}

// Read samples the cgroup at base from cpu.max: a positive limit reads as a
// capacity, "max" and non-positive limits as a present no-limit, and an
// unreadable or unparsable cpu.max as absent no-signal.
func (s *cgroupSampler) Read(ctx context.Context) (Sample, error) {
	var smp Sample
	smp.Timestamp = time.Now()

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	if frac, ok := s.readPSI(ctx); ok {
		s.psiAvailable = true
		smp.Pressure = diagnosis.Known(frac)
	} else {
		smp.Pressure = diagnosis.Unknown()
	}
	smp.PsiAvailable = s.psiAvailable

	usage, periods, throttled, statErr := s.readStat(ctx)
	if statErr != nil {
		// cpu.stat is primary: a read failure there fails the WHOLE sample,
		// never a silent drop of the throttle counters as absent no-signal.
		return smp, fmt.Errorf("read %s/cpu.stat: %w", s.base, statErr)
	}
	smp.NrPeriods = periods
	smp.NrThrottled = throttled
	smp.UsageUsec = usage
	if s.haveUsage {
		// A rising cumulative counter over a positive elapsed time derives an
		// instantaneous rate; a falling one has been reset, so no rate.
		if u, ok := usage.Get(); ok && u >= s.prevUsage {
			if elapsed := smp.Timestamp.Sub(s.prevTime).Seconds(); elapsed > 0 {
				smp.UsageCores = diagnosis.Known((u - s.prevUsage) / 1e6 / elapsed)
			}
		}
	}
	if u, ok := usage.Get(); ok {
		s.prevUsage = u
		s.prevTime = smp.Timestamp
		s.haveUsage = true
	}

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	// The same read carries the machine's CPU count, from which the snapshots'
	// CPU scope is derived.
	if busy, steal, denom, machine, ok := s.readHost(ctx); ok {
		// CPU scope: the machine's count is kept on the snapshot, and the scope
		// compares the container's allowed cpuset against it. A readable machine
		// count whose allowed set covers it reads ScopeHost; a pinned subset
		// reads ScopeAffinity; a failed cpuset read on a known machine count is
		// likewise unknown (never a silent host).
		smp.HostCpus = diagnosis.Known(machine)
		// The cpuset read carries the logical CPU count this process may use —
		// F6's "2" — beside the scope. A failed cpuset read leaves the fresh
		// sample's CpuScope as its zero value, ScopeUnknown, and LogicalCpus
		// absent: never a silent ScopeHost on a known machine count.
		if allowed, aok := s.readCpuset(ctx); aok {
			smp.LogicalCpus = diagnosis.Known(float64(allowed))
			if allowed == int(machine) {
				smp.CpuScope = ScopeHost
			} else {
				smp.CpuScope = ScopeAffinity
			}
		}
		if s.haveHost {
			// Busy cores over the interval is the busy-jiffy delta divided by
			// USER_HZ into seconds; the interval's elapsed time turns that into
			// a per-second rate. A falling busy counter (a reset) and a zero
			// elapsed time each publish nothing.
			if busy >= s.prevHostBusy {
				if elapsed := smp.Timestamp.Sub(s.prevHostTime).Seconds(); elapsed > 0 {
					smp.HostBusy = diagnosis.Known((busy - s.prevHostBusy) / userHz / elapsed)
				}
			}
			// The steal fraction is the interval's steal-jiffy delta over the
			// interval's total-jiffy delta. A falling steal counter (a reset)
			// and a non-positive denominator delta (proc/stat did not advance,
			// or is all zeros) publish nothing, never a NaN/Inf reading.
			dDenom := denom - s.prevHostDenom
			if steal >= s.prevHostSteal && dDenom > 0 {
				frac := (steal - s.prevHostSteal) / dDenom
				smp.Steal = diagnosis.Known(frac)
			}
		}
		s.prevHostBusy = busy
		s.prevHostSteal = steal
		s.prevHostDenom = denom
		s.prevHostTime = smp.Timestamp
		s.haveHost = true
	} else {
		// An unreadable machine CPU count reads ScopeUnknown — never a silent
		// ScopeHost, since a pinned idle container misread as host is F6 by
		// another route. HostCpus stays absent (its zero value) here, as the
		// machine's count could not be read to populate it.
		smp.CpuScope = ScopeUnknown
	}

	smp.Virtualized = s.resolveVirtualized(ctx)

	data, err := s.fs.ReadFile(ctx, s.base+"/cpu.max")
	if err != nil {
		// An unreadable cpu.max is no-signal: Quota stays absent.
		return smp, nil
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return smp, nil
	}

	if fields[0] == "max" {
		// Uncapped is a definite no-limit: present, but never a positive capacity.
		smp.Quota = diagnosis.Known(0.0)
		return smp, nil
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return smp, nil
	}
	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return smp, nil
	}

	if quota > 0 {
		smp.Quota = diagnosis.Known(float64(quota) / float64(period))
	} else {
		// A non-positive limit is never a positive capacity/denominator.
		smp.Quota = diagnosis.Known(0.0)
	}
	return smp, nil
}
