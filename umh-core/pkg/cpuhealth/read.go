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

// The Linux sampler: one tick's cgroup-plus-machine read, built from the
// previous tick's numbers wherever a rate is derived. Read is a composer over
// two sources — cgroupSource (cgroup_source.go) and hostSource
// (host_source.go) — neither of which can see the other's files.

package cpuhealth

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Environment capabilities. They are startup facts about this host, distinct
// from per-tick readability, and are folded into the Environment the engine
// selects instruments with.
const (
	// HasVirtualization means the host is a virtual machine.
	HasVirtualization diagnosis.Capability = "cpuhealth.HasVirtualization"
	// HasLimit means the cgroup names a positive CPU quota — see Sample.Quota
	// for what sets it (docker run --cpus, a Kubernetes CPU limit, a Compose
	// cpus: entry).
	HasLimit diagnosis.Capability = "cpuhealth.HasLimit"
	// HasPressureStats means the kernel ever published PSI (the sticky
	// PsiAvailable). The pressure instrument Requires it, so selection resolves
	// a host whose kernel never reported PSI to NoInstrument, never AllAbsent.
	HasPressureStats diagnosis.Capability = "cpuhealth.HasPressureStats"
	// HasLimitedVisibility means neither HasLimit nor HasPressureStats holds:
	// no quota to judge our own budget against, and no PSI to read the harm
	// off. It is the same condition Details.LimitedVisibility reports, named
	// the same, and it is the gate on the usage-fraction instrument — see
	// hostCpuFullSignal for why that arm needs it.
	HasLimitedVisibility diagnosis.Capability = "cpuhealth.HasLimitedVisibility"
)

// NewLinuxSampler returns a Sampler reading via fs from base.
func NewLinuxSampler(fs filesystem.Service, base string) Sampler {
	return &linuxSampler{
		cgroup: newCgroupSource(fs, base),
		host:   newHostSource(fs),
	}
}

// linuxSampler composes cgroupSource and hostSource into one Sample per tick.
// It holds no accounting state of its own — every sticky fact and baseline
// belongs to whichever source reads the file it is derived from — because it
// exists only to stamp the tick's single Timestamp and derive CPU scope, the
// one fact that needs both sources' reads to compute.
type linuxSampler struct {
	cgroup *cgroupSource
	host   *hostSource
}

// Read samples the cgroup at base from cpu.max, the container's CPU limit: a
// positive limit reads as a capacity, "max" and non-positive limits as a
// present no-limit, and an unreadable or unparsable cpu.max as absent
// no-signal. cpu.max and cpu.stat are the cgroup v2 CPU controller's files,
// documented at
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html.
func (s *linuxSampler) Read(ctx context.Context) (Sample, error) {
	var smp Sample
	smp.Reads = seedReads()

	// First because a cpu.stat failure returns before every read below it, and
	// that event needs this evidence as much as any other.
	s.recordEvidence(ctx, &smp)

	// Stamped once, here, and passed to both sources: neither cgroup nor host
	// calls time.Now() itself, so both rate derivations divide by the same
	// elapsed time and Decide never compares a machine-wide mean against a
	// cgroup mean taken from a different instant.
	ts := time.Now()
	smp.Timestamp = ts

	// cpu.pressure: PSI presence is sticky once seen; this tick's read success
	// is Pressure's own Reading, absent when the read fails this tick.
	frac, psiErr := s.cgroup.readPSI(ctx)
	if psiErr != nil {
		smp.Pressure = diagnosis.Unknown()
	} else {
		s.cgroup.psiAvailable = true
		smp.Pressure = diagnosis.Known(frac)
	}
	smp.record(OpCPUPressure, classifyRead(psiErr))
	smp.PsiAvailable = s.cgroup.psiAvailable

	stat, statErr := s.cgroup.readStat(ctx)
	// Assigned before the early return below: this text is what would not parse.
	smp.CPUStatRaw = stat.Raw
	smp.record(OpCPUStat, statOutcome(stat, statErr))
	if statErr != nil {
		// cpu.stat is primary: a read failure there fails the WHOLE sample,
		// never a silent drop of the throttle counters as absent no-signal.
		return smp, fmt.Errorf("read %s/cpu.stat: %w", s.cgroup.base, statErr)
	}
	smp.NrPeriods = stat.Periods
	smp.NrThrottled = stat.Throttled
	smp.UsageUsec = stat.Usage
	smp.UsageCores = s.cgroup.advanceUsageRate(ts, stat.Usage)

	// Host signals: the first /proc/stat read fixes a baseline and publishes
	// neither; a read after that publishes this tick's instantaneous host-busy
	// rate and per-interval steal fraction, each derived from the delta of two
	// consecutive reads. A falling cumulative counter (a host restart) is a
	// reset: the baseline is re-established and nothing is published this tick.
	// The same read carries the machine's CPU count, from which the snapshots'
	// CPU scope is derived.
	busy, steal, denom, machine, hostErr := s.host.readHost(ctx)
	smp.record(OpProcStat, classifyRead(hostErr))
	if hostErr != nil {
		// An unreadable machine CPU count reads ScopeUnknown — never a silent
		// ScopeHost, since a pinned idle container misread as host would have
		// its host headroom computed by subtracting a host-scoped busy figure
		// from an affinity-scoped count, the invalid subtraction the scope
		// exists to prevent. HostCpus stays absent (its zero value) here, as the
		// machine's count could not be read to populate it.
		smp.CpuScope = ScopeUnknown
	} else {
		smp.HostCpus = diagnosis.Known(machine)
		// Nested under a successful /proc/stat read so the cpuset stays
		// not_attempted when /proc/stat failed: the file was never opened, and
		// recording a failure for it would name the wrong one.
		s.recordCPUScope(ctx, &smp, machine)
		smp.HostBusy, smp.Steal = s.host.advanceHostRates(ts, busy, steal, denom)
	}

	virtualized, cpuinfoOutcome := s.host.readVirtualized(ctx)
	smp.Virtualized = virtualized
	smp.record(OpProcCpuinfo, cpuinfoOutcome)

	quota, cpuMaxOutcome := s.cgroup.readQuota(ctx)
	smp.Quota = quota.Limit
	smp.CPUMaxRaw = quota.Raw
	smp.record(OpCPUMax, cpuMaxOutcome)

	return smp, nil
}

// recordCPUScope compares the container's allowed cpuset against machine, the
// host's CPU count: a covering cpuset reads ScopeHost, a pinned subset reads
// ScopeAffinity. The same read carries LogicalCpus, the "2" in "pinned to 2 of
// 8 CPUs". A failed cpuset read reads ScopeUnknown with LogicalCpus absent,
// never a silent ScopeHost on a known machine count.
func (s *linuxSampler) recordCPUScope(ctx context.Context, smp *Sample, machine float64) {
	allowed, cpusetErr := s.cgroup.readCpuset(ctx)
	smp.record(OpCpusetCPUs, classifyRead(cpusetErr))
	if cpusetErr != nil {
		smp.LogicalCpus = diagnosis.Unknown()
		smp.CpuScope = ScopeUnknown

		return
	}

	smp.LogicalCpus = diagnosis.Known(float64(allowed))
	if allowed == int(machine) {
		smp.CpuScope = ScopeHost

		return
	}

	smp.CpuScope = ScopeAffinity
}

// recordEvidence puts the three evidence reads on smp: the text of
// cgroup.controllers and /proc/self/cgroup, and the base directory's entry
// count. None of them is parsed or judged here.
func (s *linuxSampler) recordEvidence(ctx context.Context, smp *Sample) {
	controllers, controllersOutcome := s.cgroup.readRawFile(ctx, s.cgroup.base+cgroupControllersFile)
	smp.CgroupControllersRaw = controllers
	smp.record(OpCgroupControllers, controllersOutcome)

	procSelf, procSelfOutcome := s.cgroup.readRawFile(ctx, procSelfCgroupPath)
	smp.ProcSelfCgroupRaw = procSelf
	smp.record(OpProcSelfCgroup, procSelfOutcome)

	baseEntries, baseDirOutcome := s.cgroup.readBaseDirEntryCount(ctx)
	smp.BaseDirEntryCount = baseEntries
	smp.record(OpBaseDir, baseDirOutcome)
}

// statOutcome reports a successful read with no usage figure as ReadEmpty,
// since ReadOK would claim a value never produced. A zero-byte file and a
// valueless usage_usec line both land there; the raw text separates them.
func statOutcome(stat statRead, err error) ReadOutcome {
	if err != nil {
		return classifyRead(err)
	}

	if _, ok := stat.Usage.Get(); !ok {
		return ReadEmpty
	}

	return ReadOK
}

// seedReads returns one ReadNotAttempted entry per op, in allReadOps order.
func seedReads() []ReadResult {
	reads := make([]ReadResult, len(allReadOps))
	for i, op := range allReadOps {
		reads[i] = ReadResult{Op: op, Outcome: ReadNotAttempted}
	}
	return reads
}

// record overwrites op's seeded entry. An op absent from allReadOps has no
// entry to overwrite and records nothing.
func (s *Sample) record(op ReadOp, outcome ReadOutcome) {
	for i := range s.Reads {
		if s.Reads[i].Op == op {
			s.Reads[i].Outcome = outcome
			return
		}
	}
}
